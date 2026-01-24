// Package validation provides build and test validation.
package validation

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// SimpleBuildTester implements BuildTester by running build and test commands.
type SimpleBuildTester struct {
	// buildCmd is the build command to run (e.g., ["go", "build", "./..."])
	buildCmd []string
	// testCmd is the test command to run (e.g., ["go", "test", "./..."])
	testCmd []string
	// timeout is the maximum time to wait for build + tests
	timeout time.Duration
}

// NewSimpleBuildTester creates a new SimpleBuildTester with the given commands.
func NewSimpleBuildTester(buildCmd, testCmd []string, timeout time.Duration) *SimpleBuildTester {
	return &SimpleBuildTester{
		buildCmd: buildCmd,
		testCmd:  testCmd,
		timeout:  timeout,
	}
}

// NewAutoBuildTester creates a build tester that auto-detects project type.
func NewAutoBuildTester(repoPath string, timeout time.Duration) (*SimpleBuildTester, error) {
	projectType := detectProjectType(repoPath)

	var buildCmd, testCmd []string

	switch projectType {
	case "go":
		buildCmd = []string{"go", "build", "./..."}
		testCmd = []string{"go", "test", "./..."}
	case "node":
		buildCmd = []string{"npm", "run", "build"}
		testCmd = []string{"npm", "test"}
	case "python":
		buildCmd = []string{"python", "-m", "py_compile", "."}
		testCmd = []string{"pytest"}
	case "rust":
		buildCmd = []string{"cargo", "build"}
		testCmd = []string{"cargo", "test"}
	default:
		// Unknown project type - return empty commands (will skip validation)
		return &SimpleBuildTester{
			buildCmd: []string{},
			testCmd:  []string{},
			timeout:  timeout,
		}, nil
	}

	return &SimpleBuildTester{
		buildCmd: buildCmd,
		testCmd:  testCmd,
		timeout:  timeout,
	}, nil
}

// RunBuildAndTests runs build and test commands.
func (t *SimpleBuildTester) RunBuildAndTests(ctx context.Context, repoPath string) (passed bool, output string, err error) {
	var sb strings.Builder

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	// Run build if configured
	if len(t.buildCmd) > 0 {
		sb.WriteString("=== Running Build ===\n")
		buildOutput, buildErr := t.runCommand(timeoutCtx, repoPath, t.buildCmd)
		sb.WriteString(buildOutput)
		sb.WriteString("\n")

		if buildErr != nil {
			return false, sb.String(), fmt.Errorf("build failed: %w", buildErr)
		}
		sb.WriteString("✓ Build passed\n\n")
	}

	// Run tests if configured
	if len(t.testCmd) > 0 {
		sb.WriteString("=== Running Tests ===\n")
		testOutput, testErr := t.runCommand(timeoutCtx, repoPath, t.testCmd)
		sb.WriteString(testOutput)
		sb.WriteString("\n")

		if testErr != nil {
			return false, sb.String(), fmt.Errorf("tests failed: %w", testErr)
		}
		sb.WriteString("✓ Tests passed\n")
	}

	// If no commands configured, skip
	if len(t.buildCmd) == 0 && len(t.testCmd) == 0 {
		return true, "No build or test commands configured (skipped)", nil
	}

	return true, sb.String(), nil
}

// runCommand runs a command and returns output and error.
func (t *SimpleBuildTester) runCommand(ctx context.Context, repoPath string, cmdParts []string) (string, error) {
	if len(cmdParts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	cmd := exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	return string(output), err
}

// detectProjectType attempts to detect the project type from the repository.
func detectProjectType(repoPath string) string {
	// Check for Go
	if fileExists(repoPath + "/go.mod") {
		return "go"
	}

	// Check for Node.js
	if fileExists(repoPath + "/package.json") {
		return "node"
	}

	// Check for Python
	if fileExists(repoPath + "/setup.py") || fileExists(repoPath + "/pyproject.toml") {
		return "python"
	}

	// Check for Rust
	if fileExists(repoPath + "/Cargo.toml") {
		return "rust"
	}

	return "unknown"
}

// fileExists checks if a file exists (simple helper).
func fileExists(path string) bool {
	_, err := exec.Command("test", "-f", path).CombinedOutput()
	return err == nil
}
