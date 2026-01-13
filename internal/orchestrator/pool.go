package orchestrator

import (
	"context"
	"log"
	"sync"

	"github.com/google/uuid"

	"github.com/shayc/alphie/internal/agent"
	"github.com/shayc/alphie/internal/config"
	"github.com/shayc/alphie/internal/learning"
	"github.com/shayc/alphie/internal/prog"
	"github.com/shayc/alphie/internal/state"
	"github.com/shayc/alphie/pkg/models"
)

// PoolConfig contains configuration options for the OrchestratorPool.
type PoolConfig struct {
	RepoPath       string
	TierConfigs    *config.TierConfigs
	Greenfield     bool
	Executor       *agent.Executor
	StateDB        *state.DB
	LearningSystem *learning.LearningSystem
	ProgClient     *prog.Client
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
	orchID := uuid.New().String()[:8]

	// Create Claude processes for this orchestrator
	decomposerClaude := agent.NewClaudeProcess(p.ctx)
	mergerClaude := agent.NewClaudeProcess(p.ctx)

	// Create orchestrator config
	orchCfg := OrchestratorConfig{
		RepoPath:         p.cfg.RepoPath,
		Tier:             tier,
		TierConfigs:      p.cfg.TierConfigs,
		Greenfield:       p.cfg.Greenfield,
		DecomposerClaude: decomposerClaude,
		MergerClaude:     mergerClaude,
		Executor:         p.cfg.Executor,
		StateDB:          p.cfg.StateDB,
		LearningSystem:   p.cfg.LearningSystem,
		ProgClient:       p.cfg.ProgClient,
	}

	orch := NewOrchestrator(orchCfg)

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
