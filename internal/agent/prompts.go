// Package agent provides the AI agent implementation for Alphie.
package agent

// ScopeGuidancePrompt is injected at task start to prevent scope creep.
// It instructs agents to stay focused on the assigned task and file new tasks
// for any discoveries instead of expanding scope.
const ScopeGuidancePrompt = `## Scope Guidance

Stay focused on this task. If you discover refactoring opportunities
or unrelated improvements, note them as new tasks but do not implement
them in this session.

To file a new task for discovered work, use:
  prog add "Task title" -p <parent-task-id>

Do NOT:
- Expand scope with unrelated refactoring
- Fix unrelated bugs you encounter
- Add features not specified in the task
- Improve code style in unrelated files

DO:
- Complete the assigned task
- Note discoveries for future tasks
- Stay within the task boundaries
`

// ValidationGuidancePrompt instructs agents to validate their work before completion.
// This ensures code is not just syntactically correct but actually functional.
const ValidationGuidancePrompt = `## Validation Requirements

Before marking your task complete, you MUST verify your work:

### For Go Code:
1. **Import paths**: Ensure all import paths match the module name in go.mod
   - Run: grep "^module" go.mod (or backend/go.mod if applicable)
   - ALL imports starting with that module must use the EXACT module name
   - Example: If go.mod says "module my-app", use "my-app/internal/http" NOT "github.com/user/my-app/internal/http"

2. **Dependencies**: Run go mod tidy to ensure dependencies are correct
   - If you added new imports, run: go mod tidy

3. **Build verification**: Ensure code compiles
   - Run: go build ./... (or cd backend && go build ./cmd/...)
   - Fix any compilation errors before finishing

### For JavaScript/TypeScript:
1. **Dependencies**: If you added npm packages or created package.json, run npm install
   - Check: Does node_modules/ exist? If not and package.json exists, run npm install
   - Verify: Run npm list to see installed packages

2. **Build verification**: If applicable, test the build
   - Frontend: npm run build (if build script exists)
   - Verify: Check for TypeScript/ESLint errors

### For Any Code:
1. **Consistency**: Ensure import paths, package names, and module references are consistent throughout
2. **Completeness**: If you created a new project structure, ensure all initialization is done (go mod init, npm install, etc.)
3. **Testing**: If the task involves runnable code, verify it actually starts/runs without immediate errors

CRITICAL: Do NOT complete the task if:
- Imports use wrong module names (check go.mod!)
- Dependencies are missing (missing node_modules/ with package.json present)
- Code doesn't compile
- You haven't run go mod tidy after adding Go imports
- You haven't run npm install after creating/modifying package.json
`
