package models

import "time"

// TaskStatus represents the current state of a task.
type TaskStatus string

const (
	// TaskStatusPending indicates the task has not started.
	TaskStatusPending TaskStatus = "pending"
	// TaskStatusInProgress indicates the task is being worked on.
	TaskStatusInProgress TaskStatus = "in_progress"
	// TaskStatusBlocked indicates the task cannot proceed.
	TaskStatusBlocked TaskStatus = "blocked"
	// TaskStatusDone indicates the task completed successfully.
	TaskStatusDone TaskStatus = "done"
	// TaskStatusFailed indicates the task failed.
	TaskStatusFailed TaskStatus = "failed"
)

// Valid returns true if the status is a known value.
func (s TaskStatus) Valid() bool {
	switch s {
	case TaskStatusPending, TaskStatusInProgress, TaskStatusBlocked, TaskStatusDone, TaskStatusFailed:
		return true
	default:
		return false
	}
}

// Task represents a unit of work in the system.
type Task struct {
	// ID is the unique identifier for this task.
	ID string `json:"id"`
	// ParentID is the ID of the parent task or epic, if any.
	ParentID string `json:"parent_id,omitempty"`
	// Title is the short description of the task.
	Title string `json:"title"`
	// Description provides detailed information about the task.
	Description string `json:"description,omitempty"`
	// AcceptanceCriteria defines the criteria for task completion.
	AcceptanceCriteria string `json:"acceptance_criteria,omitempty"`
	// Status is the current state of the task.
	Status TaskStatus `json:"status"`
	// DependsOn lists task IDs that must complete before this task.
	DependsOn []string `json:"depends_on,omitempty"`
	// AssignedTo is the ID of the agent working on this task.
	AssignedTo string `json:"assigned_to,omitempty"`
	// Tier is the agent tier required for this task.
	Tier Tier `json:"tier"`
	// CreatedAt is when the task was created.
	CreatedAt time.Time `json:"created_at"`
	// CompletedAt is when the task was completed, if applicable.
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	// Error contains the error message if the task failed.
	Error string `json:"error,omitempty"`
	// RetryCount is the number of times this task has been retried.
	RetryCount int `json:"retry_count,omitempty"`
}

// RubricScore holds quality scores for completed work.
type RubricScore struct {
	// Correctness measures functional correctness (1-3).
	Correctness int `json:"correctness"`
	// Readability measures code clarity and style (1-3).
	Readability int `json:"readability"`
	// EdgeCases measures handling of edge cases (1-3).
	EdgeCases int `json:"edge_cases"`
}

// Valid returns true if all scores are in the valid range (1-3).
func (r RubricScore) Valid() bool {
	return r.Correctness >= 1 && r.Correctness <= 3 &&
		r.Readability >= 1 && r.Readability <= 3 &&
		r.EdgeCases >= 1 && r.EdgeCases <= 3
}

// Total returns the sum of all scores.
func (r RubricScore) Total() int {
	return r.Correctness + r.Readability + r.EdgeCases
}

// Session represents an active work session.
type Session struct {
	// ID is the unique identifier for this session.
	ID string `json:"id"`
	// RootTask is the ID of the root task being worked on.
	RootTask string `json:"root_task"`
	// Tier is the agent tier for this session.
	Tier Tier `json:"tier"`
	// TokenBudget is the maximum tokens allowed for this session.
	TokenBudget int64 `json:"token_budget"`
	// TokensUsed is the number of tokens consumed so far.
	TokensUsed int64 `json:"tokens_used"`
	// StartedAt is when the session began.
	StartedAt time.Time `json:"started_at"`
	// Status is the current state of the session.
	Status TaskStatus `json:"status"`
}
