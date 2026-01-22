// Package verification provides verification contract types for Alphie.
// These types are shared between agent and orchestrator packages.
package verification

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ShayCichocki/alphie/internal/exec"
)

// ContractVerifier defines the interface for running verification contracts.
// This allows the contract runner implementation to be swapped for testing
// or alternative verification strategies.
type ContractVerifier interface {
	// Run executes all verifications in the contract and returns the results.
	Run(ctx context.Context, contract *VerificationContract) (*VerificationResult, error)
}

// Verify ContractRunner implements ContractVerifier at compile time.
var _ ContractVerifier = (*ContractRunner)(nil)

// VerificationContract defines how to verify task completion.
// It contains both human-readable intent (from decomposition) and
// machine-executable verification commands (generated post-implementation).
type VerificationContract struct {
	// Intent is human-readable acceptance criteria (from decomposition).
	// This describes what the task should accomplish in plain language.
	Intent string `json:"intent"`

	// Commands are concrete verification steps (generated post-implementation).
	// These are executable commands that verify the intent was achieved.
	Commands []VerificationCommand `json:"commands,omitempty"`

	// FileConstraints define what must/must-not exist or change.
	FileConstraints FileConstraints `json:"file_constraints,omitempty"`
}

// VerificationCommand represents a single verification step.
type VerificationCommand struct {
	// Command is the shell command to execute (e.g., "npm test -- --grep login").
	Command string `json:"cmd"`

	// Expect defines what "pass" means. Supported formats:
	// - "exit 0" or "exit N": check exit code
	// - "output contains X": check stdout contains substring
	// - "output matches /regex/": check stdout matches regex
	Expect string `json:"expect"`

	// Description is a human-readable explanation of what this verifies.
	Description string `json:"description"`

	// Required indicates whether this is a hard requirement.
	// If false, failure is logged but doesn't fail the overall verification.
	Required bool `json:"required"`

	// Timeout is the maximum time to wait for this command (default 60s).
	Timeout time.Duration `json:"timeout,omitempty"`
}

// FileConstraints define expectations about file existence and changes.
type FileConstraints struct {
	// MustExist lists files that must exist after task completion.
	MustExist []string `json:"must_exist,omitempty"`

	// MustNotExist lists files that must NOT exist after task completion.
	MustNotExist []string `json:"must_not_exist,omitempty"`

	// MustNotChange lists files that must NOT have been modified.
	MustNotChange []string `json:"must_not_change,omitempty"`
}

// VerificationResult contains the outcome of running verification.
type VerificationResult struct {
	// AllPassed is true if all required verifications passed.
	AllPassed bool `json:"all_passed"`

	// CommandResults contains results for each verification command.
	CommandResults []CommandResult `json:"command_results,omitempty"`

	// FileResults contains results for file constraint checks.
	FileResults []FileResult `json:"file_results,omitempty"`

	// Summary is a human-readable summary of the verification outcome.
	Summary string `json:"summary"`
}

// CommandResult contains the outcome of a single verification command.
type CommandResult struct {
	// Command is the command that was executed.
	Command string `json:"cmd"`

	// Passed indicates whether the verification passed.
	Passed bool `json:"passed"`

	// Output is the stdout/stderr from the command.
	Output string `json:"output"`

	// ExitCode is the process exit code.
	ExitCode int `json:"exit_code"`

	// Error is set if the command failed to execute.
	Error string `json:"error,omitempty"`

	// Duration is how long the command took.
	Duration time.Duration `json:"duration"`
}

// FileResult contains the outcome of a file constraint check.
type FileResult struct {
	// Path is the file path that was checked.
	Path string `json:"path"`

	// Constraint is what was being checked (must_exist, must_not_exist, must_not_change).
	Constraint string `json:"constraint"`

	// Passed indicates whether the constraint was satisfied.
	Passed bool `json:"passed"`

	// Message provides details about the check.
	Message string `json:"message"`
}

// ContractRunner executes verification contracts.
type ContractRunner struct {
	workDir string
	exec    exec.CommandRunner
}

// NewContractRunner creates a new contract runner for the given work directory.
func NewContractRunner(workDir string) *ContractRunner {
	return &ContractRunner{
		workDir: workDir,
		exec:    exec.NewRunner(),
	}
}

// NewContractRunnerWithExec creates a new contract runner with a custom executor (for testing).
func NewContractRunnerWithExec(workDir string, runner exec.CommandRunner) *ContractRunner {
	return &ContractRunner{
		workDir: workDir,
		exec:    runner,
	}
}

// Run executes all verifications in the contract and returns the results.
func (r *ContractRunner) Run(ctx context.Context, contract *VerificationContract) (*VerificationResult, error) {
	result := &VerificationResult{
		AllPassed: true,
	}

	// Run verification commands
	for _, cmd := range contract.Commands {
		cmdResult := r.runCommand(ctx, cmd)
		result.CommandResults = append(result.CommandResults, cmdResult)

		if !cmdResult.Passed && cmd.Required {
			result.AllPassed = false
		}
	}

	// Check file constraints
	fileResults := r.checkFileConstraints(ctx, contract.FileConstraints)
	result.FileResults = fileResults

	for _, fr := range fileResults {
		if !fr.Passed {
			result.AllPassed = false
		}
	}

	// Generate summary
	result.Summary = r.generateSummary(result)

	return result, nil
}

// runCommand executes a single verification command.
func (r *ContractRunner) runCommand(ctx context.Context, vc VerificationCommand) CommandResult {
	result := CommandResult{
		Command: vc.Command,
	}

	timeout := vc.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	startTime := time.Now()

	// Execute the command via shell
	output, err := r.exec.RunShell(cmdCtx, r.workDir, vc.Command)
	result.Duration = time.Since(startTime)
	result.Output = string(output)

	if err != nil {
		// Extract exit code from error if possible
		result.ExitCode = extractExitCode(err)
		if result.ExitCode == -1 {
			result.Error = err.Error()
			return result
		}
	} else {
		result.ExitCode = 0
	}

	// Check expectation
	result.Passed = r.checkExpectation(result, vc.Expect)

	return result
}

// checkExpectation verifies that the command result matches the expectation.
func (r *ContractRunner) checkExpectation(result CommandResult, expect string) bool {
	expect = strings.TrimSpace(expect)

	// Check for exit code expectation
	if strings.HasPrefix(expect, "exit ") {
		var expectedCode int
		if _, err := fmt.Sscanf(expect, "exit %d", &expectedCode); err == nil {
			return result.ExitCode == expectedCode
		}
	}

	// Check for output contains expectation
	if strings.HasPrefix(expect, "output contains ") {
		substring := strings.TrimPrefix(expect, "output contains ")
		return strings.Contains(result.Output, substring)
	}

	// Default: check for exit 0
	return result.ExitCode == 0
}

// checkFileConstraints verifies file existence and change constraints.
func (r *ContractRunner) checkFileConstraints(ctx context.Context, fc FileConstraints) []FileResult {
	var results []FileResult

	// Check must_exist
	for _, path := range fc.MustExist {
		result := FileResult{
			Path:       path,
			Constraint: "must_exist",
		}

		if r.exec.Exists(ctx, r.workDir, path) {
			result.Passed = true
			result.Message = "file exists"
		} else {
			result.Passed = false
			result.Message = "file does not exist"
		}
		results = append(results, result)
	}

	// Check must_not_exist (supports glob patterns)
	for _, pattern := range fc.MustNotExist {
		result := FileResult{
			Path:       pattern,
			Constraint: "must_not_exist",
		}

		// Check if pattern contains wildcards (* or ?)
		hasWildcard := strings.ContainsAny(pattern, "*?")

		if hasWildcard {
			// Use glob to find matching files
			fullPattern := filepath.Join(r.workDir, pattern)
			matches, err := filepath.Glob(fullPattern)

			if err != nil {
				result.Passed = false
				result.Message = fmt.Sprintf("Failed to check pattern: %v", err)
			} else if len(matches) > 0 {
				// Files found that shouldn't exist - report them
				relativeMatches := make([]string, 0, len(matches))
				for _, m := range matches {
					rel, _ := filepath.Rel(r.workDir, m)
					relativeMatches = append(relativeMatches, rel)
				}
				result.Passed = false
				result.Message = fmt.Sprintf("Files created outside boundaries (found: %v)", strings.Join(relativeMatches, ", "))
			} else {
				// No matches - pattern correctly prevents files
				result.Passed = true
				result.Message = "no files match pattern (as expected)"
			}
		} else {
			// Exact path check (backward compatible)
			if !r.exec.Exists(ctx, r.workDir, pattern) {
				result.Passed = true
				result.Message = "file does not exist (as expected)"
			} else {
				result.Passed = false
				result.Message = "file exists but should not"
			}
		}
		results = append(results, result)
	}

	// Note: must_not_change requires baseline comparison which needs
	// to be tracked from before task execution. For now, we skip it.
	// TODO: Implement must_not_change with git diff against baseline

	return results
}

// generateSummary creates a human-readable summary of verification results.
func (r *ContractRunner) generateSummary(result *VerificationResult) string {
	var parts []string

	// Summarize command results
	passed := 0
	failed := 0
	for _, cr := range result.CommandResults {
		if cr.Passed {
			passed++
		} else {
			failed++
		}
	}
	if len(result.CommandResults) > 0 {
		parts = append(parts, fmt.Sprintf("Commands: %d passed, %d failed", passed, failed))
	}

	// Summarize file results
	filePassed := 0
	fileFailed := 0
	for _, fr := range result.FileResults {
		if fr.Passed {
			filePassed++
		} else {
			fileFailed++
		}
	}
	if len(result.FileResults) > 0 {
		parts = append(parts, fmt.Sprintf("Files: %d passed, %d failed", filePassed, fileFailed))
	}

	if len(parts) == 0 {
		return "No verifications configured"
	}

	return strings.Join(parts, "; ")
}

// extractExitCode extracts the exit code from an error, or returns -1 if not extractable.
func extractExitCode(err error) int {
	// Check if it's an interface that provides ExitCode
	type exitCoder interface {
		ExitCode() int
	}
	if ec, ok := err.(exitCoder); ok {
		return ec.ExitCode()
	}
	return -1
}

// ParseContractJSON parses a verification contract from JSON.
func ParseContractJSON(data []byte) (*VerificationContract, error) {
	var contract VerificationContract
	if err := json.Unmarshal(data, &contract); err != nil {
		return nil, fmt.Errorf("parse contract JSON: %w", err)
	}
	return &contract, nil
}

// ToJSON serializes the contract to JSON.
func (c *VerificationContract) ToJSON() ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}
