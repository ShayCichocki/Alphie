package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/ShayCichocki/alphie/pkg/models"
	"github.com/charmbracelet/lipgloss"
)

// AgentCardData contains the data needed to render an agent card.
type AgentCardData struct {
	// ID is the agent's unique identifier.
	ID string
	// Status is the agent's current status.
	Status models.AgentStatus
	// Error contains the error message if the agent failed.
	Error string
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
	// CurrentAction describes what the agent is doing right now.
	CurrentAction string
	// CompletedAt is when the agent finished (for auto-hide logic).
	CompletedAt time.Time
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
	contentWidth := c.width - 6 // Account for border and padding

	// Line 1: Agent ID (for log correlation) + Task ID short form
	agentShort := c.data.ID
	if len(agentShort) > 8 {
		agentShort = agentShort[:8] // Show first 8 chars
	}
	taskShort := c.data.TaskID
	if len(taskShort) > 8 {
		taskShort = taskShort[:8]
	}
	idLine := fmt.Sprintf("A:%s T:%s", agentShort, taskShort)
	b.WriteString(c.labelStyle.Render(idLine))
	b.WriteString("\n")

	// Line 2: Task title
	title := c.data.TaskTitle
	if title == "" {
		title = c.data.TaskID
	}
	if len(title) > contentWidth {
		title = title[:contentWidth-3] + "..."
	}
	b.WriteString(c.idStyle.Render(title))
	b.WriteString("\n")

	// Line 3: Status with action or final state
	b.WriteString(c.renderStatusLine(contentWidth))
	b.WriteString("\n")

	// Line 4: Tokens + Cost (combined)
	tokensStr := formatTokensCompact(c.data.TokensUsed)
	costStr := fmt.Sprintf("$%.2f", c.data.Cost)
	b.WriteString(c.valueStyle.Render(tokensStr + " tokens  " + costStr))
	b.WriteString("\n")

	// Line 5: Duration
	duration := time.Since(c.data.StartedAt)
	durationStr := formatDuration(duration)
	b.WriteString(c.labelStyle.Render(durationStr))

	// Apply border and size
	return c.borderStyle.
		Width(c.width - 4).
		Height(c.height - 2).
		Render(b.String())
}

// renderStatusLine renders the status line with icon and context-aware detail.
func (c *AgentCard) renderStatusLine(maxWidth int) string {
	var icon string
	var style lipgloss.Style
	var detail string

	switch c.data.Status {
	case models.AgentStatusRunning:
		icon = iconRunning
		style = c.statusRunning
		if c.data.CurrentAction != "" {
			detail = c.data.CurrentAction
		} else {
			detail = "Working..."
		}
	case models.AgentStatusDone:
		icon = iconDone
		style = c.statusDone
		detail = "Done"
	case models.AgentStatusFailed:
		icon = iconFailed
		style = c.statusFailed
		if c.data.Error != "" {
			detail = c.data.Error
		} else {
			detail = "Failed"
		}
	case models.AgentStatusPaused:
		icon = iconPaused
		style = c.statusPaused
		detail = "Paused"
	case models.AgentStatusPending:
		icon = iconPending
		style = c.statusPending
		detail = "Pending"
	case models.AgentStatusWaitingApproval:
		icon = iconQuestion
		style = c.statusPaused
		detail = "Awaiting approval"
	default:
		icon = iconPending
		style = c.statusPending
		detail = "Unknown"
	}

	// Truncate detail to fit width (accounting for icon + space)
	iconLen := 2 // icon + space
	if len(detail) > maxWidth-iconLen {
		detail = detail[:maxWidth-iconLen-3] + "..."
	}

	return style.Render(icon + " " + detail)
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
