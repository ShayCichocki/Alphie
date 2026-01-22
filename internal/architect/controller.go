// Package architect provides tools for analyzing and auditing codebases against specifications.
package architect

import (
	"context"
	"fmt"
	"time"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/orchestrator"
	"github.com/ShayCichocki/alphie/internal/prog"
	"github.com/ShayCichocki/alphie/internal/state"
	"github.com/ShayCichocki/alphie/pkg/models"
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
	// Message is an optional status message.
	Message string
	// Timestamp is when the event occurred.
	Timestamp time.Time
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
	// ProjectName is the prog project name for task management.
	ProjectName string

	// parser parses architecture documents into feature specs.
	parser *Parser
	// auditor checks the codebase against the spec.
	auditor *Auditor
	// planner creates prog epics and tasks from gaps.
	planner *Planner
	// stopper evaluates stop conditions.
	stopper *StopChecker
	// progClient manages prog tasks.
	progClient *prog.Client
	// onProgress is called when progress events occur.
	onProgress ProgressCallback
	// runnerFactory creates ClaudeRunner instances.
	// If nil, falls back to creating ClaudeProcess (legacy).
	runnerFactory agent.ClaudeRunnerFactory
	// tokenTracker tracks cumulative token usage and cost.
	tokenTracker *agent.TokenTracker

	// Current state tracking (for progress events during execution)
	currentIteration     int
	currentFeaturesTotal int
	currentFeaturesComplete int
}

// ControllerOption is a functional option for configuring a Controller.
type ControllerOption func(*Controller)

// WithRepoPath sets the repository path for the controller.
func WithRepoPath(path string) ControllerOption {
	return func(c *Controller) {
		c.RepoPath = path
	}
}

// WithProjectName sets the prog project name.
func WithProjectName(name string) ControllerOption {
	return func(c *Controller) {
		c.ProjectName = name
	}
}

// WithProgClient sets a custom prog client.
func WithProgClient(client *prog.Client) ControllerOption {
	return func(c *Controller) {
		c.progClient = client
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
		tokenTracker: agent.NewTokenTracker("sonnet"),
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
	// Initialize prog client if not provided
	if c.progClient == nil && c.ProjectName != "" {
		client, err := prog.NewClientDefault(c.ProjectName)
		if err != nil {
			return fmt.Errorf("create prog client: %w", err)
		}
		c.progClient = client
		defer client.Close()
	}

	// Initialize planner with prog client
	if c.progClient != nil {
		c.planner = NewPlanner(c.progClient)
	}

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

		if gapsFound > 0 && c.planner != nil {
			c.emitProgress(ProgressEvent{
				Phase:            PhasePlanning,
				Iteration:        iteration,
				FeaturesComplete: completedFeatures,
				FeaturesTotal:    totalFeatures,
				GapsFound:        gapsFound,
				Cost:             totalCost,
				Message:          fmt.Sprintf("Iteration %d/%d: Planning tasks for %d gaps...", iteration, c.MaxIterations, gapsFound),
			})

			planClaude := c.createRunner(ctx)
			planResult, err := c.planner.Plan(ctx, gapReport, c.ProjectName, planClaude)
			if err != nil {
				return fmt.Errorf("plan epics (iteration %d): %w", iteration, err)
			}

			iterResult.EpicID = planResult.EpicID
			iterResult.TasksCreated = len(planResult.TaskIDs)

			// Step 5: Execute epics via /alphie skill pattern
			if planResult.EpicID != "" {
				c.emitProgress(ProgressEvent{
					Phase:            PhaseExecuting,
					Iteration:        iteration,
					FeaturesComplete: completedFeatures,
					FeaturesTotal:    totalFeatures,
					GapsFound:        gapsFound,
					TasksCreated:     len(planResult.TaskIDs),
					EpicID:           planResult.EpicID,
					Cost:             totalCost,
					Message:          fmt.Sprintf("Iteration %d/%d: Executing epic %s with %d tasks...", iteration, c.MaxIterations, planResult.EpicID, len(planResult.TaskIDs)),
				})

				completed, err := c.executeEpic(ctx, planResult.EpicID, agents)
				if err != nil {
					// Log error but continue to next iteration
					// Epic execution failures are not fatal to the loop
					c.emitProgress(ProgressEvent{
						Phase:     PhaseExecuting,
						Iteration: iteration,
						EpicID:    planResult.EpicID,
						Cost:      totalCost,
						Message:   fmt.Sprintf("Warning: epic execution failed: %v", err),
					})
				}
				iterResult.TasksCompleted = completed
			}
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

	// Count completed tasks from prog client
	if c.progClient != nil {
		completed, total, err := c.progClient.ComputeEpicProgress(epicID)
		if err != nil {
			return 0, fmt.Errorf("compute epic progress: %w", err)
		}
		// Update epic status if complete
		if completed == total && total > 0 {
			_, _ = c.progClient.UpdateEpicStatusIfComplete(epicID)
		}
		return completed, nil
	}

	return 0, nil
}

// createOrchestrator creates a new orchestrator instance for epic execution.
func (c *Controller) createOrchestrator(epicID string, agents int) (*orchestrator.Orchestrator, error) {
	// Open state database
	db, err := state.OpenProject(c.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("open state database: %w", err)
	}

	// Run migrations
	if err := db.Migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	// Create executor
	executor, err := agent.NewExecutor(agent.ExecutorConfig{
		RepoPath:      c.RepoPath,
		Model:         "sonnet",
		RunnerFactory: c.runnerFactory,
	})
	if err != nil {
		db.Close()
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
			Tier:     models.TierBuilder,
			Executor: executor,
		},
		orchestrator.WithMaxAgents(agents),
		orchestrator.WithDecomposerClaude(decomposerClaude),
		orchestrator.WithMergerClaude(mergerClaude),
		orchestrator.WithSecondReviewerClaude(secondReviewerClaude),
		orchestrator.WithRunnerFactory(c.runnerFactory),
		orchestrator.WithStateDB(db),
		orchestrator.WithProgClient(c.progClient),
		orchestrator.WithResumeEpicID(epicID),
	)

	return orch, nil
}

// handleOrchestratorEvent converts orchestrator events to progress events.
func (c *Controller) handleOrchestratorEvent(event orchestrator.OrchestratorEvent) {
	switch event.Type {
	case orchestrator.EventTaskStarted:
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
		})
	case orchestrator.EventTaskCompleted:
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
			})
		}
	}
}
