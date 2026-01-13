// Package architect provides tools for analyzing and auditing codebases against specifications.
package architect

import (
	"context"
	"fmt"
	"strings"

	"github.com/shayc/alphie/internal/prog"
)

// PlanResult contains the IDs of created prog items.
type PlanResult struct {
	// EpicID is the ID of the created epic that groups all tasks.
	EpicID string
	// TaskIDs is the list of created task IDs in dependency order.
	TaskIDs []string
}

// Planner generates prog epics and tasks from audit gaps.
type Planner struct {
	client *prog.Client
}

// NewPlanner creates a new Planner with the given prog client.
func NewPlanner(client *prog.Client) *Planner {
	return &Planner{
		client: client,
	}
}

// Phase represents a group of related gaps that can be worked on together.
type Phase struct {
	// Name is a descriptive name for this phase.
	Name string
	// Gaps contains the gaps in this phase.
	Gaps []Gap
	// DependsOnPhases lists phase indices this phase depends on.
	DependsOnPhases []int
}

// Plan generates prog epics and tasks from the gap report.
// It groups related gaps into logical phases and creates tasks with appropriate dependencies.
func (p *Planner) Plan(ctx context.Context, gaps *GapReport, projectName string) (*PlanResult, error) {
	if gaps == nil || len(gaps.Gaps) == 0 {
		return &PlanResult{}, nil
	}

	// Group gaps into phases
	phases := p.groupGapsIntoPhases(gaps.Gaps)

	// Create epic for the entire implementation
	epicTitle := p.generateEpicTitle(gaps)
	epicDesc := p.generateEpicDescription(gaps, phases)

	epicID, err := p.client.CreateEpic(epicTitle, &prog.EpicOptions{
		Project:     projectName,
		Description: epicDesc,
		Priority:    2,
	})
	if err != nil {
		return nil, fmt.Errorf("create epic: %w", err)
	}

	// Create tasks for each phase
	result := &PlanResult{
		EpicID:  epicID,
		TaskIDs: make([]string, 0, len(gaps.Gaps)),
	}

	// Track task IDs by phase for dependency management
	phaseTaskIDs := make([][]string, len(phases))

	for i, phase := range phases {
		phaseTaskIDs[i] = make([]string, 0, len(phase.Gaps))

		for _, gap := range phase.Gaps {
			// Determine dependencies for this task
			var dependsOn []string
			for _, depPhaseIdx := range phase.DependsOnPhases {
				if depPhaseIdx < i && len(phaseTaskIDs[depPhaseIdx]) > 0 {
					// Depend on the last task of the dependent phase
					dependsOn = append(dependsOn, phaseTaskIDs[depPhaseIdx][len(phaseTaskIDs[depPhaseIdx])-1])
				}
			}

			// Create task for this gap
			taskTitle := p.generateTaskTitle(gap)
			taskDesc := p.generateTaskDescription(gap)

			taskID, err := p.client.CreateTask(taskTitle, &prog.TaskOptions{
				Project:     projectName,
				Description: taskDesc,
				Priority:    p.gapPriority(gap),
				ParentID:    epicID,
				DependsOn:   dependsOn,
			})
			if err != nil {
				return result, fmt.Errorf("create task for gap %s: %w", gap.FeatureID, err)
			}

			phaseTaskIDs[i] = append(phaseTaskIDs[i], taskID)
			result.TaskIDs = append(result.TaskIDs, taskID)
		}
	}

	return result, nil
}

// groupGapsIntoPhases organizes gaps into logical phases based on status and dependencies.
// MISSING gaps are prioritized before PARTIAL gaps since they represent foundational work.
func (p *Planner) groupGapsIntoPhases(gaps []Gap) []Phase {
	if len(gaps) == 0 {
		return nil
	}

	// Separate gaps by status
	var missingGaps, partialGaps []Gap
	for _, gap := range gaps {
		if gap.Status == AuditStatusMissing {
			missingGaps = append(missingGaps, gap)
		} else {
			partialGaps = append(partialGaps, gap)
		}
	}

	var phases []Phase

	// Phase 1: Missing implementations (foundational work)
	if len(missingGaps) > 0 {
		phases = append(phases, Phase{
			Name:            "Foundation",
			Gaps:            missingGaps,
			DependsOnPhases: nil,
		})
	}

	// Phase 2: Partial implementations (refinement work)
	if len(partialGaps) > 0 {
		var deps []int
		if len(phases) > 0 {
			deps = []int{0} // Depends on foundation phase
		}
		phases = append(phases, Phase{
			Name:            "Refinement",
			Gaps:            partialGaps,
			DependsOnPhases: deps,
		})
	}

	return phases
}

// generateEpicTitle creates a title for the implementation epic.
func (p *Planner) generateEpicTitle(gaps *GapReport) string {
	missingCount := 0
	partialCount := 0
	for _, gap := range gaps.Gaps {
		if gap.Status == AuditStatusMissing {
			missingCount++
		} else {
			partialCount++
		}
	}

	if missingCount > 0 && partialCount > 0 {
		return fmt.Sprintf("Implement %d missing and refine %d partial features", missingCount, partialCount)
	} else if missingCount > 0 {
		return fmt.Sprintf("Implement %d missing features", missingCount)
	}
	return fmt.Sprintf("Refine %d partial features", partialCount)
}

// generateEpicDescription creates a detailed description for the epic.
func (p *Planner) generateEpicDescription(gaps *GapReport, phases []Phase) string {
	var sb strings.Builder

	sb.WriteString("## Overview\n\n")
	if gaps.Summary != "" {
		sb.WriteString(gaps.Summary)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Phases\n\n")
	for i, phase := range phases {
		sb.WriteString(fmt.Sprintf("### Phase %d: %s\n\n", i+1, phase.Name))
		for _, gap := range phase.Gaps {
			sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", gap.FeatureID, gap.Status, gap.Description))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// generateTaskTitle creates a title for a gap task.
func (p *Planner) generateTaskTitle(gap Gap) string {
	action := "Implement"
	if gap.Status == AuditStatusPartial {
		action = "Complete"
	}
	return fmt.Sprintf("%s %s", action, gap.FeatureID)
}

// generateTaskDescription creates a detailed description for a gap task.
func (p *Planner) generateTaskDescription(gap Gap) string {
	var sb strings.Builder

	sb.WriteString("## Gap Details\n\n")
	sb.WriteString(fmt.Sprintf("**Status:** %s\n\n", gap.Status))
	sb.WriteString(fmt.Sprintf("**Description:** %s\n\n", gap.Description))

	if gap.SuggestedAction != "" {
		sb.WriteString("## Suggested Action\n\n")
		sb.WriteString(gap.SuggestedAction)
		sb.WriteString("\n")
	}

	return sb.String()
}

// gapPriority determines the priority for a gap task.
// MISSING gaps get higher priority than PARTIAL gaps.
func (p *Planner) gapPriority(gap Gap) int {
	if gap.Status == AuditStatusMissing {
		return 1 // High priority
	}
	return 2 // Medium priority
}
