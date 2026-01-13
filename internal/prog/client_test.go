package prog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClient_CreateEpic(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	id, err := client.CreateEpic("Test Epic", nil)
	if err != nil {
		t.Fatalf("CreateEpic failed: %v", err)
	}

	if id == "" {
		t.Fatal("CreateEpic returned empty ID")
	}
	if id[:3] != "ep-" {
		t.Errorf("Expected epic ID prefix 'ep-', got %s", id[:3])
	}

	// Verify item was created
	item, err := client.GetItem(id)
	if err != nil {
		t.Fatalf("GetItem failed: %v", err)
	}
	if item.Title != "Test Epic" {
		t.Errorf("Expected title 'Test Epic', got %q", item.Title)
	}
	if item.Type != ItemTypeEpic {
		t.Errorf("Expected type 'epic', got %q", item.Type)
	}
	if item.Project != "testproj" {
		t.Errorf("Expected project 'testproj', got %q", item.Project)
	}
}

func TestClient_CreateEpicWithOptions(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	opts := &EpicOptions{
		Description: "Epic description",
		Priority:    1,
	}
	id, err := client.CreateEpic("High Priority Epic", opts)
	if err != nil {
		t.Fatalf("CreateEpic failed: %v", err)
	}

	item, err := client.GetItem(id)
	if err != nil {
		t.Fatalf("GetItem failed: %v", err)
	}
	if item.Description != "Epic description" {
		t.Errorf("Expected description 'Epic description', got %q", item.Description)
	}
	if item.Priority != 1 {
		t.Errorf("Expected priority 1, got %d", item.Priority)
	}
}

func TestClient_CreateTask(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	id, err := client.CreateTask("Test Task", nil)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	if id == "" {
		t.Fatal("CreateTask returned empty ID")
	}
	if id[:3] != "ts-" {
		t.Errorf("Expected task ID prefix 'ts-', got %s", id[:3])
	}

	item, err := client.GetItem(id)
	if err != nil {
		t.Fatalf("GetItem failed: %v", err)
	}
	if item.Title != "Test Task" {
		t.Errorf("Expected title 'Test Task', got %q", item.Title)
	}
	if item.Type != ItemTypeTask {
		t.Errorf("Expected type 'task', got %q", item.Type)
	}
}

func TestClient_CreateTaskWithParent(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	// Create parent epic
	epicID, err := client.CreateEpic("Parent Epic", nil)
	if err != nil {
		t.Fatalf("CreateEpic failed: %v", err)
	}

	// Create task with parent
	opts := &TaskOptions{
		ParentID: epicID,
	}
	taskID, err := client.CreateTask("Child Task", opts)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	task, err := client.GetItem(taskID)
	if err != nil {
		t.Fatalf("GetItem failed: %v", err)
	}
	if task.ParentID == nil || *task.ParentID != epicID {
		t.Errorf("Expected parent ID %q, got %v", epicID, task.ParentID)
	}
}

func TestClient_CreateTaskWithDependencies(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	// Create dependency task
	depID, err := client.CreateTask("Dependency Task", nil)
	if err != nil {
		t.Fatalf("CreateTask (dep) failed: %v", err)
	}

	// Create task with dependency
	opts := &TaskOptions{
		DependsOn: []string{depID},
	}
	taskID, err := client.CreateTask("Dependent Task", opts)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Verify dependency
	deps, err := client.GetDependencies(taskID)
	if err != nil {
		t.Fatalf("GetDependencies failed: %v", err)
	}
	if len(deps) != 1 || deps[0] != depID {
		t.Errorf("Expected deps [%s], got %v", depID, deps)
	}
}

func TestClient_UpdateStatus(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	id, err := client.CreateTask("Status Test", nil)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Test Start
	if err := client.Start(id); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	item, _ := client.GetItem(id)
	if item.Status != StatusInProgress {
		t.Errorf("Expected status 'in_progress', got %q", item.Status)
	}

	// Test Done
	if err := client.Done(id); err != nil {
		t.Fatalf("Done failed: %v", err)
	}
	item, _ = client.GetItem(id)
	if item.Status != StatusDone {
		t.Errorf("Expected status 'done', got %q", item.Status)
	}

	// Test Reopen
	if err := client.Reopen(id); err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	item, _ = client.GetItem(id)
	if item.Status != StatusOpen {
		t.Errorf("Expected status 'open', got %q", item.Status)
	}

	// Test Block
	if err := client.Block(id); err != nil {
		t.Fatalf("Block failed: %v", err)
	}
	item, _ = client.GetItem(id)
	if item.Status != StatusBlocked {
		t.Errorf("Expected status 'blocked', got %q", item.Status)
	}

	// Test Cancel
	if err := client.Cancel(id); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}
	item, _ = client.GetItem(id)
	if item.Status != StatusCanceled {
		t.Errorf("Expected status 'canceled', got %q", item.Status)
	}
}

func TestClient_AddLog(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	id, err := client.CreateTask("Log Test", nil)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Add log
	if err := client.AddLog(id, "Test log message"); err != nil {
		t.Fatalf("AddLog failed: %v", err)
	}

	// Get logs
	logs, err := client.GetLogs(id)
	if err != nil {
		t.Fatalf("GetLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("Expected 1 log, got %d", len(logs))
	}
	if logs[0].Message != "Test log message" {
		t.Errorf("Expected message 'Test log message', got %q", logs[0].Message)
	}
}

func TestClient_AddLog_EmptyMessage(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	id, err := client.CreateTask("Log Test", nil)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	err = client.AddLog(id, "")
	if err == nil {
		t.Error("Expected error for empty message, got nil")
	}
}

func TestClient_AddLearning(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	id, err := client.AddLearning("Test learning", nil)
	if err != nil {
		t.Fatalf("AddLearning failed: %v", err)
	}

	if id == "" {
		t.Fatal("AddLearning returned empty ID")
	}
	if id[:4] != "lrn-" {
		t.Errorf("Expected learning ID prefix 'lrn-', got %s", id[:4])
	}

	learning, err := client.GetLearning(id)
	if err != nil {
		t.Fatalf("GetLearning failed: %v", err)
	}
	if learning.Summary != "Test learning" {
		t.Errorf("Expected summary 'Test learning', got %q", learning.Summary)
	}
	if learning.Project != "testproj" {
		t.Errorf("Expected project 'testproj', got %q", learning.Project)
	}
}

func TestClient_AddLearningWithOptions(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	// Create a task to link
	taskID, err := client.CreateTask("Related Task", nil)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	opts := &LearningOptions{
		TaskID:   taskID,
		Detail:   "Detailed explanation",
		Files:    []string{"file1.go", "file2.go"},
		Concepts: []string{"testing", "patterns"},
	}
	id, err := client.AddLearning("Learning with options", opts)
	if err != nil {
		t.Fatalf("AddLearning failed: %v", err)
	}

	learning, err := client.GetLearning(id)
	if err != nil {
		t.Fatalf("GetLearning failed: %v", err)
	}
	if learning.Detail != "Detailed explanation" {
		t.Errorf("Expected detail 'Detailed explanation', got %q", learning.Detail)
	}
	if learning.TaskID == nil || *learning.TaskID != taskID {
		t.Errorf("Expected task ID %q, got %v", taskID, learning.TaskID)
	}
	if len(learning.Files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(learning.Files))
	}
	if len(learning.Concepts) != 2 {
		t.Errorf("Expected 2 concepts, got %d", len(learning.Concepts))
	}
}

func TestClient_AddLearning_RequiresProject(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	// Create client without project
	noProjectClient := NewClient(client.DB(), "")

	_, err := noProjectClient.AddLearning("Test learning", nil)
	if err == nil {
		t.Error("Expected error for learning without project, got nil")
	}
}

func TestClient_ListReadyTasks(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	// Create tasks
	task1, _ := client.CreateTask("Ready Task 1", nil)
	task2, _ := client.CreateTask("Ready Task 2", nil)

	// Create dependency chain
	depTask, _ := client.CreateTask("Blocking Task", nil)
	_, _ = client.CreateTask("Blocked Task", &TaskOptions{DependsOn: []string{depTask}})

	// List ready tasks
	ready, err := client.ListReadyTasks("")
	if err != nil {
		t.Fatalf("ListReadyTasks failed: %v", err)
	}

	// Should have 3 ready tasks (task1, task2, depTask)
	// Blocked task should not be included
	if len(ready) != 3 {
		t.Errorf("Expected 3 ready tasks, got %d", len(ready))
	}

	ids := make(map[string]bool)
	for _, item := range ready {
		ids[item.ID] = true
	}
	if !ids[task1] || !ids[task2] || !ids[depTask] {
		t.Error("Missing expected ready tasks")
	}
}

func TestClient_GetStatus(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	// Create tasks with different statuses
	task1, _ := client.CreateTask("Open Task", nil)
	task2, _ := client.CreateTask("In Progress", nil)
	task3, _ := client.CreateTask("Done Task", nil)

	_ = client.Start(task2)
	_ = client.Done(task3)

	status, err := client.GetStatus("")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if status.Open != 1 {
		t.Errorf("Expected 1 open, got %d", status.Open)
	}
	if status.InProgress != 1 {
		t.Errorf("Expected 1 in_progress, got %d", status.InProgress)
	}
	if status.Done != 1 {
		t.Errorf("Expected 1 done, got %d", status.Done)
	}
	_ = task1 // unused
}

func TestClient_ProjectScope(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	// Create task in default project
	task1, _ := client.CreateTask("Default Project Task", nil)

	// Create task in different project using options
	task2, _ := client.CreateTask("Other Project Task", &TaskOptions{Project: "otherproj"})

	item1, _ := client.GetItem(task1)
	item2, _ := client.GetItem(task2)

	if item1.Project != "testproj" {
		t.Errorf("Expected task1 project 'testproj', got %q", item1.Project)
	}
	if item2.Project != "otherproj" {
		t.Errorf("Expected task2 project 'otherproj', got %q", item2.Project)
	}
}

func TestClient_SetProject(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	if client.Project() != "testproj" {
		t.Errorf("Expected initial project 'testproj', got %q", client.Project())
	}

	client.SetProject("newproj")
	if client.Project() != "newproj" {
		t.Errorf("Expected project 'newproj', got %q", client.Project())
	}
}

func TestClient_AppendDescription(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	opts := &TaskOptions{Description: "Initial"}
	id, _ := client.CreateTask("Desc Test", opts)

	if err := client.AppendDescription(id, "Appended"); err != nil {
		t.Fatalf("AppendDescription failed: %v", err)
	}

	item, _ := client.GetItem(id)
	if item.Description != "Initial\n\nAppended" {
		t.Errorf("Unexpected description: %q", item.Description)
	}
}

func TestClient_SetTitle(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	id, _ := client.CreateTask("Original Title", nil)

	if err := client.SetTitle(id, "New Title"); err != nil {
		t.Fatalf("SetTitle failed: %v", err)
	}

	item, _ := client.GetItem(id)
	if item.Title != "New Title" {
		t.Errorf("Expected title 'New Title', got %q", item.Title)
	}
}

func TestClient_ValidationErrors(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	// Empty epic title
	_, err := client.CreateEpic("", nil)
	if err == nil {
		t.Error("Expected error for empty epic title")
	}

	// Empty task title
	_, err = client.CreateTask("", nil)
	if err == nil {
		t.Error("Expected error for empty task title")
	}

	// Invalid status
	id, _ := client.CreateTask("Test", nil)
	err = client.UpdateStatus(id, "invalid")
	if err == nil {
		t.Error("Expected error for invalid status")
	}

	// Empty learning summary
	_, err = client.AddLearning("", nil)
	if err == nil {
		t.Error("Expected error for empty learning summary")
	}

	// Empty title for SetTitle
	err = client.SetTitle(id, "")
	if err == nil {
		t.Error("Expected error for empty title in SetTitle")
	}
}

// Tests for cross-session continuity features

func TestClient_GetChildTasks(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	// Create epic
	epicID, err := client.CreateEpic("Parent Epic", nil)
	if err != nil {
		t.Fatalf("CreateEpic failed: %v", err)
	}

	// Create tasks under the epic
	task1, _ := client.CreateTask("Task 1", &TaskOptions{ParentID: epicID})
	task2, _ := client.CreateTask("Task 2", &TaskOptions{ParentID: epicID})
	task3, _ := client.CreateTask("Task 3", &TaskOptions{ParentID: epicID})

	// Create a task not under the epic
	_, _ = client.CreateTask("Unrelated Task", nil)

	// Get child tasks
	children, err := client.GetChildTasks(epicID)
	if err != nil {
		t.Fatalf("GetChildTasks failed: %v", err)
	}

	if len(children) != 3 {
		t.Errorf("Expected 3 children, got %d", len(children))
	}

	ids := make(map[string]bool)
	for _, child := range children {
		ids[child.ID] = true
	}
	if !ids[task1] || !ids[task2] || !ids[task3] {
		t.Error("Missing expected child tasks")
	}
}

func TestClient_GetEpic(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	// Create an epic
	epicID, err := client.CreateEpic("Test Epic", nil)
	if err != nil {
		t.Fatalf("CreateEpic failed: %v", err)
	}

	// GetEpic should succeed
	epic, err := client.GetEpic(epicID)
	if err != nil {
		t.Fatalf("GetEpic failed: %v", err)
	}
	if epic.Title != "Test Epic" {
		t.Errorf("Expected title 'Test Epic', got %q", epic.Title)
	}

	// Create a task
	taskID, _ := client.CreateTask("Test Task", nil)

	// GetEpic on a task should fail
	_, err = client.GetEpic(taskID)
	if err == nil {
		t.Error("Expected error getting epic for task ID")
	}
}

func TestClient_FindInProgressEpic(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	// Initially no in-progress epic
	epic, err := client.FindInProgressEpic()
	if err != nil {
		t.Fatalf("FindInProgressEpic failed: %v", err)
	}
	if epic != nil {
		t.Error("Expected no in-progress epic, got one")
	}

	// Create an epic and mark it in-progress
	epicID, _ := client.CreateEpic("In Progress Epic", nil)
	_ = client.Start(epicID)

	// Now should find the in-progress epic
	epic, err = client.FindInProgressEpic()
	if err != nil {
		t.Fatalf("FindInProgressEpic failed: %v", err)
	}
	if epic == nil {
		t.Fatal("Expected to find in-progress epic")
	}
	if epic.ID != epicID {
		t.Errorf("Expected epic ID %s, got %s", epicID, epic.ID)
	}
}

func TestClient_ComputeEpicProgress(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	// Create epic with tasks
	epicID, _ := client.CreateEpic("Progress Epic", nil)
	task1, _ := client.CreateTask("Task 1", &TaskOptions{ParentID: epicID})
	task2, _ := client.CreateTask("Task 2", &TaskOptions{ParentID: epicID})
	task3, _ := client.CreateTask("Task 3", &TaskOptions{ParentID: epicID})

	// Initially 0/3 completed
	completed, total, err := client.ComputeEpicProgress(epicID)
	if err != nil {
		t.Fatalf("ComputeEpicProgress failed: %v", err)
	}
	if completed != 0 || total != 3 {
		t.Errorf("Expected 0/3, got %d/%d", completed, total)
	}

	// Mark one task done
	_ = client.Done(task1)
	completed, total, _ = client.ComputeEpicProgress(epicID)
	if completed != 1 || total != 3 {
		t.Errorf("Expected 1/3, got %d/%d", completed, total)
	}

	// Mark all tasks done
	_ = client.Done(task2)
	_ = client.Done(task3)
	completed, total, _ = client.ComputeEpicProgress(epicID)
	if completed != 3 || total != 3 {
		t.Errorf("Expected 3/3, got %d/%d", completed, total)
	}
}

func TestClient_UpdateEpicStatusIfComplete(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	// Create epic with tasks
	epicID, _ := client.CreateEpic("Complete Epic", nil)
	task1, _ := client.CreateTask("Task 1", &TaskOptions{ParentID: epicID})
	task2, _ := client.CreateTask("Task 2", &TaskOptions{ParentID: epicID})

	// Mark epic as in-progress
	_ = client.Start(epicID)

	// Epic should not be marked done yet
	done, err := client.UpdateEpicStatusIfComplete(epicID)
	if err != nil {
		t.Fatalf("UpdateEpicStatusIfComplete failed: %v", err)
	}
	if done {
		t.Error("Expected epic not to be done yet")
	}

	epic, _ := client.GetItem(epicID)
	if epic.Status != StatusInProgress {
		t.Errorf("Expected status in_progress, got %q", epic.Status)
	}

	// Complete all tasks
	_ = client.Done(task1)
	_ = client.Done(task2)

	// Now epic should be marked done
	done, err = client.UpdateEpicStatusIfComplete(epicID)
	if err != nil {
		t.Fatalf("UpdateEpicStatusIfComplete failed: %v", err)
	}
	if !done {
		t.Error("Expected epic to be marked done")
	}

	epic, _ = client.GetItem(epicID)
	if epic.Status != StatusDone {
		t.Errorf("Expected status done, got %q", epic.Status)
	}
}

func TestClient_UpdateEpicStatusIfComplete_EmptyEpic(t *testing.T) {
	client := setupTestClient(t)
	defer client.Close()

	// Create epic with no tasks
	epicID, _ := client.CreateEpic("Empty Epic", nil)

	// Empty epic should not be marked done
	done, err := client.UpdateEpicStatusIfComplete(epicID)
	if err != nil {
		t.Fatalf("UpdateEpicStatusIfComplete failed: %v", err)
	}
	if done {
		t.Error("Empty epic should not be marked done")
	}
}

func TestClient_ListInProgressEpics(t *testing.T) {
	client := setupTestClient(t)
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
	client := setupTestClient(t)
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
	client := setupTestClient(t)
	defer client.Close()

	// Create epic with tasks
	epicID, _ := client.CreateEpic("Test Epic", nil)
	task1, _ := client.CreateTask("Open Task", &TaskOptions{ParentID: epicID})
	task2, _ := client.CreateTask("In Progress Task", &TaskOptions{ParentID: epicID})
	task3, _ := client.CreateTask("Done Task", &TaskOptions{ParentID: epicID})
	task4, _ := client.CreateTask("Canceled Task", &TaskOptions{ParentID: epicID})

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

// setupTestClient creates a test client with an in-memory database.
func setupTestClient(t *testing.T) *Client {
	t.Helper()

	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "prog-client-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	if err := db.Init(); err != nil {
		db.Close()
		t.Fatalf("Failed to init database: %v", err)
	}

	return NewClient(db, "testproj")
}
