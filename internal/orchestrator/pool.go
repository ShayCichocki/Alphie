package orchestrator

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/google/uuid"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/config"
	"github.com/ShayCichocki/alphie/internal/learning"
	"github.com/ShayCichocki/alphie/internal/prog"
	"github.com/ShayCichocki/alphie/internal/state"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// PoolConfig contains configuration options for the OrchestratorPool.
type PoolConfig struct {
	RepoPath       string
	TierConfigs    *config.TierConfigs
	Greenfield     bool
	Executor       *agent.Executor
	StateDB        state.StateStore
	LearningSystem learning.LearningProvider
	ProgClient     prog.ProgTracker
	// RunnerFactory creates ClaudeRunner instances via the Anthropic API.
	// Required - must be set before calling Submit.
	RunnerFactory agent.ClaudeRunnerFactory
}

// OrchestratorPool manages multiple concurrent orchestrators.
type OrchestratorPool struct {
	cfg PoolConfig

	// orchestrators tracks running orchestrators by ID
	orchestrators map[string]*Orchestrator
	mu            sync.RWMutex

	// events aggregates events from all orchestrators
	events chan OrchestratorEvent

	// ctx and cancel for pool lifecycle
	ctx    context.Context
	cancel context.CancelFunc

	// wg tracks running orchestrators
	wg sync.WaitGroup
}

// NewOrchestratorPool creates a new OrchestratorPool.
func NewOrchestratorPool(cfg PoolConfig) *OrchestratorPool {
	ctx, cancel := context.WithCancel(context.Background())

	return &OrchestratorPool{
		cfg:           cfg,
		orchestrators: make(map[string]*Orchestrator),
		events:        make(chan OrchestratorEvent, 100),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Submit creates and starts a new orchestrator for the given task.
// Returns the orchestrator ID.
func (p *OrchestratorPool) Submit(task string, tier models.Tier) (string, error) {
	return p.SubmitWithID(task, tier, "")
}

// SubmitWithID creates and starts a new orchestrator for the given task.
// The originalTaskID is used to link TUI task_entered events with epic_created events.
// Returns the orchestrator ID.
func (p *OrchestratorPool) SubmitWithID(task string, tier models.Tier, originalTaskID string) (string, error) {
	orchID := uuid.New().String()[:8]

	// Require runner factory
	if p.cfg.RunnerFactory == nil {
		return "", fmt.Errorf("RunnerFactory is required")
	}

	// Create Claude runners for this orchestrator
	decomposerClaude := p.cfg.RunnerFactory.NewRunner()
	mergerClaude := p.cfg.RunnerFactory.NewRunner()

	// Create orchestrator using functional options
	orch := New(
		RequiredConfig{
			RepoPath: p.cfg.RepoPath,
			Tier:     tier,
			Executor: p.cfg.Executor,
		},
		WithTierConfigs(p.cfg.TierConfigs),
		WithGreenfield(p.cfg.Greenfield),
		WithDecomposerClaude(decomposerClaude),
		WithMergerClaude(mergerClaude),
		WithRunnerFactory(p.cfg.RunnerFactory),
		WithStateDB(p.cfg.StateDB),
		WithLearningSystem(p.cfg.LearningSystem),
		WithProgClient(p.cfg.ProgClient),
		WithOriginalTaskID(originalTaskID),
	)

	p.mu.Lock()
	p.orchestrators[orchID] = orch
	p.mu.Unlock()

	// Start goroutine to forward events from this orchestrator
	go p.forwardEvents(orchID, orch)

	// Start goroutine to run the orchestrator
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		if err := orch.Run(p.ctx, task); err != nil {
			log.Printf("[pool] orchestrator %s failed: %v", orchID, err)
		}

		p.mu.Lock()
		delete(p.orchestrators, orchID)
		p.mu.Unlock()
	}()

	return orchID, nil
}

// forwardEvents forwards events from an orchestrator to the pool's event channel.
func (p *OrchestratorPool) forwardEvents(orchID string, orch *Orchestrator) {
	for event := range orch.Events() {
		select {
		case p.events <- event:
		case <-p.ctx.Done():
			return
		}
	}
}

// Events returns the channel for receiving aggregated events from all orchestrators.
func (p *OrchestratorPool) Events() <-chan OrchestratorEvent {
	return p.events
}

// Stop stops all orchestrators and waits for them to complete.
func (p *OrchestratorPool) Stop() error {
	p.cancel()

	p.mu.RLock()
	for _, orch := range p.orchestrators {
		_ = orch.Stop()
	}
	p.mu.RUnlock()

	p.wg.Wait()
	close(p.events)

	return nil
}

// Count returns the number of running orchestrators.
func (p *OrchestratorPool) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.orchestrators)
}

// DroppedEventCount returns the total dropped events across all orchestrators.
func (p *OrchestratorPool) DroppedEventCount() uint64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var total uint64
	for _, orch := range p.orchestrators {
		total += orch.DroppedEventCount()
	}
	return total
}
