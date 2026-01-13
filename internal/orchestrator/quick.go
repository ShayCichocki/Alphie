package orchestrator

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/shayc/alphie/internal/agent"
)

// QuickResult contains the outcome of a quick task execution.
type QuickResult struct {
	Success    bool
	Output     string
	Error      string
	TokensUsed int64
	Cost       float64
	Duration   time.Duration
}

// QuickExecutor runs simple tasks directly without decomposition or session branches.
type QuickExecutor struct {
	repoPath string
}

// NewQuickExecutor creates a new QuickExecutor.
func NewQuickExecutor(repoPath string) *QuickExecutor {
	return &QuickExecutor{
		repoPath: repoPath,
	}
}

// Execute runs a task directly on the current branch without decomposition.
func (q *QuickExecutor) Execute(ctx context.Context, task string) (*QuickResult, error) {
	startTime := time.Now()

	// Create token tracker for haiku model
	tracker := agent.NewTokenTracker("claude-3-5-haiku-20241022")

	// Estimate input tokens from task (rough: ~4 chars per token)
	inputTokens := int64(len(task) / 4)
	if inputTokens < 10 {
		inputTokens = 10 // minimum
	}

	// Create Claude process
	claude := agent.NewClaudeProcess(ctx)

	// Start the process in the repo directory (no worktree)
	if err := claude.Start(task, q.repoPath); err != nil {
		return &QuickResult{
			Success:  false,
			Error:    fmt.Sprintf("failed to start claude: %v", err),
			Duration: time.Since(startTime),
		}, nil
	}

	// Collect output
	var outputBuilder strings.Builder

	for event := range claude.Output() {
		switch event.Type {
		case agent.StreamEventAssistant:
			if event.Message != "" {
				outputBuilder.WriteString(event.Message)
				outputBuilder.WriteString("\n")
			}
		case agent.StreamEventError:
			return &QuickResult{
				Success:  false,
				Output:   outputBuilder.String(),
				Error:    event.Error,
				Duration: time.Since(startTime),
			}, nil
		}
	}

	// Wait for process to complete
	if err := claude.Wait(); err != nil {
		return &QuickResult{
			Success:  false,
			Output:   outputBuilder.String(),
			Error:    fmt.Sprintf("process failed: %v", err),
			Duration: time.Since(startTime),
		}, nil
	}

	// Estimate output tokens from output (rough: ~4 chars per token)
	output := outputBuilder.String()
	outputTokens := int64(len(output) / 4)
	if outputTokens < 10 {
		outputTokens = 10 // minimum
	}

	// Update tracker with soft estimates
	tracker.UpdateSoft(inputTokens, outputTokens)

	// Auto-commit any changes made by the agent
	commitErr := q.autoCommitChanges(task)
	if commitErr != nil {
		output += fmt.Sprintf("\n[Auto-commit: %v]", commitErr)
	}

	return &QuickResult{
		Success:    true,
		Output:     output,
		TokensUsed: tracker.GetUsage().TotalTokens,
		Cost:       tracker.GetCost(),
		Duration:   time.Since(startTime),
	}, nil
}

// autoCommitChanges commits any uncommitted changes in the repo.
func (q *QuickExecutor) autoCommitChanges(taskTitle string) error {
	// Check if there are any changes to commit
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = q.repoPath
	statusOutput, err := statusCmd.Output()
	if err != nil {
		return fmt.Errorf("check git status: %w", err)
	}

	// No changes to commit
	if len(statusOutput) == 0 {
		return fmt.Errorf("no changes to commit")
	}

	// Stage all changes
	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = q.repoPath
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %s: %w", string(output), err)
	}

	// Commit with task title as message
	commitMsg := fmt.Sprintf("Quick: %s", taskTitle)
	commitCmd := exec.Command("git", "commit", "-m", commitMsg)
	commitCmd.Dir = q.repoPath
	if output, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s: %w", string(output), err)
	}

	return nil
}
