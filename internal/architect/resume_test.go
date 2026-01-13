package architect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "checkpoint.json")

	state := &Checkpoint{
		Iteration:      3,
		CompletedTasks: []string{"feature-1", "feature-2"},
		PendingGaps: []Gap{
			{
				FeatureID:       "feature-3",
				Status:          AuditStatusPartial,
				Description:     "Missing tests",
				SuggestedAction: "Add unit tests",
			},
		},
		Answers: map[string]string{
			"q1": "yes",
			"q2": "no",
		},
	}

	// Save checkpoint
	if err := SaveCheckpoint(state, path); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("Checkpoint file was not created")
	}

	// Load checkpoint
	loaded, err := LoadCheckpoint(path)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}

	// Verify loaded state
	if loaded.Iteration != state.Iteration {
		t.Errorf("Iteration mismatch: got %d, want %d", loaded.Iteration, state.Iteration)
	}

	if len(loaded.CompletedTasks) != len(state.CompletedTasks) {
		t.Errorf("CompletedTasks length mismatch: got %d, want %d", len(loaded.CompletedTasks), len(state.CompletedTasks))
	}

	if len(loaded.PendingGaps) != len(state.PendingGaps) {
		t.Errorf("PendingGaps length mismatch: got %d, want %d", len(loaded.PendingGaps), len(state.PendingGaps))
	}

	if loaded.PendingGaps[0].FeatureID != state.PendingGaps[0].FeatureID {
		t.Errorf("Gap FeatureID mismatch: got %s, want %s", loaded.PendingGaps[0].FeatureID, state.PendingGaps[0].FeatureID)
	}

	if len(loaded.Answers) != len(state.Answers) {
		t.Errorf("Answers length mismatch: got %d, want %d", len(loaded.Answers), len(state.Answers))
	}

	if loaded.Answers["q1"] != state.Answers["q1"] {
		t.Errorf("Answer q1 mismatch: got %s, want %s", loaded.Answers["q1"], state.Answers["q1"])
	}
}

func TestSaveCheckpointNilState(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "checkpoint.json")

	err := SaveCheckpoint(nil, path)
	if err == nil {
		t.Error("Expected error for nil state, got nil")
	}
}

func TestLoadCheckpointNotFound(t *testing.T) {
	_, err := LoadCheckpoint("/nonexistent/path/checkpoint.json")
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

func TestLoadCheckpointInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "checkpoint.json")

	if err := os.WriteFile(path, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := LoadCheckpoint(path)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestLoadCheckpointNilAnswers(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "checkpoint.json")

	// Write JSON without answers field
	content := `{"iteration": 1, "completed_tasks": [], "pending_gaps": []}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	loaded, err := LoadCheckpoint(path)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}

	// Answers should be initialized to empty map, not nil
	if loaded.Answers == nil {
		t.Error("Expected Answers to be initialized, got nil")
	}
}

func TestDeleteCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "checkpoint.json")

	// Create a file to delete
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Delete checkpoint
	if err := DeleteCheckpoint(path); err != nil {
		t.Fatalf("DeleteCheckpoint failed: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("Checkpoint file still exists after delete")
	}
}

func TestDeleteCheckpointNotFound(t *testing.T) {
	// Deleting nonexistent file should not error
	err := DeleteCheckpoint("/nonexistent/path/checkpoint.json")
	if err != nil {
		t.Errorf("Expected no error for nonexistent file, got: %v", err)
	}
}
