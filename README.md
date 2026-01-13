# Alphie

Parallel Claude Code agent orchestrator. Decomposes tasks, runs them in isolated git worktrees, and merges the results.

![Alphie TUI](docs/Screenshot%202026-01-12%223016.png)

## Requirements

- Go 1.24+
- [Claude Code CLI](https://github.com/anthropics/claude-code) installed and authenticated
- Git

## Install

```bash
go install github.com/shayc/alphie/cmd/alphie@latest
```

Or build from source:

```bash
git clone https://github.com/shayc/alphie
cd alphie
go build -o alphie ./cmd/alphie
```

## Quick Start

```bash
# Launch interactive mode - type tasks, watch them run
alphie

# Run a single task
alphie run "Add user authentication to the API"

# Use scout tier for quick exploration
alphie run "Find all TODO comments" --tier scout

# Use architect tier for complex work
alphie run "Refactor the database layer" --tier architect

# Run without TUI
alphie run "Fix the login bug" --headless
```

## Commands

### Interactive Mode

Launch with no arguments for a persistent TUI. Type tasks, hit enter, watch them run in parallel.

```bash
alphie                    # Fresh start
alphie --resume           # Resume incomplete tasks from previous sessions
```

The tier is auto-detected from task text:
- Scout keywords: "find", "search", "list", "check", "where", "what"
- Architect keywords: "refactor", "redesign", "migrate", "rewrite"
- Everything else defaults to builder

Override with prefix: `!scout find files`, `!builder add feature`, `!architect redesign API`

### run

Execute a task with parallel agents.

```bash
alphie run <task> [flags]
```

Flags:
- `--tier` - Agent tier: `scout`, `builder` (default), or `architect`
- `--headless` - Run without TUI
- `--greenfield` - Merge directly to main (for new projects)
- `--epic <id>` - Resume an existing epic from a previous session

### implement

Iterate through an architecture spec until it's implemented.

```bash
alphie implement <arch.md> [flags]
```

Parses a markdown architecture doc, audits the codebase for gaps, plans tasks, and executes them in a loop until done.

Flags:
- `--agents` - Max concurrent workers (default 3)
- `--budget` - Cost limit in dollars
- `--max-iterations` - Hard cap on iterations (default 10)
- `--dry-run` - Show plan without executing
- `--resume` - Resume from checkpoint

### audit

Check codebase against an architecture spec.

```bash
alphie audit <arch.md> [--json]
```

### learn

Manage learnings (condition-action-outcome triples).

```bash
alphie learn                                    # List recent
alphie learn "WHEN X DO Y RESULT Z"             # Add new
alphie learn --search "query"                   # Search
alphie learn show <id>                          # Details
```

### status

Show current session state.

```bash
alphie status
```

### cleanup

Remove orphaned worktrees and old sessions.

```bash
alphie cleanup
```

## Tiers

| Tier | Agents | Model | Use Case |
|------|--------|-------|----------|
| scout | 2 | haiku | Quick exploration, research |
| builder | 3 | sonnet | Standard feature work |
| architect | 5 | opus | Complex design, refactoring |

## Configuration

Global config: `~/.config/alphie/config.yaml`
Project config: `.alphie.yaml` in project root

```bash
alphie config                    # Show all
alphie config <key>              # Show value
alphie config <key> <value>      # Set value
```

## How It Works

1. Task gets decomposed into parallelizable subtasks
2. Each subtask runs in an isolated git worktree
3. Agents execute with the Ralph loop (critique, improve, repeat)
4. Results merge back to a session branch
5. Session branch merges to main on completion

Worktrees are cleaned up automatically. If something goes wrong, `alphie cleanup` removes any orphans.

## License

MIT
