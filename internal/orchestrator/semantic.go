// Package orchestrator provides task decomposition and coordination.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/exec"
	"github.com/ShayCichocki/alphie/internal/git"
)

// mergeSystemPrompt is the system prompt for the merge conflict resolver.
const mergeSystemPrompt = `You are a merge conflict resolver. Understand the INTENT of each change, not just the text.

When resolving conflicts:
1. Analyze what each branch is trying to accomplish
2. Preserve the intent of both changes when possible
3. If changes are truly incompatible, favor the change that maintains correctness
4. Ensure the merged result compiles and maintains logical consistency
5. Explain your reasoning before providing the merged code`

// mergePromptTemplate is the template for merge requests.
const mergePromptTemplate = `Resolve the following merge conflict.

Branch 1 (%s) changes:
%s

Branch 2 (%s) changes:
%s

Conflict files: %s

Return ONLY a JSON object with this exact structure (no other text):
{
  "merged_files": {
    "path/to/file1.go": "full merged file content...",
    "path/to/file2.go": "full merged file content..."
  },
  "reasoning": "Brief explanation of how conflicts were resolved"
}`

// SemanticMergeResult contains the outcome of a semantic merge operation.
type SemanticMergeResult struct {
	// Success indicates whether the merge completed without needing human intervention.
	Success bool `json:"success"`
	// MergedFiles contains the paths of files that were successfully merged.
	MergedFiles []string `json:"merged_files"`
	// NeedsHuman indicates whether human intervention is required.
	NeedsHuman bool `json:"needs_human"`
	// Reason provides context for the merge outcome (success or failure reason).
	Reason string `json:"reason"`
	// FinalDiff contains the unified diff of the merged changes.
	// Populated only on successful merge.
	FinalDiff string `json:"final_diff,omitempty"`
	// ChangedFiles lists all files that were changed in the merge.
	// Populated only on successful merge.
	ChangedFiles []string `json:"changed_files,omitempty"`
}

// mergeResponse is the JSON structure returned by Claude for merge resolution.
type mergeResponse struct {
	MergedFiles map[string]string `json:"merged_files"`
	Reasoning   string            `json:"reasoning"`
}

// SemanticMerger uses a Claude agent to resolve merge conflicts semantically.
// It applies strict conditions before attempting auto-merge:
// - Changes in disjoint file paths, OR
// - Same file but different functions, OR
// - Both sides pass tests after merge
type SemanticMerger struct {
	// claude is the Claude runner used for merge resolution.
	// Can be either subprocess (ClaudeProcess) or direct API (ClaudeAPIAdapter).
	claude agent.ClaudeRunner
	// repoPath is the path to the git repository.
	repoPath string
	// git provides git operations.
	git git.Runner
	// exec provides command execution.
	exec exec.CommandRunner
}

// NewSemanticMerger creates a new SemanticMerger with the given Claude runner and repository path.
func NewSemanticMerger(claude agent.ClaudeRunner, repoPath string) *SemanticMerger {
	return &SemanticMerger{
		claude:   claude,
		repoPath: repoPath,
		git:      git.NewRunner(repoPath),
		exec:     exec.NewRunner(),
	}
}

// NewSemanticMergerWithRunner creates a new SemanticMerger with custom dependencies (for testing).
func NewSemanticMergerWithRunner(claude agent.ClaudeRunner, repoPath string, runner git.Runner) *SemanticMerger {
	return &SemanticMerger{
		claude:   claude,
		repoPath: repoPath,
		git:      runner,
		exec:     exec.NewRunner(),
	}
}

// NewSemanticMergerWithDeps creates a new SemanticMerger with all custom dependencies (for testing).
func NewSemanticMergerWithDeps(claude agent.ClaudeRunner, repoPath string, gitRunner git.Runner, cmdRunner exec.CommandRunner) *SemanticMerger {
	return &SemanticMerger{
		claude:   claude,
		repoPath: repoPath,
		git:      gitRunner,
		exec:     cmdRunner,
	}
}

// Merge attempts to merge two branches, resolving conflicts in the specified files.
// It follows these steps:
// 1. Get diffs from both branches
// 2. Check strict conditions (disjoint paths, different functions)
// 3. If allowed: prompt Claude with merge instructions
// 4. Validate merged code compiles and tests pass
// 5. If unresolvable: return NeedsHuman = true
func (m *SemanticMerger) Merge(ctx context.Context, branch1, branch2 string, conflictFiles []string) (*SemanticMergeResult, error) {
	// Get diffs from both branches relative to their merge base
	mergeBase, err := m.git.MergeBase(branch1, branch2)
	if err != nil {
		return nil, fmt.Errorf("get merge base: %w", err)
	}

	diff1, err := m.git.DiffBetween(mergeBase, branch1)
	if err != nil {
		return nil, fmt.Errorf("get diff for %s: %w", branch1, err)
	}

	diff2, err := m.git.DiffBetween(mergeBase, branch2)
	if err != nil {
		return nil, fmt.Errorf("get diff for %s: %w", branch2, err)
	}

	// Check if auto-merge is allowed based on strict conditions
	if !m.CanAutoMerge(diff1, diff2) {
		// Conditions not met for safe auto-merge, but we can still try with Claude
		// Only escalate if Claude also fails
	}

	// Extract affected files from diffs
	files1 := extractFilesFromDiff(diff1)
	files2 := extractFilesFromDiff(diff2)

	// Check for disjoint file paths (trivial merge)
	if areDisjoint(files1, files2) {
		return &SemanticMergeResult{
			Success:     true,
			MergedFiles: append(files1, files2...),
			NeedsHuman:  false,
			Reason:      "Changes affect disjoint file paths - trivial merge",
		}, nil
	}

	// For overlapping files, use Claude to resolve conflicts
	prompt := fmt.Sprintf(mergePromptTemplate, branch1, diff1, branch2, diff2, strings.Join(conflictFiles, ", "))

	// Prepend system prompt
	fullPrompt := mergeSystemPrompt + "\n\n" + prompt

	// Start the Claude process with the merge prompt
	if err := m.claude.Start(fullPrompt, m.repoPath); err != nil {
		return nil, fmt.Errorf("start claude process: %w", err)
	}

	// Collect the response
	var response strings.Builder
	for event := range m.claude.Output() {
		select {
		case <-ctx.Done():
			_ = m.claude.Kill()
			return nil, ctx.Err()
		default:
		}

		switch event.Type {
		case agent.StreamEventResult:
			response.WriteString(event.Message)
		case agent.StreamEventAssistant:
			response.WriteString(event.Message)
		case agent.StreamEventError:
			return &SemanticMergeResult{
				Success:    false,
				NeedsHuman: true,
				Reason:     fmt.Sprintf("Claude error during merge: %s", event.Error),
			}, nil
		}
	}

	// Wait for process to complete
	if err := m.claude.Wait(); err != nil {
		return &SemanticMergeResult{
			Success:    false,
			NeedsHuman: true,
			Reason:     fmt.Sprintf("Claude process failed: %v", err),
		}, nil
	}

	// Parse the merge response
	mergeResp, err := parseMergeResponse(response.String())
	if err != nil {
		return &SemanticMergeResult{
			Success:    false,
			NeedsHuman: true,
			Reason:     fmt.Sprintf("Failed to parse Claude response: %v", err),
		}, nil
	}

	// Write merged files to disk
	for filePath, content := range mergeResp.MergedFiles {
		fullPath := filepath.Join(m.repoPath, filePath)
		if err := writeFile(fullPath, content); err != nil {
			return &SemanticMergeResult{
				Success:    false,
				NeedsHuman: true,
				Reason:     fmt.Sprintf("Failed to write merged file %s: %v", filePath, err),
			}, nil
		}
	}

	// Validate the merge - check if code compiles
	if err := m.validateCompiles(ctx); err != nil {
		// Revert changes on validation failure
		_ = m.revertChanges()
		return &SemanticMergeResult{
			Success:    false,
			NeedsHuman: true,
			Reason:     fmt.Sprintf("Merged code does not compile: %v", err),
		}, nil
	}

	// Validate the merge - run tests
	if err := m.validateTests(ctx); err != nil {
		// Revert changes on test failure
		_ = m.revertChanges()
		return &SemanticMergeResult{
			Success:    false,
			NeedsHuman: true,
			Reason:     fmt.Sprintf("Tests fail after merge: %v", err),
		}, nil
	}

	var mergedFiles []string
	for filePath := range mergeResp.MergedFiles {
		mergedFiles = append(mergedFiles, filePath)
	}

	// Stage and commit the merged files
	if err := m.finalizeSemanticMerge(mergedFiles, branch1, branch2, mergeResp.Reasoning); err != nil {
		return &SemanticMergeResult{
			Success:    false,
			NeedsHuman: true,
			Reason:     fmt.Sprintf("Failed to finalize merge: %v", err),
		}, nil
	}

	// Get the final diff and changed files after successful merge
	finalDiff, _ := m.git.DiffBetween("HEAD^", "HEAD")
	changedFiles, _ := m.git.ChangedFilesBetween("HEAD^", "HEAD")

	return &SemanticMergeResult{
		Success:      true,
		MergedFiles:  mergedFiles,
		NeedsHuman:   false,
		Reason:       mergeResp.Reasoning,
		FinalDiff:    finalDiff,
		ChangedFiles: changedFiles,
	}, nil
}

// CanAutoMerge determines if two diffs can be safely auto-merged based on strict conditions.
// Returns true if:
// - Changes affect disjoint file paths, OR
// - Same file but changes are in different functions
func (m *SemanticMerger) CanAutoMerge(diff1, diff2 string) bool {
	files1 := extractFilesFromDiff(diff1)
	files2 := extractFilesFromDiff(diff2)

	// Condition 1: Disjoint file paths
	if areDisjoint(files1, files2) {
		return true
	}

	// Condition 2: Same files but different functions
	funcs1 := extractFunctionsFromDiff(diff1)
	funcs2 := extractFunctionsFromDiff(diff2)

	if areDisjoint(funcs1, funcs2) {
		return true
	}

	// Overlapping changes require Claude intervention
	return false
}

// validateCompiles checks if the code in the repository compiles.
// It detects the project type and uses the appropriate build command.
func (m *SemanticMerger) validateCompiles(ctx context.Context) error {
	info := GetProjectTypeInfo(m.repoPath)

	// If no build command, skip validation (unknown project type or no build script)
	if len(info.BuildCommand) == 0 {
		return nil
	}

	output, err := m.exec.Run(ctx, m.repoPath, info.BuildCommand[0], info.BuildCommand[1:]...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(output))
	}
	return nil
}

// validateTests runs the test suite and returns an error if tests fail.
// It detects the project type and uses the appropriate test command.
func (m *SemanticMerger) validateTests(ctx context.Context) error {
	info := GetProjectTypeInfo(m.repoPath)

	// If no test command, skip validation
	if len(info.TestCommand) == 0 {
		return nil
	}

	output, err := m.exec.Run(ctx, m.repoPath, info.TestCommand[0], info.TestCommand[1:]...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(output))
	}
	return nil
}

// parseMergeResponse parses Claude's JSON response into a mergeResponse.
func parseMergeResponse(response string) (*mergeResponse, error) {
	// Find the JSON object in the response
	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")
	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		return nil, fmt.Errorf("no valid JSON object found in response")
	}
	jsonStr := response[jsonStart : jsonEnd+1]

	var resp mergeResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal JSON: %w", err)
	}

	return &resp, nil
}

// extractFilesFromDiff extracts file paths from a unified diff.
func extractFilesFromDiff(diff string) []string {
	var files []string
	seen := make(map[string]bool)

	// Match "diff --git a/path b/path" or "+++ b/path" patterns
	diffPattern := regexp.MustCompile(`(?m)^diff --git a/(.+?) b/`)
	matches := diffPattern.FindAllStringSubmatch(diff, -1)
	for _, match := range matches {
		if len(match) > 1 && !seen[match[1]] {
			files = append(files, match[1])
			seen[match[1]] = true
		}
	}

	return files
}

// extractFunctionsFromDiff extracts function names that are modified in a diff.
// It looks for Go function definitions in the diff context.
func extractFunctionsFromDiff(diff string) []string {
	var funcs []string
	seen := make(map[string]bool)

	// Match function definition patterns in Go
	// Matches: func Name(...) or func (receiver) Name(...)
	funcPattern := regexp.MustCompile(`(?m)^@@.*@@.*func\s+(?:\([^)]+\)\s+)?(\w+)\s*\(`)
	matches := funcPattern.FindAllStringSubmatch(diff, -1)
	for _, match := range matches {
		if len(match) > 1 && !seen[match[1]] {
			funcs = append(funcs, match[1])
			seen[match[1]] = true
		}
	}

	// Also look for added/removed function definitions
	addedFuncPattern := regexp.MustCompile(`(?m)^[+-]func\s+(?:\([^)]+\)\s+)?(\w+)\s*\(`)
	matches = addedFuncPattern.FindAllStringSubmatch(diff, -1)
	for _, match := range matches {
		if len(match) > 1 && !seen[match[1]] {
			funcs = append(funcs, match[1])
			seen[match[1]] = true
		}
	}

	return funcs
}

// areDisjoint checks if two string slices have no common elements.
func areDisjoint(a, b []string) bool {
	set := make(map[string]bool)
	for _, s := range a {
		set[s] = true
	}
	for _, s := range b {
		if set[s] {
			return false
		}
	}
	return true
}

// writeFile writes content to a file, creating directories as needed.
func writeFile(path, content string) error {
	// Create parent directories if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	// Write content to file
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write file %s: %w", path, err)
	}

	return nil
}

// finalizeSemanticMerge stages the merged files and creates a commit.
// This is called after all files have been written and validated.
func (m *SemanticMerger) finalizeSemanticMerge(files []string, branch1, branch2, reasoning string) error {
	// Stage all merged files
	for _, file := range files {
		if err := m.git.Add(file); err != nil {
			return fmt.Errorf("stage file %s: %w", file, err)
		}
	}

	// Create commit message
	commitMsg := fmt.Sprintf("Semantic merge: %s into %s\n\n%s\n\nMerged files:\n", branch2, branch1, reasoning)
	for _, file := range files {
		commitMsg += fmt.Sprintf("  - %s\n", file)
	}

	// Create the merge commit
	if err := m.git.Commit(commitMsg); err != nil {
		return fmt.Errorf("create commit: %w", err)
	}

	return nil
}

// revertChanges discards any uncommitted changes in the working directory.
// This is called when validation fails after writing merged files.
func (m *SemanticMerger) revertChanges() error {
	// Reset staged changes
	_ = m.git.Reset("HEAD") // Ignore errors, proceed with checkout

	// Discard working directory changes
	if err := m.git.CheckoutPath("."); err != nil {
		return fmt.Errorf("revert changes: %w", err)
	}

	return nil
}
