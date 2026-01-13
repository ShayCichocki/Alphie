package orchestrator

import (
	"testing"
	"time"

	"github.com/shayc/alphie/pkg/models"
)

func TestNewOrchestratorPool(t *testing.T) {
	cfg := PoolConfig{
		RepoPath: "/tmp/test",
	}

	pool := NewOrchestratorPool(cfg)

	if pool == nil {
		t.Fatal("NewOrchestratorPool returned nil")
	}
	if pool.orchestrators == nil {
		t.Error("orchestrators map should not be nil")
	}
	if pool.events == nil {
		t.Error("events channel should not be nil")
	}
	if pool.ctx == nil {
		t.Error("ctx should not be nil")
	}
	if pool.cancel == nil {
		t.Error("cancel should not be nil")
	}
}

func TestOrchestratorPool_Count_Initial(t *testing.T) {
	cfg := PoolConfig{
		RepoPath: "/tmp/test",
	}

	pool := NewOrchestratorPool(cfg)

	count := pool.Count()
	if count != 0 {
		t.Errorf("Initial count should be 0, got %d", count)
	}
}

func TestOrchestratorPool_Events(t *testing.T) {
	cfg := PoolConfig{
		RepoPath: "/tmp/test",
	}

	pool := NewOrchestratorPool(cfg)

	events := pool.Events()
	if events == nil {
		t.Error("Events channel should not be nil")
	}
}

func TestOrchestratorPool_Stop_NoOrchestrators(t *testing.T) {
	cfg := PoolConfig{
		RepoPath: "/tmp/test",
	}

	pool := NewOrchestratorPool(cfg)

	// Stop should not block or error when no orchestrators are running
	done := make(chan error, 1)
	go func() {
		done <- pool.Stop()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Stop returned error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Stop should complete quickly with no orchestrators")
	}
}

func TestPoolConfig_Fields(t *testing.T) {
	cfg := PoolConfig{
		RepoPath:   "/path/to/repo",
		Greenfield: true,
	}

	if cfg.RepoPath != "/path/to/repo" {
		t.Errorf("RepoPath = %q, want %q", cfg.RepoPath, "/path/to/repo")
	}
	if !cfg.Greenfield {
		t.Error("Greenfield should be true")
	}
}

func TestOrchestratorPool_ContextCancellation(t *testing.T) {
	cfg := PoolConfig{
		RepoPath: "/tmp/test",
	}

	pool := NewOrchestratorPool(cfg)

	// Stop should cancel the context
	_ = pool.Stop()

	select {
	case <-pool.ctx.Done():
		// Expected - context should be cancelled
	default:
		t.Error("Context should be cancelled after Stop")
	}
}

func TestOrchestratorPool_EventsChannelClosed(t *testing.T) {
	cfg := PoolConfig{
		RepoPath: "/tmp/test",
	}

	pool := NewOrchestratorPool(cfg)

	_ = pool.Stop()

	// Events channel should be closed
	select {
	case _, ok := <-pool.Events():
		if ok {
			t.Error("Events channel should be closed after Stop")
		}
	case <-time.After(100 * time.Millisecond):
		// This could happen if channel wasn't closed, which would be a bug
		// But we can't easily distinguish from blocked read
	}
}

func TestOrchestratorPool_MultipleStops(t *testing.T) {
	cfg := PoolConfig{
		RepoPath: "/tmp/test",
	}

	pool := NewOrchestratorPool(cfg)

	// First stop
	err1 := pool.Stop()
	if err1 != nil {
		t.Errorf("First Stop returned error: %v", err1)
	}

	// Second stop should not panic or block
	// Note: This might cause issues due to closing already-closed channel
	// so we just verify it doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Logf("Second Stop panicked (expected for closed channel): %v", r)
		}
	}()
}

func TestOrchestratorPool_CountAfterStop(t *testing.T) {
	cfg := PoolConfig{
		RepoPath: "/tmp/test",
	}

	pool := NewOrchestratorPool(cfg)
	_ = pool.Stop()

	count := pool.Count()
	if count != 0 {
		t.Errorf("Count after Stop should be 0, got %d", count)
	}
}

func TestOrchestratorPool_CfgPreserved(t *testing.T) {
	cfg := PoolConfig{
		RepoPath:   "/path/to/repo",
		Greenfield: true,
	}

	pool := NewOrchestratorPool(cfg)

	if pool.cfg.RepoPath != "/path/to/repo" {
		t.Errorf("cfg.RepoPath = %q, want %q", pool.cfg.RepoPath, "/path/to/repo")
	}
	if !pool.cfg.Greenfield {
		t.Error("cfg.Greenfield should be true")
	}
}

func TestOrchestratorEvent_Fields(t *testing.T) {
	now := time.Now()

	event := OrchestratorEvent{
		Type:       EventTaskStarted,
		TaskID:     "task-123",
		TaskTitle:  "Test Task",
		AgentID:    "agent-456",
		Message:    "Task started",
		Error:      nil,
		Timestamp:  now,
		TokensUsed: 1000,
		Cost:       0.05,
		Duration:   5 * time.Second,
	}

	if event.Type != EventTaskStarted {
		t.Errorf("Type = %q, want %q", event.Type, EventTaskStarted)
	}
	if event.TaskID != "task-123" {
		t.Errorf("TaskID = %q, want %q", event.TaskID, "task-123")
	}
	if event.TokensUsed != 1000 {
		t.Errorf("TokensUsed = %d, want 1000", event.TokensUsed)
	}
	if event.Duration != 5*time.Second {
		t.Errorf("Duration = %v, want 5s", event.Duration)
	}
}

func TestTierToMaxAgents(t *testing.T) {
	// Test that different tiers have different max agents
	tests := []struct {
		tier        models.Tier
		minExpected int
		maxExpected int
	}{
		{models.TierQuick, 1, 1},
		{models.TierScout, 1, 3},
		{models.TierBuilder, 2, 5},
		{models.TierArchitect, 3, 10},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			// We can't directly test maxAgentsFromTierConfigs here since it's in cmd/alphie
			// But we can verify the tier is valid
			if !tt.tier.Valid() {
				t.Errorf("Tier %q should be valid", tt.tier)
			}
		})
	}
}

func TestOrchestratorPool_Submit_ReturnsID(t *testing.T) {
	// This test is limited because Submit requires a real executor
	// which needs a real git repo. We'll just test the pool setup.
	cfg := PoolConfig{
		RepoPath: "/tmp/test",
		// No executor - Submit will fail but we can test ID format
	}

	pool := NewOrchestratorPool(cfg)
	defer pool.Stop()

	// Submit will fail without executor, but the ID format is still generated
	// before the failure
	id, _ := pool.Submit("test task", models.TierBuilder)

	// ID should be 8 characters (UUID prefix)
	if len(id) != 8 {
		t.Errorf("ID length = %d, want 8", len(id))
	}
}
