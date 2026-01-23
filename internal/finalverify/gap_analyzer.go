// Package finalverify provides gap analysis for failed verifications.
package finalverify

import (
	"context"
	"fmt"
	"strings"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/architect"
)

// GapAnalyzer analyzes verification failures and generates actionable tasks.
type GapAnalyzer struct {
	runnerFactory agent.ClaudeRunnerFactory
}

// NewGapAnalyzer creates a new gap analyzer.
func NewGapAnalyzer(runnerFactory agent.ClaudeRunnerFactory) *GapAnalyzer {
	return &GapAnalyzer{
		runnerFactory: runnerFactory,
	}
}

// GapAnalysisResult contains the analysis of gaps and suggested tasks.
type GapAnalysisResult struct {
	// SuggestedTasks lists tasks that should be created to fix gaps.
	SuggestedTasks []SuggestedTask
	// Analysis provides an overall analysis of the gaps.
	Analysis string
	// Priority indicates the priority level (high, medium, low).
	Priority string
}

// SuggestedTask represents a task that should be created to address a gap.
type SuggestedTask struct {
	// Title is a short title for the task.
	Title string
	// Description is a detailed description of what needs to be done.
	Description string
	// FeatureID is the ID of the feature this task addresses.
	FeatureID string
	// Reason explains why this task is needed.
	Reason string
	// Priority is the task priority (critical, high, medium, low).
	Priority string
}

// AnalyzeGaps analyzes verification gaps and generates suggested tasks.
func (a *GapAnalyzer) AnalyzeGaps(ctx context.Context, input GapAnalysisInput) (*GapAnalysisResult, error) {
	if a.runnerFactory == nil {
		return nil, fmt.Errorf("runner factory not configured")
	}

	// Build the analysis prompt
	prompt := a.buildGapAnalysisPrompt(input)

	// Create Claude runner
	claude := a.runnerFactory.NewRunner()

	// Run the analysis
	response, err := a.invokeClaudeForAnalysis(ctx, claude, prompt, input.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("invoke Claude for gap analysis: %w", err)
	}

	// Parse the response
	result := a.parseAnalysisResponse(response)
	return result, nil
}

// GapAnalysisInput contains information needed for gap analysis.
type GapAnalysisInput struct {
	// RepoPath is the path to the repository.
	RepoPath string
	// Gaps are the gaps identified by the architecture audit.
	Gaps []architect.Gap
	// VerificationResult is the full verification result.
	VerificationResult *VerificationResult
	// SpecText is the original specification text.
	SpecText string
}

// buildGapAnalysisPrompt constructs the prompt for gap analysis.
func (a *GapAnalyzer) buildGapAnalysisPrompt(input GapAnalysisInput) string {
	var sb strings.Builder

	sb.WriteString("# Gap Analysis and Task Generation\n\n")
	sb.WriteString("The final verification of an implementation has failed. ")
	sb.WriteString("Your task is to analyze the gaps and generate specific, actionable tasks ")
	sb.WriteString("to address them.\n\n")

	sb.WriteString("## Original Specification\n\n")
	sb.WriteString(input.SpecText)
	sb.WriteString("\n\n")

	sb.WriteString("## Verification Results\n\n")
	if input.VerificationResult != nil {
		sb.WriteString(input.VerificationResult.Summary)
	}
	sb.WriteString("\n\n")

	sb.WriteString("## Identified Gaps\n\n")
	if len(input.Gaps) > 0 {
		for i, gap := range input.Gaps {
			sb.WriteString(fmt.Sprintf("### Gap %d: %s [%s]\n", i+1, gap.FeatureID, gap.Status))
			sb.WriteString(fmt.Sprintf("**Description**: %s\n", gap.Description))
			sb.WriteString(fmt.Sprintf("**Suggested Action**: %s\n\n", gap.SuggestedAction))
		}
	} else {
		sb.WriteString("No specific gaps identified, but verification failed.\n")
		sb.WriteString("Analyze the verification results to identify what needs to be fixed.\n\n")
	}

	sb.WriteString("## Your Task\n\n")
	sb.WriteString("1. Analyze each gap and understand what is missing or incomplete\n")
	sb.WriteString("2. For each gap, generate a specific, actionable task\n")
	sb.WriteString("3. Prioritize the tasks (critical, high, medium, low)\n")
	sb.WriteString("4. Provide an overall analysis and recommended approach\n\n")

	sb.WriteString("## Response Format\n\n")
	sb.WriteString("OVERALL_PRIORITY: [high/medium/low]\n")
	sb.WriteString("ANALYSIS: [Brief analysis of the gaps and approach]\n\n")
	sb.WriteString("TASKS:\n")
	sb.WriteString("---\n")
	sb.WriteString("TITLE: [Short task title]\n")
	sb.WriteString("FEATURE_ID: [Feature ID this addresses]\n")
	sb.WriteString("PRIORITY: [critical/high/medium/low]\n")
	sb.WriteString("DESCRIPTION: [Detailed description of what needs to be done]\n")
	sb.WriteString("REASON: [Why this task is needed]\n")
	sb.WriteString("---\n")
	sb.WriteString("(Repeat for each task)\n\n")

	sb.WriteString("Generate tasks that are:\n")
	sb.WriteString("- Specific and actionable (not vague)\n")
	sb.WriteString("- Focused on one concern each\n")
	sb.WriteString("- Include acceptance criteria\n")
	sb.WriteString("- Address the root cause, not just symptoms\n")

	return sb.String()
}

// invokeClaudeForAnalysis sends the prompt to Claude and returns the response.
func (a *GapAnalyzer) invokeClaudeForAnalysis(ctx context.Context, claude agent.ClaudeRunner, prompt, repoPath string) (string, error) {
	// Start Claude with the prompt
	temp := 0.3 // Slightly creative for task generation
	opts := &agent.StartOptions{
		Temperature: &temp,
	}

	if err := claude.StartWithOptions(prompt, repoPath, opts); err != nil {
		return "", fmt.Errorf("start claude: %w", err)
	}

	// Collect output
	var outputBuilder strings.Builder
	for event := range claude.Output() {
		switch event.Type {
		case agent.StreamEventAssistant, agent.StreamEventResult:
			if event.Message != "" {
				outputBuilder.WriteString(event.Message)
			}
		case agent.StreamEventError:
			if event.Error != "" {
				return "", fmt.Errorf("claude error: %s", event.Error)
			}
		}
	}

	// Wait for completion
	if err := claude.Wait(); err != nil {
		return "", fmt.Errorf("claude process failed: %w", err)
	}

	return outputBuilder.String(), nil
}

// parseAnalysisResponse parses Claude's gap analysis response.
func (a *GapAnalyzer) parseAnalysisResponse(response string) *GapAnalysisResult {
	result := &GapAnalysisResult{
		SuggestedTasks: []SuggestedTask{},
		Analysis:       "",
		Priority:       "medium",
	}

	lines := strings.Split(response, "\n")
	var currentTask *SuggestedTask

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "OVERALL_PRIORITY:") {
			result.Priority = strings.TrimSpace(strings.TrimPrefix(line, "OVERALL_PRIORITY:"))
		} else if strings.HasPrefix(line, "ANALYSIS:") {
			result.Analysis = strings.TrimSpace(strings.TrimPrefix(line, "ANALYSIS:"))
		} else if line == "---" {
			// Start or end of a task block
			if currentTask != nil {
				// End of task - add it
				if currentTask.Title != "" {
					result.SuggestedTasks = append(result.SuggestedTasks, *currentTask)
				}
			}
			// Start new task
			currentTask = &SuggestedTask{}
		} else if currentTask != nil {
			// Parse task fields
			if strings.HasPrefix(line, "TITLE:") {
				currentTask.Title = strings.TrimSpace(strings.TrimPrefix(line, "TITLE:"))
			} else if strings.HasPrefix(line, "FEATURE_ID:") {
				currentTask.FeatureID = strings.TrimSpace(strings.TrimPrefix(line, "FEATURE_ID:"))
			} else if strings.HasPrefix(line, "PRIORITY:") {
				currentTask.Priority = strings.TrimSpace(strings.TrimPrefix(line, "PRIORITY:"))
			} else if strings.HasPrefix(line, "DESCRIPTION:") {
				currentTask.Description = strings.TrimSpace(strings.TrimPrefix(line, "DESCRIPTION:"))
			} else if strings.HasPrefix(line, "REASON:") {
				currentTask.Reason = strings.TrimSpace(strings.TrimPrefix(line, "REASON:"))
			}
		}
	}

	// Add last task if exists
	if currentTask != nil && currentTask.Title != "" {
		result.SuggestedTasks = append(result.SuggestedTasks, *currentTask)
	}

	return result
}

// FormatTasks formats suggested tasks for display or logging.
func FormatTasks(tasks []SuggestedTask) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Generated %d tasks to address gaps:\n\n", len(tasks)))

	for i, task := range tasks {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, task.Priority, task.Title))
		if task.FeatureID != "" {
			sb.WriteString(fmt.Sprintf("   Feature: %s\n", task.FeatureID))
		}
		sb.WriteString(fmt.Sprintf("   Reason: %s\n", task.Reason))
		sb.WriteString(fmt.Sprintf("   Description: %s\n", task.Description))
		sb.WriteString("\n")
	}

	return sb.String()
}
