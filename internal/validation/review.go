// Package validation provides code review capabilities.
package validation

import (
	"context"
	"fmt"
	"strings"

	"github.com/ShayCichocki/alphie/internal/agent"
)

// CodeReviewer performs detailed code review against acceptance criteria.
type CodeReviewer struct {
	runnerFactory agent.ClaudeRunnerFactory
}

// NewCodeReviewer creates a new code reviewer.
func NewCodeReviewer(runnerFactory agent.ClaudeRunnerFactory) *CodeReviewer {
	return &CodeReviewer{
		runnerFactory: runnerFactory,
	}
}

// CodeReviewResult contains the outcome of a code review.
type CodeReviewResult struct {
	// Passed indicates whether the code meets quality standards.
	Passed bool
	// Score is a quality score out of 10.
	Score int
	// Completeness assesses how complete the implementation is (0-10).
	Completeness int
	// Correctness assesses how correct the implementation is (0-10).
	Correctness int
	// Quality assesses code quality (0-10).
	Quality int
	// Issues lists specific issues found.
	Issues []CodeIssue
	// Summary provides an overall assessment.
	Summary string
}

// CodeIssue represents a specific issue found during review.
type CodeIssue struct {
	// Severity is critical, major, minor, or suggestion.
	Severity string
	// File is the file where the issue was found.
	File string
	// Description describes the issue.
	Description string
	// Suggestion provides a suggestion for fixing it.
	Suggestion string
}

// CodeReviewInput contains all information needed for code review.
type CodeReviewInput struct {
	// TaskTitle is the title of the task.
	TaskTitle string
	// TaskDescription is the full task description.
	TaskDescription string
	// AcceptanceCriteria lists the criteria that must be met.
	AcceptanceCriteria []string
	// Implementation is the code that was written.
	Implementation string
	// ModifiedFiles lists files that were changed.
	ModifiedFiles []string
	// TestResults contains test output if available.
	TestResults string
}

// Review performs a detailed code review.
func (r *CodeReviewer) Review(ctx context.Context, input CodeReviewInput) (*CodeReviewResult, error) {
	if r.runnerFactory == nil {
		return nil, fmt.Errorf("runner factory not configured")
	}

	// Create a Claude runner for this review
	runner := r.runnerFactory.NewRunner()

	// Build the review prompt
	prompt := r.buildReviewPrompt(input)

	// Send to Claude for review
	response, err := r.invokeClaudeForReview(ctx, runner, prompt)
	if err != nil {
		return nil, fmt.Errorf("invoke Claude for review: %w", err)
	}

	// Parse the response
	result := r.parseReviewResponse(response)
	return result, nil
}

// buildReviewPrompt constructs the prompt for code review.
func (r *CodeReviewer) buildReviewPrompt(input CodeReviewInput) string {
	var sb strings.Builder

	sb.WriteString("# Code Review Task\n\n")
	sb.WriteString("You are performing a detailed code review to ensure the implementation ")
	sb.WriteString("meets all acceptance criteria and quality standards.\n\n")

	sb.WriteString("## Task Context\n\n")
	sb.WriteString(fmt.Sprintf("**Task**: %s\n\n", input.TaskTitle))
	sb.WriteString(fmt.Sprintf("**Description**:\n%s\n\n", input.TaskDescription))

	if len(input.AcceptanceCriteria) > 0 {
		sb.WriteString("**Acceptance Criteria**:\n")
		for i, criteria := range input.AcceptanceCriteria {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, criteria))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Implementation Review\n\n")
	if len(input.ModifiedFiles) > 0 {
		sb.WriteString("**Files Modified**:\n")
		for _, file := range input.ModifiedFiles {
			sb.WriteString(fmt.Sprintf("- %s\n", file))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("**Code Changes**:\n```\n")
	sb.WriteString(input.Implementation)
	sb.WriteString("\n```\n\n")

	if input.TestResults != "" {
		sb.WriteString("**Test Results**:\n```\n")
		sb.WriteString(input.TestResults)
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString("## Review Guidelines\n\n")
	sb.WriteString("Evaluate the implementation across these dimensions:\n\n")
	sb.WriteString("1. **Completeness** (0-10): Are all acceptance criteria fully addressed?\n")
	sb.WriteString("2. **Correctness** (0-10): Is the implementation correct and bug-free?\n")
	sb.WriteString("3. **Quality** (0-10): Is the code clean, maintainable, and well-structured?\n\n")

	sb.WriteString("Check for:\n")
	sb.WriteString("- Missing functionality\n")
	sb.WriteString("- Logic errors or bugs\n")
	sb.WriteString("- Security vulnerabilities\n")
	sb.WriteString("- Edge cases not handled\n")
	sb.WriteString("- Poor error handling\n")
	sb.WriteString("- Code smells or anti-patterns\n\n")

	sb.WriteString("## Response Format\n\n")
	sb.WriteString("VERDICT: [PASS/FAIL]\n")
	sb.WriteString("COMPLETENESS: [0-10]\n")
	sb.WriteString("CORRECTNESS: [0-10]\n")
	sb.WriteString("QUALITY: [0-10]\n")
	sb.WriteString("ISSUES:\n")
	sb.WriteString("- [SEVERITY] in [FILE]: [DESCRIPTION] | Suggestion: [SUGGESTION]\n")
	sb.WriteString("(Repeat for each issue, or write 'None' if no issues)\n\n")
	sb.WriteString("SUMMARY: [2-3 sentence overall assessment]\n\n")

	sb.WriteString("Severity levels: CRITICAL, MAJOR, MINOR, SUGGESTION\n")
	sb.WriteString("PASS if all acceptance criteria are met and no critical/major issues. ")
	sb.WriteString("FAIL if criteria missing or critical issues present.\n")

	return sb.String()
}

// invokeClaudeForReview sends the prompt to Claude and returns the response.
func (r *CodeReviewer) invokeClaudeForReview(ctx context.Context, runner agent.ClaudeRunner, prompt string) (string, error) {
	// Start Claude with Sonnet model for code review
	opts := &agent.StartOptions{Model: agent.ModelSonnet}
	if err := runner.StartWithOptions(prompt, "/tmp", opts); err != nil {
		return "", fmt.Errorf("start claude for review: %w", err)
	}

	// Collect the response
	var response strings.Builder
	for event := range runner.Output() {
		select {
		case <-ctx.Done():
			_ = runner.Kill()
			return "", ctx.Err()
		default:
		}

		switch event.Type {
		case agent.StreamEventResult:
			response.WriteString(event.Message)
		case agent.StreamEventAssistant:
			response.WriteString(event.Message)
		case agent.StreamEventError:
			return "", fmt.Errorf("claude review error: %s", event.Error)
		}
	}

	// Wait for completion
	if err := runner.Wait(); err != nil {
		return "", fmt.Errorf("wait for claude review: %w", err)
	}

	return response.String(), nil
}

// parseReviewResponse parses Claude's response into a structured result.
func (r *CodeReviewer) parseReviewResponse(response string) *CodeReviewResult {
	result := &CodeReviewResult{
		Passed:       false,
		Score:        0,
		Completeness: 0,
		Correctness:  0,
		Quality:      0,
		Issues:       []CodeIssue{},
		Summary:      "",
	}

	lines := strings.Split(response, "\n")
	parsingIssues := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "VERDICT:") {
			verdict := strings.TrimSpace(strings.TrimPrefix(line, "VERDICT:"))
			result.Passed = strings.ToUpper(verdict) == "PASS"
		} else if strings.HasPrefix(line, "COMPLETENESS:") {
			fmt.Sscanf(line, "COMPLETENESS: %d", &result.Completeness)
		} else if strings.HasPrefix(line, "CORRECTNESS:") {
			fmt.Sscanf(line, "CORRECTNESS: %d", &result.Correctness)
		} else if strings.HasPrefix(line, "QUALITY:") {
			fmt.Sscanf(line, "QUALITY: %d", &result.Quality)
		} else if strings.HasPrefix(line, "ISSUES:") {
			parsingIssues = true
			continue
		} else if strings.HasPrefix(line, "SUMMARY:") {
			result.Summary = strings.TrimSpace(strings.TrimPrefix(line, "SUMMARY:"))
			parsingIssues = false
		} else if parsingIssues && strings.HasPrefix(line, "- ") {
			issue := r.parseIssue(line)
			if issue != nil {
				result.Issues = append(result.Issues, *issue)
			}
		}
	}

	// Calculate overall score (average of the three dimensions)
	if result.Completeness > 0 || result.Correctness > 0 || result.Quality > 0 {
		result.Score = (result.Completeness + result.Correctness + result.Quality) / 3
	}

	return result
}

// parseIssue parses a single issue line.
func (r *CodeReviewer) parseIssue(line string) *CodeIssue {
	// Expected format: - [SEVERITY] in [FILE]: [DESCRIPTION] | Suggestion: [SUGGESTION]
	line = strings.TrimPrefix(line, "- ")

	if strings.ToLower(line) == "none" {
		return nil
	}

	parts := strings.Split(line, "|")
	if len(parts) < 2 {
		// Malformed, but try to parse what we can
		return &CodeIssue{
			Severity:    "UNKNOWN",
			Description: line,
		}
	}

	mainPart := strings.TrimSpace(parts[0])
	suggestionPart := strings.TrimSpace(parts[1])

	issue := &CodeIssue{
		Suggestion: strings.TrimPrefix(suggestionPart, "Suggestion:"),
	}

	// Parse [SEVERITY] in [FILE]: [DESCRIPTION]
	if strings.Contains(mainPart, "] in ") {
		severityEnd := strings.Index(mainPart, "]")
		if severityEnd > 0 {
			issue.Severity = strings.TrimPrefix(mainPart[:severityEnd], "[")
		}

		fileStart := strings.Index(mainPart, " in ") + 4
		fileEnd := strings.Index(mainPart, ":")
		if fileStart > 4 && fileEnd > fileStart {
			issue.File = strings.Trim(mainPart[fileStart:fileEnd], "[]")
		}

		descStart := fileEnd + 1
		if descStart < len(mainPart) {
			issue.Description = strings.TrimSpace(mainPart[descStart:])
		}
	} else {
		issue.Description = mainPart
	}

	return issue
}
