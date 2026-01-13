package state

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// InterruptedSession contains information about an interrupted session detected on startup.
type InterruptedSession struct {
	SessionID     string
	StartedAt     time.Time
	LastActivity  time.Time
	RunningAgents int
	Status        string
}

// RecoveryManager handles detection and recovery of interrupted sessions.
type RecoveryManager struct {
	db *DB
}

// NewRecoveryManager creates a new RecoveryManager with the given database.
func NewRecoveryManager(db *DB) *RecoveryManager {
	return &RecoveryManager{db: db}
}

// CheckForInterrupted detects any interrupted sessions on startup.
// It queries sessions with status != done/failed and checks if processes are still running.
// Returns nil if no interrupted session is found.
func (rm *RecoveryManager) CheckForInterrupted() (*InterruptedSession, error) {
	// Query for sessions that are not done or failed (i.e., active or in other intermediate states)
	sessions, err := rm.db.ListSessions(nil)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	for _, s := range sessions {
		// Skip completed sessions
		if s.Status == SessionCompleted || s.Status == SessionFailed || s.Status == SessionCanceled {
			continue
		}

		// Found an active/interrupted session - check for running agents
		runningAgents := 0
		agents, err := rm.db.ListAgents(nil)
		if err != nil {
			return nil, fmt.Errorf("list agents: %w", err)
		}

		var lastActivity time.Time = s.StartedAt
		for _, a := range agents {
			if a.Status == AgentRunning {
				// Check if process is still alive
				if a.PID > 0 && !isProcessAlive(a.PID) {
					// Process is dead but marked running - this is orphaned
					runningAgents++
				} else if a.PID > 0 {
					// Process is actually still running
					runningAgents++
				}
			}
			// Track last activity
			if a.StartedAt != nil && a.StartedAt.After(lastActivity) {
				lastActivity = *a.StartedAt
			}
		}

		return &InterruptedSession{
			SessionID:     s.ID,
			StartedAt:     s.StartedAt,
			LastActivity:  lastActivity,
			RunningAgents: runningAgents,
			Status:        string(s.Status),
		}, nil
	}

	return nil, nil
}

// Resume attempts to resume an interrupted session.
// It loads the session state from DB, identifies in-progress agents,
// checks if worktrees still exist, and returns info for restart.
func (rm *RecoveryManager) Resume(sessionID string) error {
	// Load session state from DB
	session, err := rm.db.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("session %s not found", sessionID)
	}

	// Identify in-progress agents
	agents, err := rm.db.ListAgents(nil)
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}

	for _, a := range agents {
		if a.Status == AgentRunning {
			// Check if process is still alive
			if a.PID > 0 && !isProcessAlive(a.PID) {
				// Process is dead - reset agent to pending for re-run
				a.Status = AgentPending
				a.PID = 0
				if err := rm.db.UpdateAgent(&a); err != nil {
					return fmt.Errorf("reset agent %s: %w", a.ID, err)
				}
				log.Printf("Reset orphaned agent %s to pending", a.ID)
			}

			// Check if worktree still exists
			if a.WorktreePath != "" {
				if _, err := os.Stat(a.WorktreePath); os.IsNotExist(err) {
					log.Printf("Worktree missing for agent %s: %s", a.ID, a.WorktreePath)
				}
			}
		}
	}

	log.Printf("Session %s resumed with state: %s", sessionID, session.Status)
	return nil
}

// Clean cleans up an interrupted session.
// It kills any orphaned processes, removes worktrees, and marks the session as failed.
func (rm *RecoveryManager) Clean(sessionID string) error {
	// Load session
	session, err := rm.db.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("session %s not found", sessionID)
	}

	// Get all agents
	agents, err := rm.db.ListAgents(nil)
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}

	// Kill orphaned processes and clean up agents
	for _, a := range agents {
		if a.Status == AgentRunning {
			// Try to kill the process if it's still alive
			if a.PID > 0 && isProcessAlive(a.PID) {
				process, err := os.FindProcess(a.PID)
				if err == nil {
					if err := process.Kill(); err != nil {
						log.Printf("Warning: failed to kill process %d: %v", a.PID, err)
					} else {
						log.Printf("Killed orphaned process %d for agent %s", a.PID, a.ID)
					}
				}
			}

			// Mark agent as failed
			a.Status = AgentFailed
			a.PID = 0
			if err := rm.db.UpdateAgent(&a); err != nil {
				return fmt.Errorf("fail agent %s: %w", a.ID, err)
			}
		}

		// Remove worktree if it exists
		if a.WorktreePath != "" {
			if err := removeWorktree(a.WorktreePath); err != nil {
				log.Printf("Warning: failed to remove worktree %s: %v", a.WorktreePath, err)
			} else {
				log.Printf("Removed worktree: %s", a.WorktreePath)
			}
		}
	}

	// Mark session as failed
	session.Status = SessionFailed
	if err := rm.db.UpdateSession(session); err != nil {
		return fmt.Errorf("mark session failed: %w", err)
	}

	// Prune git worktrees
	if err := pruneWorktrees(); err != nil {
		log.Printf("Warning: failed to prune worktrees: %v", err)
	}

	log.Printf("Session %s cleaned up and marked as failed", sessionID)
	return nil
}

// RecoveryInfo contains information about a recoverable session.
// Deprecated: Use InterruptedSession and RecoveryManager instead.
type RecoveryInfo struct {
	Session           *Session
	OrphanedAgents    []Agent
	OrphanedWorktrees []string
	StaleProcesses    []int
}

// CheckRecovery checks for interrupted sessions and orphaned resources.
// Deprecated: Use RecoveryManager.CheckForInterrupted instead.
func (db *DB) CheckRecovery() (*RecoveryInfo, error) {
	info := &RecoveryInfo{}

	// Check for active session
	session, err := db.GetActiveSession()
	if err != nil {
		return nil, fmt.Errorf("check active session: %w", err)
	}
	if session != nil {
		info.Session = session
	}

	// Check for running agents that may be stale
	status := AgentRunning
	agents, err := db.ListAgents(&status)
	if err != nil {
		return nil, fmt.Errorf("list running agents: %w", err)
	}

	for _, a := range agents {
		// Check if process is still alive
		if a.PID > 0 && !isProcessAlive(a.PID) {
			info.OrphanedAgents = append(info.OrphanedAgents, a)
			info.StaleProcesses = append(info.StaleProcesses, a.PID)
		}
	}

	// Check for orphaned worktrees
	worktrees, err := listAlphieWorktrees()
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}

	for _, wt := range worktrees {
		// Check if we have a record of this worktree
		found := false
		allAgents, _ := db.ListAgents(nil)
		for _, a := range allAgents {
			if a.WorktreePath == wt {
				found = true
				break
			}
		}
		if !found {
			info.OrphanedWorktrees = append(info.OrphanedWorktrees, wt)
		}
	}

	// Return nil if nothing to recover
	if info.Session == nil && len(info.OrphanedAgents) == 0 && len(info.OrphanedWorktrees) == 0 {
		return nil, nil
	}

	return info, nil
}

// RecoverSession attempts to recover or clean up an interrupted session.
// Deprecated: Use RecoveryManager.Resume or RecoveryManager.Clean instead.
func (db *DB) RecoverSession(resume bool) error {
	info, err := db.CheckRecovery()
	if err != nil {
		return err
	}
	if info == nil {
		return nil // Nothing to recover
	}

	if resume {
		// Resume: mark orphaned agents as pending so they can be re-run
		for _, a := range info.OrphanedAgents {
			a.Status = AgentPending
			a.PID = 0
			if err := db.UpdateAgent(&a); err != nil {
				return fmt.Errorf("reset agent %s: %w", a.ID, err)
			}
		}
	} else {
		// Clean: cancel the session and clean up
		if info.Session != nil {
			info.Session.Status = SessionCanceled
			if err := db.UpdateSession(info.Session); err != nil {
				return fmt.Errorf("cancel session: %w", err)
			}
		}

		// Mark orphaned agents as failed
		for _, a := range info.OrphanedAgents {
			a.Status = AgentFailed
			a.PID = 0
			if err := db.UpdateAgent(&a); err != nil {
				return fmt.Errorf("fail agent %s: %w", a.ID, err)
			}
		}
	}

	// Clean up orphaned worktrees
	for _, wt := range info.OrphanedWorktrees {
		if err := removeWorktree(wt); err != nil {
			// Log but don't fail
			fmt.Fprintf(os.Stderr, "warning: failed to remove worktree %s: %v\n", wt, err)
		}
	}

	// Prune git worktrees
	if err := pruneWorktrees(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to prune worktrees: %v\n", err)
	}

	return nil
}

// CleanupOrphanedResources removes all orphaned worktrees and marks stale agents.
func (db *DB) CleanupOrphanedResources() error {
	return db.RecoverSession(false)
}

// isProcessAlive checks if a process with the given PID is still running.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// listAlphieWorktrees lists all git worktrees that belong to Alphie.
func listAlphieWorktrees() ([]string, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	var worktrees []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			path := strings.TrimPrefix(line, "worktree ")
			// Only include alphie worktrees
			if strings.Contains(path, "alphie") || strings.Contains(path, "agent-") {
				worktrees = append(worktrees, path)
			}
		}
	}

	return worktrees, nil
}

// removeWorktree removes a git worktree.
func removeWorktree(path string) error {
	// Try normal remove first
	cmd := exec.Command("git", "worktree", "remove", path)
	if err := cmd.Run(); err != nil {
		// Force remove if normal fails
		cmd = exec.Command("git", "worktree", "remove", "-f", path)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("force remove worktree: %w", err)
		}
	}
	return nil
}

// pruneWorktrees prunes stale git worktree entries.
func pruneWorktrees() error {
	cmd := exec.Command("git", "worktree", "prune", "--expire", "now")
	return cmd.Run()
}

// WorktreeBasePath returns the base path for Alphie worktrees.
func WorktreeBasePath() string {
	cacheDir := os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" {
		home, _ := os.UserHomeDir()
		cacheDir = filepath.Join(home, ".cache")
	}
	return filepath.Join(cacheDir, "alphie", "worktrees")
}

// EnsureWorktreeDir ensures the worktree base directory exists.
func EnsureWorktreeDir() error {
	return os.MkdirAll(WorktreeBasePath(), 0755)
}

// AgentWorktreePath returns the worktree path for an agent.
func AgentWorktreePath(agentID string) string {
	return filepath.Join(WorktreeBasePath(), fmt.Sprintf("agent-%s", agentID))
}
