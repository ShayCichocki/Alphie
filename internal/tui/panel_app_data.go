package tui

import (
	"time"

	"github.com/ShayCichocki/alphie/pkg/models"
)

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

// checkParentCompletion checks if all children of a parent task are done.
// If so, marks the parent task as done.
func (a *PanelApp) checkParentCompletion(parentID string) {
	if parentID == "" {
		return
	}

	// Find all children of this parent
	var children []*models.Task
	for _, task := range a.tasks {
		if task.ParentID == parentID {
			children = append(children, task)
		}
	}

	// If no children, nothing to do
	if len(children) == 0 {
		return
	}

	// Check if all children are done (or failed)
	allDone := true
	for _, child := range children {
		if child.Status != models.TaskStatusDone && child.Status != models.TaskStatusFailed {
			allDone = false
			break
		}
	}

	// If all children are done, mark parent as done
	if allDone {
		for _, task := range a.tasks {
			if task.ID == parentID {
				task.Status = models.TaskStatusDone
				a.tasksPanel.SetTasks(a.tasks)
				break
			}
		}
	}
}

// removeTaskByID removes a task by its ID.
// This is used to remove the original task_entered task when epic_created arrives.
func (a *PanelApp) removeTaskByID(id string) {
	if id == "" {
		return
	}

	filtered := make([]*models.Task, 0, len(a.tasks))
	for _, task := range a.tasks {
		if task.ID != id {
			filtered = append(filtered, task)
		}
	}
	a.tasks = filtered
}
