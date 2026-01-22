// Package agent provides agent execution and lifecycle management.
package agent

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ShayCichocki/alphie/internal/learning"
)

// RetryDecision represents the decision after evaluating a failure.
type RetryDecision int

const (
	// Retry indicates the agent should retry with a suggested fix or alternative strategy.
	Retry RetryDecision = iota
	// Escalate indicates the failure should be escalated to a human.
	Escalate
	// Abort indicates the agent should stop without escalation.
	Abort
)

// String returns a human-readable representation of the retry decision.
func (d RetryDecision) String() string {
	switch d {
	case Retry:
		return "retry"
	case Escalate:
		return "escalate"
	case Abort:
		return "abort"
	default:
		return "unknown"
	}
}

// RetryContext contains information about the current retry state and suggestions.
type RetryContext struct {
	// AgentID is the identifier of the agent that failed.
	AgentID string
	// Error is the error message from the failure.
	Error string
	// Attempt is the current attempt number (1-indexed).
	Attempt int
	// SuggestedFix is a suggested fix from learnings, if found.
	SuggestedFix string
	// Strategy describes what approach to try next.
	Strategy string
	// Learnings contains any relevant learnings found for this error.
	Learnings []*learning.Learning
}

// RetryHandler manages failure handling and retry logic for agents.
// It uses a tiered strategy: first retrying with the original approach,
// then searching learnings for known fixes, and finally escalating to human.
type RetryHandler struct {
	learnings   *learning.LearningSystem
	maxAttempts int
	attempts    map[string]int       // agentID -> attempt count
	errors      map[string][]string  // agentID -> list of errors encountered
	mu          sync.RWMutex
}

// NewRetryHandler creates a new RetryHandler with the given learning system.
// The learning system is used to search for known fixes when failures occur.
func NewRetryHandler(learnings *learning.LearningSystem) *RetryHandler {
	return &RetryHandler{
		learnings:   learnings,
		maxAttempts: 5,
		attempts:    make(map[string]int),
		errors:      make(map[string][]string),
	}
}

// SetMaxAttempts sets the maximum number of attempts before escalation.
// The default is 5 attempts.
func (h *RetryHandler) SetMaxAttempts(max int) {
	if max < 1 {
		max = 1
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.maxAttempts = max
}

// MaxAttempts returns the current maximum attempts setting.
func (h *RetryHandler) MaxAttempts() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.maxAttempts
}

// HandleFailure evaluates a failure and returns a retry context and decision.
// The tiered strategy is:
//   - Attempt 1: Retry with original approach
//   - Attempts 2-4: Search learnings for similar errors, apply suggested fix or try alternative
//   - Attempt 5+: Escalate to human
func (h *RetryHandler) HandleFailure(agentID string, errorMsg string) (*RetryContext, RetryDecision) {
	h.mu.Lock()
	// Increment attempt counter
	h.attempts[agentID]++
	attempt := h.attempts[agentID]

	// Store error for context
	h.errors[agentID] = append(h.errors[agentID], errorMsg)

	maxAttempts := h.maxAttempts
	h.mu.Unlock()

	ctx := &RetryContext{
		AgentID: agentID,
		Error:   errorMsg,
		Attempt: attempt,
	}

	// Attempt 1: Simple retry with original approach
	if attempt == 1 {
		ctx.Strategy = "retry_original"
		log.Printf("[retry] agent %s: attempt %d, trying original approach again", agentID, attempt)
		return ctx, Retry
	}

	// Attempts 2 to maxAttempts-1: Search learnings and try alternatives
	if attempt < maxAttempts {
		// Search learnings for similar errors
		if h.learnings != nil {
			learnings, err := h.learnings.OnFailure(errorMsg)
			if err == nil && len(learnings) > 0 {
				// Found relevant learnings - use the best match
				best := learnings[0]
				ctx.Learnings = learnings
				ctx.SuggestedFix = best.Action
				ctx.Strategy = "apply_learning"
				log.Printf("[retry] agent %s: attempt %d, applying learning fix: %s", agentID, attempt, best.Action)
				return ctx, Retry
			}
		}

		// No learnings found - try alternative strategy based on attempt number
		ctx.Strategy = h.selectAlternativeStrategy(attempt)
		log.Printf("[retry] agent %s: attempt %d, trying alternative strategy: %s", agentID, attempt, ctx.Strategy)
		return ctx, Retry
	}

	// Final attempt reached - escalate
	ctx.Strategy = "escalate_to_human"
	log.Printf("[retry] agent %s: max attempts (%d) reached, escalating to human", agentID, maxAttempts)
	return ctx, Escalate
}

// selectAlternativeStrategy returns a strategy name based on the attempt number.
// This provides a progression of different approaches to try.
func (h *RetryHandler) selectAlternativeStrategy(attempt int) string {
	strategies := []string{
		"retry_with_context",     // Attempt 2: Include more context
		"simplify_approach",      // Attempt 3: Try simpler approach
		"decompose_task",         // Attempt 4: Break into smaller tasks
	}

	idx := attempt - 2 // Offset by 2 since attempt 1 is original
	if idx < 0 {
		idx = 0
	}
	if idx >= len(strategies) {
		idx = len(strategies) - 1
	}

	return strategies[idx]
}

// OnRetry should be called when an agent is about to retry.
// It logs the retry and returns the current attempt number.
func (h *RetryHandler) OnRetry(agentID string) int {
	h.mu.RLock()
	attempt := h.attempts[agentID]
	h.mu.RUnlock()

	log.Printf("[retry] agent %s: executing retry attempt %d", agentID, attempt)
	return attempt
}

// OnEscalate should be called when a failure is escalated to human.
// It captures all error context and optionally stores a learning candidate.
func (h *RetryHandler) OnEscalate(agentID string) *EscalationContext {
	h.mu.Lock()
	defer h.mu.Unlock()

	errors := make([]string, len(h.errors[agentID]))
	copy(errors, h.errors[agentID])
	attempts := h.attempts[agentID]

	ctx := &EscalationContext{
		AgentID:      agentID,
		Attempts:     attempts,
		Errors:       errors,
		EscalatedAt:  time.Now(),
		NeedsLearning: true, // Flag that a new learning might be useful
	}

	log.Printf("[retry] agent %s: escalated after %d attempts with %d unique errors",
		agentID, attempts, len(errors))

	return ctx
}

// Reset clears the attempt counter and error history for an agent.
// This should be called when an agent successfully completes a task
// or when starting a completely new task.
func (h *RetryHandler) Reset(agentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.attempts, agentID)
	delete(h.errors, agentID)

	log.Printf("[retry] agent %s: retry state reset", agentID)
}

// GetAttempts returns the current attempt count for an agent.
func (h *RetryHandler) GetAttempts(agentID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.attempts[agentID]
}

// GetErrors returns all errors encountered by an agent.
func (h *RetryHandler) GetErrors(agentID string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	errors := make([]string, len(h.errors[agentID]))
	copy(errors, h.errors[agentID])
	return errors
}

// EscalationContext contains all information needed when escalating to a human.
type EscalationContext struct {
	// AgentID is the identifier of the agent that was escalated.
	AgentID string
	// Attempts is the total number of attempts made.
	Attempts int
	// Errors is the list of all errors encountered during retries.
	Errors []string
	// EscalatedAt is when the escalation occurred.
	EscalatedAt time.Time
	// NeedsLearning indicates if a new learning should be created from the resolution.
	NeedsLearning bool
}

// Summary returns a human-readable summary of the escalation.
func (e *EscalationContext) Summary() string {
	uniqueErrors := make(map[string]bool)
	for _, err := range e.Errors {
		uniqueErrors[err] = true
	}

	return fmt.Sprintf("Agent %s failed after %d attempts with %d unique errors. Latest error: %s",
		e.AgentID, e.Attempts, len(uniqueErrors), e.latestError())
}

// latestError returns the most recent error or a placeholder if none.
func (e *EscalationContext) latestError() string {
	if len(e.Errors) == 0 {
		return "(no error recorded)"
	}
	return e.Errors[len(e.Errors)-1]
}
