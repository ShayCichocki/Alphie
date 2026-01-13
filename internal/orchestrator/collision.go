// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"strings"
	"sync"

	"github.com/shayc/alphie/pkg/models"
)

// SchedulerHint provides hints to the collision checker about an agent's working area.
type SchedulerHint struct {
	// PathPrefixes are the directory prefixes the agent is working in (e.g., ["src/auth/", "src/api/"]).
	PathPrefixes []string
	// Hotspots are files that have been touched frequently (>3x) during the session.
	Hotspots []string
}

// CollisionChecker tracks file access patterns to prevent scheduling conflicts.
// It ensures:
// - No concurrent tasks on the same path prefix
// - Serialized access to hotspot files
// - Maximum 2 agents on the same top-level directory
type CollisionChecker struct {
	// hints maps agent IDs to their scheduler hints.
	hints map[string]*SchedulerHint
	// hotspots maps file paths to their touch counts.
	hotspots map[string]int
	// mu protects all fields.
	mu sync.RWMutex
}

// hotspotThreshold is the number of touches after which a file is considered a hotspot.
const hotspotThreshold = 3

// maxAgentsPerTopLevel is the maximum number of agents allowed in the same top-level directory.
const maxAgentsPerTopLevel = 2

// NewCollisionChecker creates a new CollisionChecker instance.
func NewCollisionChecker() *CollisionChecker {
	return &CollisionChecker{
		hints:    make(map[string]*SchedulerHint),
		hotspots: make(map[string]int),
	}
}

// RegisterAgent registers an agent with its scheduler hints.
func (c *CollisionChecker) RegisterAgent(agentID string, hints *SchedulerHint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hints[agentID] = hints
}

// UnregisterAgent removes an agent from tracking.
func (c *CollisionChecker) UnregisterAgent(agentID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.hints, agentID)
}

// RecordTouch increments the touch count for a file path.
// When a file is touched more than hotspotThreshold times, it becomes a hotspot.
func (c *CollisionChecker) RecordTouch(agentID, filePath string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.hotspots[filePath]++

	// Update the agent's hints if the file became a hotspot.
	if c.hotspots[filePath] > hotspotThreshold {
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

// GetHotspots returns all files that have been touched more than hotspotThreshold times.
func (c *CollisionChecker) GetHotspots() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []string
	for path, count := range c.hotspots {
		if count > hotspotThreshold {
			result = append(result, path)
		}
	}
	return result
}

// CanSchedule determines if a task can be scheduled given the current running agents.
// It checks for:
// - Path prefix collisions with running agents
// - Hotspot file collisions
// - Top-level directory saturation (max 2 agents)
func (c *CollisionChecker) CanSchedule(task *models.Task, currentAgents []*models.Agent) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Get task path prefixes from description or infer from task ID.
	taskPrefixes := c.extractPathPrefixes(task)
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

// extractPathPrefixes extracts path prefixes from a task.
// It looks for common path patterns in the task description.
func (c *CollisionChecker) extractPathPrefixes(task *models.Task) []string {
	// Look for paths in the task description.
	var prefixes []string

	// Common path indicators.
	pathIndicators := []string{
		"internal/", "pkg/", "cmd/", "src/", "lib/", "test/", "tests/",
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
			if topLevelCounts[topLevel] >= maxAgentsPerTopLevel {
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
