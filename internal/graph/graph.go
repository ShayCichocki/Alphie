// Package graph provides a dependency graph for task scheduling.
package graph

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ShayCichocki/alphie/pkg/models"
)

// ErrCycleDetected indicates a circular dependency was found in the task graph.
var ErrCycleDetected = errors.New("circular dependency detected")

// DependencyGraph represents a directed acyclic graph of task dependencies.
// Tasks are nodes, and edges represent "blocked by" relationships.
type DependencyGraph struct {
	mu sync.RWMutex
	// nodes maps task ID to the task itself.
	nodes map[string]*models.Task
	// edges maps task ID to IDs of tasks it depends on (is blocked by).
	edges map[string][]string
	// completed tracks which tasks have been marked complete.
	completed map[string]bool
	// debugLog is an optional logging function.
	debugLog func(format string, args ...interface{})
}

// New creates a new empty dependency graph.
func New() *DependencyGraph {
	return &DependencyGraph{
		nodes:     make(map[string]*models.Task),
		edges:     make(map[string][]string),
		completed: make(map[string]bool),
		debugLog:  func(format string, args ...interface{}) {}, // no-op by default
	}
}

// SetDebugLog sets the debug logging function.
func (g *DependencyGraph) SetDebugLog(fn func(format string, args ...interface{})) {
	if fn != nil {
		g.debugLog = fn
	}
}

// Build constructs the dependency graph from a slice of tasks.
// Returns an error if a cycle is detected or dependencies reference unknown tasks.
func (g *DependencyGraph) Build(tasks []*models.Task) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.debugLog("[graph.Build] building graph from %d tasks", len(tasks))

	// First pass: register all tasks as nodes.
	for _, task := range tasks {
		g.debugLog("[graph.Build] adding task: id=%s title=%q depends_on=%v", task.ID, task.Title, task.DependsOn)
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

	g.debugLog("[graph.Build] final edges map: %v", g.edges)

	// Check for cycles (use internal method since we hold the lock).
	if g.hasCycleLocked() {
		return ErrCycleDetected
	}

	g.debugLog("[graph.Build] graph built successfully with %d nodes", len(g.nodes))
	return nil
}

// HasCycle returns true if the graph contains a circular dependency.
// Uses depth-first search with coloring to detect back edges.
func (g *DependencyGraph) HasCycle() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.hasCycleLocked()
}

// hasCycleLocked is the internal implementation that assumes the lock is held.
func (g *DependencyGraph) hasCycleLocked() bool {
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
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.hasCycleLocked() {
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
	g.mu.RLock()
	defer g.mu.RUnlock()

	var ready []string

	g.debugLog("[graph.GetReady] evaluating %d nodes, completed map: %v", len(g.nodes), g.completed)

	for id, task := range g.nodes {
		// Skip already completed tasks.
		if g.completed[id] {
			g.debugLog("[graph.GetReady] task %s: skipped (in completed map)", id)
			continue
		}

		// Skip tasks that are already done or failed.
		if task.Status == models.TaskStatusDone || task.Status == models.TaskStatusFailed {
			g.debugLog("[graph.GetReady] task %s: skipped (status=%s)", id, task.Status)
			continue
		}

		// Check if all dependencies are satisfied.
		allDepsComplete := true
		deps := g.edges[id]
		g.debugLog("[graph.GetReady] task %s: checking %d dependencies: %v", id, len(deps), deps)

		for _, depID := range deps {
			if !g.completed[depID] {
				// Also check the task status as a fallback.
				if depTask, exists := g.nodes[depID]; exists {
					g.debugLog("[graph.GetReady] task %s: dep %s not in completed map, checking status=%s", id, depID, depTask.Status)
					if depTask.Status != models.TaskStatusDone {
						allDepsComplete = false
						g.debugLog("[graph.GetReady] task %s: dep %s NOT satisfied (status=%s)", id, depID, depTask.Status)
						break
					}
				} else {
					g.debugLog("[graph.GetReady] task %s: dep %s not found in graph", id, depID)
					allDepsComplete = false
					break
				}
			} else {
				g.debugLog("[graph.GetReady] task %s: dep %s satisfied (in completed map)", id, depID)
			}
		}

		if allDepsComplete {
			g.debugLog("[graph.GetReady] task %s: READY (all deps satisfied)", id)
			ready = append(ready, id)
		} else {
			g.debugLog("[graph.GetReady] task %s: not ready (has unsatisfied deps)", id)
		}
	}

	g.debugLog("[graph.GetReady] returning %d ready tasks: %v", len(ready), ready)
	return ready
}

// MarkComplete marks a task as completed in the graph.
// This affects subsequent calls to GetReady.
func (g *DependencyGraph) MarkComplete(taskID string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.debugLog("[graph.MarkComplete] marking task %s as complete", taskID)
	g.completed[taskID] = true
	g.debugLog("[graph.MarkComplete] completed map now: %v", g.completed)
}

// GetTask returns the task for a given ID, or nil if not found.
func (g *DependencyGraph) GetTask(taskID string) *models.Task {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.nodes[taskID]
}

// Size returns the number of tasks in the graph.
func (g *DependencyGraph) Size() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}

// GetDependencies returns the IDs of tasks that the given task depends on.
func (g *DependencyGraph) GetDependencies(taskID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.edges[taskID]
}

// GetDependents returns the IDs of tasks that depend on the given task.
func (g *DependencyGraph) GetDependents(taskID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

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

// GetCompletedIDs returns the IDs of all tasks marked as completed in the graph.
func (g *DependencyGraph) GetCompletedIDs() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var ids []string
	for id, done := range g.completed {
		if done {
			ids = append(ids, id)
		}
	}
	return ids
}
