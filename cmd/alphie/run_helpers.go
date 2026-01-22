package main

import (
	"fmt"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/config"
	"github.com/ShayCichocki/alphie/internal/orchestrator"
	"github.com/ShayCichocki/alphie/internal/prog"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// modelForTier returns the Claude model to use for a given tier.
func modelForTier(tier models.Tier) string {
	switch tier {
	case models.TierQuick:
		return "claude-haiku-3-5-20241022"
	case models.TierScout:
		return "claude-haiku-3-5-20241022"
	case models.TierBuilder:
		return "claude-sonnet-4-20250514"
	case models.TierArchitect:
		return "claude-opus-4-20250514"
	default:
		return "claude-sonnet-4-20250514"
	}
}

// maxAgentsFromTierConfigs returns the maximum concurrent agents from tier configs.
// Falls back to hardcoded defaults if tier configs are nil.
func maxAgentsFromTierConfigs(tier models.Tier, tierConfigs *config.TierConfigs) int {
	if tierConfigs != nil {
		tc := tierConfigs.Get(tier)
		if tc != nil && tc.MaxAgents > 0 {
			return tc.MaxAgents
		}
	}
	// Fallback to hardcoded defaults
	switch tier {
	case models.TierQuick:
		return 1
	case models.TierScout:
		return 2
	case models.TierBuilder:
		return 3
	case models.TierArchitect:
		return 5
	default:
		return 3
	}
}

// checkAndReportResumableSessions checks for and reports resumable sessions from prog state.
// This is called on startup when not explicitly resuming an epic.
func checkAndReportResumableSessions(progClient *prog.Client, repoPath string) error {
	// Create worktree manager for orphan detection
	wtManager, err := agent.NewWorktreeManager("", repoPath)
	if err != nil {
		// Non-fatal - continue without worktree management
		wtManager = nil
	}

	// Create session resume checker
	checker := orchestrator.NewSessionResumeChecker(progClient, wtManager, repoPath)

	// Check for resumable sessions
	result, err := checker.CheckForResumableSessions()
	if err != nil {
		return fmt.Errorf("check for resumable sessions: %w", err)
	}

	// Report any resumable sessions found
	if result.HasResumableSessions {
		suggestion := orchestrator.FormatResumeSuggestion(result)
		if suggestion != "" {
			fmt.Print(suggestion)
		}
	}

	// Perform startup worktree cleanup
	if wtManager != nil && len(result.OrphanedWorktrees) > 0 {
		// Auto-cleanup orphaned worktrees at startup
		cleaned, err := checker.CleanupOrphanedWorktrees(func(path string) {
			fmt.Printf("Cleaned up orphaned worktree: %s\n", path)
		})
		if err != nil {
			fmt.Printf("Warning: worktree cleanup encountered errors: %v\n", err)
		} else if cleaned > 0 {
			fmt.Printf("Cleaned up %d orphaned worktree(s)\n", cleaned)
		}
	}

	// If resuming an epic would be possible, reconcile worktree state
	if result.RecommendedEpicID != "" && wtManager != nil {
		if err := checker.ReconcileProgStateWithWorktrees(result.RecommendedEpicID); err != nil {
			fmt.Printf("Warning: failed to reconcile prog state with worktrees: %v\n", err)
		}
	}

	return nil
}
