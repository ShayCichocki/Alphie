// Package orchestrator provides session resume functionality for cross-session continuity.
package orchestrator

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/shayc/alphie/internal/agent"
	"github.com/shayc/alphie/internal/prog"
)

// ResumableSession represents an in-progress session that can be resumed.
type ResumableSession struct {
	EpicID          string   // The prog epic ID
	Title           string   // Epic title (usually the original task description)
	CompletedTasks  int      // Number of completed tasks
	TotalTasks      int      // Total number of tasks in the epic
	IncompleteTasks []string // IDs of incomplete tasks
}

// SessionRecoveryResult contains the result of session recovery checks.
type SessionRecoveryResult struct {
	// HasResumableSessions indicates if any resumable sessions were found.
	HasResumableSessions bool
	// ResumableSessions contains the list of sessions that can be resumed.
	ResumableSessions []ResumableSession
	// OrphanedWorktrees contains paths to orphaned worktrees that should be cleaned.
	OrphanedWorktrees []string
	// RecommendedEpicID is the most recent in-progress epic, if any.
	// This is suggested for automatic resume.
	RecommendedEpicID string
}

// SessionResumeChecker provides functionality to check for and recover interrupted sessions.
type SessionResumeChecker struct {
	progClient      *prog.Client
	worktreeManager *agent.WorktreeManager
	repoPath        string
}

// NewSessionResumeChecker creates a new SessionResumeChecker.
// progClient may be nil if prog features are disabled.
// worktreeManager may be nil if worktree cleanup is not needed.
func NewSessionResumeChecker(progClient *prog.Client, worktreeManager *agent.WorktreeManager, repoPath string) *SessionResumeChecker {
	return &SessionResumeChecker{
		progClient:      progClient,
		worktreeManager: worktreeManager,
		repoPath:        repoPath,
	}
}

// CheckForResumableSessions checks for sessions that can be resumed.
// Returns a SessionRecoveryResult with information about resumable sessions
// and orphaned worktrees.
func (src *SessionResumeChecker) CheckForResumableSessions() (*SessionRecoveryResult, error) {
	result := &SessionRecoveryResult{}

	// Check prog for in-progress epics
	if src.progClient != nil {
		epics, err := src.progClient.ListOpenOrInProgressEpics()
		if err != nil {
			log.Printf("[session-resume] warning: failed to list epics: %v", err)
		} else if len(epics) > 0 {
			result.HasResumableSessions = true

			for _, epic := range epics {
				completed, total, err := src.progClient.ComputeEpicProgress(epic.ID)
				if err != nil {
					log.Printf("[session-resume] warning: failed to compute progress for epic %s: %v", epic.ID, err)
					continue
				}

				// Get incomplete task IDs
				incompleteTasks, err := src.progClient.GetIncompleteTasks(epic.ID)
				if err != nil {
					log.Printf("[session-resume] warning: failed to get incomplete tasks for epic %s: %v", epic.ID, err)
					continue
				}

				var incompleteIDs []string
				for _, task := range incompleteTasks {
					incompleteIDs = append(incompleteIDs, task.ID)
				}

				session := ResumableSession{
					EpicID:          epic.ID,
					Title:           epic.Title,
					CompletedTasks:  completed,
					TotalTasks:      total,
					IncompleteTasks: incompleteIDs,
				}
				result.ResumableSessions = append(result.ResumableSessions, session)

				// The first in-progress epic is the recommended one
				if epic.Status == prog.StatusInProgress && result.RecommendedEpicID == "" {
					result.RecommendedEpicID = epic.ID
				}
			}

			// If no in-progress epic, recommend the first open one
			if result.RecommendedEpicID == "" && len(epics) > 0 {
				result.RecommendedEpicID = epics[0].ID
			}
		}
	}

	// Check for orphaned worktrees
	if src.worktreeManager != nil {
		orphans, err := src.findOrphanedWorktrees()
		if err != nil {
			log.Printf("[session-resume] warning: failed to list orphaned worktrees: %v", err)
		} else {
			result.OrphanedWorktrees = orphans
		}
	}

	return result, nil
}

// findOrphanedWorktrees finds worktrees that are not associated with any active session.
func (src *SessionResumeChecker) findOrphanedWorktrees() ([]string, error) {
	if src.worktreeManager == nil {
		return nil, nil
	}

	// Get all worktrees
	worktrees, err := src.worktreeManager.List()
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}

	// Filter to alphie worktrees that might be orphaned
	var orphans []string
	for _, wt := range worktrees {
		// Skip main repo
		if wt.Path == src.repoPath {
			continue
		}

		// Check if this is an alphie worktree
		if strings.HasPrefix(wt.BranchName, "agent-") ||
			strings.HasPrefix(wt.BranchName, "alphie/") ||
			strings.HasPrefix(wt.BranchName, "session-") {
			// This is an alphie worktree - it might be orphaned
			// We'll consider it orphaned unless it matches an active prog task
			isOrphaned := true

			if src.progClient != nil {
				// Try to correlate with prog state
				// The branch name might contain a task ID we can match
				isOrphaned = !src.isWorktreeAssociatedWithActiveTask(wt)
			}

			if isOrphaned {
				orphans = append(orphans, wt.Path)
			}
		}
	}

	return orphans, nil
}

// isWorktreeAssociatedWithActiveTask checks if a worktree is associated with an active prog task.
func (src *SessionResumeChecker) isWorktreeAssociatedWithActiveTask(wt *agent.Worktree) bool {
	if src.progClient == nil {
		return false
	}

	// The worktree branch might be named after a task ID
	// Try to find it in prog
	// Branch patterns: agent-<id>, alphie/<task-id>, session-<id>
	possibleID := wt.AgentID
	if possibleID == "" {
		// Try to extract from branch name
		if strings.HasPrefix(wt.BranchName, "alphie/") {
			possibleID = strings.TrimPrefix(wt.BranchName, "alphie/")
		}
	}

	if possibleID == "" {
		return false
	}

	// Check if this ID corresponds to an active task
	item, err := src.progClient.GetItem(possibleID)
	if err != nil {
		return false
	}

	// Consider it active if the task is open or in-progress
	return item.Status == prog.StatusOpen || item.Status == prog.StatusInProgress
}

// CleanupOrphanedWorktrees removes orphaned worktrees.
// Returns the number of worktrees cleaned up.
func (src *SessionResumeChecker) CleanupOrphanedWorktrees(verbose func(string)) (int, error) {
	if src.worktreeManager == nil {
		return 0, nil
	}

	orphans, err := src.findOrphanedWorktrees()
	if err != nil {
		return 0, fmt.Errorf("find orphans: %w", err)
	}

	cleaned := 0
	for _, path := range orphans {
		if err := src.worktreeManager.Remove(path, true); err != nil {
			log.Printf("[session-resume] warning: failed to remove worktree %s: %v", path, err)
			continue
		}
		if verbose != nil {
			verbose(path)
		}
		cleaned++
	}

	// Prune any dangling worktree references
	if err := src.worktreeManager.Prune(); err != nil {
		log.Printf("[session-resume] warning: failed to prune worktrees: %v", err)
	}

	return cleaned, nil
}

// ReconcileProgStateWithWorktrees reconciles prog task state with actual worktree state.
// This handles cases where a session was interrupted and worktrees may be out of sync.
func (src *SessionResumeChecker) ReconcileProgStateWithWorktrees(epicID string) error {
	if src.progClient == nil || src.worktreeManager == nil {
		return nil
	}

	// Get tasks from the epic
	tasks, err := src.progClient.GetChildTasks(epicID)
	if err != nil {
		return fmt.Errorf("get child tasks: %w", err)
	}

	// Get current worktrees
	worktrees, err := src.worktreeManager.List()
	if err != nil {
		return fmt.Errorf("list worktrees: %w", err)
	}

	// Build a map of existing worktree paths
	worktreeMap := make(map[string]*agent.Worktree)
	for _, wt := range worktrees {
		// Use base name for matching
		worktreeMap[filepath.Base(wt.Path)] = wt
	}

	// Check each task
	for _, task := range tasks {
		// If task is in-progress but worktree doesn't exist, reset to open
		if task.Status == prog.StatusInProgress {
			expectedWorktreeName := fmt.Sprintf("agent-%s", task.ID)
			if _, exists := worktreeMap[expectedWorktreeName]; !exists {
				// Worktree doesn't exist - task was interrupted
				log.Printf("[session-resume] task %s marked in-progress but worktree missing, resetting to open", task.ID)
				if err := src.progClient.Reopen(task.ID); err != nil {
					log.Printf("[session-resume] warning: failed to reset task %s: %v", task.ID, err)
				} else {
					if err := src.progClient.AddLog(task.ID, "Task reset to open: worktree not found during session resume"); err != nil {
						log.Printf("[session-resume] warning: failed to log task reset: %v", err)
					}
				}
			}
		}
	}

	return nil
}

// FormatResumeSuggestion returns a formatted message suggesting epic resume.
func FormatResumeSuggestion(result *SessionRecoveryResult) string {
	if !result.HasResumableSessions || len(result.ResumableSessions) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n=== Resumable Sessions Found ===\n")

	for _, session := range result.ResumableSessions {
		sb.WriteString(fmt.Sprintf("\nEpic: %s\n", session.EpicID))
		sb.WriteString(fmt.Sprintf("  Title: %s\n", session.Title))
		sb.WriteString(fmt.Sprintf("  Progress: %d/%d tasks completed\n", session.CompletedTasks, session.TotalTasks))
		if len(session.IncompleteTasks) > 0 && len(session.IncompleteTasks) <= 5 {
			sb.WriteString(fmt.Sprintf("  Remaining: %s\n", strings.Join(session.IncompleteTasks, ", ")))
		} else if len(session.IncompleteTasks) > 5 {
			sb.WriteString(fmt.Sprintf("  Remaining: %d tasks\n", len(session.IncompleteTasks)))
		}
	}

	if result.RecommendedEpicID != "" {
		sb.WriteString(fmt.Sprintf("\nTo resume the most recent session, run:\n"))
		sb.WriteString(fmt.Sprintf("  alphie run --epic %s \"<task description>\"\n", result.RecommendedEpicID))
	}

	if len(result.OrphanedWorktrees) > 0 {
		sb.WriteString(fmt.Sprintf("\nNote: %d orphaned worktree(s) found. Run 'alphie cleanup' to remove them.\n", len(result.OrphanedWorktrees)))
	}

	sb.WriteString("\n================================\n")
	return sb.String()
}
