# Alphie Simplification Progress

## Overview
This document tracks progress on the comprehensive Alphie simplification project, transforming Alphie from a complex interactive tool into a focused, spec-driven orchestrator.

## Goal
Transform Alphie into a single-purpose tool: **take a spec, decompose it into a DAG, orchestrate agents through it with rigorous validation at each node, handle merge conflicts intelligently, and iterate until the implementation exactly matches the spec.**

## Overall Progress: 18% (2/11 phases complete)

### ✅ Phase 1: Remove Unwanted Commands and Files (COMPLETE)
**Status**: Complete
**Files Changed**: ~60 files deleted, 40+ modified

**Removed**:
- Commands: `interactive`, `run`, `status`, `learn`, `baseline`, `config`
- Internal systems:
  - `internal/learning/` - CAO triples database (18 files)
  - `internal/state/` - SQLite session tracking (7 files)
  - `internal/prog/` - Cross-session task management (22 files)
- Tier system: `pkg/models/tier.go`, tier-specific logic across ~25 files
- Checkpoint/rollback system in orchestrator

**Updated**:
- `cmd/alphie/root.go` - Simplified to 5 commands only (version, implement, audit, init, cleanup)
- Removed all references to deleted packages across codebase
- Replaced `models.Tier` with `interface{}` throughout
- Removed tier-based logic, simplified to single model approach

**Result**: Project builds cleanly with zero compilation errors. All unwanted features removed.

### ✅ Phase 2: Simplify Configuration System (COMPLETE)
**Status**: Complete
**Files Changed**: ~10 files

**Changes**:
- Completely rewrote `internal/config/config.go` with minimal structure
- Deleted tier-specific YAML files: `configs/architect.yaml`, `configs/scout.yaml`, `configs/builder.yaml`
- New simplified config structure:
  ```yaml
  anthropic:
    api_key: ""       # or ANTHROPIC_API_KEY env var
    backend: "api"    # "api" or "bedrock"
  aws:
    region: "us-east-1"  # or AWS_REGION env var
  execution:
    model: "sonnet"   # sonnet, haiku, opus
    max_agents: 3
    max_retries: 3
  branch:
    greenfield: false
  ```
- Environment variables override config file values
- Only project-local `.alphie.yaml` (no global config)
- Created `.alphie.yaml.example` template

**Result**: Clean, minimal configuration with sensible defaults. All config references updated.

### 🔄 Phase 3: Implement 4-Layer Node Validation (PENDING)
**Status**: Not started
**Dependencies**: None
**Estimated Effort**: Medium (3-4 hours)

**Required Work**:
1. Create `internal/validation/semantic.go` - Semantic validation layer
2. Create `internal/validation/review.go` - Code review layer
3. Move build/test validation to pre-merge (from post-merge)
4. Integrate all 4 layers into `internal/agent/executor.go`
5. Add retry logic (max 3 attempts) with failure context injection
6. Store validation results in `ExecutionResult`

**4 Validation Layers**:
1. Verification contracts (exists, needs enhancement)
2. Build + test suite (exists, needs to move pre-merge)
3. **NEW**: Semantic validation (Claude reviews implementation vs intent)
4. **NEW**: Code review (detailed review against acceptance criteria)

### 🔄 Phase 4: Implement 3-Layer Final Verification (PENDING)
**Status**: Not started
**Dependencies**: None
**Estimated Effort**: Medium (2-3 hours)

**Required Work**:
1. Enhance `internal/architect/auditor.go` for strict validation (all features COMPLETE)
2. Create `internal/validation/final_review.go` - Comprehensive semantic review
3. Integrate into `cmd/alphie/implement.go`
4. On failure: identify gaps, generate new tasks, retry

**3 Verification Layers**:
1. Architecture audit (exists, needs strict validation enhancement)
2. Build + full test suite (exists)
3. **NEW**: Comprehensive semantic review (Claude reviews entire implementation vs spec)

### 🔄 Phase 5: Enhance Merge Conflict Handling (PENDING)
**Status**: Not started
**Dependencies**: None
**Estimated Effort**: Small (1-2 hours)

**Required Work**:
1. Update `internal/orchestrator/merge_resolver.go` - Change model to Opus
2. Create `internal/orchestrator/merge_context.go` - Aggregate task history/context
3. Enhanced merge prompt with full context
4. Extended iteration budget (5 retries instead of 3)
5. Orchestrator pause/resume on conflicts

### 🔄 Phase 6: Implement User Escalation System (PENDING)
**Status**: Not started
**Dependencies**: None
**Estimated Effort**: Medium (2-3 hours)

**Required Work**:
1. Create `internal/orchestrator/escalation.go`
2. Pause orchestrator on 3rd task failure
3. Display failure details via TUI
4. Wait for user input (Retry/Skip/Abort/Manual Fix)
5. Resume logic based on user choice
6. Integrate into orchestrator run loop

### 🔄 Phase 7: Update Implement Command (PENDING)
**Status**: Not started
**Dependencies**: Phases 3, 4, 5, 6
**Estimated Effort**: Medium (3-4 hours)

**Required Work**:
1. Simplify `cmd/alphie/implement.go`
2. Remove flags: `--agents`, `--max-iterations`, `--budget`, `--dry-run`, `--resume`, `--project`
3. Keep flags: `--greenfield`, `--cli`, `--help`
4. New execution loop: parse → decompose → orchestrate → verify → iterate until perfect
5. Wire in node validation (Phase 3), final verification (Phase 4), escalation (Phase 6)

### 🔄 Phase 8: Simplify TUI for Implement Mode Only (PENDING)
**Status**: Not started
**Dependencies**: Phase 6
**Estimated Effort**: Medium (2-3 hours)

**Required Work**:
1. Update `internal/tui/implement.go` (or create new)
2. Remove task input box (no interactive submission)
3. Keep progress display: phase, running agents, completed/pending tasks, errors
4. Add escalation display (pause state with options)
5. User input handling for escalation only

### 🔄 Phase 9: Update Branch Naming with Spec Name (PENDING)
**Status**: Not started
**Dependencies**: None
**Estimated Effort**: Small (1 hour)

**Required Work**:
1. Extract and sanitize spec name from file
2. Format: `alphie-{spec-name}-{timestamp}`
3. Update `internal/orchestrator/session_manager.go` or `internal/merge/branch.go`
4. Handle greenfield mode

### 🔄 Phase 10: Update Root Command and Help (PENDING)
**Status**: Partially complete (Phase 1)
**Dependencies**: None
**Estimated Effort**: Small (1 hour)

**Required Work**:
1. Update help text for all commands
2. Update documentation references
3. Verify all command descriptions match new model

### 🔄 Phase 11: End-to-End Testing and Validation (PENDING)
**Status**: Not started
**Dependencies**: Phases 7, 8, 9, 10
**Estimated Effort**: Large (4-6 hours)

**Required Work**:
1. Create test specs for:
   - Basic flow (2-3 features)
   - Merge conflict scenario
   - Escalation scenario
   - Final verification failure
2. Run manual end-to-end tests
3. Verify all success criteria
4. Document any issues
5. Create integration tests

## Success Criteria

- [x] ✅ Only 5 commands exist: version, implement, audit, init, cleanup
- [x] ✅ No interactive mode
- [x] ✅ No run command
- [x] ✅ No tier system
- [x] ✅ No learning/state/prog systems
- [x] ✅ Minimal config structure
- [x] ✅ Project builds successfully
- [ ] 4-layer node validation implemented
- [ ] 3-layer final verification implemented
- [ ] Intelligent merge conflict handling
- [ ] User escalation on failures
- [ ] Iterative refinement until perfect
- [ ] TUI visualization for implement
- [ ] Branch naming includes spec name
- [ ] Greenfield mode support
- [ ] Backend support (API, CLI, Bedrock)

## Files Modified

**Total**: 135 files changed
- **Deleted**: ~60 files (commands, internal systems, tier configs)
- **Modified**: ~75 files (imports, references, logic simplification)

**Key Files**:
- `cmd/alphie/root.go` - Simplified command structure
- `internal/config/config.go` - Completely rewritten
- `internal/orchestrator/orchestrator.go` - Removed prog/learning/state references
- `internal/agent/executor.go` - Removed learning integration
- 25+ files with tier system removed

## Next Steps

**Immediate (Phases 3-6)**: These can be done in parallel since they're independent:
1. **Phase 3**: Implement 4-layer node validation
2. **Phase 4**: Implement 3-layer final verification
3. **Phase 5**: Enhance merge conflict handling
4. **Phase 6**: Implement user escalation

**Then Sequential**:
5. **Phase 7**: Update implement command (depends on 3-6)
6. **Phase 8**: Simplify TUI (depends on 6)
7. **Phase 9**: Update branch naming (independent)
8. **Phase 10**: Update help text (independent)
9. **Phase 11**: End-to-end testing (depends on 7-10)

## Estimated Total Time Remaining

- Phases 3-6: ~10-12 hours (parallel work possible)
- Phases 7-10: ~7-9 hours
- Phase 11: ~4-6 hours
- **Total**: ~21-27 hours of focused work

## Notes

- The cleanup was more extensive than originally estimated due to tight coupling between removed systems
- All removed features were successfully extracted without breaking builds
- Configuration system is now extremely simple and maintainable
- The simplified model makes the remaining phases more straightforward
- Documentation created during cleanup will help with remaining phases
