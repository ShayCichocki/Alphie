// Package agent provides the AI agent implementation for Alphie.
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/ShayCichocki/alphie/internal/verification"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// RalphLoop orchestrates the self-improvement loop for AI-generated code.
// It wires together: implement -> critique -> score -> improve -> repeat
// with exit conditions based on threshold or max iterations.
type RalphLoop struct {
	critique             *CritiquePrompt
	controller           *IterationController
	gates                *QualityGates
	baseline             *Baseline
	testSelector         *FocusedTestSelector
	tierIgnored interface{}
	workDir              string
	verificationContract *verification.VerificationContract
	contractRunner       *verification.ContractRunner
	// runnerFactory creates ClaudeRunner instances for critique iterations.
	// If nil, falls back to creating ClaudeProcess (legacy).
	runnerFactory ClaudeRunnerFactory
}

// RalphLoopResult contains the outcome of a Ralph loop execution.
type RalphLoopResult struct {
	// FinalScore is the last rubric score achieved.
	FinalScore *models.RubricScore
	// Iterations is the number of improvement iterations performed.
	Iterations int
	// GatesPass indicates whether all enabled quality gates passed.
	GatesPass bool
	// Output is the final output from the agent.
	Output string
	// ExitReason describes why the loop terminated.
	ExitReason string
	// GateResults contains individual gate results if gates were run.
	GateResults []*GateOutput
	// VerificationResult contains the outcome of running verification commands.
	VerificationResult *verification.VerificationResult
	// VerificationPassed indicates whether all required verification checks passed.
	VerificationPassed bool
}

// NewRalphLoop creates a new RalphLoop for the given tier and work directory.
func NewRalphLoop(tierIgnored interface{}, workDir string) *RalphLoop {
	threshold := ThresholdForTier(tierIgnored)

	return &RalphLoop{
		critique:     NewCritiquePrompt(threshold),
		controller:   NewIterationController(tierIgnored),
		gates:        NewQualityGates(workDir),
		testSelector: NewFocusedTestSelector(workDir),
		tierIgnored:  tierIgnored,
		workDir:      workDir,
	}
}

// SetBaseline sets the baseline for regression detection.
// When set, quality gates will compare current results against this baseline.
func (r *RalphLoop) SetBaseline(baseline *Baseline) {
	r.baseline = baseline
}

// EnableGate enables a specific quality gate.
func (r *RalphLoop) EnableGate(gate string) {
	switch gate {
	case "test":
		r.gates.EnableTest(true)
	case "build":
		r.gates.EnableBuild(true)
	case "lint":
		r.gates.EnableLint(true)
	case "typecheck":
		r.gates.EnableTypecheck(true)
	}
}

// EnableAllGates enables all quality gates.
func (r *RalphLoop) EnableAllGates() {
	r.gates.EnableTest(true)
	r.gates.EnableBuild(true)
	r.gates.EnableLint(true)
	r.gates.EnableTypecheck(true)
}

// SetVerificationContract sets the verification contract for the loop.
// When set, the loop will run verification commands and use results in the decision matrix.
func (r *RalphLoop) SetVerificationContract(contract *verification.VerificationContract) {
	r.verificationContract = contract
	r.contractRunner = verification.NewContractRunner(r.workDir)
}

// SetRunnerFactory sets the factory for creating ClaudeRunner instances.
// This enables using direct API calls instead of subprocess.
func (r *RalphLoop) SetRunnerFactory(factory ClaudeRunnerFactory) {
	r.runnerFactory = factory
}

// createRunner creates a new ClaudeRunner using the factory.
// The factory must be set via SetRunnerFactory before calling RunCritiqueLoop.
func (r *RalphLoop) createRunner(ctx context.Context) ClaudeRunner {
	if r.runnerFactory == nil {
		panic("RalphLoop: runnerFactory is required - call SetRunnerFactory before running")
	}
	return r.runnerFactory.NewRunner()
}

// Run executes the Ralph loop with the given Claude runner and initial prompt.
// The loop continues until either:
// 1. The score threshold is met
// 2. The maximum iterations are reached
// 3. The agent outputs DONE
// 4. An error occurs
//
// At each exit point, quality gates are run if enabled.
func (r *RalphLoop) Run(ctx context.Context, claude ClaudeRunner, initialPrompt string) (*RalphLoopResult, error) {
	result := &RalphLoopResult{}

	// Step 1: Execute initial implementation
	if err := claude.Start(initialPrompt, r.workDir); err != nil {
		return nil, fmt.Errorf("start claude process: %w", err)
	}

	// Collect the initial implementation output
	initialOutput, err := r.collectOutput(ctx, claude)
	if err != nil {
		return nil, fmt.Errorf("collect initial output: %w", err)
	}

	// Wait for the process to complete
	if err := claude.Wait(); err != nil {
		return nil, fmt.Errorf("wait for initial implementation: %w", err)
	}

	result.Output = initialOutput

	// Step 2: Enter the critique loop
	for r.controller.ShouldContinue(result.FinalScore) {
		r.controller.Increment()
		result.Iterations = r.controller.GetIteration()

		// Check context cancellation
		select {
		case <-ctx.Done():
			result.ExitReason = "context cancelled"
			return r.runGatesAndFinalize(result)
		default:
		}

		// Inject critique prompt
		critiquePrompt := r.critique.InjectCritiquePrompt(result.Output)

		// Create a new Claude runner for the critique
		critiqueCtx, cancel := context.WithCancel(ctx)
		critiqueRunner := r.createRunner(critiqueCtx)

		if err := critiqueRunner.Start(critiquePrompt, r.workDir); err != nil {
			cancel()
			return nil, fmt.Errorf("start critique process (iteration %d): %w", result.Iterations, err)
		}

		// Collect critique response
		critiqueOutput, err := r.collectOutput(critiqueCtx, critiqueRunner)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("collect critique output (iteration %d): %w", result.Iterations, err)
		}

		if err := critiqueRunner.Wait(); err != nil {
			cancel()
			// Non-fatal: runner may have completed with critique output
		}
		cancel()

		// Parse the critique response
		critiqueResult, err := ParseCritiqueResponse(critiqueOutput)
		if err != nil {
			// Continue even if parsing fails - treat as incomplete critique
			continue
		}

		// Update the final score
		result.FinalScore = &critiqueResult.Score

		// Check if agent is done (DONE marker found)
		// Note: In this legacy path without verification, DONE still runs gates
		// but doesn't have contract validation. Use RunCritiqueLoop for full validation.
		if critiqueResult.IsDone {
			// Only honor DONE if threshold is at least acceptable (threshold - 1)
			if critiqueResult.Total() >= r.critique.Threshold()-1 {
				result.ExitReason = "agent_done_acceptable"
				return r.runGatesAndFinalize(result)
			}
			// Agent marked DONE but score below acceptable threshold, continuing
		}

		// Check if threshold met
		if critiqueResult.PassesThreshold(r.critique.Threshold()) {
			result.ExitReason = fmt.Sprintf("threshold met: %d/%d",
				critiqueResult.Total(), r.critique.Threshold())
			return r.runGatesAndFinalize(result)
		}

		// Check if max iterations reached
		if r.controller.IsAtMax() {
			result.ExitReason = fmt.Sprintf("max iterations reached: %d",
				r.controller.GetMaxIterations())
			return r.runGatesAndFinalize(result)
		}

		// Agent should implement improvements - the critique output contains
		// both the critique and improvements
		result.Output = critiqueOutput
	}

	// Loop exited via ShouldContinue returning false
	if result.ExitReason == "" {
		if r.controller.IsAtMax() {
			result.ExitReason = fmt.Sprintf("max iterations reached: %d",
				r.controller.GetMaxIterations())
		} else if result.FinalScore != nil && result.FinalScore.Total() >= r.critique.Threshold() {
			result.ExitReason = fmt.Sprintf("threshold met: %d/%d",
				result.FinalScore.Total(), r.critique.Threshold())
		} else {
			result.ExitReason = "loop completed"
		}
	}

	return r.runGatesAndFinalize(result)
}

// collectOutput reads all output events from the Claude runner and returns
// the concatenated message content.
func (r *RalphLoop) collectOutput(ctx context.Context, claude ClaudeRunner) (string, error) {
	var output strings.Builder

	for {
		select {
		case <-ctx.Done():
			return output.String(), ctx.Err()
		case event, ok := <-claude.Output():
			if !ok {
				return output.String(), nil
			}

			switch event.Type {
			case StreamEventAssistant:
				output.WriteString(event.Message)
			case StreamEventResult:
				output.WriteString(event.Message)
			case StreamEventError:
				if event.Error != "" {
					return output.String(), fmt.Errorf("stream error: %s", event.Error)
				}
			}
		}
	}
}

// runGatesAndFinalize runs quality gates and sets the final result state.
func (r *RalphLoop) runGatesAndFinalize(result *RalphLoopResult) (*RalphLoopResult, error) {
	gateResults, err := r.gates.RunGates()
	if err != nil {
		return result, fmt.Errorf("run quality gates: %w", err)
	}

	result.GateResults = gateResults
	result.GatesPass = r.evaluateGates(gateResults)

	return result, nil
}

// evaluateGates checks if all enabled gates passed, using baseline for regression detection.
// When a baseline is set, failures that existed before the session are not counted.
// Gates that were skipped are not counted as failures.
func (r *RalphLoop) evaluateGates(gateResults []*GateOutput) bool {
	// If no baseline, use simple pass/fail check
	if r.baseline == nil {
		for _, gate := range gateResults {
			if gate.Result == GateFail || gate.Result == GateError {
				return false
			}
		}
		return true
	}

	// With baseline: compare current results to baseline for regression detection
	current := r.extractGateResults(gateResults)
	comparison := CompareToBaseline(current, r.baseline)

	// Pass if no regressions (new failures are bad, pre-existing failures are ok)
	return !comparison.IsRegression
}

// extractGateResults converts GateOutput to GateResults for baseline comparison.
func (r *RalphLoop) extractGateResults(gateResults []*GateOutput) *GateResults {
	result := &GateResults{}

	for _, gate := range gateResults {
		if gate.Result != GateFail && gate.Result != GateError {
			continue
		}

		// Parse output to extract individual failures
		failures := parseGateOutputForFailures(gate.Gate, gate.Output)

		switch gate.Gate {
		case "test":
			result.FailingTests = append(result.FailingTests, failures...)
		case "lint":
			result.LintErrors = append(result.LintErrors, failures...)
		case "typecheck", "build":
			result.TypeErrors = append(result.TypeErrors, failures...)
		}
	}

	return result
}

// parseGateOutputForFailures extracts individual failure identifiers from gate output.
func parseGateOutputForFailures(gate string, output string) []string {
	if output == "" {
		return nil
	}

	var failures []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Different parsing based on gate type
		switch gate {
		case "test":
			// Look for "--- FAIL:" lines
			if strings.Contains(line, "FAIL") {
				failures = append(failures, line)
			}
		case "lint":
			// Each line is typically a lint error
			if !strings.HasPrefix(line, "level=") && len(line) > 10 {
				failures = append(failures, line)
			}
		case "typecheck", "build":
			// Each line with file:line format is a type error
			if strings.Contains(line, ".go:") {
				failures = append(failures, line)
			}
		}
	}

	return failures
}

// runVerification runs the verification contract if set.
// Returns the verification result and whether all required checks passed.
func (r *RalphLoop) runVerification(ctx context.Context) (*verification.VerificationResult, bool) {
	if r.verificationContract == nil || r.contractRunner == nil {
		return nil, true // No contract = verification passes by default
	}

	verifyResult, err := r.contractRunner.Run(ctx, r.verificationContract)
	if err != nil {
		// If verification fails to run, treat as not passed
		return nil, false
	}

	return verifyResult, verifyResult.AllPassed
}

// injectVerificationContext adds verification failure details to the output.
// This helps the agent understand what needs to be fixed.
func (r *RalphLoop) injectVerificationContext(output string, vr *verification.VerificationResult) string {
	if vr == nil {
		return output
	}

	var sb strings.Builder
	sb.WriteString(output)
	sb.WriteString("\n\n## Verification Failures\n")
	sb.WriteString("The following verification checks failed:\n\n")

	for _, cr := range vr.CommandResults {
		if !cr.Passed {
			sb.WriteString(fmt.Sprintf("- **Command**: `%s`\n", cr.Command))
			sb.WriteString(fmt.Sprintf("  - Exit code: %d\n", cr.ExitCode))
			if cr.Output != "" {
				// Truncate long output
				outputPreview := cr.Output
				if len(outputPreview) > 500 {
					outputPreview = outputPreview[:500] + "... (truncated)"
				}
				sb.WriteString(fmt.Sprintf("  - Output: %s\n", outputPreview))
			}
			if cr.Error != "" {
				sb.WriteString(fmt.Sprintf("  - Error: %s\n", cr.Error))
			}
		}
	}

	for _, fr := range vr.FileResults {
		if !fr.Passed {
			sb.WriteString(fmt.Sprintf("- **File constraint** `%s` on `%s`: %s\n", fr.Constraint, fr.Path, fr.Message))
		}
	}

	sb.WriteString("\nPlease fix these issues before continuing.\n")
	return sb.String()
}

// GetThreshold returns the quality threshold for this loop.
func (r *RalphLoop) GetThreshold() int {
	return r.critique.Threshold()
}

// GetMaxIterations returns the maximum iterations for this loop.
func (r *RalphLoop) GetMaxIterations() int {
	return r.controller.GetMaxIterations()
}

// GetCurrentIteration returns the current iteration number.
func (r *RalphLoop) GetCurrentIteration() int {
	return r.controller.GetIteration()
}

// RunCritiqueLoop runs only the critique/improvement loop, skipping initial implementation.
// This is used when the initial implementation was already executed by the executor.
// It takes the initial output and runs the critique loop until threshold met or max iterations.
//
// Decision matrix (with verification):
//   - If verification PASS AND score >= threshold: EXIT SUCCESS
//   - If verification PASS AND score >= threshold-1: EXIT (acceptable)
//   - If verification FAIL: inject failure context, continue improving
//   - If max iterations: EXIT with current status
func (r *RalphLoop) RunCritiqueLoop(ctx context.Context, initialOutput string) (*RalphLoopResult, error) {
	result := &RalphLoopResult{
		Output: initialOutput,
	}

	// Skip loop entirely for Scout tier
	if r.controller.GetMaxIterations() == 0 {
		result.ExitReason = "scout tier: loop skipped"
		return r.runGatesAndFinalize(result)
	}

	// Enter the critique loop
	for r.controller.ShouldContinue(result.FinalScore) {
		r.controller.Increment()
		result.Iterations = r.controller.GetIteration()

		// Check context cancellation
		select {
		case <-ctx.Done():
			result.ExitReason = "context cancelled"
			return r.runGatesAndFinalize(result)
		default:
		}

		// Run verification if contract is set
		verifyResult, verifyPassed := r.runVerification(ctx)
		result.VerificationResult = verifyResult
		result.VerificationPassed = verifyPassed

		// Inject critique prompt
		critiquePrompt := r.critique.InjectCritiquePrompt(result.Output)

		// Create a new Claude runner for the critique
		critiqueCtx, cancel := context.WithCancel(ctx)
		critiqueRunner := r.createRunner(critiqueCtx)

		if err := critiqueRunner.Start(critiquePrompt, r.workDir); err != nil {
			cancel()
			return nil, fmt.Errorf("start critique process (iteration %d): %w", result.Iterations, err)
		}

		// Collect critique response
		critiqueOutput, err := r.collectOutput(critiqueCtx, critiqueRunner)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("collect critique output (iteration %d): %w", result.Iterations, err)
		}

		if err := critiqueRunner.Wait(); err != nil {
			cancel()
			// Non-fatal: runner may have completed with critique output
		}
		cancel()

		// Parse the critique response
		critiqueResult, err := ParseCritiqueResponse(critiqueOutput)
		if err != nil {
			// Continue even if parsing fails - treat as incomplete critique
			continue
		}

		// Update the final score
		result.FinalScore = &critiqueResult.Score

		// Decision matrix with verification
		// Best case: verification passes AND score meets threshold
		if verifyPassed && critiqueResult.PassesThreshold(r.critique.Threshold()) {
			result.ExitReason = "verification_passed_and_threshold_met"
			return r.runGatesAndFinalize(result)
		}

		// Good case: verification passes AND score is acceptable (threshold - 1)
		if verifyPassed && result.FinalScore.Total() >= r.critique.Threshold()-1 {
			result.ExitReason = "verification_passed_score_acceptable"
			return r.runGatesAndFinalize(result)
		}

		// Check if agent is done (DONE marker found)
		// DONE = "agent requests exit", not "exit granted" - still validate verification
		if critiqueResult.IsDone {
			if verifyPassed {
				// Verification passes, agent can exit
				result.ExitReason = "agent_done_verified"
				return r.runGatesAndFinalize(result)
			}
			// Agent wants to exit but verification fails - keep trying
			// (agent marked DONE but verification failed, continuing improvement)
		}

		// Check if max iterations reached
		if r.controller.IsAtMax() {
			result.ExitReason = fmt.Sprintf("max_iterations_reached: %d",
				r.controller.GetMaxIterations())
			return r.runGatesAndFinalize(result)
		}

		// Need improvement - inject verification failures into context if verification failed
		if !verifyPassed && verifyResult != nil {
			result.Output = r.injectVerificationContext(critiqueOutput, verifyResult)
		} else {
			result.Output = critiqueOutput
		}
	}

	// Loop exited via ShouldContinue returning false
	if result.ExitReason == "" {
		if r.controller.IsAtMax() {
			result.ExitReason = fmt.Sprintf("max_iterations_reached: %d",
				r.controller.GetMaxIterations())
		} else if result.FinalScore != nil && result.FinalScore.Total() >= r.critique.Threshold() {
			result.ExitReason = fmt.Sprintf("threshold_met: %d/%d",
				result.FinalScore.Total(), r.critique.Threshold())
		} else {
			result.ExitReason = "loop_completed"
		}
	}

	return r.runGatesAndFinalize(result)
}
