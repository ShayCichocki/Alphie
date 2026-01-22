// Package decompose provides quality scoring for task decompositions.
package decompose

import (
	"fmt"
	"strings"

	"github.com/ShayCichocki/alphie/internal/protect"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// Severity indicates the severity of a quality issue.
type Severity int

const (
	// SeverityInfo indicates informational feedback.
	SeverityInfo Severity = iota
	// SeverityWarning indicates a potential problem.
	SeverityWarning
	// SeverityCritical indicates a serious problem.
	SeverityCritical
)

// String returns a human-readable severity level.
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// QualityIssue represents a specific problem or concern with a task.
type QualityIssue struct {
	Severity   Severity
	Message    string
	Suggestion string
}

// TaskQualityScore represents the quality score for a single task.
type TaskQualityScore struct {
	TaskID     string
	Confidence float64 // 0.0-1.0, where 1.0 is highest confidence
	Issues     []QualityIssue
}

// DecompositionQuality represents the overall quality of a decomposition.
type DecompositionQuality struct {
	OverallConfidence float64 // 0.0-1.0, overall confidence score
	TaskScores        []TaskQualityScore
	Warnings          []string
	EstimatedParallelism int // Maximum number of tasks that can run in parallel
	TotalTasks        int
	CriticalIssues    int
}

// ScoreDecomposition analyzes a decomposition and assigns quality scores.
func ScoreDecomposition(tasks []*models.Task) DecompositionQuality {
	quality := DecompositionQuality{
		OverallConfidence: 1.0,
		TaskScores:        make([]TaskQualityScore, len(tasks)),
		TotalTasks:        len(tasks),
	}

	// Score each task individually
	for i, task := range tasks {
		score := scoreTask(task, tasks)
		quality.TaskScores[i] = score

		// Count critical issues
		for _, issue := range score.Issues {
			if issue.Severity == SeverityCritical {
				quality.CriticalIssues++
			}
		}
	}

	// Calculate overall confidence based on individual task scores
	totalConfidence := 0.0
	for _, score := range quality.TaskScores {
		totalConfidence += score.Confidence
	}
	if len(quality.TaskScores) > 0 {
		quality.OverallConfidence = totalConfidence / float64(len(quality.TaskScores))
	}

	// Apply global penalties
	quality.OverallConfidence = applyGlobalPenalties(quality.OverallConfidence, tasks)

	// Generate warnings
	quality.Warnings = generateWarnings(tasks, quality.TaskScores)

	// Calculate estimated parallelism (simple: count tasks with no dependencies)
	quality.EstimatedParallelism = calculateParallelism(tasks)

	return quality
}

// scoreTask scores an individual task.
func scoreTask(task *models.Task, allTasks []*models.Task) TaskQualityScore {
	score := TaskQualityScore{
		TaskID:     task.ID,
		Confidence: 1.0,
		Issues:     []QualityIssue{},
	}

	// Check file boundaries specificity
	if len(task.FileBoundaries) == 0 {
		score.Confidence -= 0.2
		score.Issues = append(score.Issues, QualityIssue{
			Severity:   SeverityWarning,
			Message:    "No file boundaries specified",
			Suggestion: "Add specific file or directory paths to reduce merge conflicts",
		})
	} else {
		for _, boundary := range task.FileBoundaries {
			// Check for vague boundaries like "src/" or "."
			if boundary == "." || boundary == "./" || boundary == "src/" || boundary == "src" {
				score.Confidence -= 0.3
				score.Issues = append(score.Issues, QualityIssue{
					Severity:   SeverityCritical,
					Message:    "Vague file boundary: " + boundary,
					Suggestion: "Specify more precise file or directory paths",
				})
			}
			// Check for root-level boundaries that could cause conflicts
			if strings.Count(boundary, "/") <= 1 && boundary != "." {
				score.Confidence -= 0.1
				score.Issues = append(score.Issues, QualityIssue{
					Severity:   SeverityInfo,
					Message:    "Root-level boundary may cause conflicts: " + boundary,
					Suggestion: "Consider more specific subdirectories",
				})
			}
		}
	}

	// Check for overlap with other tasks
	overlapCount := 0
	for _, other := range allTasks {
		if other.ID == task.ID {
			continue
		}
		if hasFileOverlap(task.FileBoundaries, other.FileBoundaries) {
			overlapCount++
		}
	}
	if overlapCount > 0 {
		penalty := float64(overlapCount) * 0.15 // 0.15 per overlapping task
		if penalty > 0.5 {
			penalty = 0.5 // Cap at 0.5
		}
		score.Confidence -= penalty
		score.Issues = append(score.Issues, QualityIssue{
			Severity:   SeverityWarning,
			Message:    fmt.Sprintf("File boundaries overlap with %d other tasks", overlapCount),
			Suggestion: "Review task boundaries to minimize merge conflicts",
		})
	}

	// Check dependency depth
	depth := calculateDependencyDepth(task, allTasks)
	if depth > 3 {
		penalty := float64(depth-3) * 0.1
		score.Confidence -= penalty
		score.Issues = append(score.Issues, QualityIssue{
			Severity:   SeverityWarning,
			Message:    fmt.Sprintf("Deep dependency chain (depth %d)", depth),
			Suggestion: "Consider flattening dependencies for better parallelism",
		})
	}

	// Check for missing acceptance criteria
	if task.AcceptanceCriteria == "" {
		score.Confidence -= 0.2
		score.Issues = append(score.Issues, QualityIssue{
			Severity:   SeverityWarning,
			Message:    "No acceptance criteria specified",
			Suggestion: "Add acceptance criteria to validate task completion",
		})
	}

	// Check for missing verification intent
	if task.VerificationIntent == "" {
		score.Confidence -= 0.3
		score.Issues = append(score.Issues, QualityIssue{
			Severity:   SeverityCritical,
			Message:    "No verification intent specified",
			Suggestion: "Add verification commands or checks",
		})
	}

	// Ensure confidence stays in valid range
	if score.Confidence < 0.0 {
		score.Confidence = 0.0
	}

	return score
}

// applyGlobalPenalties applies penalties based on overall decomposition characteristics.
func applyGlobalPenalties(confidence float64, tasks []*models.Task) float64 {
	// Penalty for too many tasks
	if len(tasks) > 10 {
		penalty := float64(len(tasks)-10) * 0.05
		if penalty > 0.3 {
			penalty = 0.3 // Cap at 0.3
		}
		confidence -= penalty
	}

	// Penalty for no parallelism (all tasks depend on each other in a chain)
	parallelism := calculateParallelism(tasks)
	if parallelism == 1 && len(tasks) > 3 {
		confidence -= 0.2
	}

	// Ensure confidence stays in valid range
	if confidence < 0.0 {
		confidence = 0.0
	}
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// generateWarnings creates human-readable warnings based on task scores.
func generateWarnings(tasks []*models.Task, scores []TaskQualityScore) []string {
	warnings := []string{}

	// Count critical issues
	criticalCount := 0
	for _, score := range scores {
		for _, issue := range score.Issues {
			if issue.Severity == SeverityCritical {
				criticalCount++
			}
		}
	}
	if criticalCount > 0 {
		warnings = append(warnings, fmt.Sprintf("%d critical issues found in decomposition", criticalCount))
	}

	// Warn about too many tasks
	if len(tasks) > 10 {
		warnings = append(warnings, fmt.Sprintf("Large number of tasks (%d) may be difficult to coordinate", len(tasks)))
	}

	// Warn about low overall confidence
	totalConfidence := 0.0
	for _, score := range scores {
		totalConfidence += score.Confidence
	}
	avgConfidence := totalConfidence / float64(len(scores))
	if avgConfidence < 0.5 {
		warnings = append(warnings, "Low overall confidence - consider simplifying or restructuring tasks")
	}

	return warnings
}

// hasFileOverlap checks if two file boundary lists overlap.
func hasFileOverlap(boundaries1, boundaries2 []string) bool {
	for _, b1 := range boundaries1 {
		for _, b2 := range boundaries2 {
			if pathsOverlap(b1, b2) {
				return true
			}
		}
	}
	return false
}

// pathsOverlap checks if two file paths overlap (one is a prefix of the other).
func pathsOverlap(path1, path2 string) bool {
	// Normalize paths
	p1 := strings.TrimSuffix(path1, "/")
	p2 := strings.TrimSuffix(path2, "/")

	// Check if one is a prefix of the other
	return strings.HasPrefix(p1, p2) || strings.HasPrefix(p2, p1)
}

// calculateDependencyDepth calculates the maximum dependency chain depth for a task.
func calculateDependencyDepth(task *models.Task, allTasks []*models.Task) int {
	visited := make(map[string]bool)
	return calculateDepthRecursive(task, allTasks, visited)
}

// calculateDepthRecursive recursively calculates dependency depth.
func calculateDepthRecursive(task *models.Task, allTasks []*models.Task, visited map[string]bool) int {
	if visited[task.ID] {
		return 0 // Cycle detected, don't recurse
	}
	visited[task.ID] = true

	if len(task.DependsOn) == 0 {
		return 1
	}

	maxDepth := 0
	for _, depID := range task.DependsOn {
		dep := findTaskByID(depID, allTasks)
		if dep != nil {
			depth := calculateDepthRecursive(dep, allTasks, visited)
			if depth > maxDepth {
				maxDepth = depth
			}
		}
	}

	return maxDepth + 1
}

// findTaskByID finds a task by ID in the task list.
func findTaskByID(id string, tasks []*models.Task) *models.Task {
	for _, task := range tasks {
		if task.ID == id {
			return task
		}
	}
	return nil
}

// calculateParallelism estimates the maximum number of tasks that can run in parallel.
// This is a simplified calculation: count tasks with no dependencies.
func calculateParallelism(tasks []*models.Task) int {
	// Find tasks with no dependencies
	independentCount := 0
	for _, task := range tasks {
		if len(task.DependsOn) == 0 {
			independentCount++
		}
	}

	if independentCount == 0 {
		return 1 // At least one task can run
	}
	return independentCount
}

// EnhanceWithProtectedAreaWarnings adds protected area warnings to a quality result.
// This checks if any tasks touch protected areas (based on 4 detection strategies)
// and adds appropriate warnings.
func EnhanceWithProtectedAreaWarnings(quality *DecompositionQuality, tasks []*models.Task, detector *protect.Detector) {
	if detector == nil {
		return
	}

	// Check each task's file boundaries for protected areas
	for i, task := range tasks {
		for _, boundary := range task.FileBoundaries {
			isProtected, reason := detector.IsProtectedWithReason(boundary)
			if isProtected {
				// Add warning to task score
				if i < len(quality.TaskScores) {
					quality.TaskScores[i].Issues = append(quality.TaskScores[i].Issues, QualityIssue{
						Severity:   SeverityWarning,
						Message:    fmt.Sprintf("Task touches protected area: %s", reason),
						Suggestion: "Consider manual review or enabling Scout override gates",
					})

					// Small confidence penalty for protected areas
					quality.TaskScores[i].Confidence -= 0.1
					if quality.TaskScores[i].Confidence < 0.0 {
						quality.TaskScores[i].Confidence = 0.0
					}
				}

				// Add to global warnings
				warning := fmt.Sprintf("Task '%s' touches protected area '%s': %s",
					task.Title, boundary, reason)
				quality.Warnings = append(quality.Warnings, warning)
			}
		}
	}

	// Recalculate overall confidence after adding protected area penalties
	if len(quality.TaskScores) > 0 {
		totalConfidence := 0.0
		for _, score := range quality.TaskScores {
			totalConfidence += score.Confidence
		}
		quality.OverallConfidence = totalConfidence / float64(len(quality.TaskScores))
	}
}
