// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"log"
	"sync/atomic"
	"time"
)

// EventEmitter handles event emission for the orchestrator.
// It provides a simple, thread-safe way to emit events to subscribers.
type EventEmitter struct {
	events       chan OrchestratorEvent
	droppedCount atomic.Uint64
}

// NewEventEmitter creates a new EventEmitter with the given buffer size.
func NewEventEmitter(bufferSize int) *EventEmitter {
	return &EventEmitter{
		events: make(chan OrchestratorEvent, bufferSize),
	}
}

// Emit sends an event to the events channel.
// If the channel is full, it tries with a timeout before dropping the event.
func (e *EventEmitter) Emit(event OrchestratorEvent) {
	// Try immediate send first
	select {
	case e.events <- event:
		return
	default:
		// Channel full, try with timeout
	}

	// Try with 100ms timeout to give the receiver a chance to drain
	select {
	case e.events <- event:
		return
	case <-time.After(100 * time.Millisecond):
		// Timeout expired, drop the event
		count := e.droppedCount.Add(1)
		if count%10 == 1 { // Log every 10th drop to avoid spam
			log.Printf("[orchestrator] WARNING: Event channel full, dropped event (total dropped: %d): type=%s", count, event.Type)
		}
	}
}

// DroppedCount returns the total number of events that have been dropped.
func (e *EventEmitter) DroppedCount() uint64 {
	return e.droppedCount.Load()
}

// Events returns a read-only channel of events.
// This is used by subscribers (e.g., TUI) to receive updates.
func (e *EventEmitter) Events() <-chan OrchestratorEvent {
	return e.events
}

// Close closes the events channel.
// This should be called when the orchestrator is stopped.
func (e *EventEmitter) Close() {
	close(e.events)
}

// Channel returns the underlying channel for direct access.
// This is useful for components that need to send events directly.
func (e *EventEmitter) Channel() chan<- OrchestratorEvent {
	return e.events
}
