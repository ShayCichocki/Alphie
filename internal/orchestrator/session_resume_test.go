package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ShayCichocki/alphie/internal/prog"
)

func TestSessionResumeChecker_CheckForResumableSessions_NoSessions(t *testing.T) {
	client := setupProgClient(t)
	defer client.Close()

	checker := NewSessionResumeChecker(client, nil, "")
	result, err := checker.CheckForResumableSessions()
	if err != nil {
		t.Fatalf("CheckForResumableSessions failed: %v", err)
	}

	if result.HasResumableSessions {
		t.Error("Expected no resumable sessions")
	}
	if len(result.ResumableSessions) != 0 {
		t.Errorf("Expected 0 sessions, got %d", len(result.ResumableSessions))
	}
}

func TestSessionResumeChecker_CheckForResumableSessions_WithInProgressEpic(t *testing.T) {
	client := setupProgClient(t)
	defer client.Close()

	// Create an epic and mark it in-progress
	epicID, err := client.CreateEpic("Test Epic", nil)
	if err != nil {
		t.Fatalf("CreateEpic failed: %v", err)
	}

	// Create some tasks under the epic
	task1, _ := client.CreateTask("Task 1", &prog.TaskOptions{ParentID: epicID})
	task2, _ := client.CreateTask("Task 2", &prog.TaskOptions{ParentID: epicID})
	_, _ = client.CreateTask("Task 3", &prog.TaskOptions{ParentID: epicID})

	// Mark epic as in-progress
	_ = client.Start(epicID)

	// Complete one task
	_ = client.Done(task1)

	checker := NewSessionResumeChecker(client, nil, "")
	result, err := checker.CheckForResumableSessions()
	if err != nil {
		t.Fatalf("CheckForResumableSessions failed: %v", err)
	}

	if !result.HasResumableSessions {
		t.Error("Expected resumable sessions")
	}
	if len(result.ResumableSessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(result.ResumableSessions))
	}
	if result.RecommendedEpicID != epicID {
		t.Errorf("Expected recommended epic %s, got %s", epicID, result.RecommendedEpicID)
	}

	session := result.ResumableSessions[0]
	if session.EpicID != epicID {
		t.Errorf("Expected epic ID %s, got %s", epicID, session.EpicID)
	}
	if session.Title != "Test Epic" {
		t.Errorf("Expected title 'Test Epic', got %q", session.Title)
	}
	if session.CompletedTasks != 1 {
		t.Errorf("Expected 1 completed task, got %d", session.CompletedTasks)
	}
	if session.TotalTasks != 3 {
		t.Errorf("Expected 3 total tasks, got %d", session.TotalTasks)
	}
	// 2 incomplete tasks (task2 and task3)
	if len(session.IncompleteTasks) != 2 {
		t.Errorf("Expected 2 incomplete tasks, got %d", len(session.IncompleteTasks))
	}
	_ = task2 // used in assertions
}

func TestSessionResumeChecker_CheckForResumableSessions_WithOpenEpic(t *testing.T) {
	client := setupProgClient(t)
	defer client.Close()

	// Create an open epic (not yet started)
	epicID, err := client.CreateEpic("Open Epic", nil)
	if err != nil {
		t.Fatalf("CreateEpic failed: %v", err)
	}

	// Create some tasks
	_, _ = client.CreateTask("Task 1", &prog.TaskOptions{ParentID: epicID})

	checker := NewSessionResumeChecker(client, nil, "")
	result, err := checker.CheckForResumableSessions()
	if err != nil {
		t.Fatalf("CheckForResumableSessions failed: %v", err)
	}

	if !result.HasResumableSessions {
		t.Error("Expected resumable sessions (open epic)")
	}
	if result.RecommendedEpicID != epicID {
		t.Errorf("Expected recommended epic %s, got %s", epicID, result.RecommendedEpicID)
	}
}

func TestSessionResumeChecker_CheckForResumableSessions_PrioritizesInProgress(t *testing.T) {
	client := setupProgClient(t)
	defer client.Close()

	// Create an open epic
	openEpicID, _ := client.CreateEpic("Open Epic", nil)

	// Create an in-progress epic
	inProgressEpicID, _ := client.CreateEpic("In Progress Epic", nil)
	_ = client.Start(inProgressEpicID)

	checker := NewSessionResumeChecker(client, nil, "")
	result, err := checker.CheckForResumableSessions()
	if err != nil {
		t.Fatalf("CheckForResumableSessions failed: %v", err)
	}

	// Should recommend the in-progress epic over the open one
	if result.RecommendedEpicID != inProgressEpicID {
		t.Errorf("Expected recommended epic %s (in-progress), got %s", inProgressEpicID, result.RecommendedEpicID)
	}
	_ = openEpicID // used in assertions
}

func TestSessionResumeChecker_CheckForResumableSessions_SkipsDoneEpics(t *testing.T) {
	client := setupProgClient(t)
	defer client.Close()

	// Create a done epic
	epicID, _ := client.CreateEpic("Done Epic", nil)
	_ = client.Done(epicID)

	checker := NewSessionResumeChecker(client, nil, "")
	result, err := checker.CheckForResumableSessions()
	if err != nil {
		t.Fatalf("CheckForResumableSessions failed: %v", err)
	}

	if result.HasResumableSessions {
		t.Error("Should not report done epic as resumable")
	}
}

func TestFormatResumeSuggestion_NoSessions(t *testing.T) {
	result := &SessionRecoveryResult{
		HasResumableSessions: false,
	}

	suggestion := FormatResumeSuggestion(result)
	if suggestion != "" {
		t.Errorf("Expected empty suggestion, got %q", suggestion)
	}
}

func TestFormatResumeSuggestion_WithSessions(t *testing.T) {
	result := &SessionRecoveryResult{
		HasResumableSessions: true,
		ResumableSessions: []ResumableSession{
			{
				EpicID:          "ep-123456",
				Title:           "Add feature X",
				CompletedTasks:  2,
				TotalTasks:      5,
				IncompleteTasks: []string{"ts-1", "ts-2", "ts-3"},
			},
		},
		RecommendedEpicID: "ep-123456",
	}

	suggestion := FormatResumeSuggestion(result)

	if !strings.Contains(suggestion, "Resumable Sessions Found") {
		t.Error("Expected header in suggestion")
	}
	if !strings.Contains(suggestion, "ep-123456") {
		t.Error("Expected epic ID in suggestion")
	}
	if !strings.Contains(suggestion, "Add feature X") {
		t.Error("Expected title in suggestion")
	}
	if !strings.Contains(suggestion, "2/5") {
		t.Error("Expected progress in suggestion")
	}
	if !strings.Contains(suggestion, "--epic ep-123456") {
		t.Error("Expected resume command in suggestion")
	}
}

func TestFormatResumeSuggestion_WithOrphanedWorktrees(t *testing.T) {
	result := &SessionRecoveryResult{
		HasResumableSessions: true,
		ResumableSessions: []ResumableSession{
			{
				EpicID:         "ep-123456",
				Title:          "Test",
				CompletedTasks: 0,
				TotalTasks:     1,
			},
		},
		OrphanedWorktrees: []string{"/path/to/orphan1", "/path/to/orphan2"},
		RecommendedEpicID: "ep-123456",
	}

	suggestion := FormatResumeSuggestion(result)

	if !strings.Contains(suggestion, "2 orphaned worktree") {
		t.Error("Expected orphaned worktree count in suggestion")
	}
	if !strings.Contains(suggestion, "alphie cleanup") {
		t.Error("Expected cleanup command suggestion")
	}
}

func TestClient_ListInProgressEpics(t *testing.T) {
	client := setupProgClient(t)
	defer client.Close()

	// Create epics with different statuses
	epic1, _ := client.CreateEpic("Open Epic", nil)
	epic2, _ := client.CreateEpic("In Progress Epic 1", nil)
	epic3, _ := client.CreateEpic("In Progress Epic 2", nil)
	epic4, _ := client.CreateEpic("Done Epic", nil)

	_ = client.Start(epic2)
	_ = client.Start(epic3)
	_ = client.Done(epic4)

	epics, err := client.ListInProgressEpics()
	if err != nil {
		t.Fatalf("ListInProgressEpics failed: %v", err)
	}

	if len(epics) != 2 {
		t.Errorf("Expected 2 in-progress epics, got %d", len(epics))
	}

	ids := make(map[string]bool)
	for _, e := range epics {
		ids[e.ID] = true
	}
	if !ids[epic2] || !ids[epic3] {
		t.Error("Missing expected in-progress epics")
	}
	_ = epic1 // used in test setup
}

func TestClient_ListOpenOrInProgressEpics(t *testing.T) {
	client := setupProgClient(t)
	defer client.Close()

	// Create epics with different statuses
	epic1, _ := client.CreateEpic("Open Epic", nil)
	epic2, _ := client.CreateEpic("In Progress Epic", nil)
	epic3, _ := client.CreateEpic("Done Epic", nil)
	epic4, _ := client.CreateEpic("Blocked Epic", nil)

	_ = client.Start(epic2)
	_ = client.Done(epic3)
	_ = client.Block(epic4)

	epics, err := client.ListOpenOrInProgressEpics()
	if err != nil {
		t.Fatalf("ListOpenOrInProgressEpics failed: %v", err)
	}

	// Should include open and in-progress, not done or blocked
	if len(epics) != 2 {
		t.Errorf("Expected 2 epics, got %d", len(epics))
	}

	ids := make(map[string]bool)
	for _, e := range epics {
		ids[e.ID] = true
	}
	if !ids[epic1] || !ids[epic2] {
		t.Error("Missing expected epics")
	}
}

func TestClient_GetIncompleteTasks(t *testing.T) {
	client := setupProgClient(t)
	defer client.Close()

	// Create epic with tasks
	epicID, _ := client.CreateEpic("Test Epic", nil)
	task1, _ := client.CreateTask("Open Task", &prog.TaskOptions{ParentID: epicID})
	task2, _ := client.CreateTask("In Progress Task", &prog.TaskOptions{ParentID: epicID})
	task3, _ := client.CreateTask("Done Task", &prog.TaskOptions{ParentID: epicID})
	task4, _ := client.CreateTask("Canceled Task", &prog.TaskOptions{ParentID: epicID})

	_ = client.Start(task2)
	_ = client.Done(task3)
	_ = client.Cancel(task4)

	incomplete, err := client.GetIncompleteTasks(epicID)
	if err != nil {
		t.Fatalf("GetIncompleteTasks failed: %v", err)
	}

	// Should include open and in-progress, not done or canceled
	if len(incomplete) != 2 {
		t.Errorf("Expected 2 incomplete tasks, got %d", len(incomplete))
	}

	ids := make(map[string]bool)
	for _, task := range incomplete {
		ids[task.ID] = true
	}
	if !ids[task1] || !ids[task2] {
		t.Error("Missing expected incomplete tasks")
	}
}

// setupProgClient creates a test prog client with an in-memory database.
func setupProgClient(t *testing.T) *prog.Client {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "session-resume-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := prog.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	if err := db.Init(); err != nil {
		db.Close()
		t.Fatalf("Failed to init database: %v", err)
	}

	return prog.NewClient(db, "testproj")
}
