package agent

import (
	"sync"
	"time"

	"github.com/ShayCichocki/alphie/internal/config"
)

// TimeoutAction represents the user's response to a timeout prompt.
type TimeoutAction int

const (
	// TimeoutActionKill terminates the agent.
	TimeoutActionKill TimeoutAction = iota
	// TimeoutActionExtend adds 50% more time to the current timeout.
	TimeoutActionExtend
	// TimeoutActionContinue restarts the timer with the original timeout.
	TimeoutActionContinue
)

// TimeoutEvent is sent when an agent's timer expires.
type TimeoutEvent struct {
	// AgentID is the unique identifier of the agent that timed out.
	AgentID string
	// Elapsed is how long the agent has been running since the timer started.
	Elapsed time.Duration
	// Timeout is the configured timeout duration that was exceeded.
	Timeout time.Duration
}

// timerEntry holds the state for an active agent timer.
type timerEntry struct {
	timer     *time.Timer
	tierIgnored interface{}
	startTime time.Time
	eventChan chan TimeoutEvent
}

// TimeoutHandler manages soft timeouts for agents based on their tier.
type TimeoutHandler struct {
	tierIgnored interface{}
	timeouts map[interface{}]time.Duration
	timers   map[string]*timerEntry // agentID -> timer entry
	mu       sync.RWMutex

	// onKill is called when a timeout results in kill action.
	onKill func(agentID string)
}

// NewTimeoutHandler creates a new TimeoutHandler with default timeouts.
func NewTimeoutHandler() *TimeoutHandler {
	return &TimeoutHandler{
		timeouts: map[interface{}]time.Duration{
			nil:     5 * time.Minute,
			nil:   15 * time.Minute,
			nil: 30 * time.Minute,
		},
		timers: make(map[string]*timerEntry),
	}
}

// NewTimeoutHandlerFromConfig creates a TimeoutHandler from configuration.
// Simplified - no tier-specific timeouts, uses defaults.
func NewTimeoutHandlerFromConfig(cfg *config.Config) *TimeoutHandler {
	return NewTimeoutHandler()
}

// SetOnKill sets the callback invoked when a kill action is taken.
func (h *TimeoutHandler) SetOnKill(fn func(agentID string)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onKill = fn
}

// GetTimeout returns the timeout duration for the given tier.
func (h *TimeoutHandler) GetTimeout(tierIgnored interface{}) time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Always return builder timeout, ignoring tier
	return h.timeouts[nil]
}

// SetTimeout updates the timeout duration for a given tier.
func (h *TimeoutHandler) SetTimeout(tierIgnored interface{}, timeout time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.timeouts[tierIgnored] = timeout
}

// StartTimer starts a timeout timer for the given agent and tier.
// Returns a channel that receives TimeoutEvent when the timer fires.
// If a timer already exists for this agent, it is stopped and replaced.
func (h *TimeoutHandler) StartTimer(agentID string, tierIgnored interface{}) <-chan TimeoutEvent {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Stop existing timer if present
	if entry, exists := h.timers[agentID]; exists {
		entry.timer.Stop()
		close(entry.eventChan)
	}

	timeout := h.timeouts[nil]

	eventChan := make(chan TimeoutEvent, 1)
	startTime := time.Now()

	timer := time.AfterFunc(timeout, func() {
		h.mu.RLock()
		entry, exists := h.timers[agentID]
		h.mu.RUnlock()

		if exists {
			select {
			case entry.eventChan <- TimeoutEvent{
				AgentID: agentID,
				Elapsed: time.Since(startTime),
				Timeout: timeout,
			}:
			default:
				// Channel full or closed, skip sending
			}
		}
	})

	h.timers[agentID] = &timerEntry{
		timer:       timer,
		tierIgnored: tierIgnored,
		startTime:   startTime,
		eventChan:   eventChan,
	}

	return eventChan
}

// StopTimer stops the timeout timer for the given agent.
func (h *TimeoutHandler) StopTimer(agentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if entry, exists := h.timers[agentID]; exists {
		entry.timer.Stop()
		close(entry.eventChan)
		delete(h.timers, agentID)
	}
}

// ExtendTimer extends the timeout for the given agent by the specified duration.
// The timer continues from its current position with additional time added.
func (h *TimeoutHandler) ExtendTimer(agentID string, extension time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	entry, exists := h.timers[agentID]
	if !exists {
		return
	}

	// Stop the current timer
	entry.timer.Stop()

	// Calculate remaining time by resetting with extended duration
	timeout := h.timeouts[nil] + extension

	entry.timer = time.AfterFunc(extension, func() {
		h.mu.RLock()
		e, exists := h.timers[agentID]
		h.mu.RUnlock()

		if exists {
			select {
			case e.eventChan <- TimeoutEvent{
				AgentID: agentID,
				Elapsed: time.Since(e.startTime),
				Timeout: timeout,
			}:
			default:
				// Channel full or closed, skip sending
			}
		}
	})
}

// HandleTimeout processes a timeout event based on the user's chosen action.
// - Kill: invokes the onKill callback to cancel the agent
// - Extend: adds 50% more time to the current timeout
// - Continue: restarts the timer with the original timeout duration
func (h *TimeoutHandler) HandleTimeout(agentID string, action TimeoutAction) {
	h.mu.Lock()

	entry, exists := h.timers[agentID]
	if !exists {
		h.mu.Unlock()
		return
	}

	switch action {
	case TimeoutActionKill:
		// Stop the timer and clean up
		entry.timer.Stop()
		close(entry.eventChan)
		delete(h.timers, agentID)

		// Get the callback while holding the lock
		onKill := h.onKill
		h.mu.Unlock()

		// Invoke kill callback outside of lock
		if onKill != nil {
			onKill(agentID)
		}
		return

	case TimeoutActionExtend:
		// Add 50% more time
		timeout := h.timeouts[nil]
		extension := timeout / 2
		h.mu.Unlock()

		h.ExtendTimer(agentID, extension)
		return

	case TimeoutActionContinue:
		// Restart with original timeout
		tierIgnored := entry.tierIgnored
		h.mu.Unlock()

		// Stop and restart the timer
		h.StopTimer(agentID)
		h.StartTimer(agentID, tierIgnored)
		return
	}

	h.mu.Unlock()
}

// IsTimerActive returns true if a timer is currently running for the agent.
func (h *TimeoutHandler) IsTimerActive(agentID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	_, exists := h.timers[agentID]
	return exists
}

// GetElapsed returns how long the agent has been running since the timer started.
// Returns 0 if no timer exists for the agent.
func (h *TimeoutHandler) GetElapsed(agentID string) time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if entry, exists := h.timers[agentID]; exists {
		return time.Since(entry.startTime)
	}
	return 0
}

// ActiveTimers returns the number of currently active timers.
func (h *TimeoutHandler) ActiveTimers() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.timers)
}

// StopAll stops all active timers.
func (h *TimeoutHandler) StopAll() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for agentID, entry := range h.timers {
		entry.timer.Stop()
		close(entry.eventChan)
		delete(h.timers, agentID)
	}
}
