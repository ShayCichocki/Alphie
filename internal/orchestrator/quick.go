package orchestrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	LogFile    string // Path to detailed log file
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

	// Create log file for this task
	logDir := filepath.Join(q.repoPath, ".alphie", "logs")
	_ = os.MkdirAll(logDir, 0755)
	logFileName := fmt.Sprintf("quick-%s.log", startTime.Format("2006-01-02-150405"))
	logFile := filepath.Join(logDir, logFileName)

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
		errMsg := fmt.Sprintf("failed to start claude: %v", err)
		_ = os.WriteFile(logFile, []byte(fmt.Sprintf("Task: %s\nError: %s\n", task, errMsg)), 0644)
		return &QuickResult{
			Success:  false,
			Error:    errMsg,
			Duration: time.Since(startTime),
			LogFile:  logFile,
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
			output := outputBuilder.String()
			logContent := fmt.Sprintf("Task: %s\nError: %s\n\n--- Output ---\n%s\n", task, event.Error, output)
			_ = os.WriteFile(logFile, []byte(logContent), 0644)
			return &QuickResult{
				Success:  false,
				Output:   output,
				Error:    event.Error,
				Duration: time.Since(startTime),
				LogFile:  logFile,
			}, nil
		}
	}

	// Wait for process to complete
	if err := claude.Wait(); err != nil {
		output := outputBuilder.String()
		errMsg := fmt.Sprintf("process failed: %v", err)
		logContent := fmt.Sprintf("Task: %s\nError: %s\n\n--- Output ---\n%s\n", task, errMsg, output)
		_ = os.WriteFile(logFile, []byte(logContent), 0644)
		return &QuickResult{
			Success:  false,
			Output:   output,
			Error:    errMsg,
			Duration: time.Since(startTime),
			LogFile:  logFile,
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

	// Write detailed log file
	logContent := fmt.Sprintf("Task: %s\nStarted: %s\nDuration: %s\nTokens: %d\nCost: $%.4f\n\n--- Output ---\n%s\n",
		task, startTime.Format(time.RFC3339), time.Since(startTime), tracker.GetUsage().TotalTokens, tracker.GetCost(), output)
	if commitErr != nil {
		logContent += fmt.Sprintf("\n--- Auto-commit ---\nError: %v\n", commitErr)
	} else {
		logContent += "\n--- Auto-commit ---\nSuccess\n"
	}
	_ = os.WriteFile(logFile, []byte(logContent), 0644)

	return &QuickResult{
		Success:    true,
		Output:     output,
		TokensUsed: tracker.GetUsage().TotalTokens,
		Cost:       tracker.GetCost(),
		Duration:   time.Since(startTime),
		LogFile:    logFile,
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
