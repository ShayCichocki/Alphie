package orchestrator

import (
	"sync"

	"github.com/shayc/alphie/pkg/models"
)

// Scheduler coordinates the scheduling of ready tasks to available agent slots.
// It respects the tier's max_agents limit and uses collision detection to avoid
// concurrent modifications to the same files or directories.
type Scheduler struct {
	// graph is the dependency graph of tasks.
	graph *DependencyGraph
	// tier is the agent tier for this scheduler.
	tier models.Tier
	// running maps agent IDs to their agent instances.
	running map[string]*models.Agent
	// maxAgents is the maximum number of concurrent agents allowed.
	maxAgents int
	// collision is the collision checker for avoiding file conflicts.
	collision *CollisionChecker
	// mu protects all mutable fields.
	mu sync.RWMutex
}

// NewScheduler creates a new Scheduler with the given dependency graph, tier, and max agents limit.
func NewScheduler(graph *DependencyGraph, tier models.Tier, maxAgents int) *Scheduler {
	return &Scheduler{
		graph:     graph,
		tier:      tier,
		running:   make(map[string]*models.Agent),
		maxAgents: maxAgents,
	}
}

// SetCollisionChecker sets the collision checker for this scheduler.
// If not set, collision checking is disabled.
func (s *Scheduler) SetCollisionChecker(cc *CollisionChecker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.collision = cc
}

// Schedule returns a slice of tasks that are ready to be scheduled.
// It considers:
// - Tasks with no unmet dependencies (from the graph)
// - Available agent slots (maxAgents - running count)
// - Collision avoidance rules (if a collision checker is set)
func (s *Scheduler) Schedule() []*models.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Calculate available slots.
	availableSlots := s.maxAgents - len(s.running)
	if availableSlots <= 0 {
		return nil
	}

	// Get ready task IDs from the dependency graph.
	readyIDs := s.graph.GetReady()
	if len(readyIDs) == 0 {
		return nil
	}

	// Filter out tasks that are already being worked on.
	var candidates []*models.Task
	for _, id := range readyIDs {
		// Check if this task is already assigned to a running agent.
		alreadyRunning := false
		for _, agent := range s.running {
			if agent.TaskID == id {
				alreadyRunning = true
				break
			}
		}
		if alreadyRunning {
			continue
		}

		task := s.graph.GetTask(id)
		if task != nil {
			candidates = append(candidates, task)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Filter by collision avoidance if a checker is set.
	var schedulable []*models.Task
	if s.collision != nil {
		runningAgents := s.getRunningAgentsLocked()
		for _, task := range candidates {
			if s.collision.CanSchedule(task, runningAgents) {
				schedulable = append(schedulable, task)
			}
		}
	} else {
		schedulable = candidates
	}

	// Limit to available slots.
	if len(schedulable) > availableSlots {
		schedulable = schedulable[:availableSlots]
	}

	return schedulable
}

// OnAgentStart records that an agent has started working on a task.
func (s *Scheduler) OnAgentStart(agent *models.Agent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running[agent.ID] = agent
}

// OnAgentComplete handles the completion of an agent.
// It removes the agent from the running map and marks the task complete in the graph.
func (s *Scheduler) OnAgentComplete(agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	agent, ok := s.running[agentID]
	if !ok {
		return
	}

	// Mark the task complete in the dependency graph.
	s.graph.MarkComplete(agent.TaskID)

	// Remove from running agents.
	delete(s.running, agentID)
}

// GetRunningAgents returns a slice of all currently running agents.
func (s *Scheduler) GetRunningAgents() []*models.Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getRunningAgentsLocked()
}

// getRunningAgentsLocked returns running agents without acquiring the lock.
// Caller must hold s.mu.
func (s *Scheduler) getRunningAgentsLocked() []*models.Agent {
	agents := make([]*models.Agent, 0, len(s.running))
	for _, agent := range s.running {
		agents = append(agents, agent)
	}
	return agents
}

// GetRunningCount returns the number of currently running agents.
func (s *Scheduler) GetRunningCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.running)
}
