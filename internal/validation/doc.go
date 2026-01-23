// Package validation provides comprehensive 4-layer validation for task implementations.
//
// # Overview
//
// This package implements the 4-layer validation strategy for Alphie:
//
//   1. Verification Contracts - Executable tests that verify task completion
//   2. Build + Test Suite - Project build and test commands
//   3. Semantic Validation - Claude reviews implementation against intent
//   4. Code Review - Detailed code review against acceptance criteria
//
// # Architecture
//
// The validation system is designed to be:
//   - Modular: Each layer is independent and can be used separately
//   - Extensible: New validation layers can be added easily
//   - Testable: Each component has clear interfaces for testing
//   - Asynchronous: Supports context cancellation and timeouts
//
// # Usage
//
// Basic usage with all 4 layers:
//
//	// Create components
//	contractVerifier := verification.NewContractRunner(repoPath)
//	buildTester, _ := validation.NewAutoBuildTester(repoPath, 5*time.Minute)
//	validator := validation.NewValidator(contractVerifier, buildTester, runnerFactory)
//
//	// Run validation
//	result, err := validator.Validate(ctx, validation.ValidationInput{
//	    RepoPath:             repoPath,
//	    TaskTitle:            "Add user authentication",
//	    TaskDescription:      "Implement JWT-based authentication",
//	    VerificationContract: contract,
//	    Implementation:       diffOutput,
//	    ModifiedFiles:        []string{"auth.go", "middleware.go"},
//	    AcceptanceCriteria:   []string{"Users can login", "Tokens expire after 24h"},
//	})
//
//	if result.AllPassed {
//	    fmt.Println("All validation layers passed!")
//	}
//
// # Integration with Agent Executor
//
// The validation system integrates with the agent executor through retry logic:
//
//	for attempt := 1; attempt <= maxRetries; attempt++ {
//	    // Execute task
//	    result := executor.Execute(ctx, task, opts)
//
//	    // Run validation
//	    validation := validator.Validate(ctx, validationInput)
//
//	    if validation.AllPassed {
//	        return result // Success!
//	    }
//
//	    // Inject failure context for next iteration
//	    opts.FailureContext = validation.Summary
//	}
//
//	// Max retries reached - escalate to user
//	return needsEscalation(result, validation)
//
// # Semantic Validation and Code Review
//
// These layers use Claude to perform intelligent analysis. The current
// implementation has placeholders for Claude invocation that need integration
// with agent.Executor or ClaudeRunner.
//
// To complete integration:
//   1. Update SemanticValidator.invokeClaudeForValidation to use agent.Executor
//   2. Update CodeReviewer.invokeClaudeForReview to use agent.Executor
//   3. Ensure proper context and cancellation handling
//   4. Add cost tracking for validation Claude calls
//
// # Error Handling
//
// Validation errors are categorized:
//   - Layer failures: When a validation layer fails (AllPassed = false)
//   - Execution errors: When a layer encounters an error (Error field set)
//   - Timeouts: When validation exceeds timeout (context.DeadlineExceeded)
//
// The validator continues to subsequent layers even if earlier ones fail,
// to provide comprehensive feedback. This behavior can be customized.
//
// # Performance Considerations
//
//   - Layers run sequentially (1→2→3→4)
//   - Each layer has its own timeout
//   - Semantic validation and code review involve API calls to Claude
//   - Consider parallel execution of independent layers (future optimization)
//
// # Testing
//
// Each component can be tested independently:
//
//	// Mock build tester
//	type mockBuildTester struct{}
//	func (m *mockBuildTester) RunBuildAndTests(ctx context.Context, repoPath string) (bool, string, error) {
//	    return true, "all tests passed", nil
//	}
//
//	validator := validation.NewValidator(nil, &mockBuildTester{}, nil)
//
// # Future Enhancements
//
//   - Parallel layer execution where possible
//   - Configurable layer ordering
//   - Layer skipping based on confidence scores
//   - Caching of validation results
//   - Integration with learning system for validation patterns
package validation
