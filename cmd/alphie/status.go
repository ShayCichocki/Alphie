package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ShayCichocki/alphie/internal/state"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current session state",
	Long: `Display the current state of the Alphie session.

Shows:
  - Active agents and their status
  - Running tasks and progress
  - Token usage and budget
  - Session duration and metrics`,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Get current working directory for project database
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Try project database first, then global
	dbPath := state.ProjectDBPath(cwd)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		dbPath = state.GlobalDBPath()
	}

	// Check if any database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Println("No active session. Run 'alphie run <task>' to start.")
		return nil
	}

	// Open database
	db, err := state.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Ensure schema is up to date
	if err := db.Migrate(); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	// Get active session
	session, err := db.GetActiveSession()
	if err != nil {
		return fmt.Errorf("get active session: %w", err)
	}

	if session == nil {
		fmt.Println("No active session. Run 'alphie run <task>' to start.")
		return displayRecentSessions(db)
	}

	// Display current session
	displaySession(session)

	// Get running agents
	runningStatus := state.AgentRunning
	agents, err := db.ListAgents(&runningStatus)
	if err != nil {
		return fmt.Errorf("list running agents: %w", err)
	}

	// Also get pending/waiting agents
	pendingStatus := state.AgentPending
	pendingAgents, err := db.ListAgents(&pendingStatus)
	if err != nil {
		return fmt.Errorf("list pending agents: %w", err)
	}

	waitingStatus := state.AgentWaitingApproval
	waitingAgents, err := db.ListAgents(&waitingStatus)
	if err != nil {
		return fmt.Errorf("list waiting agents: %w", err)
	}

	// Combine all active agents
	allActiveAgents := append(agents, pendingAgents...)
	allActiveAgents = append(allActiveAgents, waitingAgents...)

	displayAgents(db, agents, allActiveAgents)

	// Display recent completed sessions
	fmt.Println()
	return displayRecentSessions(db)
}

func displaySession(s *state.Session) {
	elapsed := formatDuration(time.Since(s.StartedAt))
	pct := 0
	if s.TokenBudget > 0 {
		pct = (s.TokensUsed * 100) / s.TokenBudget
	}

	fmt.Printf("Current Session: %s\n", s.ID)
	fmt.Printf("  Tier: %s\n", s.Tier)
	fmt.Printf("  Started: %s ago\n", elapsed)
	fmt.Printf("  Status: %s\n", s.Status)
	fmt.Printf("  Tokens: %s / %s (%d%%)\n",
		formatNumber(s.TokensUsed),
		formatNumber(s.TokenBudget),
		pct)
}

func displayAgents(db *state.DB, runningAgents []state.Agent, allAgents []state.Agent) {
	if len(allAgents) == 0 {
		fmt.Println("  Agents: none")
		return
	}

	fmt.Printf("  Agents: %d running\n", len(runningAgents))
	fmt.Println()

	if len(allAgents) > 0 {
		fmt.Println("Running Agents:")
		for _, a := range allAgents {
			// Get task title if available
			taskTitle := a.TaskID
			task, err := db.GetTask(a.TaskID)
			if err == nil && task != nil {
				taskTitle = task.Title
			}

			// Format duration or status
			statusStr := ""
			if a.StartedAt != nil {
				statusStr = fmt.Sprintf("(%s)", formatDuration(time.Since(*a.StartedAt)))
			} else if a.Status == state.AgentPending {
				statusStr = "(pending)"
			} else if a.Status == state.AgentWaitingApproval {
				statusStr = "(waiting)"
			}

			fmt.Printf("  %s: \"%s\" %s\n", a.ID, taskTitle, statusStr)
		}
	}
}

func displayRecentSessions(db *state.DB) error {
	// Get all sessions and filter to completed ones
	sessions, err := db.ListSessions(nil)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	// Filter to non-active sessions and limit to 5
	var recent []state.Session
	for _, s := range sessions {
		if s.Status != state.SessionActive {
			recent = append(recent, s)
			if len(recent) >= 5 {
				break
			}
		}
	}

	if len(recent) == 0 {
		return nil
	}

	fmt.Println("Recent Sessions:")
	for _, s := range recent {
		elapsed := formatDuration(time.Since(s.StartedAt))
		fmt.Printf("  %s: %s (%s ago)\n", s.ID, s.Status, elapsed)
	}

	return nil
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m > 0 {
			return fmt.Sprintf("%dh%dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	}
	days := int(d.Hours()) / 24
	return fmt.Sprintf("%dd", days)
}

// formatNumber formats a number with commas.
func formatNumber(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	// Add commas every 3 digits from the right
	var result strings.Builder
	offset := len(s) % 3
	if offset > 0 {
		result.WriteString(s[:offset])
		if len(s) > offset {
			result.WriteString(",")
		}
	}
	for i := offset; i < len(s); i += 3 {
		result.WriteString(s[i : i+3])
		if i+3 < len(s) {
			result.WriteString(",")
		}
	}
	return result.String()
}
