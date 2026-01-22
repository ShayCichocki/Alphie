// Package architect provides tools for analyzing and auditing codebases against specifications.
package architect

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/prog"
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

func (p *Planner) Plan(ctx context.Context, gaps *GapReport, projectName string, claude agent.ClaudeRunner) (*PlanResult, error) {
	if gaps == nil || len(gaps.Gaps) == 0 {
		return &PlanResult{}, nil
	}

	phases := p.groupGapsIntoPhases(ctx, gaps.Gaps, claude)

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

		for gapIdx, gap := range phase.Gaps {
			var dependsOn []string

			if gapIdx == 0 {
				for _, depPhaseIdx := range phase.DependsOnPhases {
					if depPhaseIdx < i && len(phaseTaskIDs[depPhaseIdx]) > 0 {
						dependsOn = append(dependsOn, phaseTaskIDs[depPhaseIdx][len(phaseTaskIDs[depPhaseIdx])-1])
					}
				}
			}

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

// DependencyOrderItem represents a gap with its inferred priority order.
type DependencyOrderItem struct {
	FeatureID string `json:"feature_id"`
	Priority  int    `json:"priority"`
	Reason    string `json:"reason"`
}

// DependencyOrderResponse is the structured response from Claude about gap ordering.
type DependencyOrderResponse struct {
	OrderedGaps []DependencyOrderItem `json:"ordered_gaps"`
	Rationale   string                `json:"rationale"`
}

// inferDependencyOrder uses AI to analyze gaps and determine optimal implementation order.
// Returns a map of featureID -> priority (lower number = higher priority).
func (p *Planner) inferDependencyOrder(ctx context.Context, gaps []Gap, claude agent.ClaudeRunner) (map[string]int, error) {
	if claude == nil || len(gaps) == 0 {
		return nil, nil
	}

	// Build the prompt for Claude
	var gapDescriptions strings.Builder
	gapDescriptions.WriteString("Analyze these features and determine the optimal implementation order:\n\n")
	for i, gap := range gaps {
		gapDescriptions.WriteString(fmt.Sprintf("%d. **%s** (%s)\n", i+1, gap.FeatureID, gap.Status))
		gapDescriptions.WriteString(fmt.Sprintf("   Description: %s\n", gap.Description))
		if gap.SuggestedAction != "" {
			gapDescriptions.WriteString(fmt.Sprintf("   Suggested Action: %s\n", gap.SuggestedAction))
		}
		gapDescriptions.WriteString("\n")
	}

	prompt := fmt.Sprintf(`%s
Consider:
- Foundational work (repository setup, configuration, database schema) should come first
- Backend/API work typically comes before frontend
- Dependencies between features (e.g., authentication before protected endpoints)
- Setup and infrastructure before application features

Respond with ONLY a JSON object (no markdown, no explanation), containing:
{
  "ordered_gaps": [
    {"feature_id": "...", "priority": 0, "reason": "why this comes first"},
    {"feature_id": "...", "priority": 1, "reason": "why this comes second"},
    ...
  ],
  "rationale": "Overall explanation of the ordering strategy"
}

Priority 0 = highest priority (do first), increasing numbers = lower priority (do later).
`, gapDescriptions.String())

	// Start Claude with temperature=0 for deterministic ordering
	temp := 0.0
	opts := &agent.StartOptions{
		Temperature: &temp,
	}
	if err := claude.StartWithOptions(prompt, "", opts); err != nil {
		return nil, fmt.Errorf("start claude for ordering: %w", err)
	}

	// Collect output
	var outputBuilder strings.Builder
	for event := range claude.Output() {
		switch event.Type {
		case agent.StreamEventAssistant, agent.StreamEventResult:
			if event.Message != "" {
				outputBuilder.WriteString(event.Message)
			}
		case agent.StreamEventError:
			if event.Error != "" {
				return nil, fmt.Errorf("claude error: %s", event.Error)
			}
		}
	}

	// Wait for process completion
	if err := claude.Wait(); err != nil {
		return nil, fmt.Errorf("claude wait failed: %w", err)
	}

	output := outputBuilder.String()

	// Parse the response (strip markdown code blocks if present)
	output = strings.TrimSpace(output)
	output = strings.TrimPrefix(output, "```json")
	output = strings.TrimPrefix(output, "```")
	output = strings.TrimSuffix(output, "```")
	output = strings.TrimSpace(output)

	var response DependencyOrderResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		// If parsing fails, fall back to nil
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	// Convert to map
	orderMap := make(map[string]int)
	for _, item := range response.OrderedGaps {
		orderMap[item.FeatureID] = item.Priority
	}

	return orderMap, nil
}

func (p *Planner) groupGapsIntoPhases(ctx context.Context, gaps []Gap, claude agent.ClaudeRunner) []Phase {
	if len(gaps) == 0 {
		return nil
	}

	var aiOrder map[string]int
	if claude != nil {
		var err error
		aiOrder, err = p.inferDependencyOrder(ctx, gaps, claude)
		if err != nil {
			fmt.Printf("[planner] AI ordering warning: %v, falling back to heuristic sorting\n", err)
		}
	}

	var missingGaps, partialGaps []Gap
	for _, gap := range gaps {
		if gap.Status == AuditStatusMissing {
			missingGaps = append(missingGaps, gap)
		} else {
			partialGaps = append(partialGaps, gap)
		}
	}

	p.sortGapsByMilestone(missingGaps, aiOrder)
	p.sortGapsByMilestone(partialGaps, aiOrder)

	var phases []Phase
	if len(missingGaps) > 0 {
		phases = append(phases, Phase{
			Name: "Foundation",
			Gaps: missingGaps,
		})
	}
	if len(partialGaps) > 0 {
		var deps []int
		if len(phases) > 0 {
			deps = []int{0}
		}
		phases = append(phases, Phase{
			Name:            "Refinement",
			Gaps:            partialGaps,
			DependsOnPhases: deps,
		})
	}

	return phases
}

func (p *Planner) sortGapsByMilestone(gaps []Gap, aiOrder map[string]int) {
	milestoneRegex := regexp.MustCompile(`^M(\d+)`)

	sort.SliceStable(gaps, func(i, j int) bool {
		iMatch := milestoneRegex.FindStringSubmatch(gaps[i].FeatureID)
		jMatch := milestoneRegex.FindStringSubmatch(gaps[j].FeatureID)

		if len(iMatch) > 1 && len(jMatch) > 1 {
			iNum, _ := strconv.Atoi(iMatch[1])
			jNum, _ := strconv.Atoi(jMatch[1])
			return iNum < jNum
		}
		if len(iMatch) > 1 {
			return true
		}
		if len(jMatch) > 1 {
			return false
		}

		if aiOrder != nil {
			iPriority, iHasOrder := aiOrder[gaps[i].FeatureID]
			jPriority, jHasOrder := aiOrder[gaps[j].FeatureID]
			if iHasOrder && jHasOrder {
				return iPriority < jPriority
			}
			if iHasOrder {
				return true
			}
			if jHasOrder {
				return false
			}
		}

		return gaps[i].FeatureID < gaps[j].FeatureID
	})
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
