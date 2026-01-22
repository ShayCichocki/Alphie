package agent

import (
	"fmt"
	"strings"

	"github.com/ShayCichocki/alphie/pkg/models"
)

// buildPrompt constructs the prompt for the Claude Code agent.
func (e *Executor) buildPrompt(task *models.Task, tier models.Tier, opts *ExecuteOptions) string {
	var sb strings.Builder

	// Inject scope guidance at task start to prevent scope creep
	sb.WriteString(ScopeGuidancePrompt)
	sb.WriteString("\n")

	sb.WriteString("You are working on a task.\n\n")
	sb.WriteString("Task ID: ")
	sb.WriteString(task.ID)
	sb.WriteString("\n")
	sb.WriteString("Title: ")
	sb.WriteString(task.Title)
	sb.WriteString("\n")

	if task.Description != "" {
		sb.WriteString("\nDescription:\n")
		sb.WriteString(task.Description)
		sb.WriteString("\n")
	}

	// Add file boundary constraints if specified
	if len(task.FileBoundaries) > 0 {
		sb.WriteString("\n## CRITICAL: File Boundary Constraints\n\n")
		sb.WriteString("You MUST ONLY create or modify files within these boundaries:\n\n")
		for _, boundary := range task.FileBoundaries {
			sb.WriteString(fmt.Sprintf("- `%s`\n", boundary))
		}
		sb.WriteString("\n**DO NOT**:\n")
		sb.WriteString("- Create files outside these directories\n")
		sb.WriteString("- Create files at project root unless boundaries include it\n")
		sb.WriteString("- Move or copy files to locations outside boundaries\n\n")
		sb.WriteString("Violating these constraints will cause verification to fail.\n")
	}

	sb.WriteString("\nTier: ")
	sb.WriteString(string(tier))
	sb.WriteString("\n")

	// Add tier-specific guidance
	switch tier {
	case models.TierScout:
		sb.WriteString("\nYou are operating as a Scout agent. Focus on exploration, research, and lightweight tasks.\n")
	case models.TierBuilder:
		sb.WriteString("\nYou are operating as a Builder agent. Focus on implementation and standard development tasks.\n")
	case models.TierArchitect:
		sb.WriteString("\nYou are operating as an Architect agent. Focus on complex design, architecture, and system-level decisions.\n")
	}

	// Inject relevant learnings if available
	if opts != nil && len(opts.Learnings) > 0 {
		sb.WriteString("\n## Relevant Learnings\n")
		sb.WriteString("The following learnings from previous experiences may be helpful:\n\n")
		for i, l := range opts.Learnings {
			sb.WriteString(fmt.Sprintf("### Learning %d\n", i+1))
			sb.WriteString(fmt.Sprintf("- **When**: %s\n", l.Condition))
			sb.WriteString(fmt.Sprintf("- **Do**: %s\n", l.Action))
			sb.WriteString(fmt.Sprintf("- **Result**: %s\n", l.Outcome))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\nPlease complete this task. When finished, provide a summary of what was done.\n")

	return sb.String()
}
