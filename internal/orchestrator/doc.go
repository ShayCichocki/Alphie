// Package orchestrator manages the coordination of agents and workflows.
//
// The orchestrator package provides functionality for:
//   - Task decomposition: Breaking down user requests into parallelizable subtasks
//   - Dependency management: Tracking task dependencies and execution order
//   - Agent coordination: Assigning tasks to agents based on availability and tier
//
// The Decomposer component uses Claude to intelligently break down complex requests
// into smaller, agent-sized tasks that can be executed in parallel where possible.
// Each task includes:
//   - Title and description
//   - Dependencies on other tasks
//   - Acceptance criteria for verification
//
// Example usage:
//
//	claude := agent.NewClaudeProcess(ctx)
//	decomposer := orchestrator.NewDecomposer(claude)
//	tasks, err := decomposer.Decompose(ctx, "Build a user authentication system")
package orchestrator
