# Alphie: Agent Orchestrator & Learning Engine

## Overview

Alphie orchestrates parallel Claude Code agents on workstreams, accumulates learnings, and manages tasks to maximize development throughput.

**What it does:**
- Decomposes work into parallelizable tasks
- Spawns isolated agents in git worktrees
- Self-improves code via Ralph-loop (critique → improve → repeat)
- Learns from failures and successes
- Merges safely via session branches

**Core principle:** Curiosity vs efficiency is tier-dependent.
- Scout = pure execution (infer and do)
- Builder = balanced (some questions allowed)
- Architect = full exploration (unlimited questions, human review)

---

# Part 1: Architecture Design

## 1. Task Decomposition Engine

Alphie breaks work into parallelizable units.

**Input:** User request or epic
**Output:** Dependency graph of tasks optimized for parallel execution

**Rules:**
- Tasks with no dependencies run in parallel
- Tasks are sized for single-agent completion
- Each task has clear acceptance criteria

**Parallelism Model:** Resource-based with tier presets

| Tier | Max Agents | Primary Model | Use Case |
|------|------------|---------------|----------|
| Scout | 2 | haiku | Quick exploration, simple fixes |
| Builder | 3 | sonnet | Standard feature work |
| Architect | 5 | opus | Complex redesigns, major features |

**Scheduler Collision Avoidance:**

```go
type SchedulerHint struct {
    PathPrefixes []string  // e.g., ["src/auth/", "src/api/"]
    Hotspots     []string  // Files touched >3x in session
}

// Scheduling rules:
// 1. Avoid concurrent tasks on same path prefix
// 2. Serialize tasks touching hotspot files
// 3. Max 2 agents on same top-level directory
```

**Budget Exhaustion:** Graceful wind-down
- Complete in-progress work
- Block remaining tasks
- Report what's done

**Task Sizing:** Scope-based
- Task = single logical unit (one function, one component, one test file)
- Alphie decomposes by natural code boundaries
- Tasks map to reviewable units

---

## 2. Agent Dispatch

Alphie spawns sub-agents for task execution.

**Model Selection:** Auto-selects based on task type/keywords

| Task Type | Model | Rationale |
|-----------|-------|-----------|
| Simple function/boilerplate | haiku | Fast, cheap, sufficient |
| Standard feature work | sonnet | Balance of capability/cost |
| Architecture/design decisions | opus | Requires deep reasoning |
| Code review/critique | sonnet | Balanced judgment |

No user intervention needed. Alphie infers from task labels and complexity.

---

## 3. Ralph-Loop (Self-Improvement Cycle)

**Purpose:** Quality refinement through self-critique with verification-aware governance

**Entry:** Every task enters the loop after initial implementation

**Mechanics:**
- **Reviewer:** Same agent, critic prompt (single context, cost-efficient)
- **Metric:** Structured rubric score + verification contract results
- **Verification:** Intent-based contracts generated post-implementation

| Criterion | Score Range | Description |
|-----------|-------------|-------------|
| Correctness | 1-3 | Does it work? Handle edge cases? |
| Readability | 1-3 | Is the code clear and maintainable? |
| Edge cases | 1-3 | Are failure modes handled? |

**Thresholds by tier:**
| Tier | Min Score | Rationale |
|------|-----------|-----------|
| Scout | 5/9 | Good enough, move fast |
| Builder | 7/9 | Solid quality |
| Architect | 8/9 | Excellence required |

**Decision Matrix (with verification):**
| Verification | Score | Action |
|--------------|-------|--------|
| PASS | >= threshold | EXIT SUCCESS |
| PASS | >= threshold-1 | EXIT (acceptable) |
| FAIL | any | Inject failure context, continue improving |
| any | any | max iterations → EXIT with current status |

**Exit Conditions:**
1. Verification passes AND quality threshold met
2. Verification passes AND score is acceptable (threshold - 1)
3. Agent outputs DONE marker AND verification passes (DONE is a request, not automatic exit)
4. Hidden max iterations reached (3-7 depending on tier)

**DONE Marker Validation:**
When an agent outputs "DONE", it's requesting exit—not declaring it. Verification must still pass:
- If verification passes: exit with reason "agent_done_verified"
- If verification fails: continue improving with injected failure context

**Clean Abort:** When max iterations reached without passing verification:
- Task marked as failed (not merged)
- Failure message includes verification summary
- Orchestrator emits failure event
- Work is NOT merged to session branch

**Flow:**
```
task decomposition → generate VerificationIntent
                              ↓
            DraftContract() → store to .alphie/contracts/<id>-draft.json
                              ↓
                        agent implements
                              ↓
            RefineContract() → can only ADD checks (monotonic strengthening)
                              ↓
            store final to .alphie/contracts/<id>.json
                              ↓
                    run verification
                              ↓
               ┌──────────────┴──────────────┐
               │                             │
      PASS + threshold             FAIL or below threshold
               │                             │
               ▼                             ▼
             done              inject context → critique → improve
                                             │
                                             └──────────→ repeat
```

---

## 4. Concurrency Control

**Strategy:** Pure optimistic with git worktrees

Each agent operates in an isolated worktree. No locking, no contention. Accept that conflicts will happen and semantic merge handles them.

**Worktree Lifecycle:** Per-agent ephemeral
```
agent spawns → git worktree add ~/.cache/alphie/worktrees/agent-{uuid} -b agent-{uuid}
agent works  → isolated changes in dedicated worktree
agent done   → merge to session-{id} branch (or main if --greenfield)
cleanup      → git worktree remove
```

**Protected Branches:** main, master, dev - never direct merge unless `--greenfield`

**Conflict Resolution:** Semantic merge agent (with strict conditions)

Semantic merge only allowed when:
- Changes are in disjoint file paths, OR
- Same file but different functions, OR
- Both sides pass targeted tests + full suite after merge

Otherwise: escalate to human immediately.

When merge conflicts occur:
1. Check if strict conditions allow semantic merge
2. If allowed: spawn dedicated merge agent
3. Agent reads both diffs, understands intent
4. Agent produces merged code preserving both intents
5. Run targeted tests + full suite to validate
6. If unresolvable or tests fail: escalate to user

---

## 5. Learning System

**Storage Format:** Condition → Action → Outcome (CAO) triples

```
WHEN <condition>
DO <action>
RESULT <outcome>
```

**Examples:**
```
WHEN build fails with "assets not embedded" error
DO use `go build` instead of `go run`
RESULT build succeeds with embedded assets

WHEN tests timeout on CI but pass locally
DO check for hardcoded localhost references
RESULT found and fixed 3 localhost URLs, tests pass
```

**Retrieval:** Always at task start
- Before agent begins, query learnings for relevant context
- Every task gets learning boost proactively

**Backend:** SQLite (local-only for v1)
- Location: `~/.local/share/alphie/alphie.db`
- No cross-machine sync in v1
- Future: Server-authoritative API when distributed learning needed

**Scope:** Repo-local by default
- Learnings stored per-project in `.alphie/learnings.db`
- Global learnings (user preferences) in `~/.local/share/alphie/alphie.db`
- Cross-project sync deferred to v2

**Learning Categories:**

| Category | Example | Storage |
|----------|---------|---------|
| Codebase patterns | "Auth uses JWT middleware in /api/auth/*" | Project-local |
| Failure recovery | "Build fails if assets not embedded" | Project-local |
| User preferences | "User prefers explicit error handling" | Global (machine) |
| Technique effectiveness | "Parallel writes cause races here" | Project-local |

**Extended Learning Schema:**

```go
type Learning struct {
    // Core CAO
    Condition string  // WHEN
    Action    string  // DO
    Outcome   string  // RESULT

    // Evidence
    CommitHash    string    // Where this was discovered
    LogSnippetID  string    // Reference to failure log

    // Scope
    Scope string // "repo", "module", "global"

    // Lifecycle
    TTL         time.Duration // Decay after X without triggers
    LastTriggered time.Time
    TriggerCount  int

    // Outcome type
    OutcomeType string // "tests_pass", "perf_improved", "reverted", "bug_reopened"
}
```

---

## 6. Communication Protocol

**Verbosity:** Event-based minimal

Update triggers (4 types only):
- Task started
- Task completed
- Task blocked (with reason)
- Error encountered

No stream of consciousness. Signal, not noise.

**Questions:** Tier-dependent with override gates

| Tier | Questions Allowed | Behavior |
|------|-------------------|----------|
| Scout | Zero (with overrides) | Infer and execute, unless blocked or protected area |
| Builder | 1-2 | Only if genuinely ambiguous |
| Architect | Unlimited | Full clarification permitted |

**Scout Override Gates:**
- `blocked_after_n_attempts`: Can ask after 5 failed retries
- `protected_area_detected`: Can ask when touching auth/migrations/infra

**Protected Area Detection Rules:**

```yaml
protected_areas:
  patterns:
    - "**/auth/**"
    - "**/security/**"
    - "**/migrations/**"
    - "**/infra/**"
    - "**/secrets/**"
    - "**/.env*"
    - "**/credentials*"
    - "**/Dockerfile"
    - "**/docker-compose*"
    - "**/*.pem"
    - "**/*.key"

  keywords_in_path:
    - auth
    - login
    - password
    - token
    - secret
    - key
    - migration
    - schema
    - permission
    - role
    - acl

  file_types:
    - .sql   # Database migrations
    - .tf    # Terraform
```

```go
func IsProtectedArea(path string) bool {
    for _, pattern := range config.ProtectedAreas.Patterns {
        if matched, _ := filepath.Match(pattern, path); matched {
            return true
        }
    }
    for _, keyword := range config.ProtectedAreas.Keywords {
        if strings.Contains(strings.ToLower(path), keyword) {
            return true
        }
    }
    return false
}
```

**Question Types:**
1. **Clarifying:** "Did you mean X or Y?"
2. **Confirming:** "I'll do X, correct?"
3. **Discovering:** "Why is it done this way?" (Architect tier only)

---

## 7. Failure Handling

**Retry Strategy:** Tiered with human escalation

```
Attempt 1: Original approach
Attempt 2: Alternative approach
Attempt 3: Another alternative
Attempt 4: Last autonomous try
Attempt 5: ESCALATE → human decides
```

**On each failure:**
1. Capture error context
2. Search learnings for known fix
3. If found: apply and retry
4. If not: try alternative approach
5. Log attempt via `prog log`

**Rollback:** Git worktree reset

When agent fails unrecoverably:
1. `git worktree remove` - changes gone, clean slate
2. Log failure details via `prog log`
3. Mark task blocked via `prog block`
4. Store learning if pattern identified

No partial state. Atomic rollback.

---

## 8. Quality Gates

**Required Gates:**
- [x] Tests pass
- [x] Build succeeds
- [x] Lint clean
- [x] Type check passes

**Baseline Capture:** At session start, Alphie captures the current test/lint state and stores it in `.alphie/baselines/<session-id>.json`. This baseline is used throughout the session to detect regressions.

**Strictness:** No regressions allowed (baseline-aware)

| Scenario | Action |
|----------|--------|
| Pre-existing failure (in baseline) | Allowed (not agent's fault) |
| New failure introduced | Blocked (agent must fix) |
| Worsening existing failures | Blocked (more failures than baseline = fail) |
| Touch a component | Its focused tests must pass |
| Gate not applicable | Skip (no tests = skip test gate) |

**Baseline Integration with Quality Gates:**
```go
type BaselineComparison struct {
    NewFailures      []string  // Failures not in baseline (agent's fault)
    RegressionCount  int       // How many more failures than baseline
    IsRegression     bool      // True if worse than baseline
}

// Gates now check against baseline, not just pass/fail
comparison := CompareToBaseline(currentGateResults, sessionBaseline)
if comparison.IsRegression {
    task.Status = TaskStatusFailed
}
```

**Pass/Fail Hierarchy:**
Quality gates now actually block task completion (previously they only logged warnings). The hierarchy is:
1. Safety constraints (protected areas, must_not_change) - immediate block
2. Verification contract (task-specific checks) - retry with context
3. Quality gates (baseline-aware test, build, lint) - fail if regression

**Focused Test Selection Strategy:**

```go
type FocusedTestSelector struct {
    ColocatedPattern string              // "{file}_test.go"
    PackageScope     bool                // Run package tests
    TagMapping       map[string][]string // pathPrefix → test tags
}
```

Test selection rules (in order):
1. **Co-located tests**: `src/auth/handler.go` → run `src/auth/handler_test.go`
2. **Package tests**: `src/auth/*.go` → run `go test ./src/auth/...`
3. **Tag-based tests**: touching `src/auth/*` → run tests tagged `@auth`
4. **Caller tests**: If exported function changed, find callers and run their tests
5. **Full suite**: Always run full test suite at session end (before PR)

Default behavior:
- Run focused tests after each agent completes
- Run full suite once before creating PR to main
- If focused tests find <5 tests, expand to package scope

---

## 8.5. Verification System

**Purpose:** Ensure task completion matches intent through executable contracts

The verification system bridges the gap between "task appears done" and "task actually works as intended." It provides concrete, executable verification of task outcomes.

**Three-Phase Verification (Gaming Prevention):**

| Phase | When | What |
|-------|------|------|
| Intent Capture | Task decomposition | Human-readable acceptance criteria in `task.VerificationIntent` |
| Draft Contract | Pre-implementation | Generated BEFORE agent implements; establishes minimum requirements |
| Refined Contract | Post-implementation | Can only ADD checks, never weaken the draft |

**Why Pre-Implementation Contracts:**
Generating contracts after seeing the implementation allows agents to create weak checks that rubber-stamp their work. Pre-implementation contracts set expectations based on intent, not outcomes.

**Verification Contract Structure:**

```go
type VerificationContract struct {
    // Intent is human-readable acceptance criteria (from decomposition)
    Intent string `json:"intent"`

    // Commands are concrete verification steps (generated post-implementation)
    Commands []VerificationCommand `json:"commands,omitempty"`

    // FileConstraints define what must/must-not exist or change
    FileConstraints FileConstraints `json:"file_constraints,omitempty"`
}

type VerificationCommand struct {
    Command     string        // e.g., "npm test -- --grep login"
    Expect      string        // "exit 0", "output contains X"
    Description string        // Human-readable explanation
    Required    bool          // Hard requirement vs nice-to-have
    Timeout     time.Duration // Max wait time (default 60s)
}

type FileConstraints struct {
    MustExist     []string // Files that must exist after completion
    MustNotExist  []string // Files that must NOT exist
    MustNotChange []string // Files that must NOT be modified
}
```

**Expectation Formats:**
- `exit 0` - Command must exit with code 0
- `exit N` - Command must exit with specific code N
- `output contains X` - stdout must contain substring X

**Contract Generation:**

The `verification.Generator` uses Claude (via `PromptRunner` interface) to generate concrete verification commands based on:
1. Task intent (from decomposition)
2. Files modified during implementation
3. Project context (Go, Node, Rust, Python)

```go
type PromptRunner interface {
    RunPrompt(ctx context.Context, prompt string, workDir string) (string, error)
}

type Generator struct {
    workDir      string
    promptRunner PromptRunner
}
```

**Project Type Detection:**

```go
func DetectProjectType(workDir string) ProjectType {
    if fileExists(workDir, "go.mod") { return ProjectTypeGo }
    if fileExists(workDir, "package.json") { return ProjectTypeNode }
    if fileExists(workDir, "Cargo.toml") { return ProjectTypeRust }
    if fileExists(workDir, "pyproject.toml") { return ProjectTypePython }
    return ProjectTypeUnknown
}
```

**Contract Execution:**

The `ContractRunner` executes verification contracts:
1. Run each verification command
2. Check exit codes and output expectations
3. Verify file constraints (must_exist, must_not_exist)
4. Generate summary of results

**Integration with Ralph-Loop:**

When verification is enabled:
1. After initial implementation, generate verification contract
2. Before each critique iteration, run verification
3. If verification fails, inject failure context into agent's output
4. Agent sees what specifically failed and can fix it
5. Only exit when verification passes OR max iterations reached

**Clean Abort on Failure:**

When max iterations reached without passing verification:
```go
if result.Success &&
   strings.Contains(result.RalphLoopExitReason, "max_iterations_reached") &&
   !result.VerificationPassed {
    // Don't merge - task failed to meet requirements
    task.Status = models.TaskStatusFailed
    task.Error = fmt.Sprintf("Task aborted: max iterations reached (%d) without passing verification. %s",
        result.RalphLoopIterations, result.VerificationSummary)
    // Emit failure event, don't merge to session branch
}
```

**Package Location:** `internal/verification/`
- `contract.go` - Types and ContractRunner
- `generator.go` - Generator with PromptRunner interface

---

## 9. Budget Management

**Two-Tier Tracking:**
- **Hard budget:** From `message_delta.usage` events when available
- **Soft budget:** Estimate when events missing, show "confidence: low"
- **Rate limit cap:** Concurrency also capped by API rate limits, not just tier

```go
type Budget struct {
    HardTokens     int     // From API usage events
    SoftTokens     int     // Estimated when no events
    Confidence     float64 // 0.0-1.0, shown in TUI
    RateLimitSlots int     // Available API slots
}
```

**Thresholds:**
- On budget 80%: warning in TUI
- On budget 100%: complete running agents, block new tasks
- Graceful wind-down, no hard kills

---

## 10. Human Review & Approval

**Review Gate:** Architect tier only
- When task completes quality gates → show diff in TUI
- User must approve (`y`) or reject (`n`)
- Rejected → task goes back to agent for retry

**Sampled Second Reviewer:** For Builder/Architect tiers, sample second agent when:
- Diff touches protected areas (auth, migrations, infra)
- Diff is large (>200 lines)
- Tests are weak/absent for touched code
- Changes are cross-cutting (>3 packages)

Not every time - only high-risk diffs.

**Approval Snapshot Binding:** Approval binds to:
- Base commit hash
- Diff summary hash (SHA of the diff content)
- Task ID

If ANY of these change after approval, approval expires. Must re-approve.

```go
type Approval struct {
    TaskID       string
    BaseCommit   string
    DiffHash     string
    ApprovedAt   time.Time
    ApprovedBy   string // "user" or "auto"
}

func (a *Approval) IsValid(currentBase, currentDiff string) bool {
    return a.BaseCommit == currentBase && a.DiffHash == currentDiff
}
```

---

## Tier Configuration

### Scout Tier
```yaml
tier: scout
max_agents: 2
primary_model: haiku
quality_threshold: 5
max_ralph_iterations: 3
questions_allowed: 0  # with override gates
```

### Builder Tier
```yaml
tier: builder
max_agents: 3
primary_model: sonnet
quality_threshold: 7
max_ralph_iterations: 5
questions_allowed: 2
```

### Architect Tier
```yaml
tier: architect
max_agents: 5
primary_model: opus
fallback_model: sonnet
quality_threshold: 8
max_ralph_iterations: 7
questions_allowed: unlimited
```

**Auto-Tier Selection:**

Keywords are defined in a single source of truth (`internal/orchestrator/tier_keywords.go`):

| Tier | Keywords |
|------|----------|
| Quick | typo, rename, fix typo, formatting, comment |
| Scout | find, search, list, check, where, what, show, count, look, scan, locate, which, docs, readme, documentation |
| Architect | refactor, redesign, migrate, rewrite, overhaul, restructure, auth, authentication, security, infra, schema, database |
| Builder | Everything else (default) |

User can always override with `--tier`.

**Confidence Scoring:** Auto-tier selection includes a confidence score (0.0-1.0). If confidence is low, the system defaults to Builder tier and logs the uncertainty.

---

## System Prompts

### Semantic Merge Agent

```
You are a merge conflict resolver. You will receive two diffs that conflict.

Your job:
1. Understand the INTENT of each change (not just the text)
2. Determine if intents are compatible or contradictory
3. If compatible: produce merged code that satisfies both intents
4. If contradictory: explain the conflict and recommend which to keep

Output format:
- MERGED CODE block if resolvable
- CONFLICT EXPLANATION if not resolvable

Never lose functionality from either side unless explicitly contradictory.
```

### Self-Critique Prompt (Ralph-Loop)

```
Review your implementation. Score each criterion 1-3:

CORRECTNESS (1-3):
- Does it work for the happy path?
- Does it handle edge cases?
- Are there obvious bugs?

READABILITY (1-3):
- Is the code clear without comments?
- Are names descriptive?
- Is complexity appropriate?

EDGE CASES (1-3):
- Are errors handled?
- Are nulls/empty states handled?
- Are boundaries checked?

Total: X/9

If below threshold, list specific improvements and implement them.
If at/above threshold, output DONE.
```

### Scope Guidance Prompt

```
Stay focused on this task. If you discover refactoring opportunities
or unrelated improvements, note them as new tasks but do not implement
them in this session.
```

---

# Part 2: Implementation Plan

## Technology Stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Language | Go | Fast, single binary, great concurrency |
| TUI | Bubbletea + Lipgloss | Composable, testable, modern |
| Agent execution | Claude Code subprocess | Proven tooling, file editing built-in |
| Claude output | JSON mode (`--output-format stream-json`) | Structured, parseable |
| Config | XDG + env override | Standard Unix pattern |
| State | SQLite (global + project) | Simple, embedded, crash-safe |
| Prog integration | Embedded as Go library (vendored) | Type-safe, fast |
| Local LLM | TODO (defer) | Focus on Anthropic first |

---

## CLI Commands

```bash
alphie run <task> [--tier scout|builder|architect] [--greenfield]
alphie status                    # Current session state
alphie config [key] [value]      # Manage configuration
alphie learn [query]             # Search/add learnings
alphie cleanup                   # Remove orphaned worktrees
alphie baseline                  # Show/reset baseline snapshot
```

**Flags:**
- `--tier`: Override auto-tier selection
- `--greenfield`: Direct merge to main (skip session branch + PR)

---

## Project Structure

```
alphie/
├── cmd/
│   └── alphie/
│       └── main.go              # Entry point
├── internal/
│   ├── agent/
│   │   ├── agent.go             # Agent struct and lifecycle
│   │   ├── claude.go            # Claude Code subprocess wrapper
│   │   ├── worktree.go          # Git worktree management
│   │   ├── ralph_loop.go        # Ralph-loop implementation
│   │   ├── executor.go          # Task executor with verification
│   │   ├── prompt_runner.go     # ClaudePromptRunner adapter
│   │   ├── gates.go             # Quality gate runners
│   │   ├── baseline.go          # Baseline capture for regressions
│   │   └── testselect.go        # Focused test selection
│   ├── orchestrator/
│   │   ├── orchestrator.go      # Main coordination logic
│   │   ├── decomposer.go        # Task decomposition (generates VerificationIntent)
│   │   ├── scheduler.go         # Parallel task scheduling (marks dependents blocked)
│   │   ├── tier_keywords.go     # Single source of truth for tier classification
│   │   ├── tier_selector.go     # Auto-tier selection with confidence scoring
│   │   ├── merger.go            # Merge conflict handling
│   │   ├── semantic.go          # Semantic merge agent
│   │   ├── collision.go         # Collision detection
│   │   └── pkgmerge.go          # Package file merging
│   ├── verification/
│   │   ├── contract.go          # Verification types and runner
│   │   ├── generator.go         # Contract generation (DraftContract, RefineContract)
│   │   └── storage.go           # Contract persistence to .alphie/contracts/
│   ├── tui/
│   │   ├── app.go               # Bubbletea main model
│   │   ├── tabs.go              # Tab navigation
│   │   ├── agents.go            # Agent status grid view
│   │   ├── output.go            # Live output stream view
│   │   ├── graph.go             # Dependency graph view
│   │   └── stats.go             # Token/cost tracker view
│   ├── config/
│   │   ├── config.go            # Config loading/saving
│   │   └── keys.go              # API key management
│   ├── state/
│   │   ├── db.go                # SQLite operations
│   │   ├── session.go           # Session state
│   │   └── recovery.go          # Crash recovery
│   ├── learning/
│   │   ├── cao.go               # CAO triple parser
│   │   ├── store.go             # Learning storage
│   │   ├── retrieval.go         # Learning retrieval
│   │   └── lifecycle.go         # Learning decay/TTL
│   ├── architect/               # Architecture implementation mode
│   │   ├── controller.go        # Main implementation controller
│   │   ├── auditor.go           # Architecture compliance auditor
│   │   ├── planner.go           # Task planning from spec
│   │   └── stopper.go           # Convergence detection
│   └── prog/
│       └── embed.go             # Embedded prog functionality
├── pkg/
│   └── models/
│       ├── task.go              # Task data model (with VerificationIntent)
│       ├── agent.go             # Agent data model
│       └── tier.go              # Tier configuration
└── configs/
    ├── scout.yaml
    ├── builder.yaml
    └── architect.yaml
```

---

## Core Data Models

### Agent State
```go
type Agent struct {
    ID           string
    TaskID       string
    Status       AgentStatus  // pending, running, paused, done, failed
    WorktreePath string
    PID          int          // Claude Code process ID
    StartedAt    time.Time
    TokensUsed   int
    Cost         float64
    RalphIter    int          // Current ralph-loop iteration
    RalphScore   RubricScore
}

type AgentStatus int
const (
    AgentPending AgentStatus = iota
    AgentRunning
    AgentPaused
    AgentWaitingApproval  // Has question for user
    AgentDone
    AgentFailed
)
```

### Task State
```go
type Task struct {
    ID                 string
    ParentID           string        // Epic ID if subtask
    Title              string
    Description        string
    AcceptanceCriteria string        // Human-readable acceptance criteria
    VerificationIntent string        // Intent for verification contract generation
    Status             TaskStatus    // pending, in_progress, blocked, done, failed
    DependsOn          []string      // Task IDs this blocks on
    AssignedTo         string        // Agent ID
    Tier               Tier
    TaskType           TaskType      // SETUP, FEATURE, BUGFIX, REFACTOR
    FileBoundaries     []string      // Expected files to modify (collision detection)
    CreatedAt          time.Time
    CompletedAt        *time.Time
    Error              string        // Error message if failed
    BlockedReason      string        // Why blocked: "dependency_failed:<task-id>" or "orphaned_by_crash"
    ExecutionCount     int           // Total executions across all sessions (persisted)
}
```

### Task Status Transitions

```
pending ──────────► in_progress ──────────► done
    │                    │
    │                    ▼
    │               verifying ────────► failed
    │                    │                │
    │                    │                ▼
    └──────────────► blocked ◄────────────┘
                   (dependency_failed)
```

**BlockedReason values:**
- `dependency_failed:<task-id>` - Parent task failed, this task cannot proceed
- `orphaned_by_crash` - Task was in_progress when system crashed

### Counter Definitions

Alphie uses several distinct counters. Each has a specific scope and purpose:

| Counter | Scope | Purpose | Persisted |
|---------|-------|---------|-----------|
| `RalphIteration` | Per-execution | Self-critique loop step (0-N) | No |
| `StartupRetry` | Per-execution | Claude CLI hang recovery (0-2) | No |
| `Task.ExecutionCount` | Cross-session | Total times task has executed | Yes |

**RalphIteration:** Counts iterations within a single Ralph-loop execution. Resets to 0 each time the task runs. Used to enforce max iterations per tier.

**StartupRetry:** Internal to executor. Handles Claude CLI startup failures (hangs, crashes). Hidden from users. Maximum 2 retries before marking agent failed.

**Task.ExecutionCount:** Persisted counter that survives session recovery. Incremented each time a task fails and is retried. Used for:
- Scout override unlocking (at 5 attempts, Scout can ask questions)
- Tracking task difficulty
- Debugging stuck tasks

### Session State
```go
type Session struct {
    ID            string
    RootTask      string      // Original user request
    Tier          Tier
    TokenBudget   int
    TokensUsed    int
    Agents        []Agent
    Tasks         []Task
    StartedAt     time.Time
    Status        SessionStatus
}
```

---

## TUI Design

### Tab 1: Agent Grid
```
┌─ Agents ───────────────────────────────────┐
│ [●] agent-a1b2  RUNNING   "Add auth"   2m  │
│ [●] agent-c3d4  RUNNING   "Add tests"  1m  │
│ [◐] agent-e5f6  WAITING   "Add docs"   --  │
│ [✓] agent-g7h8  DONE      "Fix bug"    3m  │
│ [✗] agent-i9j0  FAILED    "Add API"    5m  │
│ [?] agent-k1l2  QUESTION  "Clarify X"  --  │
└────────────────────────────────────────────┘
Keys: [space] pause  [k] kill  [enter] focus
```

### Tab 2: Live Output
```
┌─ Output: agent-a1b2 ───────────────────────┐
│ Reading file src/auth/handler.go...        │
│ Found existing auth middleware             │
│ Creating new JWT validation function       │
│ Writing to src/auth/jwt.go                 │
│ Running tests...                           │
│ ████████████░░░░░░░░ 60%                   │
└────────────────────────────────────────────┘
Keys: [↑↓] scroll  [1-9] switch agent
```

### Tab 3: Dependency Graph
```
┌─ Task Graph ───────────────────────────────┐
│                                            │
│  [Epic: Auth System]                       │
│      ├── [✓] Setup middleware              │
│      ├── [●] Add JWT validation ←──┐       │
│      ├── [●] Add session store     │       │
│      └── [◐] Integration tests ────┘       │
│                                            │
└────────────────────────────────────────────┘
Keys: [enter] show task details
```

### Tab 4: Stats
```
┌─ Session Stats ────────────────────────────┐
│ Tier:        Builder                       │
│ Duration:    12m 34s                       │
│ Agents:      3 running / 5 total           │
│ Tasks:       4 done / 7 total              │
│                                            │
│ Tokens:      45,230 / 100,000 (45%)        │
│ Cost:        $0.68 / $1.50 budget          │
│ ████████████████░░░░░░░░░░░░░░░ 45%        │
└────────────────────────────────────────────┘
```

### TUI Interactions
- **Pause/resume agents:** Spacebar
- **Kill specific agent:** Select + 'k'
- **Approve queued questions:** Inline answer
- **Manual merge trigger:** Select + 'm'

---

## Key Behaviors

### Agent Lifecycle
```
1. Task assigned → create worktree
2. Start Claude Code subprocess in worktree
3. Stream JSON output → update TUI
4. Ralph-loop: critique → improve → repeat
5. Quality gates: test, build, lint, typecheck
6. On pass → merge to session branch → cleanup worktree
7. Session complete → PR to main/master/dev (or fast-forward if --greenfield)
8. On fail → retry (up to 5) → escalate to human
```

### Timeout Handling
- Soft timeout per tier (Scout: 5m, Builder: 15m, Architect: 30m)
- On timeout: prompt user "Agent X taking long, kill or wait?"
- User decides: kill (fail task) or wait (extend timeout)

### Crash Recovery
- State persisted to SQLite on every change
- On startup: check for orphaned worktrees/processes
- Resume from last known state
- Prompt user: "Found interrupted session. Resume or clean?"

### Merge Conflict Handling
1. Agent finishes → attempt merge to session branch
2. If conflict → rebase agent branch on session branch
3. Re-run agent verification in rebased worktree
4. If still conflicts → spawn semantic merge agent
5. If unresolvable → escalate to human
6. Session complete → PR session branch to main

---

## Config File

Location: `~/.config/alphie/config.yaml`

```yaml
anthropic:
  api_key: ${ANTHROPIC_API_KEY}  # Env var reference

defaults:
  tier: builder
  token_budget: 100000

tui:
  refresh_rate: 100ms

timeouts:
  scout: 5m
  builder: 15m
  architect: 30m

quality_gates:
  test: true
  build: true
  lint: true
  typecheck: true
```

Project override: `.alphie.yaml` in project root (no secrets here)

---

## State Storage

### Global: `~/.local/share/alphie/alphie.db`
- Learnings (CAO triples)
- Cross-project settings
- Usage history

### Project: `.alphie/state.db`
- Current session state
- Agent states
- Task states
- Recovery checkpoints

---

## Footguns & Mitigations

| Footgun | Mitigation |
|---------|------------|
| Claude hangs | Soft timeout + user prompt |
| Alphie crashes | Persist state continuously, resume on restart |
| API rate limits | Exponential backoff + jitter, concurrency capped by rate limit slots |
| Merge storms | Session branch + PR, never direct to main/master/dev |
| Half-baked to main | Session branch gates, approval binding to snapshot |
| Semantic merge lies | Strict conditions: disjoint paths OR different funcs OR tests pass |
| Regression masked | Baseline capture at session start, no new/worse failures |
| Garbage passes gates | Human review for Architect + sampled second reviewer for risky diffs |
| Budget overrun | Graceful wind-down, two-tier tracking with confidence indicator |
| Orphaned worktrees | Startup detection + cleanup command |
| Token tracking drift | Two-tier: hard (API events) + soft (estimates with confidence) |
| Concurrent SQLite | Single writer, WAL mode |
| Approval drift | Approval binds to base commit + diff hash + task ID |
| Scope creep | Soft prompt guidance to stay focused, file new tasks for discoveries |
| Self-review bias | Sample second agent for high-risk diffs |
| Wrong tier picked | Auto-tier selection with user override |
| Scout silent wrongness | Override gates: can ask when blocked or protected area |

---

## Implementation Phases

### Phase 1: Foundation
- [ ] Project scaffolding (go mod, structure)
- [ ] Config loading (viper, XDG paths)
- [ ] SQLite state management
- [ ] Basic CLI with cobra

### Phase 2: Agent Core
- [ ] Claude Code subprocess wrapper
- [ ] JSON output parsing
- [ ] Git worktree management
- [ ] Single-agent execution (no parallelism yet)

### Phase 3: TUI
- [ ] Bubbletea app shell
- [ ] Tab navigation
- [ ] Agent grid view
- [ ] Stats view (tokens/cost)

### Phase 4: Orchestration
- [ ] Task decomposition (via Claude)
- [ ] Dependency graph
- [ ] Parallel agent scheduling
- [ ] Merge handling

### Phase 5: Ralph-Loop
- [ ] Self-critique prompt injection
- [ ] Rubric scoring parser
- [ ] Iteration control
- [ ] Quality gates integration

### Phase 6: Learning System
- [ ] CAO triple parser
- [ ] Learning storage
- [ ] Pre-task retrieval
- [ ] `alphie learn` command

### Phase 7: Polish
- [ ] Crash recovery
- [ ] Human review gate
- [ ] Live output streaming
- [ ] Dependency graph visualization

### Phase 8: Local LLM (TODO)
- [ ] LM Studio integration
- [ ] Provider abstraction
- [ ] Model routing config

---

## Key Dependencies

```go
// go.mod
require (
    github.com/charmbracelet/bubbletea v0.25+
    github.com/charmbracelet/lipgloss v0.9+
    github.com/charmbracelet/bubbles v0.17+
    github.com/spf13/cobra v1.8+
    github.com/spf13/viper v1.18+
    github.com/mattn/go-sqlite3 v1.14+
    github.com/anthropics/anthropic-sdk-go v0.1+
)
```

---

## Verification

After each phase:
1. Run `go build ./...` - must compile
2. Run `go test ./...` - tests pass
3. Manual test of new commands
4. TUI renders correctly in terminal

End-to-end test:
```bash
# Create test repo
mkdir /tmp/test-alphie && cd /tmp/test-alphie
git init && echo "package main" > main.go

# Run alphie
alphie run "Add a hello world function" --tier scout

# Verify
# - TUI shows agent progress
# - Worktree created/cleaned
# - main.go has hello world function
# - No orphaned processes
```

---

# Part 3: Deep Dives

## Deep Dive: Prog Embedding

### Prog Overview
- **Language:** Go (same as Alphie - easy embedding)
- **Database:** SQLite via `modernc.org/sqlite`
- **CLI:** Cobra (same as Alphie will use)
- **Location:** `~/.prog/prog.db`

### Prog Internal Structure
```
prog/
├── cmd/prog/main.go           # CLI commands (we don't need this)
├── internal/
│   ├── db/                    # THIS IS WHAT WE EMBED
│   │   ├── db.go              # Schema, migrations, Open()
│   │   ├── items.go           # Task/epic CRUD
│   │   ├── deps.go            # Dependency management
│   │   ├── learnings.go       # CAO storage
│   │   ├── logs.go            # Audit trail
│   │   ├── labels.go          # Tagging
│   │   └── projects.go        # Project scoping
│   ├── model/
│   │   └── item.go            # Item, Status, ItemType
│   └── tui/                   # Their TUI (we have our own)
```

### Embedding Strategy

**Recommended: Vendor the code**
1. Copy `internal/db/*.go` into `alphie/internal/prog/`
2. Adjust package names
3. Full control, no external dependency

Benefits:
- No fork maintenance
- Can customize for Alphie's needs
- Clean separation

### Core Functions to Vendor

```go
// Database operations
func Open(path string) (*DB, error)
func (db *DB) Migrate() error
func (db *DB) Close() error

// Items (Tasks/Epics)
func (db *DB) CreateItem(title string, itemType ItemType, project string) (*Item, error)
func (db *DB) GetItem(id string) (*Item, error)
func (db *DB) UpdateStatus(id string, status Status) error
func (db *DB) ListItemsFiltered(filter ItemFilter) ([]Item, error)
func (db *DB) ReadyItemsFiltered(filter ItemFilter) ([]Item, error)

// Dependencies
func (db *DB) AddDep(itemID, blockedByID string) error
func (db *DB) HasUnmetDeps(id string) (bool, error)
func (db *DB) GetDeps(id string) ([]string, error)

// Learnings (CAO triples)
func (db *DB) CreateLearning(title, details string, concepts, files []string) (*Learning, error)
func (db *DB) SearchLearnings(query string) ([]Learning, error)
func (db *DB) GetRelatedConcepts(itemID string) ([]Concept, error)

// Logs
func (db *DB) AddLog(itemID, message string) error
func (db *DB) GetLogs(itemID string) ([]Log, error)
```

### SQLite Schema (from prog)

```sql
-- Items table (tasks and epics)
CREATE TABLE items (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT,
    type TEXT NOT NULL,        -- 'task' or 'epic'
    status TEXT NOT NULL,      -- 'open', 'in_progress', 'blocked', 'done', 'canceled'
    priority INTEGER DEFAULT 2,
    parent_id TEXT,            -- Epic ID for subtasks
    project TEXT,
    created_at DATETIME,
    updated_at DATETIME
);

-- Dependencies
CREATE TABLE deps (
    item_id TEXT,
    blocked_by_id TEXT,
    PRIMARY KEY (item_id, blocked_by_id)
);

-- Learnings (CAO triples go here)
CREATE TABLE learnings (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,       -- The CAO triple
    details TEXT,              -- Extended explanation
    status TEXT DEFAULT 'active',
    created_at DATETIME
);

-- Full-text search on learnings
CREATE VIRTUAL TABLE learnings_fts USING fts5(title, details);

-- Concepts (learning categories)
CREATE TABLE concepts (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    project TEXT,
    summary TEXT
);

-- Learning-concept junction
CREATE TABLE learning_concepts (
    learning_id TEXT,
    concept_id TEXT,
    PRIMARY KEY (learning_id, concept_id)
);
```

---

## Deep Dive: Claude Code JSON Output

### Output Format Options

```bash
claude --output-format text         # Default human-readable
claude --output-format json         # Single JSON response
claude --output-format stream-json  # NDJSON streaming (what we want)
```

### Stream-JSON Event Types

Each line is a complete JSON object. Event types:

| Event | Purpose |
|-------|---------|
| `message_start` | Initializes stream, contains session ID |
| `content_block_start` | New content block begins |
| `content_block_delta` | Incremental update to block |
| `content_block_stop` | Block complete |
| `message_delta` | Message-level changes (stop_reason, usage) |
| `message_stop` | Stream complete |
| `ping` | Keep-alive |

### Content Block Types

| Type | Description |
|------|-------------|
| `text` | Normal text output |
| `tool_use` | Agent calling a tool (file read, edit, bash) |
| `thinking` | Extended thinking content |
| `server_tool_use` | Server-side tool (web search) |

### Key JSON Structures

**Message Start:**
```json
{
  "type": "message_start",
  "message": {
    "id": "msg_xxx",
    "content": [],
    "usage": {"input_tokens": 100, "output_tokens": 0}
  }
}
```

**Tool Use (file edit, bash, etc):**
```json
{
  "type": "content_block_start",
  "index": 1,
  "content_block": {
    "type": "tool_use",
    "id": "toolu_xxx",
    "name": "Edit",
    "input": {}
  }
}
```

**Tool Input Streaming:**
```json
{
  "type": "content_block_delta",
  "index": 1,
  "delta": {
    "type": "input_json_delta",
    "partial_json": "{\"file_path\": \"/src/main.go\", \"old_string\": \"...\"}"
  }
}
```

**Text Output:**
```json
{
  "type": "content_block_delta",
  "index": 0,
  "delta": {
    "type": "text_delta",
    "text": "I'll fix the authentication bug..."
  }
}
```

**Usage/Tokens (cumulative):**
```json
{
  "type": "message_delta",
  "delta": {"stop_reason": "end_turn"},
  "usage": {"input_tokens": 1500, "output_tokens": 450}
}
```

### Go Parser Strategy

```go
type StreamEvent struct {
    Type    string          `json:"type"`
    Index   int             `json:"index,omitempty"`
    Message *Message        `json:"message,omitempty"`
    Delta   *Delta          `json:"delta,omitempty"`
    ContentBlock *ContentBlock `json:"content_block,omitempty"`
}

type Delta struct {
    Type        string `json:"type"`
    Text        string `json:"text,omitempty"`
    PartialJSON string `json:"partial_json,omitempty"`
    StopReason  string `json:"stop_reason,omitempty"`
}

type ContentBlock struct {
    Type  string `json:"type"`
    ID    string `json:"id,omitempty"`
    Name  string `json:"name,omitempty"`  // Tool name
    Input any    `json:"input,omitempty"`
}

// Parse NDJSON stream
func ParseStream(r io.Reader) <-chan StreamEvent {
    ch := make(chan StreamEvent)
    go func() {
        scanner := bufio.NewScanner(r)
        for scanner.Scan() {
            var event StreamEvent
            json.Unmarshal(scanner.Bytes(), &event)
            ch <- event
        }
        close(ch)
    }()
    return ch
}
```

### Token Tracking

```go
type TokenTracker struct {
    InputTokens  int
    OutputTokens int
    mu           sync.Mutex
}

func (t *TokenTracker) Update(event StreamEvent) {
    if event.Type == "message_delta" && event.Delta != nil {
        t.mu.Lock()
        // Usage in message_delta is cumulative
        // Just take the latest value
        t.mu.Unlock()
    }
}
```

---

## Deep Dive: Git Worktree Management

### Worktree Commands for Alphie

```bash
# CREATE (per agent)
git worktree add ~/.cache/alphie/worktrees/agent-{uuid} -b agent-{uuid}

# REMOVE (on completion)
git worktree remove ~/.cache/alphie/worktrees/agent-{uuid}

# FORCE REMOVE (on failure/crash)
git worktree remove -f ~/.cache/alphie/worktrees/agent-{uuid}

# CLEANUP ORPHANS (on startup)
git worktree prune --expire now

# LIST (for status)
git worktree list --porcelain
```

### Worktree Lifecycle in Alphie

```go
type Worktree struct {
    Path       string
    BranchName string
    AgentID    string
    CreatedAt  time.Time
}

func CreateWorktree(agentID string) (*Worktree, error) {
    base := os.ExpandEnv("$HOME/.cache/alphie/worktrees")
    path := filepath.Join(base, fmt.Sprintf("agent-%s", agentID))
    branch := fmt.Sprintf("agent-%s", agentID)

    cmd := exec.Command("git", "worktree", "add", path, "-b", branch)
    if err := cmd.Run(); err != nil {
        return nil, fmt.Errorf("create worktree: %w", err)
    }

    return &Worktree{
        Path:       path,
        BranchName: branch,
        AgentID:    agentID,
        CreatedAt:  time.Now(),
    }, nil
}

func (w *Worktree) Remove(force bool) error {
    args := []string{"worktree", "remove", w.Path}
    if force {
        args = append(args, "-f")
    }
    return exec.Command("git", args...).Run()
}

func (w *Worktree) Merge(sessionBranch string) error {
    // Merge agent branch into session branch (not main)
    mainDir := getMainWorktree()

    // Checkout session branch first
    exec.Command("git", "checkout", sessionBranch).Run()

    // Merge agent branch into session branch
    cmd := exec.Command("git", "merge", w.BranchName, "--no-ff",
        "-m", fmt.Sprintf("Merge agent %s work", w.AgentID))
    cmd.Dir = mainDir
    return cmd.Run()
}
```

### Edge Cases & Mitigations

| Edge Case | Detection | Mitigation |
|-----------|-----------|------------|
| Orphaned worktree (crash) | `git worktree list` on startup | `git worktree prune --expire now` |
| Merge conflict | Non-zero exit from `git merge` | Rebase agent branch, re-run verification |
| Main branch moved | N/A (shared .git) | Agent sees new commits automatically |
| Worktree locked | `git worktree list -v` shows `(locked)` | `git worktree unlock` then remove |
| Dirty worktree | Non-zero exit from `remove` | Force remove with `-f` |
| Can't cd into worktree | Directory deleted externally | `git worktree prune` |

### Startup Recovery

```go
func RecoverOrphanedWorktrees() error {
    // 1. List all worktrees
    cmd := exec.Command("git", "worktree", "list", "--porcelain")
    output, _ := cmd.Output()

    // 2. Find alphie-agent-* worktrees
    for _, wt := range parseWorktreeList(output) {
        if strings.Contains(wt.Path, "alphie") {
            // 3. Check if we have session record
            if !sessionExists(wt.AgentID) {
                // Orphaned - clean up
                exec.Command("git", "worktree", "remove", "-f", wt.Path).Run()
            }
        }
    }

    // 4. Final prune
    return exec.Command("git", "worktree", "prune", "--expire", "now").Run()
}
```

### Merge Workflow

```go
func MergeAgentWork(w *Worktree, sessionBranch string) error {
    mainDir := getMainWorktree()

    // Ensure we're on session branch
    exec.Command("git", "checkout", sessionBranch).Run()

    // 1. Attempt merge into session branch
    cmd := exec.Command("git", "merge", w.BranchName, "--no-ff")
    cmd.Dir = mainDir
    if err := cmd.Run(); err == nil {
        return nil // Success
    }

    // 2. Conflict - abort merge
    exec.Command("git", "merge", "--abort").Run()

    // 3. Rebase agent branch on session branch
    rebaseCmd := exec.Command("git", "rebase", sessionBranch)
    rebaseCmd.Dir = w.Path
    if err := rebaseCmd.Run(); err != nil {
        // Rebase failed - spawn semantic merge agent
        return spawnMergeAgent(w, sessionBranch)
    }

    // 4. Re-run quality gates in rebased worktree
    if err := runQualityGates(w.Path); err != nil {
        return err
    }

    // 5. Try merge again into session branch
    cmd = exec.Command("git", "merge", w.BranchName, "--no-ff")
    cmd.Dir = mainDir
    return cmd.Run()
}

// After all agents complete, create PR to main
func FinalizeSession(sessionBranch string, greenfield bool) error {
    if greenfield {
        // Fast-forward main to session branch
        exec.Command("git", "checkout", "main").Run()
        return exec.Command("git", "merge", sessionBranch, "--ff-only").Run()
    }
    // Create PR via gh cli
    return exec.Command("gh", "pr", "create",
        "--base", "main",
        "--head", sessionBranch,
        "--title", fmt.Sprintf("Session %s", sessionBranch)).Run()
}
```

### Gotchas to Handle

```go
// GOTCHA 1: Can't remove while process is cd'd into it
// Solution: Always run Claude Code with explicit --cwd flag

// GOTCHA 2: git worktree remove fails silently if path doesn't exist
// Solution: Check existence first, or use prune

// GOTCHA 3: Branch name conflicts if agent ID reused
// Solution: Include timestamp or ensure unique UUIDs

// GOTCHA 4: Worktree in /tmp may be cleaned by OS
// Solution: Use ~/.cache/alphie/worktrees/ instead

// GOTCHA 5: Large repos = slow worktree creation
// Solution: Consider shallow worktrees for massive repos
```

### Recommended Worktree Location

```go
const WorktreeBaseDir = "$HOME/.cache/alphie/worktrees"

func WorktreePath(agentID string) string {
    return filepath.Join(
        os.ExpandEnv(WorktreeBaseDir),
        fmt.Sprintf("agent-%s", agentID),
    )
}
```

Using `~/.cache` instead of `/tmp`:
- Survives reboots (for crash recovery)
- User-owned (no permission issues)
- XDG-compliant location

---

# Part 4: Integration with Prog CLI

Alphie uses prog for all task and learning management.

## Task Lifecycle

```bash
prog add "task title" -p project    # Create
prog start <id>                      # Claim
prog log <id> "progress update"      # Update
prog done <id>                       # Complete
prog block <id> "reason"             # Block
```

## Parallel Execution Setup

```bash
prog add "Epic: Feature X" -e                    # Create epic
prog add "Subtask 1" --parent <epic-id>          # Add subtasks
prog add "Subtask 2" --parent <epic-id>
prog add "Subtask 3" --parent <epic-id> --blocks <subtask-4-id>  # With dependency
prog ready                                        # See parallelizable work
```

## Learning Integration

```bash
# Store in CAO format
prog learn "WHEN X DO Y RESULT Z" -c concept

# Retrieve before task
prog context -c concept

# Check existing concepts
prog concepts
```

---

# Appendix: Design Decisions

This section documents key architectural decisions and their rationale.

## Design Decision Summary

| Question | Decision |
|----------|----------|
| Agent execution | Claude Code subprocess |
| TUI framework | Bubbletea |
| Config location | XDG + env override |
| Prog integration | Embedded Go library (vendored) |
| Process timeout | Soft timeout + user prompt |
| Crash recovery | Persist state continuously |
| Rate limits | Backoff + jitter |
| Merge conflicts | Rebase + retry, semantic merge with strict conditions |
| Quality assurance | Human review for Architect + sampled second reviewer |
| Budget overrun | Graceful wind-down |
| TUI layout | Tabbed views |
| TUI controls | Pause, kill, approve, merge |
| Local LLM | Deferred to Phase 8 |
| Worktree location | ~/.cache/alphie/worktrees |
| Scout questions | Override gates: blocked_after_n OR protected_area |
| Merge target | Session branch → PR (unless --greenfield) |
| Protected branches | main, master, dev never direct merge |
| Semantic merge | Strict conditions (disjoint/different funcs/tests pass) |
| Baseline | Capture at session start, enforce no regressions |
| Token tracking | Two-tier: hard (API) + soft (estimate) with confidence |
| Learning sync | Local-only for now, defer distributed |
| Learning model | Extended: evidence, scope, TTL, outcome type |
| Review | Sample second agent for high-risk diffs |
| Approval binding | Base commit + diff hash + task ID |
| Scope control | Soft guidance via prompt |
| Concurrency | Pure optimistic with worktrees |
| Tier selection | Auto-select with override |

## Architecture Gap Resolutions

During design review, 12 critical gaps were identified and resolved:

### 1. Scout Override Gates
**Problem:** "Zero questions" conflicts with discipline loop.
**Resolution:** Scout can ask when:
- `blocked_after_n_attempts` (5 retries exhausted)
- `protected_area_detected` (touching auth/migrations/infra)

Otherwise, infer and execute.

### 2. Session Integration Branch
**Problem:** Direct merge to main/dev causes merge storms.
**Resolution:**
- Default: Agents merge to `session-{id}` branch → PR to main/dev
- `--greenfield` flag: Direct merge to main/dev (new projects only)
- Protected branches: main, master, dev - never direct merge

```
agent branches → session-{id} → PR to main
                     ↓
              gates run here
              rollback = delete session branch
```

### 3. Strict Semantic Merge Conditions
**Problem:** Semantic merge assumes compatible intent.
**Resolution:** Semantic merge only allowed when:
- Changes are in disjoint file paths, OR
- Same file but different functions, OR
- Both sides pass targeted tests + full suite after merge

Otherwise: escalate to human immediately.

### 4. Baseline Capture
**Problem:** Ignoring pre-existing failures masks regressions.
**Resolution:** At session start:
1. Record which tests/lints currently fail (baseline snapshot)
2. Enforce during session:
   - No NEW failures allowed
   - No WORSENING existing failures (more failures = blocked)
   - Touch a component → its focused tests must pass

### 5. Two-Tier Budget System
**Problem:** Token tracking via subprocess may be unreliable.
**Resolution:**
- **Hard budget:** From `message_delta.usage` events when available
- **Soft budget:** Estimate when events missing, show "confidence: low"
- **Rate limit cap:** Concurrency also capped by API rate limits, not just tier

### 6. Local-Only Sync (Deferred)
**Problem:** SQLite sync needs real conflict resolution.
**Resolution:** Defer distributed learnings. For now:
- Each machine has its own `~/.local/share/alphie/alphie.db`
- No cross-machine sync
- Future: Server-authoritative API when needed

### 7. Enhanced Learning Model
**Problem:** CAO triples too vague, no evidence or decay.
**Resolution:** Extended schema with evidence (commit hash, log snippet), scope (repo/module/global), lifecycle (TTL, trigger count), and outcome type.

### 8. Sampled Second Reviewer
**Problem:** Self-critique is biased.
**Resolution:** For Builder/Architect tiers, sample second agent when:
- Diff touches protected areas (auth, migrations, infra)
- Diff is large (>200 lines)
- Tests are weak/absent for touched code
- Changes are cross-cutting (>3 packages)

Not every time - only high-risk diffs.

### 9. Approval Snapshot Binding
**Problem:** Approval can drift if code changes after review.
**Resolution:** Approval binds to base commit hash, diff summary hash, and task ID. If ANY change after approval, must re-approve.

### 10. Soft Scope Guidance
**Problem:** Agents might inflate scope with "play."
**Resolution:** Soft guidance via prompt (no hard enforcement). Agent prompted to stay focused; if refactor opportunity discovered, file new task instead of sneaking it in.

### 11. Pure Optimistic Concurrency
**Problem:** "No locking" may cause conflicts.
**Resolution:** Keep pure optimistic with worktrees. Accept that:
- Conflicts will happen
- Semantic merge agent handles them
- Rebase + re-verify is the recovery path

No ownership hints or file locking. Complexity not worth it.

### 12. Auto-Tier Selection
**Problem:** User might pick wrong tier.
**Resolution:** Auto-select based on task signals (docs/formatting → Scout, standard features → Builder, migrations/auth/infra → Architect). User can always override with `--tier`.
