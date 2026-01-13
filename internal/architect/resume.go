// Package architect provides tools for analyzing and auditing codebases against specifications.
package architect

import (
	"encoding/json"
	"fmt"
	"os"
)

// Checkpoint stores the iteration state for resume capability.
type Checkpoint struct {
	// Iteration is the current iteration number.
	Iteration int `json:"iteration"`
	// CompletedTasks lists feature IDs that have been completed.
	CompletedTasks []string `json:"completed_tasks"`
	// PendingGaps lists gaps that still need to be addressed.
	PendingGaps []Gap `json:"pending_gaps"`
	// Answers stores user responses to questions, keyed by question ID.
	Answers map[string]string `json:"answers"`
}

// SaveCheckpoint writes the checkpoint state to the specified path.
func SaveCheckpoint(state *Checkpoint, path string) error {
	if state == nil {
		return fmt.Errorf("checkpoint state is nil")
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write checkpoint file: %w", err)
	}

	return nil
}

// LoadCheckpoint reads checkpoint state from the specified path.
func LoadCheckpoint(path string) (*Checkpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read checkpoint file: %w", err)
	}

	var state Checkpoint
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
	}

	// Initialize maps if nil to avoid nil pointer issues
	if state.Answers == nil {
		state.Answers = make(map[string]string)
	}

	return &state, nil
}

// DeleteCheckpoint removes the checkpoint file at the specified path.
func DeleteCheckpoint(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove checkpoint file: %w", err)
	}
	return nil
}
