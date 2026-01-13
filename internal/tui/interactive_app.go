package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/shayc/alphie/pkg/models"
)

// InteractiveApp is the main model for interactive mode.
// It wraps PanelApp and adds an input field for submitting tasks.
type InteractiveApp struct {
	panelApp   *PanelApp
	inputField *InputField
	width      int
	height     int
	quitting   bool

	// Callback for when a task is submitted
	onTaskSubmit func(task string, tier models.Tier)
}

// NewInteractiveApp creates a new InteractiveApp.
func NewInteractiveApp() *InteractiveApp {
	return &InteractiveApp{
		panelApp:   NewPanelApp(),
		inputField: NewInputField(),
	}
}

// SetTaskSubmitHandler sets the callback for task submissions.
func (a *InteractiveApp) SetTaskSubmitHandler(handler func(task string, tier models.Tier)) {
	a.onTaskSubmit = handler
}

// Init implements tea.Model.
func (a *InteractiveApp) Init() tea.Cmd {
	return a.inputField.Focus()
}

// Update implements tea.Model.
func (a *InteractiveApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			a.quitting = true
			return a, tea.Quit

		case "tab", "shift+tab":
			// Forward navigation keys to PanelApp
			var cmd tea.Cmd
			_, cmd = a.panelApp.Update(msg)
			return a, cmd
		}

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.updateSizes()

		// Forward to panel app
		var cmd tea.Cmd
		_, cmd = a.panelApp.Update(msg)
		cmds = append(cmds, cmd)

	case TaskSubmittedMsg:
		if a.onTaskSubmit != nil {
			a.onTaskSubmit(msg.Task, msg.Tier)
		}
		return a, nil

	case AgentUpdateMsg, TaskUpdateMsg, OrchestratorEventMsg, SessionDoneMsg, DebugLogMsg:
		// Forward these to panel app
		var cmd tea.Cmd
		_, cmd = a.panelApp.Update(msg)
		cmds = append(cmds, cmd)
		return a, tea.Batch(cmds...)
	}

	// Update input field (it handles all key events except ctrl+c)
	var inputCmd tea.Cmd
	a.inputField, inputCmd = a.inputField.Update(msg)
	cmds = append(cmds, inputCmd)

	return a, tea.Batch(cmds...)
}

// updateSizes updates the sizes of child components based on terminal size.
func (a *InteractiveApp) updateSizes() {
	// Input field takes 3 lines (border + content)
	inputHeight := 3
	panelHeight := a.height - inputHeight - 1 // -1 for spacing

	// Update panel app with adjusted height
	a.panelApp.width = a.width
	a.panelApp.height = panelHeight
	a.panelApp.layout.SetSize(a.width, panelHeight)
	a.panelApp.updatePanelSizes()

	// Update input field width
	a.inputField.SetWidth(a.width)
}

// View implements tea.Model.
func (a *InteractiveApp) View() string {
	if a.quitting {
		return "Goodbye!\n"
	}

	panels := a.panelApp.View()
	input := a.inputField.View()

	return lipgloss.JoinVertical(lipgloss.Left, panels, input)
}

// Send forwards a message to the app (for external event forwarding).
func (a *InteractiveApp) Send(msg tea.Msg) {
	// This is handled by the tea.Program, not directly
}

// GetPanelApp returns the underlying PanelApp for direct access.
func (a *InteractiveApp) GetPanelApp() *PanelApp {
	return a.panelApp
}

// NewInteractiveProgram creates a new Bubbletea program for interactive mode.
func NewInteractiveProgram() (*tea.Program, *InteractiveApp) {
	app := NewInteractiveApp()
	p := tea.NewProgram(app, tea.WithAltScreen())
	return p, app
}
