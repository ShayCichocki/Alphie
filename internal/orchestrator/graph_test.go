package orchestrator

import (
	"errors"
	"sort"
	"testing"

	"github.com/shayc/alphie/pkg/models"
)

func TestNewDependencyGraph(t *testing.T) {
	g := NewDependencyGraph()
	if g == nil {
		t.Fatal("expected non-nil graph")
	}
	if g.Size() != 0 {
		t.Errorf("expected empty graph, got size %d", g.Size())
	}
}

func TestGraphBuildSimple(t *testing.T) {
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "task-1", Title: "Task 1", Status: models.TaskStatusPending},
		{ID: "task-2", Title: "Task 2", Status: models.TaskStatusPending},
		{ID: "task-3", Title: "Task 3", Status: models.TaskStatusPending},
	}

	err := g.Build(tasks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if g.Size() != 3 {
		t.Errorf("expected size 3, got %d", g.Size())
	}
}

func TestGraphBuildWithDependencies(t *testing.T) {
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "task-1", Title: "Task 1", Status: models.TaskStatusPending},
		{ID: "task-2", Title: "Task 2", Status: models.TaskStatusPending, DependsOn: []string{"task-1"}},
		{ID: "task-3", Title: "Task 3", Status: models.TaskStatusPending, DependsOn: []string{"task-1", "task-2"}},
	}

	err := g.Build(tasks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify dependencies
	deps := g.GetDependencies("task-3")
	if len(deps) != 2 {
		t.Errorf("expected 2 dependencies for task-3, got %d", len(deps))
	}

	// Verify dependents
	dependents := g.GetDependents("task-1")
	if len(dependents) != 2 {
		t.Errorf("expected 2 dependents of task-1, got %d", len(dependents))
	}
}

func TestGraphBuildUnknownDependency(t *testing.T) {
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "task-1", Title: "Task 1", Status: models.TaskStatusPending, DependsOn: []string{"unknown-task"}},
	}

	err := g.Build(tasks)
	if err == nil {
		t.Fatal("expected error for unknown dependency")
	}
}

func TestGraphCycleDetectionSimple(t *testing.T) {
	// A -> B -> A (direct cycle)
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "A", Title: "Task A", Status: models.TaskStatusPending, DependsOn: []string{"B"}},
		{ID: "B", Title: "Task B", Status: models.TaskStatusPending, DependsOn: []string{"A"}},
	}

	err := g.Build(tasks)
	if !errors.Is(err, ErrCycleDetected) {
		t.Errorf("expected ErrCycleDetected, got %v", err)
	}
}

func TestGraphCycleDetectionThreeNodes(t *testing.T) {
	// A -> B -> C -> A (three node cycle)
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "A", Title: "Task A", Status: models.TaskStatusPending, DependsOn: []string{"B"}},
		{ID: "B", Title: "Task B", Status: models.TaskStatusPending, DependsOn: []string{"C"}},
		{ID: "C", Title: "Task C", Status: models.TaskStatusPending, DependsOn: []string{"A"}},
	}

	err := g.Build(tasks)
	if !errors.Is(err, ErrCycleDetected) {
		t.Errorf("expected ErrCycleDetected for A->B->C->A cycle, got %v", err)
	}
}

func TestGraphCycleDetectionSelfLoop(t *testing.T) {
	// A -> A (self loop)
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "A", Title: "Task A", Status: models.TaskStatusPending, DependsOn: []string{"A"}},
	}

	err := g.Build(tasks)
	if !errors.Is(err, ErrCycleDetected) {
		t.Errorf("expected ErrCycleDetected for self-loop, got %v", err)
	}
}

func TestGraphNoCycle(t *testing.T) {
	// A -> B -> C (linear, no cycle)
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "A", Title: "Task A", Status: models.TaskStatusPending},
		{ID: "B", Title: "Task B", Status: models.TaskStatusPending, DependsOn: []string{"A"}},
		{ID: "C", Title: "Task C", Status: models.TaskStatusPending, DependsOn: []string{"B"}},
	}

	err := g.Build(tasks)
	if err != nil {
		t.Fatalf("unexpected error for acyclic graph: %v", err)
	}

	if g.HasCycle() {
		t.Error("expected no cycle in linear graph")
	}
}

func TestGraphTopologicalSortLinear(t *testing.T) {
	// A -> B -> C (C depends on B, B depends on A)
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "A", Title: "Task A", Status: models.TaskStatusPending},
		{ID: "B", Title: "Task B", Status: models.TaskStatusPending, DependsOn: []string{"A"}},
		{ID: "C", Title: "Task C", Status: models.TaskStatusPending, DependsOn: []string{"B"}},
	}

	err := g.Build(tasks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error in TopologicalSort: %v", err)
	}

	if len(sorted) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(sorted))
	}

	// Verify ordering: A must come before B, B must come before C
	posA, posB, posC := -1, -1, -1
	for i, id := range sorted {
		switch id {
		case "A":
			posA = i
		case "B":
			posB = i
		case "C":
			posC = i
		}
	}

	if posA > posB {
		t.Errorf("A should come before B in topological order, got A at %d, B at %d", posA, posB)
	}
	if posB > posC {
		t.Errorf("B should come before C in topological order, got B at %d, C at %d", posB, posC)
	}
}

func TestGraphTopologicalSortDiamond(t *testing.T) {
	// Diamond shape: A -> B, A -> C, B -> D, C -> D
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "A", Title: "Task A", Status: models.TaskStatusPending},
		{ID: "B", Title: "Task B", Status: models.TaskStatusPending, DependsOn: []string{"A"}},
		{ID: "C", Title: "Task C", Status: models.TaskStatusPending, DependsOn: []string{"A"}},
		{ID: "D", Title: "Task D", Status: models.TaskStatusPending, DependsOn: []string{"B", "C"}},
	}

	err := g.Build(tasks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error in TopologicalSort: %v", err)
	}

	if len(sorted) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(sorted))
	}

	// Verify A comes first and D comes last
	positions := make(map[string]int)
	for i, id := range sorted {
		positions[id] = i
	}

	if positions["A"] > positions["B"] || positions["A"] > positions["C"] {
		t.Error("A should come before B and C")
	}
	if positions["B"] > positions["D"] || positions["C"] > positions["D"] {
		t.Error("B and C should come before D")
	}
}

func TestGraphTopologicalSortWithCycle(t *testing.T) {
	// Create graph manually to bypass Build's cycle check
	g := NewDependencyGraph()
	g.nodes["A"] = &models.Task{ID: "A", Title: "Task A"}
	g.nodes["B"] = &models.Task{ID: "B", Title: "Task B"}
	g.edges["A"] = []string{"B"}
	g.edges["B"] = []string{"A"}

	_, err := g.TopologicalSort()
	if !errors.Is(err, ErrCycleDetected) {
		t.Errorf("expected ErrCycleDetected, got %v", err)
	}
}

func TestGraphGetReady(t *testing.T) {
	// A -> B -> C
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "A", Title: "Task A", Status: models.TaskStatusPending},
		{ID: "B", Title: "Task B", Status: models.TaskStatusPending, DependsOn: []string{"A"}},
		{ID: "C", Title: "Task C", Status: models.TaskStatusPending, DependsOn: []string{"B"}},
	}

	err := g.Build(tasks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Initially only A should be ready
	ready := g.GetReady()
	if len(ready) != 1 || ready[0] != "A" {
		t.Errorf("expected only A to be ready, got %v", ready)
	}
}

func TestGraphGetReadyAfterMarkComplete(t *testing.T) {
	// A -> B -> C
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "A", Title: "Task A", Status: models.TaskStatusPending},
		{ID: "B", Title: "Task B", Status: models.TaskStatusPending, DependsOn: []string{"A"}},
		{ID: "C", Title: "Task C", Status: models.TaskStatusPending, DependsOn: []string{"B"}},
	}

	err := g.Build(tasks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Mark A complete
	g.MarkComplete("A")

	// Now B should be ready
	ready := g.GetReady()
	if len(ready) != 1 || ready[0] != "B" {
		t.Errorf("expected only B to be ready after A complete, got %v", ready)
	}
}

func TestGraphGetReadyMultiple(t *testing.T) {
	// A (no deps), B (no deps), C (depends on A and B)
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "A", Title: "Task A", Status: models.TaskStatusPending},
		{ID: "B", Title: "Task B", Status: models.TaskStatusPending},
		{ID: "C", Title: "Task C", Status: models.TaskStatusPending, DependsOn: []string{"A", "B"}},
	}

	err := g.Build(tasks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ready := g.GetReady()
	if len(ready) != 2 {
		t.Errorf("expected 2 ready tasks, got %d", len(ready))
	}

	// Sort for deterministic comparison
	sort.Strings(ready)
	if ready[0] != "A" || ready[1] != "B" {
		t.Errorf("expected A and B to be ready, got %v", ready)
	}
}

func TestGraphGetReadySkipsCompletedTasks(t *testing.T) {
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "A", Title: "Task A", Status: models.TaskStatusPending},
		{ID: "B", Title: "Task B", Status: models.TaskStatusPending},
	}

	err := g.Build(tasks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	g.MarkComplete("A")

	ready := g.GetReady()
	if len(ready) != 1 || ready[0] != "B" {
		t.Errorf("expected only B to be ready (A is complete), got %v", ready)
	}
}

func TestGraphGetReadySkipsDoneTasks(t *testing.T) {
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "A", Title: "Task A", Status: models.TaskStatusDone},
		{ID: "B", Title: "Task B", Status: models.TaskStatusPending},
	}

	err := g.Build(tasks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ready := g.GetReady()
	if len(ready) != 1 || ready[0] != "B" {
		t.Errorf("expected only B to be ready (A is done), got %v", ready)
	}
}

func TestGraphGetReadySkipsFailedTasks(t *testing.T) {
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "A", Title: "Task A", Status: models.TaskStatusFailed},
		{ID: "B", Title: "Task B", Status: models.TaskStatusPending},
	}

	err := g.Build(tasks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ready := g.GetReady()
	if len(ready) != 1 || ready[0] != "B" {
		t.Errorf("expected only B to be ready (A is failed), got %v", ready)
	}
}

func TestGraphGetTask(t *testing.T) {
	g := NewDependencyGraph()
	task := &models.Task{ID: "task-1", Title: "Task 1", Status: models.TaskStatusPending}
	tasks := []*models.Task{task}

	err := g.Build(tasks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := g.GetTask("task-1")
	if got == nil {
		t.Fatal("expected task, got nil")
	}
	if got.ID != "task-1" {
		t.Errorf("expected task-1, got %s", got.ID)
	}

	// Test non-existent task
	got = g.GetTask("non-existent")
	if got != nil {
		t.Errorf("expected nil for non-existent task, got %v", got)
	}
}

func TestGraphGetDependencies(t *testing.T) {
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "A", Title: "Task A", Status: models.TaskStatusPending},
		{ID: "B", Title: "Task B", Status: models.TaskStatusPending},
		{ID: "C", Title: "Task C", Status: models.TaskStatusPending, DependsOn: []string{"A", "B"}},
	}

	err := g.Build(tasks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	deps := g.GetDependencies("C")
	if len(deps) != 2 {
		t.Errorf("expected 2 dependencies, got %d", len(deps))
	}

	// Empty for no dependencies
	deps = g.GetDependencies("A")
	if len(deps) != 0 {
		t.Errorf("expected 0 dependencies for A, got %d", len(deps))
	}
}

func TestGraphGetDependents(t *testing.T) {
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "A", Title: "Task A", Status: models.TaskStatusPending},
		{ID: "B", Title: "Task B", Status: models.TaskStatusPending, DependsOn: []string{"A"}},
		{ID: "C", Title: "Task C", Status: models.TaskStatusPending, DependsOn: []string{"A"}},
	}

	err := g.Build(tasks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dependents := g.GetDependents("A")
	if len(dependents) != 2 {
		t.Errorf("expected 2 dependents of A, got %d", len(dependents))
	}

	// Sort for deterministic comparison
	sort.Strings(dependents)
	if dependents[0] != "B" || dependents[1] != "C" {
		t.Errorf("expected B and C as dependents, got %v", dependents)
	}
}

func TestGraphEmptyGraph(t *testing.T) {
	g := NewDependencyGraph()

	// Empty graph operations should work without panic
	err := g.Build([]*models.Task{})
	if err != nil {
		t.Fatalf("unexpected error building empty graph: %v", err)
	}

	if g.HasCycle() {
		t.Error("empty graph should not have cycle")
	}

	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error in TopologicalSort: %v", err)
	}
	if len(sorted) != 0 {
		t.Errorf("expected empty sorted list, got %v", sorted)
	}

	ready := g.GetReady()
	if len(ready) != 0 {
		t.Errorf("expected no ready tasks, got %v", ready)
	}
}

func TestGraphComplexDependencies(t *testing.T) {
	// Complex graph with multiple paths
	//       A
	//      / \
	//     B   C
	//    / \ / \
	//   D   E   F
	//    \ | /
	//     \|/
	//      G
	g := NewDependencyGraph()
	tasks := []*models.Task{
		{ID: "A", Title: "Task A", Status: models.TaskStatusPending},
		{ID: "B", Title: "Task B", Status: models.TaskStatusPending, DependsOn: []string{"A"}},
		{ID: "C", Title: "Task C", Status: models.TaskStatusPending, DependsOn: []string{"A"}},
		{ID: "D", Title: "Task D", Status: models.TaskStatusPending, DependsOn: []string{"B"}},
		{ID: "E", Title: "Task E", Status: models.TaskStatusPending, DependsOn: []string{"B", "C"}},
		{ID: "F", Title: "Task F", Status: models.TaskStatusPending, DependsOn: []string{"C"}},
		{ID: "G", Title: "Task G", Status: models.TaskStatusPending, DependsOn: []string{"D", "E", "F"}},
	}

	err := g.Build(tasks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify topological sort respects all dependencies
	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error in TopologicalSort: %v", err)
	}

	positions := make(map[string]int)
	for i, id := range sorted {
		positions[id] = i
	}

	// Verify ordering constraints
	constraints := []struct {
		before, after string
	}{
		{"A", "B"}, {"A", "C"},
		{"B", "D"}, {"B", "E"},
		{"C", "E"}, {"C", "F"},
		{"D", "G"}, {"E", "G"}, {"F", "G"},
	}

	for _, c := range constraints {
		if positions[c.before] >= positions[c.after] {
			t.Errorf("%s should come before %s", c.before, c.after)
		}
	}
}
