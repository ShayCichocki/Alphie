// Package merge provides interactive merge conflict resolution.
package merge

import (
	"context"
	"fmt"
)

// ResolutionStrategy defines how to resolve a merge conflict.
type ResolutionStrategy int

const (
	// AcceptSession keeps the current session state.
	AcceptSession ResolutionStrategy = iota
	// AcceptAgent takes the agent's changes.
	AcceptAgent
	// ManualMerge opens an editor with conflict markers.
	ManualMerge
	// SkipAgent abandons this agent's work and marks task as blocked.
	SkipAgent
	// AbortSession stops the entire orchestration.
	AbortSession
)

// String returns the human-readable name of the strategy.
func (s ResolutionStrategy) String() string {
	switch s {
	case AcceptSession:
		return "Accept Session"
	case AcceptAgent:
		return "Accept Agent"
	case ManualMerge:
		return "Manual Merge"
	case SkipAgent:
		return "Skip Agent"
	case AbortSession:
		return "Abort Session"
	default:
		return "Unknown"
	}
}

// ConflictRegion represents a specific conflicting region in a file.
type ConflictRegion struct {
	// StartLine is the starting line number of the conflict.
	StartLine int
	// EndLine is the ending line number of the conflict.
	EndLine int
	// SessionContent is the content from the session branch.
	SessionContent string
	// AgentContent is the content from the agent branch.
	AgentContent string
	// Context provides surrounding lines for context.
	Context string
}

// ConflictPresentation contains all information needed to present a conflict to the user.
type ConflictPresentation struct {
	// BaseContent is the content from the merge base (common ancestor).
	BaseContent string
	// SessionContent is the content from the session branch.
	SessionContent string
	// AgentContent is the content from the agent branch.
	AgentContent string
	// ConflictRegions identifies specific conflicting regions.
	ConflictRegions []ConflictRegion
	// FilePath is the path to the conflicting file.
	FilePath string
	// TaskID is the ID of the task that created this conflict.
	TaskID string
	// AgentID is the ID of the agent that created this conflict.
	AgentID string
	// SessionBranch is the name of the session branch.
	SessionBranch string
	// AgentBranch is the name of the agent branch.
	AgentBranch string
	// AttemptNumber is which merge attempt this is (1-based).
	AttemptNumber int
}

// Resolution represents the user's choice for resolving a conflict.
type Resolution struct {
	// Strategy is the chosen resolution strategy.
	Strategy ResolutionStrategy
	// MergedContent is the manually merged content (only for ManualMerge strategy).
	MergedContent string
	// SelectedFiles tracks which files to use for Accept strategies.
	// Map of file path -> "session" or "agent"
	SelectedFiles map[string]string
}

// HumanMergeResolver provides interactive conflict resolution.
// Implementations handle presenting conflicts to users and collecting their resolution choices.
type HumanMergeResolver interface {
	// PresentConflict presents a merge conflict to the user and waits for their resolution choice.
	// Returns the user's resolution or an error if resolution fails or is cancelled.
	PresentConflict(ctx context.Context, conflict ConflictPresentation) (Resolution, error)

	// PresentMultipleConflicts presents multiple conflicting files at once.
	// This is useful when semantic merge fails for multiple files simultaneously.
	PresentMultipleConflicts(ctx context.Context, conflicts []ConflictPresentation) (Resolution, error)
}

// NoOpResolver is a resolver that always returns an error.
// Used when interactive resolution is not available (headless mode, CI/CD).
type NoOpResolver struct{}

// PresentConflict always returns an error indicating interactive resolution is not available.
func (n *NoOpResolver) PresentConflict(ctx context.Context, conflict ConflictPresentation) (Resolution, error) {
	return Resolution{}, fmt.Errorf("interactive merge resolution not available in headless mode")
}

// PresentMultipleConflicts always returns an error indicating interactive resolution is not available.
func (n *NoOpResolver) PresentMultipleConflicts(ctx context.Context, conflicts []ConflictPresentation) (Resolution, error) {
	return Resolution{}, fmt.Errorf("interactive merge resolution not available in headless mode")
}
