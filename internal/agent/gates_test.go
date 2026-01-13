package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGateResult_String(t *testing.T) {
	tests := []struct {
		result GateResult
		want   string
	}{
		{GatePass, "pass"},
		{GateFail, "fail"},
		{GateSkip, "skip"},
		{GateError, "error"},
		{GateResult(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.result.String(); got != tt.want {
				t.Errorf("String() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestNewQualityGates_Defaults(t *testing.T) {
	qg := NewQualityGates("/tmp/test")

	// Check all gates are disabled by default
	results, err := qg.RunGates()
	if err != nil {
		t.Fatalf("RunGates() error = %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results with no gates enabled, got %d", len(results))
	}
}

func TestQualityGates_EnableDisable(t *testing.T) {
	qg := NewQualityGates("/tmp/test")

	// Enable all gates
	qg.EnableTest(true)
	qg.EnableBuild(true)
	qg.EnableLint(true)
	qg.EnableTypecheck(true)

	// Note: RunGates will skip or error for non-existent directory,
	// but the gates will still be attempted
}

func TestQualityGates_SetTimeout(t *testing.T) {
	qg := NewQualityGates("/tmp/test")
	qg.SetTimeout(10 * time.Second)

	// Timeout is internal, just verify no panic
}

func TestQualityGates_DetectProjectType(t *testing.T) {
	// Create temp directories for different project types
	tests := []struct {
		name      string
		files     []string
		wantType  string
	}{
		{
			name:     "go project",
			files:    []string{"go.mod"},
			wantType: "go",
		},
		{
			name:     "node project",
			files:    []string{"package.json"},
			wantType: "node",
		},
		{
			name:     "python setup.py",
			files:    []string{"setup.py"},
			wantType: "python",
		},
		{
			name:     "python pyproject.toml",
			files:    []string{"pyproject.toml"},
			wantType: "python",
		},
		{
			name:     "python requirements.txt",
			files:    []string{"requirements.txt"},
			wantType: "python",
		},
		{
			name:     "unknown project",
			files:    []string{"README.md"},
			wantType: "unknown",
		},
		{
			name:     "go over node",
			files:    []string{"go.mod", "package.json"},
			wantType: "go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir, err := os.MkdirTemp("", "gate-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			// Create marker files
			for _, f := range tt.files {
				path := filepath.Join(tmpDir, f)
				if err := os.WriteFile(path, []byte{}, 0644); err != nil {
					t.Fatalf("Failed to create file %s: %v", f, err)
				}
			}

			qg := NewQualityGates(tmpDir)
			got := qg.detectProjectType()
			if got != tt.wantType {
				t.Errorf("detectProjectType() = %s, want %s", got, tt.wantType)
			}
		})
	}
}

func TestQualityGates_HasGoTestFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gate-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	qg := NewQualityGates(tmpDir)

	// No test files initially
	if qg.hasGoTestFiles() {
		t.Error("hasGoTestFiles() should be false with no test files")
	}

	// Create a test file
	testFile := filepath.Join(tmpDir, "foo_test.go")
	if err := os.WriteFile(testFile, []byte("package foo"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	if !qg.hasGoTestFiles() {
		t.Error("hasGoTestFiles() should be true with test file")
	}
}

func TestQualityGates_HasNodeScript(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gate-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	qg := NewQualityGates(tmpDir)

	// No package.json
	if qg.hasNodeTestScript() {
		t.Error("hasNodeTestScript() should be false without package.json")
	}

	// Create package.json with test script
	pkgJSON := `{
		"name": "test-pkg",
		"scripts": {
			"test": "jest",
			"build": "tsc",
			"lint": "eslint"
		}
	}`
	pkgPath := filepath.Join(tmpDir, "package.json")
	if err := os.WriteFile(pkgPath, []byte(pkgJSON), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	if !qg.hasNodeTestScript() {
		t.Error("hasNodeTestScript() should be true with test script")
	}
	if !qg.hasNodeBuildScript() {
		t.Error("hasNodeBuildScript() should be true with build script")
	}
	if !qg.hasNodeLintScript() {
		t.Error("hasNodeLintScript() should be true with lint script")
	}
}

func TestQualityGates_HasTypeScript(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gate-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	qg := NewQualityGates(tmpDir)

	// No tsconfig.json
	if qg.hasTypeScript() {
		t.Error("hasTypeScript() should be false without tsconfig.json")
	}

	// Create tsconfig.json
	tsconfig := filepath.Join(tmpDir, "tsconfig.json")
	if err := os.WriteFile(tsconfig, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create tsconfig.json: %v", err)
	}

	if !qg.hasTypeScript() {
		t.Error("hasTypeScript() should be true with tsconfig.json")
	}
}

func TestQualityGates_HasPythonTests(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gate-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	qg := NewQualityGates(tmpDir)

	// No tests
	if qg.hasPythonTests() {
		t.Error("hasPythonTests() should be false without test files")
	}

	// Create tests directory
	testsDir := filepath.Join(tmpDir, "tests")
	if err := os.MkdirAll(testsDir, 0755); err != nil {
		t.Fatalf("Failed to create tests dir: %v", err)
	}

	if !qg.hasPythonTests() {
		t.Error("hasPythonTests() should be true with tests directory")
	}
}

func TestQualityGates_HasPythonTests_TestFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gate-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	qg := NewQualityGates(tmpDir)

	// Create test_ file
	testFile := filepath.Join(tmpDir, "test_foo.py")
	if err := os.WriteFile(testFile, []byte("def test_foo(): pass"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	if !qg.hasPythonTests() {
		t.Error("hasPythonTests() should be true with test_ file")
	}
}

func TestGateOutput_Fields(t *testing.T) {
	output := &GateOutput{
		Gate:     "test",
		Result:   GatePass,
		Output:   "All tests passed",
		Duration: 5 * time.Second,
	}

	if output.Gate != "test" {
		t.Errorf("Gate = %s, want test", output.Gate)
	}
	if output.Result != GatePass {
		t.Errorf("Result = %v, want GatePass", output.Result)
	}
	if output.Output != "All tests passed" {
		t.Errorf("Output = %s, want 'All tests passed'", output.Output)
	}
	if output.Duration != 5*time.Second {
		t.Errorf("Duration = %v, want 5s", output.Duration)
	}
}

func TestQualityGates_RunGates_SkipUnknownProject(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gate-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	qg := NewQualityGates(tmpDir)
	qg.EnableTest(true)
	qg.EnableBuild(true)

	results, err := qg.RunGates()
	if err != nil {
		t.Fatalf("RunGates() error = %v", err)
	}

	// All gates should skip for unknown project type
	for _, r := range results {
		if r.Result != GateSkip {
			t.Errorf("Gate %s should skip for unknown project, got %v", r.Gate, r.Result)
		}
	}
}

func TestQualityGates_RunGates_GoProject_NoTests(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gate-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create go.mod
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644); err != nil {
		t.Fatalf("Failed to create go.mod: %v", err)
	}

	qg := NewQualityGates(tmpDir)
	qg.EnableTest(true)

	results, err := qg.RunGates()
	if err != nil {
		t.Fatalf("RunGates() error = %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	if results[0].Result != GateSkip {
		t.Errorf("Test gate should skip with no test files, got %v", results[0].Result)
	}
}
