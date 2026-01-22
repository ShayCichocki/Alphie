package agent

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// Common errors for agent lifecycle management.
var (
	// ErrAgentNotFound indicates the requested agent does not exist.
	ErrAgentNotFound = errors.New("agent not found")
	// ErrInvalidTransition indicates an invalid state transition was attempted.
	ErrInvalidTransition = errors.New("invalid state transition")
	// ErrAgentAlreadyExists indicates an agent with that ID already exists.
	ErrAgentAlreadyExists = errors.New("agent already exists")
)

// LifecycleEventType represents the type of agent lifecycle event.
type LifecycleEventType string

const (
	// LifecycleEventCreated is emitted when an agent is created.
	LifecycleEventCreated LifecycleEventType = "created"
	// LifecycleEventStarted is emitted when an agent starts running.
	LifecycleEventStarted LifecycleEventType = "started"
	// LifecycleEventPaused is emitted when an agent is paused.
	LifecycleEventPaused LifecycleEventType = "paused"
	// LifecycleEventResumed is emitted when a paused agent resumes.
	LifecycleEventResumed LifecycleEventType = "resumed"
	// LifecycleEventWaitingApproval is emitted when an agent enters approval wait.
	LifecycleEventWaitingApproval LifecycleEventType = "waiting_approval"
	// LifecycleEventCompleted is emitted when an agent completes successfully.
	LifecycleEventCompleted LifecycleEventType = "completed"
	// LifecycleEventFailed is emitted when an agent fails.
	LifecycleEventFailed LifecycleEventType = "failed"
)

// LifecycleEvent represents an agent lifecycle event.
type LifecycleEvent struct {
	// Type is the kind of event that occurred.
	Type LifecycleEventType
	// AgentID is the ID of the agent that triggered the event.
	AgentID string
	// TaskID is the task the agent is working on.
	TaskID string
	// FromStatus is the previous status (empty for EventCreated).
	FromStatus models.AgentStatus
	// ToStatus is the new status.
	ToStatus models.AgentStatus
	// Timestamp is when the event occurred.
	Timestamp time.Time
	// Error contains the error message if the event is EventFailed.
	Error string
}

// LifecycleEventHandler is a function that handles agent lifecycle events.
type LifecycleEventHandler func(LifecycleEvent)

// validTransitions defines the allowed state transitions.
// Key is the current state, value is the set of valid target states.
var validTransitions = map[models.AgentStatus]map[models.AgentStatus]bool{
	models.AgentStatusPending: {
		models.AgentStatusRunning: true,
		models.AgentStatusFailed:  true,
	},
	models.AgentStatusRunning: {
		models.AgentStatusPaused:          true,
		models.AgentStatusWaitingApproval: true,
		models.AgentStatusDone:            true,
		models.AgentStatusFailed:          true,
	},
	models.AgentStatusPaused: {
		models.AgentStatusRunning: true,
		models.AgentStatusFailed:  true,
	},
	models.AgentStatusWaitingApproval: {
		models.AgentStatusRunning: true,
		models.AgentStatusFailed:  true,
	},
	// Terminal states: done and failed cannot transition to anything else
	models.AgentStatusDone:   {},
	models.AgentStatusFailed: {},
}

// CanTransition checks if a state transition is valid.
func CanTransition(from, to models.AgentStatus) bool {
	targets, ok := validTransitions[from]
	if !ok {
		return false
	}
	return targets[to]
}

// Manager handles agent lifecycle operations.
type Manager struct {
	mu       sync.RWMutex
	agents   map[string]*models.Agent
	handlers []LifecycleEventHandler
}

// NewManager creates a new agent lifecycle manager.
func NewManager() *Manager {
	return &Manager{
		agents:   make(map[string]*models.Agent),
		handlers: make([]LifecycleEventHandler, 0),
	}
}

// OnEvent registers an event handler that will be called for all lifecycle events.
func (m *Manager) OnEvent(handler LifecycleEventHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers = append(m.handlers, handler)
}

// emit sends an event to all registered handlers.
func (m *Manager) emit(event LifecycleEvent) {
	// Read lock is sufficient since we're only reading handlers slice
	m.mu.RLock()
	handlers := make([]LifecycleEventHandler, len(m.handlers))
	copy(handlers, m.handlers)
	m.mu.RUnlock()

	for _, h := range handlers {
		h(event)
	}
}

// Create creates a new agent in pending state with an auto-generated ID.
func (m *Manager) Create(taskID, worktreePath string) (*models.Agent, error) {
	return m.CreateWithID("", taskID, worktreePath)
}

// CreateWithID creates a new agent in pending state with the given ID.
// If agentID is empty, a new UUID is generated.
func (m *Manager) CreateWithID(agentID, taskID, worktreePath string) (*models.Agent, error) {
	m.mu.Lock()

	if agentID == "" {
		agentID = uuid.New().String()
	}

	agent := &models.Agent{
		ID:           agentID,
		TaskID:       taskID,
		Status:       models.AgentStatusPending,
		WorktreePath: worktreePath,
		StartedAt:    time.Now(),
		TokensUsed:   0,
		Cost:         0,
		RalphIter:    0,
		RalphScore:   nil,
	}

	m.agents[agent.ID] = agent
	m.mu.Unlock()

	m.emit(LifecycleEvent{
		Type:      LifecycleEventCreated,
		AgentID:   agent.ID,
		TaskID:    taskID,
		ToStatus:  models.AgentStatusPending,
		Timestamp: time.Now(),
	})

	return agent, nil
}

// Start transitions an agent from pending to running.
func (m *Manager) Start(agentID string, pid int) error {
	m.mu.Lock()

	agent, ok := m.agents[agentID]
	if !ok {
		m.mu.Unlock()
		return ErrAgentNotFound
	}

	if !CanTransition(agent.Status, models.AgentStatusRunning) {
		m.mu.Unlock()
		return fmt.Errorf("%w: cannot transition from %s to running", ErrInvalidTransition, agent.Status)
	}

	fromStatus := agent.Status
	agent.Status = models.AgentStatusRunning
	agent.PID = pid
	eventData := LifecycleEvent{
		Type:       LifecycleEventStarted,
		AgentID:    agent.ID,
		TaskID:     agent.TaskID,
		FromStatus: fromStatus,
		ToStatus:   models.AgentStatusRunning,
		Timestamp:  time.Now(),
	}
	m.mu.Unlock()

	m.emit(eventData)

	return nil
}

// Pause transitions a running agent to paused.
func (m *Manager) Pause(agentID string) error {
	m.mu.Lock()

	agent, ok := m.agents[agentID]
	if !ok {
		m.mu.Unlock()
		return ErrAgentNotFound
	}

	if !CanTransition(agent.Status, models.AgentStatusPaused) {
		m.mu.Unlock()
		return fmt.Errorf("%w: cannot transition from %s to paused", ErrInvalidTransition, agent.Status)
	}

	fromStatus := agent.Status
	agent.Status = models.AgentStatusPaused
	agent.PID = 0
	eventData := LifecycleEvent{
		Type:       LifecycleEventPaused,
		AgentID:    agent.ID,
		TaskID:     agent.TaskID,
		FromStatus: fromStatus,
		ToStatus:   models.AgentStatusPaused,
		Timestamp:  time.Now(),
	}
	m.mu.Unlock()

	m.emit(eventData)

	return nil
}

// Resume transitions a paused or waiting_approval agent back to running.
func (m *Manager) Resume(agentID string, pid int) error {
	m.mu.Lock()

	agent, ok := m.agents[agentID]
	if !ok {
		m.mu.Unlock()
		return ErrAgentNotFound
	}

	if !CanTransition(agent.Status, models.AgentStatusRunning) {
		m.mu.Unlock()
		return fmt.Errorf("%w: cannot transition from %s to running", ErrInvalidTransition, agent.Status)
	}

	fromStatus := agent.Status
	agent.Status = models.AgentStatusRunning
	agent.PID = pid
	eventData := LifecycleEvent{
		Type:       LifecycleEventResumed,
		AgentID:    agent.ID,
		TaskID:     agent.TaskID,
		FromStatus: fromStatus,
		ToStatus:   models.AgentStatusRunning,
		Timestamp:  time.Now(),
	}
	m.mu.Unlock()

	m.emit(eventData)

	return nil
}

// WaitApproval transitions a running agent to waiting_approval.
func (m *Manager) WaitApproval(agentID string) error {
	m.mu.Lock()

	agent, ok := m.agents[agentID]
	if !ok {
		m.mu.Unlock()
		return ErrAgentNotFound
	}

	if !CanTransition(agent.Status, models.AgentStatusWaitingApproval) {
		m.mu.Unlock()
		return fmt.Errorf("%w: cannot transition from %s to waiting_approval", ErrInvalidTransition, agent.Status)
	}

	fromStatus := agent.Status
	agent.Status = models.AgentStatusWaitingApproval
	eventData := LifecycleEvent{
		Type:       LifecycleEventWaitingApproval,
		AgentID:    agent.ID,
		TaskID:     agent.TaskID,
		FromStatus: fromStatus,
		ToStatus:   models.AgentStatusWaitingApproval,
		Timestamp:  time.Now(),
	}
	m.mu.Unlock()

	m.emit(eventData)

	return nil
}

// Complete transitions a running agent to done.
func (m *Manager) Complete(agentID string) error {
	m.mu.Lock()

	agent, ok := m.agents[agentID]
	if !ok {
		m.mu.Unlock()
		return ErrAgentNotFound
	}

	if !CanTransition(agent.Status, models.AgentStatusDone) {
		m.mu.Unlock()
		return fmt.Errorf("%w: cannot transition from %s to done", ErrInvalidTransition, agent.Status)
	}

	fromStatus := agent.Status
	agent.Status = models.AgentStatusDone
	agent.PID = 0
	eventData := LifecycleEvent{
		Type:       LifecycleEventCompleted,
		AgentID:    agent.ID,
		TaskID:     agent.TaskID,
		FromStatus: fromStatus,
		ToStatus:   models.AgentStatusDone,
		Timestamp:  time.Now(),
	}
	m.mu.Unlock()

	m.emit(eventData)

	return nil
}

// Fail transitions an agent to failed state.
func (m *Manager) Fail(agentID string, reason string) error {
	m.mu.Lock()

	agent, ok := m.agents[agentID]
	if !ok {
		m.mu.Unlock()
		return ErrAgentNotFound
	}

	if !CanTransition(agent.Status, models.AgentStatusFailed) {
		m.mu.Unlock()
		return fmt.Errorf("%w: cannot transition from %s to failed", ErrInvalidTransition, agent.Status)
	}

	fromStatus := agent.Status
	agent.Status = models.AgentStatusFailed
	agent.PID = 0
	eventData := LifecycleEvent{
		Type:       LifecycleEventFailed,
		AgentID:    agent.ID,
		TaskID:     agent.TaskID,
		FromStatus: fromStatus,
		ToStatus:   models.AgentStatusFailed,
		Timestamp:  time.Now(),
		Error:      reason,
	}
	m.mu.Unlock()

	m.emit(eventData)

	return nil
}

// GetStatus returns the current status of an agent.
func (m *Manager) GetStatus(agentID string) (models.AgentStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, ok := m.agents[agentID]
	if !ok {
		return "", ErrAgentNotFound
	}

	return agent.Status, nil
}

// Get returns a copy of the agent.
func (m *Manager) Get(agentID string) (*models.Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, ok := m.agents[agentID]
	if !ok {
		return nil, ErrAgentNotFound
	}

	// Return a copy to avoid race conditions
	copy := *agent
	return &copy, nil
}

// UpdateUsage updates the token usage and cost for an agent.
func (m *Manager) UpdateUsage(agentID string, tokensUsed int64, cost float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, ok := m.agents[agentID]
	if !ok {
		return ErrAgentNotFound
	}

	agent.TokensUsed = tokensUsed
	agent.Cost = cost
	return nil
}

// UpdateRalph updates the Ralph iteration and score for an agent.
func (m *Manager) UpdateRalph(agentID string, iter int, score *models.RubricScore) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, ok := m.agents[agentID]
	if !ok {
		return ErrAgentNotFound
	}

	agent.RalphIter = iter
	agent.RalphScore = score
	return nil
}

// List returns all agents.
func (m *Manager) List() []*models.Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*models.Agent, 0, len(m.agents))
	for _, a := range m.agents {
		copy := *a
		agents = append(agents, &copy)
	}
	return agents
}

// ListByTask returns all agents for a given task.
func (m *Manager) ListByTask(taskID string) []*models.Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*models.Agent, 0)
	for _, a := range m.agents {
		if a.TaskID == taskID {
			copy := *a
			agents = append(agents, &copy)
		}
	}
	return agents
}

// ListByStatus returns all agents with a given status.
func (m *Manager) ListByStatus(status models.AgentStatus) []*models.Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*models.Agent, 0)
	for _, a := range m.agents {
		if a.Status == status {
			copy := *a
			agents = append(agents, &copy)
		}
	}
	return agents
}

// Remove removes an agent from the manager.
// This does not affect the agent's state, it just removes tracking.
func (m *Manager) Remove(agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.agents[agentID]; !ok {
		return ErrAgentNotFound
	}

	delete(m.agents, agentID)
	return nil
}

// Load loads an existing agent into the manager (for recovery scenarios).
func (m *Manager) Load(agent *models.Agent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.agents[agent.ID]; ok {
		return ErrAgentAlreadyExists
	}

	// Make a copy to avoid external mutations
	copy := *agent
	m.agents[copy.ID] = &copy
	return nil
}
