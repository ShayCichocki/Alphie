// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"fmt"
	"log"
	"strings"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/prog"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// LearningCoordinator handles learning capture and storage from task completion.
// It analyzes task output for patterns worth learning and creates durable learnings
// with concepts for categorization, linked to tasks for evidence.
type LearningCoordinator struct {
	// progCoord provides cross-session task tracking and learning storage.
	progCoord *ProgCoordinator
	// tier is the agent tier used for concept derivation.
	tier models.Tier
}

// NewLearningCoordinator creates a new LearningCoordinator.
func NewLearningCoordinator(progCoord *ProgCoordinator, tier models.Tier) *LearningCoordinator {
	return &LearningCoordinator{
		progCoord: progCoord,
		tier:      tier,
	}
}

// CaptureOnCompletion extracts learnings from successful task completion
// and stores them via prog for cross-session knowledge retention.
func (l *LearningCoordinator) CaptureOnCompletion(task *models.Task, result *agent.ExecutionResult) {
	// Skip if prog client is not configured
	if !l.progCoord.IsConfigured() {
		return
	}

	progID := l.progCoord.TaskID(task.ID)

	// Extract learnable patterns from the task output
	learningCandidate := l.extractLearningCandidate(task, result)
	if learningCandidate == nil {
		return
	}

	// Derive concepts from the task context
	concepts := l.deriveLearningConcepts(task)

	// Create learning via prog client for cross-session durability
	learningID, err := l.progCoord.Client().AddLearning(learningCandidate.Summary, &prog.LearningOptions{
		TaskID:   progID,
		Detail:   learningCandidate.Detail,
		Concepts: concepts,
	})
	if err != nil {
		log.Printf("[orchestrator] warning: failed to capture learning for task %s: %v", task.ID, err)
		return
	}

	log.Printf("[orchestrator] captured learning %s for task %s: %s", learningID, task.ID, learningCandidate.Summary)

	// Also log to task for traceability
	l.progCoord.LogTask(task.ID, fmt.Sprintf("Captured learning: %s", learningCandidate.Summary))
}

// learningCandidate holds extracted learning information from task completion.
type learningCandidate struct {
	Summary string // Brief description of what was learned
	Detail  string // Extended details about the learning
}

// extractLearningCandidate analyzes task completion for patterns worth learning.
// It looks for:
// - Tasks that completed faster than expected
// - Tasks that used novel approaches visible in output
// - Tasks involving complex problem-solving
// - Tasks with high token efficiency
// Returns nil if no learnable pattern is detected.
func (l *LearningCoordinator) extractLearningCandidate(task *models.Task, result *agent.ExecutionResult) *learningCandidate {
	// Skip tasks with minimal output (likely trivial)
	if len(result.Output) < 100 {
		return nil
	}

	// Look for patterns indicating valuable learnings:
	// 1. Tasks that involved debugging or error resolution
	// 2. Tasks that modified configuration or setup
	// 3. Tasks that implemented significant features
	// 4. Tasks with high efficiency (low tokens for substantial output)

	// Check for debug/fix patterns in output
	output := strings.ToLower(result.Output)
	isDebugTask := strings.Contains(output, "fixed") ||
		strings.Contains(output, "resolved") ||
		strings.Contains(output, "debugging") ||
		strings.Contains(output, "error was")

	// Check for setup/config patterns
	isConfigTask := strings.Contains(output, "configured") ||
		strings.Contains(output, "setup") ||
		strings.Contains(output, "initialized")

	// Check for implementation patterns
	isImplTask := strings.Contains(output, "implemented") ||
		strings.Contains(output, "created") ||
		strings.Contains(output, "added")

	// Check for efficiency - successful completion with reasonable token usage
	isEfficient := result.TokensUsed > 0 && result.TokensUsed < 50000 && len(result.Output) > 500

	// Build learning candidate based on detected patterns
	var summary, detail string

	switch {
	case isDebugTask:
		summary = fmt.Sprintf("WHEN debugging %s DO check for similar patterns RESULT faster resolution", task.Title)
		detail = fmt.Sprintf("Task successfully resolved an issue. Output patterns indicate debugging approach used.\n\nTask: %s\nTokens used: %d\nDuration: %s",
			task.Title, result.TokensUsed, result.Duration)
	case isConfigTask:
		summary = fmt.Sprintf("WHEN setting up %s DO follow established configuration RESULT consistent setup", task.Title)
		detail = fmt.Sprintf("Task completed configuration or setup work.\n\nTask: %s\nTokens used: %d\nDuration: %s",
			task.Title, result.TokensUsed, result.Duration)
	case isImplTask && isEfficient:
		summary = fmt.Sprintf("WHEN implementing features like %s DO use efficient patterns RESULT reduced token usage", task.Title)
		detail = fmt.Sprintf("Task implemented functionality efficiently.\n\nTask: %s\nTokens used: %d\nOutput length: %d chars\nDuration: %s",
			task.Title, result.TokensUsed, len(result.Output), result.Duration)
	default:
		// No significant pattern detected
		return nil
	}

	return &learningCandidate{
		Summary: summary,
		Detail:  detail,
	}
}

// deriveLearningConcepts extracts concept names from task context for categorization.
// Concepts help organize learnings for retrieval in similar contexts.
func (l *LearningCoordinator) deriveLearningConcepts(task *models.Task) []string {
	var concepts []string

	// Add tier as a concept
	if l.tier != "" {
		concepts = append(concepts, string(l.tier))
	}

	// Extract keywords from task title for concepts
	title := strings.ToLower(task.Title)
	description := strings.ToLower(task.Description)
	combined := title + " " + description

	// Common concept patterns
	conceptKeywords := map[string]string{
		"test":        "testing",
		"debug":       "debugging",
		"fix":         "bug-fix",
		"implement":   "implementation",
		"refactor":    "refactoring",
		"config":      "configuration",
		"setup":       "setup",
		"api":         "api",
		"database":    "database",
		"frontend":    "frontend",
		"backend":     "backend",
		"security":    "security",
		"performance": "performance",
		"doc":         "documentation",
	}

	for keyword, concept := range conceptKeywords {
		if strings.Contains(combined, keyword) {
			concepts = append(concepts, concept)
		}
	}

	// Limit concepts to avoid over-categorization
	if len(concepts) > 5 {
		concepts = concepts[:5]
	}

	return concepts
}
