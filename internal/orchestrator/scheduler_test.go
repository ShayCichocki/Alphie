package orchestrator

import (
	"testing"

	"github.com/ShayCichocki/alphie/internal/graph"
	"github.com/ShayCichocki/alphie/pkg/models"
)

func TestNewScheduler(t *testing.T) {
	graph := graph.New()
	scheduler := NewScheduler(graph, models.TierBuilder, 4)

	if scheduler == nil {
		t.Fatal("expected non-nil scheduler")
	}

	if scheduler.maxAgents != 4 {
		t.Errorf("expected maxAgents 4, got %d", scheduler.maxAgents)
	}

	if scheduler.tier != models.TierBuilder {
		t.Errorf("expected tier %v, got %v", models.TierBuilder, scheduler.tier)
	}
}

func TestSchedulerScheduleEmpty(t *testing.T) {
	graph := graph.New()
	scheduler := NewScheduler(graph, models.TierBuilder, 4)

	// No tasks in graph
	ready := scheduler.Schedule()
	if len(ready) != 0 {
		t.Errorf("expected 0 ready tasks, got %d", len(ready))
	}
}

func TestSchedulerScheduleWithTasks(t *testing.T) {
	graph := graph.New()
	tasks := []*models.Task{
		{ID: "task-1", Title: "Task 1", Status: models.TaskStatusPending},
		{ID: "task-2", Title: "Task 2", Status: models.TaskStatusPending},
	}

	if err := graph.Build(tasks); err != nil {
		t.Fatalf("failed to build graph: %v", err)
	}

	scheduler := NewScheduler(graph, models.TierBuilder, 4)

	ready := scheduler.Schedule()
	if len(ready) != 2 {
		t.Errorf("expected 2 ready tasks, got %d", len(ready))
	}
}

func TestSchedulerMaxAgentsLimit(t *testing.T) {
	graph := graph.New()
	tasks := []*models.Task{
		{ID: "task-1", Title: "Task 1", Status: models.TaskStatusPending},
		{ID: "task-2", Title: "Task 2", Status: models.TaskStatusPending},
		{ID: "task-3", Title: "Task 3", Status: models.TaskStatusPending},
		{ID: "task-4", Title: "Task 4", Status: models.TaskStatusPending},
		{ID: "task-5", Title: "Task 5", Status: models.TaskStatusPending},
	}

	if err := graph.Build(tasks); err != nil {
		t.Fatalf("failed to build graph: %v", err)
	}

	// Only allow 2 agents
	scheduler := NewScheduler(graph, models.TierBuilder, 2)

	ready := scheduler.Schedule()
	if len(ready) > 2 {
		t.Errorf("expected at most 2 ready tasks (maxAgents limit), got %d", len(ready))
	}
}

func TestSchedulerOnAgentStart(t *testing.T) {
	graph := graph.New()
	scheduler := NewScheduler(graph, models.TierBuilder, 4)

	agent := &models.Agent{
		ID:     "agent-1",
		TaskID: "task-1",
		Status: models.AgentStatusRunning,
	}

	scheduler.OnAgentStart(agent)

	count := scheduler.GetRunningCount()
	if count != 1 {
		t.Errorf("expected 1 running agent, got %d", count)
	}
}

func TestSchedulerOnAgentComplete(t *testing.T) {
	graph := graph.New()
	tasks := []*models.Task{
		{ID: "task-1", Title: "Task 1", Status: models.TaskStatusPending},
	}

	if err := graph.Build(tasks); err != nil {
		t.Fatalf("failed to build graph: %v", err)
	}

	scheduler := NewScheduler(graph, models.TierBuilder, 4)

	agent := &models.Agent{
		ID:     "agent-1",
		TaskID: "task-1",
		Status: models.AgentStatusRunning,
	}

	scheduler.OnAgentStart(agent)
	scheduler.OnAgentComplete("agent-1", true)

	count := scheduler.GetRunningCount()
	if count != 0 {
		t.Errorf("expected 0 running agents after complete, got %d", count)
	}
}

func TestSchedulerGetRunningAgents(t *testing.T) {
	graph := graph.New()
	scheduler := NewScheduler(graph, models.TierBuilder, 4)

	// Add some agents
	agents := []*models.Agent{
		{ID: "agent-1", TaskID: "task-1", Status: models.AgentStatusRunning},
		{ID: "agent-2", TaskID: "task-2", Status: models.AgentStatusRunning},
	}

	for _, agent := range agents {
		scheduler.OnAgentStart(agent)
	}

	running := scheduler.GetRunningAgents()
	if len(running) != 2 {
		t.Errorf("expected 2 running agents, got %d", len(running))
	}
}

func TestSchedulerSetCollisionChecker(t *testing.T) {
	graph := graph.New()
	scheduler := NewScheduler(graph, models.TierBuilder, 4)

	cc := NewCollisionChecker()
	scheduler.SetCollisionChecker(cc)

	// No direct way to verify, but if it doesn't panic, it's good
}

func TestSchedulerWithCollisionBlocking(t *testing.T) {
	graph := graph.New()
	tasks := []*models.Task{
		{ID: "task-1", Title: "Work on internal/auth/", Description: "Update internal/auth/handler.go", Status: models.TaskStatusPending},
		{ID: "task-2", Title: "Work on internal/auth/ too", Description: "Update internal/auth/config.go", Status: models.TaskStatusPending},
	}

	if err := graph.Build(tasks); err != nil {
		t.Fatalf("failed to build graph: %v", err)
	}

	scheduler := NewScheduler(graph, models.TierBuilder, 4)
	cc := NewCollisionChecker()
	scheduler.SetCollisionChecker(cc)

	// Start an agent on task-1 with path hints
	agent := &models.Agent{
		ID:     "agent-1",
		TaskID: "task-1",
		Status: models.AgentStatusRunning,
	}
	scheduler.OnAgentStart(agent)

	// Register hints for the running agent
	cc.RegisterAgent("agent-1", []string{"internal/auth/"}, nil)

	// task-2 should be blocked due to collision
	ready := scheduler.Schedule()
	for _, task := range ready {
		if task.ID == "task-2" {
			t.Error("expected task-2 to be blocked due to collision")
		}
	}
}

func TestSchedulerSlotsExhausted(t *testing.T) {
	graph := graph.New()
	tasks := []*models.Task{
		{ID: "task-1", Title: "Task 1", Status: models.TaskStatusPending},
		{ID: "task-2", Title: "Task 2", Status: models.TaskStatusPending},
	}

	if err := graph.Build(tasks); err != nil {
		t.Fatalf("failed to build graph: %v", err)
	}

	// Only 1 slot
	scheduler := NewScheduler(graph, models.TierBuilder, 1)

	// Start an agent to fill the slot
	agent := &models.Agent{
		ID:     "agent-1",
		TaskID: "task-1",
		Status: models.AgentStatusRunning,
	}
	scheduler.OnAgentStart(agent)

	// No more slots available
	ready := scheduler.Schedule()
	if len(ready) != 0 {
		t.Errorf("expected 0 ready tasks when slots exhausted, got %d", len(ready))
	}
}

func TestSchedulerWithDependencies(t *testing.T) {
	graph := graph.New()
	tasks := []*models.Task{
		{ID: "task-1", Title: "Task 1", Status: models.TaskStatusPending},
		{ID: "task-2", Title: "Task 2", Status: models.TaskStatusPending, DependsOn: []string{"task-1"}},
		{ID: "task-3", Title: "Task 3", Status: models.TaskStatusPending, DependsOn: []string{"task-2"}},
	}

	if err := graph.Build(tasks); err != nil {
		t.Fatalf("failed to build graph: %v", err)
	}

	scheduler := NewScheduler(graph, models.TierBuilder, 4)

	// Initially only task-1 should be schedulable
	ready := scheduler.Schedule()
	if len(ready) != 1 {
		t.Errorf("expected 1 ready task initially, got %d", len(ready))
	}
	if len(ready) > 0 && ready[0].ID != "task-1" {
		t.Errorf("expected task-1 to be ready, got %s", ready[0].ID)
	}

	// Start and complete task-1
	agent := &models.Agent{ID: "agent-1", TaskID: "task-1", Status: models.AgentStatusRunning}
	scheduler.OnAgentStart(agent)
	scheduler.OnAgentComplete("agent-1", true)

	// Now task-2 should be schedulable
	ready = scheduler.Schedule()
	if len(ready) != 1 {
		t.Errorf("expected 1 ready task after task-1 complete, got %d", len(ready))
	}
	if len(ready) > 0 && ready[0].ID != "task-2" {
		t.Errorf("expected task-2 to be ready, got %s", ready[0].ID)
	}
}

func TestSchedulerSkipsAlreadyRunningTasks(t *testing.T) {
	graph := graph.New()
	tasks := []*models.Task{
		{ID: "task-1", Title: "Task 1", Status: models.TaskStatusPending},
		{ID: "task-2", Title: "Task 2", Status: models.TaskStatusPending},
	}

	if err := graph.Build(tasks); err != nil {
		t.Fatalf("failed to build graph: %v", err)
	}

	scheduler := NewScheduler(graph, models.TierBuilder, 4)

	// Start an agent on task-1
	agent := &models.Agent{ID: "agent-1", TaskID: "task-1", Status: models.AgentStatusRunning}
	scheduler.OnAgentStart(agent)

	// Schedule should only return task-2
	ready := scheduler.Schedule()
	if len(ready) != 1 {
		t.Errorf("expected 1 ready task, got %d", len(ready))
	}
	if len(ready) > 0 && ready[0].ID != "task-2" {
		t.Errorf("expected task-2 to be ready (task-1 is running), got %s", ready[0].ID)
	}
}

func TestSchedulerOnAgentCompleteUnknownAgent(t *testing.T) {
	graph := graph.New()
	scheduler := NewScheduler(graph, models.TierBuilder, 4)

	// Completing unknown agent should not panic
	scheduler.OnAgentComplete("non-existent-agent", true)

	// Running count should still be 0
	if scheduler.GetRunningCount() != 0 {
		t.Errorf("expected 0 running agents, got %d", scheduler.GetRunningCount())
	}
}

func TestSchedulerCompleteUnlocksDownstream(t *testing.T) {
	graph := graph.New()
	tasks := []*models.Task{
		{ID: "task-1", Title: "Task 1", Status: models.TaskStatusPending},
		{ID: "task-2", Title: "Task 2", Status: models.TaskStatusPending, DependsOn: []string{"task-1"}},
		{ID: "task-3", Title: "Task 3", Status: models.TaskStatusPending, DependsOn: []string{"task-1"}},
	}

	if err := graph.Build(tasks); err != nil {
		t.Fatalf("failed to build graph: %v", err)
	}

	scheduler := NewScheduler(graph, models.TierBuilder, 4)

	// Complete task-1 via agent
	agent := &models.Agent{ID: "agent-1", TaskID: "task-1", Status: models.AgentStatusRunning}
	scheduler.OnAgentStart(agent)
	scheduler.OnAgentComplete("agent-1", true)

	// Both task-2 and task-3 should now be ready
	ready := scheduler.Schedule()
	if len(ready) != 2 {
		t.Errorf("expected 2 ready tasks after task-1 complete, got %d", len(ready))
	}
}

func TestSchedulerFailedTaskDoesNotUnlockDependents(t *testing.T) {
	g := graph.New()
	task1 := &models.Task{ID: "task-1", Title: "Task 1", Status: models.TaskStatusPending}
	tasks := []*models.Task{
		task1,
		{ID: "task-2", Title: "Task 2", Status: models.TaskStatusPending, DependsOn: []string{"task-1"}},
		{ID: "task-3", Title: "Task 3", Status: models.TaskStatusPending, DependsOn: []string{"task-1"}},
	}

	if err := g.Build(tasks); err != nil {
		t.Fatalf("failed to build graph: %v", err)
	}

	scheduler := NewScheduler(g, models.TierBuilder, 4)

	// Fail task-1 via agent (success=false)
	agent := &models.Agent{ID: "agent-1", TaskID: "task-1", Status: models.AgentStatusRunning}
	scheduler.OnAgentStart(agent)
	scheduler.OnAgentComplete("agent-1", false) // FAILED

	// Simulate what orchestrator does: mark task as failed
	task1.Status = models.TaskStatusFailed

	// task-2 and task-3 should NOT be ready because task-1 failed
	ready := scheduler.Schedule()
	if len(ready) != 0 {
		t.Errorf("expected 0 ready tasks after task-1 FAILED (dependents should stay blocked), got %d", len(ready))
	}

	// Verify task-1 is not in completed map (so it doesn't unblock dependents)
	completedIDs := g.GetCompletedIDs()
	for _, id := range completedIDs {
		if id == "task-1" {
			t.Error("failed task should not be marked as completed in graph")
		}
	}
}

func TestSchedulerWithNoCollisionChecker(t *testing.T) {
	g := graph.New()
	tasks := []*models.Task{
		{ID: "task-1", Title: "Work on internal/auth/", Description: "Update internal/auth/handler.go", Status: models.TaskStatusPending},
		{ID: "task-2", Title: "Work on internal/auth/ too", Description: "Update internal/auth/config.go", Status: models.TaskStatusPending},
	}

	if err := g.Build(tasks); err != nil {
		t.Fatalf("failed to build graph: %v", err)
	}

	// No collision checker set - should allow both tasks
	scheduler := NewScheduler(g, models.TierBuilder, 4)

	ready := scheduler.Schedule()
	if len(ready) != 2 {
		t.Errorf("expected 2 ready tasks (no collision checker), got %d", len(ready))
	}
}
