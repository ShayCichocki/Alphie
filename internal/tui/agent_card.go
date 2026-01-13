package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/shayc/alphie/pkg/models"
)

// AgentCardData contains the data needed to render an agent card.
type AgentCardData struct {
	// ID is the agent's unique identifier.
	ID string
	// Status is the agent's current status.
	Status models.AgentStatus
	// TaskID is the ID of the task the agent is working on.
	TaskID string
	// TaskTitle is the title of the current task.
	TaskTitle string
	// TokensUsed is the total tokens consumed by this agent.
	TokensUsed int64
	// Cost is the total cost in dollars.
	Cost float64
	// StartedAt is when the agent started.
	StartedAt time.Time
}

// AgentCard renders a single agent as a card.
type AgentCard struct {
	data   *AgentCardData
	width  int
	height int

	// Styles
	borderStyle   lipgloss.Style
	idStyle       lipgloss.Style
	statusRunning lipgloss.Style
	statusDone    lipgloss.Style
	statusFailed  lipgloss.Style
	statusPending lipgloss.Style
	statusPaused  lipgloss.Style
	labelStyle    lipgloss.Style
	valueStyle    lipgloss.Style
}

// NewAgentCard creates a new AgentCard instance.
func NewAgentCard() *AgentCard {
	return &AgentCard{
		width:  20,
		height: 8,

		borderStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1),

		idStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")),

		statusRunning: lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")), // Green

		statusDone: lipgloss.NewStyle().
			Foreground(lipgloss.Color("28")), // Dark green

		statusFailed: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")), // Red

		statusPending: lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")), // Gray

		statusPaused: lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")), // Orange

		labelStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")),

		valueStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")),
	}
}

// SetData updates the card data.
func (c *AgentCard) SetData(data *AgentCardData) {
	c.data = data
}

// SetSize updates the card dimensions.
func (c *AgentCard) SetSize(width, height int) {
	c.width = width
	c.height = height
}

// View renders the agent card.
func (c *AgentCard) View() string {
	if c.data == nil {
		return c.borderStyle.Width(c.width - 4).Height(c.height - 2).Render("No agent")
	}

	var b strings.Builder

	// Agent ID (truncated)
	agentID := c.data.ID
	if len(agentID) > 8 {
		agentID = agentID[:8]
	}
	b.WriteString(c.idStyle.Render(agentID))
	b.WriteString("\n")

	// Status with icon
	statusStr := c.renderStatus()
	b.WriteString(statusStr)
	b.WriteString("\n")

	// Task ID (truncated)
	taskID := c.data.TaskID
	if len(taskID) > c.width-6 {
		taskID = taskID[:c.width-9] + "..."
	}
	b.WriteString(c.labelStyle.Render("Task: "))
	b.WriteString(c.valueStyle.Render(taskID))
	b.WriteString("\n")

	// Tokens
	tokensStr := formatTokensCompact(c.data.TokensUsed)
	b.WriteString(c.labelStyle.Render("Tok: "))
	b.WriteString(c.valueStyle.Render(tokensStr))
	b.WriteString("\n")

	// Cost
	costStr := fmt.Sprintf("$%.4f", c.data.Cost)
	b.WriteString(c.labelStyle.Render("Cost: "))
	b.WriteString(c.valueStyle.Render(costStr))
	b.WriteString("\n")

	// Duration
	duration := time.Since(c.data.StartedAt)
	durationStr := formatDuration(duration)
	b.WriteString(c.labelStyle.Render("Time: "))
	b.WriteString(c.valueStyle.Render(durationStr))

	// Apply border and size
	return c.borderStyle.
		Width(c.width - 4).
		Height(c.height - 2).
		Render(b.String())
}

// renderStatus renders the status line with icon.
func (c *AgentCard) renderStatus() string {
	var icon string
	var style lipgloss.Style
	var statusText string

	switch c.data.Status {
	case models.AgentStatusRunning:
		icon = iconRunning
		style = c.statusRunning
		statusText = "Running"
	case models.AgentStatusDone:
		icon = iconDone
		style = c.statusDone
		statusText = "Done"
	case models.AgentStatusFailed:
		icon = iconFailed
		style = c.statusFailed
		statusText = "Failed"
	case models.AgentStatusPaused:
		icon = iconPaused
		style = c.statusPaused
		statusText = "Paused"
	case models.AgentStatusPending:
		icon = iconPending
		style = c.statusPending
		statusText = "Pending"
	case models.AgentStatusWaitingApproval:
		icon = iconQuestion
		style = c.statusPaused
		statusText = "Waiting"
	default:
		icon = iconPending
		style = c.statusPending
		statusText = "Unknown"
	}

	return style.Render(icon + " " + statusText)
}

// formatTokensCompact formats tokens in a compact way (e.g., 1.2k, 15k, 1.5M).
func formatTokensCompact(tokens int64) string {
	if tokens < 1000 {
		return fmt.Sprintf("%d", tokens)
	}
	if tokens < 1000000 {
		return fmt.Sprintf("%.1fk", float64(tokens)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(tokens)/1000000)
}

// Width returns the card width.
func (c *AgentCard) Width() int {
	return c.width
}

// Height returns the card height.
func (c *AgentCard) Height() int {
	return c.height
}
