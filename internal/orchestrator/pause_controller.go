// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// PauseController manages pause/resume/stop state for the orchestrator.
// It provides a thread-safe way to control execution flow.
type PauseController struct {
	// paused indicates whether the orchestrator is paused.
	paused bool
	// stopped indicates whether the orchestrator has been stopped.
	stopped bool
	// mu protects all fields.
	mu sync.RWMutex
	// cond is used to signal when the orchestrator is unpaused or stopped.
	cond *sync.Cond
}

// NewPauseController creates a new PauseController.
func NewPauseController() *PauseController {
	p := &PauseController{}
	p.cond = sync.NewCond(&p.mu)
	return p
}

// Pause pauses execution. New agents will not be spawned.
func (p *PauseController) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.paused {
		p.paused = true
		log.Printf("[orchestrator] paused - no new agents will be spawned")
	}
}

// Resume resumes execution after a pause.
func (p *PauseController) Resume() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.paused {
		p.paused = false
		log.Printf("[orchestrator] resumed - agent spawning enabled")
		p.cond.Broadcast()
	}
}

// Stop signals a stop. This unblocks any WaitIfPaused calls.
func (p *PauseController) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.stopped {
		p.stopped = true
		p.cond.Broadcast()
	}
}

// IsPaused returns whether execution is currently paused.
func (p *PauseController) IsPaused() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.paused
}

// IsStopped returns whether the controller has been stopped.
func (p *PauseController) IsStopped() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stopped
}

// WaitIfPaused blocks until the orchestrator is unpaused or stopped.
// Returns an error if the context is cancelled or the controller is stopped.
func (p *PauseController) WaitIfPaused(ctx context.Context) error {
	p.mu.Lock()
	if p.paused && !p.stopped {
		// Spawn ONE goroutine to signal condition if context is cancelled
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				p.mu.Lock()
				p.cond.Broadcast()
				p.mu.Unlock()
			case <-done:
			}
		}()

		// Wait loop - no new goroutines spawned on spurious wakeups
		for p.paused && !p.stopped {
			p.cond.Wait()
			if ctx.Err() != nil {
				close(done)
				p.mu.Unlock()
				return ctx.Err()
			}
		}
		close(done)
	}
	if p.stopped {
		p.mu.Unlock()
		return fmt.Errorf("orchestrator stopped")
	}
	p.mu.Unlock()
	return nil
}
