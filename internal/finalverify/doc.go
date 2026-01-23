// Package finalverify provides comprehensive 3-layer final verification.
//
// # Overview
//
// This package implements the final verification strategy for Alphie:
//
//   1. Architecture Audit - STRICT validation that all features are COMPLETE
//   2. Build + Test Suite - Full project build and all tests must pass
//   3. Comprehensive Semantic Review - Claude reviews entire implementation vs spec
//
// This is different from package validation (internal/validation) which validates
// individual tasks before merge. Final verification validates the ENTIRE implementation
// after all tasks are complete, before declaring the implementation finished.
//
// # Architecture
//
// The final verification system consists of:
//   - FinalVerifier: Orchestrates the 3 verification layers
//   - GapAnalyzer: Analyzes failures and generates tasks to fix gaps
//   - Integration with architect.Auditor for architecture audits
//   - Integration with BuildTester for build/test validation
//
// # Usage
//
// Basic usage after all tasks complete:
//
//	// Create components
//	auditor := architect.NewAuditor()
//	buildTester, _ := validation.NewAutoBuildTester(repoPath, 5*time.Minute)
//	verifier := finalverify.NewFinalVerifier(auditor, buildTester, runnerFactory)
//
//	// Run final verification
//	result, err := verifier.Verify(ctx, finalverify.VerificationInput{
//	    RepoPath: repoPath,
//	    Spec:     parsedSpec,
//	    SpecText: originalSpecText,
//	})
//
//	if result.AllPassed {
//	    fmt.Println("✓ Implementation complete!")
//	    return success
//	}
//
//	// Verification failed - analyze gaps and generate tasks
//	gapAnalyzer := finalverify.NewGapAnalyzer(runnerFactory)
//	analysis, err := gapAnalyzer.AnalyzeGaps(ctx, finalverify.GapAnalysisInput{
//	    RepoPath:           repoPath,
//	    Gaps:               result.Gaps,
//	    VerificationResult: result,
//	    SpecText:           originalSpecText,
//	})
//
//	// Create tasks for gaps and retry
//	tasks := createTasksFromAnalysis(analysis.SuggestedTasks)
//	orchestrator.Execute(ctx, tasks) // Retry with gap-fixing tasks
//
// # Integration with Implement Command
//
// The implement command should use final verification in an iteration loop:
//
//	for {
//	    // 1. Parse spec
//	    spec := parseSpec(specPath)
//
//	    // 2. Decompose into tasks (or use gap analysis results)
//	    tasks := decompose(spec, previousGaps)
//
//	    // 3. Execute tasks via orchestrator
//	    orchestrator.Execute(ctx, tasks)
//
//	    // 4. Run final verification
//	    result := verifier.Verify(ctx, input)
//
//	    if result.AllPassed {
//	        return success // Done!
//	    }
//
//	    // 5. Analyze gaps and generate fix tasks
//	    analysis := gapAnalyzer.AnalyzeGaps(ctx, gapInput)
//
//	    // 6. Create tasks for gaps and loop
//	    previousGaps = analysis.SuggestedTasks
//	}
//
// # The 3 Verification Layers
//
// ## Layer 1: Architecture Audit (STRICT)
//
// Uses internal/architect/auditor.go to audit features:
//   - Parses spec and analyzes codebase
//   - Returns status for each feature: COMPLETE, PARTIAL, or MISSING
//   - **STRICT MODE**: Only passes if ALL features are COMPLETE
//   - Returns gaps for PARTIAL/MISSING features
//
// This is stricter than the auditor's normal mode, which may accept
// PARTIAL implementations. Final verification requires 100% completion.
//
// ## Layer 2: Build + Test Suite
//
// Runs full project build and test suite:
//   - Reuses BuildTester interface from internal/validation
//   - Runs project build command (go build, npm run build, etc.)
//   - Runs entire test suite (go test ./..., npm test, etc.)
//   - Must pass with zero errors
//
// ## Layer 3: Comprehensive Semantic Review
//
// Claude performs a thorough review of the entire implementation:
//   - Reviews ALL features together (not individually)
//   - Checks for integration issues and consistency
//   - Validates overall architecture alignment with spec
//   - Identifies subtle gaps or partial implementations
//   - Returns PASS only if implementation completely fulfills spec
//
// This is more thorough than per-task semantic validation in package validation.
//
// # Gap Analysis
//
// When verification fails, GapAnalyzer helps identify what to fix:
//
//   1. Analyzes verification failures (gaps, test failures, review feedback)
//   2. Generates specific, actionable tasks to address gaps
//   3. Prioritizes tasks (critical, high, medium, low)
//   4. Provides overall analysis and recommended approach
//
// Generated tasks are fed back into the orchestrator for another iteration.
//
// # Iteration Strategy
//
// The implement command uses final verification in an iteration loop:
//
//   - Iteration 1: Implement from spec
//   - Verify → gaps found → generate gap-fix tasks
//   - Iteration 2: Execute gap-fix tasks
//   - Verify → more gaps found → generate more tasks
//   - Iteration 3: Execute remaining gap-fix tasks
//   - Verify → all passed → success!
//
// The loop continues until verification passes or max iterations reached.
//
// # Difference from Task Validation
//
// **Task Validation** (internal/validation):
//   - Validates individual tasks before merge
//   - 4 layers: contracts, build, semantic, review
//   - Ensures each task is correct
//   - Per-task feedback and retry
//
// **Final Verification** (internal/finalverify):
//   - Validates entire implementation after all tasks complete
//   - 3 layers: audit, build, comprehensive review
//   - Ensures complete system matches spec
//   - Identifies systemic gaps and integration issues
//   - Generates tasks to fix gaps
//
// Both are necessary:
//   - Task validation prevents bad code from merging
//   - Final verification ensures the complete system is correct
//
// # Error Handling
//
// Verification errors are categorized:
//   - Layer failures: When a layer fails (AllPassed = false)
//   - Execution errors: When a layer encounters an error (Error field set)
//   - Timeouts: When verification exceeds timeout
//
// All errors include detailed context for debugging.
//
// # Performance
//
// Final verification takes ~30-120 seconds:
//   - Layer 1: 10-30 seconds (architecture audit with Claude)
//   - Layer 2: 10-60 seconds (build + full test suite)
//   - Layer 3: 10-30 seconds (comprehensive review with Claude)
//
// This is acceptable overhead before declaring implementation complete.
//
// # Testing
//
// Each component has clear interfaces for testing:
//
//	// Mock auditor
//	type mockAuditor struct{ report *architect.GapReport }
//	func (m *mockAuditor) Audit(...) (*architect.GapReport, error) {
//	    return m.report, nil
//	}
//
//	// Mock build tester
//	type mockBuildTester struct{ shouldPass bool }
//	func (m *mockBuildTester) RunBuildAndTests(...) (bool, string, error) {
//	    return m.shouldPass, "mock output", nil
//	}
//
//	// Test verifier
//	verifier := finalverify.NewFinalVerifier(mockAuditor, mockBuildTester, nil)
//	result, _ := verifier.Verify(ctx, input)
//
// # Future Enhancements
//
//   - Incremental verification (only re-verify changed features)
//   - Confidence scoring (skip review if very confident)
//   - Verification result caching
//   - Parallel layer execution where possible
//   - Learning integration (learn from common gaps)
package finalverify
