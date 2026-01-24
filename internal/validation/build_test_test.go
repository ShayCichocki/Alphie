package validation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAutoBuildTesterGo tests the auto-detection for Go projects.
func TestAutoBuildTesterGo(t *testing.T) {
	// Create a temporary directory with go.mod
	tmpDir := t.TempDir()
	goModPath := filepath.Join(tmpDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module test\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("Failed to create go.mod: %v", err)
	}

	// Create auto build tester
	tester, err := NewAutoBuildTester(tmpDir, 5*time.Second)
	if err != nil {
		t.Fatalf("NewAutoBuildTester failed: %v", err)
	}

	// Verify it detected Go
	if len(tester.buildCmd) == 0 || tester.buildCmd[0] != "go" {
		t.Errorf("Expected Go build command, got: %v", tester.buildCmd)
	}
	if len(tester.testCmd) == 0 || tester.testCmd[0] != "go" {
		t.Errorf("Expected Go test command, got: %v", tester.testCmd)
	}
}

// TestAutoBuildTesterNode tests the auto-detection for Node.js projects.
func TestAutoBuildTesterNode(t *testing.T) {
	// Create a temporary directory with package.json
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "package.json")
	if err := os.WriteFile(pkgPath, []byte(`{"name": "test"}`), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Create auto build tester
	tester, err := NewAutoBuildTester(tmpDir, 5*time.Second)
	if err != nil {
		t.Fatalf("NewAutoBuildTester failed: %v", err)
	}

	// Verify it detected Node
	if len(tester.buildCmd) == 0 || tester.buildCmd[0] != "npm" {
		t.Errorf("Expected npm build command, got: %v", tester.buildCmd)
	}
	if len(tester.testCmd) == 0 || tester.testCmd[0] != "npm" {
		t.Errorf("Expected npm test command, got: %v", tester.testCmd)
	}
}

// TestAutoBuildTesterUnknown tests handling of unknown project types.
func TestAutoBuildTesterUnknown(t *testing.T) {
	// Create a temporary directory with no project files
	tmpDir := t.TempDir()

	// Create auto build tester
	tester, err := NewAutoBuildTester(tmpDir, 5*time.Second)
	if err != nil {
		t.Fatalf("NewAutoBuildTester failed: %v", err)
	}

	// Verify it has no commands (will skip)
	if len(tester.buildCmd) != 0 {
		t.Errorf("Expected no build command for unknown type, got: %v", tester.buildCmd)
	}
	if len(tester.testCmd) != 0 {
		t.Errorf("Expected no test command for unknown type, got: %v", tester.testCmd)
	}
}

// TestSimpleBuildTesterSkipWhenNoCmds tests that tester skips when no commands configured.
func TestSimpleBuildTesterSkipWhenNoCmds(t *testing.T) {
	tester := NewSimpleBuildTester(nil, nil, 5*time.Second)

	passed, output, err := tester.RunBuildAndTests(context.Background(), "/tmp")
	if err != nil {
		t.Errorf("Expected no error when skipping, got: %v", err)
	}
	if !passed {
		t.Error("Expected passed=true when no commands configured")
	}
	if output != "No build or test commands configured (skipped)" {
		t.Errorf("Unexpected output: %s", output)
	}
}

// TestSimpleBuildTesterRealGo tests building the actual alphie project.
func TestSimpleBuildTesterRealGo(t *testing.T) {
	// Get the project root (3 levels up from internal/validation)
	projectRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("Failed to get project root: %v", err)
	}

	// Create tester for Go
	tester := NewSimpleBuildTester(
		[]string{"go", "build", "./..."},
		[]string{"go", "test", "./internal/validation"},
		30*time.Second,
	)

	// Run build and tests
	ctx := context.Background()
	passed, output, err := tester.RunBuildAndTests(ctx, projectRoot)

	// We expect the build to succeed since we just compiled it
	if err != nil {
		t.Logf("Build/test output:\n%s", output)
		t.Errorf("Build/test failed: %v", err)
	}

	if !passed {
		t.Logf("Build/test output:\n%s", output)
		t.Error("Expected build/test to pass")
	}

	// Verify output contains expected markers
	if output == "" {
		t.Error("Expected non-empty output")
	}
}
