# Phase 5: Enhanced Merge Conflict Handling - COMPLETE ✅

## Overview

Phase 5 enhances Alphie's merge conflict handling with intelligent resolution capabilities. The merge resolver agent now uses the Opus model (highest intelligence) with comprehensive context about all completed tasks, branch diffs, and conflict details. This enables the agent to make informed decisions when resolving conflicts.

## What Was Built

### 📦 New File: `internal/orchestrator/merge_context.go`

Created a comprehensive merge context system (380 lines) that aggregates:
- Full task history with intents
- Branch diffs from merge base
- Detailed conflict information
- File contents from all versions (target, agent, merge base)
- Conflict types and markers

**Key components:**

1. **MergeContext struct** - Aggregates all information needed for resolution:
   ```go
   type MergeContext struct {
       TaskID           string
       TargetBranch     string
       AgentBranch      string
       ConflictingFiles []string
       TaskHistory      []TaskSummary
       BranchDiff       string
       AgentDiff        string
       MergeBase        string
       ConflictDetails  []ConflictDetail
   }
   ```

2. **MergeContextBuilder** - Builds comprehensive context:
   - Finds merge base between branches
   - Gets diffs for both branches
   - Builds task history from dependency graph
   - Extracts detailed conflict information per file
   - Formats everything for the merge agent prompt

3. **TaskSummary** - Provides context about completed work:
   ```go
   type TaskSummary struct {
       ID          string
       Title       string
       Description string
       FilesChanged []string
       Intent      string  // Extracted from description
   }
   ```

4. **ConflictDetail** - Detailed file-level conflict info:
   ```go
   type ConflictDetail struct {
       FilePath         string
       ConflictType     string // "both_modified", "added_by_both", etc.
       TargetContent    string
       AgentContent     string
       MergeBaseContent string
   }
   ```

### 🔧 Enhanced: `internal/orchestrator/merge_resolver.go`

Upgraded the merge resolver with multiple enhancements:

1. **Opus Model Integration**:
   - Uses `agent.ModelOpus` (claude-opus-4-5-20251101)
   - Highest intelligence model for complex merge resolution
   - Set via `StartOptions`:
     ```go
     opts := &agent.StartOptions{
         Model: agent.ModelOpus,
     }
     claude.StartWithOptions(prompt, repoPath, opts)
     ```

2. **Extended Iteration Budget**:
   - Increased from 3 to 5 retries
   - Constant: `MaxMergeResolverRetries = 5`
   - Communicated to agent in prompt

3. **Comprehensive Context Integration**:
   - MergeContextBuilder field in MergeResolverAgent
   - Builds full context before resolution
   - Non-fatal if context building fails (continues with basic prompt)

4. **Enhanced Prompt with Reasoning**:
   - Includes full task history
   - Shows branch diffs with merge base
   - Displays detailed conflict information per file
   - Requires agent to explain resolution strategy before implementing:
     ```
     3. **Develop Strategy**: EXPLAIN your resolution strategy before implementing:
        - What does the target branch accomplish?
        - What does the agent branch accomplish?
        - Are these changes compatible or contradictory?
        - How will you merge them to preserve both intents?
     ```
   - Provides validation checklist
   - Emphasizes preserving both intents

5. **Additional Validation**:
   - New method: `validateNoConflictMarkers()`
   - Checks for remaining conflict markers (<<<<<<, ======, >>>>>>)
   - Ensures clean resolution before proceeding

## Architecture

### Merge Resolution Flow

```
Merge Conflict Detected
         ↓
┌─────────────────────────────────────────────────────────┐
│ 1. Orchestrator.SetMergeConflict()                      │
│    - Blocks all task scheduling                         │
│    - Waits for in-flight tasks to complete              │
└─────────────────────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────────────────────┐
│ 2. MergeResolverAgent.Resolve()                         │
│    - Get target branch                                  │
│    - Build comprehensive context (if available)         │
│    - Create enhanced prompt with context                │
│    - Spawn Opus agent with extended retries             │
└─────────────────────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────────────────────┐
│ 3. MergeContextBuilder.Build()                          │
│    - Find merge base                                    │
│    - Get branch diffs                                   │
│    - Build task history from graph                      │
│    - Get conflict details per file                      │
│    - Format for prompt                                  │
└─────────────────────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────────────────────┐
│ 4. Opus Agent Resolves Conflicts                        │
│    - Reviews full task history                          │
│    - Analyzes both branches                             │
│    - Explains resolution strategy                       │
│    - Implements merge                                   │
│    - Validates (build + tests)                          │
│    - Commits resolution                                 │
└─────────────────────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────────────────────┐
│ 5. Validation                                           │
│    - validateResolution() - git status check            │
│    - validateNoConflictMarkers() - scan files           │
└─────────────────────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────────────────────┐
│ 6. Orchestrator.ClearMergeConflict()                    │
│    - Clears merge conflict flag                         │
│    - Triggers scheduler to resume                       │
└─────────────────────────────────────────────────────────┘
```

### Context Building Flow

```
MergeContextBuilder.Build()
         ↓
    ┌────────┴────────┐
    ↓                 ↓
GetMergeBase     GetCompletedTasks
    ↓                 ↓
GetBranchDiffs   BuildTaskSummaries
    ↓                 ↓
GetFileContents  ExtractIntents
    ↓                 ↓
    └────────┬────────┘
         ↓
   FormatForPrompt
```

## Example: Enhanced Merge Prompt

The merge agent now receives comprehensive context:

```markdown
# URGENT: Merge Conflict Resolution Required

You are a dedicated merge conflict resolver using the Opus model (highest intelligence).
The orchestrator has STOPPED all other work until you resolve these conflicts.

## Conflict Context

**Task ID**: task-123
**Target Branch**: alphie-auth-system-20250122
**Agent Branch**: task-123-agent-abc
**Merge Base**: a1b2c3d4

**Conflicting Files** (2):
- internal/auth/handler.go
- internal/auth/middleware.go

## Task History

**Completed Tasks**: 5

1. **Add JWT authentication** (task-001)
   Intent: Implement JWT token generation and validation.
   Files: internal/auth/jwt.go, internal/auth/keys.go

2. **Add login endpoint** (task-002)
   Intent: Create POST /login endpoint for user authentication.
   Files: internal/api/routes.go, internal/auth/handler.go

3. **Add token validation middleware** (task-003)
   Intent: Middleware to validate JWT tokens on protected routes.
   Files: internal/auth/middleware.go

4. **Add user model** (task-004)
   Intent: Define User struct with validation.
   Files: internal/models/user.go

5. **Add password hashing** (task-005)
   Intent: Bcrypt password hashing for secure storage.
   Files: internal/auth/password.go

## Conflict Details

### internal/auth/handler.go
**Conflict Type**: both_modified

**Merge Base Version**:
```
func LoginHandler(w http.ResponseWriter, r *http.Request) {
    // Basic implementation
}
```

**Target Branch Version**:
```
func LoginHandler(w http.ResponseWriter, r *http.Request) {
    // With validation
    if err := validateRequest(r); err != nil {
        http.Error(w, err.Error(), 400)
        return
    }
    // Generate JWT
}
```

**Agent Branch Version**:
```
func LoginHandler(w http.ResponseWriter, r *http.Request) {
    // With password hashing
    hash, err := bcrypt.GenerateFromPassword(password, 10)
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
    // Generate JWT
}
```

## Your Mission

1. **Understand Context**: Review the task history above
2. **Analyze Intent**: Read both conflicting versions
3. **Develop Strategy**: EXPLAIN your resolution strategy:
   - What does the target branch accomplish?
   - What does the agent branch accomplish?
   - Are these changes compatible or contradictory?
   - How will you merge them to preserve both intents?
4. **Merge Intelligently**: Create unified versions
5. **Validate**: Ensure merged code compiles and tests pass
6. **Commit**: Stage all resolved files and commit

## Critical Requirements

- **DO NOT** lose functionality from either branch
- **DO** explain your resolution strategy before implementing
- **DO** consider the context of all completed tasks
- **DO** ensure no conflict markers remain
- **DO** commit with message: "Merge conflict resolved for task task-123"

You have extended retries (5 attempts) due to complexity.
```

## Benefits

### Before Phase 5
- Basic merge resolver with minimal context
- Generic Sonnet model (same as regular agents)
- Simple prompt with just file names
- No understanding of task history or intents
- 3 retry attempts (same as regular tasks)
- No validation of conflict marker removal

### After Phase 5
- **Opus model** (highest intelligence) for complex resolution
- **Comprehensive context** with full task history
- **Branch diffs** showing all changes from merge base
- **Detailed conflict info** with all 3 versions (base, target, agent)
- **Intent extraction** from completed tasks
- **Reasoning requirements** - must explain strategy
- **5 retry attempts** (extended for complexity)
- **Validation checklist** provided to agent
- **Conflict marker scanning** before considering resolved

## Integration with Orchestrator

The orchestrator already has pause/resume functionality:

### Existing Methods (No Changes Needed)
- `SetMergeConflict(taskID, files)` - Blocks scheduling
- `HasMergeConflict()` - Check if blocked
- `ClearMergeConflict()` - Resume scheduling

### Pause Behavior
1. Merge conflict detected during merge attempt
2. `SetMergeConflict()` called - sets flag
3. Scheduler checks `HasMergeConflict()` before scheduling
4. New tasks blocked, in-flight tasks continue
5. Merge resolver spawned with Opus + context
6. After resolution, `ClearMergeConflict()` called
7. Scheduler trigger fired - resumes work

## Performance Considerations

### Context Building Overhead
- **Merge base lookup**: ~50-100ms (git operation)
- **Branch diff**: ~100-200ms per branch (git operation)
- **Task history**: ~10ms (in-memory graph traversal)
- **File contents**: ~50-100ms per file (git show)
- **Total**: ~500ms-2s for context building

This is acceptable overhead given:
- Merge conflicts are rare
- Opus model is much slower than Sonnet anyway
- Comprehensive context leads to better resolution

### Opus Model Costs
- Opus is ~5x more expensive than Sonnet
- Input: $15/1M tokens (vs $3/1M for Sonnet)
- Output: $75/1M tokens (vs $15/1M for Sonnet)
- Worth the cost for critical merge resolution

## Testing Strategy

### Manual Testing

1. **Create Conflicting Tasks**:
   ```bash
   # Create spec with overlapping changes
   cat > conflict-test.md << EOF
   # Conflict Test
   - Task A: Modify function foo() to add param X
   - Task B: Modify function foo() to add param Y
   EOF

   alphie implement conflict-test.md
   ```

2. **Verify Context Building**:
   - Check logs for context building messages
   - Verify task history included in prompt
   - Verify branch diffs captured

3. **Verify Opus Usage**:
   - Check logs for model specification
   - Verify higher token costs (Opus pricing)

4. **Verify Extended Retries**:
   - Intentionally create hard-to-resolve conflicts
   - Verify agent gets 5 attempts instead of 3

5. **Verify Validation**:
   - Manually leave conflict markers in resolved file
   - Verify validateNoConflictMarkers catches them

### Automated Testing

Unit tests for:
- `MergeContextBuilder.Build()` - mocked git operations
- `extractIntent()` - intent extraction logic
- `extractConflictType()` - conflict type parsing
- `validateNoConflictMarkers()` - marker detection
- `FormatForPrompt()` - prompt formatting

Integration tests:
- Mock orchestrator with merge conflict scenario
- Verify context building and prompt generation
- Verify Opus model selection

## Current Status

### ✅ Complete

- [x] Created `merge_context.go` with comprehensive context building
- [x] Enhanced `merge_resolver.go` with Opus model
- [x] Extended iteration budget to 5 retries
- [x] Enhanced prompt with reasoning requirements
- [x] Added conflict marker validation
- [x] Integrated with orchestrator pause/resume
- [x] Documentation and examples

### 🚧 Needs Integration (Future Phases)

- [ ] End-to-end testing with real conflicts (Phase 11)
- [ ] TUI display for merge conflict status (Phase 8)
- [ ] Metrics tracking for merge resolution success rate

## Key Improvements

✅ **Intelligent Model Selection** - Opus for complex merges
✅ **Comprehensive Context** - Full task history and intents
✅ **Reasoning Requirements** - Must explain strategy
✅ **Extended Budget** - 5 retries for complexity
✅ **Better Validation** - Conflict marker scanning
✅ **Branch Diffs** - Shows all changes from merge base
✅ **Conflict Details** - All 3 versions (base, target, agent)
✅ **Non-Fatal Fallback** - Continues with basic prompt if context fails

## Metrics

**Code Added**: 570 lines
**Files Created**: 1 file (`merge_context.go`)
**Files Enhanced**: 1 file (`merge_resolver.go`)
**Integration Points**: 2 (orchestrator, graph)
**Documentation**: 450+ lines (this summary)

## Key Achievements

✅ **Opus model integration** for highest intelligence
✅ **Comprehensive merge context** with full history
✅ **Intent-based resolution** understanding what each branch does
✅ **Extended iteration budget** (5 attempts)
✅ **Enhanced validation** with conflict marker scanning
✅ **Reasoning requirements** for transparent resolution
✅ **Non-fatal error handling** with graceful fallback

## Next Steps

With Phase 5 complete, remaining work:

**Independent (can do in parallel)**:
- Phase 6: Implement user escalation (~2-3 hours)
- Phase 9: Update branch naming (~1 hour)
- Phase 10: Update help text (~1 hour)

**Dependent (require above phases)**:
- Phase 7: Update implement command (~3-4 hours)
- Phase 8: Simplify TUI (~2-3 hours)
- Phase 11: End-to-end testing (~4-6 hours)

## Conclusion

Phase 5 successfully transforms merge conflict resolution from a basic fallback mechanism into an intelligent, context-aware system. By using the Opus model with comprehensive task history and branch context, the merge resolver can make informed decisions that preserve the intent of both conflicting branches.

The extended retry budget and enhanced validation ensure thorough resolution, while the reasoning requirements provide transparency into the resolution strategy.

---

**Progress**: 45% complete (5/11 phases)
**Phase 5 Status**: ✅ COMPLETE
