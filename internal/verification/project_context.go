// Package verification provides project context detection for verification contracts.
package verification

import (
	"os"
	"path/filepath"
	"strings"
)

// ProjectContext contains information about the project's structure and conventions.
type ProjectContext struct {
	Type           string   // "go", "node", "rust", "python", "unknown"
	TestCommand    []string // Command to run tests
	BuildCommand   []string // Command to build
	TestPatterns   []string // Test file patterns (e.g., "*_test.go", "*.test.ts")
	TestDirectories []string // Common test directories
	LintCommand    []string // Command to run linters
}

// DetectProjectContext analyzes a repository and returns project-specific context.
func DetectProjectContext(repoPath string) *ProjectContext {
	ctx := &ProjectContext{}

	// Detect project type by examining files
	if fileExistsAtPath(filepath.Join(repoPath, "go.mod")) {
		ctx.Type = "go"
		ctx.TestCommand = []string{"go", "test", "./..."}
		ctx.BuildCommand = []string{"go", "build", "./..."}
		ctx.TestPatterns = []string{"*_test.go"}
		ctx.TestDirectories = []string{} // Go tests are inline
		ctx.LintCommand = []string{"go", "vet", "./..."}

	} else if fileExistsAtPath(filepath.Join(repoPath, "Cargo.toml")) {
		ctx.Type = "rust"
		ctx.TestCommand = []string{"cargo", "test"}
		ctx.BuildCommand = []string{"cargo", "build"}
		ctx.TestPatterns = []string{"*_test.rs", "tests/*.rs"}
		ctx.TestDirectories = []string{"tests"}
		ctx.LintCommand = []string{"cargo", "clippy"}

	} else if fileExistsAtPath(filepath.Join(repoPath, "pyproject.toml")) ||
		fileExistsAtPath(filepath.Join(repoPath, "setup.py")) ||
		fileExistsAtPath(filepath.Join(repoPath, "requirements.txt")) {
		ctx.Type = "python"

		// Detect test framework
		if dirExists(filepath.Join(repoPath, "tests")) || fileExistsAtPath(filepath.Join(repoPath, "pytest.ini")) {
			ctx.TestCommand = []string{"pytest"}
		} else {
			ctx.TestCommand = []string{"python", "-m", "unittest", "discover"}
		}

		ctx.BuildCommand = []string{"python", "-m", "py_compile"} // Basic syntax check
		ctx.TestPatterns = []string{"test_*.py", "*_test.py"}
		ctx.TestDirectories = []string{"tests", "test"}

		// Check for common linters
		if hasExecutable("ruff") || fileExistsAtPath(filepath.Join(repoPath, ".ruff.toml")) {
			ctx.LintCommand = []string{"ruff", "check", "."}
		} else if hasExecutable("pylint") {
			ctx.LintCommand = []string{"pylint", "**/*.py"}
		}

	} else if fileExistsAtPath(filepath.Join(repoPath, "package.json")) {
		ctx.Type = "node"

		// Detect test framework from package.json
		pkgData, _ := os.ReadFile(filepath.Join(repoPath, "package.json"))
		pkgContent := string(pkgData)

		if strings.Contains(pkgContent, `"test"`) {
			ctx.TestCommand = []string{"npm", "test"}
		} else if strings.Contains(pkgContent, "jest") {
			ctx.TestCommand = []string{"npx", "jest"}
		} else if strings.Contains(pkgContent, "vitest") {
			ctx.TestCommand = []string{"npx", "vitest", "run"}
		} else if strings.Contains(pkgContent, "mocha") {
			ctx.TestCommand = []string{"npx", "mocha"}
		}

		// Detect build command
		if strings.Contains(pkgContent, `"build"`) {
			ctx.BuildCommand = []string{"npm", "run", "build"}
		} else if fileExistsAtPath(filepath.Join(repoPath, "tsconfig.json")) {
			ctx.BuildCommand = []string{"npx", "tsc", "--noEmit"}
		}

		ctx.TestPatterns = []string{"*.test.ts", "*.test.js", "*.spec.ts", "*.spec.js"}
		ctx.TestDirectories = []string{"test", "tests", "__tests__"}

		// Detect linter
		if strings.Contains(pkgContent, "eslint") {
			ctx.LintCommand = []string{"npx", "eslint", "."}
		}

	} else {
		ctx.Type = "unknown"
	}

	return ctx
}

// EnhanceContractWithProjectContext adds project-specific verification commands to a contract.
func EnhanceContractWithProjectContext(contract *Contract, ctx *ProjectContext) {
	if contract == nil || ctx == nil {
		return
	}

	// Add test command if available and not already present
	if len(ctx.TestCommand) > 0 {
		testCmdStr := strings.Join(ctx.TestCommand, " ")
		if !hasCommand(contract, testCmdStr) {
			contract.Commands = append(contract.Commands, Command{
				Command:     testCmdStr,
				Expect:      "exit 0",
				Description: "Project tests pass",
				Required:    true,
			})
		}
	}

	// Add build/compile command if available
	if len(ctx.BuildCommand) > 0 {
		buildCmdStr := strings.Join(ctx.BuildCommand, " ")
		if !hasCommand(contract, buildCmdStr) {
			contract.Commands = append(contract.Commands, Command{
				Command:     buildCmdStr,
				Expect:      "exit 0",
				Description: "Project builds/compiles successfully",
				Required:    true,
			})
		}
	}

	// Add lint command if available (as optional check)
	if len(ctx.LintCommand) > 0 {
		lintCmdStr := strings.Join(ctx.LintCommand, " ")
		if !hasCommand(contract, lintCmdStr) {
			contract.Commands = append(contract.Commands, Command{
				Command:     lintCmdStr,
				Expect:      "exit 0",
				Description: "Linting passes",
				Required:    false, // Linting is optional
			})
		}
	}
}

// hasCommand checks if a contract already has a command.
func hasCommand(contract *Contract, cmdStr string) bool {
	for _, cmd := range contract.Commands {
		if cmd.Command == cmdStr || strings.Contains(cmd.Command, cmdStr) {
			return true
		}
	}
	return false
}

// fileExistsAtPath checks if a file exists at the given path.
func fileExistsAtPath(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// dirExists checks if a directory exists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// hasExecutable checks if an executable is available in PATH.
func hasExecutable(name string) bool {
	_, err := filepath.Abs(name) // Simple check, could be improved
	return err == nil
}
