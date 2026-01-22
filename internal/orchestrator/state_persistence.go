// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"time"

	"github.com/ShayCichocki/alphie/internal/state"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// createSessionState creates a new session record in the state database.
func (o *Orchestrator) createSessionState(request string) error {
	if o.stateDB == nil {
		return nil // No-op if state DB not configured
	}

	session := &state.Session{
		ID:        o.config.SessionID,
		RootTask:  request,
		Tier:      string(o.config.Tier),
		StartedAt: time.Now(),
		Status:    state.SessionActive,
	}

	return o.stateDB.CreateSession(session)
}

// updateSessionStatus updates the session status in the state database.
func (o *Orchestrator) updateSessionStatus(status state.SessionStatus) {
	if o.stateDB == nil {
		return // No-op if state DB not configured
	}

	session, err := o.stateDB.GetSession(o.config.SessionID)
	if err != nil || session == nil {
		return
	}

	session.Status = status
	o.stateDB.UpdateSession(session)
}

// persistTasks creates task records in the state database.
func (o *Orchestrator) persistTasks(tasks []*models.Task) error {
	if o.stateDB == nil {
		return nil // No-op if state DB not configured
	}

	for _, t := range tasks {
		stateTask := &state.Task{
			ID:          t.ID,
			ParentID:    t.ParentID,
			Title:       t.Title,
			Description: t.Description,
			Status:      state.TaskStatus(t.Status),
			DependsOn:   t.DependsOn,
			Tier:        string(t.Tier),
			CreatedAt:   t.CreatedAt,
		}
		if err := o.stateDB.CreateTask(stateTask); err != nil {
			return err
		}
	}

	return nil
}

// updateTaskState updates a task's status in the state database.
func (o *Orchestrator) updateTaskState(task *models.Task) {
	if o.stateDB == nil {
		return // No-op if state DB not configured
	}

	stateTask := &state.Task{
		ID:          task.ID,
		ParentID:    task.ParentID,
		Title:       task.Title,
		Description: task.Description,
		Status:      state.TaskStatus(task.Status),
		DependsOn:   task.DependsOn,
		AssignedTo:  task.AssignedTo,
		Tier:        string(task.Tier),
		CreatedAt:   task.CreatedAt,
		CompletedAt: task.CompletedAt,
	}
	o.stateDB.UpdateTask(stateTask)
}

// createAgentState creates an agent record in the state database.
func (o *Orchestrator) createAgentState(a *models.Agent) {
	if o.stateDB == nil {
		return // No-op if state DB not configured
	}

	stateAgent := &state.Agent{
		ID:           a.ID,
		TaskID:       a.TaskID,
		Status:       state.AgentStatus(a.Status),
		WorktreePath: a.WorktreePath,
		PID:          a.PID,
		StartedAt:    &a.StartedAt,
	}
	o.stateDB.CreateAgent(stateAgent)
}

// updateAgentState updates an agent's status in the state database.
func (o *Orchestrator) updateAgentState(agentID string, status string) {
	if o.stateDB == nil {
		return // No-op if state DB not configured
	}

	agent, err := o.stateDB.GetAgent(agentID)
	if err != nil || agent == nil {
		return
	}

	agent.Status = state.AgentStatus(status)
	o.stateDB.UpdateAgent(agent)
}
