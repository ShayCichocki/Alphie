package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/ShayCichocki/alphie/pkg/models"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Status icons for agent states.
const (
	iconRunning  = "[●]"
	iconWaiting  = "[◐]"
	iconDone     = "[✓]"
	iconFailed   = "[✗]"
	iconQuestion = "[?]"
	iconPaused   = "[◌]"
	iconPending  = "[○]"
)

// AgentGrid displays a list of agents with status information.
type AgentGrid struct {
	agents   []*models.Agent
	selected int
	width    int
	height   int

	// Styles
	headerStyle    lipgloss.Style
	rowStyle       lipgloss.Style
	selectedStyle  lipgloss.Style
	statusRunning  lipgloss.Style
	statusWaiting  lipgloss.Style
	statusDone     lipgloss.Style
	statusFailed   lipgloss.Style
	statusQuestion lipgloss.Style
	statusPaused   lipgloss.Style
	statusPending  lipgloss.Style
}

// NewAgentGrid creates a new AgentGrid instance.
func NewAgentGrid() *AgentGrid {
	return &AgentGrid{
		agents:   make([]*models.Agent, 0),
		selected: 0,

		headerStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("7")).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(lipgloss.Color("240")),

		rowStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")),

		selectedStyle: lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("15")).
			Bold(true),

		statusRunning: lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")), // Green

		statusWaiting: lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")), // Orange

		statusDone: lipgloss.NewStyle().
			Foreground(lipgloss.Color("28")), // Dark green

		statusFailed: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")), // Red

		statusQuestion: lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")), // Yellow

		statusPaused: lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")), // Gray

		statusPending: lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")), // Gray
	}
}

// Update handles input messages.
func (g *AgentGrid) Update(msg tea.Msg) (*AgentGrid, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if g.selected > 0 {
				g.selected--
			}
		case "down", "j":
			if g.selected < len(g.agents)-1 {
				g.selected++
			}
		case "space":
			// Pause/resume selected agent
			if agent := g.SelectedAgent(); agent != nil {
				return g, g.togglePause(agent)
			}
		case "K":
			// Kill selected agent
			if agent := g.SelectedAgent(); agent != nil {
				return g, g.killAgent(agent)
			}
		case "enter":
			// Focus on selected agent
			if agent := g.SelectedAgent(); agent != nil {
				return g, g.focusAgent(agent)
			}
		}

	case tea.WindowSizeMsg:
		g.width = msg.Width
		g.height = msg.Height
	}

	return g, nil
}

// View renders the agent grid.
func (g *AgentGrid) View() string {
	if len(g.agents) == 0 {
		return g.rowStyle.Render("No active agents")
	}

	var b strings.Builder

	// Column widths
	colStatus := 5
	colID := 10
	colTask := 30
	colDuration := 12

	// Header
	header := fmt.Sprintf("%-*s %-*s %-*s %-*s",
		colStatus, "STS",
		colID, "AGENT ID",
		colTask, "TASK",
		colDuration, "DURATION",
	)
	b.WriteString(g.headerStyle.Render(header))
	b.WriteString("\n")

	// Rows
	for i, agent := range g.agents {
		row := g.renderRow(agent, colStatus, colID, colTask, colDuration)
		if i == g.selected {
			b.WriteString(g.selectedStyle.Render(row))
		} else {
			b.WriteString(g.rowStyle.Render(row))
		}
		b.WriteString("\n")
	}

	// Footer with key hints
	b.WriteString("\n")
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render("[space] pause agent  [p] pause all  [K] kill  [enter] focus  [j/k] navigate")
	b.WriteString(footer)

	return b.String()
}

// renderRow renders a single agent row.
func (g *AgentGrid) renderRow(agent *models.Agent, colStatus, colID, colTask, colDuration int) string {
	icon := g.statusIcon(agent.Status)
	agentID := truncate(agent.ID, colID-2)
	taskName := truncate(agent.TaskID, colTask-2)
	duration := formatDuration(time.Since(agent.StartedAt))

	return fmt.Sprintf("%-*s %-*s %-*s %-*s",
		colStatus, icon,
		colID, agentID,
		colTask, taskName,
		colDuration, duration,
	)
}

// statusIcon returns the appropriate icon for an agent status.
func (g *AgentGrid) statusIcon(status models.AgentStatus) string {
	switch status {
	case models.AgentStatusRunning:
		return g.statusRunning.Render(iconRunning)
	case models.AgentStatusWaitingApproval:
		return g.statusQuestion.Render(iconQuestion)
	case models.AgentStatusDone:
		return g.statusDone.Render(iconDone)
	case models.AgentStatusFailed:
		return g.statusFailed.Render(iconFailed)
	case models.AgentStatusPaused:
		return g.statusPaused.Render(iconPaused)
	case models.AgentStatusPending:
		return g.statusPending.Render(iconPending)
	default:
		return g.statusWaiting.Render(iconWaiting)
	}
}

// Select sets the currently selected agent index.
func (g *AgentGrid) Select(index int) {
	if index >= 0 && index < len(g.agents) {
		g.selected = index
	}
}

// SelectedAgent returns the currently selected agent.
func (g *AgentGrid) SelectedAgent() *models.Agent {
	if g.selected >= 0 && g.selected < len(g.agents) {
		return g.agents[g.selected]
	}
	return nil
}

// SetAgents updates the list of agents.
func (g *AgentGrid) SetAgents(agents []*models.Agent) {
	g.agents = agents
	// Clamp selection
	if g.selected >= len(agents) {
		g.selected = len(agents) - 1
	}
	if g.selected < 0 {
		g.selected = 0
	}
}

// UpdateAgent adds or updates an agent.
func (g *AgentGrid) UpdateAgent(agent *models.Agent) {
	for i, existing := range g.agents {
		if existing.ID == agent.ID {
			g.agents[i] = agent
			return
		}
	}
	g.agents = append(g.agents, agent)
}

// AgentActionMsg is sent when an agent action is requested.
type AgentActionMsg struct {
	AgentID string
	Action  string
}

// togglePause sends a pause/resume command for an agent.
func (g *AgentGrid) togglePause(agent *models.Agent) tea.Cmd {
	action := "pause"
	if agent.Status == models.AgentStatusPaused {
		action = "resume"
	}
	return func() tea.Msg {
		return AgentActionMsg{AgentID: agent.ID, Action: action}
	}
}

// killAgent sends a kill command for an agent.
func (g *AgentGrid) killAgent(agent *models.Agent) tea.Cmd {
	return func() tea.Msg {
		return AgentActionMsg{AgentID: agent.ID, Action: "kill"}
	}
}

// AgentFocusMsg is sent when an agent is selected for focus.
type AgentFocusMsg struct {
	Agent *models.Agent
}

// focusAgent sends a focus command for an agent.
func (g *AgentGrid) focusAgent(agent *models.Agent) tea.Cmd {
	return func() tea.Msg {
		return AgentFocusMsg{Agent: agent}
	}
}

// truncate shortens a string to fit in a column.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
