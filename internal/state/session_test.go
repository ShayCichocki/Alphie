package state

import (
	"testing"
	"time"
)

// Session CRUD Tests

func TestCreateSession(t *testing.T) {
	db := setupTestDB(t)

	session := &Session{
		ID:          "sess-001",
		RootTask:    "task-root",
		Tier:        "premium",
		TokenBudget: 10000,
		TokensUsed:  0,
		StartedAt:   time.Now(),
		Status:      SessionActive,
	}

	err := db.CreateSession(session)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Verify it was created
	got, err := db.GetSession("sess-001")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetSession returned nil")
	}
	if got.ID != session.ID || got.RootTask != session.RootTask || got.Tier != session.Tier {
		t.Errorf("session mismatch: got %+v, want %+v", got, session)
	}
}

func TestGetSession(t *testing.T) {
	db := setupTestDB(t)

	// Create a session
	session := &Session{
		ID:          "sess-get-001",
		RootTask:    "task-1",
		Tier:        "standard",
		TokenBudget: 5000,
		TokensUsed:  1000,
		StartedAt:   time.Now(),
		Status:      SessionActive,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Get existing session
	got, err := db.GetSession("sess-get-001")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected session, got nil")
	}
	if got.TokenBudget != 5000 || got.TokensUsed != 1000 {
		t.Errorf("token values mismatch: budget=%d, used=%d", got.TokenBudget, got.TokensUsed)
	}

	// Get non-existing session
	got, err = db.GetSession("nonexistent")
	if err != nil {
		t.Fatalf("GetSession failed for nonexistent: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent session, got %+v", got)
	}
}

func TestUpdateSession(t *testing.T) {
	db := setupTestDB(t)

	// Create a session
	session := &Session{
		ID:          "sess-update",
		RootTask:    "task-1",
		Tier:        "standard",
		TokenBudget: 5000,
		TokensUsed:  0,
		StartedAt:   time.Now(),
		Status:      SessionActive,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Update session
	session.TokensUsed = 2500
	session.Status = SessionCompleted
	if err := db.UpdateSession(session); err != nil {
		t.Fatalf("UpdateSession failed: %v", err)
	}

	// Verify update
	got, err := db.GetSession("sess-update")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got.TokensUsed != 2500 {
		t.Errorf("TokensUsed = %d, want 2500", got.TokensUsed)
	}
	if got.Status != SessionCompleted {
		t.Errorf("Status = %s, want %s", got.Status, SessionCompleted)
	}
}

func TestDeleteSession(t *testing.T) {
	db := setupTestDB(t)

	// Create a session
	session := &Session{
		ID:          "sess-delete",
		RootTask:    "task-1",
		Tier:        "standard",
		TokenBudget: 5000,
		TokensUsed:  0,
		StartedAt:   time.Now(),
		Status:      SessionActive,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Delete session
	if err := db.DeleteSession("sess-delete"); err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	// Verify deletion
	got, err := db.GetSession("sess-delete")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}

func TestListSessions(t *testing.T) {
	db := setupTestDB(t)

	// Create multiple sessions
	sessions := []*Session{
		{ID: "sess-list-1", RootTask: "task-1", Tier: "standard", TokenBudget: 5000, StartedAt: time.Now().Add(-2 * time.Hour), Status: SessionActive},
		{ID: "sess-list-2", RootTask: "task-2", Tier: "premium", TokenBudget: 10000, StartedAt: time.Now().Add(-1 * time.Hour), Status: SessionCompleted},
		{ID: "sess-list-3", RootTask: "task-3", Tier: "standard", TokenBudget: 5000, StartedAt: time.Now(), Status: SessionActive},
	}
	for _, s := range sessions {
		if err := db.CreateSession(s); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
	}

	// List all sessions
	all, err := db.ListSessions(nil)
	if err != nil {
		t.Fatalf("ListSessions(nil) failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListSessions(nil) returned %d sessions, want 3", len(all))
	}

	// List active sessions only
	active := SessionActive
	activeList, err := db.ListSessions(&active)
	if err != nil {
		t.Fatalf("ListSessions(active) failed: %v", err)
	}
	if len(activeList) != 2 {
		t.Errorf("ListSessions(active) returned %d sessions, want 2", len(activeList))
	}

	// List completed sessions
	completed := SessionCompleted
	completedList, err := db.ListSessions(&completed)
	if err != nil {
		t.Fatalf("ListSessions(completed) failed: %v", err)
	}
	if len(completedList) != 1 {
		t.Errorf("ListSessions(completed) returned %d sessions, want 1", len(completedList))
	}
}

func TestGetActiveSession(t *testing.T) {
	db := setupTestDB(t)

	// No active session
	got, err := db.GetActiveSession()
	if err != nil {
		t.Fatalf("GetActiveSession failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil when no active session, got %+v", got)
	}

	// Create an active session
	session := &Session{
		ID:          "sess-active",
		RootTask:    "task-1",
		Tier:        "standard",
		TokenBudget: 5000,
		StartedAt:   time.Now(),
		Status:      SessionActive,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	got, err = db.GetActiveSession()
	if err != nil {
		t.Fatalf("GetActiveSession failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected active session, got nil")
	}
	if got.ID != "sess-active" {
		t.Errorf("GetActiveSession returned %s, want sess-active", got.ID)
	}
}

func TestPurgeOldSessions(t *testing.T) {
	db := setupTestDB(t)

	// Create sessions with different ages
	now := time.Now()
	sessions := []*Session{
		{ID: "sess-recent-1", RootTask: "task-1", Tier: "standard", TokenBudget: 5000, StartedAt: now.Add(-1 * 24 * time.Hour), Status: SessionCompleted},                // 1 day old
		{ID: "sess-recent-2", RootTask: "task-2", Tier: "standard", TokenBudget: 5000, StartedAt: now.Add(-15 * 24 * time.Hour), Status: SessionCompleted},               // 15 days old
		{ID: "sess-old-1", RootTask: "task-3", Tier: "standard", TokenBudget: 5000, StartedAt: now.Add(-31 * 24 * time.Hour), Status: SessionCompleted},                  // 31 days old
		{ID: "sess-old-2", RootTask: "task-4", Tier: "standard", TokenBudget: 5000, StartedAt: now.Add(-60 * 24 * time.Hour), Status: SessionFailed},                     // 60 days old
		{ID: "sess-very-old", RootTask: "task-5", Tier: "standard", TokenBudget: 5000, StartedAt: now.Add(-365 * 24 * time.Hour), Status: SessionCanceled},               // 1 year old
		{ID: "sess-active-old", RootTask: "task-6", Tier: "standard", TokenBudget: 5000, StartedAt: now.Add(-45 * 24 * time.Hour), Status: SessionActive},                // 45 days old (active but old)
	}
	for _, s := range sessions {
		if err := db.CreateSession(s); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
	}

	// Verify all sessions exist
	all, err := db.ListSessions(nil)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(all) != 6 {
		t.Fatalf("expected 6 sessions before purge, got %d", len(all))
	}

	// Purge sessions older than 30 days
	purged, err := db.PurgeOldSessions(30 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("PurgeOldSessions failed: %v", err)
	}
	if purged != 4 {
		t.Errorf("expected 4 sessions purged, got %d", purged)
	}

	// Verify remaining sessions
	remaining, err := db.ListSessions(nil)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(remaining) != 2 {
		t.Errorf("expected 2 sessions remaining, got %d", len(remaining))
	}

	// Verify the correct sessions remain (recent ones)
	ids := make(map[string]bool)
	for _, s := range remaining {
		ids[s.ID] = true
	}
	if !ids["sess-recent-1"] || !ids["sess-recent-2"] {
		t.Errorf("unexpected remaining sessions: %v", ids)
	}
}

func TestPurgeOldSessions_NoOldSessions(t *testing.T) {
	db := setupTestDB(t)

	// Create only recent sessions
	now := time.Now()
	session := &Session{
		ID:          "sess-recent",
		RootTask:    "task-1",
		Tier:        "standard",
		TokenBudget: 5000,
		StartedAt:   now.Add(-1 * 24 * time.Hour), // 1 day old
		Status:      SessionCompleted,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Purge sessions older than 30 days
	purged, err := db.PurgeOldSessions(30 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("PurgeOldSessions failed: %v", err)
	}
	if purged != 0 {
		t.Errorf("expected 0 sessions purged, got %d", purged)
	}

	// Verify session still exists
	remaining, err := db.ListSessions(nil)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("expected 1 session remaining, got %d", len(remaining))
	}
}

func TestPurgeOldSessions_EmptyDB(t *testing.T) {
	db := setupTestDB(t)

	// Purge on empty database
	purged, err := db.PurgeOldSessions(30 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("PurgeOldSessions failed: %v", err)
	}
	if purged != 0 {
		t.Errorf("expected 0 sessions purged on empty db, got %d", purged)
	}
}

// Agent CRUD Tests

func TestCreateAgent(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	agent := &Agent{
		ID:           "agent-001",
		TaskID:       "task-001",
		Status:       AgentPending,
		WorktreePath: "/path/to/worktree",
		PID:          12345,
		StartedAt:    &now,
		TokensUsed:   500,
		Cost:         0.05,
		RalphIter:    1,
		RalphScore:   85,
	}

	err := db.CreateAgent(agent)
	if err != nil {
		t.Fatalf("CreateAgent failed: %v", err)
	}

	// Verify
	got, err := db.GetAgent("agent-001")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetAgent returned nil")
	}
	if got.TaskID != "task-001" || got.PID != 12345 {
		t.Errorf("agent mismatch: got %+v", got)
	}
}

func TestCreateAgent_NilStartedAt(t *testing.T) {
	db := setupTestDB(t)

	agent := &Agent{
		ID:         "agent-nil-time",
		TaskID:     "task-001",
		Status:     AgentPending,
		StartedAt:  nil,
		TokensUsed: 0,
	}

	err := db.CreateAgent(agent)
	if err != nil {
		t.Fatalf("CreateAgent failed: %v", err)
	}

	got, err := db.GetAgent("agent-nil-time")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if got.StartedAt != nil {
		t.Errorf("expected nil StartedAt, got %v", got.StartedAt)
	}
}

func TestGetAgent(t *testing.T) {
	db := setupTestDB(t)

	// Create an agent
	agent := &Agent{
		ID:         "agent-get",
		TaskID:     "task-001",
		Status:     AgentRunning,
		TokensUsed: 1000,
	}
	if err := db.CreateAgent(agent); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Get existing
	got, err := db.GetAgent("agent-get")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if got == nil || got.TokensUsed != 1000 {
		t.Errorf("GetAgent mismatch: got %+v", got)
	}

	// Get non-existing
	got, err = db.GetAgent("nonexistent")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent, got %+v", got)
	}
}

func TestUpdateAgent(t *testing.T) {
	db := setupTestDB(t)

	agent := &Agent{
		ID:         "agent-update",
		TaskID:     "task-001",
		Status:     AgentPending,
		TokensUsed: 0,
	}
	if err := db.CreateAgent(agent); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Update status and tokens
	agent.Status = AgentRunning
	agent.TokensUsed = 500
	agent.PID = 9999
	now := time.Now()
	agent.StartedAt = &now

	if err := db.UpdateAgent(agent); err != nil {
		t.Fatalf("UpdateAgent failed: %v", err)
	}

	got, err := db.GetAgent("agent-update")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if got.Status != AgentRunning {
		t.Errorf("Status = %s, want %s", got.Status, AgentRunning)
	}
	if got.TokensUsed != 500 {
		t.Errorf("TokensUsed = %d, want 500", got.TokensUsed)
	}
	if got.PID != 9999 {
		t.Errorf("PID = %d, want 9999", got.PID)
	}
}

func TestDeleteAgent(t *testing.T) {
	db := setupTestDB(t)

	agent := &Agent{
		ID:     "agent-delete",
		TaskID: "task-001",
		Status: AgentPending,
	}
	if err := db.CreateAgent(agent); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if err := db.DeleteAgent("agent-delete"); err != nil {
		t.Fatalf("DeleteAgent failed: %v", err)
	}

	got, err := db.GetAgent("agent-delete")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}

func TestListAgents(t *testing.T) {
	db := setupTestDB(t)

	agents := []*Agent{
		{ID: "agent-list-1", TaskID: "task-1", Status: AgentPending},
		{ID: "agent-list-2", TaskID: "task-2", Status: AgentRunning},
		{ID: "agent-list-3", TaskID: "task-3", Status: AgentRunning},
		{ID: "agent-list-4", TaskID: "task-4", Status: AgentDone},
	}
	for _, a := range agents {
		if err := db.CreateAgent(a); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
	}

	// List all
	all, err := db.ListAgents(nil)
	if err != nil {
		t.Fatalf("ListAgents(nil) failed: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("ListAgents(nil) returned %d, want 4", len(all))
	}

	// List running only
	running := AgentRunning
	runningList, err := db.ListAgents(&running)
	if err != nil {
		t.Fatalf("ListAgents(running) failed: %v", err)
	}
	if len(runningList) != 2 {
		t.Errorf("ListAgents(running) returned %d, want 2", len(runningList))
	}
}

func TestListAgentsByTask(t *testing.T) {
	db := setupTestDB(t)

	agents := []*Agent{
		{ID: "agent-task-1", TaskID: "shared-task", Status: AgentDone},
		{ID: "agent-task-2", TaskID: "shared-task", Status: AgentRunning},
		{ID: "agent-task-3", TaskID: "other-task", Status: AgentPending},
	}
	for _, a := range agents {
		if err := db.CreateAgent(a); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
	}

	// Query by task
	list, err := db.ListAgentsByTask("shared-task")
	if err != nil {
		t.Fatalf("ListAgentsByTask failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ListAgentsByTask returned %d, want 2", len(list))
	}

	// Query for task with no agents
	list, err = db.ListAgentsByTask("nonexistent-task")
	if err != nil {
		t.Fatalf("ListAgentsByTask failed: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("ListAgentsByTask returned %d, want 0", len(list))
	}
}

// Task CRUD Tests

func TestCreateTask(t *testing.T) {
	db := setupTestDB(t)

	task := &Task{
		ID:          "task-001",
		ParentID:    "parent-001",
		Title:       "Test Task",
		Description: "A test task description",
		Status:      TaskPending,
		DependsOn:   []string{"dep-1", "dep-2"},
		AssignedTo:  "agent-001",
		Tier:        "premium",
		CreatedAt:   time.Now(),
	}

	err := db.CreateTask(task)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	got, err := db.GetTask("task-001")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetTask returned nil")
	}
	if got.Title != "Test Task" {
		t.Errorf("Title = %s, want Test Task", got.Title)
	}
	if len(got.DependsOn) != 2 {
		t.Errorf("DependsOn len = %d, want 2", len(got.DependsOn))
	}
}

func TestGetTask(t *testing.T) {
	db := setupTestDB(t)

	task := &Task{
		ID:        "task-get",
		Title:     "Get Test",
		Status:    TaskInProgress,
		CreatedAt: time.Now(),
	}
	if err := db.CreateTask(task); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	got, err := db.GetTask("task-get")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if got == nil || got.Status != TaskInProgress {
		t.Errorf("GetTask mismatch: got %+v", got)
	}

	// Non-existing
	got, err = db.GetTask("nonexistent")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent, got %+v", got)
	}
}

func TestUpdateTask(t *testing.T) {
	db := setupTestDB(t)

	task := &Task{
		ID:        "task-update",
		Title:     "Update Test",
		Status:    TaskPending,
		CreatedAt: time.Now(),
	}
	if err := db.CreateTask(task); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Update
	task.Status = TaskDone
	now := time.Now()
	task.CompletedAt = &now
	task.Description = "Updated description"

	if err := db.UpdateTask(task); err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	got, err := db.GetTask("task-update")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if got.Status != TaskDone {
		t.Errorf("Status = %s, want %s", got.Status, TaskDone)
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt should not be nil")
	}
	if got.Description != "Updated description" {
		t.Errorf("Description = %s, want Updated description", got.Description)
	}
}

func TestDeleteTask(t *testing.T) {
	db := setupTestDB(t)

	task := &Task{
		ID:        "task-delete",
		Title:     "Delete Test",
		Status:    TaskPending,
		CreatedAt: time.Now(),
	}
	if err := db.CreateTask(task); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if err := db.DeleteTask("task-delete"); err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}

	got, err := db.GetTask("task-delete")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}

func TestListTasks(t *testing.T) {
	db := setupTestDB(t)

	tasks := []*Task{
		{ID: "task-list-1", Title: "Task 1", Status: TaskPending, CreatedAt: time.Now()},
		{ID: "task-list-2", Title: "Task 2", Status: TaskInProgress, CreatedAt: time.Now()},
		{ID: "task-list-3", Title: "Task 3", Status: TaskDone, CreatedAt: time.Now()},
	}
	for _, task := range tasks {
		if err := db.CreateTask(task); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
	}

	// List all
	all, err := db.ListTasks(nil)
	if err != nil {
		t.Fatalf("ListTasks(nil) failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListTasks(nil) returned %d, want 3", len(all))
	}

	// List pending
	pending := TaskPending
	pendingList, err := db.ListTasks(&pending)
	if err != nil {
		t.Fatalf("ListTasks(pending) failed: %v", err)
	}
	if len(pendingList) != 1 {
		t.Errorf("ListTasks(pending) returned %d, want 1", len(pendingList))
	}
}

func TestListTasksByParent(t *testing.T) {
	db := setupTestDB(t)

	tasks := []*Task{
		{ID: "child-1", ParentID: "parent-001", Title: "Child 1", Status: TaskPending, CreatedAt: time.Now()},
		{ID: "child-2", ParentID: "parent-001", Title: "Child 2", Status: TaskPending, CreatedAt: time.Now()},
		{ID: "child-3", ParentID: "parent-002", Title: "Child 3", Status: TaskPending, CreatedAt: time.Now()},
	}
	for _, task := range tasks {
		if err := db.CreateTask(task); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
	}

	list, err := db.ListTasksByParent("parent-001")
	if err != nil {
		t.Fatalf("ListTasksByParent failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ListTasksByParent returned %d, want 2", len(list))
	}
}

func TestListReadyTasks(t *testing.T) {
	db := setupTestDB(t)

	// Create dependency task (done)
	depTask := &Task{
		ID:        "dep-done",
		Title:     "Done Dependency",
		Status:    TaskDone,
		CreatedAt: time.Now(),
	}
	if err := db.CreateTask(depTask); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Create tasks
	tasks := []*Task{
		{ID: "ready-1", Title: "Ready (no deps)", Status: TaskPending, CreatedAt: time.Now()},
		{ID: "ready-2", Title: "Ready (dep done)", Status: TaskPending, DependsOn: []string{"dep-done"}, CreatedAt: time.Now()},
		{ID: "blocked", Title: "Blocked (dep not done)", Status: TaskPending, DependsOn: []string{"nonexistent"}, CreatedAt: time.Now()},
		{ID: "in-progress", Title: "In Progress", Status: TaskInProgress, CreatedAt: time.Now()},
	}
	for _, task := range tasks {
		if err := db.CreateTask(task); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
	}

	ready, err := db.ListReadyTasks()
	if err != nil {
		t.Fatalf("ListReadyTasks failed: %v", err)
	}

	// Should return ready-1 and ready-2 (both pending with satisfied deps)
	if len(ready) != 2 {
		t.Errorf("ListReadyTasks returned %d, want 2", len(ready))
	}

	// Verify we got the right tasks
	ids := make(map[string]bool)
	for _, r := range ready {
		ids[r.ID] = true
	}
	if !ids["ready-1"] || !ids["ready-2"] {
		t.Errorf("unexpected ready tasks: %v", ids)
	}
}
