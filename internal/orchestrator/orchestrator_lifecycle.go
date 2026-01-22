// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/state"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// Run executes the full orchestration workflow:
//  1. Decompose request into tasks (or resume from existing epic)
//  2. Build dependency graph
//  3. Create session branch
//  4. Loop: schedule -> spawn agents -> wait -> merge
//  5. Cleanup session or create PR
func (o *Orchestrator) Run(ctx context.Context, request string) error {
	o.logger.Log("Run() started for request: %s", request)

	if o.pauseCtrl.IsStopped() {
		return fmt.Errorf("orchestrator has been stopped")
	}

	// Create a derived context that we can cancel
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Monitor stop channel
	go func() {
		select {
		case <-o.stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	// Create session in state DB
	if err := o.createSessionState(request); err != nil {
		return fmt.Errorf("create session state: %w", err)
	}

	// Capture baseline at session start for regression detection
	if err := o.captureBaseline(); err != nil {
		log.Printf("[orchestrator] warning: failed to capture baseline: %v", err)
	}

	// Get or decompose tasks
	tasks, err := o.resolveTasks(ctx, request)
	if err != nil {
		o.updateSessionStatus(state.SessionFailed)
		return err
	}

	// Persist tasks to state DB
	if err := o.persistTasks(tasks); err != nil {
		o.updateSessionStatus(state.SessionFailed)
		return fmt.Errorf("persist tasks: %w", err)
	}

	// Build dependency graph
	if err := o.graph.Build(tasks); err != nil {
		o.updateSessionStatus(state.SessionFailed)
		return fmt.Errorf("build dependency graph: %w", err)
	}

	// Create scheduler now that graph is built
	o.scheduler = NewScheduler(o.graph, o.config.Tier, o.config.MaxAgents)
	o.scheduler.SetCollisionChecker(o.collision)
	o.scheduler.SetGreenfield(o.config.Greenfield)
	o.scheduler.SetOrchestrator(o) // For merge conflict checking

	// Wire scheduler into spawner (scheduler wasn't available at construction)
	o.spawner.SetScheduler(o.scheduler)

	// Create merge queue for serialized, reliable merging
	o.mergeQueue = o.createMergeQueue()
	defer o.mergeQueue.Stop()

	// Create session branch
	if err := o.sessionMgr.CreateBranch(); err != nil {
		o.updateSessionStatus(state.SessionFailed)
		return fmt.Errorf("create session branch: %w", err)
	}

	// Main execution loop
	if err := o.runLoop(ctx); err != nil {
		o.handleRunError()
		o.updateSessionStatus(state.SessionFailed)
		return fmt.Errorf("execution loop: %w", err)
	}

	// Merge session branch to main
	o.finalizeSession()

	// Mark session completed and emit done event
	o.updateSessionStatus(state.SessionCompleted)
	o.updateProgEpicStatus()
	o.emitEvent(OrchestratorEvent{
		Type:      EventSessionDone,
		Message:   "All tasks completed successfully",
		Timestamp: time.Now(),
	})

	return nil
}

// captureBaseline captures the baseline at session start for regression detection.
func (o *Orchestrator) captureBaseline() error {
	baseline, err := agent.CaptureBaseline(o.config.RepoPath)
	if err != nil {
		return err
	}
	o.config.Baseline = baseline
	baselinePath := filepath.Join(o.config.RepoPath, ".alphie", "baselines", fmt.Sprintf("%s.json", o.config.SessionID))
	if saveErr := baseline.Save(baselinePath); saveErr != nil {
		log.Printf("[orchestrator] warning: failed to save baseline: %v", saveErr)
	} else {
		o.logger.Log("Baseline captured: %d failing tests, %d lint errors, %d type errors",
			len(baseline.FailingTests), len(baseline.LintErrors), len(baseline.TypeErrors))
	}
	return nil
}

// resolveTasks either loads tasks from existing epic or decomposes the request.
func (o *Orchestrator) resolveTasks(ctx context.Context, request string) ([]*models.Task, error) {
	if o.progCoord.HasResumeEpic() {
		tasks, err := o.progCoord.LoadTasksFromEpic(ctx)
		if err != nil {
			return nil, fmt.Errorf("load tasks from prog epic %s: %w", o.progCoord.EpicID(), err)
		}
		log.Printf("[orchestrator] resuming epic %s with %d tasks", o.progCoord.EpicID(), len(tasks))
		return tasks, nil
	}

	// Decompose request into tasks
	tasks, err := o.decomposer.Decompose(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("decompose request: %w", err)
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("no tasks generated from request")
	}

	// Create prog epic and tasks for cross-session tracking
	if err := o.progCoord.CreateEpicAndTasks(request, tasks); err != nil {
		log.Printf("[orchestrator] warning: failed to create prog epic/tasks: %v", err)
	}
	return tasks, nil
}

// createMergeQueue creates the merge queue for serialized merging.
func (o *Orchestrator) createMergeQueue() *MergeQueue {
	semanticMergerFactory := func() *SemanticMerger {
		if o.runnerFactory == nil {
			return o.semanticMerger
		}
		freshClaude := o.runnerFactory.NewRunner()
		return NewSemanticMerger(freshClaude, o.config.RepoPath)
	}

	mq := NewMergeQueueWithPolicy(
		o.merger,
		o.semanticMerger,
		semanticMergerFactory,
		o.config.SessionID,
		o.sessionMgr.GetBranchName(),
		o.config.Greenfield,
		DefaultMergeQueueConfig(),
		o.emitter.Channel(),
		o.config.Policy,
		o.mergeVerifier,
	)

	// Set orchestrator and git runner on the processor for merge conflict resolution
	processor := mq.GetProcessor()
	if processor != nil {
		processor.SetOrchestrator(o)
		if o.merger != nil {
			processor.SetGitRunner(o.merger.GitRunner())
		}
	}

	return mq
}

// handleRunError cleans up after a run error.
func (o *Orchestrator) handleRunError() {
	if !o.config.Greenfield {
		_ = o.sessionMgr.Cleanup()
	} else {
		_ = o.checkoutMain()
	}
}

// finalizeSession merges session branch to main and cleans up.
func (o *Orchestrator) finalizeSession() {
	if o.config.Greenfield || o.sessionMgr == nil {
		return
	}
	if err := o.sessionMgr.MergeToMain(); err != nil {
		log.Printf("[orchestrator] warning: failed to merge session to main: %v", err)
		return
	}
	log.Printf("[orchestrator] merged session branch to main")
	if err := o.sessionMgr.Cleanup(); err != nil {
		log.Printf("[orchestrator] warning: failed to cleanup session branch: %v", err)
	}
}

// updateProgEpicStatus updates the prog epic status if all tasks are complete.
func (o *Orchestrator) updateProgEpicStatus() {
	if o.progCoord.EpicID() == "" || !o.progCoord.IsConfigured() {
		return
	}
	epicID := o.progCoord.EpicID()
	if done, err := o.progCoord.Client().UpdateEpicStatusIfComplete(epicID); err != nil {
		log.Printf("[orchestrator] warning: failed to update epic status: %v", err)
	} else if done {
		log.Printf("[orchestrator] epic %s marked as done", epicID)
	}
}

// Stop signals the orchestrator to stop all work and clean up.
func (o *Orchestrator) Stop() error {
	if o.pauseCtrl.IsStopped() {
		return nil
	}
	o.pauseCtrl.Stop()

	// Signal stop
	close(o.stopCh)

	// Wait for all goroutines to finish
	o.wg.Wait()

	// Close events channel
	o.emitter.Close()

	// Cleanup session branch if not greenfield
	if !o.config.Greenfield && o.sessionMgr != nil {
		if err := o.sessionMgr.Cleanup(); err != nil {
			return fmt.Errorf("cleanup session: %w", err)
		}
	} else if o.config.Greenfield {
		_ = o.checkoutMain()
	}

	return nil
}

// checkoutMain ensures the repository is on the main branch.
func (o *Orchestrator) checkoutMain() error {
	cmd := exec.Command("git", "checkout", "main")
	cmd.Dir = o.config.RepoPath
	if err := cmd.Run(); err != nil {
		cmd = exec.Command("git", "checkout", "master")
		cmd.Dir = o.config.RepoPath
		return cmd.Run()
	}
	return nil
}

// Pause pauses the orchestrator, preventing new agents from being spawned.
func (o *Orchestrator) Pause() {
	o.pauseCtrl.Pause()
}

// Resume unpauses the orchestrator, allowing new agents to be spawned.
func (o *Orchestrator) Resume() {
	o.pauseCtrl.Resume()
}

// IsPaused returns whether the orchestrator is currently paused.
func (o *Orchestrator) IsPaused() bool {
	return o.pauseCtrl.IsPaused()
}
