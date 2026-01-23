# Phase 1 Cleanup Checklist

Quick reference for completing the cleanup of internal/learning, internal/state, internal/prog, and pkg/models/tier references.

## Quick Command Reference

```bash
# Check for remaining references
grep -r "internal/learning" --include="*.go" internal/ cmd/
grep -r "internal/state" --include="*.go" internal/ cmd/
grep -r "internal/prog" --include="*.go" internal/ cmd/
grep -r "models\.Tier" --include="*.go" internal/ cmd/

# Count remaining references
grep -r "internal/learning" --include="*.go" internal/ cmd/ | wc -l
grep -r "internal/state" --include="*.go" internal/ cmd/ | wc -l
grep -r "internal/prog" --include="*.go" internal/ cmd/ | wc -l
grep -r "models\.Tier" --include="*.go" internal/ cmd/ | wc -l

# Try to build (will show what still needs fixing)
go build ./...
```

## File-by-File Checklist

### Core Orchestrator Files

- [x] internal/orchestrator/options.go - **COMPLETE**
- [x] internal/orchestrator/pool.go - **COMPLETE**
- [ ] internal/orchestrator/orchestrator.go - **60% COMPLETE**
  - [x] Imports cleaned
  - [x] OrchestratorConfig struct cleaned
  - [ ] Orchestrator struct needs cleaning (remove: learnings, progCoord, learningCoord, effectivenessTracker, stateDB)
  - [ ] NewOrchestrator needs cleaning (remove all prog/learning init)
  - [ ] Remove GetProgClient method
  - [ ] Update QuestionsAllowedForTask

- [ ] internal/orchestrator/run_config.go
  - [ ] Remove Tier field from OrchestratorRunConfig
  - [ ] Update NewRunConfig signature

- [ ] internal/orchestrator/run_loop.go
  - [ ] Remove learning import
  - [ ] Remove progCoord.StartTask call
  - [ ] Remove progCoord.BlockTask, LogTask calls
  - [ ] Remove learnings retrieval logic (lines ~159-169)
  - [ ] Remove Tier from spawner call
  - [ ] Simplify protected area check (remove Tier conditional)

- [ ] internal/orchestrator/task_completion.go
  - [ ] Remove learning import
  - [ ] Remove progCoord.BlockTask, LogTask, CompleteTask calls (many locations)
  - [ ] Remove learningCoord.CaptureOnCompletion
  - [ ] Remove recordTaskOutcome method entirely
  - [ ] Remove learnings.OnFailure call

- [ ] internal/orchestrator/agent_spawner.go
  - [ ] Remove learning import
  - [ ] Remove Learnings field from SpawnOptions struct
  - [ ] Remove learnings from Spawn method

- [ ] internal/orchestrator/orchestrator_lifecycle.go
  - [ ] Remove state import
  - [ ] Delete or gut all state persistence methods:
    - createSessionState, updateSessionState
    - createTaskState, updateTaskState
    - createAgentState, updateAgentState
  - [ ] Remove all o.stateDB calls

### Agent Files

- [ ] internal/agent/executor.go - **10% COMPLETE**
  - [x] Remove learning import
  - [x] Remove SuggestedLearnings, LearningsUsed from ExecutionResult
  - [ ] Remove failureAnalyzer field from Executor
  - [ ] Remove FailureAnalyzer from ExecutorConfig
  - [ ] Remove failureAnalyzer init in NewExecutor
  - [ ] Remove Learnings from ExecuteOptions
  - [ ] Remove learning injection logic in ExecuteWithOptions

- [ ] internal/agent/model_selector.go
  - [ ] Remove tier-based model selection
  - [ ] Simplify to single model or user-specified model

- [ ] internal/agent/executor_prompt.go
  - [ ] Remove tier from prompt generation

- [ ] internal/agent/executor_gates.go
  - [ ] Remove tier-specific gate logic

- [ ] internal/agent/ralph_loop.go
  - [ ] Remove tier references

### Tier System - DELETE THESE FILES

- [ ] internal/orchestrator/tier_selector.go - **DELETE**
- [ ] internal/orchestrator/tier_selector_test.go - **DELETE**
- [ ] internal/orchestrator/tier_keywords.go - **DELETE**
- [ ] internal/tui/tier_classifier.go - **DELETE**
- [ ] internal/tui/tier_classifier_test.go - **DELETE**

### Tier System - CLEAN THESE FILES

- [ ] internal/orchestrator/scheduler.go
  - [ ] Remove tier logic from scheduling decisions

- [ ] internal/orchestrator/scheduler_test.go
  - [ ] Update tests for simplified scheduler

### TUI Files

- [ ] internal/tui/app.go
  - [ ] Remove tier selection UI

- [ ] internal/tui/interactive_app.go
  - [ ] Remove tier configuration UI

- [ ] internal/tui/review.go
  - [ ] Remove tier references

- [ ] internal/tui/input_field.go (if has tier refs)
  - [ ] Check and clean

### Command Files

- [ ] cmd/alphie/init.go
  - [ ] Remove learning import
  - [ ] Remove state import
  - [ ] Remove database initialization
  - [ ] Simplify .alphie directory setup

- [ ] cmd/alphie/cleanup.go
  - [ ] Review and update if exists

### Test Files

- [ ] internal/orchestrator/orchestrator_test.go
  - [ ] Remove tier from test cases
  - [ ] Update mocks

- [ ] internal/orchestrator/pool_test.go
  - [ ] Remove tier from test cases

- [ ] internal/orchestrator/scheduler_test.go
  - [ ] Update for simplified scheduler

- [ ] internal/orchestrator/override_test.go
  - [ ] Remove tier references

- [ ] internal/agent/executor_test.go
  - [ ] Remove learning from tests

- [ ] internal/agent/model_selector_test.go
  - [ ] Update for simplified model selection

- [ ] internal/agent/ralph_test.go
  - [ ] Remove tier references

- [ ] internal/agent/iteration_test.go
  - [ ] Check for tier/learning references

- [ ] internal/agent/prompts_test.go
  - [ ] Remove tier from prompt tests

- [ ] internal/integration/orchestration_test.go
  - [ ] Update integration tests

- [ ] internal/tui/*_test.go files
  - [ ] Update all TUI tests

### Architecture/Documentation Files

- [ ] docs/architecture.md
  - [ ] Update to reflect removed systems

- [ ] docs/review.md
  - [ ] Update to reflect simplified design

- [ ] docs/plan.md
  - [ ] Update roadmap

## Verification Steps

After completing each file:

1. ✅ Check that file compiles: `go build <file_path>`
2. ✅ Check for lingering imports: `grep -E "learning|state|prog" <file_path>`
3. ✅ Check for Tier references: `grep "models\.Tier\|Tier:" <file_path>`
4. ✅ Run related tests: `go test <package_path>`

After completing all files:

1. ✅ Full build: `go build ./...`
2. ✅ All tests: `go test ./...`
3. ✅ Grep for any remaining references:
   ```bash
   grep -r "internal/learning" --include="*.go" .
   grep -r "internal/state" --include="*.go" .
   grep -r "internal/prog" --include="*.go" .
   grep -r "models\.Tier" --include="*.go" .
   ```
4. ✅ Manual test: Run alphie on a simple task
5. ✅ Check logs for any learning/state/prog references

## Common Patterns to Remove

### Pattern 1: ProgCoordinator calls
```go
// REMOVE THESE:
o.progCoord.StartTask(taskID)
o.progCoord.CompleteTask(taskID)
o.progCoord.BlockTask(taskID, msg)
o.progCoord.LogTask(taskID, msg)
```

### Pattern 2: Learning retrieval
```go
// REMOVE THIS:
var taskLearnings []*learning.Learning
if o.learnings != nil {
    learnings, err := o.learnings.OnTaskStart(task.Description, nil)
    // ...
}
```

### Pattern 3: State persistence
```go
// REMOVE THESE:
o.stateDB.CreateSession(...)
o.stateDB.UpdateTask(...)
o.createAgentState(...)
o.updateTaskState(...)
```

### Pattern 4: Tier-based logic
```go
// REMOVE THIS:
if o.config.Tier == models.TierScout {
    // Scout-specific logic
}

// REPLACE WITH:
// Fixed behavior for all agents
```

### Pattern 5: Tier selection
```go
// REMOVE THIS:
tier := selectTierForTask(description)

// REPLACE WITH:
// No tier selection needed
```

## Current Status Summary

- **Completed:** 4 files (100%)
- **In Progress:** 2 files (partial)
- **Not Started:** ~20 files
- **To Delete:** 5 files
- **Overall:** ~20% complete

## Time Estimates

- **Core orchestrator files:** ~2-3 hours
- **Agent files:** ~1-2 hours
- **Tier system removal:** ~1-2 hours
- **TUI files:** ~1 hour
- **Tests:** ~1-2 hours
- **Verification:** ~1 hour
- **Total:** ~4-6 hours

---

**Last Updated:** 2026-01-22
