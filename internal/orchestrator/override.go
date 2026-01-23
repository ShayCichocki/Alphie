// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"sync"

	"github.com/ShayCichocki/alphie/internal/orchestrator/policy"
	"github.com/ShayCichocki/alphie/internal/protect"
)

// ScoutOverrideGate manages conditions under which a Scout agent can ask questions.
// By default, Scout agents have zero questions allowed, but this can be overridden
// when specific conditions are met:
//   - blocked_after_n_attempts: After N failed retries (default 5)
//   - protected_area_detected: When the task touches protected code areas
type ScoutOverrideGate struct {
	// protected is the checker for protected code areas.
	protected *protect.Detector
	// policy contains configurable override thresholds.
	policy *policy.OverridePolicy
	// tierConfigs provides tier-specific question limits.
	tierConfigs interface{}
	// taskAttempts tracks the number of attempts per task.
	taskAttempts map[string]int
	// taskProtected tracks whether a task touches protected areas.
	taskProtected map[string]bool
	// mu protects all mutable fields.
	mu sync.RWMutex
}

// ScoutOverrideConfig contains configuration for the ScoutOverrideGate.
// Deprecated: Use policy.OverridePolicy instead.
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
	p := policy.Default().Override
	return ScoutOverrideConfig{
		BlockedAfterNAttempts: p.BlockedAfterNAttempts,
		ProtectedAreaDetected: p.ProtectedAreaDetected,
	}
}

// NewScoutOverrideGate creates a new ScoutOverrideGate with the given configuration.
func NewScoutOverrideGate(protected *protect.Detector, cfg ScoutOverrideConfig) *ScoutOverrideGate {
	p := &policy.OverridePolicy{
		BlockedAfterNAttempts: cfg.BlockedAfterNAttempts,
		ProtectedAreaDetected: cfg.ProtectedAreaDetected,
	}
	return NewScoutOverrideGateWithPolicy(protected, p, nil)
}

// NewScoutOverrideGateWithPolicy creates a new ScoutOverrideGate with policy and tier configs.
func NewScoutOverrideGateWithPolicy(protected *protect.Detector, p *policy.OverridePolicy, tierCfg interface{}) *ScoutOverrideGate {
	if p == nil {
		p = &policy.Default().Override
	}
	if p.BlockedAfterNAttempts <= 0 {
		p.BlockedAfterNAttempts = 5
	}

	return &ScoutOverrideGate{
		protected:     protected,
		policy:        p,
		tierConfigs:   tierCfg,
		taskAttempts:  make(map[string]int),
		taskProtected: make(map[string]bool),
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
		if attempts >= g.policy.BlockedAfterNAttempts {
			return true
		}
	}

	// Check if protected area detected
	if g.policy.ProtectedAreaDetected {
		if protected, ok := g.taskProtected[taskID]; ok && protected {
			return true
		}
	}

	return false
}

// CanAskQuestionWithCount checks if questions are allowed based on a provided
// execution count. This is preferred over CanAskQuestion as it uses the
// persisted Task.ExecutionCount instead of ephemeral in-memory tracking.
func (g *ScoutOverrideGate) CanAskQuestionWithCount(executionCount int) bool {
	return executionCount >= g.policy.BlockedAfterNAttempts
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
		if attempts >= g.policy.BlockedAfterNAttempts {
			return "blocked_after_n_attempts"
		}
	}

	// Check protected area
	if g.policy.ProtectedAreaDetected {
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
	return g.policy.BlockedAfterNAttempts
}

// IsProtectedAreaEnabled returns whether protected area detection is enabled.
func (g *ScoutOverrideGate) IsProtectedAreaEnabled() bool {
	return g.policy.ProtectedAreaDetected
}

// QuestionsAllowed calculates the number of questions allowed for a tier and task.
// For Scout tier, this is normally 0 but can be overridden by gate conditions.
// For other tiers, it returns the standard allowance from loaded config.
func QuestionsAllowed(tier interface{}, gate *ScoutOverrideGate, taskID string) int {
	return QuestionsAllowedWithConfig(tier, gate, taskID, nil)
}

// QuestionsAllowedWithConfig calculates questions allowed with explicit tier config.
// This is the preferred function as it doesn't rely on global state.
func QuestionsAllowedWithConfig(tier interface{}, gate *ScoutOverrideGate, taskID string, tierCfg interface{}) int {
	// Simplified: always use default questions allowed
	questionsAllowed := getDefaultQuestionsAllowed(tier)

	// For Scout tier, apply override gate logic
	if tier == nil {
		if questionsAllowed == 0 && gate != nil && gate.CanAskQuestion(taskID) {
			return 1 // Allow one question when override is active
		}
	}

	return questionsAllowed
}

// getDefaultQuestionsAllowed returns hardcoded defaults for questions allowed.
// This is used as a fallback when tier configs are not loaded.
func getDefaultQuestionsAllowed(tier interface{}) int {
	switch tier {
	case nil:
		return 0 // Scout normally cannot ask questions
	case nil:
		return 2 // Builder can ask 1-2 questions
	case nil:
		return -1 // Architect has unlimited questions (-1 = unlimited)
	default:
		return 0
	}
}
