package agent

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ShayCichocki/alphie/pkg/models"
)

// autoCommitChanges commits any uncommitted changes in the worktree.
// This ensures agent changes are preserved when the worktree is removed.
func (e *Executor) autoCommitChanges(worktreePath, taskTitle string) error {
	// Check if there are any changes to commit
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = worktreePath
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
	addCmd.Dir = worktreePath
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %s: %w", string(output), err)
	}

	// Commit with task title as message
	commitMsg := fmt.Sprintf("Agent: %s", taskTitle)
	commitCmd := exec.Command("git", "commit", "-m", commitMsg)
	commitCmd.Dir = worktreePath
	if output, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s: %w", string(output), err)
	}

	return nil
}

// getModifiedFiles returns a list of files modified in the worktree since the last commit.
// This is used to determine what files were created/modified for verification generation.
func (e *Executor) getModifiedFiles(workDir string) []string {
	// Try to get files modified compared to the parent commit
	cmd := exec.Command("git", "diff", "--name-only", "HEAD~1")
	cmd.Dir = workDir
	output, err := cmd.Output()
	if err != nil {
		// If that fails (e.g., no parent commit), try getting all changed files
		cmd = exec.Command("git", "diff", "--name-only", "--cached")
		cmd.Dir = workDir
		output, err = cmd.Output()
		if err != nil {
			return nil
		}
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var result []string
	for _, line := range lines {
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

// writeLogFile writes the execution log to the specified file.
func (e *Executor) writeLogFile(logFile string, task *models.Task, tier models.Tier, result *ExecutionResult, startTime time.Time) {
	var logContent strings.Builder
	logContent.WriteString(fmt.Sprintf("Task: %s\n", task.Title))
	logContent.WriteString(fmt.Sprintf("Task ID: %s\n", task.ID))
	logContent.WriteString(fmt.Sprintf("Tier: %s\n", tier))
	logContent.WriteString(fmt.Sprintf("Model: %s\n", result.Model))
	logContent.WriteString(fmt.Sprintf("Started: %s\n", startTime.Format(time.RFC3339)))
	logContent.WriteString(fmt.Sprintf("Duration: %s\n", result.Duration))
	logContent.WriteString(fmt.Sprintf("Tokens: %d\n", result.TokensUsed))
	logContent.WriteString(fmt.Sprintf("Cost: $%.4f\n", result.Cost))
	logContent.WriteString(fmt.Sprintf("Success: %v\n", result.Success))
	if result.Error != "" {
		logContent.WriteString(fmt.Sprintf("Error: %s\n", result.Error))
	}
	logContent.WriteString("\n--- Output ---\n")
	logContent.WriteString(result.Output)
	logContent.WriteString("\n")
	_ = os.WriteFile(logFile, []byte(logContent.String()), 0644)
}
