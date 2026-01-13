// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"sync"

	"github.com/shayc/alphie/internal/agent"
)

// BudgetStatus represents the current state of budget consumption.
type BudgetStatus int

const (
	// BudgetOK indicates usage is below the warning threshold (<80%).
	BudgetOK BudgetStatus = iota
	// BudgetWarning indicates usage is between warning and exhaustion (80-99%).
	BudgetWarning
	// BudgetExhausted indicates budget is fully consumed (>=100%).
	BudgetExhausted
)

// String returns a human-readable representation of the budget status.
func (s BudgetStatus) String() string {
	switch s {
	case BudgetOK:
		return "OK"
	case BudgetWarning:
		return "Warning"
	case BudgetExhausted:
		return "Exhausted"
	default:
		return "Unknown"
	}
}

// DefaultWarningThreshold is the default percentage at which warnings begin.
const DefaultWarningThreshold = 0.80

// BudgetHandler monitors token usage against a configured budget and provides
// graceful wind-down when the budget is exhausted.
type BudgetHandler struct {
	// budget is the maximum allowed tokens.
	budget int64
	// used is the current token consumption.
	used int64
	// tracker is the aggregate token tracker for all agents.
	tracker *agent.AggregateTracker
	// warningThreshold is the percentage (0.0-1.0) at which warnings begin.
	warningThreshold float64
	// exhausted indicates if OnExhausted has been called.
	exhausted bool
	// mu protects mutable state.
	mu sync.RWMutex
}

// NewBudgetHandler creates a new BudgetHandler with the specified token budget.
func NewBudgetHandler(budget int64) *BudgetHandler {
	return &BudgetHandler{
		budget:           budget,
		warningThreshold: DefaultWarningThreshold,
	}
}

// SetTracker sets the aggregate token tracker for monitoring usage.
func (h *BudgetHandler) SetTracker(tracker *agent.AggregateTracker) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.tracker = tracker
}

// Update adds the specified number of tokens to the usage counter.
// This is called when token usage is reported from agents.
func (h *BudgetHandler) Update(tokensUsed int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.used += tokensUsed
}

// syncFromTracker updates the internal used count from the tracker if available.
// Must be called with lock held.
func (h *BudgetHandler) syncFromTracker() {
	if h.tracker != nil {
		usage := h.tracker.GetUsage()
		h.used = usage.TotalTokens
	}
}

// CheckBudget returns the current budget status based on usage percentage.
// Returns:
//   - BudgetOK: usage < 80% (or configured warning threshold)
//   - BudgetWarning: usage 80-99%
//   - BudgetExhausted: usage >= 100%
func (h *BudgetHandler) CheckBudget() BudgetStatus {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.syncFromTracker()

	if h.budget <= 0 {
		return BudgetOK // No budget limit set
	}

	percentage := float64(h.used) / float64(h.budget)

	if percentage >= 1.0 {
		return BudgetExhausted
	}
	if percentage >= h.warningThreshold {
		return BudgetWarning
	}
	return BudgetOK
}

// GetUsage returns the current usage statistics.
// Returns: used tokens, total budget, and usage percentage (0.0-1.0).
func (h *BudgetHandler) GetUsage() (used, budget int64, percentage float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.syncFromTracker()

	used = h.used
	budget = h.budget

	if budget <= 0 {
		percentage = 0.0
	} else {
		percentage = float64(used) / float64(budget)
	}

	return used, budget, percentage
}

// CanStartNew returns true if new tasks can be started.
// Returns false when budget is exhausted to block new task scheduling.
func (h *BudgetHandler) CanStartNew() bool {
	return h.CheckBudget() != BudgetExhausted
}

// OnExhausted is called when the budget reaches 100%.
// This signals the orchestrator to:
//  1. Block new task scheduling
//  2. Allow in-progress agents to complete their current work
//
// This method is idempotent - calling it multiple times has no additional effect.
func (h *BudgetHandler) OnExhausted() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.exhausted {
		return // Already handled
	}

	h.exhausted = true

	// The orchestrator will check IsExhausted() to determine if it should
	// block new tasks. In-progress agents are allowed to complete naturally.
}

// IsExhausted returns true if the budget has been exhausted and OnExhausted
// has been called.
func (h *BudgetHandler) IsExhausted() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.exhausted
}

// GetWarningThreshold returns the current warning threshold (0.0-1.0).
func (h *BudgetHandler) GetWarningThreshold() float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.warningThreshold
}

// SetWarningThreshold sets the warning threshold percentage (0.0-1.0).
// The threshold must be between 0 and 1; invalid values are clamped.
func (h *BudgetHandler) SetWarningThreshold(threshold float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Clamp threshold to valid range
	if threshold < 0 {
		threshold = 0
	}
	if threshold > 1 {
		threshold = 1
	}

	h.warningThreshold = threshold
}

// Reset clears the usage counter and exhausted flag.
// This is useful for testing or when starting a new session.
func (h *BudgetHandler) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.used = 0
	h.exhausted = false
}

// SetBudget updates the budget limit.
func (h *BudgetHandler) SetBudget(budget int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.budget = budget
}
