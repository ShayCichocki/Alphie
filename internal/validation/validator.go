// Package validation provides comprehensive 4-layer validation for task implementations.
package validation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/verification"
)

// Validator orchestrates the 4-layer validation process:
// 1. Verification contracts
// 2. Build + test suite
// 3. Semantic validation
// 4. Code review
type Validator struct {
	// contractVerifier runs verification contracts
	contractVerifier verification.ContractVerifier
	// buildTester runs build and test commands
	buildTester BuildTester
	// semanticValidator performs semantic validation
	semanticValidator *SemanticValidator
	// codeReviewer performs code review
	codeReviewer *CodeReviewer
	// runnerFactory creates Claude runners for validation
	runnerFactory agent.ClaudeRunnerFactory
}

// BuildTester defines the interface for running build and test commands.
type BuildTester interface {
	// RunBuildAndTests runs the project's build and test commands.
	// Returns true if all passed, along with output and any error.
	RunBuildAndTests(ctx context.Context, repoPath string) (passed bool, output string, err error)
}

// NewValidator creates a new 4-layer validator.
func NewValidator(
	contractVerifier verification.ContractVerifier,
	buildTester BuildTester,
	runnerFactory agent.ClaudeRunnerFactory,
) *Validator {
	return &Validator{
		contractVerifier:  contractVerifier,
		buildTester:       buildTester,
		semanticValidator: NewSemanticValidator(runnerFactory),
		codeReviewer:      NewCodeReviewer(runnerFactory),
		runnerFactory:     runnerFactory,
	}
}

// ValidationInput contains all information needed for 4-layer validation.
type ValidationInput struct {
	// RepoPath is the path to the repository being validated.
	RepoPath string
	// TaskTitle is the task title.
	TaskTitle string
	// TaskDescription is the full task description.
	TaskDescription string
	// VerificationContract is the verification contract if available.
	VerificationContract *verification.VerificationContract
	// Implementation contains the code changes (diff or full content).
	Implementation string
	// ModifiedFiles lists files that were changed.
	ModifiedFiles []string
	// AcceptanceCriteria lists acceptance criteria for the task.
	AcceptanceCriteria []string
}

// ValidationResult contains the comprehensive results of all 4 validation layers.
type ValidationResult struct {
	// AllPassed indicates if all 4 layers passed.
	AllPassed bool
	// Layers contains results from each validation layer.
	Layers ValidationLayers
	// Summary provides a human-readable summary.
	Summary string
	// FailureReason explains why validation failed (if it did).
	FailureReason string
	// Duration is the total time taken for all validations.
	Duration time.Duration
}

// ValidationLayers contains results from each of the 4 validation layers.
type ValidationLayers struct {
	// Layer1 is the verification contracts result.
	Layer1 *LayerResult
	// Layer2 is the build + test result.
	Layer2 *LayerResult
	// Layer3 is the semantic validation result.
	Layer3 *LayerResult
	// Layer4 is the code review result.
	Layer4 *LayerResult
}

// LayerResult contains the result from a single validation layer.
type LayerResult struct {
	// Name is the layer name (e.g., "Verification Contracts").
	Name string
	// Passed indicates if this layer passed.
	Passed bool
	// Output contains detailed output from this layer.
	Output string
	// Score is a quality score if applicable (0-10).
	Score int
	// Duration is the time taken for this layer.
	Duration time.Duration
	// Error is any error that occurred.
	Error error
}

// Validate runs all 4 validation layers in sequence.
// If any layer fails, subsequent layers may be skipped (configurable).
func (v *Validator) Validate(ctx context.Context, input ValidationInput) (*ValidationResult, error) {
	startTime := time.Now()

	result := &ValidationResult{
		AllPassed: false,
		Layers:    ValidationLayers{},
		Summary:   "",
	}

	// Layer 1: Verification Contracts
	layer1 := v.runLayer1(ctx, input)
	result.Layers.Layer1 = layer1
	if !layer1.Passed {
		result.AllPassed = false
		result.FailureReason = "Layer 1 (Verification Contracts) failed"
		result.Duration = time.Since(startTime)
		result.Summary = v.buildSummary(result)
		return result, nil
	}

	// Layer 2: Build + Test Suite
	layer2 := v.runLayer2(ctx, input)
	result.Layers.Layer2 = layer2
	if !layer2.Passed {
		result.AllPassed = false
		result.FailureReason = "Layer 2 (Build + Tests) failed"
		result.Duration = time.Since(startTime)
		result.Summary = v.buildSummary(result)
		return result, nil
	}

	// Layer 3: Semantic Validation
	layer3 := v.runLayer3(ctx, input)
	result.Layers.Layer3 = layer3
	if !layer3.Passed {
		result.AllPassed = false
		result.FailureReason = "Layer 3 (Semantic Validation) failed"
		result.Duration = time.Since(startTime)
		result.Summary = v.buildSummary(result)
		return result, nil
	}

	// Layer 4: Code Review
	layer4 := v.runLayer4(ctx, input)
	result.Layers.Layer4 = layer4
	if !layer4.Passed {
		result.AllPassed = false
		result.FailureReason = "Layer 4 (Code Review) failed"
		result.Duration = time.Since(startTime)
		result.Summary = v.buildSummary(result)
		return result, nil
	}

	// All layers passed!
	result.AllPassed = true
	result.Duration = time.Since(startTime)
	result.Summary = v.buildSummary(result)

	return result, nil
}

// runLayer1 runs Layer 1: Verification Contracts
func (v *Validator) runLayer1(ctx context.Context, input ValidationInput) *LayerResult {
	startTime := time.Now()
	layer := &LayerResult{
		Name:   "Verification Contracts",
		Passed: false,
	}

	// If no contract verifier or no contract, skip but pass
	if v.contractVerifier == nil || input.VerificationContract == nil {
		layer.Passed = true
		layer.Output = "No verification contract provided (skipped)"
		layer.Duration = time.Since(startTime)
		return layer
	}

	// Run the verification contract
	contractResult, err := v.contractVerifier.Run(ctx, input.VerificationContract)
	if err != nil {
		layer.Error = err
		layer.Output = fmt.Sprintf("Error running verification: %v", err)
		layer.Duration = time.Since(startTime)
		return layer
	}

	layer.Passed = contractResult.AllPassed
	layer.Output = contractResult.Summary
	layer.Duration = time.Since(startTime)

	return layer
}

// runLayer2 runs Layer 2: Build + Test Suite
func (v *Validator) runLayer2(ctx context.Context, input ValidationInput) *LayerResult {
	startTime := time.Now()
	layer := &LayerResult{
		Name:   "Build + Tests",
		Passed: false,
	}

	// If no build tester, skip but pass
	if v.buildTester == nil {
		layer.Passed = true
		layer.Output = "No build tester configured (skipped)"
		layer.Duration = time.Since(startTime)
		return layer
	}

	// Run build and tests
	passed, output, err := v.buildTester.RunBuildAndTests(ctx, input.RepoPath)
	layer.Passed = passed
	layer.Output = output
	layer.Error = err
	layer.Duration = time.Since(startTime)

	return layer
}

// runLayer3 runs Layer 3: Semantic Validation
func (v *Validator) runLayer3(ctx context.Context, input ValidationInput) *LayerResult {
	startTime := time.Now()
	layer := &LayerResult{
		Name:   "Semantic Validation",
		Passed: false,
	}

	// If no semantic validator, skip but pass
	if v.semanticValidator == nil {
		layer.Passed = true
		layer.Output = "Semantic validation not configured (skipped)"
		layer.Duration = time.Since(startTime)
		return layer
	}

	// Build semantic validation input
	semanticInput := SemanticValidationInput{
		TaskTitle:       input.TaskTitle,
		TaskDescription: input.TaskDescription,
		TaskIntent: func() string {
			if input.VerificationContract != nil {
				return input.VerificationContract.Intent
			}
			return ""
		}(),
		Implementation: input.Implementation,
		ModifiedFiles:  input.ModifiedFiles,
	}

	// Run semantic validation
	semanticResult, err := v.semanticValidator.Validate(ctx, semanticInput)
	if err != nil {
		layer.Error = err
		layer.Output = fmt.Sprintf("Error during semantic validation: %v", err)
		layer.Duration = time.Since(startTime)
		return layer
	}

	layer.Passed = semanticResult.Passed
	layer.Output = fmt.Sprintf("Reasoning: %s\nConcerns: %v\nSuggestions: %v",
		semanticResult.Reasoning,
		semanticResult.Concerns,
		semanticResult.Suggestions)
	layer.Duration = time.Since(startTime)

	return layer
}

// runLayer4 runs Layer 4: Code Review
func (v *Validator) runLayer4(ctx context.Context, input ValidationInput) *LayerResult {
	startTime := time.Now()
	layer := &LayerResult{
		Name:   "Code Review",
		Passed: false,
	}

	// If no code reviewer, skip but pass
	if v.codeReviewer == nil {
		layer.Passed = true
		layer.Output = "Code review not configured (skipped)"
		layer.Duration = time.Since(startTime)
		return layer
	}

	// Build code review input
	reviewInput := CodeReviewInput{
		TaskTitle:          input.TaskTitle,
		TaskDescription:    input.TaskDescription,
		AcceptanceCriteria: input.AcceptanceCriteria,
		Implementation:     input.Implementation,
		ModifiedFiles:      input.ModifiedFiles,
	}

	// Run code review
	reviewResult, err := v.codeReviewer.Review(ctx, reviewInput)
	if err != nil {
		layer.Error = err
		layer.Output = fmt.Sprintf("Error during code review: %v", err)
		layer.Duration = time.Since(startTime)
		return layer
	}

	layer.Passed = reviewResult.Passed
	layer.Score = reviewResult.Score
	layer.Output = fmt.Sprintf("Score: %d/10 (Completeness: %d, Correctness: %d, Quality: %d)\nIssues: %d\nSummary: %s",
		reviewResult.Score,
		reviewResult.Completeness,
		reviewResult.Correctness,
		reviewResult.Quality,
		len(reviewResult.Issues),
		reviewResult.Summary)
	layer.Duration = time.Since(startTime)

	return layer
}

// buildSummary creates a human-readable summary of validation results.
func (v *Validator) buildSummary(result *ValidationResult) string {
	var sb strings.Builder

	sb.WriteString("4-Layer Validation Results:\n")

	layers := []*LayerResult{
		result.Layers.Layer1,
		result.Layers.Layer2,
		result.Layers.Layer3,
		result.Layers.Layer4,
	}

	for i, layer := range layers {
		if layer == nil {
			continue
		}
		status := "✗ FAIL"
		if layer.Passed {
			status = "✓ PASS"
		}
		sb.WriteString(fmt.Sprintf("\nLayer %d (%s): %s [%v]\n", i+1, layer.Name, status, layer.Duration))
		if layer.Score > 0 {
			sb.WriteString(fmt.Sprintf("  Score: %d/10\n", layer.Score))
		}
		if !layer.Passed && layer.Output != "" {
			// Show first 200 chars of output
			output := layer.Output
			if len(output) > 200 {
				output = output[:200] + "..."
			}
			sb.WriteString(fmt.Sprintf("  Details: %s\n", output))
		}
	}

	if result.AllPassed {
		sb.WriteString("\n✓ All validation layers passed!\n")
	} else {
		sb.WriteString(fmt.Sprintf("\n✗ Validation failed: %s\n", result.FailureReason))
	}

	sb.WriteString(fmt.Sprintf("\nTotal Duration: %v\n", result.Duration))

	return sb.String()
}
