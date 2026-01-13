package agent

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestNewFocusedTestSelector(t *testing.T) {
	selector := NewFocusedTestSelector("/repo/path")
	if selector.repoPath != "/repo/path" {
		t.Errorf("repoPath = %s, want /repo/path", selector.repoPath)
	}
	if selector.minTests != 5 {
		t.Errorf("minTests = %d, want 5", selector.minTests)
	}
}

func TestFocusedTestSelector_SetMinTests(t *testing.T) {
	selector := NewFocusedTestSelector("/repo")
	selector.SetMinTests(10)
	if selector.minTests != 10 {
		t.Errorf("minTests = %d, want 10", selector.minTests)
	}
}

func TestFocusedTestSelector_GetColocated(t *testing.T) {
	selector := NewFocusedTestSelector("/repo")

	tests := []struct {
		name string
		file string
		want string
	}{
		{
			name: "regular Go file",
			file: "pkg/handler.go",
			want: "pkg/handler_test.go",
		},
		{
			name: "nested path",
			file: "internal/api/service/user.go",
			want: "internal/api/service/user_test.go",
		},
		{
			name: "root level file",
			file: "main.go",
			want: "main_test.go",
		},
		{
			name: "already test file",
			file: "pkg/handler_test.go",
			want: "pkg/handler_test.go",
		},
		{
			name: "non-Go file",
			file: "README.md",
			want: "",
		},
		{
			name: "JavaScript file",
			file: "src/app.js",
			want: "",
		},
		{
			name: "YAML file",
			file: "config.yaml",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selector.GetColocated(tt.file)
			if got != tt.want {
				t.Errorf("GetColocated(%q) = %q, want %q", tt.file, got, tt.want)
			}
		})
	}
}

func TestFocusedTestSelector_GetPackageTests(t *testing.T) {
	// Create temp repo structure
	tmpDir, err := os.MkdirTemp("", "testselect-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create package structure
	pkgDir := filepath.Join(tmpDir, "pkg", "handlers")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("Failed to create pkg dir: %v", err)
	}

	// Create source files
	files := []string{
		"user.go",
		"user_test.go",
		"auth.go",
		"auth_test.go",
		"helper.go",
		"middleware_test.go",
	}
	for _, f := range files {
		path := filepath.Join(pkgDir, f)
		if err := os.WriteFile(path, []byte("package handlers"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", f, err)
		}
	}

	selector := NewFocusedTestSelector(tmpDir)
	tests, err := selector.GetPackageTests("pkg/handlers")
	if err != nil {
		t.Fatalf("GetPackageTests() error = %v", err)
	}

	if len(tests) != 3 {
		t.Errorf("Expected 3 test files, got %d", len(tests))
	}

	// Check that all test files are found
	expected := map[string]bool{
		"pkg/handlers/user_test.go":       true,
		"pkg/handlers/auth_test.go":       true,
		"pkg/handlers/middleware_test.go": true,
	}
	for _, test := range tests {
		if !expected[test] {
			t.Errorf("Unexpected test file: %s", test)
		}
	}
}

func TestFocusedTestSelector_GetPackageTests_NonExistent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testselect-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	selector := NewFocusedTestSelector(tmpDir)
	tests, err := selector.GetPackageTests("nonexistent/package")

	// Should not error for non-existent path
	if err != nil {
		t.Errorf("GetPackageTests() error = %v, want nil", err)
	}

	if tests != nil && len(tests) > 0 {
		t.Errorf("Expected nil or empty slice, got %v", tests)
	}
}

func TestFocusedTestSelector_SelectTests_ColocatedOnly(t *testing.T) {
	// Create temp repo structure
	tmpDir, err := os.MkdirTemp("", "testselect-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create package with files
	pkgDir := filepath.Join(tmpDir, "pkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("Failed to create pkg dir: %v", err)
	}

	files := []string{
		"a.go", "a_test.go",
		"b.go", "b_test.go",
		"c.go", "c_test.go",
		"d.go", "d_test.go",
		"e.go", "e_test.go",
		"f.go", "f_test.go",
	}
	for _, f := range files {
		path := filepath.Join(pkgDir, f)
		if err := os.WriteFile(path, []byte("package pkg"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", f, err)
		}
	}

	selector := NewFocusedTestSelector(tmpDir)
	selector.SetMinTests(5)

	// Change 6 files (more than minTests)
	changed := []string{
		"pkg/a.go",
		"pkg/b.go",
		"pkg/c.go",
		"pkg/d.go",
		"pkg/e.go",
		"pkg/f.go",
	}

	tests, err := selector.SelectTests(changed)
	if err != nil {
		t.Fatalf("SelectTests() error = %v", err)
	}

	if len(tests) != 6 {
		t.Errorf("Expected 6 tests (colocated), got %d", len(tests))
	}
}

func TestFocusedTestSelector_SelectTests_PackageExpansion(t *testing.T) {
	// Create temp repo structure
	tmpDir, err := os.MkdirTemp("", "testselect-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create package with many test files
	pkgDir := filepath.Join(tmpDir, "pkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("Failed to create pkg dir: %v", err)
	}

	// Create files - only some have colocated tests
	files := []string{
		"a.go", "a_test.go",
		"b.go", "b_test.go",
		"c.go", // no test
		"helper.go",
		"extra_test.go", // orphan test
		"integration_test.go",
	}
	for _, f := range files {
		path := filepath.Join(pkgDir, f)
		if err := os.WriteFile(path, []byte("package pkg"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", f, err)
		}
	}

	selector := NewFocusedTestSelector(tmpDir)
	selector.SetMinTests(5)

	// Change 2 files (below minTests, should trigger expansion)
	changed := []string{
		"pkg/a.go",
		"pkg/c.go", // has no colocated test
	}

	tests, err := selector.SelectTests(changed)
	if err != nil {
		t.Fatalf("SelectTests() error = %v", err)
	}

	// Should include all tests in the package due to expansion
	if len(tests) < 4 {
		t.Errorf("Expected at least 4 tests (package expansion), got %d", len(tests))
	}

	// Verify extra_test and integration_test are included
	testSet := make(map[string]bool)
	for _, test := range tests {
		testSet[test] = true
	}

	if !testSet["pkg/extra_test.go"] {
		t.Error("Expected extra_test.go to be included in package expansion")
	}
	if !testSet["pkg/integration_test.go"] {
		t.Error("Expected integration_test.go to be included in package expansion")
	}
}

func TestFocusedTestSelector_SelectTests_MultiplePackages(t *testing.T) {
	// Create temp repo structure
	tmpDir, err := os.MkdirTemp("", "testselect-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create two packages
	pkg1Dir := filepath.Join(tmpDir, "pkg1")
	pkg2Dir := filepath.Join(tmpDir, "pkg2")
	if err := os.MkdirAll(pkg1Dir, 0755); err != nil {
		t.Fatalf("Failed to create pkg1 dir: %v", err)
	}
	if err := os.MkdirAll(pkg2Dir, 0755); err != nil {
		t.Fatalf("Failed to create pkg2 dir: %v", err)
	}

	// Create files in both packages
	pkg1Files := []string{"a.go", "a_test.go", "b_test.go"}
	pkg2Files := []string{"x.go", "x_test.go", "y_test.go"}

	for _, f := range pkg1Files {
		path := filepath.Join(pkg1Dir, f)
		if err := os.WriteFile(path, []byte("package pkg1"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}
	for _, f := range pkg2Files {
		path := filepath.Join(pkg2Dir, f)
		if err := os.WriteFile(path, []byte("package pkg2"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	selector := NewFocusedTestSelector(tmpDir)
	selector.SetMinTests(5)

	// Change files in both packages
	changed := []string{"pkg1/a.go", "pkg2/x.go"}

	tests, err := selector.SelectTests(changed)
	if err != nil {
		t.Fatalf("SelectTests() error = %v", err)
	}

	// Should expand both packages since < minTests
	if len(tests) < 4 {
		t.Errorf("Expected at least 4 tests, got %d", len(tests))
	}
}

func TestFocusedTestSelector_SelectTests_NoDuplicates(t *testing.T) {
	// Create temp repo structure
	tmpDir, err := os.MkdirTemp("", "testselect-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create package
	pkgDir := filepath.Join(tmpDir, "pkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("Failed to create pkg dir: %v", err)
	}

	files := []string{"a.go", "a_test.go"}
	for _, f := range files {
		path := filepath.Join(pkgDir, f)
		if err := os.WriteFile(path, []byte("package pkg"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	selector := NewFocusedTestSelector(tmpDir)
	selector.SetMinTests(5) // Will trigger expansion

	// Include both source and test file in changed
	changed := []string{"pkg/a.go", "pkg/a_test.go"}

	tests, err := selector.SelectTests(changed)
	if err != nil {
		t.Fatalf("SelectTests() error = %v", err)
	}

	// Count occurrences - should be no duplicates
	counts := make(map[string]int)
	for _, test := range tests {
		counts[test]++
	}

	for test, count := range counts {
		if count > 1 {
			t.Errorf("Duplicate test file: %s (count=%d)", test, count)
		}
	}
}

func TestFocusedTestSelector_SelectTests_EmptyInput(t *testing.T) {
	selector := NewFocusedTestSelector("/repo")

	tests, err := selector.SelectTests([]string{})
	if err != nil {
		t.Fatalf("SelectTests() error = %v", err)
	}

	if len(tests) != 0 {
		t.Errorf("Expected 0 tests for empty input, got %d", len(tests))
	}
}

func TestFocusedTestSelector_SelectTests_NonGoFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testselect-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	selector := NewFocusedTestSelector(tmpDir)

	// Only non-Go files changed
	changed := []string{
		"README.md",
		"config.yaml",
		".gitignore",
	}

	tests, err := selector.SelectTests(changed)
	if err != nil {
		t.Fatalf("SelectTests() error = %v", err)
	}

	if len(tests) != 0 {
		t.Errorf("Expected 0 tests for non-Go files, got %d", len(tests))
	}
}

func TestFocusedTestSelector_SelectTests_TestFileOnly(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testselect-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create package with test file
	pkgDir := filepath.Join(tmpDir, "pkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("Failed to create pkg dir: %v", err)
	}

	testFile := filepath.Join(pkgDir, "foo_test.go")
	if err := os.WriteFile(testFile, []byte("package pkg"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	selector := NewFocusedTestSelector(tmpDir)
	selector.SetMinTests(1) // Won't trigger expansion

	// Only test file changed
	changed := []string{"pkg/foo_test.go"}

	tests, err := selector.SelectTests(changed)
	if err != nil {
		t.Fatalf("SelectTests() error = %v", err)
	}

	if len(tests) != 1 {
		t.Errorf("Expected 1 test, got %d", len(tests))
	}

	if len(tests) > 0 && tests[0] != "pkg/foo_test.go" {
		t.Errorf("Expected pkg/foo_test.go, got %s", tests[0])
	}
}

func TestFocusedTestSelector_SelectTests_Deterministic(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testselect-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create package
	pkgDir := filepath.Join(tmpDir, "pkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("Failed to create pkg dir: %v", err)
	}

	files := []string{"a.go", "a_test.go", "b.go", "b_test.go", "c.go", "c_test.go"}
	for _, f := range files {
		path := filepath.Join(pkgDir, f)
		if err := os.WriteFile(path, []byte("package pkg"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	selector := NewFocusedTestSelector(tmpDir)
	changed := []string{"pkg/a.go", "pkg/b.go", "pkg/c.go"}

	// Run multiple times and check for consistency
	var firstResult []string
	for i := 0; i < 5; i++ {
		tests, err := selector.SelectTests(changed)
		if err != nil {
			t.Fatalf("SelectTests() error = %v", err)
		}

		// Sort for comparison
		sort.Strings(tests)

		if i == 0 {
			firstResult = tests
		} else {
			if len(tests) != len(firstResult) {
				t.Errorf("Iteration %d: got %d tests, want %d", i, len(tests), len(firstResult))
			}
			for j, test := range tests {
				if j < len(firstResult) && test != firstResult[j] {
					t.Errorf("Iteration %d: mismatch at index %d: %s vs %s", i, j, test, firstResult[j])
				}
			}
		}
	}
}

// Tests for tag-based test selection (Level 3)

func TestDefaultTagMapping(t *testing.T) {
	mapping := DefaultTagMapping()

	expected := map[string][]string{
		"auth": {"@auth"},
		"api":  {"@api"},
		"db":   {"@db"},
	}

	if len(mapping) != len(expected) {
		t.Errorf("DefaultTagMapping() has %d entries, want %d", len(mapping), len(expected))
	}

	for prefix, tags := range expected {
		got, ok := mapping[prefix]
		if !ok {
			t.Errorf("DefaultTagMapping() missing prefix %q", prefix)
			continue
		}
		if len(got) != len(tags) {
			t.Errorf("DefaultTagMapping()[%q] = %v, want %v", prefix, got, tags)
		}
		for i, tag := range tags {
			if got[i] != tag {
				t.Errorf("DefaultTagMapping()[%q][%d] = %q, want %q", prefix, i, got[i], tag)
			}
		}
	}
}

func TestFocusedTestSelector_NewWithDefaultTags(t *testing.T) {
	selector := NewFocusedTestSelector("/repo")

	mapping := selector.GetTagMapping()
	if mapping == nil {
		t.Fatal("Expected default tag mapping to be set")
	}

	// Check default mappings exist
	if _, ok := mapping["auth"]; !ok {
		t.Error("Expected 'auth' mapping in defaults")
	}
	if _, ok := mapping["api"]; !ok {
		t.Error("Expected 'api' mapping in defaults")
	}
	if _, ok := mapping["db"]; !ok {
		t.Error("Expected 'db' mapping in defaults")
	}
}

func TestFocusedTestSelector_SetTagMapping(t *testing.T) {
	selector := NewFocusedTestSelector("/repo")

	// Set custom mapping
	customMapping := map[string][]string{
		"custom": {"@custom"},
		"other":  {"@other", "@related"},
	}
	selector.SetTagMapping(customMapping)

	got := selector.GetTagMapping()
	if len(got) != 2 {
		t.Errorf("GetTagMapping() has %d entries, want 2", len(got))
	}

	// Verify default mappings are replaced
	if _, ok := got["auth"]; ok {
		t.Error("Expected 'auth' mapping to be removed after SetTagMapping")
	}
}

func TestFocusedTestSelector_SetTagMappingNil(t *testing.T) {
	selector := NewFocusedTestSelector("/repo")

	// Disable tag-based selection
	selector.SetTagMapping(nil)

	got := selector.GetTagMapping()
	if got != nil {
		t.Errorf("GetTagMapping() = %v, want nil", got)
	}
}

func TestFocusedTestSelector_AddTagMapping(t *testing.T) {
	selector := NewFocusedTestSelector("/repo")

	// Add new mapping
	selector.AddTagMapping("security", []string{"@security", "@auth"})

	got := selector.GetTagMapping()
	if tags, ok := got["security"]; !ok {
		t.Error("Expected 'security' mapping to be added")
	} else if len(tags) != 2 || tags[0] != "@security" || tags[1] != "@auth" {
		t.Errorf("AddTagMapping() = %v, want [@security @auth]", tags)
	}

	// Original defaults should still exist
	if _, ok := got["auth"]; !ok {
		t.Error("Expected 'auth' default mapping to still exist")
	}
}

func TestFocusedTestSelector_AddTagMappingOverwrite(t *testing.T) {
	selector := NewFocusedTestSelector("/repo")

	// Overwrite existing mapping
	selector.AddTagMapping("auth", []string{"@authentication", "@login"})

	got := selector.GetTagMapping()
	tags := got["auth"]
	if len(tags) != 2 || tags[0] != "@authentication" || tags[1] != "@login" {
		t.Errorf("AddTagMapping() overwrite = %v, want [@authentication @login]", tags)
	}
}

func TestFocusedTestSelector_AddTagMappingToNil(t *testing.T) {
	selector := NewFocusedTestSelector("/repo")
	selector.SetTagMapping(nil)

	// Add mapping to nil map
	selector.AddTagMapping("new", []string{"@new"})

	got := selector.GetTagMapping()
	if got == nil {
		t.Fatal("Expected map to be created")
	}
	if _, ok := got["new"]; !ok {
		t.Error("Expected 'new' mapping to be added")
	}
}

func TestFocusedTestSelector_GetTagsForPath(t *testing.T) {
	selector := NewFocusedTestSelector("/repo")

	tests := []struct {
		name     string
		file     string
		wantTags []string
	}{
		{
			name:     "auth prefix in nested path",
			file:     "src/auth/handler.go",
			wantTags: []string{"@auth"},
		},
		{
			name:     "auth prefix at root",
			file:     "auth/login.go",
			wantTags: []string{"@auth"},
		},
		{
			name:     "api prefix",
			file:     "internal/api/routes.go",
			wantTags: []string{"@api"},
		},
		{
			name:     "db prefix",
			file:     "pkg/db/connection.go",
			wantTags: []string{"@db"},
		},
		{
			name:     "no matching prefix",
			file:     "src/utils/helper.go",
			wantTags: nil,
		},
		{
			name:     "partial match should not work",
			file:     "src/authentication/handler.go",
			wantTags: nil, // "authentication" != "auth"
		},
		{
			name:     "deeply nested auth",
			file:     "internal/services/auth/oauth/google.go",
			wantTags: []string{"@auth"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selector.GetTagsForPath(tt.file)

			if len(got) != len(tt.wantTags) {
				t.Errorf("GetTagsForPath(%q) = %v, want %v", tt.file, got, tt.wantTags)
				return
			}

			for i, tag := range tt.wantTags {
				if got[i] != tag {
					t.Errorf("GetTagsForPath(%q)[%d] = %q, want %q", tt.file, i, got[i], tag)
				}
			}
		})
	}
}

func TestFocusedTestSelector_GetTagsForPath_MultiPrefixMapping(t *testing.T) {
	selector := NewFocusedTestSelector("/repo")
	selector.SetTagMapping(map[string][]string{
		"src/auth": {"@auth", "@security"},
		"api":      {"@api"},
	})

	tests := []struct {
		name     string
		file     string
		wantTags []string
	}{
		{
			name:     "multi-component prefix",
			file:     "src/auth/handler.go",
			wantTags: []string{"@auth", "@security"},
		},
		{
			name:     "single component prefix in nested",
			file:     "internal/api/routes.go",
			wantTags: []string{"@api"},
		},
		{
			name:     "auth without src prefix should not match",
			file:     "other/auth/handler.go",
			wantTags: nil, // "src/auth" doesn't match "other/auth"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selector.GetTagsForPath(tt.file)

			if len(got) != len(tt.wantTags) {
				t.Errorf("GetTagsForPath(%q) = %v, want %v", tt.file, got, tt.wantTags)
				return
			}

			// Create a set for comparison since order may vary
			gotSet := make(map[string]bool)
			for _, tag := range got {
				gotSet[tag] = true
			}
			for _, tag := range tt.wantTags {
				if !gotSet[tag] {
					t.Errorf("GetTagsForPath(%q) missing tag %q", tt.file, tag)
				}
			}
		})
	}
}

func TestFocusedTestSelector_GetTagsForPath_NilMapping(t *testing.T) {
	selector := NewFocusedTestSelector("/repo")
	selector.SetTagMapping(nil)

	got := selector.GetTagsForPath("src/auth/handler.go")
	if got != nil {
		t.Errorf("GetTagsForPath() with nil mapping = %v, want nil", got)
	}
}

func TestFocusedTestSelector_SelectTestsWithTags(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testselect-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create auth package with test files
	authDir := filepath.Join(tmpDir, "src", "auth")
	if err := os.MkdirAll(authDir, 0755); err != nil {
		t.Fatalf("Failed to create auth dir: %v", err)
	}

	files := []string{"handler.go", "handler_test.go", "login.go", "login_test.go"}
	for _, f := range files {
		path := filepath.Join(authDir, f)
		if err := os.WriteFile(path, []byte("package auth"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", f, err)
		}
	}

	selector := NewFocusedTestSelector(tmpDir)
	selector.SetMinTests(10) // Force package expansion

	changed := []string{"src/auth/handler.go"}

	result, err := selector.SelectTestsWithTags(changed)
	if err != nil {
		t.Fatalf("SelectTestsWithTags() error = %v", err)
	}

	// Should have test files
	if len(result.TestFiles) == 0 {
		t.Error("Expected test files in result")
	}

	// Should have @auth tag
	hasAuthTag := false
	for _, tag := range result.TestTags {
		if tag == "@auth" {
			hasAuthTag = true
			break
		}
	}
	if !hasAuthTag {
		t.Errorf("Expected @auth tag in result, got %v", result.TestTags)
	}
}

func TestFocusedTestSelector_SelectTestsWithTags_MultiplePaths(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testselect-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create auth and api packages
	authDir := filepath.Join(tmpDir, "src", "auth")
	apiDir := filepath.Join(tmpDir, "src", "api")
	if err := os.MkdirAll(authDir, 0755); err != nil {
		t.Fatalf("Failed to create auth dir: %v", err)
	}
	if err := os.MkdirAll(apiDir, 0755); err != nil {
		t.Fatalf("Failed to create api dir: %v", err)
	}

	// Create files
	if err := os.WriteFile(filepath.Join(authDir, "handler.go"), []byte("package auth"), 0644); err != nil {
		t.Fatalf("Failed to create auth file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(apiDir, "routes.go"), []byte("package api"), 0644); err != nil {
		t.Fatalf("Failed to create api file: %v", err)
	}

	selector := NewFocusedTestSelector(tmpDir)

	changed := []string{
		"src/auth/handler.go",
		"src/api/routes.go",
	}

	result, err := selector.SelectTestsWithTags(changed)
	if err != nil {
		t.Fatalf("SelectTestsWithTags() error = %v", err)
	}

	// Should have both @auth and @api tags
	tagSet := make(map[string]bool)
	for _, tag := range result.TestTags {
		tagSet[tag] = true
	}

	if !tagSet["@auth"] {
		t.Error("Expected @auth tag in result")
	}
	if !tagSet["@api"] {
		t.Error("Expected @api tag in result")
	}
}

func TestFocusedTestSelector_SelectTestsWithTags_NoDuplicateTags(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testselect-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create auth package
	authDir := filepath.Join(tmpDir, "src", "auth")
	if err := os.MkdirAll(authDir, 0755); err != nil {
		t.Fatalf("Failed to create auth dir: %v", err)
	}

	files := []string{"handler.go", "login.go"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(authDir, f), []byte("package auth"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	selector := NewFocusedTestSelector(tmpDir)

	// Multiple files from same path prefix
	changed := []string{
		"src/auth/handler.go",
		"src/auth/login.go",
	}

	result, err := selector.SelectTestsWithTags(changed)
	if err != nil {
		t.Fatalf("SelectTestsWithTags() error = %v", err)
	}

	// Should only have @auth once
	authCount := 0
	for _, tag := range result.TestTags {
		if tag == "@auth" {
			authCount++
		}
	}
	if authCount != 1 {
		t.Errorf("Expected exactly 1 @auth tag, got %d", authCount)
	}
}

func TestPathContainsPrefix(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		prefix string
		want   bool
	}{
		{
			name:   "simple prefix at start",
			file:   "auth/handler.go",
			prefix: "auth",
			want:   true,
		},
		{
			name:   "prefix in middle",
			file:   "src/auth/handler.go",
			prefix: "auth",
			want:   true,
		},
		{
			name:   "multi-component prefix",
			file:   "src/auth/handler.go",
			prefix: "src/auth",
			want:   true,
		},
		{
			name:   "prefix not at component boundary",
			file:   "src/authentication/handler.go",
			prefix: "auth",
			want:   false,
		},
		{
			name:   "prefix longer than path",
			file:   "src/auth.go",
			prefix: "src/auth/deep",
			want:   false,
		},
		{
			name:   "exact match",
			file:   "auth",
			prefix: "auth",
			want:   true,
		},
		{
			name:   "deeply nested",
			file:   "a/b/c/d/auth/e/f.go",
			prefix: "auth",
			want:   true,
		},
		{
			name:   "multi-component at end",
			file:   "pkg/internal/api",
			prefix: "internal/api",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathContainsPrefix(tt.file, tt.prefix)
			if got != tt.want {
				t.Errorf("pathContainsPrefix(%q, %q) = %v, want %v", tt.file, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestBuildTestRunPattern(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		want string
	}{
		{
			name: "empty tags",
			tags: []string{},
			want: "",
		},
		{
			name: "nil tags",
			tags: nil,
			want: "",
		},
		{
			name: "single tag",
			tags: []string{"@auth"},
			want: "Test.*@auth",
		},
		{
			name: "multiple tags",
			tags: []string{"@auth", "@api"},
			want: "Test.*(@auth|@api)",
		},
		{
			name: "three tags",
			tags: []string{"@auth", "@api", "@db"},
			want: "Test.*(@auth|@api|@db)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildTestRunPattern(tt.tags)
			if got != tt.want {
				t.Errorf("BuildTestRunPattern(%v) = %q, want %q", tt.tags, got, tt.want)
			}
		})
	}
}

// Tests for caller test detection (Level 4)

func TestFocusedTestSelector_GetCallerTests(t *testing.T) {
	// Create temp repo structure
	tmpDir, err := os.MkdirTemp("", "testselect-caller-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create source package with exported function
	srcDir := filepath.Join(tmpDir, "pkg", "utils")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}

	// Create file with exported function
	utilsContent := `package utils

func ProcessData(input string) string {
	return input + "-processed"
}

func helperFunc() {}
`
	if err := os.WriteFile(filepath.Join(srcDir, "utils.go"), []byte(utilsContent), 0644); err != nil {
		t.Fatalf("Failed to create utils.go: %v", err)
	}

	// Create caller package
	callerDir := filepath.Join(tmpDir, "pkg", "handler")
	if err := os.MkdirAll(callerDir, 0755); err != nil {
		t.Fatalf("Failed to create caller dir: %v", err)
	}

	// Create file that calls the exported function
	handlerContent := `package handler

import "pkg/utils"

func Handle(input string) string {
	return utils.ProcessData(input)
}
`
	if err := os.WriteFile(filepath.Join(callerDir, "handler.go"), []byte(handlerContent), 0644); err != nil {
		t.Fatalf("Failed to create handler.go: %v", err)
	}

	// Create test file for caller
	handlerTestContent := `package handler

import "testing"

func TestHandle(t *testing.T) {}
`
	if err := os.WriteFile(filepath.Join(callerDir, "handler_test.go"), []byte(handlerTestContent), 0644); err != nil {
		t.Fatalf("Failed to create handler_test.go: %v", err)
	}

	selector := NewFocusedTestSelector(tmpDir)
	tests, err := selector.GetCallerTests("pkg/utils/utils.go")
	if err != nil {
		t.Fatalf("GetCallerTests() error = %v", err)
	}

	// Should find handler_test.go because handler.go calls ProcessData
	if len(tests) != 1 {
		t.Errorf("Expected 1 test file, got %d: %v", len(tests), tests)
	}

	if len(tests) > 0 && tests[0] != "pkg/handler/handler_test.go" {
		t.Errorf("Expected pkg/handler/handler_test.go, got %s", tests[0])
	}
}

func TestFocusedTestSelector_GetCallerTests_SkipsTestFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testselect-caller-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	selector := NewFocusedTestSelector(tmpDir)

	// Test files should return nil
	tests, err := selector.GetCallerTests("pkg/utils_test.go")
	if err != nil {
		t.Fatalf("GetCallerTests() error = %v", err)
	}

	if tests != nil {
		t.Errorf("Expected nil for test file, got %v", tests)
	}
}

func TestFocusedTestSelector_GetCallerTests_NonGoFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testselect-caller-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	selector := NewFocusedTestSelector(tmpDir)

	// Non-Go files should return nil
	tests, err := selector.GetCallerTests("README.md")
	if err != nil {
		t.Fatalf("GetCallerTests() error = %v", err)
	}

	if tests != nil {
		t.Errorf("Expected nil for non-Go file, got %v", tests)
	}
}

func TestFocusedTestSelector_GetCallerTests_NoExportedFunctions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testselect-caller-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create file with only unexported functions
	pkgDir := filepath.Join(tmpDir, "pkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("Failed to create pkg dir: %v", err)
	}

	content := `package pkg

func privateFunc() {}
func anotherPrivate() int { return 0 }
`
	if err := os.WriteFile(filepath.Join(pkgDir, "private.go"), []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create private.go: %v", err)
	}

	selector := NewFocusedTestSelector(tmpDir)
	tests, err := selector.GetCallerTests("pkg/private.go")
	if err != nil {
		t.Fatalf("GetCallerTests() error = %v", err)
	}

	if tests != nil {
		t.Errorf("Expected nil for file with no exported functions, got %v", tests)
	}
}

func TestFocusedTestSelector_GetCallerTests_MultipleCallers(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testselect-caller-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create shared library
	libDir := filepath.Join(tmpDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("Failed to create lib dir: %v", err)
	}

	libContent := `package lib

func SharedHelper(s string) string {
	return s
}
`
	if err := os.WriteFile(filepath.Join(libDir, "lib.go"), []byte(libContent), 0644); err != nil {
		t.Fatalf("Failed to create lib.go: %v", err)
	}

	// Create first caller
	caller1Dir := filepath.Join(tmpDir, "caller1")
	if err := os.MkdirAll(caller1Dir, 0755); err != nil {
		t.Fatalf("Failed to create caller1 dir: %v", err)
	}

	caller1Content := `package caller1

import "lib"

func Use1() string {
	return lib.SharedHelper("1")
}
`
	if err := os.WriteFile(filepath.Join(caller1Dir, "use1.go"), []byte(caller1Content), 0644); err != nil {
		t.Fatalf("Failed to create use1.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(caller1Dir, "use1_test.go"), []byte("package caller1"), 0644); err != nil {
		t.Fatalf("Failed to create use1_test.go: %v", err)
	}

	// Create second caller
	caller2Dir := filepath.Join(tmpDir, "caller2")
	if err := os.MkdirAll(caller2Dir, 0755); err != nil {
		t.Fatalf("Failed to create caller2 dir: %v", err)
	}

	caller2Content := `package caller2

import "lib"

func Use2() string {
	return lib.SharedHelper("2")
}
`
	if err := os.WriteFile(filepath.Join(caller2Dir, "use2.go"), []byte(caller2Content), 0644); err != nil {
		t.Fatalf("Failed to create use2.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(caller2Dir, "use2_test.go"), []byte("package caller2"), 0644); err != nil {
		t.Fatalf("Failed to create use2_test.go: %v", err)
	}

	selector := NewFocusedTestSelector(tmpDir)
	tests, err := selector.GetCallerTests("lib/lib.go")
	if err != nil {
		t.Fatalf("GetCallerTests() error = %v", err)
	}

	// Should find both callers' tests
	if len(tests) != 2 {
		t.Errorf("Expected 2 test files, got %d: %v", len(tests), tests)
	}

	testSet := make(map[string]bool)
	for _, test := range tests {
		testSet[test] = true
	}

	if !testSet["caller1/use1_test.go"] {
		t.Error("Expected caller1/use1_test.go to be included")
	}
	if !testSet["caller2/use2_test.go"] {
		t.Error("Expected caller2/use2_test.go to be included")
	}
}
