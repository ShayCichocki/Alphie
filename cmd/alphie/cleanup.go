package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/state"
	"github.com/spf13/cobra"
)

var (
	cleanupForce    bool
	cleanupVerbose  bool
	cleanupDryRun   bool
	cleanupSessions bool
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove orphaned worktrees and old sessions",
	Long: `Clean up orphaned git worktrees and old session data.

This command:
  - Lists all Alphie-related worktrees
  - Identifies orphaned worktrees (no active session)
  - Removes orphaned worktrees and their branches
  - Runs git worktree prune

With --sessions flag:
  - Deletes sessions older than 30 days from the database

Use this after a crash or interrupted session to clean up.

Examples:
  alphie cleanup              # Interactive cleanup with confirmation
  alphie cleanup --force      # Skip confirmation prompt
  alphie cleanup --dry-run    # Show what would be removed
  alphie cleanup -v           # Verbose output showing each removal
  alphie cleanup --sessions   # Also purge sessions older than 30 days`,
	RunE: runCleanup,
}

func init() {
	cleanupCmd.Flags().BoolVarP(&cleanupForce, "force", "f", false, "Skip confirmation prompt")
	cleanupCmd.Flags().BoolVarP(&cleanupVerbose, "verbose", "v", false, "Show each worktree as it's removed")
	cleanupCmd.Flags().BoolVar(&cleanupDryRun, "dry-run", false, "Show what would be removed without removing")
	cleanupCmd.Flags().BoolVar(&cleanupSessions, "sessions", false, "Purge sessions older than 30 days")
}

func runCleanup(cmd *cobra.Command, args []string) error {
	// Find git repository root
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	repoPath, err := findGitRoot(cwd)
	if err != nil {
		return fmt.Errorf("find git repository: %w", err)
	}

	// Create worktree manager
	wtManager, err := agent.NewWorktreeManager("", repoPath)
	if err != nil {
		return fmt.Errorf("create worktree manager: %w", err)
	}

	// Get active sessions from database
	activeSessions, err := getActiveSessions()
	if err != nil {
		// If we can't get active sessions, assume none are active
		// This is safer for cleanup
		if cleanupVerbose {
			fmt.Printf("Warning: Could not query active sessions: %v\n", err)
			fmt.Println("Proceeding with empty active session list")
		}
		activeSessions = []string{}
	}

	// List orphans
	orphans, err := wtManager.ListOrphans(activeSessions)
	if err != nil {
		return fmt.Errorf("list orphaned worktrees: %w", err)
	}

	if len(orphans) == 0 && !cleanupSessions {
		fmt.Println("No orphaned worktrees found.")
		return nil
	}

	if len(orphans) > 0 {
		// Display orphan count and list
		fmt.Printf("Found %d orphaned worktree(s):\n", len(orphans))
		for _, wt := range orphans {
			fmt.Printf("  - %s (branch: %s)\n", wt.Path, wt.BranchName)
		}
		fmt.Println()

		// Dry run - just show what would be removed
		if cleanupDryRun {
			fmt.Println("Dry run mode - no worktrees were removed.")
		} else {
			// Confirm before removal unless --force is specified
			if !cleanupForce {
				fmt.Print("Remove these worktrees? [y/N] ")
				reader := bufio.NewReader(os.Stdin)
				response, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("read confirmation: %w", err)
				}

				response = strings.TrimSpace(strings.ToLower(response))
				if response != "y" && response != "yes" {
					fmt.Println("Worktree cleanup cancelled.")
				} else {
					// Perform cleanup
					var verboseCallback func(path string)
					if cleanupVerbose {
						verboseCallback = func(path string) {
							fmt.Printf("Removed: %s\n", path)
						}
					}

					removed, err := wtManager.CleanupOrphans(activeSessions, verboseCallback)
					if err != nil {
						return fmt.Errorf("cleanup orphaned worktrees: %w", err)
					}

					fmt.Printf("Successfully removed %d orphaned worktree(s).\n", removed)
				}
			} else {
				// --force mode
				var verboseCallback func(path string)
				if cleanupVerbose {
					verboseCallback = func(path string) {
						fmt.Printf("Removed: %s\n", path)
					}
				}

				removed, err := wtManager.CleanupOrphans(activeSessions, verboseCallback)
				if err != nil {
					return fmt.Errorf("cleanup orphaned worktrees: %w", err)
				}

				fmt.Printf("Successfully removed %d orphaned worktree(s).\n", removed)
			}
		}
	} else {
		fmt.Println("No orphaned worktrees found.")
	}

	// Handle session cleanup if --sessions flag is set
	if cleanupSessions {
		if err := cleanupOldSessions(cwd); err != nil {
			return err
		}
	}

	return nil
}

// cleanupOldSessions purges sessions older than 30 days.
func cleanupOldSessions(cwd string) error {
	const sessionMaxAge = 30 * 24 * time.Hour // 30 days

	dbPath := state.ProjectDBPath(cwd)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		// Fall back to global database
		dbPath = state.GlobalDBPath()
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Println("No database found - no sessions to purge.")
		return nil
	}

	db, err := state.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if cleanupDryRun {
		// Count sessions that would be purged
		sessions, err := db.ListSessions(nil)
		if err != nil {
			return fmt.Errorf("list sessions: %w", err)
		}

		cutoff := time.Now().Add(-sessionMaxAge)
		count := 0
		for _, s := range sessions {
			if s.StartedAt.Before(cutoff) {
				count++
			}
		}
		fmt.Printf("Dry run: would purge %d session(s) older than 30 days.\n", count)
		return nil
	}

	purged, err := db.PurgeOldSessions(sessionMaxAge)
	if err != nil {
		return fmt.Errorf("purge old sessions: %w", err)
	}

	if purged > 0 {
		fmt.Printf("Purged %d session(s) older than 30 days.\n", purged)
	} else {
		fmt.Println("No sessions older than 30 days found.")
	}

	return nil
}

// getActiveSessions returns the list of active session IDs from the database.
func getActiveSessions() ([]string, error) {
	// Try project database first
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	dbPath := state.ProjectDBPath(cwd)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		// Fall back to global database
		dbPath = state.GlobalDBPath()
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		// No database exists, return empty list
		return []string{}, nil
	}

	db, err := state.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Query active sessions
	activeStatus := state.SessionActive
	sessions, err := db.ListSessions(&activeStatus)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	sessionIDs := make([]string, len(sessions))
	for i, s := range sessions {
		sessionIDs[i] = s.ID
	}

	return sessionIDs, nil
}

// findGitRoot finds the root of the git repository starting from the given directory.
func findGitRoot(startDir string) (string, error) {
	dir := startDir
	for {
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not in a git repository")
		}
		dir = parent
	}
}
