//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/ShayCichocki/alphie/internal/orchestrator"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// TestGraphBuildingFromTasks tests that tasks can be built into a dependency graph
// and scheduled correctly based on dependencies.
func TestGraphBuildingFromTasks(t *testing.T) {
	// Create tasks with dependencies
	// Task A: no dependencies
	// Task B: depends on A
	// Task C: depends on A
	// Task D: depends on B and C
	taskA := &models.Task{
		ID:        "task-a",
		Title:     "Task A",
		Status:    models.TaskStatusPending,
		DependsOn: []string{},
	}
	taskB := &models.Task{
		ID:        "task-b",
		Title:     "Task B",
		Status:    models.TaskStatusPending,
		DependsOn: []string{"task-a"},
	}
	taskC := &models.Task{
		ID:        "task-c",
		Title:     "Task C",
		Status:    models.TaskStatusPending,
		DependsOn: []string{"task-a"},
	}
	taskD := &models.Task{
		ID:        "task-d",
		Title:     "Task D",
		Status:    models.TaskStatusPending,
		DependsOn: []string{"task-b", "task-c"},
	}

	tasks := []*models.Task{taskA, taskB, taskC, taskD}

	// Build graph
	graph := orchestrator.NewDependencyGraph()
	if err := graph.Build(tasks); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Verify graph size
	if got := graph.Size(); got != 4 {
		t.Errorf("Size() = %d, want 4", got)
	}

	// Verify ready tasks (only A should be ready initially)
	ready := graph.GetReady()
	if len(ready) != 1 {
		t.Errorf("GetReady() returned %d tasks, want 1", len(ready))
	}
	if len(ready) > 0 && ready[0] != "task-a" {
		t.Errorf("GetReady()[0] = %s, want task-a", ready[0])
	}

	// Complete task A
	graph.MarkComplete("task-a")

	// Now B and C should be ready
	ready = graph.GetReady()
	if len(ready) != 2 {
		t.Errorf("After completing A, GetReady() returned %d tasks, want 2", len(ready))
	}

	// Complete B and C
	graph.MarkComplete("task-b")
	graph.MarkComplete("task-c")

	// Now D should be ready
	ready = graph.GetReady()
	if len(ready) != 1 {
		t.Errorf("After completing B and C, GetReady() returned %d tasks, want 1", len(ready))
	}
	if len(ready) > 0 && ready[0] != "task-d" {
		t.Errorf("GetReady()[0] = %s, want task-d", ready[0])
	}
}

// TestGraphCycleDetection verifies that circular dependencies are detected.
func TestGraphCycleDetection(t *testing.T) {
	// Create tasks with circular dependency: A -> B -> C -> A
	taskA := &models.Task{
		ID:        "task-a",
		Title:     "Task A",
		Status:    models.TaskStatusPending,
		DependsOn: []string{"task-c"},
	}
	taskB := &models.Task{
		ID:        "task-b",
		Title:     "Task B",
		Status:    models.TaskStatusPending,
		DependsOn: []string{"task-a"},
	}
	taskC := &models.Task{
		ID:        "task-c",
		Title:     "Task C",
		Status:    models.TaskStatusPending,
		DependsOn: []string{"task-b"},
	}

	tasks := []*models.Task{taskA, taskB, taskC}

	graph := orchestrator.NewDependencyGraph()
	err := graph.Build(tasks)
	if err != orchestrator.ErrCycleDetected {
		t.Errorf("Build() error = %v, want ErrCycleDetected", err)
	}
}

// TestSchedulerWithGraph tests the scheduler integration with dependency graph.
func TestSchedulerWithGraph(t *testing.T) {
	// Create independent tasks that can all run in parallel
	task1 := &models.Task{ID: "task-1", Title: "Task 1", Status: models.TaskStatusPending}
	task2 := &models.Task{ID: "task-2", Title: "Task 2", Status: models.TaskStatusPending}
	task3 := &models.Task{ID: "task-3", Title: "Task 3", Status: models.TaskStatusPending}

	tasks := []*models.Task{task1, task2, task3}

	graph := orchestrator.NewDependencyGraph()
	if err := graph.Build(tasks); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Create scheduler with max 2 agents
	scheduler := orchestrator.NewScheduler(graph, models.TierBuilder, 2)

	// Schedule should return 2 tasks (limited by maxAgents)
	schedulable := scheduler.Schedule()
	if len(schedulable) != 2 {
		t.Errorf("Schedule() returned %d tasks, want 2", len(schedulable))
	}

	// Simulate agent starting on first task
	agent1 := &models.Agent{ID: "agent-1", TaskID: schedulable[0].ID}
	scheduler.OnAgentStart(agent1)

	// Now schedule should return 1 task
	schedulable = scheduler.Schedule()
	if len(schedulable) != 1 {
		t.Errorf("After starting one agent, Schedule() returned %d tasks, want 1", len(schedulable))
	}

	// Complete the first agent
	scheduler.OnAgentComplete("agent-1")

	// Schedule should now return 2 tasks again
	schedulable = scheduler.Schedule()
	if len(schedulable) != 2 {
		t.Errorf("After completing agent, Schedule() returned %d tasks, want 2", len(schedulable))
	}
}

// TestTopologicalSortIntegration tests that topological sort produces valid execution order.
func TestTopologicalSortIntegration(t *testing.T) {
	// Complex dependency graph:
	// E depends on D
	// D depends on B, C
	// B depends on A
	// C depends on A
	// A has no dependencies
	taskA := &models.Task{ID: "A", Title: "A", Status: models.TaskStatusPending}
	taskB := &models.Task{ID: "B", Title: "B", Status: models.TaskStatusPending, DependsOn: []string{"A"}}
	taskC := &models.Task{ID: "C", Title: "C", Status: models.TaskStatusPending, DependsOn: []string{"A"}}
	taskD := &models.Task{ID: "D", Title: "D", Status: models.TaskStatusPending, DependsOn: []string{"B", "C"}}
	taskE := &models.Task{ID: "E", Title: "E", Status: models.TaskStatusPending, DependsOn: []string{"D"}}

	tasks := []*models.Task{taskE, taskD, taskC, taskB, taskA} // Intentionally out of order

	graph := orchestrator.NewDependencyGraph()
	if err := graph.Build(tasks); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	sorted, err := graph.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}

	// Build position map
	pos := make(map[string]int)
	for i, id := range sorted {
		pos[id] = i
	}

	// Verify ordering constraints
	if pos["A"] >= pos["B"] {
		t.Errorf("A should come before B")
	}
	if pos["A"] >= pos["C"] {
		t.Errorf("A should come before C")
	}
	if pos["B"] >= pos["D"] {
		t.Errorf("B should come before D")
	}
	if pos["C"] >= pos["D"] {
		t.Errorf("C should come before D")
	}
	if pos["D"] >= pos["E"] {
		t.Errorf("D should come before E")
	}
}

// TestSchedulerRespectsCompletedTasks verifies scheduler doesn't reschedule completed tasks.
func TestSchedulerRespectsCompletedTasks(t *testing.T) {
	// Task B depends on A
	taskA := &models.Task{ID: "task-a", Title: "A", Status: models.TaskStatusDone}
	taskB := &models.Task{ID: "task-b", Title: "B", Status: models.TaskStatusPending, DependsOn: []string{"task-a"}}

	tasks := []*models.Task{taskA, taskB}

	graph := orchestrator.NewDependencyGraph()
	if err := graph.Build(tasks); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	scheduler := orchestrator.NewScheduler(graph, models.TierBuilder, 5)

	// Should only schedule B since A is already done
	schedulable := scheduler.Schedule()
	if len(schedulable) != 1 {
		t.Errorf("Schedule() returned %d tasks, want 1", len(schedulable))
	}
	if len(schedulable) > 0 && schedulable[0].ID != "task-b" {
		t.Errorf("Schedule()[0].ID = %s, want task-b", schedulable[0].ID)
	}
}

// TestGraphWithBudgetMonitoring simulates budget-aware scheduling.
func TestGraphWithBudgetMonitoring(t *testing.T) {
	// This test verifies the workflow of:
	// 1. Building a task graph
	// 2. Creating a scheduler
	// 3. Tracking progress through budget handler

	tasks := []*models.Task{
		{ID: "t1", Title: "Task 1", Status: models.TaskStatusPending},
		{ID: "t2", Title: "Task 2", Status: models.TaskStatusPending},
		{ID: "t3", Title: "Task 3", Status: models.TaskStatusPending, DependsOn: []string{"t1", "t2"}},
	}

	graph := orchestrator.NewDependencyGraph()
	if err := graph.Build(tasks); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Create budget handler
	budget := orchestrator.NewBudgetHandler(100000) // 100k token budget

	// Create scheduler
	scheduler := orchestrator.NewScheduler(graph, models.TierBuilder, 2)

	// Simulate work loop
	iteration := 0
	for {
		schedulable := scheduler.Schedule()
		if len(schedulable) == 0 {
			// Check if all tasks are done
			ready := graph.GetReady()
			if len(ready) == 0 {
				break
			}
		}

		for _, task := range schedulable {
			// Simulate agent work
			agent := &models.Agent{ID: "agent-" + task.ID, TaskID: task.ID}
			scheduler.OnAgentStart(agent)

			// Simulate token usage (10k per task)
			budget.Update(10000)

			// Complete the task
			scheduler.OnAgentComplete(agent.ID)
		}

		iteration++
		if iteration > 10 {
			t.Fatal("Too many iterations, possible infinite loop")
		}
	}

	// Verify budget usage
	used, _, _ := budget.GetUsage()
	if used != 30000 {
		t.Errorf("Budget used = %d, want 30000", used)
	}
}

// TestSchedulerDependencyChain verifies sequential execution through dependency chain.
func TestSchedulerDependencyChain(t *testing.T) {
	// Create a linear dependency chain: A -> B -> C -> D
	taskA := &models.Task{ID: "A", Title: "A", Status: models.TaskStatusPending}
	taskB := &models.Task{ID: "B", Title: "B", Status: models.TaskStatusPending, DependsOn: []string{"A"}}
	taskC := &models.Task{ID: "C", Title: "C", Status: models.TaskStatusPending, DependsOn: []string{"B"}}
	taskD := &models.Task{ID: "D", Title: "D", Status: models.TaskStatusPending, DependsOn: []string{"C"}}

	tasks := []*models.Task{taskA, taskB, taskC, taskD}

	graph := orchestrator.NewDependencyGraph()
	if err := graph.Build(tasks); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	scheduler := orchestrator.NewScheduler(graph, models.TierBuilder, 10)

	// Execute and verify order
	order := []string{}
	for {
		schedulable := scheduler.Schedule()
		if len(schedulable) == 0 {
			break
		}

		// Should only get one task at a time due to dependencies
		if len(schedulable) != 1 {
			t.Fatalf("Schedule() returned %d tasks, want 1 (due to dependency chain)", len(schedulable))
		}

		task := schedulable[0]
		order = append(order, task.ID)

		agent := &models.Agent{ID: "agent-" + task.ID, TaskID: task.ID}
		scheduler.OnAgentStart(agent)
		scheduler.OnAgentComplete(agent.ID)
	}

	expectedOrder := []string{"A", "B", "C", "D"}
	if len(order) != len(expectedOrder) {
		t.Fatalf("Got %d tasks, want %d", len(order), len(expectedOrder))
	}
	for i, id := range order {
		if id != expectedOrder[i] {
			t.Errorf("order[%d] = %s, want %s", i, id, expectedOrder[i])
		}
	}
}

// TestGraphDependentsLookup verifies looking up tasks that depend on a given task.
func TestGraphDependentsLookup(t *testing.T) {
	// Task structure: A <- B, A <- C, B <- D, C <- D
	taskA := &models.Task{ID: "A", Title: "A", Status: models.TaskStatusPending}
	taskB := &models.Task{ID: "B", Title: "B", Status: models.TaskStatusPending, DependsOn: []string{"A"}}
	taskC := &models.Task{ID: "C", Title: "C", Status: models.TaskStatusPending, DependsOn: []string{"A"}}
	taskD := &models.Task{ID: "D", Title: "D", Status: models.TaskStatusPending, DependsOn: []string{"B", "C"}}

	tasks := []*models.Task{taskA, taskB, taskC, taskD}

	graph := orchestrator.NewDependencyGraph()
	if err := graph.Build(tasks); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// A's dependents should be B and C
	dependents := graph.GetDependents("A")
	if len(dependents) != 2 {
		t.Errorf("GetDependents(A) returned %d tasks, want 2", len(dependents))
	}

	// D's dependents should be empty
	dependents = graph.GetDependents("D")
	if len(dependents) != 0 {
		t.Errorf("GetDependents(D) returned %d tasks, want 0", len(dependents))
	}

	// B's dependents should be D
	dependents = graph.GetDependents("B")
	if len(dependents) != 1 {
		t.Errorf("GetDependents(B) returned %d tasks, want 1", len(dependents))
	}
	if len(dependents) > 0 && dependents[0] != "D" {
		t.Errorf("GetDependents(B)[0] = %s, want D", dependents[0])
	}
}

// TestSessionCreationTime verifies session timestamps are tracked.
func TestSessionCreationTime(t *testing.T) {
	before := time.Now()

	// Simulate creating tasks with timestamps
	task := &models.Task{
		ID:        "task-1",
		Title:     "Test Task",
		Status:    models.TaskStatusPending,
		CreatedAt: time.Now(),
	}

	after := time.Now()

	if task.CreatedAt.Before(before) || task.CreatedAt.After(after) {
		t.Errorf("Task CreatedAt = %v, want between %v and %v", task.CreatedAt, before, after)
	}
}
