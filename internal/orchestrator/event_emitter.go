// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

// EventEmitter handles event emission for the orchestrator.
// It provides a simple, thread-safe way to emit events to subscribers.
type EventEmitter struct {
	events chan OrchestratorEvent
}

// NewEventEmitter creates a new EventEmitter with the given buffer size.
func NewEventEmitter(bufferSize int) *EventEmitter {
	return &EventEmitter{
		events: make(chan OrchestratorEvent, bufferSize),
	}
}

// Emit sends an event to the events channel.
// If the channel is full, the event is dropped to avoid blocking.
func (e *EventEmitter) Emit(event OrchestratorEvent) {
	select {
	case e.events <- event:
	default:
		// Channel full, drop event to avoid blocking
	}
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
