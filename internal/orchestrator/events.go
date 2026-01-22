// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"time"
)

// EventType represents the type of orchestrator event.
type EventType string

const (
	// EventTaskStarted indicates a task has started execution.
	EventTaskStarted EventType = "task_started"
	// EventTaskCompleted indicates a task completed successfully.
	EventTaskCompleted EventType = "task_completed"
	// EventTaskFailed indicates a task failed.
	EventTaskFailed EventType = "task_failed"
	// EventMergeStarted indicates a merge operation has started.
	EventMergeStarted EventType = "merge_started"
	// EventMergeCompleted indicates a merge operation completed.
	EventMergeCompleted EventType = "merge_completed"
	// EventSecondReviewStarted indicates a second review has started.
	EventSecondReviewStarted EventType = "second_review_started"
	// EventSecondReviewCompleted indicates a second review has completed.
	EventSecondReviewCompleted EventType = "second_review_completed"
	// EventSessionDone indicates the entire session is complete.
	EventSessionDone EventType = "session_done"
	// EventTaskBlocked indicates a task is blocked and cannot proceed.
	EventTaskBlocked EventType = "task_blocked"
	// EventTaskQueued indicates a task is ready and queued for execution.
	EventTaskQueued EventType = "task_queued"
	// EventAgentProgress provides periodic updates on agent execution.
	EventAgentProgress EventType = "agent_progress"
	// EventEpicCreated indicates a new epic has been created to track subtasks.
	EventEpicCreated EventType = "epic_created"
)

// OrchestratorEvent represents an event emitted by the orchestrator.
// These events are used to update the TUI and track progress.
type OrchestratorEvent struct {
	// Type is the kind of event.
	Type EventType
	// TaskID is the ID of the related task, if applicable.
	TaskID string
	// TaskTitle is the title of the related task, if applicable.
	TaskTitle string
	// ParentID is the ID of the parent task/epic, if applicable.
	ParentID string
	// AgentID is the ID of the related agent, if applicable.
	AgentID string
	// Message provides additional context about the event.
	Message string
	// Error contains error details for failure events.
	Error error
	// Timestamp is when the event occurred.
	Timestamp time.Time
	// TokensUsed is the current total tokens used (for progress events).
	TokensUsed int64
	// Cost is the current total cost (for progress events).
	Cost float64
	// Duration is the elapsed time (for progress events).
	Duration time.Duration
	// LogFile is the path to the detailed execution log.
	LogFile string
	// CurrentAction describes what the agent is doing right now (e.g., "Reading auth.go").
	CurrentAction string
	// OriginalTaskID is the task ID from TUI's task_entered event (for epic_created events).
	OriginalTaskID string
}
