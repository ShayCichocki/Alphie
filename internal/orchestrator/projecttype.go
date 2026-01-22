// Package orchestrator provides project type detection for validation.
package orchestrator

import (
	"os"
	"path/filepath"
)

// ProjectType represents the primary language/framework of a project.
type ProjectType string

const (
	// ProjectTypeGo indicates a Go project (has go.mod).
	ProjectTypeGo ProjectType = "go"
	// ProjectTypeNode indicates a Node.js/JavaScript/TypeScript project (has package.json).
	ProjectTypeNode ProjectType = "node"
	// ProjectTypeRust indicates a Rust project (has Cargo.toml).
	ProjectTypeRust ProjectType = "rust"
	// ProjectTypePython indicates a Python project (has pyproject.toml or requirements.txt).
	ProjectTypePython ProjectType = "python"
	// ProjectTypeUnknown indicates the project type couldn't be detected.
	ProjectTypeUnknown ProjectType = "unknown"
)

// ProjectTypeInfo contains details about a detected project type.
type ProjectTypeInfo struct {
	// Type is the primary project type.
	Type ProjectType
	// BuildCommand is the command to build/compile the project.
	BuildCommand []string
	// TestCommand is the command to run tests.
	TestCommand []string
	// HasBuildScript indicates if the project has a custom build script.
	HasBuildScript bool
}

// DetectProjectType analyzes a directory and returns the project type.
// It checks for common project files in order of specificity.
func DetectProjectType(repoPath string) ProjectType {
	// Check for Go project
	if fileExists(filepath.Join(repoPath, "go.mod")) {
		return ProjectTypeGo
	}

	// Check for Rust project
	if fileExists(filepath.Join(repoPath, "Cargo.toml")) {
		return ProjectTypeRust
	}

	// Check for Python project (multiple indicators)
	if fileExists(filepath.Join(repoPath, "pyproject.toml")) ||
		fileExists(filepath.Join(repoPath, "setup.py")) ||
		fileExists(filepath.Join(repoPath, "requirements.txt")) {
		return ProjectTypePython
	}

	// Check for Node.js project (check last since it's common)
	if fileExists(filepath.Join(repoPath, "package.json")) {
		return ProjectTypeNode
	}

	return ProjectTypeUnknown
}

// GetProjectTypeInfo returns detailed information about the project type,
// including appropriate build and test commands.
func GetProjectTypeInfo(repoPath string) *ProjectTypeInfo {
	projectType := DetectProjectType(repoPath)

	info := &ProjectTypeInfo{
		Type: projectType,
	}

	switch projectType {
	case ProjectTypeGo:
		info.BuildCommand = []string{"go", "build", "./..."}
		info.TestCommand = []string{"go", "test", "./..."}

	case ProjectTypeNode:
		// Check if there's a build script in package.json
		info.HasBuildScript = hasNodeScript(repoPath, "build")
		if info.HasBuildScript {
			info.BuildCommand = []string{"npm", "run", "build"}
		} else {
			// For TypeScript projects without explicit build, try tsc
			if fileExists(filepath.Join(repoPath, "tsconfig.json")) {
				info.BuildCommand = []string{"npx", "tsc", "--noEmit"}
			}
			// No build command if neither exists
		}

		// Check for test script
		if hasNodeScript(repoPath, "test") {
			info.TestCommand = []string{"npm", "test"}
		}

	case ProjectTypeRust:
		info.BuildCommand = []string{"cargo", "build"}
		info.TestCommand = []string{"cargo", "test"}

	case ProjectTypePython:
		// Python validation is tricky, use type checking if available
		if fileExists(filepath.Join(repoPath, "pyproject.toml")) {
			info.BuildCommand = []string{"python", "-m", "py_compile"}
		}
		// Check for pytest or unittest
		if dirExists(filepath.Join(repoPath, "tests")) {
			info.TestCommand = []string{"python", "-m", "pytest"}
		}

	case ProjectTypeUnknown:
		// No commands for unknown projects
	}

	return info
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// dirExists checks if a directory exists at the given path.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// hasNodeScript checks if package.json has a specific script defined.
func hasNodeScript(repoPath, scriptName string) bool {
	pkgPath := filepath.Join(repoPath, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return false
	}

	// Simple check - look for "scriptName": in the scripts section
	// This is a heuristic to avoid full JSON parsing
	return containsScript(string(data), scriptName)
}

// containsScript is a simple heuristic to check if a script exists in package.json.
func containsScript(content, scriptName string) bool {
	// Look for "scripts" section and the script name
	// This is a simple string search, not a full JSON parse
	scriptsIdx := indexOf(content, `"scripts"`)
	if scriptsIdx == -1 {
		return false
	}

	// Look for the script name after the scripts section
	scriptPattern := `"` + scriptName + `"`
	return indexOf(content[scriptsIdx:], scriptPattern) != -1
}

// indexOf returns the index of substr in s, or -1 if not found.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
