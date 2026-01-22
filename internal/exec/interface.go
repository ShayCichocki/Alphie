// Package exec provides an interface for command execution.
package exec

import (
	"context"
)

// CommandRunner defines the interface for running external commands.
// This abstraction allows mocking command execution in tests.
type CommandRunner interface {
	// Run executes a command and returns combined stdout/stderr output.
	// The working directory is set to workDir if non-empty.
	Run(ctx context.Context, workDir string, name string, args ...string) (output []byte, err error)

	// RunShell executes a shell command through "sh -c".
	// This is a convenience method for running complex shell commands.
	RunShell(ctx context.Context, workDir string, command string) (output []byte, err error)

	// Exists checks if a file exists at the given path.
	// The working directory is set to workDir if non-empty.
	Exists(ctx context.Context, workDir string, path string) bool
}
