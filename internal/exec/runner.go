package exec

import (
	"context"
	"os/exec"
)

// ExecRunner implements CommandRunner using os/exec.
type ExecRunner struct{}

// NewRunner creates a new ExecRunner.
func NewRunner() *ExecRunner {
	return &ExecRunner{}
}

// Run executes a command and returns combined stdout/stderr output.
func (r *ExecRunner) Run(ctx context.Context, workDir string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	return cmd.CombinedOutput()
}

// RunShell executes a shell command through "sh -c".
func (r *ExecRunner) RunShell(ctx context.Context, workDir string, command string) ([]byte, error) {
	return r.Run(ctx, workDir, "sh", "-c", command)
}

// Exists checks if a file exists at the given path.
func (r *ExecRunner) Exists(ctx context.Context, workDir string, path string) bool {
	cmd := exec.CommandContext(ctx, "test", "-e", path)
	if workDir != "" {
		cmd.Dir = workDir
	}
	return cmd.Run() == nil
}

// Verify ExecRunner implements CommandRunner at compile time.
var _ CommandRunner = (*ExecRunner)(nil)
