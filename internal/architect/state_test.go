package architect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateStore(t *testing.T) {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "state_test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Test NewStateStore
	store, err := NewStateStore(dbPath)
	if err != nil {
		t.Fatalf("NewStateStore: %v", err)
	}
	defer store.Close()

	// Test CreateSession
	session, err := store.CreateSession("/path/to/arch.md")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if session.ID == "" {
		t.Error("session ID should not be empty")
	}
	if session.ArchDoc != "/path/to/arch.md" {
		t.Errorf("ArchDoc = %q, want %q", session.ArchDoc, "/path/to/arch.md")
	}
	if session.Status != "started" {
		t.Errorf("Status = %q, want %q", session.Status, "started")
	}
	if session.Iteration != 0 {
		t.Errorf("Iteration = %d, want 0", session.Iteration)
	}

	// Test GetSession
	retrieved, err := store.GetSession(session.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if retrieved.ID != session.ID {
		t.Errorf("ID = %q, want %q", retrieved.ID, session.ID)
	}
	if retrieved.ArchDoc != session.ArchDoc {
		t.Errorf("ArchDoc = %q, want %q", retrieved.ArchDoc, session.ArchDoc)
	}

	// Test UpdateSession
	session.Iteration = 3
	session.TotalCost = 0.05
	session.Status = "running"
	session.CheckpointPath = "/tmp/checkpoint.json"
	if err := store.UpdateSession(session); err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}

	updated, err := store.GetSession(session.ID)
	if err != nil {
		t.Fatalf("GetSession after update: %v", err)
	}
	if updated.Iteration != 3 {
		t.Errorf("Iteration = %d, want 3", updated.Iteration)
	}
	if updated.TotalCost != 0.05 {
		t.Errorf("TotalCost = %f, want 0.05", updated.TotalCost)
	}
	if updated.Status != "running" {
		t.Errorf("Status = %q, want %q", updated.Status, "running")
	}
	if updated.CheckpointPath != "/tmp/checkpoint.json" {
		t.Errorf("CheckpointPath = %q, want %q", updated.CheckpointPath, "/tmp/checkpoint.json")
	}

	// Test DeleteSession
	if err := store.DeleteSession(session.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	// Verify deletion
	_, err = store.GetSession(session.ID)
	if err == nil {
		t.Error("GetSession after delete should return error")
	}
}

func TestStateStoreNotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "state_test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewStateStore(dbPath)
	if err != nil {
		t.Fatalf("NewStateStore: %v", err)
	}
	defer store.Close()

	// Test GetSession with non-existent ID
	_, err = store.GetSession("non-existent")
	if err == nil {
		t.Error("GetSession should return error for non-existent ID")
	}

	// Test UpdateSession with non-existent ID
	session := &Session{ID: "non-existent"}
	err = store.UpdateSession(session)
	if err == nil {
		t.Error("UpdateSession should return error for non-existent ID")
	}

	// Test DeleteSession with non-existent ID
	err = store.DeleteSession("non-existent")
	if err == nil {
		t.Error("DeleteSession should return error for non-existent ID")
	}
}
