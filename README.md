# Alphie

Parallel Claude Code agent orchestrator. Decomposes tasks, runs them in isolated git worktrees, and merges the results.

![Alphie TUI](docs/screenshot.png)

## Overview

Alphie spawns multiple Claude Code agents to work on your codebase in parallel. Each agent operates in an isolated git worktree, implements its assigned task through a self-improvement loop (Ralph-loop), and results are merged back via semantic merging that understands code intent.

**Key Features:**
- Parallel task execution with automatic decomposition
- Isolated git worktrees per agent (no conflicts during work)
- Self-improvement loops with quality scoring
- Semantic merge resolution for parallel changes
- Tiered execution (quick/scout/builder/architect)
- Real-time TUI dashboard
- Cross-session learning system
- Cost and token tracking

## Requirements

- Go 1.24+
- [Claude Code CLI](https://github.com/anthropics/claude-code) installed and authenticated
- Git

## Installation

```bash
go install github.com/ShayCichocki/alphie/cmd/alphie@latest
```

Or build from source:

```bash
git clone https://github.com/ShayCichocki/alphie
cd alphie
go build -o alphie ./cmd/alphie
```

## Quick Start

```bash
# Launch interactive mode - type tasks, watch them run
alphie

# Run a single task
alphie run "Add user authentication to the API"

# Use quick tier for simple fixes (single agent, fast)
alphie run "Fix typo in README" --tier quick

# Use scout tier for exploration
alphie run "Find all TODO comments" --tier scout

# Use architect tier for complex work
alphie run "Refactor the database layer" --tier architect

# Run without TUI (headless mode)
alphie run "Fix the login bug" --headless

# Greenfield mode - merges directly to main (for new projects)
alphie run "Set up project structure" --greenfield
```

## Commands

### Interactive Mode

Launch with no arguments for a persistent TUI. Type tasks, hit enter, watch them run in parallel.

```bash
alphie                    # Fresh start
alphie --resume           # Resume incomplete tasks from previous sessions
alphie --greenfield       # Enable greenfield mode for new projects
```

**Tier Detection:** Tasks are auto-classified based on keywords:
- **Quick:** "typo", "rename", "fix typo", "formatting", "comment"
- **Scout:** "find", "search", "list", "check", "where", "what", "show", "count", "look", "scan", "locate", "which", "docs", "readme", "documentation"
- **Architect:** "refactor", "redesign", "migrate", "rewrite", "overhaul", "restructure", "auth", "authentication", "security", "infra", "schema", "database"
- **Builder:** Everything else (default)

**Override with prefix:** `!quick fix typo`, `!scout find files`, `!builder add feature`, `!architect redesign API`

### run

Execute a task with parallel agents.

```bash
alphie run <task> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--tier` | Agent tier: `quick`, `scout`, `builder` (default), `architect` |
| `--headless` | Run without TUI, print to stdout |
| `--greenfield` | Merge directly to main (for new projects) |
| `--epic <id>` | Resume an existing epic from a previous session |
| `--quick` | Force quick mode (single agent, no decomposition) |
| `--parallel` | Force parallel mode (default for builder/architect) |
| `--single` | Force single-agent mode |

### implement

Iterate through an architecture spec until it's implemented.

```bash
alphie implement <arch.md> [flags]
```

Parses a markdown architecture doc, audits the codebase for gaps, plans tasks, and executes them in a loop until done.

**Flags:**
| Flag | Description |
|------|-------------|
| `--agents` | Max concurrent workers (default 3) |
| `--budget` | Cost limit in dollars |
| `--max-iterations` | Hard cap on iterations (default 10) |
| `--no-converge-after` | Stop if no progress for N iterations (default 3) |
| `--dry-run` | Show plan without executing |
| `--resume` | Resume from checkpoint |
| `--project` | Prog project name override |

### audit

Check codebase against an architecture spec.

```bash
alphie audit <arch.md> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--json` | Output structured JSON |

### learn

Manage learnings (condition-action-outcome triples).

```bash
alphie learn                              # List recent learnings
alphie learn "WHEN X DO Y RESULT Z"       # Add new learning
alphie learn --search "query"             # Search learnings
alphie learn --concept <name>             # Filter by concept
alphie learn show <id>                    # Show learning details
alphie learn --delete <id>                # Delete a learning
```

### status

Show current session state.

```bash
alphie status
```

### config

View or modify configuration.

```bash
alphie config                    # Show all settings
alphie config <key>              # Show specific value
alphie config <key> <value>      # Set value
```

### cleanup

Remove orphaned worktrees and old sessions.

```bash
alphie cleanup [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-f, --force` | Skip confirmation |
| `-v, --verbose` | Show each removal |
| `--dry-run` | Show what would be removed |
| `--sessions` | Purge sessions older than 30 days |

### baseline

Show or reset baseline snapshot of current test/lint status.

```bash
alphie baseline
```

## Tiers

| Tier | Agents | Model | Max Ralph Iterations | Use Case |
|------|--------|-------|----------------------|----------|
| **quick** | 1 | haiku | 3 | Simple tasks (typo fixes, renames, single-file edits) |
| **scout** | 2 | haiku | 3 | Quick exploration, research, finding code |
| **builder** | 3 | sonnet | 5 | Standard feature work, bug fixes, tests |
| **architect** | 5 | opus | 7 | Complex design, refactoring, migrations |

**Quality Thresholds:**
- Scout: 5/9 on rubric
- Builder: 7/9 on rubric
- Architect: 8/9 on rubric

## How It Works

### Execution Flow

```
Task Input
    │
    ▼
┌─────────────────┐
│ Task Decomposer │ ← Uses Claude to break into parallelizable subtasks
└─────────────────┘
    │
    ▼
┌─────────────────┐
│ Dependency Graph│ ← Creates execution order respecting dependencies
└─────────────────┘
    │
    ▼
┌─────────────────┐
│   Scheduler     │ ← Assigns tasks to agents, respects tier limits
└─────────────────┘
    │
    ▼
┌─────────────────────────────────────────────┐
│  Agent Worktrees (Parallel)                 │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐     │
│  │ Agent 1 │  │ Agent 2 │  │ Agent 3 │     │
│  │ (task A)│  │ (task B)│  │ (task C)│     │
│  └────┬────┘  └────┬────┘  └────┬────┘     │
│       │            │            │           │
│       ▼            ▼            ▼           │
│  ┌─────────────────────────────────────┐   │
│  │         Ralph-Loop (per agent)       │   │
│  │ Implement → Critique → Improve → ... │   │
│  └─────────────────────────────────────┘   │
└─────────────────────────────────────────────┘
    │
    ▼
┌─────────────────┐
│ Semantic Merger │ ← Merges all worktrees to session branch
└─────────────────┘
    │
    ▼
┌─────────────────┐
│  Session Branch │ ← Creates PR to main (or fast-forward if greenfield)
└─────────────────┘
```

### Ralph-Loop

Each agent runs a self-improvement cycle with verification-aware governance:

1. **Implement** - Execute the assigned task
2. **Verify** - Run verification commands (tests, file checks)
3. **Critique** - Self-evaluate against acceptance criteria
4. **Score** - Rate on rubric (Correctness, Readability, Edge Cases)
5. **Improve** - If verification fails or below threshold, inject context and refine

**Decision Matrix:**
| Verification | Score | Action |
|--------------|-------|--------|
| PASS | >= threshold | Exit success |
| PASS | >= threshold-1 | Exit acceptable |
| FAIL | any | Inject failure context, continue improving |
| DONE marker | PASS | Exit with "agent_done_verified" |
| DONE marker | FAIL | Continue improving (DONE is a request, not granted) |

**DONE Marker Validation:** When an agent outputs a "DONE" marker, verification still runs. The agent is requesting exit, not declaring it. Only if verification passes does the loop exit.

**Clean Abort:** If max iterations reached without passing verification, the task is marked failed and NOT merged. This prevents low-quality work from reaching the session branch.

### Verification System

Alphie uses verification contracts to ensure task completion matches intent:

**Three-Phase Verification (Gaming Prevention):**
1. **Intent Capture** (decomposition) - Human-readable acceptance criteria stored in `task.VerificationIntent`
2. **Draft Contract** (pre-implementation) - Generated BEFORE agent implements; establishes minimum requirements
3. **Refined Contract** (post-implementation) - Can only ADD checks, never weaken the draft

**Why Pre-Implementation Contracts:**
Generating contracts after seeing the implementation allows agents to create weak checks that rubber-stamp their work. Pre-implementation contracts set expectations based on intent, not outcomes.

**Verification Commands:**
```json
{
  "commands": [
    {
      "cmd": "go test ./src/auth/...",
      "expect": "exit 0",
      "description": "Auth tests pass",
      "required": true
    }
  ],
  "file_constraints": {
    "must_exist": ["src/auth/jwt.go"],
    "must_not_exist": [],
    "must_not_change": ["go.mod"]
  }
}
```

**Monotonic Strengthening:** The refined contract must include all draft constraints plus any new ones. This ensures verification can only become stricter, never weaker.

Verification is run before each critique iteration. If verification fails, the failure details are injected into the agent's context so it can fix specific issues.

### Semantic Merging

When parallel agents edit overlapping code, Alphie uses intelligent conflict resolution:

1. **Collision Detection** - Identifies when tasks might touch the same files
2. **Critical File Detection** - Recognizes package.json, go.mod, and other special files
3. **Smart Package Merging** - Unions dependencies, merges workspace arrays
4. **Semantic Analysis** - Understands intent, not just text diffs
5. **Validation** - Runs tests/build after merge to verify correctness

High-risk changes trigger a second review from another agent.

### Worktree Isolation

Each agent works in a completely isolated git worktree:
- Location: `~/.cache/alphie/worktrees/agent-{uuid}`
- No interference between parallel agents
- Clean merge back to session branch
- Automatic cleanup on completion

## TUI Dashboard

The interactive TUI displays four panels:

**Agent Cards** - Real-time status per agent:
- Current action (e.g., "Reading auth.go")
- Tokens used and cost
- Duration and status (running/done/failed)

**Tasks Panel** - Task dependency visualization:
- Status indicators (pending/running/done/failed)
- Parent-child relationships for epics
- Blocked vs ready tasks

**Logs Panel** - Streaming execution logs:
- Per-agent log streams
- Searchable and scrollable

**Stats Panel** - Session overview:
- Total token usage and cost
- Progress tracking
- Budget remaining

**Keyboard Controls:**
| Key | Action |
|-----|--------|
| `q` / `ctrl+c` | Quit |
| `tab` / `1,2,3` | Switch tabs |
| `space` | Pause/resume agents |
| `k` | Kill selected agent |
| `enter` | Focus/approve |
| Arrow keys | Navigate/scroll |

## Learning System

Alphie accumulates knowledge via CAO triples (Condition-Action-Outcome):

```
WHEN <condition>
DO <action>
RESULT <outcome>
```

**Examples:**
```
WHEN go.sum conflicts appear after merge
DO run 'go mod tidy' to reconcile
RESULT clean merge without manual intervention

WHEN React tests fail with act() warnings
DO wrap state updates in act() blocks
RESULT tests pass without async timing issues
```

**Storage:**
- Project-local: `.alphie/learnings.db`
- Global: `~/.local/share/alphie/alphie.db`

Learnings are automatically retrieved at task start to provide relevant context to agents.

## Configuration

**Global config:** `~/.config/alphie/config.yaml`
**Project config:** `.alphie.yaml` (overrides global)
**Tier configs:** `configs/scout.yaml`, `configs/builder.yaml`, `configs/architect.yaml`

**Key Settings:**
```yaml
# API configuration
anthropic:
  api_key: ${ANTHROPIC_API_KEY}

# Defaults
defaults:
  tier: builder
  token_budget: 100000

# TUI
tui:
  refresh_rate: 100ms

# Timeouts per tier
timeouts:
  scout: 5m
  builder: 15m
  architect: 30m

# Quality gates
quality_gates:
  test: true
  build: true
  lint: true
  typecheck: true
```

## Project Structure

```
alphie/
├── cmd/alphie/           # CLI entry points
│   ├── main.go           # Main entry
│   ├── root.go           # Root command + interactive mode
│   ├── run.go            # Run command
│   ├── implement.go      # Implement command
│   └── ...
├── internal/
│   ├── agent/            # Agent execution
│   │   ├── executor.go   # Agent executor with verification
│   │   ├── claude.go     # Claude subprocess wrapper
│   │   ├── worktree.go   # Git worktree management
│   │   ├── ralph_loop.go # Self-improvement loop
│   │   ├── prompt_runner.go # Claude prompt adapter
│   │   └── gates.go      # Quality gate runners
│   ├── orchestrator/     # Task orchestration
│   │   ├── orchestrator.go  # Main orchestrator
│   │   ├── scheduler.go     # Task scheduling
│   │   ├── tier_keywords.go # Unified tier classification
│   │   ├── merger.go        # Git merging
│   │   ├── semantic.go      # Semantic merge agent
│   │   ├── collision.go     # Collision detection
│   │   └── pkgmerge.go      # Package file merging
│   ├── verification/     # Verification contracts
│   │   ├── contract.go   # Contract types and runner
│   │   ├── generator.go  # Contract generation (draft/refine)
│   │   └── storage.go    # Contract persistence
│   ├── tui/              # Terminal UI
│   │   ├── panel_app.go  # Main TUI application
│   │   ├── agents_panel.go
│   │   ├── tasks_panel.go
│   │   └── ...
│   ├── learning/         # Learning system
│   ├── state/            # State persistence
│   ├── config/           # Configuration
│   ├── architect/        # Architecture implementation mode
│   └── prog/             # Prog integration
├── pkg/models/           # Shared data models
├── configs/              # Tier configuration files
└── docs/                 # Documentation
```

## Guarantees

Alphie provides hard guarantees about code safety:

- **No merge without verification green** - Tasks must pass verification contracts
- **No merge of failed tasks** - Tasks that fail after max iterations are never merged
- **Baseline prevents new regressions** - Pre-existing failures are allowed, new failures are not
- **Protected areas require human review** - Auth, migrations, infra trigger Architect-tier scrutiny
- **Approvals bind to snapshot** - Approval binds to base commit + diff hash; any change invalidates
- **DONE marker requires validation** - Agent "DONE" output is a request, not automatic exit; verification must pass

## Artifacts

Alphie stores persistent artifacts in `.alphie/`:

```
.alphie/
├── contracts/          # Verification contracts per task
│   ├── <task-id>-draft.json   # Pre-implementation contract
│   └── <task-id>.json         # Final contract (can only strengthen)
├── baselines/          # Test/lint baseline snapshots
│   └── <session-id>.json
├── state.db            # Session and task state
└── learnings.db        # Project-local learnings
```

## Safety Features

- **Session Branches** - Never merges directly to main/master (unless `--greenfield`)
- **Quality Gates** - Tests, build, lint must pass before merge (baseline-aware)
- **Verification Contracts** - Pre-implementation contracts prevent gaming; can only be strengthened
- **Clean Abort** - Tasks that fail verification after max iterations are not merged
- **Collision Prevention** - Detects when tasks might touch the same files
- **Human Review Gates** - Architect tier and risky changes require approval
- **Protected Areas** - Auth, migrations, infra trigger additional scrutiny
- **Budget Limits** - Configurable cost caps with graceful wind-down
- **Worktree Cleanup** - Automatic cleanup of orphaned worktrees
- **Dependency Blocking** - When a task fails, dependents are marked blocked with reason

## Troubleshooting

**Orphaned worktrees after crash:**
```bash
alphie cleanup --verbose
```

**Resume interrupted session:**
```bash
alphie --resume
```

**Check current state:**
```bash
alphie status
```

**View learnings:**
```bash
alphie learn --search "relevant query"
```

## License

MIT
