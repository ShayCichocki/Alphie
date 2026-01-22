// Package merge provides critical file detection and smart merging for package manager files.
package merge

import (
	"path/filepath"
	"strings"
)

// CriticalFilePatterns are files that often cause merge conflicts when touched
// by multiple agents. These are checked during merge to trigger smart merge logic.
var CriticalFilePatterns = []string{
	// JavaScript/TypeScript ecosystem
	"package.json",
	"package-lock.json",
	"yarn.lock",
	"pnpm-lock.yaml",
	".npmrc",

	// Go ecosystem
	"go.mod",
	"go.sum",

	// Rust ecosystem
	"Cargo.toml",
	"Cargo.lock",

	// Python ecosystem
	"pyproject.toml",
	"requirements.txt",
	"setup.py",
	"poetry.lock",
	"Pipfile",
	"Pipfile.lock",

	// Ruby ecosystem
	"Gemfile",
	"Gemfile.lock",

	// Java ecosystem
	"pom.xml",
	"build.gradle",
	"build.gradle.kts",

	// .NET ecosystem
	"packages.config",

	// PHP ecosystem
	"composer.json",
	"composer.lock",

	// Root configs (language-agnostic)
	"tsconfig.json",
	"jsconfig.json",
	"Makefile",
	"Dockerfile",
	"docker-compose.yml",
	"docker-compose.yaml",
	".gitignore",
	".gitattributes",
}

// CriticalWildcardPatterns are glob patterns for critical files.
var CriticalWildcardPatterns = []string{
	".eslintrc*",
	".prettierrc*",
	"*.csproj",
	"*.sln",
	".env*",
}

// MonorepoSubdirs are common subdirectory names in monorepo structures.
// Package manager files in these directories are treated as critical.
var MonorepoSubdirs = []string{
	"client",
	"server",
	"frontend",
	"backend",
	"web",
	"api",
	"app",
	"apps",
	"packages",
	"services",
	"libs",
	"shared",
}

// CriticalPackageFiles are the basenames of package manager files that are critical
// regardless of whether they're at root or in a monorepo subdirectory.
var CriticalPackageFiles = []string{
	"package.json",
	"go.mod",
	"Cargo.toml",
	"pyproject.toml",
	"tsconfig.json",
}

// LockFiles are files that should be regenerated rather than merged.
// After merging the main config (e.g., package.json), these should be regenerated.
var LockFiles = map[string]string{
	"package-lock.json": "npm install",
	"yarn.lock":         "yarn install",
	"pnpm-lock.yaml":    "pnpm install",
	"go.sum":            "go mod tidy",
	"Cargo.lock":        "cargo build",
	"poetry.lock":       "poetry lock",
	"Pipfile.lock":      "pipenv lock",
	"Gemfile.lock":      "bundle install",
	"composer.lock":     "composer install",
}

// IsCriticalFile checks if a file path matches any critical file pattern.
func IsCriticalFile(path string) bool {
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimPrefix(path, "/")

	base := filepath.Base(path)
	dir := filepath.Dir(path)

	isRoot := !strings.Contains(path, "/") || path == base

	if isRoot {
		for _, pattern := range CriticalFilePatterns {
			if base == pattern {
				return true
			}
		}

		for _, pattern := range CriticalWildcardPatterns {
			if matched, _ := filepath.Match(pattern, base); matched {
				return true
			}
		}
	}

	if isInMonorepoSubdir(dir) && isCriticalPackageFile(base) {
		return true
	}

	return false
}

func isInMonorepoSubdir(dir string) bool {
	parts := strings.Split(dir, "/")
	if len(parts) == 0 {
		return false
	}

	firstDir := parts[0]
	for _, subdir := range MonorepoSubdirs {
		if strings.EqualFold(firstDir, subdir) {
			return true
		}
	}

	if len(parts) >= 2 {
		parentDir := parts[0]
		if strings.EqualFold(parentDir, "packages") ||
			strings.EqualFold(parentDir, "apps") ||
			strings.EqualFold(parentDir, "services") ||
			strings.EqualFold(parentDir, "libs") {
			return true
		}
	}

	return false
}

func isCriticalPackageFile(base string) bool {
	for _, pkg := range CriticalPackageFiles {
		if base == pkg {
			return true
		}
	}
	return false
}

// IsLockFile checks if a file is a lock file that should be regenerated.
func IsLockFile(path string) bool {
	base := filepath.Base(path)
	_, isLock := LockFiles[base]
	return isLock
}

// GetLockFileCommand returns the command to regenerate a lock file.
func GetLockFileCommand(path string) string {
	base := filepath.Base(path)
	return LockFiles[base]
}

// GetCriticalFilesFromList filters a list of file paths to only critical files.
func GetCriticalFilesFromList(files []string) []string {
	var critical []string
	for _, f := range files {
		if IsCriticalFile(f) {
			critical = append(critical, f)
		}
	}
	return critical
}

// HasCriticalFileOverlap checks if two lists of files have overlapping critical files.
func HasCriticalFileOverlap(files1, files2 []string) bool {
	critical1 := make(map[string]bool)
	for _, f := range files1 {
		if IsCriticalFile(f) {
			critical1[filepath.Base(f)] = true
		}
	}

	for _, f := range files2 {
		if IsCriticalFile(f) {
			if critical1[filepath.Base(f)] {
				return true
			}
		}
	}

	return false
}

// CategorizeCriticalFiles separates files into those that can be smart-merged
// and those that should be regenerated (lock files).
func CategorizeCriticalFiles(files []string) (mergeable, regenerate []string) {
	for _, f := range files {
		if IsLockFile(f) {
			regenerate = append(regenerate, f)
		} else if IsCriticalFile(f) {
			mergeable = append(mergeable, f)
		}
	}
	return
}

// CodeExtensions are file extensions that indicate source code files.
var CodeExtensions = []string{
	".go", ".js", ".ts", ".jsx", ".tsx", ".py", ".rb", ".java",
	".c", ".cpp", ".h", ".hpp", ".rs", ".swift", ".kt", ".scala",
	".cs", ".php", ".vue", ".svelte",
}

// IsCodeFile returns true if the file is a source code file.
func IsCodeFile(path string) bool {
	for _, ext := range CodeExtensions {
		if strings.HasSuffix(strings.ToLower(path), ext) {
			return true
		}
	}
	return false
}
