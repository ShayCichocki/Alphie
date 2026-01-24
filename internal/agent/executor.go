// Package agent provides the AI agent implementation for Alphie.
package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ShayCichocki/alphie/pkg/models"
)

// ExecutionResult contains the outcome of a single task execution.
// Uses primitive fields for post-processing results to avoid leaking
// orchestrator-specific types across package boundaries.
type ExecutionResult struct {
	// Core result fields (always populated)
	// Success indicates whether the task completed successfully.
	Success bool
	// Output contains the captured output from the agent.
	Output string
	// Error contains the error message if execution failed.
	Error string

	// Metrics (always populated)
	// TokensUsed is the total number of tokens consumed.
	TokensUsed int64
	// Cost is the total cost in dollars.
	Cost float64
	// Duration is how long the execution took.
	Duration time.Duration

	// Context (always populated)
	// AgentID is the ID of the agent that executed the task.
	AgentID string
	// WorktreePath is the path to the worktree used for execution.
	WorktreePath string
	// Model is the Claude model that was dynamically selected for this task.
	Model string
	// LogFile is the path to the detailed execution log.
	LogFile string

	// Post-processing results (primitives, zero values = not run)
	// LoopIterations is the number of self-critique loop iterations performed.
	// Zero indicates the loop was not run.
	LoopIterations int
	// LoopExitReason describes why the loop terminated (empty if not run).
	LoopExitReason string
	// GatesPassed indicates whether quality gates passed. Nil means gates were not run.
	GatesPassed *bool
	// VerifyPassed indicates whether verification passed. Nil means verification was not run.
	VerifyPassed *bool
	// VerifySummary is a human-readable summary of verification results.
	VerifySummary string
}

// AreGatesPassed returns whether quality gates passed, or true if not run.
func (r *ExecutionResult) AreGatesPassed() bool {
	return r.GatesPassed == nil || *r.GatesPassed
}

// IsVerified returns whether verification passed, or true if not run.
func (r *ExecutionResult) IsVerified() bool {
	return r.VerifyPassed == nil || *r.VerifyPassed
}

// TaskValidator defines the interface for validating task implementations.
// This interface allows the Executor to validate without importing the validation package directly.
type TaskValidator interface {
	// Validate runs 4-layer validation and returns the result.
	Validate(ctx context.Context, input TaskValidationInput) (*TaskValidationResult, error)
}

// TaskValidationInput contains information needed for task validation.
type TaskValidationInput struct {
	RepoPath             string
	TaskTitle            string
	TaskDescription      string
	VerificationContract interface{} // verification.VerificationContract
	Implementation       string
	ModifiedFiles        []string
	AcceptanceCriteria   []string
}

// TaskValidationResult contains the outcome of task validation.
type TaskValidationResult struct {
	AllPassed     bool
	Summary       string
	FailureReason string
}

// Executor wires together worktree creation, API execution,
// stream parsing, token tracking, and cleanup for single-agent task execution.
type Executor struct {
	worktreeMgr   WorktreeProvider
	tokenTracker  TokenAggregator
	agentMgr      AgentLifecycle
	model         string
	taskTimeout   time.Duration

	// Runner factory for creating ClaudeRunner instances (API-based)
	runnerFactory ClaudeRunnerFactory

	// validator performs 4-layer validation after task completion (optional)
	validator TaskValidator
}

// ExecutorConfig contains configuration options for the Executor.
type ExecutorConfig struct {
	// WorktreeBaseDir is where worktrees are created (defaults to ~/.cache/alphie/worktrees).
	WorktreeBaseDir string
	// RepoPath is the path to the main git repository.
	RepoPath string
	// Model is the Claude model to use for cost calculation.
	Model string
	// TaskTimeout is the maximum duration for a single task execution.
	// Default is 10 minutes if not specified.
	TaskTimeout time.Duration

	// RunnerFactory creates ClaudeRunner instances for API execution.
	// Required - the Executor always uses direct API calls.
	RunnerFactory ClaudeRunnerFactory

	// Optional dependency injection (nil = use defaults)
	// TokenTracker is the token aggregator. If nil, NewAggregateTracker() is used.
	TokenTracker TokenAggregator
	// AgentManager is the agent lifecycle manager. If nil, NewManager() is used.
	AgentManager AgentLifecycle
	// Validator performs 4-layer validation after task completion. If nil, validation is skipped.
	Validator TaskValidator
}

// NewExecutor creates a new Executor with the given configuration.
func NewExecutor(cfg ExecutorConfig) (*Executor, error) {
	worktreeMgr, err := NewWorktreeManager(cfg.WorktreeBaseDir, cfg.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("create worktree manager: %w", err)
	}

	model := cfg.Model
	if model == "" {
		model = "sonnet"
	}

	taskTimeout := cfg.TaskTimeout
	if taskTimeout == 0 {
		taskTimeout = 20 * time.Minute
	}

	// RunnerFactory is always required - API is the only execution path
	if cfg.RunnerFactory == nil {
		return nil, fmt.Errorf("RunnerFactory is required")
	}

	// Use injected dependencies or fall back to defaults
	tokenTracker := cfg.TokenTracker
	if tokenTracker == nil {
		tokenTracker = NewAggregateTracker()
	}

	agentMgr := cfg.AgentManager
	if agentMgr == nil {
		agentMgr = NewManager()
	}

	return &Executor{
		worktreeMgr:   worktreeMgr,
		tokenTracker:  tokenTracker,
		agentMgr:      agentMgr,
		model:         model,
		taskTimeout:   taskTimeout,
		runnerFactory: cfg.RunnerFactory,
		validator:     cfg.Validator,
	}, nil
}

// ProgressUpdate contains current execution progress information.
type ProgressUpdate struct {
	// AgentID is the ID of the agent executing the task.
	AgentID string
	// TokensUsed is the current total tokens consumed.
	TokensUsed int64
	// Cost is the current total cost in dollars.
	Cost float64
	// Duration is time elapsed since execution started.
	Duration time.Duration
	// CurrentAction describes what the agent is doing right now (e.g., "Reading auth.go").
	CurrentAction string
}

// ProgressCallback is called periodically during task execution with progress updates.
type ProgressCallback func(update ProgressUpdate)

// ExecuteOptions contains optional parameters for task execution.
type ExecuteOptions struct {
	// AgentID is the pre-assigned agent ID to use. If empty, a new ID is generated.
	// This allows the orchestrator to track agents consistently.
	AgentID string
	// OnProgress is called periodically with execution progress updates.
	// Can be nil if progress updates are not needed.
	OnProgress ProgressCallback
	// EnableRalphLoop enables the self-critique loop after initial execution.
	// When enabled, the agent will critique its own work and iterate until
	// the quality threshold is met or max iterations reached.
	// Tier-specific: Scout skips (0 iterations), Builder gets 3, Architect gets 5.
	EnableRalphLoop bool
	// EnableQualityGates runs quality gates (lint, build, test, typecheck) after execution.
	// Gates are tierIgnored-specific: Scout=lint, Builder=build+lint+typecheck, Architect=all.
	EnableQualityGates bool
	// Baseline is the session baseline for regression detection.
	// When set, quality gates compare against baseline to detect new failures.
	Baseline *Baseline
	// StructureRules provides directory structure guidance to the agent.
	// When set, the agent receives information about common directory patterns.
	StructureRules interface{} // Uses interface{} to avoid circular dependency
}

// Execute runs a single task with a single agent.
// It creates an isolated worktree, starts the Claude Code process,
// streams and parses output, tracks tokens, waits for completion,
// cleans up the worktree, and returns the result.
func (e *Executor) Execute(ctx context.Context, task *models.Task, tierIgnored interface{}) (*ExecutionResult, error) {
	return e.ExecuteWithOptions(ctx, task, tierIgnored, nil)
}

// startupTimeout is how long we wait for the Claude CLI to produce its first output.
// If no output is received within this time, we assume startup hung and retry.
const startupTimeout = 45 * time.Second

// maxStartupRetries is the maximum number of times to retry if startup hangs.
const maxStartupRetries = 2

// ExecuteWithOptions runs a single task with a single agent, accepting optional parameters.
// It creates an isolated worktree, starts the Claude Code process,
// streams and parses output, tracks tokens, waits for completion,
// cleans up the worktree, and returns the result.
func (e *Executor) ExecuteWithOptions(ctx context.Context, task *models.Task, tierIgnored interface{}, opts *ExecuteOptions) (*ExecutionResult, error) {
	startTime := time.Now()
	result := &ExecutionResult{}

	// Learning system removed - no learnings tracking

	// Apply task timeout
	ctx, cancel := context.WithTimeout(ctx, e.taskTimeout)
	defer cancel()

	// Create log file for this task
	logDir := filepath.Join(e.worktreeMgr.RepoPath(), ".alphie", "logs")
	_ = os.MkdirAll(logDir, 0755)
	logFileName := fmt.Sprintf("task-%s-%s.log", task.ID[:8], startTime.Format("150405"))
	logFile := filepath.Join(logDir, logFileName)
	result.LogFile = logFile

	// 1. Create worktree
	worktree, err := e.worktreeMgr.Create(task.ID)
	if err != nil {
		return nil, fmt.Errorf("create worktree: %w", err)
	}
	result.WorktreePath = worktree.Path

	// Ensure cleanup happens regardless of outcome
	defer func() {
		// Force remove the worktree on cleanup
		_ = e.worktreeMgr.Remove(worktree.Path, true)
	}()

	// 2. Create agent and token tracker
	// Use provided AgentID if available, otherwise generate a new one
	var agentID string
	if opts != nil && opts.AgentID != "" {
		agentID = opts.AgentID
	}
	agent, err := e.agentMgr.CreateWithID(agentID, task.ID, worktree.Path)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}
	result.AgentID = agent.ID

	// Select model dynamically based on task keywords and tierIgnored
	selectedModel := SelectModel(task, tierIgnored)
	result.Model = selectedModel
	tracker := NewTokenTracker(selectedModel)
	e.tokenTracker.Add(agent.ID, tracker)
	defer e.tokenTracker.Remove(agent.ID)

	// 3. Build the prompt from task
	prompt := e.buildPrompt(task, tierIgnored, opts)

	// Declare variables used across both pre-impl contract and execution
	var proc ClaudeRunner
	var procErr error
	var outputBuilder strings.Builder
	var currentAction string

	// 3b. Generate draft verification contract BEFORE implementation
	// This establishes minimum verification requirements that cannot be weakened
	verifyCtx := e.generateDraftContract(ctx, task.ID, task.VerificationIntent, task.FileBoundaries, worktree.Path)

	// 4. Start Claude Code process with retry logic for startup hangs

	for attempt := 0; attempt <= maxStartupRetries; attempt++ {
		if attempt > 0 {
			// Log retry attempt
			outputBuilder.WriteString(fmt.Sprintf("\n[Retry attempt %d: previous startup timed out after %v]\n", attempt, startupTimeout))
			// Brief delay before retry to avoid hammering
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}

		// Create runner via factory (API is the only execution path)
		proc = e.runnerFactory.NewRunner()
		startOpts := &StartOptions{Model: selectedModel}
		if err := proc.StartWithOptions(prompt, worktree.Path, startOpts); err != nil {
			_ = e.agentMgr.Fail(agent.ID, fmt.Sprintf("failed to start process: %v", err))
			return nil, fmt.Errorf("start claude process: %w", err)
		}

		// Transition agent to running (only on first attempt)
		if attempt == 0 {
			if err := e.agentMgr.Start(agent.ID, proc.PID()); err != nil {
				_ = proc.Kill()
				return nil, fmt.Errorf("start agent: %w", err)
			}
		}

		// 5. Stream and parse output with startup timeout detection
		startupTimedOut := false
		gotFirstOutput := false
		startupDeadline := time.Now().Add(startupTimeout)
		lastProgressUpdate := time.Now()
		progressInterval := 2 * time.Second

	streamLoop:
		for {
			select {
			case event, ok := <-proc.Output():
				if !ok {
					// Channel closed, process done
					break streamLoop
				}

				gotFirstOutput = true
				e.processStreamEvent(event, tracker, &outputBuilder)

				// Track current tool action
				if event.ToolAction != "" {
					currentAction = event.ToolAction
				}

				// Send periodic progress updates
				if opts != nil && opts.OnProgress != nil && time.Since(lastProgressUpdate) >= progressInterval {
					usage := tracker.GetUsage()
					opts.OnProgress(ProgressUpdate{
						AgentID:       agent.ID,
						TokensUsed:    usage.TotalTokens,
						Cost:          tracker.GetCost(),
						Duration:      time.Since(startTime),
						CurrentAction: currentAction,
					})
					lastProgressUpdate = time.Now()
				}

			case <-time.After(100 * time.Millisecond):
				// Check startup timeout only if we haven't received any output yet
				if !gotFirstOutput && time.Now().After(startupDeadline) {
					startupTimedOut = true
					outputBuilder.WriteString(fmt.Sprintf("\n[Startup timeout: no output received within %v]\n", startupTimeout))
					_ = proc.Kill()
					break streamLoop
				}
			}
		}

		// If startup timed out and we have retries left, continue to next attempt
		if startupTimedOut && attempt < maxStartupRetries {
			continue
		}

		// Otherwise, we're done with retries (either success or final failure)
		procErr = proc.Wait()
		break
	}

	// Capture final results
	result.Output = outputBuilder.String()
	result.Duration = time.Since(startTime)

	usage := tracker.GetUsage()
	result.TokensUsed = usage.TotalTokens
	result.Cost = tracker.GetCost()

	// Update agent with usage
	_ = e.agentMgr.UpdateUsage(agent.ID, usage.TotalTokens, result.Cost)

	// 6b. Run Ralph-loop if enabled and appropriate for tierIgnored
	if procErr == nil {
		e.runRalphLoopIfEnabled(ctx, result, task, tierIgnored, opts, worktree.Path, verifyCtx)
	}

	// 7. Auto-commit any changes made by the agent
	// This ensures changes are preserved when the worktree is removed
	if procErr == nil {
		if err := e.autoCommitChanges(worktree.Path, task.Title); err != nil {
			// Log but don't fail - agent might have made no changes
			result.Output += fmt.Sprintf("\n[Auto-commit: %v]", err)
		}
	}

	// 8. Determine success/failure
	if procErr != nil || ctx.Err() != nil {
		e.handleExecutionFailure(ctx, result, proc, procErr, agent.ID)
	} else {
		result.Success = true
		_ = e.agentMgr.Complete(agent.ID)
	}

	// Run quality gates if enabled and execution succeeded
	if result.Success {
		e.runQualityGatesIfEnabled(result, opts, worktree.Path, tierIgnored, agent.ID)
	}

	// Run 4-layer validation if execution succeeded
	// This provides comprehensive validation beyond contracts and gates
	if result.Success {
		e.run4LayerValidation(ctx, result, task, worktree.Path, opts)
	}

	// Unified pass/fail: both verification and gates must pass
	e.checkVerificationPassed(result, agent.ID)

	// Write detailed log file
	e.writeLogFile(logFile, task, tierIgnored, result, startTime)

	return result, nil
}


// run4LayerValidation runs comprehensive 4-layer validation after task completion.
// This validates:
// 1. Verification contracts (already run via verifyCtx)
// 2. Build + test suite
// 3. Semantic validation (Claude reviews against intent)
// 4. Code review (detailed quality assessment)
func (e *Executor) run4LayerValidation(ctx context.Context, result *ExecutionResult, task *models.Task, worktreePath string, opts *ExecuteOptions) {
	// Skip if no validator configured
	if e.validator == nil {
		return
	}

	// Skip if execution failed
	if !result.Success {
		return
	}

	// Get repo path for validation
	repoPath := e.worktreeMgr.RepoPath()

	// Get implementation diff and modified files
	implementation := e.getImplementationDiff(worktreePath)
	modifiedFiles := e.getModifiedFiles(worktreePath)

	// Convert acceptance criteria string to slice
	var acceptanceCriteria []string
	if task.AcceptanceCriteria != "" {
		acceptanceCriteria = strings.Split(task.AcceptanceCriteria, "\n")
	}

	// Build validation input
	validationInput := TaskValidationInput{
		RepoPath:             repoPath,
		TaskTitle:            task.Title,
		TaskDescription:      task.Description,
		VerificationContract: nil, // Task doesn't have contract field yet
		Implementation:       implementation,
		ModifiedFiles:        modifiedFiles,
		AcceptanceCriteria:   acceptanceCriteria,
	}

	// Run validation
	validationResult, err := e.validator.Validate(ctx, validationInput)
	if err != nil {
		result.VerifySummary = fmt.Sprintf("Validation error: %v", err)
		passed := false
		result.VerifyPassed = &passed
		return
	}

	// Update result with validation outcome
	result.VerifyPassed = &validationResult.AllPassed
	result.VerifySummary = validationResult.Summary

	// If validation failed, mark the entire execution as failed
	if !validationResult.AllPassed {
		result.Success = false
		result.Error = validationResult.FailureReason
	}
}
