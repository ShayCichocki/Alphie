package orchestrator

import (
	"errors"
	"fmt"

	"github.com/shayc/alphie/pkg/models"
)

// ErrCycleDetected indicates a circular dependency was found in the task graph.
var ErrCycleDetected = errors.New("circular dependency detected")

// DependencyGraph represents a directed acyclic graph of task dependencies.
// Tasks are nodes, and edges represent "blocked by" relationships.
type DependencyGraph struct {
	// nodes maps task ID to the task itself.
	nodes map[string]*models.Task
	// edges maps task ID to IDs of tasks it depends on (is blocked by).
	edges map[string][]string
	// completed tracks which tasks have been marked complete.
	completed map[string]bool
}

// NewDependencyGraph creates a new empty dependency graph.
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		nodes:     make(map[string]*models.Task),
		edges:     make(map[string][]string),
		completed: make(map[string]bool),
	}
}

// Build constructs the dependency graph from a slice of tasks.
// Returns an error if a cycle is detected or dependencies reference unknown tasks.
func (g *DependencyGraph) Build(tasks []*models.Task) error {
	// First pass: register all tasks as nodes.
	for _, task := range tasks {
		g.nodes[task.ID] = task
		g.edges[task.ID] = nil // Initialize edges slice.
	}

	// Second pass: build edges from DependsOn fields.
	for _, task := range tasks {
		for _, depID := range task.DependsOn {
			if _, exists := g.nodes[depID]; !exists {
				return fmt.Errorf("task %s depends on unknown task %s", task.ID, depID)
			}
			g.edges[task.ID] = append(g.edges[task.ID], depID)
		}
	}

	// Check for cycles.
	if g.HasCycle() {
		return ErrCycleDetected
	}

	return nil
}

// HasCycle returns true if the graph contains a circular dependency.
// Uses depth-first search with coloring to detect back edges.
func (g *DependencyGraph) HasCycle() bool {
	// Color states: 0 = white (unvisited), 1 = gray (in progress), 2 = black (done).
	colors := make(map[string]int)
	for id := range g.nodes {
		colors[id] = 0
	}

	var hasCycle bool
	var visit func(id string) bool
	visit = func(id string) bool {
		colors[id] = 1 // Mark as in progress.

		for _, depID := range g.edges[id] {
			switch colors[depID] {
			case 1:
				// Found a back edge - cycle detected.
				return true
			case 0:
				if visit(depID) {
					return true
				}
			}
			// color == 2 means already processed, skip.
		}

		colors[id] = 2 // Mark as done.
		return false
	}

	for id := range g.nodes {
		if colors[id] == 0 {
			if visit(id) {
				hasCycle = true
				break
			}
		}
	}

	return hasCycle
}

// TopologicalSort returns task IDs in an order where all dependencies
// come before the tasks that depend on them.
// Returns an error if the graph contains a cycle.
func (g *DependencyGraph) TopologicalSort() ([]string, error) {
	if g.HasCycle() {
		return nil, ErrCycleDetected
	}

	// Track visited nodes and build result in reverse post-order.
	visited := make(map[string]bool)
	var result []string

	var visit func(id string)
	visit = func(id string) {
		if visited[id] {
			return
		}
		visited[id] = true

		// Visit all dependencies first.
		for _, depID := range g.edges[id] {
			visit(depID)
		}

		// Add this node after its dependencies.
		result = append(result, id)
	}

	// Visit all nodes.
	for id := range g.nodes {
		visit(id)
	}

	return result, nil
}

// GetReady returns task IDs that have no unmet dependencies and are not yet completed.
// These tasks can be executed in parallel.
func (g *DependencyGraph) GetReady() []string {
	var ready []string

	for id, task := range g.nodes {
		// Skip already completed tasks.
		if g.completed[id] {
			continue
		}

		// Skip tasks that are already done or failed.
		if task.Status == models.TaskStatusDone || task.Status == models.TaskStatusFailed {
			continue
		}

		// Check if all dependencies are satisfied.
		allDepsComplete := true
		for _, depID := range g.edges[id] {
			if !g.completed[depID] {
				// Also check the task status as a fallback.
				if depTask, exists := g.nodes[depID]; exists {
					if depTask.Status != models.TaskStatusDone {
						allDepsComplete = false
						break
					}
				} else {
					allDepsComplete = false
					break
				}
			}
		}

		if allDepsComplete {
			ready = append(ready, id)
		}
	}

	return ready
}

// MarkComplete marks a task as completed in the graph.
// This affects subsequent calls to GetReady.
func (g *DependencyGraph) MarkComplete(taskID string) {
	g.completed[taskID] = true
}

// GetTask returns the task for a given ID, or nil if not found.
func (g *DependencyGraph) GetTask(taskID string) *models.Task {
	return g.nodes[taskID]
}

// Size returns the number of tasks in the graph.
func (g *DependencyGraph) Size() int {
	return len(g.nodes)
}

// GetDependencies returns the IDs of tasks that the given task depends on.
func (g *DependencyGraph) GetDependencies(taskID string) []string {
	return g.edges[taskID]
}

// GetDependents returns the IDs of tasks that depend on the given task.
func (g *DependencyGraph) GetDependents(taskID string) []string {
	var dependents []string
	for id, deps := range g.edges {
		for _, depID := range deps {
			if depID == taskID {
				dependents = append(dependents, id)
				break
			}
		}
	}
	return dependents
}
