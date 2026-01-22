# Collision Detection in Alphie

Alphie uses a sophisticated **4-layer collision detection system** to prevent merge conflicts when multiple agents work in parallel. This document explains how it works and how to configure it.

## Overview

The collision detection system analyzes task file boundaries **before execution** to determine which tasks can safely run in parallel. It uses four layers of increasingly strict checks to prevent conflicts.

## The 4-Layer System

### Layer 1: SETUP Task Serialization

**What it does:** Forces all SETUP tasks to run sequentially before other tasks.

**Why:** SETUP tasks often modify configuration files, dependencies, or project structure that other tasks depend on. Running them in parallel would cause race conditions.

**Example:**
```
Task A: Install dependencies (SETUP)
Task B: Install linters (SETUP)
Task C: Implement feature (TASK)

Execution order: A → B → C
```

**Configuration:** Automatic, cannot be disabled.

---

### Layer 2: Critical File Conflicts

**What it does:** Prevents parallel modification of critical files like `package.json`, `go.mod`, `Cargo.toml`, etc.

**Why:** These files are central to project configuration. Concurrent modifications lead to complex merge conflicts.

**Critical files:**
- `package.json`, `package-lock.json` (Node.js)
- `go.mod`, `go.sum` (Go)
- `Cargo.toml`, `Cargo.lock` (Rust)
- `pyproject.toml`, `requirements.txt` (Python)
- `.env`, `.env.*` (Environment configs)
- Root-level config files (`.gitignore`, `.prettierrc`, etc.)

**Example:**
```
Task A: Add lodash dependency → modifies package.json
Task B: Add express dependency → modifies package.json

Execution: Serialized (A → B) to avoid package.json conflict
```

**Configuration:** Configurable via `.alphie.yaml`:
```yaml
collision_detection:
  critical_files:
    - "package.json"
    - "go.mod"
    - "custom-config.yaml"
```

---

### Layer 3: Greenfield Root-Touching Conflicts

**What it does:** In greenfield mode (new projects), prevents multiple tasks from creating files in the root directory simultaneously.

**Why:** Root-level file creation in parallel can cause race conditions and unclear project structure.

**Example:**
```
Task A: Create README.md
Task B: Create LICENSE
Task C: Create .gitignore

Greenfield mode: Serialized to ensure clean project initialization
Normal mode: Can run in parallel (root already exists)
```

**Configuration:**
- Enabled automatically in greenfield mode
- Disable greenfield mode with `--greenfield=false`

---

### Layer 4: General Collision Avoidance

**What it does:** Prevents tasks with overlapping file boundaries from running concurrently.

**Checks:**
1. **Path Prefix Overlap:** `internal/auth/` conflicts with `internal/auth/handlers/`
2. **Hotspot Files:** Files modified by many tasks (>3) are treated as merge conflict risks
3. **Directory Saturation:** If >70% of tasks touch the same directory, serialize access

**Example:**
```
Task A: Modify internal/auth/handler.go
Task B: Modify internal/auth/service.go
Task C: Modify internal/api/routes.go

Analysis:
- Tasks A & B: Overlap (both touch internal/auth/) → Serialize
- Task C: Independent → Can run with A or B

Execution: (A → B) || C
```

**Configuration:** Adjustable thresholds in `.alphie.yaml`:
```yaml
collision_detection:
  hotspot_threshold: 3        # Files modified by >3 tasks are "hot"
  directory_saturation: 0.7   # Serialize if >70% of tasks touch same dir
```

---

## How It Works: Pre-Flight Analysis

Before starting agents, the scheduler performs collision analysis:

```go
// 1. Extract file boundaries from all tasks
taskBoundaries := extractFileBoundaries(tasks)

// 2. Apply 4-layer checks
layer1Results := serializeSetupTasks(tasks)
layer2Results := detectCriticalFileConflicts(taskBoundaries)
layer3Results := detectGreenfieldRootConflicts(taskBoundaries)
layer4Results := detectGeneralOverlap(taskBoundaries)

// 3. Build collision graph
collisionGraph := buildGraph(layer1Results, layer2Results, layer3Results, layer4Results)

// 4. Graph coloring to determine parallelism
independentSets := colorGraph(collisionGraph)
```

The result is a **dependency graph** where tasks that would collide are connected, forcing sequential execution.

---

## Configuration

### Basic Configuration

Create `.alphie.yaml` in your repository:

```yaml
collision_detection:
  # Disable specific layers (not recommended)
  disable_critical_files: false
  disable_greenfield_root: false
  disable_general_overlap: false

  # Add custom critical files
  critical_files:
    - "docker-compose.yml"
    - "Makefile"
    - ".github/workflows/*.yml"

  # Adjust Layer 4 thresholds
  hotspot_threshold: 3
  directory_saturation: 0.7
```

### Advanced: Custom File Boundaries

When decomposing tasks, specify precise file boundaries:

```yaml
tasks:
  - title: "Add authentication"
    file_boundaries:
      - "internal/auth/"      # Precise boundary
    # NOT: "internal/" or "." (too broad)
```

**Best practices:**
- Use specific directories, not root paths
- One module per task when possible
- Avoid overlapping boundaries

---

## Observing Collision Detection

### In Logs

Enable debug logging to see collision decisions:

```bash
alphie run --debug
```

Look for entries like:
```
[scheduler] Layer 2: Tasks T1 and T2 conflict on package.json
[scheduler] Layer 4: Tasks T3 and T4 overlap in internal/auth/
[scheduler] Serialization required: T1 → T2
[scheduler] Max parallelism: 3 tasks
```

### In Decomposition Review UI

When using `--review` mode, the TUI shows:
- **Green tasks:** Can run in parallel with all other tasks
- **Yellow tasks:** Serialized due to critical file overlap
- **Red tasks:** High collision risk (hotspot file)

---

## Troubleshooting

### Problem: Everything runs sequentially

**Cause:** Tasks have overly broad file boundaries (e.g., `"src/"`, `"."`).

**Solution:** Decompose with more specific boundaries:
```diff
- file_boundaries: ["src/"]
+ file_boundaries: ["src/components/auth/"]
```

---

### Problem: Unexpected serialization

**Cause:** Layer 2 critical file conflict or Layer 4 directory saturation.

**Solution 1:** If false positive, adjust critical files:
```yaml
collision_detection:
  critical_files: []  # Disable critical file detection
```

**Solution 2:** Increase directory saturation threshold:
```yaml
collision_detection:
  directory_saturation: 0.9  # Allow more parallel access
```

---

### Problem: Merge conflicts still occur

**Cause:** Tasks modify the same lines in files, which boundary-based detection can't prevent.

**Solution:** Alphie uses **semantic merge** to resolve line-level conflicts automatically. If semantic merge fails, **human escalation** triggers.

Collision detection prevents *file-level* conflicts. Semantic merge handles *line-level* conflicts.

---

## Performance Impact

Collision detection adds ~50-200ms overhead per decomposition. This is negligible compared to:
- Agent execution time (minutes)
- Cost of merge conflicts (retries, semantic merge, human escalation)

**Recommendation:** Keep all layers enabled for maximum reliability.

---

## Implementation Details

**Location:** `/internal/orchestrator/collision.go`, `/internal/orchestrator/scheduler.go`

**Algorithm:** Graph coloring with greedy heuristics

**Data structures:**
- `CollisionChecker`: Manages collision state
- `CollisionGraph`: Represents task conflicts
- `Scheduler`: Applies collision rules to execution

---

## Related Features

- **Protected Areas:** Some files (e.g., auth, migrations) trigger Scout override gates regardless of collision detection
- **Semantic Merge:** Handles line-level conflicts when collision detection allows parallel execution
- **Verification Contracts:** Validate that parallel execution didn't cause semantic issues

---

## FAQ

**Q: Can I disable collision detection entirely?**
A: Not recommended. You can disable layers 2-4 individually, but layer 1 (SETUP serialization) is always active.

**Q: What if I have a monorepo with independent modules?**
A: Use precise file boundaries per module. Collision detection works well with microservice/module boundaries.

**Q: Does collision detection account for git conflicts?**
A: Yes, it analyzes file paths the same way git would. If git would merge cleanly, collision detection allows parallelism.

**Q: How does this compare to GitHub's merge queue?**
A: GitHub serializes *all* PRs. Alphie allows parallelism when safe, falling back to serialization only when necessary. This is much faster for independent changes.

---

## Summary

Alphie's 4-layer collision detection prevents merge conflicts by:
1. Serializing SETUP tasks
2. Protecting critical files
3. Handling greenfield initialization carefully
4. Analyzing general file overlap

This enables safe parallel execution while maintaining high throughput. Configure thresholds in `.alphie.yaml` to match your project's needs.
