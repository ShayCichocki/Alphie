// Package agent provides the AI agent implementation for Alphie.
package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Baseline captures the state of tests, lints, and type errors at session start.
// This allows enforcing "no regressions" during the session - pre-existing failures
// are allowed, but new or worse failures are blocked.
type Baseline struct {
	// FailingTests is the list of test identifiers that were failing at capture time.
	FailingTests []string `json:"failing_tests"`
	// LintErrors is the list of lint error messages at capture time.
	LintErrors []string `json:"lint_errors"`
	// TypeErrors is the list of type/compilation error messages at capture time.
	TypeErrors []string `json:"type_errors"`
	// CapturedAt is when this baseline was captured.
	CapturedAt time.Time `json:"captured_at"`
}

// GateResults represents the current state of tests, lints, and type checks.
// This is compared against the baseline to detect regressions.
type GateResults struct {
	// FailingTests is the list of currently failing test identifiers.
	FailingTests []string `json:"failing_tests"`
	// LintErrors is the list of current lint error messages.
	LintErrors []string `json:"lint_errors"`
	// TypeErrors is the list of current type/compilation error messages.
	TypeErrors []string `json:"type_errors"`
}

// Comparison contains the result of comparing current state to a baseline.
type Comparison struct {
	// NewFailures contains failures that were not in the baseline (regressions).
	NewFailures []string `json:"new_failures"`
	// WorseLints is the count of additional lint errors compared to baseline.
	// Positive means more errors than baseline, negative means fewer.
	WorseLints int `json:"worse_lints"`
	// Improved contains failures from the baseline that are now passing.
	Improved []string `json:"improved"`
	// IsRegression is true if there are new failures or worse lints.
	IsRegression bool `json:"is_regression"`
}

// CaptureBaseline runs tests, lint, and typecheck to capture the current state.
// This should be called at session start to establish the baseline.
func CaptureBaseline(repoPath string) (*Baseline, error) {
	baseline := &Baseline{
		CapturedAt: time.Now(),
	}

	// Run tests and capture failures
	failingTests, err := runTests(repoPath)
	if err != nil {
		// Error running tests is not fatal - we capture whatever we can
		baseline.FailingTests = failingTests
	} else {
		baseline.FailingTests = failingTests
	}

	// Run lint and capture errors
	lintErrors, err := runLint(repoPath)
	if err != nil {
		baseline.LintErrors = lintErrors
	} else {
		baseline.LintErrors = lintErrors
	}

	// Run typecheck and capture errors
	typeErrors, err := runTypecheck(repoPath)
	if err != nil {
		baseline.TypeErrors = typeErrors
	} else {
		baseline.TypeErrors = typeErrors
	}

	return baseline, nil
}

// CompareToBaseline compares current gate results against a baseline.
// It identifies new failures, improvements, and determines if there's a regression.
func CompareToBaseline(current *GateResults, baseline *Baseline) *Comparison {
	if current == nil || baseline == nil {
		return &Comparison{
			IsRegression: current != nil && baseline == nil,
		}
	}

	comparison := &Comparison{}

	// Build a set of baseline failures for quick lookup
	baselineSet := make(map[string]bool)
	for _, f := range baseline.FailingTests {
		baselineSet[f] = true
	}
	for _, f := range baseline.LintErrors {
		baselineSet[f] = true
	}
	for _, f := range baseline.TypeErrors {
		baselineSet[f] = true
	}

	// Build a set of current failures
	currentSet := make(map[string]bool)
	for _, f := range current.FailingTests {
		currentSet[f] = true
	}
	for _, f := range current.LintErrors {
		currentSet[f] = true
	}
	for _, f := range current.TypeErrors {
		currentSet[f] = true
	}

	// Find new failures (in current but not in baseline)
	for f := range currentSet {
		if !baselineSet[f] {
			comparison.NewFailures = append(comparison.NewFailures, f)
		}
	}

	// Find improvements (in baseline but not in current)
	for f := range baselineSet {
		if !currentSet[f] {
			comparison.Improved = append(comparison.Improved, f)
		}
	}

	// Calculate lint difference
	comparison.WorseLints = len(current.LintErrors) - len(baseline.LintErrors)

	// Determine if this is a regression
	comparison.IsRegression = len(comparison.NewFailures) > 0 || comparison.WorseLints > 0

	return comparison
}

// Save persists the baseline to a file.
func (b *Baseline) Save(path string) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

// LoadBaseline loads a baseline from a file.
func LoadBaseline(path string) (*Baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var baseline Baseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, err
	}

	return &baseline, nil
}

// runTests executes tests and returns a list of failing test identifiers.
func runTests(repoPath string) ([]string, error) {
	var failures []string

	// Try go test first
	cmd := exec.Command("go", "test", "./...", "-json")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		// Parse the output even on error - tests may have run but some failed
		failures = parseGoTestJSON(output)
		if len(failures) > 0 {
			return failures, nil
		}
		// If no JSON output, try parsing plain output
		if exitErr, ok := err.(*exec.ExitError); ok {
			failures = parseGoTestPlain(exitErr.Stderr)
		}
		return failures, err
	}

	return parseGoTestJSON(output), nil
}

// runLint executes the linter and returns a list of lint errors.
func runLint(repoPath string) ([]string, error) {
	var lintErrors []string

	// Try golangci-lint first
	cmd := exec.Command("golangci-lint", "run", "--out-format=json", "./...")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		// golangci-lint returns non-zero on lint errors
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Combine stdout and stderr
			combined := append(output, exitErr.Stderr...)
			lintErrors = parseGolangciLintJSON(combined)
			if len(lintErrors) > 0 {
				return lintErrors, nil
			}
		}
	}

	if len(output) > 0 {
		lintErrors = parseGolangciLintJSON(output)
	}

	// Fall back to go vet if golangci-lint not available
	if err != nil && len(lintErrors) == 0 {
		cmd = exec.Command("go", "vet", "./...")
		cmd.Dir = repoPath
		output, vetErr := cmd.CombinedOutput()
		if vetErr != nil {
			lintErrors = parseGoVetOutput(output)
			return lintErrors, nil
		}
	}

	return lintErrors, nil
}

// runTypecheck runs the Go compiler to check for type errors.
func runTypecheck(repoPath string) ([]string, error) {
	var typeErrors []string

	cmd := exec.Command("go", "build", "-o", "/dev/null", "./...")
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		typeErrors = parseGoBuildOutput(output)
		return typeErrors, nil
	}

	return typeErrors, nil
}

// parseGoTestJSON parses Go test JSON output and extracts failing test names.
func parseGoTestJSON(output []byte) []string {
	var failures []string
	seenFailures := make(map[string]bool)

	lines := bytes.Split(output, []byte("\n"))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		var event struct {
			Action  string `json:"Action"`
			Package string `json:"Package"`
			Test    string `json:"Test"`
		}

		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}

		if event.Action == "fail" && event.Test != "" {
			key := event.Package + "/" + event.Test
			if !seenFailures[key] {
				failures = append(failures, key)
				seenFailures[key] = true
			}
		}
	}

	return failures
}

// parseGoTestPlain parses plain Go test output for failures.
func parseGoTestPlain(output []byte) []string {
	var failures []string

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "--- FAIL:") {
			// Extract test name from "--- FAIL: TestName (0.00s)"
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				failures = append(failures, parts[2])
			}
		}
	}

	return failures
}

// parseGolangciLintJSON parses golangci-lint JSON output.
func parseGolangciLintJSON(output []byte) []string {
	var lintErrors []string

	var result struct {
		Issues []struct {
			FromLinter string `json:"FromLinter"`
			Text       string `json:"Text"`
			Pos        struct {
				Filename string `json:"Filename"`
				Line     int    `json:"Line"`
				Column   int    `json:"Column"`
			} `json:"Pos"`
		} `json:"Issues"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		// If JSON parsing fails, treat each line as an error
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "{") {
				lintErrors = append(lintErrors, line)
			}
		}
		return lintErrors
	}

	for _, issue := range result.Issues {
		errorStr := issue.Pos.Filename + ":" +
			strings.TrimSpace(issue.FromLinter) + ": " +
			strings.TrimSpace(issue.Text)
		lintErrors = append(lintErrors, errorStr)
	}

	return lintErrors
}

// parseGoVetOutput parses go vet output.
func parseGoVetOutput(output []byte) []string {
	var errors []string

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			errors = append(errors, line)
		}
	}

	return errors
}

// parseGoBuildOutput parses go build output for type/compilation errors.
func parseGoBuildOutput(output []byte) []string {
	var errors []string

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip package path lines (start with #) and empty lines
		if line != "" && !strings.HasPrefix(line, "#") {
			errors = append(errors, line)
		}
	}

	return errors
}
