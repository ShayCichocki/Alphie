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
