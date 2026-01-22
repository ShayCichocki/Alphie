// Package agent provides the AI agent implementation for Alphie.
package agent

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ShayCichocki/alphie/internal/git"
)

// Worktree represents a git worktree managed by Alphie.
type Worktree struct {
	Path       string    // Absolute path to the worktree directory
	BranchName string    // Name of the branch associated with this worktree
	AgentID    string    // ID of the agent that owns this worktree
	CreatedAt  time.Time // When the worktree was created
}

// WorktreeProvider defines the interface for worktree management.
// This interface allows mocking worktree operations in tests.
type WorktreeProvider interface {
	// Create creates a new worktree for the given agent.
	Create(agentID string) (*Worktree, error)
	// Remove removes a worktree at the given path.
	Remove(path string, force bool) error
	// Unlock unlocks a locked worktree.
	Unlock(path string) error
	// List returns all worktrees managed by this repository.
	List() ([]*Worktree, error)
	// Prune removes references to worktrees that no longer exist on disk.
	Prune() error
	// RecoverOrphaned finds and removes orphaned worktrees.
	RecoverOrphaned() ([]string, error)
	// ListOrphans returns a list of orphaned worktrees.
	ListOrphans(activeSessions []string) ([]*Worktree, error)
	// CleanupOrphans removes orphaned worktrees and returns the count of removed.
	CleanupOrphans(activeSessions []string, verbose func(path string)) (int, error)
	// StartupCleanup performs orphan detection and cleanup at startup.
	StartupCleanup(activeSessions []string) (int, error)
	// BaseDir returns the base directory where worktrees are created.
	BaseDir() string
	// RepoPath returns the path to the main git repository.
	RepoPath() string
}

// Verify WorktreeManager implements WorktreeProvider at compile time.
var _ WorktreeProvider = (*WorktreeManager)(nil)

// WorktreeManager handles git worktree operations for agent isolation.
type WorktreeManager struct {
	baseDir  string // Base directory for worktrees (e.g., ~/.cache/alphie/worktrees)
	repoPath string // Path to the main git repository
	git      git.Runner
	mu       sync.Mutex
}

// NewWorktreeManager creates a new WorktreeManager.
// baseDir is where worktrees will be created (defaults to ~/.cache/alphie/worktrees).
// repoPath is the path to the main git repository.
func NewWorktreeManager(baseDir, repoPath string) (*WorktreeManager, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home directory: %w", err)
		}
		baseDir = filepath.Join(home, ".cache", "alphie", "worktrees")
	}

	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create worktree base directory: %w", err)
	}

	return &WorktreeManager{
		baseDir:  baseDir,
		repoPath: repoPath,
		git:      git.NewRunner(repoPath),
	}, nil
}

// NewWorktreeManagerWithRunner creates a new WorktreeManager with a custom git runner (for testing).
func NewWorktreeManagerWithRunner(baseDir, repoPath string, runner git.Runner) (*WorktreeManager, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home directory: %w", err)
		}
		baseDir = filepath.Join(home, ".cache", "alphie", "worktrees")
	}

	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create worktree base directory: %w", err)
	}

	return &WorktreeManager{
		baseDir:  baseDir,
		repoPath: repoPath,
		git:      runner,
	}, nil
}

// Create creates a new worktree for the given agent.
// Returns the created Worktree with path and branch information.
func (m *WorktreeManager) Create(agentID string) (*Worktree, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if agentID == "" {
		agentID = uuid.New().String()
	}

	branchName := fmt.Sprintf("agent-%s", agentID)
	worktreePath := filepath.Join(m.baseDir, branchName)

	// Create the worktree with a new branch
	if err := m.git.WorktreeAddNewBranch(worktreePath, branchName); err != nil {
		return nil, fmt.Errorf("create worktree: %w", err)
	}

	return &Worktree{
		Path:       worktreePath,
		BranchName: branchName,
		AgentID:    agentID,
		CreatedAt:  time.Now(),
	}, nil
}

// Remove removes a worktree at the given path.
// If force is true, removes the worktree even if there are uncommitted changes.
func (m *WorktreeManager) Remove(path string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.git.WorktreeRemoveOptionalForce(path, force); err != nil {
		return fmt.Errorf("remove worktree: %w", err)
	}

	return nil
}

// Unlock unlocks a locked worktree.
func (m *WorktreeManager) Unlock(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.git.WorktreeUnlock(path); err != nil {
		return fmt.Errorf("unlock worktree: %w", err)
	}

	return nil
}

// List returns all worktrees managed by this repository.
func (m *WorktreeManager) List() ([]*Worktree, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	output, err := m.git.WorktreeListPorcelain()
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}

	return m.parseWorktreeList(output)
}

// parseWorktreeList parses the output of 'git worktree list --porcelain'.
func (m *WorktreeManager) parseWorktreeList(output string) ([]*Worktree, error) {
	var worktrees []*Worktree
	var current *Worktree

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			if current != nil {
				worktrees = append(worktrees, current)
				current = nil
			}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			current = &Worktree{
				Path: strings.TrimPrefix(line, "worktree "),
			}
		} else if strings.HasPrefix(line, "branch ") && current != nil {
			// Format: branch refs/heads/<name>
			branchRef := strings.TrimPrefix(line, "branch ")
			current.BranchName = strings.TrimPrefix(branchRef, "refs/heads/")

			// Extract agent ID from branch name if it matches our pattern
			if strings.HasPrefix(current.BranchName, "agent-") {
				current.AgentID = strings.TrimPrefix(current.BranchName, "agent-")
			}
		}
	}

	// Don't forget the last worktree if output doesn't end with blank line
	if current != nil {
		worktrees = append(worktrees, current)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse worktree list: %w", err)
	}

	return worktrees, nil
}

// Prune removes references to worktrees that no longer exist on disk.
func (m *WorktreeManager) Prune() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.git.WorktreePruneExpireNow(); err != nil {
		return fmt.Errorf("prune worktrees: %w", err)
	}

	return nil
}

// RecoverOrphaned finds and removes orphaned worktrees.
// Orphaned worktrees are those that exist in the base directory but are not
// tracked by git, or whose agent processes are no longer running.
func (m *WorktreeManager) RecoverOrphaned() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// First prune any that git knows about but don't exist
	if err := m.git.WorktreePruneExpireNow(); err != nil {
		return nil, fmt.Errorf("prune worktrees: %w", err)
	}

	// Get list of worktrees known to git
	output, err := m.git.WorktreeListPorcelain()
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}

	knownPaths := make(map[string]bool)
	worktrees, err := m.parseWorktreeListUnlocked(output)
	if err != nil {
		return nil, err
	}
	for _, wt := range worktrees {
		knownPaths[wt.Path] = true
	}

	// Check base directory for any unknown worktrees
	var recovered []string
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read worktree base directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		path := filepath.Join(m.baseDir, entry.Name())
		if knownPaths[path] {
			// Git knows about this worktree - leave it alone.
			// It's being tracked and may be in active use.
			continue
		}

		// Orphan directory - git doesn't know about it.
		// Try to remove it as a worktree first (in case git lost track but it's still valid)
		_ = m.git.WorktreeUnlock(path) // Ignore errors, it may not be locked

		if err := m.git.WorktreeRemove(path); err != nil {
			// Git worktree remove failed, try direct removal
			if err := os.RemoveAll(path); err != nil {
				continue // Skip if we can't remove it
			}
		}
		recovered = append(recovered, path)
	}

	return recovered, nil
}

// parseWorktreeListUnlocked is like parseWorktreeList but doesn't acquire the lock.
// Used internally when the lock is already held.
func (m *WorktreeManager) parseWorktreeListUnlocked(output string) ([]*Worktree, error) {
	var worktrees []*Worktree
	var current *Worktree

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			if current != nil {
				worktrees = append(worktrees, current)
				current = nil
			}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			current = &Worktree{
				Path: strings.TrimPrefix(line, "worktree "),
			}
		} else if strings.HasPrefix(line, "branch ") && current != nil {
			branchRef := strings.TrimPrefix(line, "branch ")
			current.BranchName = strings.TrimPrefix(branchRef, "refs/heads/")

			if strings.HasPrefix(current.BranchName, "agent-") {
				current.AgentID = strings.TrimPrefix(current.BranchName, "agent-")
			}
		}
	}

	if current != nil {
		worktrees = append(worktrees, current)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse worktree list: %w", err)
	}

	return worktrees, nil
}

// BaseDir returns the base directory where worktrees are created.
func (m *WorktreeManager) BaseDir() string {
	return m.baseDir
}

// RepoPath returns the path to the main git repository.
func (m *WorktreeManager) RepoPath() string {
	return m.repoPath
}

// alphieWorktreePatterns are patterns used to identify Alphie-managed worktrees.
var alphieWorktreePatterns = []string{"agent-", "alphie/", "session-"}

// isAlphieWorktree checks if a worktree is managed by Alphie based on branch name patterns.
func isAlphieWorktree(wt *Worktree) bool {
	for _, pattern := range alphieWorktreePatterns {
		if strings.HasPrefix(wt.BranchName, pattern) {
			return true
		}
	}
	return false
}

// extractSessionID extracts the session identifier from a worktree branch name.
// Returns empty string if the worktree is not an Alphie-managed worktree.
func extractSessionID(wt *Worktree) string {
	for _, pattern := range alphieWorktreePatterns {
		if strings.HasPrefix(wt.BranchName, pattern) {
			return strings.TrimPrefix(wt.BranchName, pattern)
		}
	}
	return ""
}

// ListOrphans returns a list of orphaned worktrees.
// An orphaned worktree is one that:
// - Matches Alphie worktree patterns (agent-, alphie/, session-)
// - Does not have a corresponding entry in activeSessions
// - Is not the main repository worktree
func (m *WorktreeManager) ListOrphans(activeSessions []string) ([]*Worktree, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get all worktrees
	output, err := m.git.WorktreeListPorcelain()
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}

	worktrees, err := m.parseWorktreeListUnlocked(output)
	if err != nil {
		return nil, err
	}

	// Build a set of active session IDs for fast lookup
	activeSet := make(map[string]bool)
	for _, sessionID := range activeSessions {
		activeSet[sessionID] = true
	}

	// Find orphaned worktrees
	var orphans []*Worktree
	for _, wt := range worktrees {
		// Skip non-Alphie worktrees
		if !isAlphieWorktree(wt) {
			continue
		}

		// Skip the main repo (path equals repoPath)
		if wt.Path == m.repoPath {
			continue
		}

		// Check if this worktree's session is active
		sessionID := extractSessionID(wt)
		if sessionID != "" && activeSet[sessionID] {
			continue
		}

		// This is an orphan
		orphans = append(orphans, wt)
	}

	return orphans, nil
}

// CleanupOrphans removes orphaned worktrees and returns the count of removed worktrees.
// An orphaned worktree is one that:
// - Matches Alphie worktree patterns (agent-, alphie/, session-)
// - Does not have a corresponding entry in activeSessions
//
// The cleanup process:
// 1. List all Alphie worktrees
// 2. Filter to worktrees not matching any active session
// 3. Remove orphans with git worktree remove -f
// 4. Prune with git worktree prune --expire now
// 5. Return count of removed worktrees
//
// If verbose callback is provided, it will be called for each removed worktree.
func (m *WorktreeManager) CleanupOrphans(activeSessions []string, verbose func(path string)) (int, error) {
	orphans, err := m.ListOrphans(activeSessions)
	if err != nil {
		return 0, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	removed := 0
	for _, wt := range orphans {
		// Try to unlock in case it's locked
		_ = m.git.WorktreeUnlock(wt.Path) // Ignore errors, it may not be locked

		// Force remove the worktree
		if err := m.git.WorktreeRemove(wt.Path); err != nil {
			// If git worktree remove fails, try removing the directory directly
			if err := os.RemoveAll(wt.Path); err != nil {
				continue // Skip if we can't remove it
			}
		}

		if verbose != nil {
			verbose(wt.Path)
		}
		removed++
	}

	// Final prune to clean up any dangling references
	_ = m.git.WorktreePruneExpireNow() // Ignore errors, worktrees already removed

	return removed, nil
}

// StartupCleanup performs orphan detection and cleanup at startup.
// It queries the database for active sessions and cleans up any orphaned worktrees.
// This should be called when the application starts to recover from crashes.
func (m *WorktreeManager) StartupCleanup(activeSessions []string) (int, error) {
	return m.CleanupOrphans(activeSessions, nil)
}
