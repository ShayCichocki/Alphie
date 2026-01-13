package agent

import (
	"sync"
	"testing"

	"github.com/shayc/alphie/pkg/models"
)

func TestCanTransition(t *testing.T) {
	tests := []struct {
		name     string
		from     models.AgentStatus
		to       models.AgentStatus
		expected bool
	}{
		// Valid transitions from pending
		{"pending to running", models.AgentStatusPending, models.AgentStatusRunning, true},
		{"pending to failed", models.AgentStatusPending, models.AgentStatusFailed, true},

		// Invalid transitions from pending
		{"pending to paused", models.AgentStatusPending, models.AgentStatusPaused, false},
		{"pending to done", models.AgentStatusPending, models.AgentStatusDone, false},
		{"pending to waiting_approval", models.AgentStatusPending, models.AgentStatusWaitingApproval, false},

		// Valid transitions from running
		{"running to paused", models.AgentStatusRunning, models.AgentStatusPaused, true},
		{"running to waiting_approval", models.AgentStatusRunning, models.AgentStatusWaitingApproval, true},
		{"running to done", models.AgentStatusRunning, models.AgentStatusDone, true},
		{"running to failed", models.AgentStatusRunning, models.AgentStatusFailed, true},

		// Invalid transitions from running
		{"running to pending", models.AgentStatusRunning, models.AgentStatusPending, false},

		// Valid transitions from paused
		{"paused to running", models.AgentStatusPaused, models.AgentStatusRunning, true},
		{"paused to failed", models.AgentStatusPaused, models.AgentStatusFailed, true},

		// Invalid transitions from paused
		{"paused to done", models.AgentStatusPaused, models.AgentStatusDone, false},
		{"paused to pending", models.AgentStatusPaused, models.AgentStatusPending, false},

		// Valid transitions from waiting_approval
		{"waiting_approval to running", models.AgentStatusWaitingApproval, models.AgentStatusRunning, true},
		{"waiting_approval to failed", models.AgentStatusWaitingApproval, models.AgentStatusFailed, true},

		// Invalid transitions from waiting_approval
		{"waiting_approval to done", models.AgentStatusWaitingApproval, models.AgentStatusDone, false},
		{"waiting_approval to paused", models.AgentStatusWaitingApproval, models.AgentStatusPaused, false},

		// Terminal states (no valid transitions)
		{"done to running", models.AgentStatusDone, models.AgentStatusRunning, false},
		{"done to failed", models.AgentStatusDone, models.AgentStatusFailed, false},
		{"done to pending", models.AgentStatusDone, models.AgentStatusPending, false},
		{"failed to running", models.AgentStatusFailed, models.AgentStatusRunning, false},
		{"failed to done", models.AgentStatusFailed, models.AgentStatusDone, false},
		{"failed to pending", models.AgentStatusFailed, models.AgentStatusPending, false},

		// Unknown state
		{"unknown to running", models.AgentStatus("unknown"), models.AgentStatusRunning, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanTransition(tt.from, tt.to)
			if result != tt.expected {
				t.Errorf("CanTransition(%q, %q) = %v, want %v", tt.from, tt.to, result, tt.expected)
			}
		})
	}
}

func TestManagerCreate(t *testing.T) {
	m := NewManager()

	agent, err := m.Create("task-123", "/path/to/worktree")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if agent.ID == "" {
		t.Error("Create() returned agent with empty ID")
	}
	if agent.TaskID != "task-123" {
		t.Errorf("Create() TaskID = %q, want %q", agent.TaskID, "task-123")
	}
	if agent.WorktreePath != "/path/to/worktree" {
		t.Errorf("Create() WorktreePath = %q, want %q", agent.WorktreePath, "/path/to/worktree")
	}
	if agent.Status != models.AgentStatusPending {
		t.Errorf("Create() Status = %q, want %q", agent.Status, models.AgentStatusPending)
	}
}

func TestManagerStartValidTransition(t *testing.T) {
	m := NewManager()

	agent, _ := m.Create("task-123", "/path/to/worktree")
	err := m.Start(agent.ID, 1234)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	status, _ := m.GetStatus(agent.ID)
	if status != models.AgentStatusRunning {
		t.Errorf("GetStatus() = %q, want %q", status, models.AgentStatusRunning)
	}
}

func TestManagerStartInvalidTransition(t *testing.T) {
	m := NewManager()

	agent, _ := m.Create("task-123", "/path/to/worktree")
	_ = m.Start(agent.ID, 1234)
	_ = m.Complete(agent.ID)

	// Try to start from done state (invalid)
	err := m.Start(agent.ID, 1234)
	if err == nil {
		t.Error("Start() from done state should return error")
	}
}

func TestManagerPause(t *testing.T) {
	m := NewManager()

	agent, _ := m.Create("task-123", "/path/to/worktree")
	_ = m.Start(agent.ID, 1234)
	err := m.Pause(agent.ID)
	if err != nil {
		t.Fatalf("Pause() error = %v", err)
	}

	status, _ := m.GetStatus(agent.ID)
	if status != models.AgentStatusPaused {
		t.Errorf("GetStatus() = %q, want %q", status, models.AgentStatusPaused)
	}
}

func TestManagerResume(t *testing.T) {
	m := NewManager()

	agent, _ := m.Create("task-123", "/path/to/worktree")
	_ = m.Start(agent.ID, 1234)
	_ = m.Pause(agent.ID)
	err := m.Resume(agent.ID, 5678)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}

	status, _ := m.GetStatus(agent.ID)
	if status != models.AgentStatusRunning {
		t.Errorf("GetStatus() = %q, want %q", status, models.AgentStatusRunning)
	}
}

func TestManagerWaitApproval(t *testing.T) {
	m := NewManager()

	agent, _ := m.Create("task-123", "/path/to/worktree")
	_ = m.Start(agent.ID, 1234)
	err := m.WaitApproval(agent.ID)
	if err != nil {
		t.Fatalf("WaitApproval() error = %v", err)
	}

	status, _ := m.GetStatus(agent.ID)
	if status != models.AgentStatusWaitingApproval {
		t.Errorf("GetStatus() = %q, want %q", status, models.AgentStatusWaitingApproval)
	}
}

func TestManagerComplete(t *testing.T) {
	m := NewManager()

	agent, _ := m.Create("task-123", "/path/to/worktree")
	_ = m.Start(agent.ID, 1234)
	err := m.Complete(agent.ID)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	status, _ := m.GetStatus(agent.ID)
	if status != models.AgentStatusDone {
		t.Errorf("GetStatus() = %q, want %q", status, models.AgentStatusDone)
	}
}

func TestManagerFail(t *testing.T) {
	m := NewManager()

	agent, _ := m.Create("task-123", "/path/to/worktree")
	_ = m.Start(agent.ID, 1234)
	err := m.Fail(agent.ID, "test error")
	if err != nil {
		t.Fatalf("Fail() error = %v", err)
	}

	status, _ := m.GetStatus(agent.ID)
	if status != models.AgentStatusFailed {
		t.Errorf("GetStatus() = %q, want %q", status, models.AgentStatusFailed)
	}
}

func TestManagerAgentNotFound(t *testing.T) {
	m := NewManager()

	_, err := m.GetStatus("nonexistent")
	if err != ErrAgentNotFound {
		t.Errorf("GetStatus() error = %v, want ErrAgentNotFound", err)
	}

	err = m.Start("nonexistent", 1234)
	if err != ErrAgentNotFound {
		t.Errorf("Start() error = %v, want ErrAgentNotFound", err)
	}
}

func TestManagerGet(t *testing.T) {
	m := NewManager()

	agent, _ := m.Create("task-123", "/path/to/worktree")
	retrieved, err := m.Get(agent.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved.ID != agent.ID {
		t.Errorf("Get() ID = %q, want %q", retrieved.ID, agent.ID)
	}
}

func TestManagerUpdateUsage(t *testing.T) {
	m := NewManager()

	agent, _ := m.Create("task-123", "/path/to/worktree")
	err := m.UpdateUsage(agent.ID, 1000, 0.05)
	if err != nil {
		t.Fatalf("UpdateUsage() error = %v", err)
	}

	retrieved, _ := m.Get(agent.ID)
	if retrieved.TokensUsed != 1000 {
		t.Errorf("TokensUsed = %d, want %d", retrieved.TokensUsed, 1000)
	}
	if retrieved.Cost != 0.05 {
		t.Errorf("Cost = %f, want %f", retrieved.Cost, 0.05)
	}
}

func TestManagerList(t *testing.T) {
	m := NewManager()

	_, _ = m.Create("task-1", "/path/1")
	_, _ = m.Create("task-2", "/path/2")

	agents := m.List()
	if len(agents) != 2 {
		t.Errorf("List() returned %d agents, want 2", len(agents))
	}
}

func TestManagerListByTask(t *testing.T) {
	m := NewManager()

	_, _ = m.Create("task-1", "/path/1")
	_, _ = m.Create("task-1", "/path/2")
	_, _ = m.Create("task-2", "/path/3")

	agents := m.ListByTask("task-1")
	if len(agents) != 2 {
		t.Errorf("ListByTask() returned %d agents, want 2", len(agents))
	}
}

func TestManagerListByStatus(t *testing.T) {
	m := NewManager()

	a1, _ := m.Create("task-1", "/path/1")
	a2, _ := m.Create("task-2", "/path/2")
	_, _ = m.Create("task-3", "/path/3")

	_ = m.Start(a1.ID, 1234)
	_ = m.Start(a2.ID, 5678)

	agents := m.ListByStatus(models.AgentStatusRunning)
	if len(agents) != 2 {
		t.Errorf("ListByStatus() returned %d agents, want 2", len(agents))
	}

	pendingAgents := m.ListByStatus(models.AgentStatusPending)
	if len(pendingAgents) != 1 {
		t.Errorf("ListByStatus(pending) returned %d agents, want 1", len(pendingAgents))
	}
}

func TestManagerRemove(t *testing.T) {
	m := NewManager()

	agent, _ := m.Create("task-123", "/path/to/worktree")
	err := m.Remove(agent.ID)
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	_, err = m.Get(agent.ID)
	if err != ErrAgentNotFound {
		t.Errorf("Get() after Remove() error = %v, want ErrAgentNotFound", err)
	}
}

func TestManagerLoad(t *testing.T) {
	m := NewManager()

	existing := &models.Agent{
		ID:           "existing-id",
		TaskID:       "task-123",
		Status:       models.AgentStatusRunning,
		WorktreePath: "/path/to/worktree",
	}

	err := m.Load(existing)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	retrieved, err := m.Get("existing-id")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if retrieved.Status != models.AgentStatusRunning {
		t.Errorf("Status = %q, want %q", retrieved.Status, models.AgentStatusRunning)
	}
}

func TestManagerLoadDuplicate(t *testing.T) {
	m := NewManager()

	agent, _ := m.Create("task-123", "/path/to/worktree")
	duplicate := &models.Agent{
		ID:     agent.ID,
		TaskID: "task-456",
	}

	err := m.Load(duplicate)
	if err != ErrAgentAlreadyExists {
		t.Errorf("Load() error = %v, want ErrAgentAlreadyExists", err)
	}
}

func TestManagerLifecycleEventHandler(t *testing.T) {
	m := NewManager()

	var receivedEvents []LifecycleEvent
	m.OnEvent(func(e LifecycleEvent) {
		receivedEvents = append(receivedEvents, e)
	})

	agent, _ := m.Create("task-123", "/path/to/worktree")
	_ = m.Start(agent.ID, 1234)
	_ = m.Complete(agent.ID)

	if len(receivedEvents) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(receivedEvents))
	}

	if receivedEvents[0].Type != LifecycleEventCreated {
		t.Errorf("Event[0].Type = %q, want %q", receivedEvents[0].Type, LifecycleEventCreated)
	}
	if receivedEvents[1].Type != LifecycleEventStarted {
		t.Errorf("Event[1].Type = %q, want %q", receivedEvents[1].Type, LifecycleEventStarted)
	}
	if receivedEvents[2].Type != LifecycleEventCompleted {
		t.Errorf("Event[2].Type = %q, want %q", receivedEvents[2].Type, LifecycleEventCompleted)
	}
}

func TestManagerConcurrentAccess(t *testing.T) {
	m := NewManager()

	agent, _ := m.Create("task-123", "/path/to/worktree")
	_ = m.Start(agent.ID, 1234)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = m.Get(agent.ID)
			_, _ = m.GetStatus(agent.ID)
			_ = m.List()
		}()
	}
	wg.Wait()
}
