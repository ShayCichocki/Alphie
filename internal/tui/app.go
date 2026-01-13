// Package tui provides the terminal user interface for Alphie.
package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/shayc/alphie/pkg/models"
)

// Tab constants for navigation.
const (
	TabAgents = iota
	TabTasks
	TabLogs
)

// AgentUpdateMsg is sent when agent state changes.
type AgentUpdateMsg struct {
	Agent *models.Agent
}

// TaskUpdateMsg is sent when task state changes.
type TaskUpdateMsg struct {
	Task *models.Task
}

// TokenUpdateMsg is sent when token usage is updated.
type TokenUpdateMsg struct {
	AgentID    string
	TokensUsed int64
	Cost       float64
}

// KeyMsg represents a key press event.
type KeyMsg struct {
	Key string
}

// OrchestratorEventMsg wraps an orchestrator event for the TUI.
type OrchestratorEventMsg struct {
	Type       string
	TaskID     string
	TaskTitle  string
	AgentID    string
	Message    string
	Error      string
	Timestamp  time.Time
	TokensUsed int64         // For progress events
	Cost       float64       // For progress events
	Duration   time.Duration // For progress events
}

// SessionDoneMsg signals that the orchestrator session has completed.
type SessionDoneMsg struct {
	Success bool
	Message string
}

// DebugLogMsg is sent to add a debug message to the logs.
type DebugLogMsg struct {
	Message string
}

// LogEntry represents a log message displayed in the logs tab.
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
}

// App is the main bubbletea model for the Alphie TUI.
type App struct {
	// currentTab is the currently selected tab.
	currentTab int
	// session is the active work session.
	session *models.Session
	// agents is the list of active agents.
	agents []*models.Agent
	// tasks is the list of tasks.
	tasks []*models.Task
	// logs is the list of log entries.
	logs []LogEntry
	// width is the terminal width.
	width int
	// height is the terminal height.
	height int
	// quitting indicates the app is shutting down.
	quitting bool
	// sessionDone indicates the orchestrator session has completed.
	sessionDone bool
	// sessionSuccess indicates if the session completed successfully.
	sessionSuccess bool
	// sessionMessage holds the final session message.
	sessionMessage string
}

// New creates a new App instance.
func New() *App {
	return &App{
		currentTab: TabAgents,
		agents:     make([]*models.Agent, 0),
		tasks:      make([]*models.Task, 0),
		logs:       make([]LogEntry, 0),
	}
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			a.quitting = true
			return a, tea.Quit
		case "tab":
			a.currentTab = (a.currentTab + 1) % 3
		case "1":
			a.currentTab = TabAgents
		case "2":
			a.currentTab = TabTasks
		case "3":
			a.currentTab = TabLogs
		}

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

	case AgentUpdateMsg:
		a.updateAgent(msg.Agent)

	case TaskUpdateMsg:
		a.updateTask(msg.Task)

	case TokenUpdateMsg:
		a.updateTokens(msg.AgentID, msg.TokensUsed, msg.Cost)

	case OrchestratorEventMsg:
		a.handleOrchestratorEvent(msg)

	case SessionDoneMsg:
		a.sessionDone = true
		a.sessionSuccess = msg.Success
		a.sessionMessage = msg.Message

	case DebugLogMsg:
		a.logs = append(a.logs, LogEntry{
			Timestamp: time.Now(),
			Level:     "DEBUG",
			Message:   msg.Message,
		})
	}

	return a, nil
}

// View implements tea.Model.
func (a *App) View() string {
	if a.quitting {
		return "Goodbye!\n"
	}

	var content string
	switch a.currentTab {
	case TabAgents:
		content = a.viewAgents()
	case TabTasks:
		content = a.viewTasks()
	case TabLogs:
		content = a.viewLogs()
	}

	return fmt.Sprintf("%s\n\n%s\n\n%s", a.viewHeader(), content, a.viewFooter())
}

// viewHeader renders the tab bar.
func (a *App) viewHeader() string {
	tabs := []string{"Agents", "Tasks", "Logs"}
	var header string
	for i, tab := range tabs {
		if i == a.currentTab {
			header += fmt.Sprintf("[%s] ", tab)
		} else {
			header += fmt.Sprintf(" %s  ", tab)
		}
	}
	return header
}

// viewAgents renders the agents tab.
func (a *App) viewAgents() string {
	if len(a.agents) == 0 {
		return "No active agents"
	}

	var view string
	for _, agent := range a.agents {
		view += fmt.Sprintf("  %s [%s] Task: %s Tokens: %d\n",
			agent.ID[:8], agent.Status, agent.TaskID, agent.TokensUsed)
	}
	return view
}

// viewTasks renders the tasks tab.
func (a *App) viewTasks() string {
	if len(a.tasks) == 0 {
		return "No tasks"
	}

	var view string
	for _, task := range a.tasks {
		view += fmt.Sprintf("  %s [%s] %s\n",
			task.ID, task.Status, task.Title)
	}
	return view
}

// viewLogs renders the logs tab.
func (a *App) viewLogs() string {
	if len(a.logs) == 0 {
		return "No log entries"
	}

	// Show the most recent logs (up to 20)
	start := 0
	if len(a.logs) > 20 {
		start = len(a.logs) - 20
	}

	var view string
	for _, entry := range a.logs[start:] {
		ts := entry.Timestamp.Format("15:04:05")
		view += fmt.Sprintf("  %s [%s] %s\n", ts, entry.Level, entry.Message)
	}
	return view
}

// viewFooter renders the footer with help text.
func (a *App) viewFooter() string {
	if a.sessionDone {
		if a.sessionSuccess {
			return fmt.Sprintf("✓ %s | Press q to exit", a.sessionMessage)
		}
		return fmt.Sprintf("✗ %s | Press q to exit", a.sessionMessage)
	}
	return "Press 1/2/3 or Tab to switch tabs | q to quit"
}

// updateAgent adds or updates an agent in the list.
func (a *App) updateAgent(agent *models.Agent) {
	for i, existing := range a.agents {
		if existing.ID == agent.ID {
			a.agents[i] = agent
			return
		}
	}
	a.agents = append(a.agents, agent)
}

// updateTask adds or updates a task in the list.
func (a *App) updateTask(task *models.Task) {
	for i, existing := range a.tasks {
		if existing.ID == task.ID {
			a.tasks[i] = task
			return
		}
	}
	a.tasks = append(a.tasks, task)
}

// updateTokens updates token usage for an agent.
func (a *App) updateTokens(agentID string, tokensUsed int64, cost float64) {
	for _, agent := range a.agents {
		if agent.ID == agentID {
			agent.TokensUsed = tokensUsed
			agent.Cost = cost
			return
		}
	}
}

// SetSession sets the active session.
func (a *App) SetSession(session *models.Session) {
	a.session = session
}

// GetSession returns the active session.
func (a *App) GetSession() *models.Session {
	return a.session
}

// Run starts the TUI application.
func Run() error {
	app := New()
	p := tea.NewProgram(app, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// NewProgram creates a new Bubbletea program that can be used to run the TUI.
// The returned program can receive messages via Send().
func NewProgram() (*tea.Program, *App) {
	app := New()
	p := tea.NewProgram(app, tea.WithAltScreen())
	return p, app
}

// handleOrchestratorEvent processes an orchestrator event and updates state.
func (a *App) handleOrchestratorEvent(msg OrchestratorEventMsg) {
	// Add to logs
	level := "INFO"
	if msg.Error != "" {
		level = "ERROR"
	}
	a.logs = append(a.logs, LogEntry{
		Timestamp: msg.Timestamp,
		Level:     level,
		Message:   msg.Message,
	})

	// Update agent/task state based on event type
	switch msg.Type {
	case "task_started":
		// Create or update agent entry
		if msg.AgentID != "" {
			agent := a.findOrCreateAgent(msg.AgentID)
			agent.TaskID = msg.TaskID
			agent.Status = models.AgentStatusRunning
		}
		// Update task status
		if msg.TaskID != "" {
			task := a.findOrCreateTask(msg.TaskID)
			task.Status = models.TaskStatusInProgress
			task.AssignedTo = msg.AgentID
		}

	case "task_completed":
		// Update agent status
		if msg.AgentID != "" {
			agent := a.findOrCreateAgent(msg.AgentID)
			agent.Status = models.AgentStatusDone
		}
		// Update task status
		if msg.TaskID != "" {
			task := a.findOrCreateTask(msg.TaskID)
			task.Status = models.TaskStatusDone
		}

	case "task_failed":
		// Update agent status
		if msg.AgentID != "" {
			agent := a.findOrCreateAgent(msg.AgentID)
			agent.Status = models.AgentStatusFailed
		}
		// Update task status
		if msg.TaskID != "" {
			task := a.findOrCreateTask(msg.TaskID)
			task.Status = models.TaskStatusFailed
		}

	case "merge_started", "merge_completed":
		// Log only, no state changes needed

	case "session_done":
		a.sessionDone = true
		a.sessionSuccess = msg.Error == ""
		a.sessionMessage = msg.Message
	}
}

// findOrCreateAgent finds an agent by ID or creates a new one.
func (a *App) findOrCreateAgent(id string) *models.Agent {
	for _, agent := range a.agents {
		if agent.ID == id {
			return agent
		}
	}
	agent := &models.Agent{
		ID:     id,
		Status: models.AgentStatusPending,
	}
	a.agents = append(a.agents, agent)
	return agent
}

// findOrCreateTask finds a task by ID or creates a new one.
func (a *App) findOrCreateTask(id string) *models.Task {
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
