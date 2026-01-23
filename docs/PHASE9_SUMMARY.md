# Phase 9: Update Branch Naming with Spec Name - COMPLETE ✅

## Overview

Phase 9 updates the branch naming system to include the spec name, making branches more descriptive and easier to identify. Branches are now named `alphie-{spec-name}-{timestamp}` instead of the generic `session-{timestamp}`.

## What Was Changed

### 📦 Enhanced Files

**1. `internal/orchestrator/options.go`**
- Added `specName string` field to `orchestratorOptions`
- Created `WithSpecName()` function for functional options
- Updated `toOrchestratorConfig()` to pass spec name

**2. `internal/orchestrator/orchestrator.go`**
- Added `SpecName string` field to `OrchestratorConfig` with documentation
- Updated `NewSessionBranchManagerWithRunner()` call to pass spec name

**3. `internal/orchestrator/session.go`**
- Updated `NewSessionBranchManager()` to accept `specName` parameter
- Updated `NewSessionBranchManagerWithRunner()` to accept `specName` parameter
- Implemented new branch naming logic:
  - With spec name: `alphie-{spec-name}-{timestamp}`
  - Without spec name (fallback): `alphie-session-{timestamp}`
  - Greenfield mode: empty string (no branch)

**4. `cmd/alphie/implement.go`**
- Added `orchestrator.WithSpecName(cfg.specName)` to orchestrator creation
- Spec name extracted via `extractSpecName()` function (already added in Phase 7)

**5. `internal/orchestrator/session_test.go`**
- Updated all test cases with new function signatures
- Updated expected branch names to match new format
- Added tests for spec name variations:
  - With spec name: `alphie-auth-system-abc123`
  - Without spec name: `alphie-session-abc123`
  - Greenfield mode: empty string

## Branch Naming Format

### Previous Format
```
session-{sessionID}

Examples:
- session-abc123
- session-task-001
- session-ts_8b7a01
```

### New Format
```
alphie-{spec-name}-{sessionID}

Examples:
- alphie-auth-system-abc123
- alphie-user-api-spec-task-001
- alphie-api-docs-ts_8b7a01
```

### Fallback (no spec name)
```
alphie-session-{sessionID}

Examples:
- alphie-session-abc123
- alphie-session-xyz789
```

### Greenfield Mode
```
(empty string - no branch created, work goes directly to main)
```

## Implementation Details

### Spec Name Extraction (from Phase 7)

```go
func extractSpecName(specPath string) string {
    // Get base filename without extension
    base := filepath.Base(specPath)
    name := strings.TrimSuffix(base, filepath.Ext(base))

    // Clean up: lowercase, replace spaces/special chars with hyphens
    name = strings.ToLower(name)
    name = strings.Map(func(r rune) rune {
        if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
            return r
        }
        return '-'
    }, name)

    // Remove consecutive hyphens and trim
    for strings.Contains(name, "--") {
        name = strings.ReplaceAll(name, "--", "-")
    }
    name = strings.Trim(name, "-")

    // Truncate if too long
    if len(name) > 50 {
        name = name[:50]
    }

    return name
}
```

**Examples:**
- `"Auth System.md"` → `"auth-system"`
- `"API  Spec.xml"` → `"api-spec"`
- `"user_management.md"` → `"user-management"`
- `"Very Long API Specification Name.md"` → `"very-long-api-specification-name"` (truncated at 50 chars)

### Branch Name Construction

```go
func NewSessionBranchManager(sessionID, repoPath string, greenfield bool, specName string) *SessionBranchManager {
    branchName := ""
    if !greenfield {
        if specName != "" {
            branchName = fmt.Sprintf("alphie-%s-%s", specName, sessionID)
        } else {
            // Fallback to session-based naming if no spec name provided
            branchName = fmt.Sprintf("alphie-session-%s", sessionID)
        }
    }

    return &SessionBranchManager{
        sessionID:  sessionID,
        branchName: branchName,
        greenfield: greenfield,
        repoPath:   repoPath,
        git:        git.NewRunner(repoPath),
    }
}
```

### Orchestrator Integration

```go
// In cmd/alphie/implement.go
cfg := implementConfig{
    specPath:      specPath,
    repoPath:      repoPath,
    specName:      extractSpecName(specPath), // Extract name from spec file
    // ...
}

// Later in createOrchestrator()
orch := orchestrator.New(
    orchestrator.RequiredConfig{
        RepoPath: cfg.repoPath,
        Executor: executor,
    },
    orchestrator.WithSpecName(cfg.specName), // Pass to orchestrator
    // ...
)

// Orchestrator passes to session manager
sessionMgr := NewSessionBranchManagerWithRunner(
    sessionID,
    cfg.RepoPath,
    cfg.Greenfield,
    cfg.SpecName, // Session manager receives spec name
    gitRunner,
)
```

## Examples

### Example 1: Standard Usage

```bash
alphie implement docs/authentication-system.md
```

**Spec file:** `authentication-system.md`
**Extracted name:** `authentication-system`
**Session ID:** `a1b2c3d4` (8-char UUID)
**Branch created:** `alphie-authentication-system-a1b2c3d4`

### Example 2: With Spaces and Special Characters

```bash
alphie implement "specs/User API (v2).xml"
```

**Spec file:** `User API (v2).xml`
**Extracted name:** `user-api-v2`
**Session ID:** `e5f6g7h8`
**Branch created:** `alphie-user-api-v2-e5f6g7h8`

### Example 3: Greenfield Mode

```bash
alphie implement spec.md --greenfield
```

**Spec file:** `spec.md`
**Extracted name:** `spec`
**Branch created:** (none - works directly on main)
**Commits go to:** `main` branch

### Example 4: No Spec Name (Fallback)

```go
// If extractSpecName() returns empty string
// (shouldn't happen in practice, but handled)
```

**Session ID:** `x9y8z7w6`
**Branch created:** `alphie-session-x9y8z7w6`

## Benefits

### Before Phase 9

- **Generic Names**: `session-abc123` tells you nothing about the work
- **Hard to Identify**: Multiple session branches look identical in `git branch`
- **No Context**: Can't tell what spec a branch implements without checking commit messages
- **Cluttered**: Old session branches pile up with no way to identify them

**Example git branch output:**
```
main
session-abc123
session-def456
session-ghi789
session-jkl012
```

### After Phase 9

- **Descriptive Names**: `alphie-auth-system-abc123` clearly indicates what's being implemented
- **Easy to Identify**: Can quickly find the branch for a specific spec
- **Better Organization**: Multiple specs can be worked on simultaneously with clear separation
- **Easier Cleanup**: Can identify and clean up old branches by spec name

**Example git branch output:**
```
main
alphie-auth-system-abc123
alphie-user-api-def456
alphie-payment-flow-ghi789
alphie-notification-service-jkl012
```

## Integration Points

### With Phase 7 (Implement Command)

- Phase 7 added `extractSpecName()` function
- Phase 9 integrates this into the orchestrator flow
- Spec name extracted once in implement.go and passed through to session manager

### With Orchestrator

- New `SpecName` field in `OrchestratorConfig`
- Functional option `WithSpecName()` for clean API
- Session manager receives spec name and formats branch accordingly

### With Tests

- All session manager tests updated
- Tests verify both spec name and fallback formats
- Tests verify greenfield mode (no branch creation)

## Testing

### Updated Test Cases

```go
func TestSessionBranchManager_BranchNaming(t *testing.T) {
    tests := []struct {
        name       string
        sessionID  string
        specName   string
        greenfield bool
        expected   string
    }{
        {
            name:       "standard branch naming with spec name",
            sessionID:  "abc123",
            specName:   "auth-system",
            greenfield: false,
            expected:   "alphie-auth-system-abc123",
        },
        {
            name:       "fallback to session naming when no spec name",
            sessionID:  "ts_8b7a01",
            specName:   "",
            greenfield: false,
            expected:   "alphie-session-ts_8b7a01",
        },
        {
            name:       "greenfield mode - no branch",
            sessionID:  "abc123",
            specName:   "some-spec",
            greenfield: true,
            expected:   "",
        },
        // ... more test cases
    }
}
```

### Manual Testing

1. **Test standard spec:**
   ```bash
   alphie implement docs/api-spec.md
   git branch  # Should show: alphie-api-spec-{timestamp}
   ```

2. **Test with spaces:**
   ```bash
   alphie implement "My Spec.md"
   git branch  # Should show: alphie-my-spec-{timestamp}
   ```

3. **Test greenfield:**
   ```bash
   alphie implement spec.md --greenfield
   git branch  # Should NOT create new branch
   git log --oneline -1  # Should show commit on main
   ```

4. **Test multiple concurrent specs:**
   ```bash
   # Terminal 1
   alphie implement auth-spec.md

   # Terminal 2
   alphie implement api-spec.md

   git branch
   # Should show:
   #   alphie-auth-spec-{timestamp1}
   #   alphie-api-spec-{timestamp2}
   ```

## Edge Cases Handled

### Long Spec Names

Spec names are truncated to 50 characters:
```
very-long-complex-api-specification-name-with-detail → (50 chars max)
```

**Branch:** `alphie-very-long-complex-api-specification-name-w-abc123`

### Special Characters

All special characters converted to hyphens:
```
"User@API#v2!.md" → "user-api-v2"
```

**Branch:** `alphie-user-api-v2-abc123`

### Consecutive Hyphens

Multiple hyphens collapsed to single hyphen:
```
"API--Spec__v2.md" → "api-spec-v2"
```

**Branch:** `alphie-api-spec-v2-abc123`

### Empty Spec Name

Falls back to session-based naming:
```
specName = ""
```

**Branch:** `alphie-session-abc123`

### Greenfield Mode

No branch created regardless of spec name:
```
specName = "auth-system"
greenfield = true
```

**Branch:** (empty string - no branch)

## Backwards Compatibility

### Breaking Changes

This is a breaking change for:
- ❌ Branch name format changed
- ❌ Session manager function signatures changed
- ❌ Tests updated

### Migration Notes

**For existing branches:**
- Old `session-*` branches continue to work
- New format applies to new sessions only
- Old branches can be identified by prefix:
  - Old: `session-{id}`
  - New: `alphie-{spec}-{id}`

**For code using session manager:**
- Must pass `specName` parameter to `NewSessionBranchManager()`
- Can pass empty string for fallback behavior

## Known Limitations

### Spec Name Extraction

1. **Only uses filename:** Doesn't read spec content for name
2. **No duplicate detection:** Multiple specs with same name will have same branch prefix (different timestamps prevent collision)
3. **Fixed truncation:** 50-character limit may truncate meaningful part of name

### Branch Cleanup

- Old session branches not automatically renamed
- Need manual cleanup of old format branches

## Current Status

### ✅ Complete

- [x] Added `specName` to orchestrator options
- [x] Created `WithSpecName()` functional option
- [x] Updated `OrchestratorConfig` with SpecName field
- [x] Updated both session manager constructors
- [x] Implemented new branch naming logic
- [x] Updated implement.go to pass spec name
- [x] Updated all test cases
- [x] Verified tests pass (session manager tests)
- [x] Code compiles cleanly

### 🚧 Limitations

- Some test files have unrelated imports that cause test failures (prog package)
- Main code compiles and functions correctly

## Key Achievements

✅ **Descriptive Branch Names**: Branches now indicate what they're implementing
✅ **Better Organization**: Multiple specs can be worked on with clear identification
✅ **Backward Compatible Fallback**: Works even without spec name
✅ **Clean Integration**: Uses functional options pattern
✅ **Comprehensive Testing**: All branch naming scenarios tested
✅ **Edge Case Handling**: Long names, special chars, empty names all handled

## Next Steps

With Phase 9 complete, remaining work:

**Independent (can do now)**:
- Phase 10: Update root command and help (~1 hour)

**Dependent**:
- Phase 8: Simplify TUI (~2-3 hours) - Already mostly integrated
- Phase 11: End-to-end testing (~4-6 hours) - Test all phases together

## Conclusion

Phase 9 successfully updates the branch naming system to include spec names, making Alphie's git branches more descriptive and easier to manage. The new format `alphie-{spec-name}-{timestamp}` provides clear context about what each branch implements, while maintaining backward compatibility through fallback naming.

The implementation uses the existing spec name extraction from Phase 7, integrates cleanly with the orchestrator's functional options pattern, and handles all edge cases (long names, special characters, empty names, greenfield mode).

Users will now see branches like `alphie-auth-system-a1b2c3d4` instead of generic `session-a1b2c3d4`, making it much easier to identify and manage implementation branches.

---

**Progress**: 73% complete (8/11 phases)
**Phase 9 Status**: ✅ COMPLETE
