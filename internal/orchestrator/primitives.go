// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"time"

	"github.com/ShayCichocki/alphie/internal/agent"
)

// TaskOutcome represents the result of a task execution.
// This primitive unifies the various success/failure/abort states
// that were previously scattered across the codebase.
type TaskOutcome struct {
	// Status indicates the final state of the task.
	Status OutcomeStatus
	// TaskID is the ID of the completed task.
	TaskID string
	// AgentID is the ID of the agent that executed the task.
	AgentID string
	// Result contains the execution result from the agent.
	Result *agent.ExecutionResult
	// Error contains any error that occurred (nil on success).
	Error error
	// Duration is the total execution time.
	Duration time.Duration
	// MergeResult contains merge outcome if applicable.
	MergeResult *MergeOutcome
	// Metadata contains additional outcome-specific data.
	Metadata map[string]interface{}
}

// OutcomeStatus represents the final status of a task.
type OutcomeStatus int

const (
	// OutcomeSuccess indicates the task completed and merged successfully.
	OutcomeSuccess OutcomeStatus = iota
	// OutcomeFailed indicates the task failed execution.
	OutcomeFailed
	// OutcomeAborted indicates the task was aborted (max iterations without verification).
	OutcomeAborted
	// OutcomeMergeFailed indicates the task succeeded but merge failed.
	OutcomeMergeFailed
	// OutcomeCancelled indicates the task was cancelled by user/system.
	OutcomeCancelled
	// OutcomeEscalation indicates the task needed user escalation and was handled.
	OutcomeEscalation
)

// String returns a human-readable status name.
func (s OutcomeStatus) String() string {
	switch s {
	case OutcomeSuccess:
		return "success"
	case OutcomeFailed:
		return "failed"
	case OutcomeAborted:
		return "aborted"
	case OutcomeMergeFailed:
		return "merge_failed"
	case OutcomeCancelled:
		return "cancelled"
	case OutcomeEscalation:
		return "escalation"
	default:
		return "unknown"
	}
}

// IsTerminal returns true if this is a terminal state (no further processing).
func (s OutcomeStatus) IsTerminal() bool {
	return s != OutcomeSuccess // Only success might lead to more processing
}

// Note: MergeOutcome is defined in merge_queue.go

// FileBoundary represents the file boundaries for a task.
// This primitive makes file boundaries explicit instead of using []string.
type FileBoundary struct {
	// Paths are the directory or file paths the task may modify.
	Paths []string
	// IsExplicit indicates if boundaries were explicitly set (vs inferred).
	IsExplicit bool
	// Confidence is 0-100 indicating confidence in the boundaries.
	Confidence int
}

// NewFileBoundary creates a new FileBoundary from paths.
func NewFileBoundary(paths []string, explicit bool) *FileBoundary {
	confidence := 50 // Default confidence for inferred
	if explicit {
		confidence = 100
	}
	return &FileBoundary{
		Paths:      paths,
		IsExplicit: explicit,
		Confidence: confidence,
	}
}

// Contains checks if a path falls within this boundary.
func (b *FileBoundary) Contains(path string) bool {
	if len(b.Paths) == 0 {
		return true // No boundaries means everything is in scope
	}
	for _, prefix := range b.Paths {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// Overlaps checks if two boundaries have any overlap.
func (b *FileBoundary) Overlaps(other *FileBoundary) bool {
	if b == nil || other == nil {
		return false
	}
	for _, p1 := range b.Paths {
		for _, p2 := range other.Paths {
			if pathsOverlap(p1, p2) {
				return true
			}
		}
	}
	return false
}

// ProgressReport represents a progress update from an agent.
// This primitive standardizes progress reporting across the system.
type ProgressReport struct {
	// AgentID is the agent sending the update.
	AgentID string
	// TaskID is the task being executed.
	TaskID string
	// Phase describes the current execution phase.
	Phase ExecutionPhase
	// Message is a human-readable status message.
	Message string
	// TokensUsed is the cumulative tokens consumed.
	TokensUsed int
	// Cost is the cumulative cost in dollars.
	Cost float64
	// Duration is the time since execution started.
	Duration time.Duration
	// Iteration is the current Ralph loop iteration (if applicable).
	Iteration int
	// Timestamp is when this report was generated.
	Timestamp time.Time
}

// ExecutionPhase describes the current phase of task execution.
type ExecutionPhase int

const (
	// PhaseStarting indicates the task is starting.
	PhaseStarting ExecutionPhase = iota
	// PhaseImplementing indicates the agent is implementing.
	PhaseImplementing
	// PhaseVerifying indicates the agent is verifying work.
	PhaseVerifying
	// PhaseCritiquing indicates the agent is self-critiquing.
	PhaseCritiquing
	// PhaseImproving indicates the agent is improving based on critique.
	PhaseImproving
	// PhaseMerging indicates work is being merged.
	PhaseMerging
	// PhaseCompleted indicates the task is done.
	PhaseCompleted
)

// String returns a human-readable phase name.
func (p ExecutionPhase) String() string {
	switch p {
	case PhaseStarting:
		return "starting"
	case PhaseImplementing:
		return "implementing"
	case PhaseVerifying:
		return "verifying"
	case PhaseCritiquing:
		return "critiquing"
	case PhaseImproving:
		return "improving"
	case PhaseMerging:
		return "merging"
	case PhaseCompleted:
		return "completed"
	default:
		return "unknown"
	}
}
