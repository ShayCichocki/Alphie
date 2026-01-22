// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/learning"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// runLoop is the main execution loop that schedules, spawns, and merges work.
func (o *Orchestrator) runLoop(ctx context.Context) error {
	inflightTasks := make(map[string]*inflight)
	var inflightMu sync.Mutex

	// Aggregate channel for completion notifications
	completionCh := make(chan string, o.config.MaxAgents)

	for {
		select {
		case <-ctx.Done():
			// Cancel all in-flight tasks
			inflightMu.Lock()
			for _, inf := range inflightTasks {
				inf.cancelFn()
			}
			inflightMu.Unlock()
			return ctx.Err()

		case agentID := <-completionCh:
			// Handle task completion
			inflightMu.Lock()
			var completedTask *inflight
			for _, inf := range inflightTasks {
				if inf.agentID == agentID {
					completedTask = inf
					break
				}
			}
			if completedTask != nil {
				delete(inflightTasks, completedTask.taskID)
			}
			inflightMu.Unlock()

			if completedTask != nil {
				// Get the result from registry
				result := o.registry.GetResult(agentID)

				if result != nil {
					outcome := o.handleTaskCompletion(ctx, completedTask.taskID, result, completedTask.startTime)
					// Log outcome for debugging
					o.logger.Log("[runLoop] task %s completed with outcome: %s", completedTask.taskID, outcome.Status.String())
					// Note: Merge failures are logged and tracked but don't stop the session.
					// The task is marked as failed and other agents continue working.
					// Only fatal orchestrator errors should stop the runLoop.
				}
			}

		default:
			// Check if we're done
			o.logger.Log("[runLoop] checking for ready tasks...")
			ready := o.scheduler.Schedule()
			inflightMu.Lock()
			inflightCount := len(inflightTasks)
			inflightMu.Unlock()

			o.logger.Log("[runLoop] Schedule() returned %d ready tasks, %d inflight", len(ready), inflightCount)

			if len(ready) == 0 && inflightCount == 0 {
				// No more tasks to schedule and none in flight - we're done
				o.logger.Log("[runLoop] EXITING: no ready tasks and no inflight tasks")
				return nil
			}

			if len(ready) == 0 {
				// Nothing to schedule, wait for completions
				select {
				case <-ctx.Done():
					return ctx.Err()
				case agentID := <-completionCh:
					// Re-process this completion in the next iteration
					go func() { completionCh <- agentID }()
				case <-time.After(o.config.Policy.Loop.PollInterval):
					// Small delay to avoid busy-waiting
				}
				continue
			}

			// Check if paused - wait until resumed before spawning new agents
			if err := o.pauseCtrl.WaitIfPaused(ctx); err != nil {
				return err
			}

			// Spawn agents for ready tasks
			if err := o.spawnAgents(ctx, ready, inflightTasks, &inflightMu, completionCh); err != nil {
				return err
			}
		}
	}
}

// inflight represents an in-flight task being executed by an agent.
type inflight struct {
	taskID    string
	agentID   string
	startTime time.Time
	doneCh    chan *agent.ExecutionResult
	cancelFn  context.CancelFunc
}

// spawnAgents spawns agents for the given ready tasks using the AgentSpawner.
func (o *Orchestrator) spawnAgents(ctx context.Context, ready []*models.Task, inflightTasks map[string]*inflight, inflightMu *sync.Mutex, completionCh chan string) error {
	inflightMu.Lock()
	workersRunning := len(inflightTasks)
	inflightMu.Unlock()

	for i, task := range ready {
		// Add delay between parallel agent spawns (skip first)
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(o.config.Policy.Loop.SpawnStagger):
				// Stagger delay to prevent Claude CLI race conditions
				o.logger.Log("[runLoop] stagger delay before spawning agent %d", i+1)
			}
		}

		// Emit task queued event
		o.emitEvent(OrchestratorEvent{
			Type:      EventTaskQueued,
			TaskID:    task.ID,
			TaskTitle: task.Title,
			ParentID:  task.ParentID,
			Message:   fmt.Sprintf("Task queued: %s", task.Title),
			Timestamp: time.Now(),
		})

		// Check for protected areas
		isProtected := o.protected.IsProtected(task.Description) || o.protected.IsProtected(task.Title)
		if isProtected {
			// For Scout tier, mark in override gate to allow questions
			// For other tiers, they can already ask questions, so proceed
			if o.config.Tier == models.TierScout {
				o.overrideGate.SetProtectedArea(task.ID, true)
				log.Printf("[orchestrator] task %s touches protected area, Scout can now ask questions", task.ID)
			}
		}

		// Retrieve relevant learnings for this task
		var taskLearnings []*learning.Learning
		if o.learnings != nil {
			learnings, err := o.learnings.OnTaskStart(task.Description, nil)
			if err != nil {
				log.Printf("[orchestrator] warning: failed to retrieve learnings for task %s: %v", task.ID, err)
			} else if len(learnings) > 0 {
				taskLearnings = learnings
				log.Printf("[orchestrator] retrieved %d learnings for task %s", len(learnings), task.ID)
			}
		}

		// Create agent context
		taskCtx, taskCancel := context.WithCancel(ctx)

		agentID, resultCh := o.spawner.Spawn(taskCtx, task, SpawnOptions{
			Tier:           o.config.Tier,
			Learnings:      taskLearnings,
			Baseline:       o.config.Baseline,
			WorkersRunning: workersRunning + i + 1,
			WorkersBlocked: 0,
		})

		// Create agent model for state persistence
		agentModel := &models.Agent{
			ID:        agentID,
			TaskID:    task.ID,
			Status:    models.AgentStatusRunning,
			StartedAt: time.Now(),
		}

		// Persist agent state
		o.createAgentState(agentModel)

		// Track in-flight task
		inf := &inflight{
			taskID:    task.ID,
			agentID:   agentID,
			startTime: time.Now(),
			doneCh:    make(chan *agent.ExecutionResult, 1),
			cancelFn:  taskCancel,
		}

		inflightMu.Lock()
		inflightTasks[task.ID] = inf
		inflightMu.Unlock()

		// Update task status and assign to agent
		task.Status = models.TaskStatusInProgress
		task.AssignedTo = agentID
		o.updateTaskState(task)

		// Update prog task status to in_progress
		o.progCoord.StartTask(task.ID)

		// Wait for result in background and signal completion
		o.wg.Add(1)
		go func(t *models.Task, aID string, resultCh <-chan SpawnResult) {
			defer o.wg.Done()

			// Wait for spawner result
			result := <-resultCh

			// Store result in registry
			o.registry.StoreResult(aID, result.Result)

			// Signal completion
			select {
			case completionCh <- aID:
			case <-taskCtx.Done():
			}
		}(task, agentID, resultCh)
	}

	return nil
}
