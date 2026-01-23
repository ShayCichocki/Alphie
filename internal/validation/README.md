# Validation Package

Comprehensive 4-layer validation system for Alphie task implementations.

## Overview

This package provides a rigorous validation framework that ensures every task implementation meets quality standards before merging. It implements the validation strategy from the Alphie Simplification Plan Phase 3.

## 4 Validation Layers

### Layer 1: Verification Contracts
- **Purpose**: Executable tests that verify task completion
- **Implementation**: Uses existing `internal/verification/contract.go`
- **What it checks**: Commands pass, files exist/don't exist, expectations met
- **When to use**: When you have specific verification commands for a task

### Layer 2: Build + Test Suite
- **Purpose**: Ensure code compiles and tests pass
- **Implementation**: `build_test.go` with auto-detection of project type
- **What it checks**: `go build`, `npm test`, `cargo build`, etc.
- **When to use**: Always - critical for code integrity

### Layer 3: Semantic Validation
- **Purpose**: Claude reviews if implementation matches intent
- **Implementation**: `semantic.go` with Claude-based analysis
- **What it checks**: Does the code fulfill the task requirements?
- **When to use**: For complex tasks where correctness isn't obvious from tests

### Layer 4: Code Review
- **Purpose**: Detailed quality assessment
- **Implementation**: `review.go` with structured review criteria
- **What it checks**: Completeness (0-10), Correctness (0-10), Quality (0-10)
- **When to use**: Always - ensures high code quality

## Architecture

```
validator.go (orchestrator)
    ├── Layer 1: Verification Contracts (verification.ContractVerifier)
    ├── Layer 2: Build + Tests (BuildTester interface)
    ├── Layer 3: Semantic Validation (SemanticValidator)
    └── Layer 4: Code Review (CodeReviewer)
```

## Usage

### Basic Usage

```go
import (
    "github.com/ShayCichocki/alphie/internal/validation"
    "github.com/ShayCichocki/alphie/internal/verification"
)

// Create components
contractVerifier := verification.NewContractRunner(repoPath)
buildTester, _ := validation.NewAutoBuildTester(repoPath, 5*time.Minute)
validator := validation.NewValidator(contractVerifier, buildTester, runnerFactory)

// Run validation
result, err := validator.Validate(ctx, validation.ValidationInput{
    RepoPath:             "/path/to/repo",
    TaskTitle:            "Add user authentication",
    TaskDescription:      "Implement JWT-based authentication",
    VerificationContract: contract,
    Implementation:       diffOutput,
    ModifiedFiles:        []string{"auth.go", "middleware.go"},
    AcceptanceCriteria:   []string{"Users can login", "Tokens expire"},
})

if result.AllPassed {
    fmt.Println("✓ All layers passed!")
    fmt.Println(result.Summary)
} else {
    fmt.Printf("✗ Failed: %s\n", result.FailureReason)
    fmt.Println(result.Summary)
}
```

### With Retry Logic

```go
// Create retry handler
retryHandler := validation.NewRetryHandler(validation.DefaultRetryConfig())
retryExecutor := validation.NewExecuteWithRetry(validator, retryHandler, 3)

// Execute with retry
result, validation, err := retryExecutor.Execute(
    ctx,
    taskExecutor,
    execInput,
    validationInput,
)

if validation != nil && validation.AllPassed {
    // Success!
} else {
    // Max retries reached - escalate to user
}
```

## Integration with Agent Executor

The validation system is designed to integrate with `internal/agent/executor.go`:

```go
// In agent/executor.go Execute method, after task completion:

// 1. Create validation input
validationInput := validation.ValidationInput{
    RepoPath:             e.repoPath,
    TaskTitle:            task.Title,
    TaskDescription:      task.Description,
    VerificationContract: task.VerificationContract,
    Implementation:       getDiff(worktreePath),
    ModifiedFiles:        getModifiedFiles(worktreePath),
    AcceptanceCriteria:   task.AcceptanceCriteria,
}

// 2. Run validation
validationResult, err := e.validator.Validate(ctx, validationInput)
if err != nil {
    return handleValidationError(err)
}

// 3. Store results in ExecutionResult
result.ValidationPassed = &validationResult.AllPassed
result.ValidationSummary = validationResult.Summary

// 4. Only merge if validation passed
if !validationResult.AllPassed {
    result.Success = false
    result.Error = validationResult.FailureReason
    return result
}
```

## Current Status & TODOs

### ✅ Complete
- [x] Layer 1 integration (verification contracts exist)
- [x] Layer 2 implementation (build + test with auto-detection)
- [x] Layer 3 structure (semantic validation framework)
- [x] Layer 4 structure (code review framework)
- [x] Validator orchestrator
- [x] Retry logic with feedback injection
- [x] Documentation and examples

### 🚧 Needs Integration
- [ ] **Claude invocation in semantic.go and review.go**
  - Currently has placeholder `invokeClaudeForValidation/Review` methods
  - Need to integrate with `agent.Executor` or create dedicated Claude runner
  - Should reuse `agent.ClaudeRunnerFactory` for consistency

- [ ] **Add validation to agent.ExecutionResult**
  - Add fields: `ValidationPassed *bool`, `ValidationSummary string`
  - Update `agent/executor.go` to run validation after execution

- [ ] **Wire into orchestrator**
  - Orchestrator should use validator before merging
  - Retry logic should inject validation feedback
  - Escalation should include validation details

### 📋 Future Enhancements
- [ ] Parallel layer execution (where possible)
- [ ] Configurable layer ordering
- [ ] Layer result caching
- [ ] Progressive validation (stop early on critical failures)
- [ ] Learning integration (capture validation patterns)

## Testing

Each component has clear interfaces for testing:

```go
// Mock build tester
type mockBuildTester struct {
    shouldPass bool
}

func (m *mockBuildTester) RunBuildAndTests(ctx context.Context, repoPath string) (bool, string, error) {
    return m.shouldPass, "mock output", nil
}

// Test validator
func TestValidator(t *testing.T) {
    validator := validation.NewValidator(nil, &mockBuildTester{shouldPass: true}, nil)
    result, err := validator.Validate(ctx, input)
    assert.NoError(t, err)
    assert.True(t, result.AllPassed)
}
```

## Files

- `validator.go` - Main orchestrator for 4-layer validation
- `semantic.go` - Semantic validation (Layer 3)
- `review.go` - Code review (Layer 4)
- `build_test.go` - Build + test validation (Layer 2)
- `retry.go` - Retry logic with validation feedback
- `doc.go` - Package documentation
- `README.md` - This file

## Error Handling

Validation distinguishes between:

1. **Layer Failures** - Validation didn't pass (AllPassed = false)
2. **Execution Errors** - Error running validation (Error field set)
3. **Timeouts** - Validation exceeded timeout (context.DeadlineExceeded)

The validator continues through all layers even on failures, providing comprehensive feedback.

## Performance

- Layers run sequentially (takes ~5-30 seconds total depending on project)
- Layer 1: ~1-5 seconds (verification commands)
- Layer 2: ~5-60 seconds (build + tests)
- Layer 3: ~3-10 seconds (Claude semantic review)
- Layer 4: ~5-15 seconds (Claude code review)

Semantic and code review layers involve API calls, adding latency but providing valuable insights.

## Contributing

When adding new validation layers:

1. Create new file (e.g., `security.go`)
2. Implement validation logic
3. Add to `validator.go` orchestrator
4. Update `ValidationLayers` struct
5. Update documentation
6. Add tests

## License

Same as parent project (Alphie).
