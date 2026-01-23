// Package tui provides the terminal user interface for Alphie.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ImplementState tracks the current implementation progress.
type ImplementState struct {
	Iteration        int     // Current iteration number (no max - iterate until complete)
	FeaturesComplete int
	FeaturesTotal    int
	Cost             float64 // Total cost so far (no budget limit)
	CurrentPhase     string
	WorkersRunning   int
	WorkersBlocked   int
	StopConditions   []string
	BlockedQuestions []string
	// ActiveWorkers maps agent ID -> task info for debugging
	ActiveWorkers map[string]WorkerInfo
}

// WorkerInfo contains information about an active worker for display.
type WorkerInfo struct {
	AgentID   string
	TaskID    string
	TaskTitle string
	Status    string // "running", "blocked", etc.
}

// ImplementUpdateMsg is sent when implementation state changes.
type ImplementUpdateMsg struct {
	State ImplementState
}

// ImplementView displays the implementation progress tab.
type ImplementView struct {
	state  ImplementState
	width  int
	height int

	// Styles
	headerStyle   lipgloss.Style
	labelStyle    lipgloss.Style
	valueStyle    lipgloss.Style
	progressFull  lipgloss.Style
	progressEmpty lipgloss.Style
	phaseStyle    lipgloss.Style
	warningStyle  lipgloss.Style
	blockedStyle  lipgloss.Style
	runningStyle  lipgloss.Style
}

// NewImplementView creates a new ImplementView instance.
func NewImplementView() *ImplementView {
	return &ImplementView{
		state: ImplementState{},

		headerStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(lipgloss.Color("238")).
			MarginBottom(1),

		labelStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Width(16),

		valueStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Bold(true),

		progressFull: lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")),

		progressEmpty: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")),

		phaseStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true),

		warningStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")),

		blockedStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")),

		runningStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")),
	}
}

// Update handles input messages.
func (v *ImplementView) Update(msg tea.Msg) (*ImplementView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
	case ImplementUpdateMsg:
		v.state = msg.State
	}
	return v, nil
}

// View renders the implementation progress display.
func (v *ImplementView) View() string {
	var b strings.Builder

	// Header
	b.WriteString(v.headerStyle.Render("Implementation Progress"))
	b.WriteString("\n")

	// Iteration and Cost on same line (no limits - iterate until complete)
	iterStr := fmt.Sprintf("%d", v.state.Iteration)
	costStr := fmt.Sprintf("$%.2f", v.state.Cost)
	b.WriteString(v.labelStyle.Render("Iteration:"))
	b.WriteString(v.valueStyle.Render(iterStr))
	b.WriteString("  ")
	b.WriteString(v.labelStyle.Render("Cost:"))
	b.WriteString(v.valueStyle.Render(costStr))
	b.WriteString("\n")

	// Features completed/remaining
	featurePct := float64(0)
	if v.state.FeaturesTotal > 0 {
		featurePct = float64(v.state.FeaturesComplete) / float64(v.state.FeaturesTotal) * 100
	}
	featureStr := fmt.Sprintf("%d/%d complete (%.0f%%)",
		v.state.FeaturesComplete, v.state.FeaturesTotal, featurePct)
	b.WriteString(v.labelStyle.Render("Features:"))
	b.WriteString(v.valueStyle.Render(featureStr))
	b.WriteString("\n")

	// Progress bar
	b.WriteString(v.renderProgressBar(featurePct, 30))
	b.WriteString("\n\n")

	// Current Phase
	phase := v.state.CurrentPhase
	if phase == "" {
		phase = "none"
	}
	b.WriteString(v.labelStyle.Render("Current Phase:"))
	b.WriteString(v.phaseStyle.Render(phase))
	b.WriteString("\n")

	// Workers status
	workersStr := fmt.Sprintf("%s running, %s blocked",
		v.runningStyle.Render(fmt.Sprintf("%d", v.state.WorkersRunning)),
		v.blockedStyle.Render(fmt.Sprintf("%d", v.state.WorkersBlocked)))
	b.WriteString(v.labelStyle.Render("Workers:"))
	b.WriteString(workersStr)
	b.WriteString("\n")

	// Show active workers with their IDs for debugging
	if len(v.state.ActiveWorkers) > 0 {
		b.WriteString("\n")
		b.WriteString(v.labelStyle.Render("Active Workers:"))
		b.WriteString("\n")
		for _, worker := range v.state.ActiveWorkers {
			agentShort := worker.AgentID
			if len(agentShort) > 12 {
				agentShort = agentShort[:12]
			}
			taskShort := worker.TaskID
			if len(taskShort) > 8 {
				taskShort = taskShort[:8]
			}
			title := worker.TaskTitle
			if len(title) > 40 {
				title = title[:37] + "..."
			}

			statusStyle := v.runningStyle
			if worker.Status == "blocked" {
				statusStyle = v.blockedStyle
			}

			workerLine := fmt.Sprintf("  %s  A:%s T:%s  %s",
				statusStyle.Render(worker.Status),
				agentShort,
				taskShort,
				title)
			b.WriteString(workerLine)
			b.WriteString("\n")
		}
	}

	// Blocked questions (if any)
	if len(v.state.BlockedQuestions) > 0 {
		b.WriteString("\n")
		b.WriteString(v.labelStyle.Render("Blocked On:"))
		b.WriteString("\n")
		for _, q := range v.state.BlockedQuestions {
			b.WriteString("  ")
			b.WriteString(v.blockedStyle.Render("? "))
			b.WriteString(q)
			b.WriteString("\n")
		}
	}

	// Stop conditions (if any)
	if len(v.state.StopConditions) > 0 {
		b.WriteString("\n")
		b.WriteString(v.labelStyle.Render("Stop Conditions:"))
		b.WriteString("\n")
		for _, sc := range v.state.StopConditions {
			b.WriteString("  - ")
			b.WriteString(sc)
			b.WriteString("\n")
		}
	}

	return b.String()
}

// renderProgressBar renders a progress bar.
func (v *ImplementView) renderProgressBar(pct float64, width int) string {
	if pct > 100 {
		pct = 100
	}
	if pct < 0 {
		pct = 0
	}

	filled := int(pct / 100 * float64(width))
	empty := width - filled

	bar := v.progressFull.Render(strings.Repeat("█", filled)) +
		v.progressEmpty.Render(strings.Repeat("░", empty))

	return fmt.Sprintf("  %s %.0f%%", bar, pct)
}

// SetState updates the implementation state.
func (v *ImplementView) SetState(state ImplementState) {
	v.state = state
}

// SetSize sets the view dimensions.
func (v *ImplementView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// GetState returns the current implementation state.
func (v *ImplementView) GetState() ImplementState {
	return v.state
}

// ImplementLogEntry represents a log entry in the implementation log.
type ImplementLogEntry struct {
	Timestamp time.Time
	Phase     string
	Message   string
}

// ImplementApp is the main bubbletea model for the implement command TUI.
type ImplementApp struct {
	view     *ImplementView
	logs     []ImplementLogEntry
	width    int
	height   int
	quitting bool
	done     bool
	err      error

	// Styles
	logStyle     lipgloss.Style
	logTimeStyle lipgloss.Style
	errorStyle   lipgloss.Style
	doneStyle    lipgloss.Style
}

// NewImplementApp creates a new ImplementApp instance.
func NewImplementApp() *ImplementApp {
	return &ImplementApp{
		view: NewImplementView(),
		logs: make([]ImplementLogEntry, 0),

		logStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")),

		logTimeStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")),

		errorStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true),

		doneStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")).
			Bold(true),
	}
}

// Init implements tea.Model.
func (a *ImplementApp) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (a *ImplementApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			a.quitting = true
			return a, tea.Quit
		}

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.view.SetSize(msg.Width, msg.Height)

	case ImplementUpdateMsg:
		a.view.SetState(msg.State)

	case ImplementLogMsg:
		a.logs = append(a.logs, ImplementLogEntry{
			Timestamp: msg.Timestamp,
			Phase:     msg.Phase,
			Message:   msg.Message,
		})

	case ImplementDoneMsg:
		a.done = true
		if msg.Err != nil {
			a.err = msg.Err
		}
		// Don't quit immediately - let user see final state
	}

	return a, nil
}

// View implements tea.Model.
func (a *ImplementApp) View() string {
	if a.quitting {
		return "Implementation cancelled.\n"
	}

	var b strings.Builder

	// Header
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Render("=== Alphie Implement ===")
	b.WriteString(header)
	b.WriteString("\n\n")

	// Progress view
	b.WriteString(a.view.View())
	b.WriteString("\n")

	// Logs section
	b.WriteString(a.renderLogs())

	// Status footer
	b.WriteString("\n")
	if a.done {
		if a.err != nil {
			b.WriteString(a.errorStyle.Render(fmt.Sprintf("Error: %v", a.err)))
		} else {
			b.WriteString(a.doneStyle.Render("Implementation complete! Press q to exit."))
		}
	} else {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("Press q to cancel"))
	}
	b.WriteString("\n")

	return b.String()
}

// renderLogs renders the recent log entries.
func (a *ImplementApp) renderLogs() string {
	if len(a.logs) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("252")).
		Render("Activity Log"))
	b.WriteString("\n")

	// Show last 8 log entries
	start := 0
	if len(a.logs) > 8 {
		start = len(a.logs) - 8
	}

	for _, entry := range a.logs[start:] {
		ts := a.logTimeStyle.Render(entry.Timestamp.Format("15:04:05"))
		phase := lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Width(10).
			Render(entry.Phase)
		msg := a.logStyle.Render(entry.Message)
		b.WriteString(fmt.Sprintf("  %s %s %s\n", ts, phase, msg))
	}

	return b.String()
}

// ImplementLogMsg is sent when a log entry should be added.
type ImplementLogMsg struct {
	Timestamp time.Time
	Phase     string
	Message   string
}

// ImplementDoneMsg is sent when implementation completes.
type ImplementDoneMsg struct {
	Err error
}

// NewImplementProgram creates a new Bubbletea program for the implement TUI.
func NewImplementProgram() (*tea.Program, *ImplementApp) {
	app := NewImplementApp()
	p := tea.NewProgram(app, tea.WithAltScreen())
	return p, app
}

// TODO: Escalation UI Support
// The escalation handler is fully integrated into the orchestrator and emits
// EventTaskEscalation events. To complete the TUI integration:
// 1. Add escalation fields to ImplementState (isEscalating bool, escalationTask, etc.)
// 2. Handle EventTaskEscalation in the main TUI update loop
// 3. Display escalation prompt with options when escalating
// 4. Capture user keypress (r=retry, s=skip, a=abort, m=manual)
// 5. Send response back to orchestrator via RespondToEscalation()
// For now, escalation works but requires CLI/API interaction rather than TUI prompts.
