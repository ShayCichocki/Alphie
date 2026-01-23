package orchestrator

import (
	"fmt"
	"strings"

	"github.com/ShayCichocki/alphie/internal/git"
)

// protectedBranches are branches that cannot be directly worked on.
var protectedBranches = []string{"main", "master", "dev"}

// SessionBranchManager manages git branches for orchestrator sessions.
// It creates isolated session branches for agent work and handles cleanup.
type SessionBranchManager struct {
	sessionID  string
	branchName string
	greenfield bool
	repoPath   string
	git        git.Runner
}

// NewSessionBranchManager creates a new SessionBranchManager.
// If greenfield is true, work goes directly to main without creating a session branch.
// Branch name format: alphie-{spec-name}-{timestamp}
func NewSessionBranchManager(sessionID, repoPath string, greenfield bool, specName string) *SessionBranchManager {
	branchName := ""
	if !greenfield {
		if specName != "" {
			branchName = fmt.Sprintf("alphie-%s-%s", specName, sessionID)
		} else {
			// Fallback to session-based naming if no spec name provided
			branchName = fmt.Sprintf("alphie-session-%s", sessionID)
		}
	}

	return &SessionBranchManager{
		sessionID:  sessionID,
		branchName: branchName,
		greenfield: greenfield,
		repoPath:   repoPath,
		git:        git.NewRunner(repoPath),
	}
}

// NewSessionBranchManagerWithRunner creates a new SessionBranchManager with a custom git runner (for testing).
// Branch name format: alphie-{spec-name}-{timestamp}
func NewSessionBranchManagerWithRunner(sessionID, repoPath string, greenfield bool, specName string, runner git.Runner) *SessionBranchManager {
	branchName := ""
	if !greenfield {
		if specName != "" {
			branchName = fmt.Sprintf("alphie-%s-%s", specName, sessionID)
		} else {
			// Fallback to session-based naming if no spec name provided
			branchName = fmt.Sprintf("alphie-session-%s", sessionID)
		}
	}

	return &SessionBranchManager{
		sessionID:  sessionID,
		branchName: branchName,
		greenfield: greenfield,
		repoPath:   repoPath,
		git:        runner,
	}
}

// CreateBranch creates the session branch if not in greenfield mode.
// Returns nil if greenfield mode is enabled (no branch creation needed).
func (m *SessionBranchManager) CreateBranch() error {
	if m.greenfield {
		return nil
	}

	// Check if branch already exists
	exists, err := m.git.BranchExists(m.branchName)
	if err != nil {
		return fmt.Errorf("failed to check if branch exists: %w", err)
	}

	if exists {
		// Branch exists, check it out
		if err := m.git.CheckoutBranch(m.branchName); err != nil {
			return fmt.Errorf("failed to checkout existing branch %s: %w", m.branchName, err)
		}
		return nil
	}

	// Create and checkout new branch
	if err := m.git.CreateAndCheckoutBranch(m.branchName); err != nil {
		return fmt.Errorf("failed to create branch %s: %w", m.branchName, err)
	}

	return nil
}

// GetBranchName returns the session branch name.
// Returns empty string if in greenfield mode.
func (m *SessionBranchManager) GetBranchName() string {
	return m.branchName
}

// IsProtected checks if the given branch name is a protected branch.
func (m *SessionBranchManager) IsProtected(branch string) bool {
	normalized := strings.TrimSpace(strings.ToLower(branch))
	for _, protected := range protectedBranches {
		if normalized == protected {
			return true
		}
	}
	return false
}

// MergeToMain merges the session branch into main (or master).
// This should be called after all tasks complete successfully.
// Returns nil if greenfield mode is enabled (no branch to merge).
func (m *SessionBranchManager) MergeToMain() error {
	if m.greenfield {
		return nil
	}

	if m.branchName == "" {
		return nil
	}

	// Determine the main branch name
	mainBranch := "main"
	exists, err := m.git.BranchExists("main")
	if err != nil {
		return fmt.Errorf("failed to check if main branch exists: %w", err)
	}
	if !exists {
		// Try master if main doesn't exist
		exists, err = m.git.BranchExists("master")
		if err != nil {
			return fmt.Errorf("failed to check if master branch exists: %w", err)
		}
		if !exists {
			return fmt.Errorf("failed to find main or master branch")
		}
		mainBranch = "master"
	}

	// Commit any pending changes on session branch before merging to main
	// This catches any uncommitted changes from agent merges
	if _, err := m.git.Run("add", "."); err != nil {
		// If add fails, it might be because there are no changes - that's OK
		// But we should still try to continue
	}
	if _, err := m.git.Run("commit", "-m", fmt.Sprintf("Auto-commit pending changes before merging session %s", m.sessionID)); err != nil {
		// If commit fails, it might be because there are no changes - that's OK
		// Continue with the merge
	}

	// Checkout the main branch
	if err := m.git.CheckoutBranch(mainBranch); err != nil {
		return fmt.Errorf("failed to checkout %s: %w", mainBranch, err)
	}

	// Merge the session branch into main with a custom message
	if err := m.git.MergeNoFFMessage(m.branchName, fmt.Sprintf("Merge session %s", m.sessionID)); err != nil {
		return fmt.Errorf("failed to merge session branch %s into %s: %w", m.branchName, mainBranch, err)
	}

	return nil
}

// Cleanup deletes the session branch.
// This is typically called when a session is cancelled or after successful merge.
// Returns nil if greenfield mode is enabled (no branch to clean up).
func (m *SessionBranchManager) Cleanup() error {
	if m.greenfield {
		return nil
	}

	if m.branchName == "" {
		return nil
	}

	// Determine the main branch name
	mainBranch := "main"
	exists, err := m.git.BranchExists("main")
	if err != nil {
		mainBranch = "master"
	} else if !exists {
		mainBranch = "master"
	}

	// First, checkout main to avoid deleting the current branch
	if err := m.git.CheckoutBranch(mainBranch); err != nil {
		return fmt.Errorf("failed to checkout %s before cleanup: %w", mainBranch, err)
	}

	// Delete the session branch
	if err := m.git.DeleteBranch(m.branchName); err != nil {
		return fmt.Errorf("failed to delete session branch %s: %w", m.branchName, err)
	}

	return nil
}
