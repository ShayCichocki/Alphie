// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"sync"

	"github.com/shayc/alphie/internal/config"
	"github.com/shayc/alphie/pkg/models"
)

// tierConfigsOrchestrator holds the loaded tier configurations for the orchestrator.
// This is used by QuestionsAllowed to get tier-specific question limits.
var tierConfigsOrchestrator *config.TierConfigs

// tierConfigsOrchestratorMu protects tierConfigsOrchestrator from concurrent access.
var tierConfigsOrchestratorMu sync.RWMutex

// SetOrchestratorTierConfigs updates the tier configurations used by the orchestrator.
// This should be called at startup after loading configs/*.yaml.
func SetOrchestratorTierConfigs(configs *config.TierConfigs) {
	tierConfigsOrchestratorMu.Lock()
	defer tierConfigsOrchestratorMu.Unlock()
	tierConfigsOrchestrator = configs
}

// ScoutOverrideGate manages conditions under which a Scout agent can ask questions.
// By default, Scout agents have zero questions allowed, but this can be overridden
// when specific conditions are met:
//   - blocked_after_n_attempts: After N failed retries (default 5)
//   - protected_area_detected: When the task touches protected code areas
type ScoutOverrideGate struct {
	// protected is the detector for protected code areas.
	protected *ProtectedAreaDetector
	// blockedAfterN is the number of failed attempts before allowing questions.
	blockedAfterN int
	// protectedAreaEnabled controls whether protected area detection allows questions.
	protectedAreaEnabled bool
	// taskAttempts tracks the number of attempts per task.
	taskAttempts map[string]int
	// taskProtected tracks whether a task touches protected areas.
	taskProtected map[string]bool
	// mu protects all mutable fields.
	mu sync.RWMutex
}

// ScoutOverrideConfig contains configuration for the ScoutOverrideGate.
type ScoutOverrideConfig struct {
	// BlockedAfterNAttempts is the number of failed attempts before allowing questions.
	// Default is 5.
	BlockedAfterNAttempts int
	// ProtectedAreaDetected enables questions when protected areas are detected.
	// Default is true.
	ProtectedAreaDetected bool
}

// DefaultScoutOverrideConfig returns the default configuration.
func DefaultScoutOverrideConfig() ScoutOverrideConfig {
	return ScoutOverrideConfig{
		BlockedAfterNAttempts: 5,
		ProtectedAreaDetected: true,
	}
}

// NewScoutOverrideGate creates a new ScoutOverrideGate with the given configuration.
func NewScoutOverrideGate(protected *ProtectedAreaDetector, cfg ScoutOverrideConfig) *ScoutOverrideGate {
	blockedAfterN := cfg.BlockedAfterNAttempts
	if blockedAfterN <= 0 {
		blockedAfterN = 5 // Default
	}

	return &ScoutOverrideGate{
		protected:            protected,
		blockedAfterN:        blockedAfterN,
		protectedAreaEnabled: cfg.ProtectedAreaDetected,
		taskAttempts:         make(map[string]int),
		taskProtected:        make(map[string]bool),
	}
}

// CanAskQuestion determines if a Scout agent can ask a question for the given task.
// Returns true if any override condition is met:
//   - The task has exceeded the blocked_after_n_attempts threshold
//   - The task touches protected areas (and protectedAreaEnabled is true)
func (g *ScoutOverrideGate) CanAskQuestion(taskID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Check if blocked after N attempts
	if attempts, ok := g.taskAttempts[taskID]; ok {
		if attempts >= g.blockedAfterN {
			return true
		}
	}

	// Check if protected area detected
	if g.protectedAreaEnabled {
		if protected, ok := g.taskProtected[taskID]; ok && protected {
			return true
		}
	}

	return false
}

// RecordAttempt increments the attempt counter for a task.
// This should be called after each failed attempt.
func (g *ScoutOverrideGate) RecordAttempt(taskID string) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.taskAttempts[taskID]++
	return g.taskAttempts[taskID]
}

// GetAttempts returns the current attempt count for a task.
func (g *ScoutOverrideGate) GetAttempts(taskID string) int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.taskAttempts[taskID]
}

// CheckProtectedArea evaluates whether a task touches protected areas
// based on the task description and file paths. The result is cached.
func (g *ScoutOverrideGate) CheckProtectedArea(taskID string, paths []string) bool {
	if g.protected == nil {
		return false
	}

	for _, path := range paths {
		if g.protected.IsProtected(path) {
			g.mu.Lock()
			g.taskProtected[taskID] = true
			g.mu.Unlock()
			return true
		}
	}

	return false
}

// SetProtectedArea explicitly marks a task as touching protected areas.
// This can be used when protected area detection happens elsewhere.
func (g *ScoutOverrideGate) SetProtectedArea(taskID string, protected bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.taskProtected[taskID] = protected
}

// IsProtectedArea returns whether a task has been marked as touching protected areas.
func (g *ScoutOverrideGate) IsProtectedArea(taskID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.taskProtected[taskID]
}

// GetOverrideReason returns a human-readable reason why questions are allowed.
// Returns an empty string if no override condition is met.
func (g *ScoutOverrideGate) GetOverrideReason(taskID string) string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Check blocked after N
	if attempts, ok := g.taskAttempts[taskID]; ok {
		if attempts >= g.blockedAfterN {
			return "blocked_after_n_attempts"
		}
	}

	// Check protected area
	if g.protectedAreaEnabled {
		if protected, ok := g.taskProtected[taskID]; ok && protected {
			return "protected_area_detected"
		}
	}

	return ""
}

// Reset clears all tracking state for a task.
// This should be called when a task completes or is cancelled.
func (g *ScoutOverrideGate) Reset(taskID string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	delete(g.taskAttempts, taskID)
	delete(g.taskProtected, taskID)
}

// GetBlockedAfterN returns the configured blocked_after_n_attempts threshold.
func (g *ScoutOverrideGate) GetBlockedAfterN() int {
	return g.blockedAfterN
}

// IsProtectedAreaEnabled returns whether protected area detection is enabled.
func (g *ScoutOverrideGate) IsProtectedAreaEnabled() bool {
	return g.protectedAreaEnabled
}

// QuestionsAllowed calculates the number of questions allowed for a tier and task.
// For Scout tier, this is normally 0 but can be overridden by gate conditions.
// For other tiers, it returns the standard allowance from loaded config.
func QuestionsAllowed(tier models.Tier, gate *ScoutOverrideGate, taskID string) int {
	// Get questions allowed from loaded config, with fallback defaults
	tierConfigsOrchestratorMu.RLock()
	cfg := tierConfigsOrchestrator
	tierConfigsOrchestratorMu.RUnlock()

	var questionsAllowed int
	if cfg != nil {
		tierCfg := cfg.Get(tier)
		if tierCfg != nil {
			questionsAllowed = tierCfg.GetQuestionsAllowedInt()
		} else {
			questionsAllowed = getDefaultQuestionsAllowed(tier)
		}
	} else {
		questionsAllowed = getDefaultQuestionsAllowed(tier)
	}

	// For Scout tier, apply override gate logic
	if tier == models.TierScout {
		if questionsAllowed == 0 && gate != nil && gate.CanAskQuestion(taskID) {
			return 1 // Allow one question when override is active
		}
	}

	return questionsAllowed
}

// getDefaultQuestionsAllowed returns hardcoded defaults for questions allowed.
// This is used as a fallback when tier configs are not loaded.
func getDefaultQuestionsAllowed(tier models.Tier) int {
	switch tier {
	case models.TierScout:
		return 0 // Scout normally cannot ask questions
	case models.TierBuilder:
		return 2 // Builder can ask 1-2 questions
	case models.TierArchitect:
		return -1 // Architect has unlimited questions (-1 = unlimited)
	default:
		return 0
	}
}
