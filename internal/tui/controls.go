package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// PauseAgentMsg requests pausing or resuming an agent.
type PauseAgentMsg struct {
	AgentID string
	Pause   bool // true = pause, false = resume
}

// PauseOrchestratorMsg requests pausing or resuming the orchestrator.
// When paused, no new agents will be spawned; existing agents continue running.
type PauseOrchestratorMsg struct {
	Pause bool // true = pause, false = resume
}

// KillAgentMsg requests terminating an agent.
type KillAgentMsg struct {
	AgentID string
}

// ApproveQuestionMsg provides an answer to an agent's question.
type ApproveQuestionMsg struct {
	AgentID string
	Answer  string
}

// TriggerMergeMsg requests a manual merge for an agent.
type TriggerMergeMsg struct {
	AgentID string
}

// ShowHelpMsg toggles the help overlay.
type ShowHelpMsg struct {
	Show bool
}

// AppState provides an interface to the application state needed by ControlHandler.
type AppState interface {
	// SelectedAgentID returns the ID of the currently selected agent, or empty string if none.
	SelectedAgentID() string
	// IsAgentPaused returns whether the specified agent is paused.
	IsAgentPaused(agentID string) bool
	// IsAgentWaiting returns whether the specified agent is waiting for approval.
	IsAgentWaiting(agentID string) bool
	// IsHelpVisible returns whether the help overlay is currently shown.
	IsHelpVisible() bool
	// IsOrchestratorPaused returns whether the orchestrator is currently paused.
	IsOrchestratorPaused() bool
}

// ControlHandler handles keyboard input for agent control operations.
type ControlHandler struct {
	app AppState
}

// NewControlHandler creates a new ControlHandler with the given app state.
func NewControlHandler(app AppState) *ControlHandler {
	return &ControlHandler{app: app}
}

// HandleKey processes a key press and returns appropriate commands.
// Returns nil if the key is not handled by this handler.
func (h *ControlHandler) HandleKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case " ":
		// Spacebar: pause/resume selected agent
		if agentID := h.app.SelectedAgentID(); agentID != "" {
			return h.PauseAgent(agentID)
		}

	case "p":
		// p: pause/resume orchestrator (global pause - stops spawning new agents)
		return h.PauseOrchestrator()

	case "k":
		// k: kill selected agent
		if agentID := h.app.SelectedAgentID(); agentID != "" {
			return h.KillAgent(agentID)
		}

	case "m":
		// m: trigger merge for selected agent
		if agentID := h.app.SelectedAgentID(); agentID != "" {
			return h.TriggerMerge(agentID)
		}

	case "?":
		// ?: toggle help overlay
		return h.toggleHelp()

	case "q":
		// q: quit
		return tea.Quit

	case "ctrl+c":
		// Ctrl+C: quit
		return tea.Quit
	}

	return nil
}

// PauseAgent returns a command to pause or resume the specified agent.
// If the agent is currently paused, it will be resumed; otherwise it will be paused.
func (h *ControlHandler) PauseAgent(agentID string) tea.Cmd {
	isPaused := h.app.IsAgentPaused(agentID)
	return func() tea.Msg {
		return PauseAgentMsg{
			AgentID: agentID,
			Pause:   !isPaused, // toggle: if paused, resume; if running, pause
		}
	}
}

// PauseOrchestrator returns a command to pause or resume the orchestrator.
// When paused, no new agents will be spawned; existing agents continue running.
func (h *ControlHandler) PauseOrchestrator() tea.Cmd {
	isPaused := h.app.IsOrchestratorPaused()
	return func() tea.Msg {
		return PauseOrchestratorMsg{
			Pause: !isPaused, // toggle: if paused, resume; if running, pause
		}
	}
}

// KillAgent returns a command to terminate the specified agent.
func (h *ControlHandler) KillAgent(agentID string) tea.Cmd {
	return func() tea.Msg {
		return KillAgentMsg{AgentID: agentID}
	}
}

// ApproveQuestion returns a command to provide an answer to an agent's question.
// This is typically called when an agent is in the waiting_approval state.
func (h *ControlHandler) ApproveQuestion(agentID string, answer string) tea.Cmd {
	return func() tea.Msg {
		return ApproveQuestionMsg{
			AgentID: agentID,
			Answer:  answer,
		}
	}
}

// TriggerMerge returns a command to manually trigger a merge for the specified agent.
func (h *ControlHandler) TriggerMerge(agentID string) tea.Cmd {
	return func() tea.Msg {
		return TriggerMergeMsg{AgentID: agentID}
	}
}

// toggleHelp returns a command to toggle the help overlay visibility.
func (h *ControlHandler) toggleHelp() tea.Cmd {
	showHelp := !h.app.IsHelpVisible()
	return func() tea.Msg {
		return ShowHelpMsg{Show: showHelp}
	}
}

// HelpText returns the text to display in the help overlay.
func HelpText() string {
	return `Keyboard Controls:

Navigation:
  j/k, up/down    Navigate agent list
  1/2/3, Tab      Switch tabs

Agent Control:
  Space           Pause/resume selected agent
  p               Pause/resume orchestrator (stops spawning new agents)
  k               Kill selected agent
  m               Trigger manual merge

Other:
  ?               Toggle this help
  q, Ctrl+C       Quit

Press ? to close this help.`
}
