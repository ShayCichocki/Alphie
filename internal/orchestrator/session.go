package orchestrator

import (
	"fmt"
	"os/exec"
	"strings"
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
}

// NewSessionBranchManager creates a new SessionBranchManager.
// If greenfield is true, work goes directly to main without creating a session branch.
func NewSessionBranchManager(sessionID, repoPath string, greenfield bool) *SessionBranchManager {
	branchName := ""
	if !greenfield {
		branchName = fmt.Sprintf("session-%s", sessionID)
	}

	return &SessionBranchManager{
		sessionID:  sessionID,
		branchName: branchName,
		greenfield: greenfield,
		repoPath:   repoPath,
	}
}

// CreateBranch creates the session branch if not in greenfield mode.
// Returns nil if greenfield mode is enabled (no branch creation needed).
func (m *SessionBranchManager) CreateBranch() error {
	if m.greenfield {
		return nil
	}

	// Check if branch already exists
	cmd := exec.Command("git", "rev-parse", "--verify", m.branchName)
	cmd.Dir = m.repoPath
	if err := cmd.Run(); err == nil {
		// Branch exists, check it out
		cmd = exec.Command("git", "checkout", m.branchName)
		cmd.Dir = m.repoPath
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to checkout existing branch %s: %w", m.branchName, err)
		}
		return nil
	}

	// Create and checkout new branch
	cmd = exec.Command("git", "checkout", "-b", m.branchName)
	cmd.Dir = m.repoPath
	if err := cmd.Run(); err != nil {
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
	cmd := exec.Command("git", "rev-parse", "--verify", "main")
	cmd.Dir = m.repoPath
	if err := cmd.Run(); err != nil {
		// Try master if main doesn't exist
		cmd = exec.Command("git", "rev-parse", "--verify", "master")
		cmd.Dir = m.repoPath
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to find main or master branch: %w", err)
		}
		mainBranch = "master"
	}

	// Checkout the main branch
	cmd = exec.Command("git", "checkout", mainBranch)
	cmd.Dir = m.repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to checkout %s: %s: %w", mainBranch, string(output), err)
	}

	// Merge the session branch into main
	cmd = exec.Command("git", "merge", "--no-ff", "-m", fmt.Sprintf("Merge session %s", m.sessionID), m.branchName)
	cmd.Dir = m.repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to merge session branch %s into %s: %s: %w", m.branchName, mainBranch, string(output), err)
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
	cmd := exec.Command("git", "rev-parse", "--verify", "main")
	cmd.Dir = m.repoPath
	if err := cmd.Run(); err != nil {
		mainBranch = "master"
	}

	// First, checkout main to avoid deleting the current branch
	cmd = exec.Command("git", "checkout", mainBranch)
	cmd.Dir = m.repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to checkout %s before cleanup: %s: %w", mainBranch, string(output), err)
	}

	// Delete the session branch
	cmd = exec.Command("git", "branch", "-D", m.branchName)
	cmd.Dir = m.repoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete session branch %s: %w", m.branchName, err)
	}

	return nil
}
