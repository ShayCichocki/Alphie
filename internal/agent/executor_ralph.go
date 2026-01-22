// Package agent provides the AI agent implementation for Alphie.
package agent

import (
	"context"
	"fmt"

	"github.com/ShayCichocki/alphie/pkg/models"
)

// runRalphLoopIfEnabled runs the Ralph self-critique loop if conditions are met.
// It returns the modified output and updates the result with loop outcomes.
func (e *Executor) runRalphLoopIfEnabled(
	ctx context.Context,
	result *ExecutionResult,
	task *models.Task,
	tier models.Tier,
	opts *ExecuteOptions,
	worktreePath string,
	verifyCtx *verificationContext,
) {
	if opts == nil || !opts.EnableRalphLoop || !e.shouldRunRalphLoop(tier) {
		return
	}

	ralphLoop := NewRalphLoop(tier, worktreePath)
	ralphLoop.SetRunnerFactory(e.runnerFactory)

	// Enable gates based on tier for the ralph loop's internal gate checks
	gateConfig := GateConfigForTier(tier)
	if gateConfig.Lint {
		ralphLoop.EnableGate("lint")
	}
	if gateConfig.Build {
		ralphLoop.EnableGate("build")
	}
	if gateConfig.Test {
		ralphLoop.EnableGate("test")
	}
	if gateConfig.TypeCheck {
		ralphLoop.EnableGate("typecheck")
	}

	// Set baseline for regression detection if provided
	if opts.Baseline != nil {
		ralphLoop.SetBaseline(opts.Baseline)
	}

	// Generate verification contract using draft→refine flow
	// Draft was generated pre-implementation; now refine post-implementation
	if task.VerificationIntent != "" {
		modifiedFiles := e.getModifiedFiles(worktreePath)
		finalContract := e.refineVerificationContract(ctx, verifyCtx, task.ID, task.VerificationIntent, modifiedFiles, worktreePath)
		result.Output += verifyCtx.output.String()
		if finalContract != nil {
			ralphLoop.SetVerificationContract(finalContract)
		}
	}

	loopResult, loopErr := ralphLoop.RunCritiqueLoop(ctx, result.Output)
	if loopErr != nil {
		// Log but don't fail - loop is enhancement, not critical path
		result.Output += fmt.Sprintf("\n[Ralph-loop error: %v]\n", loopErr)
		return
	}

	// Update result with loop outcomes (primitive fields)
	result.LoopIterations = loopResult.Iterations
	result.LoopExitReason = loopResult.ExitReason
	result.VerifyPassed = &loopResult.VerificationPassed
	if loopResult.VerificationResult != nil {
		result.VerifySummary = loopResult.VerificationResult.Summary
	}
	if loopResult.Output != "" {
		result.Output = loopResult.Output
	}
}

// handleExecutionFailure processes a failed execution, setting error info and capturing learnings.
func (e *Executor) handleExecutionFailure(
	ctx context.Context,
	result *ExecutionResult,
	proc ClaudeRunner,
	procErr error,
	agentID string,
) {
	result.Success = false

	if ctx.Err() == context.DeadlineExceeded {
		result.Error = fmt.Sprintf("task timed out after %v", e.taskTimeout)
	} else if procErr != nil {
		result.Error = procErr.Error()
		if stderr := proc.Stderr(); stderr != "" {
			result.Error += "; stderr: " + stderr
		}
	} else if ctx.Err() != nil {
		result.Error = ctx.Err().Error()
	}

	// Add diagnostic information if process failed without producing output
	if result.TokensUsed == 0 && result.Duration > 60*1e9 { // 60 seconds in nanoseconds
		result.Error += fmt.Sprintf(" [diagnostic: process ran for %v but used 0 tokens - "+
			"likely hung during startup or authentication. Check Claude CLI access and credentials.]",
			result.Duration.Round(1e9))
	}

	_ = e.agentMgr.Fail(agentID, result.Error)

	// Capture potential learnings from failure
	if e.failureAnalyzer != nil {
		result.SuggestedLearnings = e.failureAnalyzer.AnalyzeFailure(result.Output, result.Error)
	}
}

// runQualityGatesIfEnabled runs quality gates and updates result accordingly.
func (e *Executor) runQualityGatesIfEnabled(
	result *ExecutionResult,
	opts *ExecuteOptions,
	worktreePath string,
	tier models.Tier,
	agentID string,
) {
	if opts == nil || !opts.EnableQualityGates {
		return
	}

	gateResults := e.runQualityGates(worktreePath, tier)
	passed := e.evaluateGatesWithBaseline(gateResults, opts.Baseline)
	result.GatesPassed = &passed

	// Gates failing now blocks task completion
	if !passed {
		result.Success = false
		result.Error = "quality gates failed (regression detected or new failures)"
		_ = e.agentMgr.Fail(agentID, result.Error)
	}
}

// checkVerificationPassed checks if verification passed and updates result if failed.
func (e *Executor) checkVerificationPassed(result *ExecutionResult, agentID string) {
	if result.Success && !result.IsVerified() {
		result.Success = false
		result.Error = "verification contract failed"
		_ = e.agentMgr.Fail(agentID, result.Error)
	}
}
