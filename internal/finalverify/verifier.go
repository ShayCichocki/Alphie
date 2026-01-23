// Package finalverify provides comprehensive 3-layer final verification.
// This validates the entire implementation after all tasks complete.
package finalverify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/architect"
)

// FinalVerifier orchestrates the 3-layer final verification process:
// 1. Architecture audit (all features must be COMPLETE)
// 2. Build + full test suite
// 3. Comprehensive semantic review
type FinalVerifier struct {
	// auditor performs architecture audits
	auditor *architect.Auditor
	// buildTester runs build and test commands
	buildTester BuildTester
	// runnerFactory creates Claude runners for semantic review
	runnerFactory agent.ClaudeRunnerFactory
}

// BuildTester defines the interface for running build and test commands.
type BuildTester interface {
	// RunBuildAndTests runs the project's build and test commands.
	RunBuildAndTests(ctx context.Context, repoPath string) (passed bool, output string, err error)
}

// NewFinalVerifier creates a new final verifier.
func NewFinalVerifier(
	auditor *architect.Auditor,
	buildTester BuildTester,
	runnerFactory agent.ClaudeRunnerFactory,
) *FinalVerifier {
	return &FinalVerifier{
		auditor:       auditor,
		buildTester:   buildTester,
		runnerFactory: runnerFactory,
	}
}

// VerificationInput contains all information needed for final verification.
type VerificationInput struct {
	// RepoPath is the path to the repository.
	RepoPath string
	// Spec is the architecture specification.
	Spec *architect.ArchSpec
	// SpecText is the original spec text for semantic review.
	SpecText string
}

// VerificationResult contains the comprehensive results of all 3 verification layers.
type VerificationResult struct {
	// AllPassed indicates if all 3 layers passed.
	AllPassed bool
	// Layers contains results from each verification layer.
	Layers VerificationLayers
	// Summary provides a human-readable summary.
	Summary string
	// FailureReason explains why verification failed (if it did).
	FailureReason string
	// Gaps lists gaps if architecture audit found any.
	Gaps []architect.Gap
	// CompletionPercentage is the percentage of features complete (0-100).
	CompletionPercentage float64
	// Duration is the total time taken for all verifications.
	Duration time.Duration
}

// VerificationLayers contains results from each of the 3 verification layers.
type VerificationLayers struct {
	// Layer1 is the architecture audit result.
	Layer1 *LayerResult
	// Layer2 is the build + test result.
	Layer2 *LayerResult
	// Layer3 is the comprehensive semantic review result.
	Layer3 *LayerResult
}

// LayerResult contains the result from a single verification layer.
type LayerResult struct {
	// Name is the layer name (e.g., "Architecture Audit").
	Name string
	// Passed indicates if this layer passed.
	Passed bool
	// Output contains detailed output from this layer.
	Output string
	// Duration is the time taken for this layer.
	Duration time.Duration
	// Error is any error that occurred.
	Error error
	// Details contains layer-specific details (e.g., gap report).
	Details interface{}
}

// Verify runs all 3 verification layers in sequence.
func (v *FinalVerifier) Verify(ctx context.Context, input VerificationInput) (*VerificationResult, error) {
	startTime := time.Now()

	result := &VerificationResult{
		AllPassed: false,
		Layers:    VerificationLayers{},
		Gaps:      []architect.Gap{},
	}

	// Layer 1: Architecture Audit (STRICT - all features must be COMPLETE)
	layer1 := v.runLayer1(ctx, input)
	result.Layers.Layer1 = layer1

	if !layer1.Passed {
		result.AllPassed = false
		result.FailureReason = "Layer 1 (Architecture Audit) failed - not all features are complete"

		// Extract gaps from audit
		if gapReport, ok := layer1.Details.(*architect.GapReport); ok {
			result.Gaps = gapReport.Gaps
			result.CompletionPercentage = v.calculateCompletion(gapReport)
		}

		result.Duration = time.Since(startTime)
		result.Summary = v.buildSummary(result)
		return result, nil
	}

	// Layer 2: Build + Full Test Suite
	layer2 := v.runLayer2(ctx, input)
	result.Layers.Layer2 = layer2

	if !layer2.Passed {
		result.AllPassed = false
		result.FailureReason = "Layer 2 (Build + Tests) failed"
		result.Duration = time.Since(startTime)
		result.Summary = v.buildSummary(result)
		return result, nil
	}

	// Layer 3: Comprehensive Semantic Review
	layer3 := v.runLayer3(ctx, input)
	result.Layers.Layer3 = layer3

	if !layer3.Passed {
		result.AllPassed = false
		result.FailureReason = "Layer 3 (Comprehensive Semantic Review) failed"
		result.Duration = time.Since(startTime)
		result.Summary = v.buildSummary(result)
		return result, nil
	}

	// All layers passed!
	result.AllPassed = true
	result.CompletionPercentage = 100.0
	result.Duration = time.Since(startTime)
	result.Summary = v.buildSummary(result)

	return result, nil
}

// runLayer1 runs Layer 1: Architecture Audit (STRICT)
func (v *FinalVerifier) runLayer1(ctx context.Context, input VerificationInput) *LayerResult {
	startTime := time.Now()
	layer := &LayerResult{
		Name:   "Architecture Audit",
		Passed: false,
	}

	if v.auditor == nil || input.Spec == nil {
		layer.Passed = false
		layer.Output = "No auditor or spec provided"
		layer.Duration = time.Since(startTime)
		return layer
	}

	// Create a Claude runner for the audit
	claude := v.runnerFactory.NewRunner()

	// Run the audit
	gapReport, err := v.auditor.Audit(ctx, input.Spec, input.RepoPath, claude)
	if err != nil {
		layer.Error = err
		layer.Output = fmt.Sprintf("Error running audit: %v", err)
		layer.Duration = time.Since(startTime)
		return layer
	}

	layer.Details = gapReport

	// STRICT VALIDATION: All features must be COMPLETE
	allComplete := true
	completeCount := 0
	totalCount := len(gapReport.Features)

	for _, fs := range gapReport.Features {
		if fs.Status == architect.AuditStatusComplete {
			completeCount++
		} else {
			allComplete = false
		}
	}

	layer.Passed = allComplete
	layer.Output = fmt.Sprintf("Features: %d/%d complete\n%s", completeCount, totalCount, gapReport.Summary)

	if !allComplete {
		layer.Output += fmt.Sprintf("\n\nGaps found: %d\n", len(gapReport.Gaps))
		for i, gap := range gapReport.Gaps {
			layer.Output += fmt.Sprintf("\n%d. [%s] %s: %s", i+1, gap.Status, gap.FeatureID, gap.Description)
		}
	}

	layer.Duration = time.Since(startTime)
	return layer
}

// runLayer2 runs Layer 2: Build + Full Test Suite
func (v *FinalVerifier) runLayer2(ctx context.Context, input VerificationInput) *LayerResult {
	startTime := time.Now()
	layer := &LayerResult{
		Name:   "Build + Tests",
		Passed: false,
	}

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

// runLayer3 runs Layer 3: Comprehensive Semantic Review
func (v *FinalVerifier) runLayer3(ctx context.Context, input VerificationInput) *LayerResult {
	startTime := time.Now()
	layer := &LayerResult{
		Name:   "Comprehensive Semantic Review",
		Passed: false,
	}

	if v.runnerFactory == nil {
		layer.Passed = true
		layer.Output = "No runner factory configured (skipped)"
		layer.Duration = time.Since(startTime)
		return layer
	}

	// Create a Claude runner for the review
	claude := v.runnerFactory.NewRunner()

	// Build comprehensive review prompt
	prompt := v.buildComprehensiveReviewPrompt(input)

	// Run the review
	response, err := v.invokeClaudeForReview(ctx, claude, prompt, input.RepoPath)
	if err != nil {
		layer.Error = err
		layer.Output = fmt.Sprintf("Error during comprehensive review: %v", err)
		layer.Duration = time.Since(startTime)
		return layer
	}

	// Parse the response
	reviewResult := v.parseReviewResponse(response)
	layer.Passed = reviewResult.Passed
	layer.Output = fmt.Sprintf("Verdict: %s\nReasoning: %s\nGaps: %v\nRecommendations: %v",
		func() string {
			if reviewResult.Passed {
				return "PASS"
			}
			return "FAIL"
		}(),
		reviewResult.Reasoning,
		reviewResult.Gaps,
		reviewResult.Recommendations)
	layer.Duration = time.Since(startTime)

	return layer
}

// buildComprehensiveReviewPrompt constructs the prompt for comprehensive semantic review.
func (v *FinalVerifier) buildComprehensiveReviewPrompt(input VerificationInput) string {
	var sb strings.Builder

	sb.WriteString("# Comprehensive Final Review\n\n")
	sb.WriteString("You are performing a comprehensive final review of an entire implementation ")
	sb.WriteString("against its architecture specification. This is the final check before declaring ")
	sb.WriteString("the implementation complete.\n\n")

	sb.WriteString("## Architecture Specification\n\n")
	sb.WriteString(input.SpecText)
	sb.WriteString("\n\n")

	sb.WriteString("## Your Task\n\n")
	sb.WriteString("Perform a thorough review of the ENTIRE implementation and determine:\n\n")
	sb.WriteString("1. Does the implementation fulfill ALL requirements in the specification?\n")
	sb.WriteString("2. Are there any gaps, partial implementations, or missing features?\n")
	sb.WriteString("3. Is the overall architecture sound and consistent with the spec?\n")
	sb.WriteString("4. Are there any systemic issues or concerns?\n\n")

	sb.WriteString("## Review Guidelines\n\n")
	sb.WriteString("- Be STRICT: The implementation must fully match the specification\n")
	sb.WriteString("- Check for completeness across all features\n")
	sb.WriteString("- Verify feature integration and consistency\n")
	sb.WriteString("- Look for gaps, partial implementations, or workarounds\n")
	sb.WriteString("- Consider edge cases and error handling\n")
	sb.WriteString("- Assess overall code quality and maintainability\n\n")

	sb.WriteString("## Response Format\n\n")
	sb.WriteString("VERDICT: [PASS/FAIL]\n")
	sb.WriteString("REASONING: [2-3 sentences explaining your verdict]\n")
	sb.WriteString("GAPS: [List any gaps or partial implementations, or 'None']\n")
	sb.WriteString("RECOMMENDATIONS: [List any recommendations, or 'None']\n\n")

	sb.WriteString("**IMPORTANT**: Only return PASS if the implementation COMPLETELY fulfills ")
	sb.WriteString("the specification with no gaps or partial implementations.\n")

	return sb.String()
}

// invokeClaudeForReview sends the prompt to Claude and returns the response.
func (v *FinalVerifier) invokeClaudeForReview(ctx context.Context, claude agent.ClaudeRunner, prompt, repoPath string) (string, error) {
	// Start Claude with the prompt
	temp := 0.0 // Use deterministic temperature for reviews
	opts := &agent.StartOptions{
		Temperature: &temp,
	}

	if err := claude.StartWithOptions(prompt, repoPath, opts); err != nil {
		return "", fmt.Errorf("start claude: %w", err)
	}

	// Collect output
	var outputBuilder strings.Builder
	for event := range claude.Output() {
		switch event.Type {
		case agent.StreamEventAssistant, agent.StreamEventResult:
			if event.Message != "" {
				outputBuilder.WriteString(event.Message)
			}
		case agent.StreamEventError:
			if event.Error != "" {
				return "", fmt.Errorf("claude error: %s", event.Error)
			}
		}
	}

	// Wait for completion
	if err := claude.Wait(); err != nil {
		return "", fmt.Errorf("claude process failed: %w", err)
	}

	return outputBuilder.String(), nil
}

// ComprehensiveReviewResult contains the result of comprehensive semantic review.
type ComprehensiveReviewResult struct {
	Passed          bool
	Reasoning       string
	Gaps            []string
	Recommendations []string
}

// parseReviewResponse parses Claude's comprehensive review response.
func (v *FinalVerifier) parseReviewResponse(response string) *ComprehensiveReviewResult {
	result := &ComprehensiveReviewResult{
		Passed:          false,
		Gaps:            []string{},
		Recommendations: []string{},
	}

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "VERDICT:") {
			verdict := strings.TrimSpace(strings.TrimPrefix(line, "VERDICT:"))
			result.Passed = strings.ToUpper(verdict) == "PASS"
		} else if strings.HasPrefix(line, "REASONING:") {
			result.Reasoning = strings.TrimSpace(strings.TrimPrefix(line, "REASONING:"))
		} else if strings.HasPrefix(line, "GAPS:") {
			gaps := strings.TrimSpace(strings.TrimPrefix(line, "GAPS:"))
			if gaps != "" && strings.ToLower(gaps) != "none" {
				result.Gaps = strings.Split(gaps, ";")
			}
		} else if strings.HasPrefix(line, "RECOMMENDATIONS:") {
			recs := strings.TrimSpace(strings.TrimPrefix(line, "RECOMMENDATIONS:"))
			if recs != "" && strings.ToLower(recs) != "none" {
				result.Recommendations = strings.Split(recs, ";")
			}
		}
	}

	return result
}

// calculateCompletion calculates the completion percentage from a gap report.
func (v *FinalVerifier) calculateCompletion(report *architect.GapReport) float64 {
	if len(report.Features) == 0 {
		return 0.0
	}

	completeCount := 0
	for _, fs := range report.Features {
		if fs.Status == architect.AuditStatusComplete {
			completeCount++
		}
	}

	return (float64(completeCount) / float64(len(report.Features))) * 100.0
}

// buildSummary creates a human-readable summary of verification results.
func (v *FinalVerifier) buildSummary(result *VerificationResult) string {
	var sb strings.Builder

	sb.WriteString("3-Layer Final Verification Results:\n")

	layers := []*LayerResult{
		result.Layers.Layer1,
		result.Layers.Layer2,
		result.Layers.Layer3,
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

		if !layer.Passed && layer.Output != "" {
			// Show first 300 chars of output
			output := layer.Output
			if len(output) > 300 {
				output = output[:300] + "..."
			}
			sb.WriteString(fmt.Sprintf("  Details: %s\n", output))
		}
	}

	if result.AllPassed {
		sb.WriteString("\n✓ All verification layers passed!\n")
		sb.WriteString("Implementation is complete and matches specification.\n")
	} else {
		sb.WriteString(fmt.Sprintf("\n✗ Verification failed: %s\n", result.FailureReason))

		if result.CompletionPercentage > 0 {
			sb.WriteString(fmt.Sprintf("Completion: %.1f%%\n", result.CompletionPercentage))
		}

		if len(result.Gaps) > 0 {
			sb.WriteString(fmt.Sprintf("\nGaps to address: %d\n", len(result.Gaps)))
		}
	}

	sb.WriteString(fmt.Sprintf("\nTotal Duration: %v\n", result.Duration))

	return sb.String()
}
