// Package tui provides the terminal user interface for Alphie's implement command.
//
// This package contains a simplified, read-only TUI that displays implementation
// progress in real-time. It is used exclusively by the implement command to show:
//   - Current implementation phase (parse, audit, orchestrate, verify)
//   - Feature completion progress (e.g., 3/5 features complete)
//   - Active workers and their current tasks
//   - Activity log with recent events
//   - Stop conditions and blocked questions
//
// The TUI is read-only and does not support interactive task submission.
// Users can only quit with 'q' or Ctrl+C.
//
// Usage:
//
//	program, app := tui.NewImplementProgram()
//	go program.Run()
//
//	// Send state updates
//	program.Send(tui.ImplementUpdateMsg{State: state})
//
//	// Send log messages
//	program.Send(tui.ImplementLogMsg{
//	    Timestamp: time.Now(),
//	    Phase:     "orchestrate",
//	    Message:   "Starting task execution",
//	})
//
//	// Signal completion
//	program.Send(tui.ImplementDoneMsg{Err: nil})
//
// The TUI automatically renders progress bars, format timestamps, and highlights
// active workers with their agent IDs and task titles.
package tui
