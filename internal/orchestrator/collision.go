// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"strings"
	"sync"

	"github.com/ShayCichocki/alphie/internal/graph"
	"github.com/ShayCichocki/alphie/internal/merge"
	"github.com/ShayCichocki/alphie/internal/orchestrator/policy"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// SchedulerHint provides hints to the collision checker about an agent's working area.
type SchedulerHint struct {
	// PathPrefixes are the directory prefixes the agent is working in (e.g., ["src/auth/", "src/api/"]).
	PathPrefixes []string
	// Hotspots are files that have been touched frequently during the session.
	Hotspots []string
}

// CollisionChecker tracks file access patterns to prevent scheduling conflicts.
// It ensures:
// - No concurrent tasks on the same path prefix
// - Serialized access to hotspot files
// - Limited agents on the same top-level directory
type CollisionChecker struct {
	// collisionPolicy contains configurable collision thresholds.
	collisionPolicy *policy.CollisionPolicy
	// schedulingPolicy contains configurable scheduling patterns.
	schedulingPolicy *policy.SchedulingPolicy
	// hints maps agent IDs to their scheduler hints.
	hints map[string]*SchedulerHint
	// hotspots maps file paths to their touch counts.
	hotspots map[string]int
	// mu protects all fields.
	mu sync.RWMutex
}

// NewCollisionChecker creates a new CollisionChecker with default policy.
func NewCollisionChecker() *CollisionChecker {
	cfg := policy.Default()
	return NewCollisionCheckerWithPolicy(&cfg.Collision, &cfg.Scheduling)
}

// NewCollisionCheckerWithPolicy creates a new CollisionChecker with custom policy.
func NewCollisionCheckerWithPolicy(cp *policy.CollisionPolicy, sp *policy.SchedulingPolicy) *CollisionChecker {
	cfg := policy.Default()
	if cp == nil {
		cp = &cfg.Collision
	}
	if sp == nil {
		sp = &cfg.Scheduling
	}
	return &CollisionChecker{
		collisionPolicy:  cp,
		schedulingPolicy: sp,
		hints:            make(map[string]*SchedulerHint),
		hotspots:         make(map[string]int),
	}
}

// --- Registration ---

// RegisterAgent registers an agent with path prefixes and hotspots it may touch.
func (c *CollisionChecker) RegisterAgent(agentID string, pathPrefixes, hotspots []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hints[agentID] = &SchedulerHint{
		PathPrefixes: pathPrefixes,
		Hotspots:     hotspots,
	}
}

// UnregisterAgent removes an agent from tracking.
func (c *CollisionChecker) UnregisterAgent(agentID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.hints, agentID)
}

// --- Scheduling ---

// CanSchedule determines if a task can be scheduled given the current running agents.
// It checks for:
// - Path prefix collisions with running agents
// - Hotspot file collisions
// - Top-level directory saturation (max 2 agents)
func (c *CollisionChecker) CanSchedule(task *models.Task, currentAgents []*models.Agent) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Get task path prefixes from description or infer from task ID.
	taskPrefixes := c.ExtractPathPrefixes(task)
	if len(taskPrefixes) == 0 {
		// No path information available, allow scheduling.
		return true
	}

	// Check each running agent for conflicts.
	for _, agent := range currentAgents {
		if agent.Status != models.AgentStatusRunning {
			continue
		}

		hints, ok := c.hints[agent.ID]
		if !ok {
			continue
		}

		// Check for path prefix collision.
		if c.hasPathPrefixCollision(taskPrefixes, hints.PathPrefixes) {
			return false
		}

		// Check for hotspot collision.
		if c.hasHotspotCollision(taskPrefixes, hints.Hotspots) {
			return false
		}
	}

	// Check top-level directory saturation.
	if !c.checkTopLevelLimit(taskPrefixes, currentAgents) {
		return false
	}

	return true
}

// ExtractPathPrefixes extracts path prefixes from a task.
// It prefers explicit FileBoundaries if present, otherwise falls back to
// parsing common path patterns from the task description.
func (c *CollisionChecker) ExtractPathPrefixes(task *models.Task) []string {
	// Prefer explicit FileBoundaries if set (from decomposer)
	if len(task.FileBoundaries) > 0 {
		return task.FileBoundaries
	}

	// Fall back to extracting paths from description for backwards compatibility
	var prefixes []string

	// Common path indicators for various project structures.
	pathIndicators := []string{
		"internal/", "pkg/", "cmd/", "src/", "lib/", "test/", "tests/",
		"server/", "client/", "backend/", "frontend/", "api/", "web/", "app/",
	}

	desc := task.Description + " " + task.Title
	words := strings.Fields(desc)

	for _, word := range words {
		// Clean up the word.
		word = strings.Trim(word, ".,;:\"'`()[]{}*")

		for _, indicator := range pathIndicators {
			if strings.Contains(word, indicator) {
				// Extract the path prefix (up to the file or subdirectory).
				idx := strings.Index(word, indicator)
				prefix := word[idx:]

				// Ensure it ends with a slash for directory matching.
				if !strings.HasSuffix(prefix, "/") {
					// Find the last slash to get the directory.
					lastSlash := strings.LastIndex(prefix, "/")
					if lastSlash > 0 {
						prefix = prefix[:lastSlash+1]
					}
				}

				prefixes = append(prefixes, prefix)
			}
		}
	}

	return prefixes
}

// MightTouchRoot checks if a task might modify root-level files based on its description.
// This is used in greenfield mode to serialize tasks that might conflict on package.json, etc.
func (c *CollisionChecker) MightTouchRoot(task *models.Task) bool {
	// Check if file boundaries explicitly include critical config files
	for _, boundary := range task.FileBoundaries {
		// Use the merge package's critical file detection for consistency
		if merge.IsCriticalFile(boundary) {
			return true
		}
		// Also check root-level files (no directory separator)
		if !strings.Contains(boundary, "/") {
			return true
		}
	}

	// Check description for root-touching keywords
	desc := strings.ToLower(task.Description + " " + task.Title)
	for _, pattern := range c.schedulingPolicy.RootTouchingPatterns {
		if strings.Contains(desc, strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

// GetCriticalFileBoundaries returns the critical config files from a task's file boundaries.
// These are files that should trigger serialization (package.json, go.mod, Cargo.toml, etc.)
func (c *CollisionChecker) GetCriticalFileBoundaries(task *models.Task) []string {
	var critical []string
	for _, boundary := range task.FileBoundaries {
		if merge.IsCriticalFile(boundary) {
			critical = append(critical, boundary)
		}
	}
	return critical
}

// HasCriticalFileConflict checks if a task's critical files conflict with a running agent's.
// This is more specific than MightTouchRoot - it checks for EXACT file overlaps.
func (c *CollisionChecker) HasCriticalFileConflict(task *models.Task, runningAgents []*models.Agent, graph *graph.DependencyGraph) bool {
	taskCritical := c.GetCriticalFileBoundaries(task)
	if len(taskCritical) == 0 {
		return false
	}

	taskCriticalSet := make(map[string]bool)
	for _, f := range taskCritical {
		taskCriticalSet[f] = true
	}

	// Check if any running agent touches the same critical files
	for _, agent := range runningAgents {
		if agent.Status != models.AgentStatusRunning {
			continue
		}

		runningTask := graph.GetTask(agent.TaskID)
		if runningTask == nil {
			continue
		}

		for _, boundary := range runningTask.FileBoundaries {
			if taskCriticalSet[boundary] {
				return true
			}
		}
	}

	return false
}

// HasRootTouchingConflict checks if a task would conflict with a running agent
// that might also touch root files. Used in greenfield mode.
func (c *CollisionChecker) HasRootTouchingConflict(task *models.Task, runningAgents []*models.Agent, graph *graph.DependencyGraph) bool {
	// If this task doesn't touch root, no conflict
	if !c.MightTouchRoot(task) {
		return false
	}

	// Check if any running agent also touches root
	for _, agent := range runningAgents {
		if agent.Status != models.AgentStatusRunning {
			continue
		}

		runningTask := graph.GetTask(agent.TaskID)
		if runningTask != nil && c.MightTouchRoot(runningTask) {
			return true
		}
	}

	return false
}

// --- Hotspot Tracking ---

// RecordTouch increments the touch count for a file path.
// When a file is touched more than the hotspot threshold, it becomes a hotspot.
func (c *CollisionChecker) RecordTouch(agentID, filePath string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.hotspots[filePath]++

	// Update the agent's hints if the file became a hotspot.
	if c.hotspots[filePath] > c.collisionPolicy.HotspotThreshold {
		if hints, ok := c.hints[agentID]; ok {
			// Check if already in hotspots.
			for _, h := range hints.Hotspots {
				if h == filePath {
					return
				}
			}
			hints.Hotspots = append(hints.Hotspots, filePath)
		}
	}
}

// GetHotspots returns all files that have been touched more than the hotspot threshold.
func (c *CollisionChecker) GetHotspots() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []string
	for path, count := range c.hotspots {
		if count > c.collisionPolicy.HotspotThreshold {
			result = append(result, path)
		}
	}
	return result
}

// --- Internal Helpers ---

// hasPathPrefixCollision checks if any task prefix overlaps with agent prefixes.
func (c *CollisionChecker) hasPathPrefixCollision(taskPrefixes, agentPrefixes []string) bool {
	for _, tp := range taskPrefixes {
		for _, ap := range agentPrefixes {
			// Check if one is a prefix of the other.
			if strings.HasPrefix(tp, ap) || strings.HasPrefix(ap, tp) {
				return true
			}
		}
	}
	return false
}

// hasHotspotCollision checks if any task prefix might touch a hotspot file.
func (c *CollisionChecker) hasHotspotCollision(taskPrefixes, hotspots []string) bool {
	for _, tp := range taskPrefixes {
		for _, hs := range hotspots {
			// Check if the hotspot is within the task's working area.
			if strings.HasPrefix(hs, tp) {
				return true
			}
		}
	}
	return false
}

// checkTopLevelLimit ensures no more than maxAgentsPerTopLevel agents work in the same top-level directory.
func (c *CollisionChecker) checkTopLevelLimit(taskPrefixes []string, currentAgents []*models.Agent) bool {
	// Count agents per top-level directory.
	topLevelCounts := make(map[string]int)

	for _, agent := range currentAgents {
		if agent.Status != models.AgentStatusRunning {
			continue
		}

		hints, ok := c.hints[agent.ID]
		if !ok {
			continue
		}

		for _, prefix := range hints.PathPrefixes {
			topLevel := c.getTopLevelDir(prefix)
			if topLevel != "" {
				topLevelCounts[topLevel]++
			}
		}
	}

	// Check if adding this task would exceed the limit.
	for _, prefix := range taskPrefixes {
		topLevel := c.getTopLevelDir(prefix)
		if topLevel != "" {
			if topLevelCounts[topLevel] >= c.collisionPolicy.MaxAgentsPerTopLevel {
				return false
			}
		}
	}

	return true
}

// getTopLevelDir extracts the top-level directory from a path prefix.
// For example, "internal/orchestrator/" returns "internal".
func (c *CollisionChecker) getTopLevelDir(prefix string) string {
	// Remove leading slash if present.
	prefix = strings.TrimPrefix(prefix, "/")

	// Find the first slash.
	idx := strings.Index(prefix, "/")
	if idx > 0 {
		return prefix[:idx]
	}
	if len(prefix) > 0 {
		return prefix
	}
	return ""
}
