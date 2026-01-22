// Package decompose provides validation for task decompositions.
package decompose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ShayCichocki/alphie/pkg/models"
)

// ValidationResult contains the results of validating a decomposition.
type ValidationResult struct {
	Valid          bool
	Errors         []string
	Warnings       []string
	SuggestedFixes map[string]string // taskID -> suggested fix
}

// Validator validates task decompositions against repository structure and constraints.
type Validator struct {
	repoPath string
}

// NewValidator creates a new decomposition validator.
func NewValidator(repoPath string) *Validator {
	return &Validator{
		repoPath: repoPath,
	}
}

// Validate performs comprehensive validation on a task decomposition.
func (v *Validator) Validate(tasks []*models.Task) ValidationResult {
	result := ValidationResult{
		Valid:          true,
		Errors:         []string{},
		Warnings:       []string{},
		SuggestedFixes: make(map[string]string),
	}

	// 1. Validate cycles (critical)
	if err := ValidateNoCycles(tasks); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Dependency cycle detected: %v", err))
	}

	// 2. Validate file boundaries against actual repo structure
	v.validateFileBoundaries(tasks, &result)

	// 3. Validate task references (all dependencies exist)
	v.validateReferences(tasks, &result)

	// 4. Validate task structure (required fields)
	v.validateTaskStructure(tasks, &result)

	// 5. Check for common anti-patterns
	v.checkAntiPatterns(tasks, &result)

	return result
}

// validateFileBoundaries checks if specified file boundaries actually exist in the repository.
func (v *Validator) validateFileBoundaries(tasks []*models.Task, result *ValidationResult) {
	for _, task := range tasks {
		for _, boundary := range task.FileBoundaries {
			// Skip validation for very broad boundaries
			if boundary == "." || boundary == "./" {
				continue
			}

			fullPath := filepath.Join(v.repoPath, boundary)

			// Check if path exists
			info, err := os.Stat(fullPath)
			if err != nil {
				if os.IsNotExist(err) {
					// Path doesn't exist - check for similar paths
					suggested := v.findSimilarPath(boundary)
					if suggested != "" {
						result.Warnings = append(result.Warnings,
							fmt.Sprintf("Task '%s': File boundary '%s' does not exist. Did you mean '%s'?",
								task.Title, boundary, suggested))
						result.SuggestedFixes[task.ID] = fmt.Sprintf("Change file boundary from '%s' to '%s'", boundary, suggested)
					} else {
						result.Warnings = append(result.Warnings,
							fmt.Sprintf("Task '%s': File boundary '%s' does not exist in repository",
								task.Title, boundary))
					}
				}
				continue
			}

			// Warn if boundary is very broad (entire directory with many files)
			if info.IsDir() {
				fileCount := v.countFilesInDir(fullPath, 100) // Count up to 100 files
				if fileCount > 50 {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("Task '%s': Boundary '%s' contains %d+ files, consider narrowing scope",
							task.Title, boundary, fileCount))
				}
			}
		}
	}
}

// validateReferences checks that all task dependencies reference valid tasks.
func (v *Validator) validateReferences(tasks []*models.Task, result *ValidationResult) {
	taskIDs := make(map[string]bool)
	for _, task := range tasks {
		taskIDs[task.ID] = true
	}

	for _, task := range tasks {
		for _, depID := range task.DependsOn {
			if !taskIDs[depID] {
				result.Valid = false
				result.Errors = append(result.Errors,
					fmt.Sprintf("Task '%s': References non-existent dependency '%s'", task.Title, depID))
			}
		}
	}
}

// validateTaskStructure checks that tasks have required fields.
func (v *Validator) validateTaskStructure(tasks []*models.Task, result *ValidationResult) {
	for _, task := range tasks {
		if task.Title == "" {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("Task %s: Missing title", task.ID))
		}

		if task.Description == "" {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Task '%s': Missing description", task.Title))
		}

		if len(task.FileBoundaries) == 0 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Task '%s': No file boundaries specified (may cause merge conflicts)", task.Title))
		}
	}
}

// checkAntiPatterns looks for common problematic patterns in decompositions.
func (v *Validator) checkAntiPatterns(tasks []*models.Task, result *ValidationResult) {
	// Anti-pattern 1: All tasks depend on each other in a chain (no parallelism)
	if len(tasks) > 3 {
		parallelizable := 0
		for _, task := range tasks {
			if len(task.DependsOn) == 0 {
				parallelizable++
			}
		}
		if parallelizable <= 1 {
			result.Warnings = append(result.Warnings,
				"Decomposition has minimal parallelism - most tasks form a dependency chain")
		}
	}

	// Anti-pattern 2: Too many SETUP tasks (>30% of total)
	setupCount := 0
	for _, task := range tasks {
		if task.TaskType == models.TaskTypeSetup {
			setupCount++
		}
	}
	if float64(setupCount)/float64(len(tasks)) > 0.3 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("%d/%d tasks are SETUP - consider consolidating setup work", setupCount, len(tasks)))
	}

	// Anti-pattern 3: Very long task titles (likely too much detail)
	for _, task := range tasks {
		if len(task.Title) > 100 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Task '%s...': Title is very long (%d chars), consider shortening",
					task.Title[:50], len(task.Title)))
		}
	}

	// Anti-pattern 4: Tasks with overlapping file boundaries (high merge conflict risk)
	overlapCount := 0
	for i := 0; i < len(tasks); i++ {
		for j := i + 1; j < len(tasks); j++ {
			if hasFileOverlap(tasks[i].FileBoundaries, tasks[j].FileBoundaries) {
				overlapCount++
			}
		}
	}
	if overlapCount > len(tasks)/2 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("High file boundary overlap detected (%d pairs) - increased merge conflict risk", overlapCount))
	}
}

// findSimilarPath attempts to find a similar existing path for a typo.
func (v *Validator) findSimilarPath(boundary string) string {
	// Get parent directory
	dir := filepath.Dir(boundary)
	name := filepath.Base(boundary)

	fullDir := filepath.Join(v.repoPath, dir)
	entries, err := os.ReadDir(fullDir)
	if err != nil {
		return ""
	}

	// Look for similar names (simple string matching)
	bestMatch := ""
	bestScore := 0

	for _, entry := range entries {
		score := similarityScore(name, entry.Name())
		if score > bestScore && score > 50 { // At least 50% similar
			bestScore = score
			bestMatch = filepath.Join(dir, entry.Name())
		}
	}

	return bestMatch
}

// similarityScore calculates a simple similarity score between two strings (0-100).
func similarityScore(s1, s2 string) int {
	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)

	// Exact match
	if s1 == s2 {
		return 100
	}

	// Substring match
	if strings.Contains(s2, s1) || strings.Contains(s1, s2) {
		return 80
	}

	// Common prefix
	commonPrefix := 0
	minLen := len(s1)
	if len(s2) < minLen {
		minLen = len(s2)
	}
	for i := 0; i < minLen; i++ {
		if s1[i] == s2[i] {
			commonPrefix++
		} else {
			break
		}
	}

	return (commonPrefix * 100) / minLen
}

// countFilesInDir counts files in a directory (up to maxCount).
func (v *Validator) countFilesInDir(dirPath string, maxCount int) int {
	count := 0

	filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() {
			count++
			if count >= maxCount {
				return filepath.SkipDir // Stop after maxCount
			}
		}
		return nil
	})

	return count
}
