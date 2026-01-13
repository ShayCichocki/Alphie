package state

import (
	"os"
	"testing"
	"time"
)

func TestNewRecoveryManager(t *testing.T) {
	db := setupTestDB(t)
	rm := NewRecoveryManager(db)
	if rm == nil {
		t.Fatal("NewRecoveryManager returned nil")
	}
	if rm.db != db {
		t.Error("RecoveryManager.db not set correctly")
	}
}

func TestCheckForInterrupted_NoSessions(t *testing.T) {
	db := setupTestDB(t)
	rm := NewRecoveryManager(db)

	interrupted, err := rm.CheckForInterrupted()
	if err != nil {
		t.Fatalf("CheckForInterrupted failed: %v", err)
	}
	if interrupted != nil {
		t.Errorf("expected nil when no sessions, got %+v", interrupted)
	}
}

func TestCheckForInterrupted_CompletedSession(t *testing.T) {
	db := setupTestDB(t)
	rm := NewRecoveryManager(db)

	// Create completed session
	session := &Session{
		ID:        "sess-complete",
		RootTask:  "task-1",
		Tier:      "standard",
		StartedAt: time.Now(),
		Status:    SessionCompleted,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	interrupted, err := rm.CheckForInterrupted()
	if err != nil {
		t.Fatalf("CheckForInterrupted failed: %v", err)
	}
	if interrupted != nil {
		t.Errorf("expected nil for completed session, got %+v", interrupted)
	}
}

func TestCheckForInterrupted_FailedSession(t *testing.T) {
	db := setupTestDB(t)
	rm := NewRecoveryManager(db)

	// Create failed session
	session := &Session{
		ID:        "sess-failed",
		RootTask:  "task-1",
		Tier:      "standard",
		StartedAt: time.Now(),
		Status:    SessionFailed,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	interrupted, err := rm.CheckForInterrupted()
	if err != nil {
		t.Fatalf("CheckForInterrupted failed: %v", err)
	}
	if interrupted != nil {
		t.Errorf("expected nil for failed session, got %+v", interrupted)
	}
}

func TestCheckForInterrupted_CanceledSession(t *testing.T) {
	db := setupTestDB(t)
	rm := NewRecoveryManager(db)

	// Create canceled session
	session := &Session{
		ID:        "sess-canceled",
		RootTask:  "task-1",
		Tier:      "standard",
		StartedAt: time.Now(),
		Status:    SessionCanceled,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	interrupted, err := rm.CheckForInterrupted()
	if err != nil {
		t.Fatalf("CheckForInterrupted failed: %v", err)
	}
	if interrupted != nil {
		t.Errorf("expected nil for canceled session, got %+v", interrupted)
	}
}

func TestCheckForInterrupted_ActiveSession(t *testing.T) {
	db := setupTestDB(t)
	rm := NewRecoveryManager(db)

	startTime := time.Now().Add(-1 * time.Hour)
	session := &Session{
		ID:        "sess-active",
		RootTask:  "task-1",
		Tier:      "standard",
		StartedAt: startTime,
		Status:    SessionActive,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	interrupted, err := rm.CheckForInterrupted()
	if err != nil {
		t.Fatalf("CheckForInterrupted failed: %v", err)
	}
	if interrupted == nil {
		t.Fatal("expected interrupted session info, got nil")
	}
	if interrupted.SessionID != "sess-active" {
		t.Errorf("SessionID = %s, want sess-active", interrupted.SessionID)
	}
	if interrupted.Status != "active" {
		t.Errorf("Status = %s, want active", interrupted.Status)
	}
}

func TestCheckForInterrupted_WithRunningAgents(t *testing.T) {
	db := setupTestDB(t)
	rm := NewRecoveryManager(db)

	// Create active session
	session := &Session{
		ID:        "sess-with-agents",
		RootTask:  "task-1",
		Tier:      "standard",
		StartedAt: time.Now().Add(-1 * time.Hour),
		Status:    SessionActive,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Create running agent with non-existent PID (orphaned)
	agentTime := time.Now().Add(-30 * time.Minute)
	agent := &Agent{
		ID:        "agent-orphan",
		TaskID:    "task-1",
		Status:    AgentRunning,
		PID:       999999, // Non-existent PID
		StartedAt: &agentTime,
	}
	if err := db.CreateAgent(agent); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	interrupted, err := rm.CheckForInterrupted()
	if err != nil {
		t.Fatalf("CheckForInterrupted failed: %v", err)
	}
	if interrupted == nil {
		t.Fatal("expected interrupted session info")
	}
	if interrupted.RunningAgents != 1 {
		t.Errorf("RunningAgents = %d, want 1", interrupted.RunningAgents)
	}
	// Last activity should be agent start time
	if interrupted.LastActivity.Before(agentTime.Add(-time.Second)) {
		t.Errorf("LastActivity = %v, should be around %v", interrupted.LastActivity, agentTime)
	}
}

func TestResume_NonExistentSession(t *testing.T) {
	db := setupTestDB(t)
	rm := NewRecoveryManager(db)

	err := rm.Resume("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent session")
	}
}

func TestResume_ResetsOrphanedAgents(t *testing.T) {
	db := setupTestDB(t)
	rm := NewRecoveryManager(db)

	// Create session
	session := &Session{
		ID:        "sess-resume",
		RootTask:  "task-1",
		Tier:      "standard",
		StartedAt: time.Now(),
		Status:    SessionActive,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Create orphaned agent (running with dead PID)
	agent := &Agent{
		ID:     "agent-orphan-resume",
		TaskID: "task-1",
		Status: AgentRunning,
		PID:    999999, // Non-existent
	}
	if err := db.CreateAgent(agent); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Resume
	if err := rm.Resume("sess-resume"); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}

	// Check agent was reset to pending
	got, err := db.GetAgent("agent-orphan-resume")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if got.Status != AgentPending {
		t.Errorf("agent status = %s, want %s", got.Status, AgentPending)
	}
	if got.PID != 0 {
		t.Errorf("agent PID = %d, want 0", got.PID)
	}
}

func TestClean_NonExistentSession(t *testing.T) {
	db := setupTestDB(t)
	rm := NewRecoveryManager(db)

	err := rm.Clean("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent session")
	}
}

func TestClean_MarksSessionFailed(t *testing.T) {
	db := setupTestDB(t)
	rm := NewRecoveryManager(db)

	session := &Session{
		ID:        "sess-clean",
		RootTask:  "task-1",
		Tier:      "standard",
		StartedAt: time.Now(),
		Status:    SessionActive,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if err := rm.Clean("sess-clean"); err != nil {
		t.Fatalf("Clean failed: %v", err)
	}

	got, err := db.GetSession("sess-clean")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got.Status != SessionFailed {
		t.Errorf("session status = %s, want %s", got.Status, SessionFailed)
	}
}

func TestClean_FailsRunningAgents(t *testing.T) {
	db := setupTestDB(t)
	rm := NewRecoveryManager(db)

	session := &Session{
		ID:        "sess-clean-agents",
		RootTask:  "task-1",
		Tier:      "standard",
		StartedAt: time.Now(),
		Status:    SessionActive,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	agent := &Agent{
		ID:     "agent-to-fail",
		TaskID: "task-1",
		Status: AgentRunning,
		PID:    999999,
	}
	if err := db.CreateAgent(agent); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if err := rm.Clean("sess-clean-agents"); err != nil {
		t.Fatalf("Clean failed: %v", err)
	}

	got, err := db.GetAgent("agent-to-fail")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if got.Status != AgentFailed {
		t.Errorf("agent status = %s, want %s", got.Status, AgentFailed)
	}
	if got.PID != 0 {
		t.Errorf("agent PID = %d, want 0", got.PID)
	}
}

// Legacy API Tests
// Note: Some legacy functions (CheckRecovery, RecoverSession) call git worktree
// commands which may fail in test environments. We test them with appropriate
// error handling.

func TestCheckRecovery_WithActiveSession(t *testing.T) {
	db := setupTestDB(t)

	session := &Session{
		ID:        "sess-recovery",
		RootTask:  "task-1",
		Tier:      "standard",
		StartedAt: time.Now(),
		Status:    SessionActive,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	info, err := db.CheckRecovery()
	if err != nil {
		t.Fatalf("CheckRecovery failed: %v", err)
	}
	if info == nil {
		t.Fatal("expected recovery info, got nil")
	}
	if info.Session == nil || info.Session.ID != "sess-recovery" {
		t.Errorf("unexpected session: %+v", info.Session)
	}
}

func TestCheckRecovery_WithOrphanedAgents(t *testing.T) {
	db := setupTestDB(t)

	// Create running agent with dead PID
	agent := &Agent{
		ID:     "orphan-agent",
		TaskID: "task-1",
		Status: AgentRunning,
		PID:    999999, // Non-existent
	}
	if err := db.CreateAgent(agent); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	info, err := db.CheckRecovery()
	if err != nil {
		t.Fatalf("CheckRecovery failed: %v", err)
	}
	if info == nil {
		t.Fatal("expected recovery info, got nil")
	}
	if len(info.OrphanedAgents) != 1 {
		t.Errorf("OrphanedAgents = %d, want 1", len(info.OrphanedAgents))
	}
}

func TestIsProcessAlive(t *testing.T) {
	// Test with invalid PID
	if isProcessAlive(0) {
		t.Error("PID 0 should not be alive")
	}
	if isProcessAlive(-1) {
		t.Error("PID -1 should not be alive")
	}

	// Test with our own PID (should be alive)
	if !isProcessAlive(os.Getpid()) {
		t.Error("our own PID should be alive")
	}

	// Test with non-existent PID
	if isProcessAlive(999999) {
		t.Error("non-existent PID should not be alive")
	}
}

func TestWorktreeBasePath(t *testing.T) {
	// Save and restore env
	original := os.Getenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", original)

	// Test with XDG_CACHE_HOME set
	os.Setenv("XDG_CACHE_HOME", "/custom/cache")
	path := WorktreeBasePath()
	expected := "/custom/cache/alphie/worktrees"
	if path != expected {
		t.Errorf("WorktreeBasePath() = %q, want %q", path, expected)
	}

	// Test without XDG_CACHE_HOME
	os.Unsetenv("XDG_CACHE_HOME")
	path = WorktreeBasePath()
	home, _ := os.UserHomeDir()
	expected = home + "/.cache/alphie/worktrees"
	if path != expected {
		t.Errorf("WorktreeBasePath() = %q, want %q", path, expected)
	}
}

func TestAgentWorktreePath(t *testing.T) {
	// Save and restore env
	original := os.Getenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", original)

	os.Setenv("XDG_CACHE_HOME", "/cache")
	path := AgentWorktreePath("test-agent")
	expected := "/cache/alphie/worktrees/agent-test-agent"
	if path != expected {
		t.Errorf("AgentWorktreePath() = %q, want %q", path, expected)
	}
}

func TestEnsureWorktreeDir(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("XDG_CACHE_HOME", dir)
	defer os.Unsetenv("XDG_CACHE_HOME")

	err := EnsureWorktreeDir()
	if err != nil {
		t.Fatalf("EnsureWorktreeDir failed: %v", err)
	}

	expected := dir + "/alphie/worktrees"
	if _, err := os.Stat(expected); os.IsNotExist(err) {
		t.Errorf("worktree dir not created: %s", expected)
	}
}

func TestInterruptedSession_Fields(t *testing.T) {
	// Test that InterruptedSession struct fields are accessible
	now := time.Now()
	is := &InterruptedSession{
		SessionID:     "test-session",
		StartedAt:     now,
		LastActivity:  now,
		RunningAgents: 5,
		Status:        "active",
	}

	if is.SessionID != "test-session" {
		t.Errorf("SessionID = %s, want test-session", is.SessionID)
	}
	if is.RunningAgents != 5 {
		t.Errorf("RunningAgents = %d, want 5", is.RunningAgents)
	}
	if is.Status != "active" {
		t.Errorf("Status = %s, want active", is.Status)
	}
}

func TestRecoveryInfo_Fields(t *testing.T) {
	// Test that RecoveryInfo struct fields are accessible
	info := &RecoveryInfo{
		Session:           &Session{ID: "test"},
		OrphanedAgents:    []Agent{{ID: "agent-1"}},
		OrphanedWorktrees: []string{"/path/to/worktree"},
		StaleProcesses:    []int{1234},
	}

	if info.Session.ID != "test" {
		t.Errorf("Session.ID = %s, want test", info.Session.ID)
	}
	if len(info.OrphanedAgents) != 1 {
		t.Errorf("OrphanedAgents len = %d, want 1", len(info.OrphanedAgents))
	}
	if len(info.OrphanedWorktrees) != 1 {
		t.Errorf("OrphanedWorktrees len = %d, want 1", len(info.OrphanedWorktrees))
	}
	if len(info.StaleProcesses) != 1 {
		t.Errorf("StaleProcesses len = %d, want 1", len(info.StaleProcesses))
	}
}
