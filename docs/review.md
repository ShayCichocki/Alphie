# Alphie Codebase Review - Full Analysis

**Date:** 2026-01-13
**Updated:** 2026-01-14
**Scope:** Full codebase review for brittleness, missing implementations, and failure points

---

## Remediation Progress

### Completed Fixes

| Date | Fix | Files Changed |
|------|-----|---------------|
| 2026-01-14 | **Merge Queue** - Serializes all merge operations, prevents race conditions | `merge_queue.go` (new), `orchestrator.go` |
| 2026-01-14 | **Semantic merge retry with backoff** - 3 retries with exponential backoff (2s→4s→8s) | `merge_queue.go` |
| 2026-01-14 | **Semantic merger factory** - Creates fresh Claude processes above normal agent limits | `merge_queue.go`, `orchestrator.go` |
| 2026-01-14 | **Merge fallback strategies** - SmartMerge for critical files, graceful degradation | `merge_queue.go` |
| 2026-01-14 | **DependencyGraph mutex** - Added sync.RWMutex to protect concurrent map access | `graph.go` |
| 2026-01-14 | **Worktree recovery logic fixed** - Was removing active worktrees instead of orphans | `worktree.go` |
| 2026-01-14 | **Pause/resume goroutine leak fixed** - Moved goroutine spawn outside loop | `orchestrator.go` |

### Remaining Critical Issues: 8 (was 11)

---

## Summary

| Dimension | CRITICAL | HIGH | MEDIUM | LOW | Verdict |
|-----------|----------|------|--------|-----|---------|
| Core Orchestration | ~~3~~ 2 | 5 | 6 | 3 | **FAIL** |
| State Management | 3 | 4 | 5 | 3 | **FAIL** |
| Git/Merge Operations | ~~3~~ 2 | 3 | 4 | 3 | **FAIL** |
| Verification System | 2 | 3 | 4 | 5 | **FAIL** |
| **TOTAL** | ~~**11**~~ **8** | ~~**15**~~ **14** | **19** | **14** | **FAIL** |

## Overall: FAIL - 8 Critical + 14 High Issues Remain (4 Fixed)

---

## CRITICAL Issues (Must Fix)

### Race Conditions & Concurrency

| # | Issue | File | Impact |
|---|-------|------|--------|
| 1 | ~~**DependencyGraph has no mutex**~~ | ~~`graph.go`~~ | ~~Multiple goroutines read/write `nodes`, `edges`, `completed` maps concurrently → crashes, data corruption~~ **FIXED: Added sync.RWMutex** |
| 2 | ~~**No merge lock - concurrent merges race**~~ | ~~`merger.go`~~ | ~~Two agents finishing simultaneously corrupt session branch~~ **FIXED: MergeQueue serializes all merges** |
| 3 | **Scheduler mutates Task objects without sync** | `scheduler.go:240-248` | `markDependentsBlocked()` mutates shared Task.Status causing data races |

### Infinite Loops & Goroutine Leaks

| # | Issue | File | Impact |
|---|-------|------|--------|
| 4 | **runLoop re-injects completions infinitely** | `orchestrator.go:637-638` | When no tasks ready, completion re-injected via goroutine → infinite loop, leaked goroutines |
| 5 | ~~**Pause/resume spawns goroutine per iteration**~~ | ~~`orchestrator.go:651-662`~~ | ~~Repeated pause/unpause accumulates orphan goroutines → memory leak, potential deadlock~~ **FIXED** |

### State Persistence Failures

| # | Issue | File | Impact |
|---|-------|------|--------|
| 6 | **Agent PID never persisted** | `orchestrator.go:725` | PID is 0 in database → recovery can't kill orphaned Claude processes |
| 7 | **graph.completed map is memory-only** | `graph.go:204-208` | Crash loses all dependency completion state → tasks re-executed or stuck |
| 8 | **Task/Agent status updates not atomic** | `orchestrator.go:757-759, 900-906` | Crash between updates → inconsistent state, tasks stuck forever |

### Git Operation Failures

| # | Issue | File | Impact |
|---|-------|------|--------|
| 9 | **Revert failure silently ignored** | `semantic.go:203-221` | Validation fails, revert fails → repo left in broken merged state |
| 10 | **Rebase abort failure not handled** | `merger.go:98-113` | Repo stuck in mid-rebase state, all subsequent ops fail |

### Dead/Missing Code

| # | Issue | File | Impact |
|---|-------|------|--------|
| 11 | **FocusedTestSelector never used** | `ralph_loop.go:21,57` | 426-line subsystem (`testselect.go`) is completely dead code - focused testing doesn't work |

---

## HIGH Issues (Should Fix Before Production)

### Data Loss & Corruption

| Issue | File | Impact |
|-------|------|--------|
| ~~Worktree cleanup logic **INVERTED** (BUG)~~ | ~~`worktree.go:242-263`~~ | ~~Recovery removes ACTIVE worktrees instead of orphaned ones~~ **FIXED** |
| Non-atomic smart merge file writes | `pkgmerge.go:396-426` | Disk full mid-write → partial merge applied |
| WorktreePath never set in agent record | `orchestrator.go:1385-1393` | Recovery can't clean up orphaned worktrees |
| Session resume ignores dependencies | `orchestrator.go:536-542` | Tasks execute out of order after crash |

### Lost Completions & Hangs

| Issue | File | Impact |
|-------|------|--------|
| Context cancellation loses completion signal | `executor.go:810-818` | Result stored but completion never signaled → orchestrator hangs |
| Stderr overflow drops diagnostic info | `claude.go:215-224` | Critical error messages silently lost |
| New ClaudeProcess per iteration without cleanup | `ralph_loop.go:143-149` | Zombie Claude processes accumulate |

### Silent Failures

| Issue | File | Impact |
|-------|------|--------|
| `must_not_change` constraint not implemented | `contract.go:263-266` | Contracts specify protected files but constraint ignored |
| JSON parsing failures return minimal contract | `generator.go:217-258` | Claude garbage → silent degradation to no verification |
| Draft/Refine errors suppressed | `generator.go:280-388` | Network/Claude failures invisible |
| Lock file regeneration errors ignored | `pkgmerge.go:412-423` | package-lock.json/go.sum out of sync |

### Recovery Gaps

| Issue | File | Impact |
|-------|------|--------|
| ListAgents returns ALL sessions | `recovery.go:100-103` | Recovery processes wrong session's agents |
| No atomic state transition for task assignment | `orchestrator.go:730-759` | Crash mid-assignment → orphaned agents or unassigned tasks |

---

## MEDIUM Issues (Should Fix)

| Category | Count | Examples |
|----------|-------|----------|
| Error handling ignores DB failures | 4 | `updateSessionStatus()`, `updateTaskState()`, `createAgentState()` all ignore return values |
| Event/channel drops | 2 | `emitEvent()` silently drops when channel full; TUI shows stale state |
| Lock contention | 2 | WorktreeManager holds mutex during git ops; MergeHandler no git lock detection |
| Input validation missing | 3 | No contract validation, invalid TaskType accepted, empty titles allowed |
| Status type mismatch | 1 | `state.TaskStatus` has `Canceled`, `models.TaskStatus` has `Failed` - casting breaks |
| Naive implementations | 2 | `hasNodeScript()` uses string matching not JSON parsing; collision checker parses descriptions |

---

## LOW Issues (Nice to Fix)

| Category | Count | Examples |
|----------|-------|----------|
| Resource leaks | 2 | Debug log file never closed, no timeout maximum on contracts |
| Missing context | 2 | Git errors don't include command run, time parsing errors ignored |
| Code quality | 3 | Deprecated prompt still used, redundant error handling, scheduler returns mutable pointers |

---

## Top 10 Fixes by Impact

| Priority | Fix | Effort | Impact | Status |
|----------|-----|--------|--------|--------|
| 1 | ~~Add mutex to DependencyGraph~~ | ~~Low~~ | ~~Prevents crashes under load~~ | **DONE** ✓ |
| 2 | ~~Serialize merge operations~~ | ~~Medium~~ | ~~Prevents branch corruption~~ | **DONE** ✓ |
| 3 | ~~Fix worktree recovery logic (inverted condition)~~ | ~~Low~~ | ~~Prevents data loss~~ | **DONE** ✓ |
| 4 | Fix runLoop completion re-injection | Medium | Prevents infinite loops | |
| 5 | ~~Fix pause/resume goroutine leak~~ | ~~Low~~ | ~~Prevents memory exhaustion~~ | **DONE** ✓ |
| 6 | Make task/agent state updates atomic | High | Consistent recovery | |
| 7 | Wire FocusedTestSelector into RalphLoop | Medium | Makes focused testing work | |
| 8 | Implement `must_not_change` constraint | Medium | Protects critical files | |
| 9 | Add error propagation to DB operations | Medium | Makes failures visible | |
| 10 | Fix pause/resume goroutine leak | Low | Prevents memory exhaustion | |

---

## Architecture Observations

### Fundamental Design Issues

1. **Dual source of truth everywhere** - Task status in models vs state DB vs graph.completed map
2. **No transaction boundaries** - Multi-step operations have no atomicity
3. **Silent degradation pattern** - Errors converted to fallbacks without visibility
4. **Missing integration** - Components exist but aren't wired (FocusedTestSelector, must_not_change)

### What Works Well

- Verification contract concept is sound
- Baseline-aware gate evaluation is well-designed
- Tier keyword unification was good
- Ralph-loop structure is reasonable

### What Needs Rethinking

- State persistence strategy (need proper transactions)
- Concurrency model (need explicit locking strategy)
- Error handling philosophy (fail loudly vs degrade silently)
- Recovery guarantees (what invariants are maintained?)

---

## Recommended Next Steps

1. ~~**Immediate**: Fix the inverted worktree recovery logic - this is a shipped bug~~ **DONE** ✓
2. ~~**This week**: Add DependencyGraph mutex - crashes are likely under parallel load~~ **DONE** ✓
3. ~~**This week**: Serialize merge operations - branch corruption is catastrophic~~ **DONE** ✓
4. **Soon**: Audit all error handling - too many silent failures
5. **Planning**: Design proper transaction boundaries for state updates

---

## Detailed Findings by Component

### Core Orchestration (`internal/orchestrator/`)

#### orchestrator.go
- Lines 637-638: Completion re-injection creates infinite loop potential
- ~~Lines 651-662: Pause/resume goroutine leak~~ **FIXED**
- Lines 725: Agent PID never set (always 0)
- Lines 730-759: No atomic state transition for task assignment
- Lines 757-759, 900-906: Task/agent status updates not atomic
- Lines 1257-1262: `emitEvent()` silently drops events when channel full
- Lines 1319-1331, 1376-1377, 1393, 1408: DB errors silently ignored

#### graph.go
- ~~Lines 21-22: No mutex on DependencyGraph struct~~ **FIXED: Added sync.RWMutex**
- ~~Lines 147, 204, 211: Concurrent access to maps without synchronization~~ **FIXED: All methods now lock-protected**
- Lines 170-180: Dual source of truth (completed map vs task status)
- Lines 204-208: `completed` map never persisted

#### scheduler.go
- Lines 60-171: `Schedule()` uses RLock but iterates map that can be modified
- Lines 173-210: `OnAgentStart`/`OnAgentComplete` acquire full Lock
- Lines 222-228: `getRunningAgentsLocked()` returns mutable pointers
- Lines 240-248: `markDependentsBlocked()` mutates shared Task objects

#### merger.go
- ~~Lines 54-147: No mutex/lock for merge operations~~ **FIXED: MergeQueue serializes all merges**
- Lines 88-95: Checkout failure leaves repo on wrong branch
- Lines 98-113: Rebase abort failure silently ignored
- Lines 241-242: Pull failure silently ignored
- Lines 299-308: Git errors missing command context

#### merge_queue.go (NEW)
- Serializes all merge operations through single worker goroutine
- Retry with exponential backoff for semantic merges (3 retries, 2s→4s→8s)
- Semantic merger factory creates fresh Claude processes above normal limits
- Fallback strategies: SmartMerge for critical files, graceful degradation for others

#### semantic.go
- Lines 189-199: File writes not coordinated with git operations
- Lines 203-221: Revert failure silently ignored

#### pkgmerge.go
- Lines 396-426: Non-atomic file writes (no rollback)
- Lines 412-423: Lock file regeneration errors ignored

### Agent Execution (`internal/agent/`)

#### executor.go
- Lines 249-332: Startup retry doesn't clean up previous process
- Lines 810-818: Context cancellation loses completion signal

#### ralph_loop.go
- Lines 21, 56-57: FocusedTestSelector created but never used
- Lines 143-149, 460-464: New ClaudeProcess per iteration without cleanup

#### worktree.go
- Lines 57-82: Mutex held during long git operations
- ~~Lines 242-263: **BUG** - Recovery logic inverted (removes known worktrees)~~ **FIXED**

#### claude.go
- Lines 215-224: Stderr overflow silently drops messages

#### baseline.go
- Lines 59-84: Redundant error handling, errors discarded

#### gates.go
- Lines 416-424: `hasNodeScript()` uses naive string matching

### State Management (`internal/state/`)

#### recovery.go
- Lines 60: Checks `a.PID > 0` but PIDs never persisted
- Lines 100-103, 144-147: `ListAgents(nil)` returns all sessions
- Lines 324-336: `isProcessAlive()` fails for permission reasons

#### session.go
- Lines 32-40: Status enum differs from models.TaskStatus
- Lines 113, 166, 386, 521: Time parsing errors ignored

#### db.go
- Lines 172-207: No session_id foreign key on agents/tasks
- Lines 270-289: `PurgeOldSessions` no cascade delete

### Verification (`internal/verification/`)

#### contract.go
- Lines 35-39, 198-218: Regex matching documented but not implemented
- Lines 263-266: `must_not_change` not implemented (explicit TODO)
- Lines 308-315: No validation in `ParseContractJSON()`

#### generator.go
- Lines 93-139: Deprecated prompt still in use
- Lines 217-258: JSON parsing failures return minimal contract silently
- Lines 280-316, 321-388: Draft/Refine errors suppressed

#### storage.go
- No concurrent access protection
