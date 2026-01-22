// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/learning"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// SpawnOptions contains configuration for spawning an agent.
type SpawnOptions struct {
	// Tier is the agent tier for execution.
	Tier models.Tier
	// Learnings are relevant learnings for this task.
	Learnings []*learning.Learning
	// Baseline is the session baseline for regression detection.
	Baseline *agent.Baseline
	// OnProgress is called with progress updates.
	OnProgress func(ProgressReport)
}

// SpawnResult contains the outcome of a spawned agent.
type SpawnResult struct {
	// AgentID is the ID of the agent.
	AgentID string
	// TaskID is the ID of the task.
	TaskID string
	// Result is the execution result.
	Result *agent.ExecutionResult
	// Error is any error that occurred during spawning.
	Error error
}

// DefaultAgentSpawner spawns task agents using the task executor.
type DefaultAgentSpawner struct {
	executor    agent.TaskExecutor
	collision   *CollisionChecker
	scheduler   *Scheduler
	events      chan<- OrchestratorEvent
	repoPath    string
}

// NewAgentSpawner creates a new DefaultAgentSpawner.
func NewAgentSpawner(
	executor agent.TaskExecutor,
	collision *CollisionChecker,
	scheduler *Scheduler,
	events chan<- OrchestratorEvent,
	repoPath string,
) *DefaultAgentSpawner {
	return &DefaultAgentSpawner{
		executor:  executor,
		collision: collision,
		scheduler: scheduler,
		events:    events,
		repoPath:  repoPath,
	}
}

// SetScheduler sets the task scheduler after construction.
func (s *DefaultAgentSpawner) SetScheduler(scheduler *Scheduler) {
	s.scheduler = scheduler
}

// Spawn starts an agent for the given task.
// Returns the agent ID immediately and a channel for the execution result.
func (s *DefaultAgentSpawner) Spawn(ctx context.Context, task *models.Task, opts SpawnOptions) (string, <-chan SpawnResult) {
	resultCh := make(chan SpawnResult, 1)

	// Create agent model
	agentModel := &models.Agent{
		ID:        uuid.New().String(),
		TaskID:    task.ID,
		Status:    models.AgentStatusRunning,
		StartedAt: time.Now(),
	}

	// Register with scheduler (if set)
	if s.scheduler != nil {
		s.scheduler.OnAgentStart(agentModel)
	}

	// Register with collision checker
	pathPrefixes := s.collision.ExtractPathPrefixes(task)
	s.collision.RegisterAgent(agentModel.ID, pathPrefixes, nil)

	// Emit task started event
	s.emitEvent(OrchestratorEvent{
		Type:      EventTaskStarted,
		TaskID:    task.ID,
		TaskTitle: task.Title,
		ParentID:  task.ParentID,
		AgentID:   agentModel.ID,
		Message:   fmt.Sprintf("Started task: %s", task.Title),
		Timestamp: time.Now(),
	})

	// Spawn agent goroutine
	go func() {
		defer close(resultCh)

		execOpts := &agent.ExecuteOptions{
			AgentID:            agentModel.ID,
			Learnings:          opts.Learnings,
			EnableRalphLoop:    true,
			EnableQualityGates: true,
			Baseline:           opts.Baseline,
			OnProgress: func(update agent.ProgressUpdate) {
				if opts.OnProgress != nil {
					opts.OnProgress(ProgressReport{
						AgentID:    update.AgentID,
						TaskID:     task.ID,
						Phase:      PhaseImplementing,
						Message:    fmt.Sprintf("Agent progress: %d tokens, $%.4f", update.TokensUsed, update.Cost),
						TokensUsed: int(update.TokensUsed),
						Cost:       update.Cost,
						Duration:   update.Duration,
						Timestamp:  time.Now(),
					})
				}
				s.emitEvent(OrchestratorEvent{
					Type:          EventAgentProgress,
					TaskID:        task.ID,
					AgentID:       update.AgentID,
					Message:       fmt.Sprintf("Agent progress: %d tokens, $%.4f", update.TokensUsed, update.Cost),
					Timestamp:     time.Now(),
					TokensUsed:    update.TokensUsed,
					Cost:          update.Cost,
					Duration:      update.Duration,
					CurrentAction: update.CurrentAction,
				})
			},
		}

		result, err := s.executor.ExecuteWithOptions(ctx, task, opts.Tier, execOpts)
		if err != nil {
			log.Printf("[spawner] task %s execution error: %v", task.ID, err)
			result = &agent.ExecutionResult{
				Success: false,
				Error:   err.Error(),
				AgentID: agentModel.ID,
			}
		}

		resultCh <- SpawnResult{
			AgentID: agentModel.ID,
			TaskID:  task.ID,
			Result:  result,
			Error:   err,
		}
	}()

	return agentModel.ID, resultCh
}

// emitEvent sends an event to the events channel.
func (s *DefaultAgentSpawner) emitEvent(event OrchestratorEvent) {
	select {
	case s.events <- event:
	default:
		// Channel full, drop event to avoid blocking
	}
}

