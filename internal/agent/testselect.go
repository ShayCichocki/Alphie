// Package agent provides the AI agent implementation for Alphie.
package agent

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// FocusedTestSelector selects tests relevant to changed files.
// It uses co-located test patterns (file.go -> file_test.go),
// package scope expansion when insufficient tests are found,
// and tag-based test selection for path prefix to test tag mapping.
type FocusedTestSelector struct {
	repoPath   string
	minTests   int
	tagMapping map[string][]string // pathPrefix → test tags
}

// DefaultTagMapping returns the default path prefix to test tag mappings.
// These are common mappings that can be overridden via SetTagMapping.
func DefaultTagMapping() map[string][]string {
	return map[string][]string{
		"auth": {"@auth"},
		"api":  {"@api"},
		"db":   {"@db"},
	}
}

// NewFocusedTestSelector creates a new FocusedTestSelector for the given repository.
// It initializes with default tag mappings for common path prefixes.
func NewFocusedTestSelector(repoPath string) *FocusedTestSelector {
	return &FocusedTestSelector{
		repoPath:   repoPath,
		minTests:   5,
		tagMapping: DefaultTagMapping(),
	}
}

// SetMinTests sets the minimum number of tests before expanding to package scope.
func (f *FocusedTestSelector) SetMinTests(min int) {
	f.minTests = min
}

// SetTagMapping sets the path prefix to test tag mappings.
// Pass nil to disable tag-based selection entirely.
// The mapping keys are path prefixes (e.g., "auth", "src/auth"),
// and values are lists of test tags (e.g., ["@auth", "@security"]).
func (f *FocusedTestSelector) SetTagMapping(mapping map[string][]string) {
	f.tagMapping = mapping
}

// AddTagMapping adds or updates a single path prefix to test tag mapping.
// If the prefix already exists, the tags are replaced.
func (f *FocusedTestSelector) AddTagMapping(pathPrefix string, tags []string) {
	if f.tagMapping == nil {
		f.tagMapping = make(map[string][]string)
	}
	f.tagMapping[pathPrefix] = tags
}

// GetTagMapping returns the current tag mapping configuration.
func (f *FocusedTestSelector) GetTagMapping() map[string][]string {
	return f.tagMapping
}

// SelectTestResult contains the results of test selection including
// both specific test files and test tags for tag-based selection.
type SelectTestResult struct {
	// TestFiles contains specific test file paths to run.
	TestFiles []string
	// TestTags contains test tags (e.g., "@auth") for tag-based selection.
	// These tags can be used with `go test -run` or similar test runners.
	TestTags []string
}

// SelectTests returns test files relevant to the given changed files.
// It first finds co-located tests, then expands to package scope if needed.
// This is a convenience wrapper around SelectTestsWithTags that only returns files.
func (f *FocusedTestSelector) SelectTests(changedFiles []string) ([]string, error) {
	result, err := f.SelectTestsWithTags(changedFiles)
	if err != nil {
		return nil, err
	}
	return result.TestFiles, nil
}

// SelectTestsWithTags returns test selection results including both test files
// and test tags relevant to the given changed files.
// It follows a 3-level selection strategy:
//  1. Co-located tests: file.go -> file_test.go
//  2. Package scope: expand to all tests in the package if < minTests found
//  3. Tag-based: map path prefixes to test tags (e.g., src/auth/* -> @auth)
func (f *FocusedTestSelector) SelectTestsWithTags(changedFiles []string) (*SelectTestResult, error) {
	testFiles := make(map[string]struct{})
	testTags := make(map[string]struct{})

	// Step 1: Find co-located tests for each changed file
	for _, file := range changedFiles {
		colocated := f.GetColocated(file)
		if colocated != "" {
			fullPath := filepath.Join(f.repoPath, colocated)
			if _, err := os.Stat(fullPath); err == nil {
				testFiles[colocated] = struct{}{}
			}
		}
	}

	// Step 2: If < minTests found, expand to package scope
	if len(testFiles) < f.minTests {
		pkgsSeen := make(map[string]struct{})
		for _, file := range changedFiles {
			pkgPath := filepath.Dir(file)
			if _, seen := pkgsSeen[pkgPath]; !seen {
				pkgsSeen[pkgPath] = struct{}{}
				pkgTests, err := f.GetPackageTests(pkgPath)
				if err != nil {
					return nil, err
				}
				for _, t := range pkgTests {
					testFiles[t] = struct{}{}
				}
			}
		}
	}

	// Step 3: Find tag-based tests based on path prefixes
	for _, file := range changedFiles {
		tags := f.GetTagsForPath(file)
		for _, tag := range tags {
			testTags[tag] = struct{}{}
		}
	}

	// Convert maps to slices
	fileResult := make([]string, 0, len(testFiles))
	for t := range testFiles {
		fileResult = append(fileResult, t)
	}

	tagResult := make([]string, 0, len(testTags))
	for t := range testTags {
		tagResult = append(tagResult, t)
	}

	return &SelectTestResult{
		TestFiles: fileResult,
		TestTags:  tagResult,
	}, nil
}

// GetColocated returns the co-located test file for a given source file.
// For example, handler.go -> handler_test.go
// Returns empty string if the file is already a test file or not a Go file.
func (f *FocusedTestSelector) GetColocated(file string) string {
	// Skip non-Go files
	if !strings.HasSuffix(file, ".go") {
		return ""
	}

	// Skip if already a test file
	if strings.HasSuffix(file, "_test.go") {
		return file
	}

	// Convert source file to test file
	base := strings.TrimSuffix(file, ".go")
	return base + "_test.go"
}

// GetPackageTests returns all test files in the given package directory.
func (f *FocusedTestSelector) GetPackageTests(pkgPath string) ([]string, error) {
	fullPath := filepath.Join(f.repoPath, pkgPath)
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var tests []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, "_test.go") {
			tests = append(tests, filepath.Join(pkgPath, name))
		}
	}

	return tests, nil
}

// GetTagsForPath returns the test tags that match the given file path.
// It checks all configured path prefixes and returns tags for any that match.
// For example, if tagMapping has "auth" -> ["@auth"] and the file is
// "src/auth/handler.go", this returns ["@auth"].
func (f *FocusedTestSelector) GetTagsForPath(file string) []string {
	if f.tagMapping == nil {
		return nil
	}

	var result []string
	seen := make(map[string]struct{})

	// Normalize path separators
	normalizedFile := filepath.ToSlash(file)

	for prefix, tags := range f.tagMapping {
		normalizedPrefix := filepath.ToSlash(prefix)

		// Check if the file path contains the prefix as a path component
		// This handles cases like:
		// - "auth" matches "src/auth/handler.go" and "auth/handler.go"
		// - "src/auth" matches "src/auth/handler.go" but not "other/src/auth/x.go"
		if pathContainsPrefix(normalizedFile, normalizedPrefix) {
			for _, tag := range tags {
				if _, exists := seen[tag]; !exists {
					seen[tag] = struct{}{}
					result = append(result, tag)
				}
			}
		}
	}

	return result
}

// pathContainsPrefix checks if a file path contains the given prefix as a path component.
// For example:
// - pathContainsPrefix("src/auth/handler.go", "auth") returns true
// - pathContainsPrefix("auth/handler.go", "auth") returns true
// - pathContainsPrefix("other/authentication/x.go", "auth") returns false
// - pathContainsPrefix("src/auth/handler.go", "src/auth") returns true
func pathContainsPrefix(file, prefix string) bool {
	// Split both paths into components
	fileComponents := strings.Split(file, "/")
	prefixComponents := strings.Split(prefix, "/")

	if len(prefixComponents) > len(fileComponents) {
		return false
	}

	// Try to match prefix at each position in file path
	for i := 0; i <= len(fileComponents)-len(prefixComponents); i++ {
		match := true
		for j := 0; j < len(prefixComponents); j++ {
			if fileComponents[i+j] != prefixComponents[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}

	return false
}

// BuildTestRunPattern creates a regex pattern for running tests with specific tags.
// This pattern can be used with `go test -run <pattern>`.
// For example, tags ["@auth", "@api"] would produce "Test.*(@auth|@api)".
func BuildTestRunPattern(tags []string) string {
	if len(tags) == 0 {
		return ""
	}

	if len(tags) == 1 {
		return "Test.*" + tags[0]
	}

	// Join multiple tags with OR
	return "Test.*(" + strings.Join(tags, "|") + ")"
}

// GetCallerTests finds tests for functions that call exported functions in the changed file.
// It parses Go files in the repo, finds call sites of exported functions from the changed file,
// and returns the test files for those calling packages.
func (f *FocusedTestSelector) GetCallerTests(changedFile string) ([]string, error) {
	// Skip non-Go files and test files
	if !strings.HasSuffix(changedFile, ".go") || strings.HasSuffix(changedFile, "_test.go") {
		return nil, nil
	}

	// Extract exported function names from the changed file
	exportedFuncs, err := f.getExportedFunctions(changedFile)
	if err != nil || len(exportedFuncs) == 0 {
		return nil, err
	}

	// Find files that call these exported functions
	callerFiles, err := f.findCallers(exportedFuncs)
	if err != nil {
		return nil, err
	}

	// Get test files for the caller files
	testFiles := make(map[string]struct{})
	for _, caller := range callerFiles {
		colocated := f.GetColocated(caller)
		if colocated != "" {
			fullPath := filepath.Join(f.repoPath, colocated)
			if _, err := os.Stat(fullPath); err == nil {
				testFiles[colocated] = struct{}{}
			}
		}
	}

	result := make([]string, 0, len(testFiles))
	for t := range testFiles {
		result = append(result, t)
	}

	return result, nil
}

// getExportedFunctions parses a Go file and returns the names of exported functions.
func (f *FocusedTestSelector) getExportedFunctions(file string) ([]string, error) {
	fullPath := filepath.Join(f.repoPath, file)

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, fullPath, nil, 0)
	if err != nil {
		return nil, err
	}

	var exported []string
	for _, decl := range node.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			// Skip methods (have a receiver)
			if fn.Recv != nil {
				continue
			}
			name := fn.Name.Name
			// Check if exported (starts with uppercase)
			if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
				exported = append(exported, name)
			}
		}
	}

	return exported, nil
}

// findCallers searches all Go files in the repo for calls to the given function names.
func (f *FocusedTestSelector) findCallers(funcNames []string) ([]string, error) {
	funcSet := make(map[string]struct{}, len(funcNames))
	for _, name := range funcNames {
		funcSet[name] = struct{}{}
	}

	var callers []string

	err := filepath.Walk(f.repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip directories and non-Go files
		if info.IsDir() {
			// Skip common non-source directories
			name := info.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" || name == ".worktrees" {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Parse the file and look for calls
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil // Skip files that don't parse
		}

		found := false
		ast.Inspect(node, func(n ast.Node) bool {
			if found {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Check for direct function call: FuncName()
			if ident, ok := call.Fun.(*ast.Ident); ok {
				if _, exists := funcSet[ident.Name]; exists {
					found = true
					return false
				}
			}

			// Check for package-qualified call: pkg.FuncName()
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if _, exists := funcSet[sel.Sel.Name]; exists {
					found = true
					return false
				}
			}

			return true
		})

		if found {
			relPath, err := filepath.Rel(f.repoPath, path)
			if err == nil {
				callers = append(callers, relPath)
			}
		}

		return nil
	})

	return callers, err
}
