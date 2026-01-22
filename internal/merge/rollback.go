// Package merge provides rollback functionality for failed merges.
package merge

import (
	"fmt"

	"github.com/ShayCichocki/alphie/internal/git"
)

// RollbackManager handles rolling back the session branch to a previous checkpoint.
type RollbackManager struct {
	repo       git.Runner
	checkpoints *CheckpointManager
}

// NewRollbackManager creates a new rollback manager.
func NewRollbackManager(repo git.Runner, checkpoints *CheckpointManager) *RollbackManager {
	return &RollbackManager{
		repo:        repo,
		checkpoints: checkpoints,
	}
}

// RollbackOptions specifies rollback behavior.
type RollbackOptions struct {
	// TargetAgentID is the agent whose checkpoint to roll back to.
	// If empty, rolls back to the last good checkpoint.
	TargetAgentID string
	// Hard performs a hard reset (discards all changes).
	// If false, performs a mixed reset (keeps changes in working directory).
	Hard bool
}

// RollbackResult contains the result of a rollback operation.
type RollbackResult struct {
	// Success indicates whether the rollback succeeded.
	Success bool
	// PreviousCommit is the commit SHA before rollback.
	PreviousCommit string
	// NewCommit is the commit SHA after rollback.
	NewCommit string
	// Checkpoint is the checkpoint that was rolled back to.
	Checkpoint *Checkpoint
	// Error contains any error that occurred.
	Error error
}

// Rollback rolls back the session branch to a previous checkpoint.
// It performs a hard or mixed reset to the checkpoint's commit SHA.
func (rm *RollbackManager) Rollback(opts RollbackOptions) (*RollbackResult, error) {
	// Get current commit before rollback
	previousCommit, err := rm.repo.Run("rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("get current commit: %w", err)
	}

	// Determine target checkpoint
	var targetCheckpoint *Checkpoint
	if opts.TargetAgentID != "" {
		targetCheckpoint, err = rm.checkpoints.GetCheckpoint(opts.TargetAgentID)
		if err != nil {
			return nil, fmt.Errorf("get checkpoint: %w", err)
		}
	} else {
		// Roll back to last good checkpoint
		targetCheckpoint = rm.checkpoints.GetLastGoodCheckpoint()
		if targetCheckpoint == nil {
			return nil, fmt.Errorf("no good checkpoints available for rollback")
		}
	}

	// Perform reset
	resetType := "--mixed"
	if opts.Hard {
		resetType = "--hard"
	}

	if _, err := rm.repo.Run("reset", resetType, targetCheckpoint.CommitSHA); err != nil {
		return &RollbackResult{
			Success:        false,
			PreviousCommit: previousCommit,
			Checkpoint:     targetCheckpoint,
			Error:          fmt.Errorf("git reset failed: %w", err),
		}, err
	}

	// Get new commit after reset
	newCommit, err := rm.repo.Run("rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("get new commit: %w", err)
	}

	return &RollbackResult{
		Success:        true,
		PreviousCommit: previousCommit,
		NewCommit:      newCommit,
		Checkpoint:     targetCheckpoint,
	}, nil
}

// RollbackToCheckpoint is a convenience method that rolls back to a specific checkpoint by agent ID.
func (rm *RollbackManager) RollbackToCheckpoint(agentID string, hard bool) (*RollbackResult, error) {
	return rm.Rollback(RollbackOptions{
		TargetAgentID: agentID,
		Hard:          hard,
	})
}

// RollbackToLastGood is a convenience method that rolls back to the last good checkpoint.
func (rm *RollbackManager) RollbackToLastGood(hard bool) (*RollbackResult, error) {
	return rm.Rollback(RollbackOptions{
		Hard: hard,
	})
}

// GetRollbackOptions returns a list of available rollback options.
// Each option includes the checkpoint and a description.
type RollbackOption struct {
	Checkpoint  *Checkpoint
	Description string
}

// ListRollbackOptions returns all good checkpoints as rollback options.
func (rm *RollbackManager) ListRollbackOptions() []RollbackOption {
	goodCheckpoints := rm.checkpoints.ListGoodCheckpoints()
	options := make([]RollbackOption, len(goodCheckpoints))

	for i, cp := range goodCheckpoints {
		options[i] = RollbackOption{
			Checkpoint:  cp,
			Description: fmt.Sprintf("Agent %s (Task %s) - %s", cp.AgentID, cp.TaskID, cp.CreatedAt.Format("15:04:05")),
		}
	}

	return options
}
