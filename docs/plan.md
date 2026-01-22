# Alphie Governance & Ralph-Loop Integration Plan

## Executive Summary

**Problem**: Alphie's quality control is theater. Acceptance criteria are generated but not verified. Ralph-loop exists but is dead code. Quality gates run but don't block. The system *looks* trustworthy but isn't.

**Solution**: Wire up the existing Ralph-loop, add verifiable contracts, integrate verification into the loop, and define clear failure contracts.

---

## Current State (Brutal Truth)

| Component | Status | Reality |
|-----------|--------|---------|
| Ralph-Loop code | EXISTS | Never called - dead code in `ralph_loop.go` |
| Acceptance criteria | Generated | Stored in task, never checked |
| Quality gates | Run | Don't block anything, just report |
| Self-critique | Implemented | Regex parsing of Claude self-assessment |
| Learning capture | Partial | `OnTaskComplete` is a 3-line stub |

---

## Design Decisions (From Discussion)

1. **Verification depth**: Rich contracts - commands, output parsing, file constraints
2. **Loop design**: Both verification gates AND self-critique scoring
3. **Verification timing**: Hybrid - intent during decomposition, concrete post-implementation
4. **Failure contract**: Abort cleanly - nothing merged, clear error, user decides
5. **Passthrough mode**: Add for debugging, cost control, trusted simple tasks

---

## Implementation Plan

### Phase 1: Wire Up Ralph-Loop (The Quick Win)

**Goal**: Make the existing Ralph-loop code actually run.

**Files to modify**:
- `internal/agent/executor.go`

**Changes**:

1. In `ExecuteWithOptions()`, after initial Claude execution (~line 280):
```go
// After: result.Output = output
if opts.EnableRalphLoop && e.shouldRunRalphLoop(tier) {
    loop := NewRalphLoop(RalphLoopConfig{
        Tier:        tier,
        Claude:      claudeProcess,
        Learnings:   opts.Learnings,
        MaxIters:    e.maxIterationsForTier(tier),
        Threshold:   e.thresholdForTier(tier),
    })

    loopResult, err := loop.Run(ctx, taskPrompt, result.Output)
    if err != nil {
        // Log but don't fail - loop is enhancement
        log.Printf("[executor] ralph-loop error: %v", err)
    } else {
        result.RalphLoopIterations = loopResult.Iterations
        result.RalphLoopExitReason = loopResult.ExitReason
        result.Output = loopResult.FinalOutput
        result.RubricScore = loopResult.FinalScore
    }
}
```

2. Add helper methods:
```go
func (e *Executor) shouldRunRalphLoop(tier models.Tier) bool {
    return tier >= models.TierBuilder // Skip for Quick/Scout
}

func (e *Executor) maxIterationsForTier(tier models.Tier) int {
    switch tier {
    case models.TierBuilder: return 3
    case models.TierArchitect: return 5
    default: return 0
    }
}

func (e *Executor) thresholdForTier(tier models.Tier) int {
    switch tier {
    case models.TierBuilder: return 7
    case models.TierArchitect: return 8
    default: return 5
    }
}
```

**Estimated effort**: ~50 lines of code, 1-2 hours

---

### Phase 2: Verification Contract Schema

**Goal**: Define the contract structure that decomposer will generate.

**New file**: `internal/orchestrator/contract.go`

```go
// VerificationContract defines how to verify task completion
type VerificationContract struct {
    // Intent is human-readable acceptance criteria (from decomposition)
    Intent string `json:"intent"`

    // Commands are concrete verification steps (generated post-implementation)
    Commands []VerificationCommand `json:"commands,omitempty"`

    // FileConstraints define what must/must-not exist or change
    FileConstraints FileConstraints `json:"file_constraints,omitempty"`
}

type VerificationCommand struct {
    Command     string `json:"cmd"`           // e.g., "npm test -- --grep login"
    Expect      string `json:"expect"`        // "exit 0" or "output contains 'passed'"
    Description string `json:"description"`   // Human-readable purpose
    Required    bool   `json:"required"`      // Soft vs hard failure
}

type FileConstraints struct {
    MustExist      []string `json:"must_exist,omitempty"`
    MustNotExist   []string `json:"must_not_exist,omitempty"`
    MustNotChange  []string `json:"must_not_change,omitempty"`
}
```

**Update decomposer prompt** in `internal/orchestrator/decomposer.go`:

Add to task schema:
```json
{
  "verification_intent": "User can authenticate with valid credentials and receive a session token",
  "file_constraints": {
    "must_exist": ["src/routes/auth.ts"],
    "must_not_change": ["src/routes/admin.ts"]
  }
}
```

**Estimated effort**: ~100 lines, 2-3 hours

---

### Phase 3: Post-Implementation Verification Generation

**Goal**: Generate concrete verification commands after code is written.

**New file**: `internal/orchestrator/verify_gen.go`

```go
// GenerateVerification creates concrete verification from intent + actual code
func GenerateVerification(ctx context.Context, claude *agent.ClaudeProcess,
    intent string, fileConstraints FileConstraints, writtenFiles []string) (*VerificationContract, error) {

    prompt := fmt.Sprintf(`Given this task intent and the files that were created/modified,
generate concrete verification commands.

Intent: %s

Files created/modified:
%s

Output JSON with verification commands that can be run to verify the intent was achieved.
Consider: existing test files, API endpoints created, CLI commands available.

Schema:
{
  "commands": [
    {"cmd": "command to run", "expect": "exit 0 OR output contains X", "required": true/false}
  ]
}`, intent, strings.Join(writtenFiles, "\n"))

    // Call Claude, parse response
    // ...
}
```

**Integration point**: Call this in `executor.go` after initial implementation, before Ralph-loop critique.

**Estimated effort**: ~150 lines, 3-4 hours

---

### Phase 4: Integrate Verification into Ralph-Loop

**Goal**: Make the loop use verification results, not just self-assessment.

**Modify**: `internal/agent/ralph_loop.go`

Current flow:
```
implement → self-critique → if below threshold: improve → repeat
```

New flow:
```
implement → generate verification → run verification
  → if verification PASS AND score >= threshold: EXIT SUCCESS
  → if verification FAIL: inject failure context → improve → repeat
  → if score < threshold but verification PASS: one more iteration
  → if max iterations: EXIT with status
```

**Key change in `Run()`**:
```go
func (r *RalphLoop) Run(ctx context.Context, task string, initialOutput string) (*RalphLoopResult, error) {
    output := initialOutput

    for iteration := 0; iteration < r.maxIterations; iteration++ {
        // Run verification commands
        verifyResult := r.runVerification(ctx)

        // Get self-critique score
        score, improvements := r.getCritique(ctx, output)

        // Decision matrix
        if verifyResult.AllPassed && score.Total() >= r.threshold {
            return &RalphLoopResult{
                ExitReason: "verification_passed_and_threshold_met",
                // ...
            }, nil
        }

        if verifyResult.AllPassed && score.Total() >= r.threshold - 1 {
            // Close enough with passing tests - accept
            return &RalphLoopResult{
                ExitReason: "verification_passed_score_acceptable",
                // ...
            }, nil
        }

        // Need improvement - inject context
        improvePrompt := r.buildImprovePrompt(output, verifyResult, improvements)
        output, err = r.executeImprovement(ctx, improvePrompt)
        if err != nil {
            return nil, fmt.Errorf("improvement iteration %d: %w", iteration, err)
        }
    }

    return &RalphLoopResult{
        ExitReason: "max_iterations_reached",
        // ...
    }, nil
}
```

**Estimated effort**: ~200 lines of changes, 4-6 hours

---

### Phase 5: Add Passthrough Mode

**Goal**: Simple "just run Claude" mode for debugging/cost control.

**Modify**: `cmd/alphie/run.go`

Add flag:
```go
runCmd.Flags().BoolVar(&passthrough, "passthrough", false, "Skip orchestration, run Claude directly")
```

Implementation:
```go
if passthrough {
    result, err := executor.ExecuteDirect(ctx, task, ExecuteDirectOptions{
        WorktreeIsolation: !noWorktree,
        InjectLearnings:   true,
        TrackCost:         true,
    })
    // Print result, exit
    return
}
// ... existing orchestration path
```

**New method in executor**:
```go
func (e *Executor) ExecuteDirect(ctx context.Context, task string, opts ExecuteDirectOptions) (*DirectResult, error) {
    // Create worktree if requested
    // Inject learnings if requested
    // Run Claude once
    // Track cost
    // Return result
}
```

**Estimated effort**: ~100 lines, 2-3 hours

---

### Phase 6: Clean Abort on Failure

**Goal**: When verification fails after max iterations, abort cleanly with useful error.

**Modify**: `internal/orchestrator/orchestrator.go`

In task completion handler:
```go
if !result.Success || result.RalphLoopExitReason == "max_iterations_reached" {
    // Don't merge this task's worktree
    // Emit detailed failure event
    o.emitEvent(OrchestratorEvent{
        Type:    EventTaskFailed,
        TaskID:  task.ID,
        Error:   buildFailureReport(result),
        Message: fmt.Sprintf("Task failed after %d iterations. Verification: %s. Score: %d/%d",
            result.RalphLoopIterations,
            result.VerificationSummary,
            result.RubricScore.Total(),
            9),
    })

    // Clean up worktree without merging
    // ...

    return // Don't proceed with merge
}
```

**Update TUI** to show failure details clearly.

**Estimated effort**: ~50 lines, 1-2 hours

---

## Files to Modify Summary

| File | Changes |
|------|---------|
| `internal/agent/executor.go` | Wire up Ralph-loop, add passthrough mode |
| `internal/agent/ralph_loop.go` | Integrate verification into loop |
| `internal/orchestrator/contract.go` | NEW: Verification contract schema |
| `internal/orchestrator/verify_gen.go` | NEW: Post-implementation verification generation |
| `internal/orchestrator/decomposer.go` | Add verification intent to task schema |
| `internal/orchestrator/orchestrator.go` | Clean abort handling |
| `cmd/alphie/run.go` | Add --passthrough flag |
| `internal/tui/panel_app.go` | Display failure details |

---

## Verification (How to Test)

### Phase 1 (Ralph-Loop)
```bash
# Run a builder-tier task and check logs for ralph-loop iterations
alphie run "Add a hello world endpoint to the API" --tier builder
# Check: RalphLoopIterations > 0 in output/logs
```

### Phase 2-4 (Verification)
```bash
# Run a task and observe verification commands being generated
alphie run "Add user login with email/password" --tier builder
# Check: Logs show verification commands running
# Check: Loop iterates when verification fails
```

### Phase 5 (Passthrough)
```bash
# Compare passthrough vs normal
alphie run "Fix typo in README" --passthrough
# Check: No decomposition, single Claude call, fast completion
```

### Phase 6 (Abort)
```bash
# Trigger intentional failure
alphie run "Implement impossible feature XYZ" --tier builder
# Check: Clean abort message, no partial merge, clear error
```

---

## Future Work

### 1. Learning System Integration
- `OnTaskComplete` is currently a 3-line stub that returns nil
- Need to capture learnings from successful executions
- Store failure patterns for future task guidance
- Track learning efficacy (did this learning actually help?)

### 2. Multi-Model Orchestration (N-Many SDK)
- Support multiple backends: Claude CLI, Claude API, OpenAI API
- Different models for different phases: GPT for decomposition, Claude for execution
- Local models for critique/scoring to reduce cost
- Ensemble voting for high-risk decisions

### 3. Stacked PRs
- Session branches as PR chains for incremental review
- Each decomposed task as a reviewable PR
- Auto-merge on approval, auto-rebase on conflicts
- Integration with GitHub stacked PR tooling

### 4. Human-in-the-Loop Gates
- Mid-execution approval for high-risk changes
- Review points between decomposition and execution
- Merge approval UI in TUI
- Configurable approval requirements per tier

### 5. Integrator Role
- Post-parallel normalization agent
- Detects style inconsistencies (camelCase vs snake_case)
- Removes duplicate helper functions
- Updates shared documentation
- Produces coherent changelog

### 6. Distributed Prog
- Cross-machine task coordination
- Shared learning database
- Distributed worktree management
- Remote agent execution

### 7. Adaptive Thresholds
- Learn optimal thresholds from project history
- Adjust quality gates based on code area (stricter for auth)
- Track success rates by tier and adjust recommendations

### 8. Observability & Metrics
- Quality trends over time
- Success rates by tier
- Cost per task type
- Learning efficacy tracking
- Failure pattern analysis

---

## Success Criteria

After implementation:
- [ ] Ralph-loop actually runs (RalphLoopIterations > 0 in results)
- [ ] Verification commands are generated and executed
- [ ] Loop iterates when verification fails
- [ ] Clean abort when max iterations reached
- [ ] Passthrough mode works for simple tasks
- [ ] README accurately reflects what alphie does

---

## Architecture Notes

### Why This Design

**Hybrid Verification (Intent + Concrete)**
- Intent during decomposition catches "what should happen" before code exists
- Concrete commands post-implementation catch "did it actually work"
- Neither alone is sufficient: intent-only is unverifiable, concrete-only might verify wrong behavior

**Both Self-Critique AND Verification**
- Verification grounds the loop in reality (tests pass/fail)
- Self-critique provides improvement direction (what to fix)
- Tests passing ≠ good code (could be spaghetti that works)
- Self-assessment alone ≠ correct code (Claude might not notice bugs)

**Clean Abort (Not Partial Merge)**
- Partial merges create inconsistent state that's hard to recover from
- User can always retry with more context
- Failed task stays in worktree for manual inspection
- Clear contract: success = merged, failure = nothing merged

**Passthrough Mode**
- Not everything needs orchestration
- Debugging alphie itself requires bypassing orchestration
- Cost control for simple tasks
- Trust the user to know when to use it
