// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"strings"

	"github.com/ShayCichocki/alphie/pkg/models"
)

// FileOverlap describes an overlap between two tasks' file boundaries.
type FileOverlap struct {
	Task1ID         string
	Task1Title      string
	Task2ID         string
	Task2Title      string
	OverlappingPath string
}

// PreflightAnalysis contains the results of pre-flight overlap detection.
type PreflightAnalysis struct {
	// Overlaps lists all detected file boundary overlaps.
	Overlaps []FileOverlap
	// RecommendSerial indicates that some tasks should be serialized due to overlaps.
	RecommendSerial bool
	// MaxParallelism is the recommended max agents considering overlaps.
	MaxParallelism int
}

// AnalyzePreFlight checks all tasks for file boundary overlaps before scheduling begins.
// This allows early detection of tasks that would conflict and need serialization.
func (c *CollisionChecker) AnalyzePreFlight(tasks []*models.Task) *PreflightAnalysis {
	result := &PreflightAnalysis{
		MaxParallelism: len(tasks), // Start optimistic
	}

	// Check each pair of tasks for overlaps
	for i := 0; i < len(tasks); i++ {
		for j := i + 1; j < len(tasks); j++ {
			task1 := tasks[i]
			task2 := tasks[j]

			// Skip if either task is already complete
			if task1.Status == models.TaskStatusDone || task2.Status == models.TaskStatusDone {
				continue
			}

			// Get file boundaries for each task
			boundaries1 := c.ExtractPathPrefixes(task1)
			boundaries2 := c.ExtractPathPrefixes(task2)

			// Check for overlaps
			for _, b1 := range boundaries1 {
				for _, b2 := range boundaries2 {
					if pathsOverlap(b1, b2) {
						overlap := FileOverlap{
							Task1ID:         task1.ID,
							Task1Title:      task1.Title,
							Task2ID:         task2.ID,
							Task2Title:      task2.Title,
							OverlappingPath: b1, // or b2, they overlap
						}
						result.Overlaps = append(result.Overlaps, overlap)
					}
				}
			}
		}
	}

	// Calculate recommended parallelism based on overlaps
	if len(result.Overlaps) > 0 {
		result.RecommendSerial = true
		// Build conflict graph and find max independent set (simplified: just reduce by overlap count)
		// More sophisticated: use graph coloring to find true max parallelism
		result.MaxParallelism = c.calculateMaxParallelism(tasks, result.Overlaps)
	}

	return result
}

// pathsOverlap checks if two paths have any overlap (one is a prefix of the other,
// or they are the same path, or they share a common file).
func pathsOverlap(p1, p2 string) bool {
	// Normalize paths
	p1 = strings.TrimPrefix(p1, "/")
	p2 = strings.TrimPrefix(p2, "/")

	// Check if one is a prefix of the other
	if strings.HasPrefix(p1, p2) || strings.HasPrefix(p2, p1) {
		return true
	}

	// Check for exact match (same file)
	if p1 == p2 {
		return true
	}

	return false
}

// calculateMaxParallelism determines the maximum number of tasks that can run
// in parallel without file boundary conflicts.
func (c *CollisionChecker) calculateMaxParallelism(tasks []*models.Task, overlaps []FileOverlap) int {
	if len(overlaps) == 0 {
		return len(tasks)
	}

	// Build adjacency list of conflicts
	conflicts := make(map[string]map[string]bool)
	for _, task := range tasks {
		conflicts[task.ID] = make(map[string]bool)
	}
	for _, overlap := range overlaps {
		conflicts[overlap.Task1ID][overlap.Task2ID] = true
		conflicts[overlap.Task2ID][overlap.Task1ID] = true
	}

	// Greedy coloring to estimate chromatic number (minimum serial groups)
	// The max parallelism is the size of the largest independent set
	colors := make(map[string]int)
	maxColor := 0

	for _, task := range tasks {
		// Find the smallest color not used by neighbors
		usedColors := make(map[int]bool)
		for conflictID := range conflicts[task.ID] {
			if color, ok := colors[conflictID]; ok {
				usedColors[color] = true
			}
		}

		// Assign smallest available color
		color := 0
		for usedColors[color] {
			color++
		}
		colors[task.ID] = color
		if color > maxColor {
			maxColor = color
		}
	}

	// The number of colors used is the chromatic number
	// Max parallelism is total tasks divided by chromatic number (rounded up)
	chromaticNumber := maxColor + 1
	if chromaticNumber == 0 {
		return len(tasks)
	}

	// Return the number of tasks in the largest color class
	colorCounts := make(map[int]int)
	for _, color := range colors {
		colorCounts[color]++
	}
	maxParallel := 0
	for _, count := range colorCounts {
		if count > maxParallel {
			maxParallel = count
		}
	}

	return maxParallel
}
