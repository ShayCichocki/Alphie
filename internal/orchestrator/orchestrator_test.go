package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ShayCichocki/alphie/internal/prog"
	"github.com/ShayCichocki/alphie/pkg/models"
)

func TestNewOrchestrator(t *testing.T) {
	config := OrchestratorConfig{
		RepoPath:   "/tmp/test-repo",
		Tier:       models.TierBuilder,
		MaxAgents:  4,
		Greenfield: true,
	}

	orch := NewOrchestrator(config)
	if orch == nil {
		t.Fatal("expected non-nil orchestrator")
	}

	if orch.config.Tier != models.TierBuilder {
		t.Errorf("expected tier %v, got %v", models.TierBuilder, orch.config.Tier)
	}

	if orch.config.MaxAgents != 4 {
		t.Errorf("expected maxAgents 4, got %d", orch.config.MaxAgents)
	}

	if !orch.config.Greenfield {
		t.Error("expected greenfield to be true")
	}

	if orch.config.RepoPath != "/tmp/test-repo" {
		t.Errorf("expected repoPath '/tmp/test-repo', got %q", orch.config.RepoPath)
	}
}

func TestNewOrchestratorDefaultMaxAgents(t *testing.T) {
	config := OrchestratorConfig{
		RepoPath:   "/tmp/test-repo",
		Tier:       models.TierScout,
		MaxAgents:  0, // Should default to 4
		Greenfield: true,
	}

	orch := NewOrchestrator(config)
	if orch.config.MaxAgents != 4 {
		t.Errorf("expected default maxAgents 4, got %d", orch.config.MaxAgents)
	}
}

func TestOrchestratorEvents(t *testing.T) {
	config := OrchestratorConfig{
		RepoPath:   "/tmp/test-repo",
		Tier:       models.TierBuilder,
		Greenfield: true,
	}

	orch := NewOrchestrator(config)
	eventCh := orch.Events()
	if eventCh == nil {
		t.Fatal("expected non-nil events channel")
	}
}

func TestOrchestratorEmitEvent(t *testing.T) {
	config := OrchestratorConfig{
		RepoPath:   "/tmp/test-repo",
		Tier:       models.TierBuilder,
		Greenfield: true,
	}

	orch := NewOrchestrator(config)

	// Emit an event
	event := OrchestratorEvent{
		Type:      EventTaskStarted,
		TaskID:    "test-task",
		Message:   "Test message",
		Timestamp: time.Now(),
	}
	orch.emitEvent(event)

	// Verify event was received
	select {
	case received := <-orch.Events():
		if received.Type != EventTaskStarted {
			t.Errorf("expected type %v, got %v", EventTaskStarted, received.Type)
		}
		if received.TaskID != "test-task" {
			t.Errorf("expected taskID 'test-task', got %q", received.TaskID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected to receive event")
	}
}

func TestOrchestratorStop(t *testing.T) {
	config := OrchestratorConfig{
		RepoPath:   "/tmp/test-repo",
		Tier:       models.TierBuilder,
		Greenfield: true,
	}

	orch := NewOrchestrator(config)

	// Stop should succeed on first call
	if err := orch.Stop(); err != nil {
		t.Errorf("expected no error on first stop, got %v", err)
	}

	// Stop should be idempotent
	if err := orch.Stop(); err != nil {
		t.Errorf("expected no error on second stop, got %v", err)
	}
}

func TestOrchestratorRunAfterStop(t *testing.T) {
	config := OrchestratorConfig{
		RepoPath:   "/tmp/test-repo",
		Tier:       models.TierBuilder,
		Greenfield: true,
	}

	orch := NewOrchestrator(config)

	// Stop the orchestrator
	if err := orch.Stop(); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	// Run should fail after stop
	err := orch.Run(context.Background(), "test request")
	if err == nil {
		t.Error("expected error when running after stop")
	}
}

func TestOrchestratorGetSessionBranch(t *testing.T) {
	t.Run("greenfield mode", func(t *testing.T) {
		config := OrchestratorConfig{
			RepoPath:   "/tmp/test-repo",
			Tier:       models.TierBuilder,
			Greenfield: true,
		}

		orch := NewOrchestrator(config)
		branch := orch.GetSessionBranch()
		if branch != "" {
			t.Errorf("expected empty branch in greenfield mode, got %q", branch)
		}
	})

	t.Run("session mode", func(t *testing.T) {
		config := OrchestratorConfig{
			RepoPath:   "/tmp/test-repo",
			Tier:       models.TierBuilder,
			Greenfield: false,
		}

		orch := NewOrchestrator(config)
		branch := orch.GetSessionBranch()
		if branch == "" {
			t.Error("expected non-empty branch in session mode")
		}
		if len(branch) < 8 {
			t.Errorf("expected branch name with session ID, got %q", branch)
		}
	})
}

func TestEventTypes(t *testing.T) {
	// Verify all event types are distinct
	eventTypes := []EventType{
		EventTaskStarted,
		EventTaskCompleted,
		EventTaskFailed,
		EventMergeStarted,
		EventMergeCompleted,
		EventSecondReviewStarted,
		EventSecondReviewCompleted,
		EventSessionDone,
	}

	seen := make(map[EventType]bool)
	for _, et := range eventTypes {
		if seen[et] {
			t.Errorf("duplicate event type: %v", et)
		}
		seen[et] = true
	}
}

func TestOrchestratorWithSecondReviewer(t *testing.T) {
	// Test that second reviewer is created when configured
	config := OrchestratorConfig{
		RepoPath:             "/tmp/test-repo",
		Tier:                 models.TierBuilder,
		MaxAgents:            4,
		Greenfield:           false,
		SecondReviewerClaude: nil, // No Claude, reviewer should be nil
	}

	orch := NewOrchestrator(config)
	if orch.secondReviewer != nil {
		t.Error("expected nil second reviewer when Claude not configured")
	}
}

func TestOrchestratorGreenfieldNoSecondReviewer(t *testing.T) {
	// Test that second reviewer is not created in greenfield mode
	config := OrchestratorConfig{
		RepoPath:   "/tmp/test-repo",
		Tier:       models.TierBuilder,
		MaxAgents:  4,
		Greenfield: true,
	}

	orch := NewOrchestrator(config)
	if orch.secondReviewer != nil {
		t.Error("expected nil second reviewer in greenfield mode")
	}
}

func TestCreateProgEpicAndTasks(t *testing.T) {
	// Setup test prog client
	tmpDir, err := os.MkdirTemp("", "orchestrator-prog-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := prog.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init database: %v", err)
	}

	progClient := prog.NewClient(db, "testproj")

	config := OrchestratorConfig{
		RepoPath:   "/tmp/test-repo",
		Tier:       models.TierBuilder,
		Greenfield: true,
		ProgClient: progClient,
	}

	orch := NewOrchestrator(config)

	// Create test tasks with dependencies
	now := time.Now()
	tasks := []*models.Task{
		{
			ID:          "task-1",
			Title:       "First task",
			Description: "Description 1",
			Status:      models.TaskStatusPending,
			CreatedAt:   now,
		},
		{
			ID:          "task-2",
			Title:       "Second task",
			Description: "Description 2",
			DependsOn:   []string{"task-1"},
			Status:      models.TaskStatusPending,
			CreatedAt:   now,
		},
		{
			ID:          "task-3",
			Title:       "Third task",
			Description: "Description 3",
			DependsOn:   []string{"task-1", "task-2"},
			Status:      models.TaskStatusPending,
			CreatedAt:   now,
		},
	}

	// Test CreateEpicAndTasks via progCoord
	request := "Build a new feature with multiple components"
	err = orch.progCoord.CreateEpicAndTasks(request, tasks)
	if err != nil {
		t.Fatalf("CreateEpicAndTasks failed: %v", err)
	}

	// Verify epic was created
	status, err := progClient.GetStatus("")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	// Should have 1 epic and 3 tasks (all open)
	totalOpen := status.Open
	if totalOpen != 4 {
		t.Errorf("Expected 4 open items (1 epic + 3 tasks), got %d", totalOpen)
	}

	// Verify ready tasks (task-1 should be ready as it has no deps, plus the epic itself)
	ready, err := progClient.ListReadyTasks("")
	if err != nil {
		t.Fatalf("ListReadyTasks failed: %v", err)
	}

	// task-1 is ready (no deps), epic is also ready, task-2 and task-3 have deps
	if len(ready) != 2 {
		t.Errorf("Expected 2 ready items (1 epic + 1 task), got %d", len(ready))
	}
}

func TestCreateProgEpicAndTasksNilClient(t *testing.T) {
	config := OrchestratorConfig{
		RepoPath:   "/tmp/test-repo",
		Tier:       models.TierBuilder,
		Greenfield: true,
		ProgClient: nil, // No prog client
	}

	orch := NewOrchestrator(config)

	now := time.Now()
	tasks := []*models.Task{
		{
			ID:        "task-1",
			Title:     "Test task",
			Status:    models.TaskStatusPending,
			CreatedAt: now,
		},
	}

	// Should succeed (no-op) when client is nil
	err := orch.progCoord.CreateEpicAndTasks("Test request", tasks)
	if err != nil {
		t.Errorf("Expected no error with nil client, got: %v", err)
	}
}

func TestCreateProgEpicAndTasksLongRequest(t *testing.T) {
	// Setup test prog client
	tmpDir, err := os.MkdirTemp("", "orchestrator-prog-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := prog.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init database: %v", err)
	}

	progClient := prog.NewClient(db, "testproj")

	config := OrchestratorConfig{
		RepoPath:   "/tmp/test-repo",
		Tier:       models.TierBuilder,
		Greenfield: true,
		ProgClient: progClient,
	}

	orch := NewOrchestrator(config)

	// Create a very long request
	longRequest := "This is a very long request that exceeds the 100 character limit and should be truncated when used as an epic title. The full request should be preserved in the description though."

	now := time.Now()
	tasks := []*models.Task{
		{
			ID:        "task-1",
			Title:     "Test task",
			Status:    models.TaskStatusPending,
			CreatedAt: now,
		},
	}

	err = orch.progCoord.CreateEpicAndTasks(longRequest, tasks)
	if err != nil {
		t.Fatalf("CreateEpicAndTasks failed: %v", err)
	}

	// The function should have succeeded with truncated title
}
