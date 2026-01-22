// Package decompose provides interactive review of task decompositions.
package decompose

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ShayCichocki/alphie/pkg/models"
)

// ReviewDecision represents the user's decision after reviewing a decomposition.
type ReviewDecision struct {
	Approved bool
	Modified bool
	Tasks    []*models.Task // Modified task list (if Modified is true)
	Reason   string         // Reason for rejection (if Approved is false)
}

// DecompositionReviewer provides interactive review of decompositions.
type DecompositionReviewer struct {
	reader *bufio.Reader
}

// NewDecompositionReviewer creates a new decomposition reviewer.
func NewDecompositionReviewer() *DecompositionReviewer {
	return &DecompositionReviewer{
		reader: bufio.NewReader(os.Stdin),
	}
}

// Review presents a decomposition to the user for review and approval.
func (dr *DecompositionReviewer) Review(
	ctx context.Context,
	tasks []*models.Task,
	quality DecompositionQuality,
	validation ValidationResult,
) (ReviewDecision, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ReviewDecision{}, ctx.Err()
	default:
	}

	// Display decomposition
	dr.displayHeader()
	dr.displayQualitySummary(quality)
	dr.displayValidationResults(validation)
	dr.displayTaskList(tasks, quality.TaskScores)
	dr.displayDependencyGraph(tasks)

	// Present options
	return dr.promptForDecision(tasks, quality, validation)
}

// displayHeader shows the review header.
func (dr *DecompositionReviewer) displayHeader() {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("DECOMPOSITION REVIEW")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()
}

// displayQualitySummary shows the quality assessment.
func (dr *DecompositionReviewer) displayQualitySummary(quality DecompositionQuality) {
	fmt.Println("Quality Assessment:")
	fmt.Printf("  Overall Confidence: %.1f%% ", quality.OverallConfidence*100)
	if quality.OverallConfidence >= 0.7 {
		fmt.Println("✓ GOOD")
	} else if quality.OverallConfidence >= 0.5 {
		fmt.Println("⚠ MODERATE")
	} else {
		fmt.Println("✗ LOW")
	}

	fmt.Printf("  Total Tasks: %d\n", quality.TotalTasks)
	fmt.Printf("  Estimated Parallelism: %d tasks can run concurrently\n", quality.EstimatedParallelism)
	fmt.Printf("  Critical Issues: %d\n", quality.CriticalIssues)

	if len(quality.Warnings) > 0 {
		fmt.Println("\n  Warnings:")
		for _, warning := range quality.Warnings {
			fmt.Printf("    ⚠ %s\n", warning)
		}
	}
	fmt.Println()
}

// displayValidationResults shows validation errors and warnings.
func (dr *DecompositionReviewer) displayValidationResults(validation ValidationResult) {
	if !validation.Valid {
		fmt.Println("Validation FAILED:")
		for _, err := range validation.Errors {
			fmt.Printf("  ✗ %s\n", err)
		}
		fmt.Println()
	} else if len(validation.Warnings) > 0 {
		fmt.Println("Validation Warnings:")
		for _, warning := range validation.Warnings {
			fmt.Printf("  ⚠ %s\n", warning)
		}
		fmt.Println()
	}
}

// displayTaskList shows all tasks with their scores.
func (dr *DecompositionReviewer) displayTaskList(tasks []*models.Task, scores []TaskQualityScore) {
	fmt.Println(strings.Repeat("-", 80))
	fmt.Println("Task List:")
	fmt.Println()

	// Create task ID to score map for quick lookup
	scoreMap := make(map[string]TaskQualityScore)
	for _, score := range scores {
		scoreMap[score.TaskID] = score
	}

	for i, task := range tasks {
		score, hasScore := scoreMap[task.ID]
		confidence := "N/A"
		if hasScore {
			confidence = fmt.Sprintf("%.0f%%", score.Confidence*100)
		}

		fmt.Printf("[%d] %s\n", i+1, task.Title)
		fmt.Printf("    Type: %s | Confidence: %s\n", task.TaskType, confidence)

		if len(task.FileBoundaries) > 0 {
			fmt.Printf("    Files: %s\n", strings.Join(task.FileBoundaries, ", "))
		}

		if len(task.DependsOn) > 0 {
			depTitles := make([]string, 0, len(task.DependsOn))
			for _, depID := range task.DependsOn {
				depTask := findTaskByID(depID, tasks)
				if depTask != nil {
					depTitles = append(depTitles, depTask.Title)
				}
			}
			fmt.Printf("    Depends on: %s\n", strings.Join(depTitles, ", "))
		}

		if hasScore && len(score.Issues) > 0 {
			fmt.Println("    Issues:")
			for _, issue := range score.Issues {
				icon := "ℹ"
				if issue.Severity == SeverityCritical {
					icon = "✗"
				} else if issue.Severity == SeverityWarning {
					icon = "⚠"
				}
				fmt.Printf("      %s %s\n", icon, issue.Message)
			}
		}

		fmt.Println()
	}
}

// displayDependencyGraph shows a simple text-based dependency visualization.
func (dr *DecompositionReviewer) displayDependencyGraph(tasks []*models.Task) {
	fmt.Println(strings.Repeat("-", 80))
	fmt.Println("Dependency Graph:")
	fmt.Println()

	// Group tasks by dependency level
	levels := dr.computeDependencyLevels(tasks)

	for level := 0; level < len(levels); level++ {
		if len(levels[level]) == 0 {
			continue
		}

		fmt.Printf("Level %d: ", level+1)
		taskNames := make([]string, 0, len(levels[level]))
		for _, task := range levels[level] {
			taskNames = append(taskNames, task.Title)
		}
		fmt.Println(strings.Join(taskNames, ", "))
	}

	fmt.Println()
}

// computeDependencyLevels groups tasks by their dependency depth.
func (dr *DecompositionReviewer) computeDependencyLevels(tasks []*models.Task) [][]*models.Task {
	levels := make([][]*models.Task, 0)
	remaining := make([]*models.Task, len(tasks))
	copy(remaining, tasks)
	completed := make(map[string]bool)

	for len(remaining) > 0 {
		levelTasks := []*models.Task{}

		// Find tasks whose dependencies are all completed
		newRemaining := []*models.Task{}
		for _, task := range remaining {
			canRun := true
			for _, depID := range task.DependsOn {
				if !completed[depID] {
					canRun = false
					break
				}
			}

			if canRun {
				levelTasks = append(levelTasks, task)
				completed[task.ID] = true
			} else {
				newRemaining = append(newRemaining, task)
			}
		}

		if len(levelTasks) == 0 {
			// Cycle or orphaned tasks - add remaining as final level
			levels = append(levels, remaining)
			break
		}

		levels = append(levels, levelTasks)
		remaining = newRemaining
	}

	return levels
}

// promptForDecision asks the user to approve, modify, or reject the decomposition.
func (dr *DecompositionReviewer) promptForDecision(
	tasks []*models.Task,
	quality DecompositionQuality,
	validation ValidationResult,
) (ReviewDecision, error) {
	fmt.Println(strings.Repeat("-", 80))
	fmt.Println("Review Decision:")
	fmt.Println("  1. Approve  - Proceed with this decomposition")
	fmt.Println("  2. Modify   - Edit tasks (NOT YET IMPLEMENTED)")
	fmt.Println("  3. Reject   - Reject decomposition and provide feedback")
	fmt.Println("  4. Details  - View detailed task information")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Println()

	for {
		fmt.Print("Enter your choice (1-4): ")
		line, err := dr.reader.ReadString('\n')
		if err != nil {
			return ReviewDecision{}, fmt.Errorf("failed to read input: %w", err)
		}

		line = strings.TrimSpace(line)
		choice, err := strconv.Atoi(line)
		if err != nil || choice < 1 || choice > 4 {
			fmt.Println("Invalid choice. Please enter a number between 1 and 4.")
			continue
		}

		switch choice {
		case 1: // Approve
			// Show confirmation for low confidence decompositions
			if quality.OverallConfidence < 0.5 {
				fmt.Printf("\nWARNING: Confidence is low (%.0f%%). Are you sure? (y/n): ", quality.OverallConfidence*100)
				confirmLine, _ := dr.reader.ReadString('\n')
				if strings.ToLower(strings.TrimSpace(confirmLine)) != "y" {
					fmt.Println("Approval cancelled. Choose again.")
					continue
				}
			}

			fmt.Println("\n✓ Decomposition approved. Starting execution...")
			return ReviewDecision{
				Approved: true,
				Modified: false,
				Tasks:    tasks,
			}, nil

		case 2: // Modify
			fmt.Println("\nTask modification is not yet implemented.")
			fmt.Println("You can approve the current decomposition or reject it.")
			continue

		case 3: // Reject
			fmt.Print("\nReason for rejection: ")
			reasonLine, err := dr.reader.ReadString('\n')
			if err != nil {
				return ReviewDecision{}, fmt.Errorf("failed to read reason: %w", err)
			}

			fmt.Println("\n✗ Decomposition rejected.")
			return ReviewDecision{
				Approved: false,
				Modified: false,
				Reason:   strings.TrimSpace(reasonLine),
			}, nil

		case 4: // Details
			fmt.Print("\nEnter task number to view details (or 0 to go back): ")
			detailLine, _ := dr.reader.ReadString('\n')
			taskNum, err := strconv.Atoi(strings.TrimSpace(detailLine))
			if err != nil || taskNum < 0 || taskNum > len(tasks) {
				fmt.Println("Invalid task number.")
				continue
			}
			if taskNum == 0 {
				continue
			}

			dr.displayTaskDetails(tasks[taskNum-1], quality.TaskScores)
		}
	}
}

// displayTaskDetails shows detailed information about a single task.
func (dr *DecompositionReviewer) displayTaskDetails(task *models.Task, scores []TaskQualityScore) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Printf("Task Details: %s\n", task.Title)
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("\nDescription:\n%s\n\n", task.Description)
	fmt.Printf("Type: %s\n", task.TaskType)
	fmt.Printf("Status: %s\n", task.Status)

	if len(task.FileBoundaries) > 0 {
		fmt.Println("\nFile Boundaries:")
		for _, boundary := range task.FileBoundaries {
			fmt.Printf("  - %s\n", boundary)
		}
	}

	if len(task.DependsOn) > 0 {
		fmt.Println("\nDependencies:")
		for _, depID := range task.DependsOn {
			fmt.Printf("  - %s\n", depID)
		}
	}

	if task.AcceptanceCriteria != "" {
		fmt.Printf("\nAcceptance Criteria:\n%s\n", task.AcceptanceCriteria)
	}

	if task.VerificationIntent != "" {
		fmt.Printf("\nVerification Intent:\n%s\n", task.VerificationIntent)
	}

	// Find score for this task
	for _, score := range scores {
		if score.TaskID == task.ID {
			fmt.Printf("\nConfidence Score: %.0f%%\n", score.Confidence*100)
			if len(score.Issues) > 0 {
				fmt.Println("\nIssues:")
				for _, issue := range score.Issues {
					fmt.Printf("  [%s] %s\n", issue.Severity, issue.Message)
					if issue.Suggestion != "" {
						fmt.Printf("      → %s\n", issue.Suggestion)
					}
				}
			}
			break
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Print("\nPress Enter to continue...")
	dr.reader.ReadString('\n')
}
