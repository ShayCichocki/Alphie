// Package architect provides tools for analyzing and auditing codebases against specifications.
package architect

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/prog"
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
			Message:   "Parsing architecture document...",
		})

		claude := c.createRunner(ctx)
		spec, err := c.parser.Parse(ctx, archDoc, claude)
		if err != nil {
			return fmt.Errorf("parse architecture doc (iteration %d): %w", iteration, err)
		}

		// Step 2: Audit codebase for gaps
		c.emitProgress(ProgressEvent{
			Phase:         PhaseAuditing,
			Iteration:     iteration,
			FeaturesTotal: len(spec.Features),
			Cost:          totalCost,
			Message:       fmt.Sprintf("Auditing codebase against %d features...", len(spec.Features)),
		})

		auditClaude := c.createRunner(ctx)
		gapReport, err := c.auditor.Audit(ctx, spec, c.RepoPath, auditClaude)
		if err != nil {
			return fmt.Errorf("audit codebase (iteration %d): %w", iteration, err)
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

		// Determine if progress was made
		progressMade := lastGapCount < 0 || gapsFound < lastGapCount
		lastGapCount = gapsFound

		// Estimate cost for this iteration (placeholder - actual cost tracking would need integration)
		iterationCost := 0.01 * float64(gapsFound+1) // Simple estimate
		totalCost += iterationCost

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

		// Step 4: Plan epics from gaps (if there are gaps and we have a planner)
		if gapsFound > 0 && c.planner != nil {
			c.emitProgress(ProgressEvent{
				Phase:            PhasePlanning,
				Iteration:        iteration,
				FeaturesComplete: completedFeatures,
				FeaturesTotal:    totalFeatures,
				GapsFound:        gapsFound,
				Cost:             totalCost,
				Message:          fmt.Sprintf("Planning tasks for %d gaps...", gapsFound),
			})

			planResult, err := c.planner.Plan(ctx, gapReport, c.ProjectName)
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
					Message:          fmt.Sprintf("Executing epic %s with %d tasks...", planResult.EpicID, len(planResult.TaskIDs)),
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
			Message:          fmt.Sprintf("Iteration %d complete: %d/%d features, %d gaps remaining", iteration, completedFeatures, totalFeatures, gapsFound),
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

// executeEpic runs the /alphie skill pattern to execute an epic.
// It invokes Claude with the /alphie command to process the epic's tasks.
// Returns the number of tasks completed and any error.
func (c *Controller) executeEpic(ctx context.Context, epicID string, agents int) (int, error) {
	// Build the command to invoke Claude with /alphie skill
	// The /alphie skill pattern expects an epic ID and handles task execution
	prompt := fmt.Sprintf("/alphie run --epic %s --headless", epicID)
	if agents > 0 {
		prompt = fmt.Sprintf("/alphie run --epic %s --headless --agents %d", epicID, agents)
	}

	// Execute via Claude CLI
	cmd := exec.CommandContext(ctx, "claude", "--print", prompt)
	if c.RepoPath != "" {
		cmd.Dir = c.RepoPath
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("execute claude: %w (output: %s)", err, string(output))
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

	// If no prog client, estimate from output
	// Look for completion indicators in the output
	completedCount := strings.Count(string(output), "[DONE]")
	return completedCount, nil
}
