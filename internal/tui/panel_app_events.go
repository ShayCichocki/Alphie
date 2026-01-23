package tui

import (
	"fmt"
	"log"
	"time"

	"github.com/ShayCichocki/alphie/pkg/models"
)

// handleOrchestratorEvent processes orchestrator events.
func (a *PanelApp) handleOrchestratorEvent(msg OrchestratorEventMsg) {
	log.Printf("[TUI] Received event: type=%s, taskID=%s, agentID=%s", msg.Type, msg.TaskID, msg.AgentID)

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
	case "task_entered":
		a.handleTaskEntered(msg)
	case "epic_created":
		a.handleEpicCreated(msg)
	case "task_queued":
		a.handleTaskQueued(msg)
	case "task_started":
		a.handleTaskStarted(msg)
	case "task_completed":
		a.handleTaskCompleted(msg)
	case "task_failed":
		a.handleTaskFailed(msg)
	case "agent_progress":
		a.handleAgentProgress(msg)
	case "merge_started", "merge_completed":
		// Log only, no state changes needed
	case "session_done":
		a.handleSessionDone(msg)
	}
}

func (a *PanelApp) handleTaskEntered(msg OrchestratorEventMsg) {
	// Immediate feedback when user submits a task (before orchestrator processes it)
	if msg.TaskID != "" {
		task := a.findOrCreateTask(msg.TaskID)
		task.Title = msg.TaskTitle
		task.Status = models.TaskStatusPending
		a.tasksPanel.SetTasks(a.tasks)
		a.updateFooterCounts()
	}
}

func (a *PanelApp) handleEpicCreated(msg OrchestratorEventMsg) {
	// Create a parent task for the epic so subtasks can be grouped under it
	// Also remove any duplicate task_entered task with the original task ID
	if msg.TaskID != "" {
		// Remove the original task_entered task by its ID
		// This prevents duplicate entries when orchestrator creates an epic
		if msg.OriginalTaskID != "" {
			a.removeTaskByID(msg.OriginalTaskID)
		}

		task := a.findOrCreateTask(msg.TaskID)
		task.Title = msg.TaskTitle
		task.Status = models.TaskStatusInProgress
		a.tasksPanel.SetTasks(a.tasks)
		a.updateFooterCounts()
	}
}

func (a *PanelApp) handleTaskQueued(msg OrchestratorEventMsg) {
	if msg.TaskID != "" {
		task := a.findOrCreateTask(msg.TaskID)
		if msg.TaskTitle != "" {
			task.Title = msg.TaskTitle
		}
		if msg.ParentID != "" {
			task.ParentID = msg.ParentID
		}
		a.tasksPanel.SetTasks(a.tasks)
		a.updateFooterCounts()
	}
}

func (a *PanelApp) handleTaskStarted(msg OrchestratorEventMsg) {
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
		if msg.ParentID != "" {
			task.ParentID = msg.ParentID
		}
		a.tasksPanel.SetTasks(a.tasks)
	}
	a.updateFooterCounts()
}

func (a *PanelApp) handleTaskCompleted(msg OrchestratorEventMsg) {
	log.Printf("[TUI] handleTaskCompleted: taskID=%s, agentID=%s", msg.TaskID, msg.AgentID)

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
		log.Printf("[TUI] Setting task %s status to Done (was: %s)", task.ID, task.Status)
		task.Status = models.TaskStatusDone
		if msg.ParentID != "" {
			task.ParentID = msg.ParentID
		}
		a.tasksPanel.SetTasks(a.tasks)
		log.Printf("[TUI] Task panel updated for task %s", msg.TaskID)
		// Check if all siblings under the same parent are done
		a.checkParentCompletion(task.ParentID)
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
}

func (a *PanelApp) handleTaskFailed(msg OrchestratorEventMsg) {
	log.Printf("[TUI] handleTaskFailed: taskID=%s, agentID=%s, error=%s", msg.TaskID, msg.AgentID, msg.Error)

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
		log.Printf("[TUI] Setting task %s status to Failed (was: %s)", task.ID, task.Status)
		task.Status = models.TaskStatusFailed
		task.Error = msg.Error // Store the error message
		a.tasksPanel.SetTasks(a.tasks)
		log.Printf("[TUI] Task panel updated for task %s", msg.TaskID)
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
}

func (a *PanelApp) handleAgentProgress(msg OrchestratorEventMsg) {
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
}

func (a *PanelApp) handleSessionDone(msg OrchestratorEventMsg) {
	a.sessionDone = true
	a.sessionSuccess = msg.Error == ""
	a.sessionMessage = msg.Message
	a.footer.SetSessionDone(true, a.sessionSuccess, a.sessionMessage)
}
