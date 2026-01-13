package models

import "time"

// AgentStatus represents the current state of an agent.
type AgentStatus string

const (
	// AgentStatusPending indicates the agent has not started.
	AgentStatusPending AgentStatus = "pending"
	// AgentStatusRunning indicates the agent is actively working.
	AgentStatusRunning AgentStatus = "running"
	// AgentStatusPaused indicates the agent is temporarily stopped.
	AgentStatusPaused AgentStatus = "paused"
	// AgentStatusWaitingApproval indicates the agent is waiting for human review.
	AgentStatusWaitingApproval AgentStatus = "waiting_approval"
	// AgentStatusDone indicates the agent completed its work.
	AgentStatusDone AgentStatus = "done"
	// AgentStatusFailed indicates the agent encountered an error.
	AgentStatusFailed AgentStatus = "failed"
)

// Valid returns true if the status is a known value.
func (s AgentStatus) Valid() bool {
	switch s {
	case AgentStatusPending, AgentStatusRunning, AgentStatusPaused,
		AgentStatusWaitingApproval, AgentStatusDone, AgentStatusFailed:
		return true
	default:
		return false
	}
}

// Agent represents a Claude Code agent instance.
type Agent struct {
	// ID is the unique identifier for this agent.
	ID string `json:"id"`
	// TaskID is the ID of the task this agent is working on.
	TaskID string `json:"task_id"`
	// Status is the current state of the agent.
	Status AgentStatus `json:"status"`
	// WorktreePath is the path to the agent's git worktree.
	WorktreePath string `json:"worktree_path,omitempty"`
	// PID is the process ID of the running agent.
	PID int `json:"pid,omitempty"`
	// StartedAt is when the agent began working.
	StartedAt time.Time `json:"started_at"`
	// TokensUsed is the number of tokens consumed by this agent.
	TokensUsed int64 `json:"tokens_used"`
	// Cost is the total cost in dollars for this agent's API usage.
	Cost float64 `json:"cost"`
	// RalphIter is the current Ralph review iteration number.
	RalphIter int `json:"ralph_iter"`
	// RalphScore is the most recent review score from Ralph.
	RalphScore *RubricScore `json:"ralph_score,omitempty"`
}
