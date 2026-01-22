// Package orchestrator provides task decomposition and coordination.
package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/orchestrator/policy"
	"github.com/ShayCichocki/alphie/internal/protect"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// ReviewTrigger contains information about why a second review was triggered.
type ReviewTrigger struct {
	// Triggered indicates whether a second review is needed.
	Triggered bool
	// Reasons lists why the second review was triggered.
	Reasons []string
}

// SecondReviewResult contains the outcome of a second review.
type SecondReviewResult struct {
	// Approved indicates whether the reviewer approved the changes.
	Approved bool
	// Concerns lists any issues identified by the reviewer.
	Concerns []string
	// ReviewerOutput contains the raw output from the reviewer.
	ReviewerOutput string
}

// SecondReviewer triggers and coordinates second agent reviews for high-risk diffs.
type SecondReviewer struct {
	protected *protect.Detector
	// claude is the Claude runner for reviews.
	// Can be either subprocess (ClaudeProcess) or direct API (ClaudeAPIAdapter).
	claude agent.ClaudeRunner
	// policy contains configurable review thresholds.
	policy *policy.ReviewPolicy
}

// NewSecondReviewer creates a new SecondReviewer with the given dependencies.
func NewSecondReviewer(protected *protect.Detector, claude agent.ClaudeRunner) *SecondReviewer {
	return NewSecondReviewerWithPolicy(protected, claude, &policy.Default().Review)
}

// NewSecondReviewerWithPolicy creates a new SecondReviewer with custom policy.
func NewSecondReviewerWithPolicy(protected *protect.Detector, claude agent.ClaudeRunner, p *policy.ReviewPolicy) *SecondReviewer {
	if p == nil {
		p = &policy.Default().Review
	}
	return &SecondReviewer{
		protected: protected,
		claude:    claude,
		policy:    p,
	}
}

// ShouldSecondReview determines whether a diff requires a second review.
// It checks multiple conditions and returns a trigger with reasons if ANY condition is met.
//
// Conditions checked:
//  1. Touches protected areas (auth, migrations, infra)
//  2. Large diff (>200 lines)
//  3. Weak or absent tests for touched code
//  4. Cross-cutting changes (>3 packages)
func (r *SecondReviewer) ShouldSecondReview(diff string, changedFiles []string, task *models.Task) *ReviewTrigger {
	trigger := &ReviewTrigger{
		Triggered: false,
		Reasons:   []string{},
	}

	// Check for protected areas
	if r.touchesProtectedAreas(changedFiles) {
		trigger.Triggered = true
		trigger.Reasons = append(trigger.Reasons, "touches protected areas (auth, migrations, infra, security)")
	}

	// Check for large diff
	if r.isLargeDiff(diff) {
		trigger.Triggered = true
		trigger.Reasons = append(trigger.Reasons, fmt.Sprintf("large diff exceeds %d lines", r.policy.LargeDiffThreshold))
	}

	// Check for weak/absent tests
	if r.hasWeakTests(changedFiles) {
		trigger.Triggered = true
		trigger.Reasons = append(trigger.Reasons, "weak or absent tests for touched code")
	}

	// Check for cross-cutting changes
	if r.isCrossCutting(changedFiles) {
		trigger.Triggered = true
		trigger.Reasons = append(trigger.Reasons, fmt.Sprintf("cross-cutting changes affect >%d packages", r.policy.CrossCuttingThreshold))
	}

	return trigger
}

// touchesProtectedAreas checks if any changed files are in protected areas.
func (r *SecondReviewer) touchesProtectedAreas(changedFiles []string) bool {
	if r.protected == nil {
		return false
	}

	for _, file := range changedFiles {
		if r.protected.IsProtected(file) {
			return true
		}
	}
	return false
}

// isLargeDiff checks if the diff exceeds the line threshold.
func (r *SecondReviewer) isLargeDiff(diff string) bool {
	lines := strings.Count(diff, "\n")
	return lines > r.policy.LargeDiffThreshold
}

// hasWeakTests checks if the changed files lack corresponding test coverage.
// It looks for source files without matching test files.
func (r *SecondReviewer) hasWeakTests(changedFiles []string) bool {
	sourceFiles := make(map[string]bool)
	testFiles := make(map[string]bool)

	for _, file := range changedFiles {
		// Skip non-source files
		if !isSourceFile(file) {
			continue
		}

		if isTestFile(file) {
			// Track test file base names
			base := getTestBaseName(file)
			testFiles[base] = true
		} else {
			// Track source file base names
			base := getSourceBaseName(file)
			sourceFiles[base] = true
		}
	}

	// Check if any source files lack corresponding tests
	for source := range sourceFiles {
		if !testFiles[source] {
			return true
		}
	}

	return false
}

// isCrossCutting checks if changes span more than the cross-cutting threshold packages.
func (r *SecondReviewer) isCrossCutting(changedFiles []string) bool {
	packages := make(map[string]bool)

	for _, file := range changedFiles {
		pkg := extractPackage(file)
		if pkg != "" {
			packages[pkg] = true
		}
	}

	return len(packages) > r.policy.CrossCuttingThreshold
}

// RequestReview spawns a second Claude agent to review the diff.
func (r *SecondReviewer) RequestReview(ctx context.Context, diff string, taskDescription string) (*SecondReviewResult, error) {
	if r.claude == nil {
		return nil, fmt.Errorf("claude process not configured")
	}

	prompt := buildReviewPrompt(diff, taskDescription)

	// Start the Claude process with the review prompt
	if err := r.claude.Start(prompt, ""); err != nil {
		return nil, fmt.Errorf("start claude process: %w", err)
	}

	// Collect the output
	var output strings.Builder
	for event := range r.claude.Output() {
		switch event.Type {
		case agent.StreamEventAssistant, agent.StreamEventResult:
			output.WriteString(event.Message)
		case agent.StreamEventError:
			return nil, fmt.Errorf("claude error: %s", event.Error)
		}
	}

	// Wait for process to complete
	if err := r.claude.Wait(); err != nil {
		return nil, fmt.Errorf("wait for claude: %w", err)
	}

	// Parse the response
	return parseReviewResponse(output.String()), nil
}

// buildReviewPrompt constructs the prompt for the second review agent.
func buildReviewPrompt(diff, taskDescription string) string {
	return fmt.Sprintf(`You are a code reviewer performing a second review of high-risk changes.

TASK DESCRIPTION:
%s

DIFF TO REVIEW:
%s

Please review this diff carefully and provide your assessment.

Your response MUST include:
1. A clear APPROVED or NOT APPROVED verdict on the first line
2. A list of concerns, if any (prefix each with "CONCERN:")
3. Any recommendations for improvement

Focus on:
- Security vulnerabilities
- Breaking changes
- Data integrity issues
- Missing error handling
- Potential performance problems

If you approve, state "APPROVED" on the first line.
If you have concerns that block approval, state "NOT APPROVED" on the first line.`, taskDescription, diff)
}

// parseReviewResponse extracts approval status and concerns from the reviewer output.
func parseReviewResponse(output string) *SecondReviewResult {
	result := &SecondReviewResult{
		Approved:       false,
		Concerns:       []string{},
		ReviewerOutput: output,
	}

	lines := strings.Split(output, "\n")

	// Check first non-empty line for approval status
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		upperLine := strings.ToUpper(line)
		if strings.Contains(upperLine, "APPROVED") && !strings.Contains(upperLine, "NOT APPROVED") {
			result.Approved = true
		}
		break
	}

	// Extract concerns
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "CONCERN:") {
			concern := strings.TrimPrefix(line, "CONCERN:")
			concern = strings.TrimPrefix(concern, "concern:")
			concern = strings.TrimSpace(concern)
			if concern != "" {
				result.Concerns = append(result.Concerns, concern)
			}
		}
	}

	return result
}

// isSourceFile checks if a file is a source code file.
func isSourceFile(path string) bool {
	extensions := []string{".go", ".js", ".ts", ".py", ".java", ".rb", ".rs", ".c", ".cpp", ".h"}
	for _, ext := range extensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

// isTestFile checks if a file is a test file.
func isTestFile(path string) bool {
	testPatterns := []string{"_test.go", ".test.js", ".test.ts", "_test.py", "Test.java", "_spec.rb", "_test.rs"}
	for _, pattern := range testPatterns {
		if strings.Contains(path, pattern) {
			return true
		}
	}
	return false
}

// getTestBaseName extracts the base name from a test file.
func getTestBaseName(path string) string {
	// Remove directory prefix
	base := path
	if idx := strings.LastIndex(path, "/"); idx != -1 {
		base = path[idx+1:]
	}

	// Remove test suffix patterns
	testPatterns := []string{"_test.go", ".test.js", ".test.ts", "_test.py", "Test.java", "_spec.rb", "_test.rs"}
	for _, pattern := range testPatterns {
		if strings.HasSuffix(base, pattern) {
			// Handle patterns that start with underscore or dot
			if strings.HasPrefix(pattern, "_") || strings.HasPrefix(pattern, ".") {
				base = strings.TrimSuffix(base, pattern)
			} else {
				// For patterns like "Test.java"
				base = strings.TrimSuffix(base, pattern)
			}
			break
		}
	}

	return base
}

// getSourceBaseName extracts the base name from a source file.
func getSourceBaseName(path string) string {
	// Remove directory prefix
	base := path
	if idx := strings.LastIndex(path, "/"); idx != -1 {
		base = path[idx+1:]
	}

	// Remove extension
	if idx := strings.LastIndex(base, "."); idx != -1 {
		base = base[:idx]
	}

	return base
}

// extractPackage extracts the package/directory path from a file path.
func extractPackage(path string) string {
	if idx := strings.LastIndex(path, "/"); idx != -1 {
		return path[:idx]
	}
	return ""
}
