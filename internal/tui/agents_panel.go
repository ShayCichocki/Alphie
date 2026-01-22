package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ShayCichocki/alphie/pkg/models"
)

const (
	// agentAutoHideDelay is how long completed agents remain visible.
	agentAutoHideDelay = 30 * time.Second
)

// AgentsPanel displays a grid of agent cards.
type AgentsPanel struct {
	agents       []*AgentCardData
	width        int
	height       int
	focused      bool
	selected     int
	scrollOffset int

	// Card dimensions
	cardWidth  int
	cardHeight int

	// Styles
	titleStyle  lipgloss.Style
	borderStyle lipgloss.Style
	emptyStyle  lipgloss.Style
}

// NewAgentsPanel creates a new AgentsPanel instance.
func NewAgentsPanel() *AgentsPanel {
	return &AgentsPanel{
		agents:     make([]*AgentCardData, 0),
		cardWidth:  22,
		cardHeight: 9,

		titleStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Padding(0, 1),

		borderStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")),

		emptyStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true),
	}
}

// SetAgents updates the list of agents with auto-hide filtering.
func (p *AgentsPanel) SetAgents(agents []*models.Agent) {
	cutoff := time.Now().Add(-agentAutoHideDelay)
	p.agents = make([]*AgentCardData, 0, len(agents))
	for _, agent := range agents {
		// Auto-hide: filter out completed (done) agents after 30 seconds.
		// Keep failed agents visible so users can investigate.
		if agent.Status == models.AgentStatusDone && !agent.CompletedAt.IsZero() && agent.CompletedAt.Before(cutoff) {
			continue
		}
		p.agents = append(p.agents, &AgentCardData{
			ID:            agent.ID,
			Status:        agent.Status,
			Error:         agent.Error,
			TaskID:        agent.TaskID,
			TaskTitle:     agent.TaskTitle,
			TokensUsed:    agent.TokensUsed,
			Cost:          agent.Cost,
			StartedAt:     agent.StartedAt,
			CurrentAction: agent.CurrentAction,
			CompletedAt:   agent.CompletedAt,
		})
	}
	// Ensure selected index is valid
	if p.selected >= len(p.agents) {
		p.selected = len(p.agents) - 1
	}
	if p.selected < 0 {
		p.selected = 0
	}
}

// UpdateAgent updates a single agent's data.
func (p *AgentsPanel) UpdateAgent(agentID string, tokens int64, cost float64) {
	for _, agent := range p.agents {
		if agent.ID == agentID {
			agent.TokensUsed = tokens
			agent.Cost = cost
			return
		}
	}
}

// SetSize updates the panel dimensions.
func (p *AgentsPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// SetFocused sets whether this panel has keyboard focus.
func (p *AgentsPanel) SetFocused(focused bool) {
	p.focused = focused
}

// Update handles input messages.
func (p *AgentsPanel) Update(msg tea.Msg) (*AgentsPanel, tea.Cmd) {
	if !p.focused {
		return p, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		cols := p.calculateColumns()
		if cols < 1 {
			cols = 1
		}

		switch msg.String() {
		case "up", "k":
			if p.selected >= cols {
				p.selected -= cols
				p.ensureVisible()
			}
		case "down", "j":
			if p.selected+cols < len(p.agents) {
				p.selected += cols
				p.ensureVisible()
			}
		case "left", "h":
			if p.selected > 0 {
				p.selected--
				p.ensureVisible()
			}
		case "right", "l":
			if p.selected < len(p.agents)-1 {
				p.selected++
				p.ensureVisible()
			}
		}
	}

	return p, nil
}

// calculateColumns returns how many columns fit in the panel width.
func (p *AgentsPanel) calculateColumns() int {
	// Account for borders and padding
	availableWidth := p.width - 4
	if availableWidth < p.cardWidth {
		return 1
	}
	return availableWidth / p.cardWidth
}

// ensureVisible adjusts scroll offset to keep selected item visible.
func (p *AgentsPanel) ensureVisible() {
	cols := p.calculateColumns()
	if cols < 1 {
		cols = 1
	}

	// Calculate visible rows
	availableHeight := p.height - 4 // Account for title, borders
	visibleRows := availableHeight / p.cardHeight
	if visibleRows < 1 {
		visibleRows = 1
	}

	selectedRow := p.selected / cols
	if selectedRow < p.scrollOffset {
		p.scrollOffset = selectedRow
	} else if selectedRow >= p.scrollOffset+visibleRows {
		p.scrollOffset = selectedRow - visibleRows + 1
	}
}

// View renders the agents panel.
func (p *AgentsPanel) View() string {
	var b strings.Builder

	// Title
	title := "Agents"
	if p.focused {
		title = "[Agents]"
	}
	b.WriteString(p.titleStyle.Render(title))
	b.WriteString("\n")

	if len(p.agents) == 0 {
		b.WriteString(p.emptyStyle.Render("  No active agents"))
	} else {
		// Calculate grid layout
		cols := p.calculateColumns()
		if cols < 1 {
			cols = 1
		}

		availableHeight := p.height - 4
		visibleRows := availableHeight / p.cardHeight
		if visibleRows < 1 {
			visibleRows = 1
		}

		// Render visible cards
		startRow := p.scrollOffset
		endRow := startRow + visibleRows

		for row := startRow; row < endRow; row++ {
			var rowCards []string
			for col := 0; col < cols; col++ {
				idx := row*cols + col
				if idx >= len(p.agents) {
					break
				}

				card := NewAgentCard()
				card.SetSize(p.cardWidth, p.cardHeight)
				card.SetData(p.agents[idx])

				// Highlight selected card
				cardView := card.View()
				if idx == p.selected && p.focused {
					cardView = lipgloss.NewStyle().
						Border(lipgloss.RoundedBorder()).
						BorderForeground(lipgloss.Color("63")). // Blue highlight
						Render(strings.TrimSpace(cardView))
				}

				rowCards = append(rowCards, cardView)
			}

			if len(rowCards) > 0 {
				b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, rowCards...))
				b.WriteString("\n")
			}
		}

		// Scroll indicators
		totalRows := (len(p.agents) + cols - 1) / cols
		if p.scrollOffset > 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  ... more above\n"))
		}
		if endRow < totalRows {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  ... more below"))
		}
	}

	// Apply border and size constraints
	content := b.String()
	borderColor := lipgloss.Color("240")
	if p.focused {
		borderColor = lipgloss.Color("63") // Blue when focused
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(p.width - 2).
		Height(p.height - 2).
		Render(content)
}

// SelectedAgent returns the currently selected agent data, or nil if none.
func (p *AgentsPanel) SelectedAgent() *AgentCardData {
	if len(p.agents) == 0 || p.selected >= len(p.agents) {
		return nil
	}
	return p.agents[p.selected]
}

// AgentCount returns the total number of agents.
func (p *AgentsPanel) AgentCount() int {
	return len(p.agents)
}

// RunningCount returns the number of running agents.
func (p *AgentsPanel) RunningCount() int {
	count := 0
	for _, agent := range p.agents {
		if agent.Status == models.AgentStatusRunning {
			count++
		}
	}
	return count
}
