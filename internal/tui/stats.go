package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/ShayCichocki/alphie/pkg/models"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TokenTracker is an interface for tracking token usage.
// This allows decoupling from the concrete agent.TokenTracker.
type TokenTracker interface {
	GetUsage() TokenUsage
	GetHardUsage() TokenUsage
	GetSoftUsage() TokenUsage
	GetCost() float64
	GetConfidence() float64
}

// TokenUsage represents token usage counts.
type TokenUsage struct {
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
}

// StatsView displays session statistics including tier, duration, agent/task
// counts, token usage with progress bar, cost tracking, and confidence indicator.
type StatsView struct {
	session *models.Session
	agents  []*models.Agent
	tasks   []*models.Task
	width   int
	height  int

	// Token tracking
	inputTokens  int64
	outputTokens int64
	tokenBudget  int64
	hardTokens   int64 // API-reported tokens
	softTokens   int64 // Estimated tokens
	cost         float64
	costBudget   float64
	confidence   float64

	// Styles
	labelStyle     lipgloss.Style
	valueStyle     lipgloss.Style
	tierStyle      lipgloss.Style
	progressFull   lipgloss.Style
	progressEmpty  lipgloss.Style
	warningStyle   lipgloss.Style
	successStyle   lipgloss.Style
	headerStyle    lipgloss.Style
	confidenceLow  lipgloss.Style
	confidenceMed  lipgloss.Style
	confidenceHigh lipgloss.Style
}

// NewStatsView creates a new StatsView instance.
func NewStatsView() *StatsView {
	return &StatsView{
		agents:      make([]*models.Agent, 0),
		tasks:       make([]*models.Task, 0),
		tokenBudget: 100000,
		costBudget:  10.00,
		confidence:  1.0,

		labelStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Width(12),

		valueStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Bold(true),

		tierStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true),

		progressFull: lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")),

		progressEmpty: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")),

		warningStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")),

		successStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")),

		headerStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(lipgloss.Color("238")).
			MarginBottom(1),

		confidenceLow: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")),

		confidenceMed: lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")),

		confidenceHigh: lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")),
	}
}

// Update handles input messages.
func (s *StatsView) Update(msg tea.Msg) (*StatsView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
	}
	return s, nil
}

// View renders the stats display.
func (s *StatsView) View() string {
	var b strings.Builder

	// Header
	b.WriteString(s.headerStyle.Render("Session Statistics"))
	b.WriteString("\n")

	// TODO: tier removed - show default value
	// Tier
	tier := "Default"
	// if s.session != nil {
	// 	tier = string(s.session.Tier)
	// 	if tier == "" {
	// 		tier = "Unknown"
	// 	}
	// }
	b.WriteString(s.renderRow("Tier:", s.tierStyle.Render(tier)))
	b.WriteString("\n")

	// Duration
	duration := s.getDuration()
	b.WriteString(s.renderRow("Duration:", s.valueStyle.Render(duration)))
	b.WriteString("\n")

	// Agents
	running, total := s.countAgents()
	agentStr := fmt.Sprintf("%d running / %d total", running, total)
	b.WriteString(s.renderRow("Agents:", s.valueStyle.Render(agentStr)))
	b.WriteString("\n")

	// Tasks
	done, taskTotal := s.countTasks()
	taskStr := fmt.Sprintf("%d done / %d total", done, taskTotal)
	b.WriteString(s.renderRow("Tasks:", s.valueStyle.Render(taskStr)))
	b.WriteString("\n\n")

	// Token usage with progress bar
	tokens := s.inputTokens + s.outputTokens
	tokenPct := float64(0)
	if s.tokenBudget > 0 {
		tokenPct = float64(tokens) / float64(s.tokenBudget) * 100
	}
	tokenStr := fmt.Sprintf("%s / %s (%0.1f%%)",
		formatNumber(tokens),
		formatNumber(s.tokenBudget),
		tokenPct)
	b.WriteString(s.renderRow("Tokens:", s.valueStyle.Render(tokenStr)))
	b.WriteString("\n")

	// Token progress bar
	b.WriteString(s.renderProgressBar(tokenPct, 30))
	b.WriteString("\n\n")

	// Cost
	costPct := float64(0)
	if s.costBudget > 0 {
		costPct = s.cost / s.costBudget * 100
	}
	costStr := fmt.Sprintf("$%.2f / $%.2f budget (%0.1f%%)",
		s.cost,
		s.costBudget,
		costPct)
	costStyle := s.valueStyle
	if costPct > 90 {
		costStyle = s.warningStyle
	}
	b.WriteString(s.renderRow("Cost:", costStyle.Render(costStr)))
	b.WriteString("\n")

	// Cost progress bar
	b.WriteString(s.renderProgressBar(costPct, 30))
	b.WriteString("\n\n")

	// Confidence indicator (for soft budget estimates)
	confStr := s.formatConfidence()
	b.WriteString(s.renderRow("Confidence:", confStr))
	b.WriteString("\n")

	// Token breakdown
	b.WriteString("\n")
	b.WriteString(s.labelStyle.Render("Breakdown:"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Input:  %s\n", formatNumber(s.inputTokens)))
	b.WriteString(fmt.Sprintf("  Output: %s\n", formatNumber(s.outputTokens)))
	b.WriteString("\n")
	b.WriteString(s.labelStyle.Render("Budget:"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Hard:   %s (API-reported)\n", formatNumber(s.hardTokens)))
	b.WriteString(fmt.Sprintf("  Soft:   %s (estimated)\n", formatNumber(s.softTokens)))

	return b.String()
}

// renderRow renders a label-value pair.
func (s *StatsView) renderRow(label, value string) string {
	return s.labelStyle.Render(label) + " " + value
}

// renderProgressBar renders a progress bar.
func (s *StatsView) renderProgressBar(pct float64, width int) string {
	if pct > 100 {
		pct = 100
	}
	if pct < 0 {
		pct = 0
	}

	filled := int(pct / 100 * float64(width))
	empty := width - filled

	// Choose color based on percentage
	fullStyle := s.progressFull
	if pct > 90 {
		fullStyle = s.warningStyle
	}

	bar := fullStyle.Render(strings.Repeat("█", filled)) +
		s.progressEmpty.Render(strings.Repeat("░", empty))

	return fmt.Sprintf("  [%s]", bar)
}

// formatConfidence formats the confidence indicator.
func (s *StatsView) formatConfidence() string {
	pct := s.confidence * 100
	label := "Unknown"
	style := s.confidenceLow

	switch {
	case pct >= 90:
		label = "High"
		style = s.confidenceHigh
	case pct >= 70:
		label = "Medium"
		style = s.confidenceMed
	case pct >= 50:
		label = "Low"
		style = s.confidenceLow
	default:
		label = "Very Low"
		style = s.confidenceLow
	}

	return style.Render(fmt.Sprintf("%s (%.0f%% from API)", label, pct))
}

// getDuration returns the formatted session duration.
func (s *StatsView) getDuration() string {
	if s.session == nil {
		return "0s"
	}

	d := time.Since(s.session.StartedAt)
	return formatDuration(d)
}

// countAgents returns (running, total) agent counts.
func (s *StatsView) countAgents() (int, int) {
	running := 0
	for _, a := range s.agents {
		if a.Status == models.AgentStatusRunning {
			running++
		}
	}
	return running, len(s.agents)
}

// countTasks returns (done, total) task counts.
func (s *StatsView) countTasks() (int, int) {
	done := 0
	for _, t := range s.tasks {
		if t.Status == models.TaskStatusDone {
			done++
		}
	}
	return done, len(s.tasks)
}

// SetSession sets the active session.
func (s *StatsView) SetSession(session *models.Session) {
	s.session = session
	if session != nil {
		s.tokenBudget = session.TokenBudget
	}
}

// SetTokenUsage sets the current token usage and budget.
func (s *StatsView) SetTokenUsage(input, output, budget int64) {
	s.inputTokens = input
	s.outputTokens = output
	if budget > 0 {
		s.tokenBudget = budget
	}
}

// SetCost sets the current cost and budget.
func (s *StatsView) SetCost(cost, budget float64) {
	s.cost = cost
	if budget > 0 {
		s.costBudget = budget
	}
}

// SetConfidence sets the confidence indicator.
func (s *StatsView) SetConfidence(confidence float64) {
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	s.confidence = confidence
}

// SetAgents updates the list of agents.
func (s *StatsView) SetAgents(agents []*models.Agent) {
	s.agents = agents
}

// SetTasks updates the list of tasks.
func (s *StatsView) SetTasks(tasks []*models.Task) {
	s.tasks = tasks
}

// SetSize sets the view dimensions.
func (s *StatsView) SetSize(width, height int) {
	s.width = width
	s.height = height
}

// UpdateFromTracker updates token stats from a TokenTracker.
func (s *StatsView) UpdateFromTracker(tracker TokenTracker) {
	if tracker == nil {
		return
	}
	usage := tracker.GetUsage()
	s.inputTokens = usage.InputTokens
	s.outputTokens = usage.OutputTokens
	s.cost = tracker.GetCost()
	s.confidence = tracker.GetConfidence()

	// Get hard vs soft token breakdown
	hardUsage := tracker.GetHardUsage()
	softUsage := tracker.GetSoftUsage()
	s.hardTokens = hardUsage.TotalTokens
	s.softTokens = softUsage.TotalTokens
}

// formatNumber formats a number with comma separators.
func formatNumber(n int64) string {
	str := fmt.Sprintf("%d", n)
	if n < 0 {
		str = str[1:]
	}

	result := ""
	for i, c := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}

	if n < 0 {
		result = "-" + result
	}
	return result
}
