// Package architect provides tools for analyzing and auditing codebases against specifications.
package architect

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/orchestrator"
	"github.com/ShayCichocki/alphie/internal/validation"
	"github.com/ShayCichocki/alphie/internal/verification"
)

// ProgressPhase represents the current phase of the implementation loop.
type ProgressPhase string

const (
	// PhaseParsing indicates the architecture document is being parsed.
	PhaseParsing ProgressPhase = "parsing"
	// PhaseAuditing indicates the codebase is being audited.
	PhaseAuditing ProgressPhase = "auditing"
	// PhasePlanning indicates epics and tasks are being planned.
	PhasePlanning ProgressPhase = "planning"
	// PhaseExecuting indicates tasks are being executed.
	PhaseExecuting ProgressPhase = "executing"
	// PhaseComplete indicates the iteration completed.
	PhaseComplete ProgressPhase = "complete"
)

// ProgressEvent represents a progress update from the controller.
type ProgressEvent struct {
	// Phase is the current phase of the implementation loop.
	Phase ProgressPhase
	// Iteration is the current iteration number (1-based).
	Iteration int
	// MaxIterations is the maximum number of iterations.
	MaxIterations int
	// FeaturesComplete is the number of features fully implemented.
	FeaturesComplete int
	// FeaturesTotal is the total number of features.
	FeaturesTotal int
	// GapsFound is the number of gaps found in the current audit.
	GapsFound int
	// TasksCreated is the number of tasks created.
	TasksCreated int
	// TasksCompleted is the number of tasks completed.
	TasksCompleted int
	// EpicID is the current epic being processed.
	EpicID string
	// Cost is the cumulative cost so far.
	Cost float64
	// CostBudget is the cost budget limit.
	CostBudget float64
	// WorkersRunning is the number of agents currently executing tasks.
	WorkersRunning int
	// WorkersBlocked is the number of tasks blocked by dependencies or collisions.
	WorkersBlocked int
	// ActiveWorkers contains detailed info about each active worker for debugging.
	ActiveWorkers map[string]WorkerInfo
	// Message is an optional status message.
	Message string
	// Timestamp is when the event occurred.
	Timestamp time.Time
}

// WorkerInfo contains information about an active worker.
type WorkerInfo struct {
	AgentID   string
	TaskID    string
	TaskTitle string
	Status    string
}

// ProgressCallback is called when progress events occur.
type ProgressCallback func(event ProgressEvent)

// Controller orchestrates the architecture iteration loop.
// It parses the architecture document, audits the codebase for gaps,
// plans epics from gaps, executes them, and repeats until done or stopped.
type Controller struct {
	// MaxIterations is the maximum number of iterations before stopping.
	// A value of 0 means no limit.
	MaxIterations int
	// Budget is the maximum cost allowed (in dollars).
	// A value of 0 means no limit.
	Budget float64
	// NoConvergeAfter is the number of consecutive iterations without progress
	// before considering the loop converged. A value of 0 means no convergence check.
	NoConvergeAfter int

	// RepoPath is the path to the repository being audited.
	RepoPath string

	// parser parses architecture documents into feature specs.
	parser *Parser
	// auditor checks the codebase against the spec.
	auditor *Auditor
	// planner creates prog epics and tasks from gaps.
	planner *Planner
	// stopper evaluates stop conditions.
	stopper *StopChecker
	// onProgress is called when progress events occur.
	onProgress ProgressCallback
	// runnerFactory creates ClaudeRunner instances.
	// If nil, falls back to creating ClaudeProcess (legacy).
	runnerFactory agent.ClaudeRunnerFactory
	// tokenTracker tracks cumulative token usage and cost.
	tokenTracker *agent.TokenTracker

	// Current state tracking (for progress events during execution)
	currentIteration        int
	currentFeaturesTotal    int
	currentFeaturesComplete int

	// Feature completion tracking (for real-time updates during execution)
	featureToTasks map[string][]string // maps feature ID to task IDs
	completedTasks map[string]bool     // tracks which tasks are done
	featureGaps    map[string]Gap      // maps feature ID to gap details

	// Active worker tracking (for UI display)
	activeWorkers map[string]WorkerInfo // maps agent ID to worker info
}

// ControllerOption is a functional option for configuring a Controller.
type ControllerOption func(*Controller)

// WithRepoPath sets the repository path for the controller.
func WithRepoPath(path string) ControllerOption {
	return func(c *Controller) {
		c.RepoPath = path
	}
}

// WithProgressCallback sets a callback for progress events.
func WithProgressCallback(cb ProgressCallback) ControllerOption {
	return func(c *Controller) {
		c.onProgress = cb
	}
}

// WithRunnerFactory sets the factory for creating ClaudeRunner instances.
func WithRunnerFactory(factory agent.ClaudeRunnerFactory) ControllerOption {
	return func(c *Controller) {
		c.runnerFactory = factory
	}
}

// createRunner creates a new ClaudeRunner using the factory.
// The factory must be set via WithRunnerFactory option.
func (c *Controller) createRunner(ctx context.Context) agent.ClaudeRunner {
	if c.runnerFactory == nil {
		panic("Controller: runnerFactory is required - use WithRunnerFactory option")
	}
	return c.runnerFactory.NewRunner()
}

// emitProgress sends a progress event if a callback is set.
func (c *Controller) emitProgress(event ProgressEvent) {
	if c.onProgress != nil {
		event.Timestamp = time.Now()
		event.MaxIterations = c.MaxIterations
		event.CostBudget = c.Budget
		c.onProgress(event)
	}
}

// NewController creates a new Controller with the given configuration.
func NewController(maxIterations int, budget float64, noConvergeAfter int, opts ...ControllerOption) *Controller {
	c := &Controller{
		MaxIterations:   maxIterations,
		Budget:          budget,
		NoConvergeAfter: noConvergeAfter,
		parser:          NewParser(),
		auditor:         NewAuditor(),
		stopper: NewStopChecker(StopConfig{
			MaxIterations:   maxIterations,
			BudgetLimit:     budget,
			NoProgressLimit: noConvergeAfter,
		}),
		tokenTracker:  agent.NewTokenTracker("sonnet"),
		activeWorkers: make(map[string]WorkerInfo),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// IterationResult captures the outcome of a single iteration.
type IterationResult struct {
	// Iteration is the iteration number (1-based).
	Iteration int
	// GapsFound is the number of gaps identified in this iteration.
	GapsFound int
	// GapsRemaining is the number of gaps still remaining after execution.
	GapsRemaining int
	// EpicID is the prog epic ID created for this iteration (if any).
	EpicID string
	// TasksCreated is the number of tasks created in this iteration.
	TasksCreated int
	// TasksCompleted is the number of tasks completed in this iteration.
	TasksCompleted int
	// ProgressMade indicates if progress was made compared to the previous iteration.
	ProgressMade bool
	// Cost is the estimated cost incurred in this iteration.
	Cost float64
}

// RunResult captures the final result of the controller run.
type RunResult struct {
	// Iterations contains results from each iteration.
	Iterations []IterationResult
	// StopReason is why the controller stopped.
	StopReason StopReason
	// TotalCost is the cumulative cost across all iterations.
	TotalCost float64
	// FinalCompletionPct is the final completion percentage.
	FinalCompletionPct float64
}

// Run executes the architecture iteration loop.
// It parses the architecture document, audits for gaps, plans epics,
// executes them via the /alphie skill pattern, and repeats until
// a stop condition is met.
func (c *Controller) Run(ctx context.Context, archDoc string, agents int) error {
	// Prog client removed - keeping stateless

	var result RunResult
	var totalCost float64
	var lastGapCount int = -1
	var lastIterationCost float64

	for iteration := 1; ; iteration++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Step 1: Parse architecture document
		c.emitProgress(ProgressEvent{
			Phase:     PhaseParsing,
			Iteration: iteration,
			Cost:      totalCost,
			Message:   fmt.Sprintf("Iteration %d/%d: Parsing architecture document...", iteration, c.MaxIterations),
		})

		claude := c.createRunner(ctx)
		spec, err := c.parser.Parse(ctx, archDoc, claude)
		if err != nil {
			return fmt.Errorf("parse architecture doc (iteration %d): %w", iteration, err)
		}

		// Track tokens from parsing
		if apiRunner, ok := claude.(*agent.ClaudeAPIAdapter); ok {
			apiClient := apiRunner.Client()
			if apiClient != nil {
				input, output := apiClient.Tracker().Total()
				c.tokenTracker.Update(agent.MessageDeltaUsage{
					InputTokens:  input,
					OutputTokens: output,
				})
			}
		}

		// Step 2: Audit codebase for gaps
		c.emitProgress(ProgressEvent{
			Phase:         PhaseAuditing,
			Iteration:     iteration,
			FeaturesTotal: len(spec.Features),
			Cost:          totalCost,
			Message:       fmt.Sprintf("Iteration %d/%d: Auditing codebase against %d features...", iteration, c.MaxIterations, len(spec.Features)),
		})

		auditClaude := c.createRunner(ctx)
		gapReport, err := c.auditor.Audit(ctx, spec, c.RepoPath, auditClaude)
		if err != nil {
			return fmt.Errorf("audit codebase (iteration %d): %w", iteration, err)
		}

		// Track tokens from auditing
		if apiRunner, ok := auditClaude.(*agent.ClaudeAPIAdapter); ok {
			apiClient := apiRunner.Client()
			if apiClient != nil {
				input, output := apiClient.Tracker().Total()
				c.tokenTracker.Update(agent.MessageDeltaUsage{
					InputTokens:  input,
					OutputTokens: output,
				})
			}
		}

		// Calculate metrics
		gapsFound := len(gapReport.Gaps)
		completedFeatures := 0
		for _, fs := range gapReport.Features {
			if fs.Status == AuditStatusComplete {
				completedFeatures++
			}
		}
		totalFeatures := len(spec.Features)
		completionPct := 0.0
		if totalFeatures > 0 {
			completionPct = float64(completedFeatures) / float64(totalFeatures) * 100.0
		}

		// Update controller state for progress events
		c.currentIteration = iteration
		c.currentFeaturesTotal = totalFeatures
		c.currentFeaturesComplete = completedFeatures

		// Determine if progress was made
		progressMade := lastGapCount < 0 || gapsFound < lastGapCount
		lastGapCount = gapsFound

		// Get real cost from token tracker
		totalCost = c.tokenTracker.GetCost()
		iterationCost := totalCost - lastIterationCost // Delta for this iteration
		lastIterationCost = totalCost

		iterResult := IterationResult{
			Iteration:     iteration,
			GapsFound:     gapsFound,
			GapsRemaining: gapsFound,
			ProgressMade:  progressMade,
			Cost:          iterationCost,
		}

		// Step 3: Check stop conditions
		stopReason, shouldStop := c.stopper.Check(iteration, totalCost, completionPct, progressMade)
		if shouldStop {
			result.Iterations = append(result.Iterations, iterResult)
			result.StopReason = stopReason
			result.TotalCost = totalCost
			result.FinalCompletionPct = completionPct
			return nil
		}

		// Planner removed - this needs to be replaced with direct orchestration
		// For now, skip planning phase if gaps are found
		if gapsFound > 0 {
			log.Printf("[architect] Found %d gaps but planning system is disabled", gapsFound)
		}

		result.Iterations = append(result.Iterations, iterResult)

		// Emit iteration complete event
		c.emitProgress(ProgressEvent{
			Phase:            PhaseComplete,
			Iteration:        iteration,
			FeaturesComplete: completedFeatures,
			FeaturesTotal:    totalFeatures,
			GapsFound:        gapsFound,
			TasksCreated:     iterResult.TasksCreated,
			TasksCompleted:   iterResult.TasksCompleted,
			EpicID:           iterResult.EpicID,
			Cost:             totalCost,
			Message:          fmt.Sprintf("Iteration %d/%d complete: %d/%d features, %d gaps remaining", iteration, c.MaxIterations, completedFeatures, totalFeatures, gapsFound),
		})

		// If no gaps, we're done
		if gapsFound == 0 {
			result.StopReason = StopReasonComplete
			result.TotalCost = totalCost
			result.FinalCompletionPct = 100.0
			c.emitProgress(ProgressEvent{
				Phase:            PhaseComplete,
				Iteration:        iteration,
				FeaturesComplete: totalFeatures,
				FeaturesTotal:    totalFeatures,
				Cost:             totalCost,
				Message:          "All features implemented!",
			})
			return nil
		}
	}
}

// executeEpic runs the orchestrator directly to execute an epic's tasks.
// It streams progress events to the TUI and tracks worker state in real-time.
// Returns the number of tasks completed and any error.
func (c *Controller) executeEpic(ctx context.Context, epicID string, agents int) (int, error) {
	// Create orchestrator for this epic
	orch, err := c.createOrchestrator(epicID, agents)
	if err != nil {
		return 0, fmt.Errorf("create orchestrator: %w", err)
	}
	defer orch.Stop()

	// Subscribe to events for progress updates
	eventsCh := orch.Events()

	// Start event processing goroutine
	eventsDone := make(chan struct{})
	go func() {
		defer close(eventsDone)
		for event := range eventsCh {
			c.handleOrchestratorEvent(event)
		}
	}()

	// Run orchestrator (empty request since we're resuming an epic)
	err = orch.Run(ctx, "")

	// Wait for event processing to complete
	<-eventsDone

	if err != nil {
		return 0, fmt.Errorf("orchestrator run: %w", err)
	}

	// Prog client removed - return 0 for now
	return 0, nil
}

// createOrchestrator creates a new orchestrator instance for epic execution.
func (c *Controller) createOrchestrator(epicID string, agents int) (*orchestrator.Orchestrator, error) {
	// Create 4-layer validator for task validation
	validator := c.createValidator()

	// Create executor
	executor, err := agent.NewExecutor(agent.ExecutorConfig{
		RepoPath:      c.RepoPath,
		Model:         "sonnet",
		RunnerFactory: c.runnerFactory,
		Validator:     validator,
	})
	if err != nil {
		return nil, fmt.Errorf("create executor: %w", err)
	}

	// Create Claude runners for decomposer and merger
	decomposerClaude := c.runnerFactory.NewRunner()
	mergerClaude := c.runnerFactory.NewRunner()
	secondReviewerClaude := c.runnerFactory.NewRunner()

	// Create orchestrator with all required dependencies
	orch := orchestrator.New(
		orchestrator.RequiredConfig{
			RepoPath: c.RepoPath,
			Executor: executor,
		},
		orchestrator.WithMaxAgents(agents),
		orchestrator.WithDecomposerClaude(decomposerClaude),
		orchestrator.WithMergerClaude(mergerClaude),
		orchestrator.WithSecondReviewerClaude(secondReviewerClaude),
		orchestrator.WithRunnerFactory(c.runnerFactory),
	)

	return orch, nil
}

// createValidator creates a 4-layer validator for task validation.
// The validator runs after task execution to ensure quality standards are met.
func (c *Controller) createValidator() agent.TaskValidator {
	// Create contract verifier (Layer 1)
	contractVerifier := verification.NewContractRunner(c.RepoPath)

	// Create build tester with auto-detection (Layer 2)
	buildTester, err := validation.NewAutoBuildTester(c.RepoPath, 5*time.Minute)
	if err != nil {
		// If build tester creation fails, log but use nil (validation will skip build tests)
		log.Printf("[validator] Failed to create build tester: %v", err)
		buildTester = nil
	}

	// Create validator with all 4 layers
	validator := validation.NewValidator(contractVerifier, buildTester, c.runnerFactory)

	// Wrap in adapter to implement agent.TaskValidator interface
	return validation.NewValidatorAdapter(validator)
}

// handleOrchestratorEvent converts orchestrator events to progress events.
func (c *Controller) handleOrchestratorEvent(event orchestrator.OrchestratorEvent) {
	switch event.Type {
	case orchestrator.EventTaskStarted:
		// Track active worker
		c.activeWorkers[event.AgentID] = WorkerInfo{
			AgentID:   event.AgentID,
			TaskID:    event.TaskID,
			TaskTitle: event.TaskTitle,
			Status:    "running",
		}

		c.emitProgress(ProgressEvent{
			Phase:            PhaseExecuting,
			Iteration:        c.currentIteration,
			MaxIterations:    c.MaxIterations,
			FeaturesComplete: c.currentFeaturesComplete,
			FeaturesTotal:    c.currentFeaturesTotal,
			Message:          fmt.Sprintf("Started: %s", event.TaskTitle),
			Cost:             event.Cost,
			WorkersRunning:   event.WorkersRunning,
			WorkersBlocked:   event.WorkersBlocked,
			ActiveWorkers:    c.cloneActiveWorkers(),
		})
	case orchestrator.EventTaskCompleted:
		// Remove from active workers
		delete(c.activeWorkers, event.AgentID)

		// Update feature completion tracking
		c.updateFeatureCompletion(event.TaskID)

		c.emitProgress(ProgressEvent{
			Phase:            PhaseExecuting,
			Iteration:        c.currentIteration,
			MaxIterations:    c.MaxIterations,
			FeaturesComplete: c.currentFeaturesComplete,
			FeaturesTotal:    c.currentFeaturesTotal,
			Message:          fmt.Sprintf("Completed: %s", event.TaskTitle),
			Cost:             event.Cost,
			WorkersRunning:   event.WorkersRunning,
			WorkersBlocked:   event.WorkersBlocked,
			ActiveWorkers:    c.cloneActiveWorkers(),
		})
	case orchestrator.EventTaskFailed:
		// Remove from active workers
		delete(c.activeWorkers, event.AgentID)

		c.emitProgress(ProgressEvent{
			Phase:            PhaseExecuting,
			Iteration:        c.currentIteration,
			MaxIterations:    c.MaxIterations,
			FeaturesComplete: c.currentFeaturesComplete,
			FeaturesTotal:    c.currentFeaturesTotal,
			Message:          fmt.Sprintf("Failed: %s", event.TaskTitle),
			Cost:             event.Cost,
			WorkersRunning:   event.WorkersRunning,
			WorkersBlocked:   event.WorkersBlocked,
			ActiveWorkers:    c.cloneActiveWorkers(),
		})
	case orchestrator.EventAgentProgress:
		if event.CurrentAction != "" {
			c.emitProgress(ProgressEvent{
				Phase:            PhaseExecuting,
				Iteration:        c.currentIteration,
				MaxIterations:    c.MaxIterations,
				FeaturesComplete: c.currentFeaturesComplete,
				FeaturesTotal:    c.currentFeaturesTotal,
				Message:          event.CurrentAction,
				Cost:             event.Cost,
				WorkersRunning:   event.WorkersRunning,
				WorkersBlocked:   event.WorkersBlocked,
				ActiveWorkers:    c.cloneActiveWorkers(),
			})
		}
	case orchestrator.EventTaskEscalation:
		// Emit escalation event as progress event
		c.emitProgress(ProgressEvent{
			Phase:            PhaseExecuting,
			Iteration:        c.currentIteration,
			MaxIterations:    c.MaxIterations,
			FeaturesComplete: c.currentFeaturesComplete,
			FeaturesTotal:    c.currentFeaturesTotal,
			Message:          fmt.Sprintf("Escalation: %s - %s", event.TaskTitle, event.Message),
			Cost:             event.Cost,
			WorkersRunning:   event.WorkersRunning,
			WorkersBlocked:   event.WorkersBlocked,
			ActiveWorkers:    c.cloneActiveWorkers(),
		})
	}
}

// cloneActiveWorkers creates a copy of the active workers map for safe passing.
func (c *Controller) cloneActiveWorkers() map[string]WorkerInfo {
	clone := make(map[string]WorkerInfo, len(c.activeWorkers))
	for k, v := range c.activeWorkers {
		clone[k] = v
	}
	return clone
}

// updateFeatureCompletion checks if completing this task completes a feature,
// and if so, increments the feature completion count.
func (c *Controller) updateFeatureCompletion(taskID string) {
	// Skip if tracking not initialized
	if c.featureToTasks == nil || c.completedTasks == nil {
		return
	}

	// Mark this task as completed
	c.completedTasks[taskID] = true

	// Find which feature(s) this task belongs to
	for featureID, taskIDs := range c.featureToTasks {
		// Check if this feature contains the completed task
		hasTask := false
		for _, tid := range taskIDs {
			if tid == taskID {
				hasTask = true
				break
			}
		}

		if !hasTask {
			continue
		}

		// Check if all tasks for this feature are now complete
		allComplete := true
		for _, tid := range taskIDs {
			if !c.completedTasks[tid] {
				allComplete = false
				break
			}
		}

		// If all tasks complete, increment the feature count (only once per feature)
		if allComplete {
			// Remove the feature from tracking so we don't count it again
			delete(c.featureToTasks, featureID)
			c.currentFeaturesComplete++
			log.Printf("[architect] Feature %s completed, total features complete: %d/%d",
				featureID, c.currentFeaturesComplete, c.currentFeaturesTotal)

			// Emit progress event to update TUI
			c.emitProgress(ProgressEvent{
				Phase:            PhaseExecuting,
				Iteration:        c.currentIteration,
				MaxIterations:    c.MaxIterations,
				FeaturesComplete: c.currentFeaturesComplete,
				FeaturesTotal:    c.currentFeaturesTotal,
				Message:          fmt.Sprintf("Feature completed: %s (%d/%d)", featureID, c.currentFeaturesComplete, c.currentFeaturesTotal),
				Cost:             c.tokenTracker.GetCost(),
			})
		}
	}
}
