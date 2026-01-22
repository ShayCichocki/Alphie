package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ShayCichocki/alphie/pkg/models"
)

// InteractiveApp is the main model for interactive mode.
// It wraps PanelApp and adds an input field for submitting tasks.
type InteractiveApp struct {
	panelApp   *PanelApp
	inputField *InputField
	width      int
	height     int
	quitting   bool

	// inputFocused tracks whether the input field has focus (vs panels)
	inputFocused bool

	// Callback for when a task is submitted
	onTaskSubmit func(task string, tier models.Tier)

	// Callback for when a task retry is requested
	onTaskRetry func(taskID, taskTitle string, tier models.Tier)
}

// NewInteractiveApp creates a new InteractiveApp.
func NewInteractiveApp() *InteractiveApp {
	return &InteractiveApp{
		panelApp:     NewPanelApp(),
		inputField:   NewInputField(),
		inputFocused: true, // Input starts with focus
	}
}

// SetTaskSubmitHandler sets the callback for task submissions.
func (a *InteractiveApp) SetTaskSubmitHandler(handler func(task string, tier models.Tier)) {
	a.onTaskSubmit = handler
}

// SetTaskRetryHandler sets the callback for task retry requests.
func (a *InteractiveApp) SetTaskRetryHandler(handler func(taskID, taskTitle string, tier models.Tier)) {
	a.onTaskRetry = handler
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

		case "1", "2":
			// Tab switching - always forward to panel app
			var cmd tea.Cmd
			_, cmd = a.panelApp.Update(msg)
			// If switching to logs tab, blur input
			if msg.String() == "2" {
				a.inputFocused = false
				a.inputField.Blur()
			}
			// If switching to main tab and no panel is focused, focus input
			if msg.String() == "1" && a.panelApp.ActiveTab() == ViewTabLogs {
				a.inputFocused = true
				return a, a.inputField.Focus()
			}
			return a, cmd

		case "tab":
			// On logs tab, tab does nothing (logs handles all navigation)
			if a.panelApp.ActiveTab() == ViewTabLogs {
				return a, nil
			}
			// Cycle focus on main tab: input -> Tasks -> Agents -> input
			if a.inputFocused {
				// Move focus from input to first panel (Tasks)
				a.inputFocused = false
				a.inputField.Blur()
				a.panelApp.SetFocusedPanel(PanelTasks)
				return a, nil
			} else if a.panelApp.FocusedPanel() == PanelAgents {
				// At Agents panel, cycle back to input
				a.inputFocused = true
				return a, a.inputField.Focus()
			} else {
				// Tasks -> Agents
				a.panelApp.SetFocusedPanel(PanelAgents)
				return a, nil
			}

		case "shift+tab":
			// On logs tab, shift+tab does nothing
			if a.panelApp.ActiveTab() == ViewTabLogs {
				return a, nil
			}
			// Reverse cycle on main tab: input -> Agents -> Tasks -> input
			if a.inputFocused {
				// Move focus from input to last panel on main tab (Agents)
				a.inputFocused = false
				a.inputField.Blur()
				a.panelApp.SetFocusedPanel(PanelAgents)
				return a, nil
			} else if a.panelApp.FocusedPanel() == PanelTasks {
				// At Tasks panel, cycle back to input
				a.inputFocused = true
				return a, a.inputField.Focus()
			} else {
				// Agents -> Tasks
				a.panelApp.SetFocusedPanel(PanelTasks)
				return a, nil
			}

		case "escape":
			// Return focus to input field
			if !a.inputFocused {
				a.inputFocused = true
				return a, a.inputField.Focus()
			}

		default:
			// Route other keys based on focus
			if a.inputFocused {
				// Input has focus - send to input field
				var inputCmd tea.Cmd
				a.inputField, inputCmd = a.inputField.Update(msg)
				return a, inputCmd
			} else {
				// Panel has focus - send to panel app
				var cmd tea.Cmd
				_, cmd = a.panelApp.Update(msg)
				return a, cmd
			}
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

	case TaskRetryMsg:
		if a.onTaskRetry != nil {
			a.onTaskRetry(msg.TaskID, msg.TaskTitle, msg.Tier)
		}
		return a, nil

	case AgentUpdateMsg, TaskUpdateMsg, OrchestratorEventMsg, SessionDoneMsg, DebugLogMsg:
		// Forward these to panel app
		var cmd tea.Cmd
		_, cmd = a.panelApp.Update(msg)
		cmds = append(cmds, cmd)
		return a, tea.Batch(cmds...)
	}

	return a, tea.Batch(cmds...)
}

// updateSizes updates the sizes of child components based on terminal size.
func (a *InteractiveApp) updateSizes() {
	var panelHeight int

	// Input field takes 3 lines (border + content) - only on main tab
	if a.panelApp.ActiveTab() == ViewTabLogs {
		panelHeight = a.height
	} else {
		inputHeight := 3
		panelHeight = a.height - inputHeight - 1 // -1 for spacing
	}

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

	// Hide input field on Logs tab
	if a.panelApp.ActiveTab() == ViewTabLogs {
		return panels
	}

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
