# Phase 1 Cleanup Status: Remove internal/learning, internal/state, internal/prog References

## Overview

This document tracks the progress of removing unwanted feature packages (internal/learning, internal/state, internal/prog, pkg/models/tier) from the Alphie codebase as part of Phase 1 simplification.

## Completion Status: ~20% Complete

### ✅ Fully Completed Files

1. **/Users/shaycichocki/Alphie/internal/orchestrator/options.go**
   - ✅ Removed imports: learning, state, prog, config, models
   - ✅ Removed Tier field from RequiredConfig
   - ✅ Removed from orchestratorOptions: tierConfigs, stateDB, learningSystem, progClient, resumeEpicID
   - ✅ Removed With functions: WithTierConfigs, WithStateDB, WithLearningSystem, WithProgClient, WithResumeEpicID
   - ✅ Updated toOrchestratorConfig to remove all references

2. **/Users/shaycichocki/Alphie/internal/orchestrator/pool.go**
   - ✅ Removed imports: config, learning, state, prog, models
   - ✅ Removed from PoolConfig: TierConfigs, StateDB, LearningSystem, ProgClient
   - ✅ Updated Submit/SubmitWithID methods signature (removed tier parameter)
   - ✅ Cleaned up orchestrator instantiation

### 🔄 Partially Completed Files

3. **/Users/shaycichocki/Alphie/internal/orchestrator/orchestrator.go** - 40% done
   - ✅ Removed imports: learning, state, prog
   - ✅ Removed from OrchestratorConfig: Tier, TierConfigs, StateDB, LearningSystem, ProgClient, ResumeEpicID
   - ✅ Cleaned up OrchestratorConfig struct
   - ❌ Need to remove from Orchestrator struct: learnings, progCoord, learningCoord, effectivenessTracker, stateDB
   - ❌ Need to update NewOrchestrator function (remove prog/learning coordinator init)
   - ❌ Need to remove GetProgClient method
   - ❌ Need to update QuestionsAllowedForTask

4. **/Users/shaycichocki/Alphie/internal/agent/executor.go** - 10% done
   - ✅ Removed learning import
   - ✅ Removed SuggestedLearnings, LearningsUsed fields from ExecutionResult
   - ❌ Need to remove failureAnalyzer field and logic
   - ❌ Need to remove Learnings field from ExecuteOptions
   - ❌ Need to remove all learning retrieval/injection logic

## Pending Files (Not Started)

### High Priority - Core Orchestrator Files

5. **/Users/shaycichocki/Alphie/internal/orchestrator/run_config.go**
   - Remove Tier field
   - Update NewRunConfig signature

6. **/Users/shaycichocki/Alphie/internal/orchestrator/run_loop.go**
   - Remove learning import
   - Remove all progCoord calls
   - Remove learnings retrieval logic
   - Remove Tier usage
   - Simplify protected area detection

7. **/Users/shaycichocki/Alphie/internal/orchestrator/task_completion.go**
   - Remove learning import
   - Remove all progCoord calls (multiple locations)
   - Remove learningCoord.CaptureOnCompletion
   - Remove recordTaskOutcome method
   - Remove learnings.OnFailure logic

8. **/Users/shaycichocki/Alphie/internal/orchestrator/agent_spawner.go**
   - Remove learning import
   - Remove Learnings from SpawnOptions
   - Update Spawn method

9. **/Users/shaycichocki/Alphie/internal/orchestrator/orchestrator_lifecycle.go**
   - Remove state import
   - Remove all stateDB method calls
   - Remove state persistence logic entirely

### Tier System Files - To Be Deleted

10. **/Users/shaycichocki/Alphie/internal/orchestrator/tier_selector.go** - DELETE
11. **/Users/shaycichocki/Alphie/internal/orchestrator/tier_selector_test.go** - DELETE
12. **/Users/shaycichocki/Alphie/internal/orchestrator/tier_keywords.go** - DELETE
13. **/Users/shaycichocki/Alphie/internal/tui/tier_classifier.go** - DELETE
14. **/Users/shaycichocki/Alphie/internal/tui/tier_classifier_test.go** - DELETE

### Tier System Files - To Be Cleaned

15. **/Users/shaycichocki/Alphie/internal/orchestrator/scheduler.go**
    - Remove tier-based logic
    - Simplify to single agent type

16. **/Users/shaycichocki/Alphie/internal/orchestrator/scheduler_test.go**
    - Update tests for simplified scheduler

17. **/Users/shaycichocki/Alphie/internal/agent/model_selector.go**
    - Simplify model selection (no tier-based selection)

18. **/Users/shaycichocki/Alphie/internal/agent/executor_prompt.go**
    - Remove tier from prompts

19. **/Users/shaycichocki/Alphie/internal/agent/executor_gates.go**
    - Remove tier-specific quality gates

20. **/Users/shaycichocki/Alphie/internal/agent/ralph_loop.go**
    - Remove tier references

### Command/Init Files

21. **/Users/shaycichocki/Alphie/cmd/alphie/init.go**
    - Remove learning and state imports
    - Remove database initialization

22. **/Users/shaycichocki/Alphie/cmd/alphie/cleanup.go** (if exists)
    - Review and update

### TUI Files

23. **/Users/shaycichocki/Alphie/internal/tui/app.go**
    - Remove tier selection
    - Simplify agent configuration

24. **/Users/shaycichocki/Alphie/internal/tui/interactive_app.go**
    - Remove tier UI elements

25. **/Users/shaycichocki/Alphie/internal/tui/review.go**
    - Remove tier references

### Test Files

26-30. Various test files with models.Tier references
    - Need to update all test instantiations
    - Remove tier parameters from test cases

## Decision Required: Tier Replacement Strategy

The tier system (Scout/Builder/Architect) is deeply integrated. We need to choose a replacement:

### Option A: Single Fixed Behavior (RECOMMENDED)
- All agents behave identically
- Maximum simplification
- No user configuration needed
- Easiest to maintain

### Option B: Complexity Flag
- Simple binary: simple/complex tasks
- Minimal configuration
- Less dramatic change

### Option C: Model-Based
- Different Claude models (Haiku/Sonnet/Opus)
- Same behavior, different performance
- User still needs to specify model

**Recommendation:** Option A - Complete removal of tier concept. All agents use same behavior with best-available Claude model.

## Current Blockers

1. **Compilation Errors:** Code won't compile until more files are cleaned
2. **Tier System:** Need decision on replacement strategy before proceeding
3. **State Persistence:** Need to decide if any session state should be kept (recommend: no, keep it stateless)
4. **Prog Integration:** All cross-session tracking being removed - confirm this is desired

## Next Steps (Prioritized)

1. ✅ **DONE:** Document current status (this file)
2. 🔄 **IN PROGRESS:** Decide on tier replacement strategy
3. ⏳ **NEXT:** Complete orchestrator.go cleanup
4. ⏳ Finish agent/executor.go cleanup
5. ⏳ Clean up run_loop.go and task_completion.go
6. ⏳ Remove all state persistence (orchestrator_lifecycle.go)
7. ⏳ Delete tier system files
8. ⏳ Update all tier references to fixed behavior
9. ⏳ Update cmd/alphie/init.go
10. ⏳ Update TUI files
11. ⏳ Run full build and fix all compilation errors
12. ⏳ Update/fix tests
13. ⏳ Final verification and testing

## Estimated Effort

- **Completed:** ~20% (4 files fully done, 2 partially done)
- **Remaining:** ~80%
- **Files to modify:** ~20 files
- **Files to delete:** ~5 files
- **Estimated time:** 4-6 hours of focused work
- **Risk level:** Medium (many interdependencies, compilation will be broken during cleanup)

## Testing Strategy

After cleanup:
1. Ensure code compiles without errors
2. Run all unit tests
3. Test orchestration end-to-end with simple task
4. Test merge conflict handling
5. Test verification system
6. Test TUI interaction

---

**Last Updated:** 2026-01-22
**Status:** 🔄 In Progress (20% complete)
