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
	// Escalation fields
	IsEscalating       bool
	EscalationTaskID   string
	EscalationTask     string
	EscalationReason   string
	EscalationAttempts int
	EscalationLogFile       string
	EscalationErrorDetails  string // Detailed error message
	EscalationValidationSum string // Validation summary if available
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

	// Escalation prompt (if escalating)
	if v.state.IsEscalating {
		b.WriteString("\n\n")
		b.WriteString(v.renderEscalationPrompt())
	}

	return b.String()
}

// renderEscalationPrompt renders the escalation prompt UI.
func (v *ImplementView) renderEscalationPrompt() string {
	var b strings.Builder

	// Header
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("214")).
		Render("⚠ TASK ESCALATION REQUIRED ⚠")
	b.WriteString(header)
	b.WriteString("\n\n")

	// Task info
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("Task: "))
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(v.state.EscalationTask))
	b.WriteString("\n")

	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("Attempts: "))
	b.WriteString(fmt.Sprintf("%d", v.state.EscalationAttempts))
	b.WriteString("\n")

	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("Reason: "))
	b.WriteString(v.state.EscalationReason)
	b.WriteString("\n\n")

	// Show detailed error if available
	if v.state.EscalationErrorDetails != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")).Render("Error Details:"))
		b.WriteString("\n")
		errorBox := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("196")).
			Padding(0, 1).
			Width(v.width - 10)
		b.WriteString(errorBox.Render(v.state.EscalationErrorDetails))
		b.WriteString("\n\n")
	}

	// Show validation summary if available
	if v.state.EscalationValidationSum != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).Render("Validation Summary:"))
		b.WriteString("\n")
		validationBox := lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("214")).
			Padding(0, 1).
			Width(v.width - 10)
		b.WriteString(validationBox.Render(v.state.EscalationValidationSum))
		b.WriteString("\n\n")
	}

	if v.state.EscalationLogFile != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Full Log: "))
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(v.state.EscalationLogFile))
		b.WriteString("\n")
	}

	// Options
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Choose an action:"))
	b.WriteString("\n\n")

	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("214")).
		Bold(true).
		Padding(0, 1)

	b.WriteString("  ")
	b.WriteString(keyStyle.Render("r"))
	b.WriteString("  Retry - Run the task again with same configuration\n")

	b.WriteString("  ")
	b.WriteString(keyStyle.Render("s"))
	b.WriteString("  Skip - Skip this task and its dependents, continue with remaining work\n")

	b.WriteString("  ")
	b.WriteString(keyStyle.Render("a"))
	b.WriteString("  Abort - Stop the entire execution\n")

	b.WriteString("  ")
	b.WriteString(keyStyle.Render("m"))
	b.WriteString("  Manual Fix - Pause execution, manually fix the code, then continue\n")

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

	// Escalation handling
	escalationHandler EscalationResponseHandler

	// Styles
	logStyle     lipgloss.Style
	logTimeStyle lipgloss.Style
	errorStyle   lipgloss.Style
	doneStyle    lipgloss.Style
	escalationStyle lipgloss.Style
	escalationKeyStyle lipgloss.Style
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

		escalationStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("214")).
			Padding(1, 2),

		escalationKeyStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("214")).
			Bold(true).
			Padding(0, 1),
	}
}

// SetEscalationHandler sets the callback for responding to escalations.
func (a *ImplementApp) SetEscalationHandler(handler EscalationResponseHandler) {
	a.escalationHandler = handler
}

// Init implements tea.Model.
func (a *ImplementApp) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (a *ImplementApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// If escalating, handle escalation keypresses
		currentState := a.view.state
		if currentState.IsEscalating {
			switch msg.String() {
			case "r":
				a.handleEscalationResponse("retry")
				return a, nil
			case "s":
				a.handleEscalationResponse("skip")
				return a, nil
			case "a":
				a.handleEscalationResponse("abort")
				return a, nil
			case "m":
				a.handleEscalationResponse("manual")
				return a, nil
			}
		}

		// Normal keypresses
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

	case EscalationMsg:
		// Update state to show escalation prompt
		currentState := a.view.state
		currentState.IsEscalating = true
		currentState.EscalationTaskID = msg.TaskID
		currentState.EscalationTask = msg.TaskTitle
		currentState.EscalationReason = msg.Reason
		currentState.EscalationAttempts = msg.Attempts
		currentState.EscalationLogFile = msg.LogFile
		currentState.EscalationErrorDetails = msg.ErrorDetails
		currentState.EscalationValidationSum = msg.ValidationSummary
		a.view.SetState(currentState)

		// Log escalation
		a.logs = append(a.logs, ImplementLogEntry{
			Timestamp: time.Now(),
			Phase:     "ESCALATION",
			Message:   fmt.Sprintf("Task '%s' needs escalation: %s", msg.TaskTitle, msg.Reason),
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

// handleEscalationResponse processes the user's escalation choice.
func (a *ImplementApp) handleEscalationResponse(action string) {
	if a.escalationHandler == nil {
		return
	}

	// Send response via handler
	if err := a.escalationHandler(action); err != nil {
		// Log error but continue
		a.logs = append(a.logs, ImplementLogEntry{
			Timestamp: time.Now(),
			Phase:     "ESCALATION",
			Message:   fmt.Sprintf("Error sending escalation response: %v", err),
		})
		return
	}

	// Clear escalation state
	currentState := a.view.state
	currentState.IsEscalating = false
	currentState.EscalationTaskID = ""
	currentState.EscalationTask = ""
	currentState.EscalationReason = ""
	currentState.EscalationAttempts = 0
	currentState.EscalationLogFile = ""
	a.view.SetState(currentState)

	// Log response
	actionName := map[string]string{
		"retry":  "Retry",
		"skip":   "Skip",
		"abort":  "Abort",
		"manual": "Manual Fix",
	}[action]
	a.logs = append(a.logs, ImplementLogEntry{
		Timestamp: time.Now(),
		Phase:     "ESCALATION",
		Message:   fmt.Sprintf("User chose: %s", actionName),
	})
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

// EscalationMsg is sent when a task needs user escalation.
type EscalationMsg struct {
	TaskID           string
	TaskTitle        string
	Reason           string
	Attempts         int
	LogFile          string
	ErrorDetails     string // Detailed error from the execution result
	ValidationSummary string // Summary from 4-layer validation if available
}

// EscalationResponseHandler is a callback for sending escalation responses.
// It takes the chosen action (retry/skip/abort/manual) and returns an error if the response failed.
type EscalationResponseHandler func(action string) error

// NewImplementProgram creates a new Bubbletea program for the implement TUI.
func NewImplementProgram() (*tea.Program, *ImplementApp) {
	app := NewImplementApp()
	p := tea.NewProgram(app, tea.WithAltScreen())
	return p, app
}

// Escalation UI implementation complete:
// - ImplementState has escalation fields (IsEscalating, EscalationTask, etc.)
// - EscalationMsg is handled in Update() method
// - Escalation prompt displayed in View() with r/s/a/m options
// - User keypresses captured and sent via EscalationResponseHandler callback
// - The orchestrator should set the handler using SetEscalationHandler()
//
// Integration example:
//   tuiApp.SetEscalationHandler(func(action string) error {
//       response := &orchestrator.EscalationResponse{
//           Action: orchestrator.EscalationAction(action),
//           Timestamp: time.Now(),
//       }
//       return orchestrator.escalationHandler.RespondToEscalation(response)
//   })
