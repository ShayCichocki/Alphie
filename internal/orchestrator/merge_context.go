package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/ShayCichocki/alphie/internal/git"
	"github.com/ShayCichocki/alphie/internal/graph"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// MergeContext aggregates comprehensive context for intelligent merge conflict resolution.
// This provides the merge agent with full task history, branch diffs, and conflict details.
type MergeContext struct {
	// TaskID is the ID of the task that triggered the conflict.
	TaskID string
	// TargetBranch is the branch being merged into (e.g., session branch or main).
	TargetBranch string
	// AgentBranch is the branch with conflicting changes.
	AgentBranch string
	// ConflictingFiles lists files with merge conflicts.
	ConflictingFiles []string
	// TaskHistory contains all completed tasks with summaries.
	TaskHistory []TaskSummary
	// BranchDiff contains the diff from target branch.
	BranchDiff string
	// AgentDiff contains the diff from agent branch.
	AgentDiff string
	// MergeBase is the common ancestor commit.
	MergeBase string
	// ConflictDetails provides detailed information about each conflicting file.
	ConflictDetails []ConflictDetail
}

// TaskSummary contains a summary of a completed task for context.
type TaskSummary struct {
	ID          string
	Title       string
	Description string
	FilesChanged []string
	Intent      string
}

// ConflictDetail provides detailed information about a specific conflict.
type ConflictDetail struct {
	FilePath     string
	ConflictType string // "both_modified", "added_by_both", etc.
	TargetContent string
	AgentContent string
	MergeBaseContent string
}

// MergeContextBuilder builds comprehensive merge context for conflict resolution.
type MergeContextBuilder struct {
	repoPath string
	git      git.Runner
	graph    *graph.DependencyGraph // For accessing completed task information
}

// NewMergeContextBuilder creates a new merge context builder.
func NewMergeContextBuilder(repoPath string, gitRunner git.Runner, g *graph.DependencyGraph) *MergeContextBuilder {
	return &MergeContextBuilder{
		repoPath: repoPath,
		git:      gitRunner,
		graph:    g,
	}
}

// Build constructs comprehensive merge context for the given conflict.
func (b *MergeContextBuilder) Build(ctx context.Context, targetBranch string, req *MergeRequest, conflictFiles []string) (*MergeContext, error) {
	mergeCtx := &MergeContext{
		TaskID:           req.TaskID,
		TargetBranch:     targetBranch,
		AgentBranch:      req.AgentBranch,
		ConflictingFiles: conflictFiles,
		TaskHistory:      []TaskSummary{},
		ConflictDetails:  []ConflictDetail{},
	}

	// Get merge base
	mergeBase, err := b.getMergeBase(targetBranch, req.AgentBranch)
	if err != nil {
		// Non-fatal - continue without merge base
		mergeCtx.MergeBase = "unknown"
	} else {
		mergeCtx.MergeBase = mergeBase
	}

	// Get branch diffs
	targetDiff, err := b.getBranchDiff(mergeBase, targetBranch)
	if err == nil {
		mergeCtx.BranchDiff = targetDiff
	}

	agentDiff, err := b.getBranchDiff(mergeBase, req.AgentBranch)
	if err == nil {
		mergeCtx.AgentDiff = agentDiff
	}

	// Build task history
	if b.graph != nil {
		mergeCtx.TaskHistory = b.buildTaskHistory()
	}

	// Get detailed conflict information
	for _, file := range conflictFiles {
		detail, err := b.getConflictDetail(file, targetBranch, req.AgentBranch, mergeBase)
		if err == nil {
			mergeCtx.ConflictDetails = append(mergeCtx.ConflictDetails, detail)
		}
	}

	return mergeCtx, nil
}

// getMergeBase finds the common ancestor commit between two branches.
func (b *MergeContextBuilder) getMergeBase(branch1, branch2 string) (string, error) {
	output, err := b.git.MergeBase(branch1, branch2)
	if err != nil {
		return "", fmt.Errorf("get merge base: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// getBranchDiff gets the diff for a branch relative to the merge base.
func (b *MergeContextBuilder) getBranchDiff(base, branch string) (string, error) {
	if base == "" || base == "unknown" {
		return "", fmt.Errorf("invalid merge base")
	}
	output, err := b.git.DiffBetween(base, branch)
	if err != nil {
		return "", fmt.Errorf("get branch diff: %w", err)
	}
	return output, nil
}

// buildTaskHistory constructs summaries of all completed tasks.
func (b *MergeContextBuilder) buildTaskHistory() []TaskSummary {
	if b.graph == nil {
		return []TaskSummary{}
	}

	summaries := []TaskSummary{}

	// Get completed task IDs from the graph
	completedIDs := b.graph.GetCompletedIDs()

	for _, taskID := range completedIDs {
		task := b.graph.GetTask(taskID)
		if task == nil {
			continue
		}

		summary := TaskSummary{
			ID:          task.ID,
			Title:       task.Title,
			Description: task.Description,
			Intent:      extractIntent(task),
			FilesChanged: task.FileBoundaries,
		}
		summaries = append(summaries, summary)
	}

	return summaries
}

// extractIntent extracts the core intent from a task's description.
// This provides a concise summary of what the task was trying to accomplish.
func extractIntent(task *models.Task) string {
	// Use the first sentence of the description as the intent
	desc := strings.TrimSpace(task.Description)
	if desc == "" {
		return task.Title
	}

	// Find the first period or newline
	for i, r := range desc {
		if r == '.' || r == '\n' {
			if i > 0 {
				return desc[:i+1]
			}
		}
	}

	// If no period/newline, use first 100 chars
	if len(desc) > 100 {
		return desc[:100] + "..."
	}
	return desc
}

// getConflictDetail gets detailed information about a specific conflicting file.
func (b *MergeContextBuilder) getConflictDetail(filePath, targetBranch, agentBranch, mergeBase string) (ConflictDetail, error) {
	detail := ConflictDetail{
		FilePath: filePath,
	}

	// Get conflict type from git status
	status, err := b.git.Status()
	if err == nil {
		detail.ConflictType = extractConflictType(status, filePath)
	}

	// Get file contents from each branch
	targetContent, err := b.getFileContent(targetBranch, filePath)
	if err == nil {
		detail.TargetContent = targetContent
	}

	agentContent, err := b.getFileContent(agentBranch, filePath)
	if err == nil {
		detail.AgentContent = agentContent
	}

	if mergeBase != "" && mergeBase != "unknown" {
		baseContent, err := b.getFileContent(mergeBase, filePath)
		if err == nil {
			detail.MergeBaseContent = baseContent
		}
	}

	return detail, nil
}

// getFileContent retrieves the content of a file from a specific branch/commit.
func (b *MergeContextBuilder) getFileContent(ref, filePath string) (string, error) {
	output, err := b.git.ShowFile(ref, filePath)
	if err != nil {
		return "", fmt.Errorf("get file content from %s: %w", ref, err)
	}
	return output, nil
}

// extractConflictType extracts the conflict type for a file from git status output.
func extractConflictType(status, filePath string) string {
	lines := strings.Split(status, "\n")
	for _, line := range lines {
		if strings.Contains(line, filePath) {
			if strings.Contains(line, "both modified") {
				return "both_modified"
			}
			if strings.Contains(line, "added by us") {
				return "added_by_us"
			}
			if strings.Contains(line, "added by them") {
				return "added_by_them"
			}
			if strings.Contains(line, "deleted by us") {
				return "deleted_by_us"
			}
			if strings.Contains(line, "deleted by them") {
				return "deleted_by_them"
			}
			if strings.Contains(line, "both added") {
				return "added_by_both"
			}
		}
	}
	return "unknown"
}

// FormatForPrompt formats the merge context into a human-readable string for the merge agent prompt.
func (mc *MergeContext) FormatForPrompt() string {
	var sb strings.Builder

	sb.WriteString("## Conflict Context\n\n")
	sb.WriteString(fmt.Sprintf("**Task ID**: %s\n", mc.TaskID))
	sb.WriteString(fmt.Sprintf("**Target Branch**: %s (integrated work from all completed tasks)\n", mc.TargetBranch))
	sb.WriteString(fmt.Sprintf("**Agent Branch**: %s (new work that conflicts)\n", mc.AgentBranch))
	sb.WriteString(fmt.Sprintf("**Merge Base**: %s\n\n", mc.MergeBase))

	// Conflicting files
	sb.WriteString(fmt.Sprintf("**Conflicting Files** (%d):\n", len(mc.ConflictingFiles)))
	for _, file := range mc.ConflictingFiles {
		sb.WriteString(fmt.Sprintf("- %s\n", file))
	}
	sb.WriteString("\n")

	// Task history
	if len(mc.TaskHistory) > 0 {
		sb.WriteString("## Task History\n\n")
		sb.WriteString(fmt.Sprintf("**Completed Tasks**: %d\n\n", len(mc.TaskHistory)))
		for i, task := range mc.TaskHistory {
			sb.WriteString(fmt.Sprintf("%d. **%s** (%s)\n", i+1, task.Title, task.ID))
			sb.WriteString(fmt.Sprintf("   Intent: %s\n", task.Intent))
			if len(task.FilesChanged) > 0 {
				sb.WriteString(fmt.Sprintf("   Files: %s\n", strings.Join(task.FilesChanged, ", ")))
			}
			sb.WriteString("\n")
		}
	}

	// Conflict details
	if len(mc.ConflictDetails) > 0 {
		sb.WriteString("## Conflict Details\n\n")
		for _, detail := range mc.ConflictDetails {
			sb.WriteString(fmt.Sprintf("### %s\n", detail.FilePath))
			sb.WriteString(fmt.Sprintf("**Conflict Type**: %s\n\n", detail.ConflictType))

			if detail.MergeBaseContent != "" {
				sb.WriteString("**Merge Base Version**:\n```\n")
				sb.WriteString(truncateContent(detail.MergeBaseContent, 500))
				sb.WriteString("\n```\n\n")
			}

			if detail.TargetContent != "" {
				sb.WriteString(fmt.Sprintf("**Target Branch (%s) Version**:\n```\n", mc.TargetBranch))
				sb.WriteString(truncateContent(detail.TargetContent, 500))
				sb.WriteString("\n```\n\n")
			}

			if detail.AgentContent != "" {
				sb.WriteString(fmt.Sprintf("**Agent Branch (%s) Version**:\n```\n", mc.AgentBranch))
				sb.WriteString(truncateContent(detail.AgentContent, 500))
				sb.WriteString("\n```\n\n")
			}
		}
	}

	// Branch diffs summary
	if mc.BranchDiff != "" {
		sb.WriteString("## Target Branch Changes\n\n")
		sb.WriteString("```diff\n")
		sb.WriteString(truncateContent(mc.BranchDiff, 1000))
		sb.WriteString("\n```\n\n")
	}

	if mc.AgentDiff != "" {
		sb.WriteString("## Agent Branch Changes\n\n")
		sb.WriteString("```diff\n")
		sb.WriteString(truncateContent(mc.AgentDiff, 1000))
		sb.WriteString("\n```\n\n")
	}

	return sb.String()
}

// truncateContent truncates content to the specified length if needed.
func truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "\n... (truncated)"
}
