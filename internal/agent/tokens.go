// Package agent provides the AI agent implementation for Alphie.
package agent

import (
	"fmt"
	"sync"
	"time"
)

// ModelPricing contains pricing per 1M tokens for a model.
type ModelPricing struct {
	InputPerMillion  float64 // Cost per 1M input tokens
	OutputPerMillion float64 // Cost per 1M output tokens
}

// DefaultModelPricing contains pricing for known Claude models.
var DefaultModelPricing = map[string]ModelPricing{
	"claude-opus-4-5-20251101":   {InputPerMillion: 15.00, OutputPerMillion: 75.00},
	"claude-sonnet-4-20250514":   {InputPerMillion: 3.00, OutputPerMillion: 15.00},
	"claude-3-5-sonnet-20241022": {InputPerMillion: 3.00, OutputPerMillion: 15.00},
	"claude-3-5-haiku-20241022":  {InputPerMillion: 0.80, OutputPerMillion: 4.00},
}

// TokenUsage represents aggregated token usage information.
type TokenUsage struct {
	// InputTokens is the total input tokens used.
	InputTokens int64
	// OutputTokens is the total output tokens used.
	OutputTokens int64
	// TotalTokens is InputTokens + OutputTokens.
	TotalTokens int64
}

// TokenEvent represents a token tracking event for audit trail.
type TokenEvent struct {
	Timestamp    time.Time
	Type         string // "usage", "delta", "completion", "estimate"
	InputTokens  int64
	OutputTokens int64
	Source       string // "api" or "estimate"
}

// ValidationIssue represents a problem detected in token tracking.
type ValidationIssue struct {
	Severity  string // "critical", "warning", "info"
	Message   string
	Timestamp time.Time
}

// TokenTracker provides two-tier token tracking with hard (API-reported)
// and soft (estimated) counts, along with confidence indicators.
type TokenTracker struct {
	mu sync.RWMutex

	// HardTokens are tokens reported directly from the API.
	HardTokens TokenUsage

	// SoftTokens are estimated tokens (e.g., from content length).
	SoftTokens TokenUsage

	// Confidence indicates reliability of the token count (0.0-1.0).
	// 1.0 = all tokens from hard API counts
	// 0.0 = all tokens are soft estimates
	Confidence float64

	// Model is the model ID used for cost calculation.
	Model string

	// Pricing overrides default model pricing if set.
	Pricing *ModelPricing

	// Event tracking for validation
	EventLog           []TokenEvent
	LastUsageEvent     time.Time
	ExpectedEventCount int
	ActualEventCount   int
	StartTime          time.Time

	// Debug mode enables detailed event logging
	DebugMode bool

	// Validation issues
	Issues []ValidationIssue
}

// NewTokenTracker creates a new TokenTracker for the given model.
func NewTokenTracker(model string) *TokenTracker {
	return &TokenTracker{
		Model:          model,
		Confidence:     1.0, // Start confident, degrades with soft tokens
		EventLog:       make([]TokenEvent, 0),
		StartTime:      time.Now(),
		LastUsageEvent: time.Now(),
		Issues:         make([]ValidationIssue, 0),
	}
}

// EnableDebugMode enables detailed event logging for debugging.
func (t *TokenTracker) EnableDebugMode() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.DebugMode = true
}

// MessageDeltaUsage represents token usage from a message_delta.usage event.
type MessageDeltaUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// Update processes a message_delta.usage event with hard token counts.
func (t *TokenTracker) Update(usage MessageDeltaUsage) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.HardTokens.InputTokens += usage.InputTokens
	t.HardTokens.OutputTokens += usage.OutputTokens
	t.HardTokens.TotalTokens = t.HardTokens.InputTokens + t.HardTokens.OutputTokens

	// Record event
	event := TokenEvent{
		Timestamp:    time.Now(),
		Type:         "usage",
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		Source:       "api",
	}
	t.EventLog = append(t.EventLog, event)
	t.LastUsageEvent = event.Timestamp
	t.ActualEventCount++

	t.recalculateConfidence()
}

// UpdateSoft adds soft (estimated) token counts.
func (t *TokenTracker) UpdateSoft(input, output int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.SoftTokens.InputTokens += input
	t.SoftTokens.OutputTokens += output
	t.SoftTokens.TotalTokens = t.SoftTokens.InputTokens + t.SoftTokens.OutputTokens

	// Record event if in debug mode
	if t.DebugMode {
		event := TokenEvent{
			Timestamp:    time.Now(),
			Type:         "estimate",
			InputTokens:  input,
			OutputTokens: output,
			Source:       "estimate",
		}
		t.EventLog = append(t.EventLog, event)
	}

	t.recalculateConfidence()

	// Check if confidence has degraded
	t.checkConfidenceDegradation()
}

// recalculateConfidence updates the confidence score based on hard vs soft ratio.
// Must be called with lock held.
func (t *TokenTracker) recalculateConfidence() {
	hard := t.HardTokens.TotalTokens
	soft := t.SoftTokens.TotalTokens
	total := hard + soft

	if total == 0 {
		t.Confidence = 1.0
		return
	}

	// Confidence is the proportion of tokens that are hard (API-reported)
	t.Confidence = float64(hard) / float64(total)
}

// GetUsage returns the combined token usage (hard + soft).
func (t *TokenTracker) GetUsage() TokenUsage {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return TokenUsage{
		InputTokens:  t.HardTokens.InputTokens + t.SoftTokens.InputTokens,
		OutputTokens: t.HardTokens.OutputTokens + t.SoftTokens.OutputTokens,
		TotalTokens:  t.HardTokens.TotalTokens + t.SoftTokens.TotalTokens,
	}
}

// GetHardUsage returns only the hard (API-reported) token usage.
func (t *TokenTracker) GetHardUsage() TokenUsage {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.HardTokens
}

// GetSoftUsage returns only the soft (estimated) token usage.
func (t *TokenTracker) GetSoftUsage() TokenUsage {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.SoftTokens
}

// GetConfidence returns the confidence indicator (0.0-1.0).
func (t *TokenTracker) GetConfidence() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.Confidence
}

// GetCost calculates the total cost based on model pricing.
func (t *TokenTracker) GetCost() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	pricing := t.getPricing()
	if pricing == nil {
		return 0.0
	}

	usage := TokenUsage{
		InputTokens:  t.HardTokens.InputTokens + t.SoftTokens.InputTokens,
		OutputTokens: t.HardTokens.OutputTokens + t.SoftTokens.OutputTokens,
	}

	inputCost := float64(usage.InputTokens) / 1_000_000 * pricing.InputPerMillion
	outputCost := float64(usage.OutputTokens) / 1_000_000 * pricing.OutputPerMillion

	return inputCost + outputCost
}

// getPricing returns the pricing for this tracker's model.
// Must be called with lock held.
func (t *TokenTracker) getPricing() *ModelPricing {
	if t.Pricing != nil {
		return t.Pricing
	}

	if pricing, ok := DefaultModelPricing[t.Model]; ok {
		return &pricing
	}

	return nil
}

// SetPricing sets custom pricing for cost calculation.
func (t *TokenTracker) SetPricing(pricing ModelPricing) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Pricing = &pricing
}

// Validate performs validation checks on the token tracker state.
// Returns a list of validation issues.
func (t *TokenTracker) Validate() []ValidationIssue {
	t.mu.RLock()
	defer t.mu.RUnlock()

	issues := []ValidationIssue{}
	now := time.Now()

	// Check for missing usage events
	timeSinceLastEvent := now.Sub(t.LastUsageEvent)
	runtimeDuration := now.Sub(t.StartTime)

	// Only check if tracker has been running for more than 30 seconds
	if runtimeDuration > 30*time.Second && timeSinceLastEvent > 30*time.Second && t.ExpectedEventCount > 0 {
		issues = append(issues, ValidationIssue{
			Severity:  "critical",
			Message:   fmt.Sprintf("No token usage events received in %v", timeSinceLastEvent.Round(time.Second)),
			Timestamp: now,
		})
	}

	// Check confidence level
	if t.Confidence < 0.5 {
		issues = append(issues, ValidationIssue{
			Severity:  "critical",
			Message:   fmt.Sprintf("Token tracking confidence too low: %.2f", t.Confidence),
			Timestamp: now,
		})
	} else if t.Confidence < 0.8 {
		issues = append(issues, ValidationIssue{
			Severity:  "warning",
			Message:   fmt.Sprintf("Token tracking confidence degraded: %.2f", t.Confidence),
			Timestamp: now,
		})
	}

	// Check event count mismatch (if we expected events but didn't get them)
	if t.ExpectedEventCount > 0 && t.ActualEventCount < t.ExpectedEventCount {
		shortage := t.ExpectedEventCount - t.ActualEventCount
		issues = append(issues, ValidationIssue{
			Severity:  "warning",
			Message:   fmt.Sprintf("Missing %d expected token events", shortage),
			Timestamp: now,
		})
	}

	return issues
}

// checkConfidenceDegradation checks if confidence has degraded and records issues.
// Must be called with lock held.
func (t *TokenTracker) checkConfidenceDegradation() {
	if t.Confidence < 0.8 && t.Confidence >= 0.5 {
		// Warning level
		issue := ValidationIssue{
			Severity:  "warning",
			Message:   fmt.Sprintf("Confidence degraded to %.2f - falling back to soft token estimates", t.Confidence),
			Timestamp: time.Now(),
		}
		t.Issues = append(t.Issues, issue)
	} else if t.Confidence < 0.5 {
		// Critical level
		issue := ValidationIssue{
			Severity:  "critical",
			Message:   fmt.Sprintf("Confidence critically low (%.2f) - token tracking may be unreliable", t.Confidence),
			Timestamp: time.Now(),
		}
		t.Issues = append(t.Issues, issue)
	}
}

// GetIssues returns all validation issues.
func (t *TokenTracker) GetIssues() []ValidationIssue {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return append([]ValidationIssue{}, t.Issues...) // Return a copy
}

// GetEventLog returns a copy of the event log.
func (t *TokenTracker) GetEventLog() []TokenEvent {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.DebugMode {
		return nil // Event log only available in debug mode
	}

	return append([]TokenEvent{}, t.EventLog...) // Return a copy
}

// AggregateTracker tracks tokens across multiple agents.
type AggregateTracker struct {
	mu       sync.RWMutex
	trackers map[string]*TokenTracker
}

// NewAggregateTracker creates a new aggregate tracker.
func NewAggregateTracker() *AggregateTracker {
	return &AggregateTracker{
		trackers: make(map[string]*TokenTracker),
	}
}

// Add adds a tracker for an agent ID.
func (a *AggregateTracker) Add(agentID string, tracker *TokenTracker) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.trackers[agentID] = tracker
}

// Remove removes a tracker for an agent ID.
func (a *AggregateTracker) Remove(agentID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.trackers, agentID)
}

// Get returns the tracker for an agent ID.
func (a *AggregateTracker) Get(agentID string) *TokenTracker {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.trackers[agentID]
}

// GetUsage returns the combined usage across all tracked agents.
func (a *AggregateTracker) GetUsage() TokenUsage {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var total TokenUsage
	for _, t := range a.trackers {
		usage := t.GetUsage()
		total.InputTokens += usage.InputTokens
		total.OutputTokens += usage.OutputTokens
		total.TotalTokens += usage.TotalTokens
	}

	return total
}

// GetHardUsage returns the combined hard (API-reported) usage across all tracked agents.
func (a *AggregateTracker) GetHardUsage() TokenUsage {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var total TokenUsage
	for _, t := range a.trackers {
		usage := t.GetHardUsage()
		total.InputTokens += usage.InputTokens
		total.OutputTokens += usage.OutputTokens
		total.TotalTokens += usage.TotalTokens
	}

	return total
}

// GetSoftUsage returns the combined soft (estimated) usage across all tracked agents.
func (a *AggregateTracker) GetSoftUsage() TokenUsage {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var total TokenUsage
	for _, t := range a.trackers {
		usage := t.GetSoftUsage()
		total.InputTokens += usage.InputTokens
		total.OutputTokens += usage.OutputTokens
		total.TotalTokens += usage.TotalTokens
	}

	return total
}

// GetCost returns the combined cost across all tracked agents.
func (a *AggregateTracker) GetCost() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var total float64
	for _, t := range a.trackers {
		total += t.GetCost()
	}

	return total
}

// GetConfidence returns the weighted average confidence across all agents.
// Agents with more tokens have more weight.
func (a *AggregateTracker) GetConfidence() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var weightedSum float64
	var totalWeight int64

	for _, t := range a.trackers {
		usage := t.GetUsage()
		weight := usage.TotalTokens
		if weight > 0 {
			weightedSum += t.GetConfidence() * float64(weight)
			totalWeight += weight
		}
	}

	if totalWeight == 0 {
		return 1.0
	}

	return weightedSum / float64(totalWeight)
}

// Count returns the number of tracked agents.
func (a *AggregateTracker) Count() int {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return len(a.trackers)
}
