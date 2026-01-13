// Package agent provides the AI agent implementation for Alphie.
package agent

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GateResult represents the outcome of a quality gate check.
type GateResult int

const (
	// GatePass indicates the gate check succeeded.
	GatePass GateResult = iota
	// GateFail indicates the gate check failed.
	GateFail
	// GateSkip indicates the gate was skipped (not applicable).
	GateSkip
	// GateError indicates an error occurred while running the gate.
	GateError
)

// String returns the string representation of a GateResult.
func (r GateResult) String() string {
	switch r {
	case GatePass:
		return "pass"
	case GateFail:
		return "fail"
	case GateSkip:
		return "skip"
	case GateError:
		return "error"
	default:
		return "unknown"
	}
}

// GateOutput contains the result of running a single quality gate.
type GateOutput struct {
	// Gate is the name of the quality gate (test, build, lint, typecheck).
	Gate string
	// Result indicates whether the gate passed, failed, was skipped, or errored.
	Result GateResult
	// Output contains stdout/stderr from the gate command.
	Output string
	// Duration is how long the gate took to run.
	Duration time.Duration
}

// QualityGates runs quality checks (tests, build, lint, typecheck) on a codebase.
type QualityGates struct {
	testEnabled      bool
	buildEnabled     bool
	lintEnabled      bool
	typecheckEnabled bool
	workDir          string
	timeout          time.Duration
}

// NewQualityGates creates a new QualityGates runner for the given work directory.
// All gates are disabled by default; use the Enable* methods to enable them.
func NewQualityGates(workDir string) *QualityGates {
	return &QualityGates{
		testEnabled:      false,
		buildEnabled:     false,
		lintEnabled:      false,
		typecheckEnabled: false,
		workDir:          workDir,
		timeout:          5 * time.Minute,
	}
}

// EnableTest enables or disables the test gate.
func (q *QualityGates) EnableTest(enabled bool) {
	q.testEnabled = enabled
}

// EnableBuild enables or disables the build gate.
func (q *QualityGates) EnableBuild(enabled bool) {
	q.buildEnabled = enabled
}

// EnableLint enables or disables the lint gate.
func (q *QualityGates) EnableLint(enabled bool) {
	q.lintEnabled = enabled
}

// EnableTypecheck enables or disables the typecheck gate.
func (q *QualityGates) EnableTypecheck(enabled bool) {
	q.typecheckEnabled = enabled
}

// SetTimeout sets the timeout for each individual gate.
func (q *QualityGates) SetTimeout(d time.Duration) {
	q.timeout = d
}

// RunGates runs all enabled quality gates and returns their results.
// Gates that are not applicable (e.g., no test files) return GateSkip.
func (q *QualityGates) RunGates() ([]*GateOutput, error) {
	var results []*GateOutput

	if q.testEnabled {
		results = append(results, q.runTests())
	}

	if q.buildEnabled {
		results = append(results, q.runBuild())
	}

	if q.lintEnabled {
		results = append(results, q.runLint())
	}

	if q.typecheckEnabled {
		results = append(results, q.runTypecheck())
	}

	return results, nil
}

// runTests runs the test suite for the project.
func (q *QualityGates) runTests() *GateOutput {
	output := &GateOutput{
		Gate: "test",
	}

	// Detect project type and check for test files
	projectType := q.detectProjectType()

	start := time.Now()
	defer func() {
		output.Duration = time.Since(start)
	}()

	switch projectType {
	case "go":
		// Check for Go test files
		if !q.hasGoTestFiles() {
			output.Result = GateSkip
			output.Output = "No Go test files found"
			return output
		}
		return q.runCommand(output, "go", "test", "./...")

	case "node":
		// Check if package.json has a test script
		if !q.hasNodeTestScript() {
			output.Result = GateSkip
			output.Output = "No test script in package.json"
			return output
		}
		return q.runCommand(output, "npm", "test")

	case "python":
		// Check for pytest or unittest files
		if !q.hasPythonTests() {
			output.Result = GateSkip
			output.Output = "No Python test files found"
			return output
		}
		return q.runCommand(output, "python", "-m", "pytest")

	default:
		output.Result = GateSkip
		output.Output = "Unknown project type, cannot run tests"
		return output
	}
}

// runBuild runs the build command for the project.
func (q *QualityGates) runBuild() *GateOutput {
	output := &GateOutput{
		Gate: "build",
	}

	projectType := q.detectProjectType()

	start := time.Now()
	defer func() {
		output.Duration = time.Since(start)
	}()

	switch projectType {
	case "go":
		return q.runCommand(output, "go", "build", "./...")

	case "node":
		if !q.hasNodeBuildScript() {
			output.Result = GateSkip
			output.Output = "No build script in package.json"
			return output
		}
		return q.runCommand(output, "npm", "run", "build")

	case "python":
		// Python doesn't typically have a build step for pure Python
		output.Result = GateSkip
		output.Output = "Python projects typically don't require building"
		return output

	default:
		output.Result = GateSkip
		output.Output = "Unknown project type, cannot run build"
		return output
	}
}

// runLint runs the linter for the project.
func (q *QualityGates) runLint() *GateOutput {
	output := &GateOutput{
		Gate: "lint",
	}

	projectType := q.detectProjectType()

	start := time.Now()
	defer func() {
		output.Duration = time.Since(start)
	}()

	switch projectType {
	case "go":
		// Use go vet as a basic linter, or golangci-lint if available
		if q.commandExists("golangci-lint") {
			return q.runCommand(output, "golangci-lint", "run", "./...")
		}
		return q.runCommand(output, "go", "vet", "./...")

	case "node":
		if !q.hasNodeLintScript() {
			output.Result = GateSkip
			output.Output = "No lint script in package.json"
			return output
		}
		return q.runCommand(output, "npm", "run", "lint")

	case "python":
		if q.commandExists("ruff") {
			return q.runCommand(output, "ruff", "check", ".")
		} else if q.commandExists("flake8") {
			return q.runCommand(output, "flake8", ".")
		}
		output.Result = GateSkip
		output.Output = "No Python linter (ruff, flake8) found"
		return output

	default:
		output.Result = GateSkip
		output.Output = "Unknown project type, cannot run lint"
		return output
	}
}

// runTypecheck runs type checking for the project.
func (q *QualityGates) runTypecheck() *GateOutput {
	output := &GateOutput{
		Gate: "typecheck",
	}

	projectType := q.detectProjectType()

	start := time.Now()
	defer func() {
		output.Duration = time.Since(start)
	}()

	switch projectType {
	case "go":
		// Go doesn't have a separate typecheck; it's part of build/vet
		output.Result = GateSkip
		output.Output = "Go type checking is handled by build gate"
		return output

	case "node":
		// Check for TypeScript
		if !q.hasTypeScript() {
			output.Result = GateSkip
			output.Output = "Not a TypeScript project"
			return output
		}
		return q.runCommand(output, "npx", "tsc", "--noEmit")

	case "python":
		if q.commandExists("mypy") {
			return q.runCommand(output, "mypy", ".")
		}
		output.Result = GateSkip
		output.Output = "mypy not found"
		return output

	default:
		output.Result = GateSkip
		output.Output = "Unknown project type, cannot run typecheck"
		return output
	}
}

// runCommand executes a command and populates the GateOutput.
func (q *QualityGates) runCommand(output *GateOutput, name string, args ...string) *GateOutput {
	ctx, cancel := context.WithTimeout(context.Background(), q.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = q.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Combine stdout and stderr
	var combined strings.Builder
	if stdout.Len() > 0 {
		combined.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		if combined.Len() > 0 {
			combined.WriteString("\n")
		}
		combined.WriteString(stderr.String())
	}
	output.Output = combined.String()

	if ctx.Err() == context.DeadlineExceeded {
		output.Result = GateError
		output.Output = "Command timed out: " + output.Output
		return output
	}

	if err != nil {
		// Check if it's an exit error (command ran but failed)
		if _, ok := err.(*exec.ExitError); ok {
			output.Result = GateFail
		} else {
			// Command failed to start or other error
			output.Result = GateError
			output.Output = "Error running command: " + err.Error() + "\n" + output.Output
		}
	} else {
		output.Result = GatePass
	}

	return output
}

// detectProjectType determines the type of project in the work directory.
func (q *QualityGates) detectProjectType() string {
	// Check for Go project
	if _, err := os.Stat(filepath.Join(q.workDir, "go.mod")); err == nil {
		return "go"
	}

	// Check for Node.js project
	if _, err := os.Stat(filepath.Join(q.workDir, "package.json")); err == nil {
		return "node"
	}

	// Check for Python project
	if _, err := os.Stat(filepath.Join(q.workDir, "setup.py")); err == nil {
		return "python"
	}
	if _, err := os.Stat(filepath.Join(q.workDir, "pyproject.toml")); err == nil {
		return "python"
	}
	if _, err := os.Stat(filepath.Join(q.workDir, "requirements.txt")); err == nil {
		return "python"
	}

	return "unknown"
}

// hasGoTestFiles checks if the project has any Go test files.
func (q *QualityGates) hasGoTestFiles() bool {
	found := false
	filepath.Walk(q.workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, "_test.go") {
			found = true
			return filepath.SkipAll
		}
		// Skip vendor and hidden directories
		if info.IsDir() && (info.Name() == "vendor" || strings.HasPrefix(info.Name(), ".")) {
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

// hasNodeTestScript checks if package.json has a test script.
func (q *QualityGates) hasNodeTestScript() bool {
	return q.hasNodeScript("test")
}

// hasNodeBuildScript checks if package.json has a build script.
func (q *QualityGates) hasNodeBuildScript() bool {
	return q.hasNodeScript("build")
}

// hasNodeLintScript checks if package.json has a lint script.
func (q *QualityGates) hasNodeLintScript() bool {
	return q.hasNodeScript("lint")
}

// hasNodeScript checks if package.json has a specific script.
func (q *QualityGates) hasNodeScript(script string) bool {
	content, err := os.ReadFile(filepath.Join(q.workDir, "package.json"))
	if err != nil {
		return false
	}
	// Simple check - look for "script": in the scripts section
	// A more robust solution would parse the JSON
	return strings.Contains(string(content), `"`+script+`"`)
}

// hasTypeScript checks if the project uses TypeScript.
func (q *QualityGates) hasTypeScript() bool {
	// Check for tsconfig.json
	if _, err := os.Stat(filepath.Join(q.workDir, "tsconfig.json")); err == nil {
		return true
	}
	return false
}

// hasPythonTests checks if the project has Python test files.
func (q *QualityGates) hasPythonTests() bool {
	// Check for tests directory
	testsDir := filepath.Join(q.workDir, "tests")
	if _, err := os.Stat(testsDir); err == nil {
		return true
	}

	// Check for test_ files in the root
	found := false
	entries, err := os.ReadDir(q.workDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "test_") && strings.HasSuffix(entry.Name(), ".py") {
			found = true
			break
		}
	}
	return found
}

// commandExists checks if a command is available in PATH.
func (q *QualityGates) commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
