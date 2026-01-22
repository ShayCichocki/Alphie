// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"sync"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// AgentRegistry manages agent state and results.
// It provides thread-safe storage and retrieval of agent information.
type AgentRegistry struct {
	// agents maps agent IDs to agent models.
	agents map[string]*models.Agent
	// results maps agent IDs to execution results.
	results map[string]*agent.ExecutionResult
	// mu protects all fields.
	mu sync.RWMutex
}

// NewAgentRegistry creates a new AgentRegistry.
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		agents:  make(map[string]*models.Agent),
		results: make(map[string]*agent.ExecutionResult),
	}
}

// Register adds an agent to the registry.
func (r *AgentRegistry) Register(a *models.Agent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[a.ID] = a
}

// StoreResult stores an execution result for an agent.
func (r *AgentRegistry) StoreResult(agentID string, result *agent.ExecutionResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results[agentID] = result
}

// GetResult retrieves the execution result for an agent.
// Returns nil if no result is stored for the agent.
func (r *AgentRegistry) GetResult(agentID string) *agent.ExecutionResult {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.results[agentID]
}

// GetAgent retrieves an agent by ID.
// Returns nil if the agent is not registered.
func (r *AgentRegistry) GetAgent(agentID string) *models.Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.agents[agentID]
}

// Unregister removes an agent and its result from the registry.
func (r *AgentRegistry) Unregister(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, agentID)
	delete(r.results, agentID)
}

// AllAgents returns a copy of all registered agents.
func (r *AgentRegistry) AllAgents() []*models.Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]*models.Agent, 0, len(r.agents))
	for _, a := range r.agents {
		agents = append(agents, a)
	}
	return agents
}

// Count returns the number of registered agents.
func (r *AgentRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.agents)
}
