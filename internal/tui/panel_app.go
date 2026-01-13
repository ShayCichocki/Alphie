package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/shayc/alphie/pkg/models"
)

// Panel indices.
const (
	PanelTasks  = 0
	PanelAgents = 1
	PanelLogs   = 2
)

// View tab indices (for 2-tab layout: Main vs Logs).
const (
	ViewTabMain = 0 // Tasks + Agents combined
	ViewTabLogs = 1 // Full-screen logs
)

// tabBarHeight is the height of the tab indicator bar.
const tabBarHeight = 1

// PanelApp is the main bubbletea model for the panel-based TUI.
type PanelApp struct {
	// Panels
	header      *Header
	tasksPanel  *TasksPanel
	agentsPanel *AgentsPanel
	logsPanel   *LogsPanel
	footer      *Footer

	// Layout
	layout *LayoutManager

	// State
	activeTab      int // 0 = main (tasks+agents), 1 = logs
	focusedPanel   int
	width          int
	height         int
	quitting       bool
	sessionDone    bool
	sessionSuccess bool
	sessionMessage string

	// Data
	agents []*models.Agent
	tasks  []*models.Task

	// showHeader controls whether the header is displayed.
	showHeader bool
}

// NewPanelApp creates a new PanelApp instance.
func NewPanelApp() *PanelApp {
	return &PanelApp{
		header:       NewHeader(),
		tasksPanel:   NewTasksPanel(),
		agentsPanel:  NewAgentsPanel(),
		logsPanel:    NewLogsPanel(),
		footer:       NewFooter(),
		layout:       NewLayoutManager(80, 24),
		focusedPanel: PanelAgents, // Start with agents panel focused
		agents:       make([]*models.Agent, 0),
		tasks:        make([]*models.Task, 0),
		showHeader:   true,
	}
}

// SetShowHeader controls whether the header is displayed.
func (a *PanelApp) SetShowHeader(show bool) {
	a.showHeader = show
	if show {
		a.layout.SetHeaderHeight(a.header.Height())
	} else {
		a.layout.SetHeaderHeight(0)
	}
}

// Init implements tea.Model.
func (a *PanelApp) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (a *PanelApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			a.quitting = true
			return a, tea.Quit

		case "1":
			// Switch to main tab
			if a.activeTab != ViewTabMain {
				a.activeTab = ViewTabMain
				a.focusedPanel = PanelAgents
				a.updatePanelFocus()
				a.updatePanelSizes()
				a.footer.SetActiveTab(ViewTabMain)
			}
		case "2":
			// Switch to logs tab
			if a.activeTab != ViewTabLogs {
				a.activeTab = ViewTabLogs
				a.focusedPanel = PanelLogs
				a.updatePanelFocus()
				a.updatePanelSizes()
				a.footer.SetActiveTab(ViewTabLogs)
			}

		case "left", "h":
			if a.activeTab == ViewTabMain && !a.panelHandlesKey(msg.String()) {
				// On main tab, cycle between Tasks and Agents
				if a.focusedPanel == PanelAgents {
					a.focusedPanel = PanelTasks
				}
				a.updatePanelFocus()
			}
		case "right", "l":
			if a.activeTab == ViewTabMain && !a.panelHandlesKey(msg.String()) {
				// On main tab, cycle between Tasks and Agents
				if a.focusedPanel == PanelTasks {
					a.focusedPanel = PanelAgents
				}
				a.updatePanelFocus()
			}
		case "tab":
			if a.activeTab == ViewTabMain {
				// Cycle between Tasks and Agents on main tab
				if a.focusedPanel == PanelTasks {
					a.focusedPanel = PanelAgents
				} else {
					a.focusedPanel = PanelTasks
				}
				a.updatePanelFocus()
			}
			// On logs tab, tab key does nothing (logs panel handles all input)
		case "shift+tab":
			if a.activeTab == ViewTabMain {
				// Cycle between Tasks and Agents on main tab
				if a.focusedPanel == PanelAgents {
					a.focusedPanel = PanelTasks
				} else {
					a.focusedPanel = PanelAgents
				}
				a.updatePanelFocus()
			}
		}

		// Forward to focused panel based on active tab
		if a.activeTab == ViewTabLogs {
			var cmd tea.Cmd
			a.logsPanel, cmd = a.logsPanel.Update(msg)
			cmds = append(cmds, cmd)
		} else {
			switch a.focusedPanel {
			case PanelTasks:
				var cmd tea.Cmd
				a.tasksPanel, cmd = a.tasksPanel.Update(msg)
				cmds = append(cmds, cmd)
			case PanelAgents:
				var cmd tea.Cmd
				a.agentsPanel, cmd = a.agentsPanel.Update(msg)
				cmds = append(cmds, cmd)
			}
		}

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.layout.SetSize(msg.Width, msg.Height)
		a.updatePanelSizes()

	case AgentUpdateMsg:
		a.updateAgent(msg.Agent)

	case TaskUpdateMsg:
		a.updateTask(msg.Task)

	case OrchestratorEventMsg:
		a.handleOrchestratorEvent(msg)

	case SessionDoneMsg:
		a.sessionDone = true
		a.sessionSuccess = msg.Success
		a.sessionMessage = msg.Message
		a.footer.SetSessionDone(true, msg.Success, msg.Message)

	case DebugLogMsg:
		a.logsPanel.AddLog(PanelLogEntry{
			Timestamp: time.Now(),
			Level:     LogLevelDebug,
			Message:   msg.Message,
		})
	}

	return a, tea.Batch(cmds...)
}

// panelHandlesKey returns true if the focused panel uses left/right for its own navigation.
func (a *PanelApp) panelHandlesKey(key string) bool {
	// On logs tab, logs panel handles all navigation
	if a.activeTab == ViewTabLogs {
		return true
	}
	// On main tab, agents panel uses left/right for card navigation
	if a.focusedPanel == PanelAgents && (key == "left" || key == "right" || key == "h" || key == "l") {
		return true
	}
	return false
}

// updatePanelFocus updates focus state on all panels.
func (a *PanelApp) updatePanelFocus() {
	a.tasksPanel.SetFocused(a.focusedPanel == PanelTasks)
	a.agentsPanel.SetFocused(a.focusedPanel == PanelAgents)
	a.logsPanel.SetFocused(a.focusedPanel == PanelLogs)
	a.footer.SetFocusedPanel(a.focusedPanel)
}

// updatePanelSizes updates panel dimensions based on layout and active tab.
func (a *PanelApp) updatePanelSizes() {
	a.header.SetWidth(a.width)
	a.footer.SetWidth(a.width)

	// Calculate dimensions based on active tab
	if a.activeTab == ViewTabLogs {
		dims := a.layout.CalculateLogsTab(tabBarHeight)
		a.logsPanel.SetSize(dims.LogsWidth, dims.ContentHeight)
	} else {
		dims := a.layout.CalculateMainTab(tabBarHeight)
		a.tasksPanel.SetSize(dims.TasksWidth, dims.ContentHeight)
		a.agentsPanel.SetSize(dims.AgentsWidth, dims.ContentHeight)
	}
}

// View implements tea.Model.
func (a *PanelApp) View() string {
	if a.quitting {
		return "Goodbye!\n"
	}

	var content string

	if a.activeTab == ViewTabLogs {
		// Tab 2: Full-screen logs (panel handles its own sizing via SetSize)
		content = a.logsPanel.View()
	} else {
		// Tab 1: Tasks + Agents side-by-side
		dims := a.layout.CalculateMainTab(tabBarHeight)
		tasksView := lipgloss.NewStyle().
			Width(dims.TasksWidth).
			Height(dims.ContentHeight).
			Render(a.tasksPanel.View())
		agentsView := lipgloss.NewStyle().
			Width(dims.AgentsWidth).
			Height(dims.ContentHeight).
			Render(a.agentsPanel.View())
		content = lipgloss.JoinHorizontal(lipgloss.Top, tasksView, agentsView)
	}

	// Tab indicator bar
	tabIndicator := a.renderTabIndicator()
	footer := a.footer.View()

	// Combine all parts
	if a.showHeader {
		header := a.header.View()
		return header + "\n" + tabIndicator + content + "\n" + footer
	}
	return tabIndicator + content + "\n" + footer
}

// renderTabIndicator renders the tab bar showing active tab.
func (a *PanelApp) renderTabIndicator() string {
	activeStyle := lipgloss.NewStyle().Bold(true).Reverse(true)
	inactiveStyle := lipgloss.NewStyle().Faint(true)

	tab1 := " 1:Main "
	tab2 := " 2:Logs "

	if a.activeTab == ViewTabMain {
		tab1 = activeStyle.Render(tab1)
		tab2 = inactiveStyle.Render(tab2)
	} else {
		tab1 = inactiveStyle.Render(tab1)
		tab2 = activeStyle.Render(tab2)
	}

	return tab1 + tab2 + "\n"
}

// updateAgent adds or updates an agent.
func (a *PanelApp) updateAgent(agent *models.Agent) {
	for i, existing := range a.agents {
		if existing.ID == agent.ID {
			a.agents[i] = agent
			a.agentsPanel.SetAgents(a.agents)
			return
		}
	}
	a.agents = append(a.agents, agent)
	a.agentsPanel.SetAgents(a.agents)
}

// updateTask adds or updates a task.
func (a *PanelApp) updateTask(task *models.Task) {
	for i, existing := range a.tasks {
		if existing.ID == task.ID {
			a.tasks[i] = task
			a.tasksPanel.SetTasks(a.tasks)
			return
		}
	}
	a.tasks = append(a.tasks, task)
	a.tasksPanel.SetTasks(a.tasks)
}

// handleOrchestratorEvent processes orchestrator events.
func (a *PanelApp) handleOrchestratorEvent(msg OrchestratorEventMsg) {
	// Determine log level based on event type
	level := LogLevelInfo
	if msg.Error != "" {
		level = LogLevelError
	}

	// Handle progress events differently - aggregate instead of spam
	if msg.Type == "agent_progress" {
		a.logsPanel.UpdateProgress(msg.AgentID, PanelLogEntry{
			Timestamp: msg.Timestamp,
			Level:     level,
			AgentID:   msg.AgentID,
			TaskID:    msg.TaskID,
			Message:   msg.Message,
		})
		// Also update agent state below, but don't add to logs
	} else {
		// Regular events: add to logs
		a.logsPanel.AddLog(PanelLogEntry{
			Timestamp: msg.Timestamp,
			Level:     level,
			AgentID:   msg.AgentID,
			TaskID:    msg.TaskID,
			Message:   msg.Message,
		})
	}

	// Update agent/task state based on event type
	switch msg.Type {
	case "task_queued":
		if msg.TaskID != "" {
			task := a.findOrCreateTask(msg.TaskID)
			if msg.TaskTitle != "" {
				task.Title = msg.TaskTitle
			}
			a.tasksPanel.SetTasks(a.tasks)
		}

	case "task_started":
		// Create or update agent entry
		if msg.AgentID != "" {
			agent := a.findOrCreateAgent(msg.AgentID)
			agent.TaskID = msg.TaskID
			agent.TaskTitle = msg.TaskTitle
			agent.Status = models.AgentStatusRunning
			agent.StartedAt = msg.Timestamp
			a.agentsPanel.SetAgents(a.agents)
		}
		// Update task status
		if msg.TaskID != "" {
			task := a.findOrCreateTask(msg.TaskID)
			task.Status = models.TaskStatusInProgress
			task.AssignedTo = msg.AgentID
			if msg.TaskTitle != "" {
				task.Title = msg.TaskTitle
			}
			a.tasksPanel.SetTasks(a.tasks)
		}
		a.updateFooterCounts()

	case "task_completed":
		// Update agent status
		if msg.AgentID != "" {
			agent := a.findOrCreateAgent(msg.AgentID)
			agent.Status = models.AgentStatusDone
			agent.CompletedAt = time.Now()
			agent.CurrentAction = ""
			a.agentsPanel.SetAgents(a.agents)
			// Clear live progress for this agent
			a.logsPanel.ClearProgress(msg.AgentID)
		}
		// Update task status
		if msg.TaskID != "" {
			task := a.findOrCreateTask(msg.TaskID)
			task.Status = models.TaskStatusDone
			a.tasksPanel.SetTasks(a.tasks)
		}
		// Add log entry with log file path
		if msg.LogFile != "" {
			a.logsPanel.AddLog(PanelLogEntry{
				Timestamp: msg.Timestamp,
				Level:     LogLevelInfo,
				Message:   fmt.Sprintf("Log: %s", msg.LogFile),
			})
		}
		a.updateFooterCounts()

	case "task_failed":
		// Update agent status
		if msg.AgentID != "" {
			agent := a.findOrCreateAgent(msg.AgentID)
			agent.Status = models.AgentStatusFailed
			agent.Error = msg.Error // Store the error message
			agent.CompletedAt = time.Now()
			agent.CurrentAction = ""
			a.agentsPanel.SetAgents(a.agents)
			// Clear live progress for this agent
			a.logsPanel.ClearProgress(msg.AgentID)
		}
		// Update task status
		if msg.TaskID != "" {
			task := a.findOrCreateTask(msg.TaskID)
			task.Status = models.TaskStatusFailed
			task.Error = msg.Error // Store the error message
			a.tasksPanel.SetTasks(a.tasks)
		}
		// Add log entry with log file path
		if msg.LogFile != "" {
			a.logsPanel.AddLog(PanelLogEntry{
				Timestamp: msg.Timestamp,
				Level:     LogLevelError,
				Message:   fmt.Sprintf("Log: %s", msg.LogFile),
			})
		}
		a.updateFooterCounts()

	case "agent_progress":
		// Update agent progress (tokens, cost, current action)
		if msg.AgentID != "" {
			agent := a.findOrCreateAgent(msg.AgentID)
			agent.TokensUsed = msg.TokensUsed
			agent.Cost = msg.Cost
			if msg.CurrentAction != "" {
				agent.CurrentAction = msg.CurrentAction
			}
			a.agentsPanel.SetAgents(a.agents)
		}

	case "merge_started", "merge_completed":
		// Log only, no state changes needed

	case "session_done":
		a.sessionDone = true
		a.sessionSuccess = msg.Error == ""
		a.sessionMessage = msg.Message
		a.footer.SetSessionDone(true, a.sessionSuccess, a.sessionMessage)
	}
}

// findOrCreateAgent finds an agent by ID or creates a new one.
func (a *PanelApp) findOrCreateAgent(id string) *models.Agent {
	for _, agent := range a.agents {
		if agent.ID == id {
			return agent
		}
	}
	agent := &models.Agent{
		ID:        id,
		Status:    models.AgentStatusPending,
		StartedAt: time.Now(),
	}
	a.agents = append(a.agents, agent)
	return agent
}

// findOrCreateTask finds a task by ID or creates a new one.
func (a *PanelApp) findOrCreateTask(id string) *models.Task {
	for _, task := range a.tasks {
		if task.ID == id {
			return task
		}
	}
	task := &models.Task{
		ID:     id,
		Status: models.TaskStatusPending,
	}
	a.tasks = append(a.tasks, task)
	return task
}

// updateFooterCounts updates the footer with current task counts.
func (a *PanelApp) updateFooterCounts() {
	counts := TaskCounts{}
	for _, task := range a.tasks {
		switch task.Status {
		case models.TaskStatusDone:
			counts.Done++
		case models.TaskStatusFailed:
			counts.Failed++
		case models.TaskStatusInProgress:
			counts.Running++
		}
	}
	a.footer.SetTaskCounts(counts)
}

// FocusedPanel returns the index of the currently focused panel.
func (a *PanelApp) FocusedPanel() int {
	return a.focusedPanel
}

// SetFocusedPanel sets which panel is focused.
func (a *PanelApp) SetFocusedPanel(panel int) {
	a.focusedPanel = panel
	a.updatePanelFocus()
}

// ActiveTab returns the currently active tab index.
func (a *PanelApp) ActiveTab() int {
	return a.activeTab
}

// NewPanelProgram creates a new Bubbletea program for the panel-based TUI.
// The returned program can receive messages via Send().
func NewPanelProgram() (*tea.Program, *PanelApp) {
	app := NewPanelApp()
	p := tea.NewProgram(app, tea.WithAltScreen())
	return p, app
}
