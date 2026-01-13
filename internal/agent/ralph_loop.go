// Package agent provides the AI agent implementation for Alphie.
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/shayc/alphie/pkg/models"
)

// RalphLoop orchestrates the self-improvement loop for AI-generated code.
// It wires together: implement -> critique -> score -> improve -> repeat
// with exit conditions based on threshold or max iterations.
type RalphLoop struct {
	critique     *CritiquePrompt
	controller   *IterationController
	gates        *QualityGates
	baseline     *Baseline
	testSelector *FocusedTestSelector
	tier         models.Tier
	workDir      string
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
}

// NewRalphLoop creates a new RalphLoop for the given tier and work directory.
func NewRalphLoop(tier models.Tier, workDir string) *RalphLoop {
	threshold := ThresholdForTier(tier)

	return &RalphLoop{
		critique:     NewCritiquePrompt(threshold),
		controller:   NewIterationController(tier),
		gates:        NewQualityGates(workDir),
		testSelector: NewFocusedTestSelector(workDir),
		tier:         tier,
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

// Run executes the Ralph loop with the given Claude process and initial prompt.
// The loop continues until either:
// 1. The score threshold is met
// 2. The maximum iterations are reached
// 3. The agent outputs DONE
// 4. An error occurs
//
// At each exit point, quality gates are run if enabled.
func (r *RalphLoop) Run(ctx context.Context, claude *ClaudeProcess, initialPrompt string) (*RalphLoopResult, error) {
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

		// Create a new Claude process for the critique
		critiqueCtx, cancel := context.WithCancel(ctx)
		critiqueProcess := NewClaudeProcess(critiqueCtx)

		if err := critiqueProcess.Start(critiquePrompt, r.workDir); err != nil {
			cancel()
			return nil, fmt.Errorf("start critique process (iteration %d): %w", result.Iterations, err)
		}

		// Collect critique response
		critiqueOutput, err := r.collectOutput(critiqueCtx, critiqueProcess)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("collect critique output (iteration %d): %w", result.Iterations, err)
		}

		if err := critiqueProcess.Wait(); err != nil {
			cancel()
			// Non-fatal: process may have completed with critique output
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
		if critiqueResult.IsDone {
			result.ExitReason = "agent marked DONE"
			return r.runGatesAndFinalize(result)
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

// collectOutput reads all output events from the Claude process and returns
// the concatenated message content.
func (r *RalphLoop) collectOutput(ctx context.Context, claude *ClaudeProcess) (string, error) {
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

// evaluateGates checks if all enabled gates passed.
// Gates that were skipped are not counted as failures.
func (r *RalphLoop) evaluateGates(gateResults []*GateOutput) bool {
	for _, gate := range gateResults {
		if gate.Result == GateFail || gate.Result == GateError {
			return false
		}
	}
	return true
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
