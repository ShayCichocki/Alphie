# Final Verification Package

Comprehensive 3-layer final verification system for validating complete implementations against specifications.

## Overview

This package provides the **final checkpoint** before declaring an implementation complete. While `internal/validation` validates individual tasks before merge, `finalverify` validates the **entire implementation** after all tasks complete.

**Key Principle**: Build it right, no matter how long it takes. Iterate until verification passes.

## The 3 Verification Layers

### Layer 1: Architecture Audit (STRICT) ✅
**Purpose**: Ensure all features are 100% complete

**How it works**:
- Uses `internal/architect/auditor.go` to audit codebase
- Claude analyzes each feature against the spec
- Returns status: COMPLETE, PARTIAL, or MISSING
- **STRICT MODE**: Only passes if ALL features are COMPLETE

**When it runs**: First layer of final verification

**Example result**:
```
Features: 8/10 complete
Gaps found: 2

1. [PARTIAL] authentication: Token refresh not implemented
2. [MISSING] admin-panel: Admin interface not found
```

### Layer 2: Build + Full Test Suite ✅
**Purpose**: Ensure code compiles and all tests pass

**How it works**:
- Reuses `BuildTester` interface from `internal/validation`
- Runs project build command (all packages/modules)
- Runs entire test suite (all tests, not just changed)
- Must pass with zero errors

**When it runs**: After architecture audit passes

**Supported projects**: Go, Node.js, Python, Rust (auto-detected)

### Layer 3: Comprehensive Semantic Review ✅
**Purpose**: Thorough review of entire system

**How it works**:
- Claude reviews the ENTIRE implementation (not individual tasks)
- Checks all features together for integration and consistency
- Validates overall architecture alignment with spec
- Identifies subtle gaps or partial implementations
- Only returns PASS if implementation COMPLETELY fulfills spec

**When it runs**: After build + tests pass

**More thorough than per-task validation**: Reviews integration, consistency, edge cases

## Architecture

```
FinalVerifier
├── Layer 1: Architecture Audit
│   └── Uses architect.Auditor + ClaudeRunner
├── Layer 2: Build + Tests
│   └── Uses BuildTester interface
└── Layer 3: Comprehensive Review
    └── Uses ClaudeRunner with comprehensive prompt

GapAnalyzer
└── Analyzes failures and generates tasks
```

## Usage

### Basic Final Verification

```go
import (
    "github.com/ShayCichocki/alphie/internal/finalverify"
    "github.com/ShayCichocki/alphie/internal/architect"
    "github.com/ShayCichocki/alphie/internal/validation"
)

// Create components
auditor := architect.NewAuditor()
buildTester, _ := validation.NewAutoBuildTester(repoPath, 5*time.Minute)
verifier := finalverify.NewFinalVerifier(auditor, buildTester, runnerFactory)

// Run final verification
result, err := verifier.Verify(ctx, finalverify.VerificationInput{
    RepoPath: repoPath,
    Spec:     parsedSpec,
    SpecText: originalSpecText,
})

if result.AllPassed {
    fmt.Println("✓ Implementation complete!")
    fmt.Println(result.Summary)
} else {
    fmt.Printf("✗ Failed: %s\n", result.FailureReason)
    fmt.Printf("Completion: %.1f%%\n", result.CompletionPercentage)
    fmt.Printf("Gaps: %d\n", len(result.Gaps))
}
```

### With Gap Analysis

```go
if !result.AllPassed {
    // Analyze gaps and generate tasks
    gapAnalyzer := finalverify.NewGapAnalyzer(runnerFactory)
    analysis, err := gapAnalyzer.AnalyzeGaps(ctx, finalverify.GapAnalysisInput{
        RepoPath:           repoPath,
        Gaps:               result.Gaps,
        VerificationResult: result,
        SpecText:           originalSpecText,
    })

    if err != nil {
        return err
    }

    fmt.Println("Generated tasks to fix gaps:")
    fmt.Println(finalverify.FormatTasks(analysis.SuggestedTasks))

    // Create tasks and retry
    for _, suggestedTask := range analysis.SuggestedTasks {
        task := createTask(suggestedTask)
        tasks = append(tasks, task)
    }

    // Execute gap-fixing tasks
    orchestrator.Execute(ctx, tasks)
}
```

### Complete Iteration Loop

```go
// In cmd/alphie/implement.go
func runImplement(specPath string) error {
    spec := parseSpec(specPath)

    for iteration := 1; iteration <= maxIterations; iteration++ {
        // Decompose into tasks
        tasks := decompose(spec, previousGaps)

        // Execute tasks
        orchestrator.Execute(ctx, tasks)

        // Final verification
        result, err := verifier.Verify(ctx, verificationInput)
        if err != nil {
            return err
        }

        if result.AllPassed {
            fmt.Println("✓ All verification passed!")
            return nil
        }

        // Analyze gaps
        analysis, err := gapAnalyzer.AnalyzeGaps(ctx, gapAnalysisInput)
        if err != nil {
            return err
        }

        // Generate tasks for next iteration
        previousGaps = analysis.SuggestedTasks

        fmt.Printf("Iteration %d: %d gaps found, creating fix tasks...\n",
            iteration, len(analysis.SuggestedTasks))
    }

    return fmt.Errorf("max iterations reached")
}
```

## Integration Points

### 1. Implement Command (Phase 7)

The implement command will use final verification:

```go
// After all tasks complete
result := finalVerifier.Verify(ctx, input)

if result.AllPassed {
    return success
}

// Generate gap-fix tasks and retry
analysis := gapAnalyzer.AnalyzeGaps(ctx, gapInput)
newTasks := createTasks(analysis.SuggestedTasks)
orchestrator.Execute(ctx, newTasks)
```

### 2. Orchestrator

The orchestrator should expose verification results:

```go
type OrchestratorResult struct {
    Success            bool
    TasksCompleted     int
    FinalVerification  *finalverify.VerificationResult
}
```

### 3. TUI

Display verification progress in TUI:

```
=== Final Verification ===
Layer 1 (Architecture Audit): ✓ PASS [12.3s]
Layer 2 (Build + Tests): ✓ PASS [45.1s]
Layer 3 (Comprehensive Review): ✓ PASS [23.7s]

✓ All verification layers passed!
Implementation complete: 100%
```

## Difference from Task Validation

| Aspect | Task Validation | Final Verification |
|--------|----------------|-------------------|
| **Package** | internal/validation | internal/finalverify |
| **When** | Before merging each task | After all tasks complete |
| **Scope** | Individual task | Entire implementation |
| **Layers** | 4 (contracts, build, semantic, review) | 3 (audit, build, comprehensive review) |
| **Purpose** | Prevent bad code from merging | Ensure complete system is correct |
| **Feedback** | Per-task fixes | System-level gap analysis |
| **Retry** | 3 attempts then escalate | Iteration loop with gap-fix tasks |

**Both are necessary**:
- Task validation ensures quality at merge time
- Final verification ensures completeness at the end

## Example Flow

### Scenario: Implementing Authentication System

**Initial Implementation**:
```
Tasks completed: 5/5
- Add JWT authentication
- Add login endpoint
- Add token validation middleware
- Add user model
- Add password hashing
```

**Final Verification - Iteration 1**:
```
Layer 1 (Architecture Audit): ✗ FAIL
  Features: 3/5 complete
  Gaps:
  - [PARTIAL] authentication: Token refresh not implemented
  - [MISSING] logout: Logout endpoint not found

Completion: 60%
```

**Gap Analysis**:
```
Generated 2 tasks:
1. [high] Implement token refresh endpoint
   - Add /auth/refresh endpoint
   - Validate refresh tokens
   - Return new access token

2. [high] Implement logout endpoint
   - Add /auth/logout endpoint
   - Invalidate tokens
   - Clear session
```

**Iteration 2 - Execute Gap Tasks**:
```
Executing 2 gap-fix tasks...
Task 1: Implement token refresh ✓
Task 2: Implement logout ✓
```

**Final Verification - Iteration 2**:
```
Layer 1 (Architecture Audit): ✓ PASS [15.2s]
  Features: 5/5 complete

Layer 2 (Build + Tests): ✓ PASS [32.1s]
  Build successful, 47 tests passed

Layer 3 (Comprehensive Review): ✓ PASS [18.9s]
  Implementation completely fulfills specification

✓ All verification layers passed!
Total Duration: 66.2s
```

**Result**: Implementation complete!

## Response Formats

### Layer 1: Architecture Audit

```
Features: 8/10 complete

Complete (8):
- authentication: Fully implemented with JWT
- user-management: CRUD operations working
- [... 6 more]

Gaps (2):
1. [PARTIAL] admin-panel: Missing user management features
2. [MISSING] logging: No logging implementation found
```

### Layer 3: Comprehensive Review

```
VERDICT: FAIL
REASONING: Implementation is mostly complete but missing critical
    error handling in payment flow and admin features are incomplete.
GAPS: Payment error handling; Admin user management; Audit logging
RECOMMENDATIONS: Add comprehensive error handling; Complete admin features;
    Add audit trail
```

### Gap Analysis

```
OVERALL_PRIORITY: high
ANALYSIS: Two critical gaps require immediate attention. Payment error
    handling is critical for production. Admin features can be addressed
    in a second task.

TASKS:
---
TITLE: Add payment error handling
FEATURE_ID: payments
PRIORITY: critical
DESCRIPTION: Implement comprehensive error handling for payment flow...
REASON: Required for production safety
---
TITLE: Complete admin user management
FEATURE_ID: admin-panel
PRIORITY: high
DESCRIPTION: Implement missing admin CRUD operations...
REASON: Feature marked as PARTIAL in audit
---
```

## Performance

Expected duration for final verification:
- **Layer 1**: 10-30 seconds (architecture audit)
- **Layer 2**: 10-60 seconds (build + full test suite)
- **Layer 3**: 10-30 seconds (comprehensive review)
- **Total**: 30-120 seconds

Gap analysis adds: 15-30 seconds

This is acceptable overhead before declaring implementation complete.

## Testing

Each component can be tested independently:

```go
// Mock auditor
type mockAuditor struct{ report *architect.GapReport }
func (m *mockAuditor) Audit(...) (*architect.GapReport, error) {
    return m.report, nil
}

// Mock build tester
type mockBuildTester struct{ shouldPass bool }
func (m *mockBuildTester) RunBuildAndTests(...) (bool, string, error) {
    return m.shouldPass, "tests passed", nil
}

// Test verifier
func TestFinalVerifier(t *testing.T) {
    auditor := &mockAuditor{report: allCompleteReport}
    buildTester := &mockBuildTester{shouldPass: true}
    verifier := finalverify.NewFinalVerifier(auditor, buildTester, nil)

    result, err := verifier.Verify(ctx, input)
    assert.NoError(t, err)
    assert.True(t, result.AllPassed)
}
```

## Current Status

✅ **Complete**: All 3 layers implemented
✅ **Complete**: Gap analyzer implemented
✅ **Complete**: Comprehensive documentation
✅ **Complete**: Integration examples

🚧 **Needs Integration**:
- Wire into cmd/alphie/implement.go (Phase 7)
- Add TUI display for verification results (Phase 8)
- End-to-end testing (Phase 11)

## Files

- `verifier.go` (490 lines) - Main orchestrator for 3-layer verification
- `gap_analyzer.go` (240 lines) - Gap analysis and task generation
- `doc.go` (180 lines) - Package documentation
- `README.md` (this file) - User guide

**Total**: ~910 lines of final verification infrastructure

## Future Enhancements

- [ ] Incremental verification (only verify changed features)
- [ ] Confidence scoring (skip review if very confident)
- [ ] Verification result caching
- [ ] Parallel layer execution
- [ ] Learning integration (learn from common gaps)
- [ ] Custom verification layers for specific needs

## Contributing

When adding features:
1. Maintain the 3-layer structure
2. Keep strict validation for Layer 1
3. Update gap analysis to handle new failure modes
4. Add integration examples
5. Update documentation

## License

Same as parent project (Alphie).
