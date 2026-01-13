package orchestrator

import (
	"testing"
)

func TestExtractFilesFromDiff(t *testing.T) {
	tests := []struct {
		name     string
		diff     string
		expected []string
	}{
		{
			name:     "empty diff",
			diff:     "",
			expected: nil,
		},
		{
			name: "single file",
			diff: `diff --git a/internal/foo.go b/internal/foo.go
index abc123..def456 100644
--- a/internal/foo.go
+++ b/internal/foo.go
@@ -1,3 +1,4 @@
 package foo
+// added comment`,
			expected: []string{"internal/foo.go"},
		},
		{
			name: "multiple files",
			diff: `diff --git a/file1.go b/file1.go
index abc123..def456 100644
--- a/file1.go
+++ b/file1.go
@@ -1,1 +1,2 @@
+new line
diff --git a/file2.go b/file2.go
index 111222..333444 100644
--- a/file2.go
+++ b/file2.go
@@ -5,1 +5,2 @@
+another line`,
			expected: []string{"file1.go", "file2.go"},
		},
		{
			name: "nested paths",
			diff: `diff --git a/internal/orchestrator/merger.go b/internal/orchestrator/merger.go
index abc..def 100644
--- a/internal/orchestrator/merger.go
+++ b/internal/orchestrator/merger.go
@@ -1 +1 @@
-old
+new`,
			expected: []string{"internal/orchestrator/merger.go"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractFilesFromDiff(tc.diff)
			if len(result) != len(tc.expected) {
				t.Errorf("expected %d files, got %d", len(tc.expected), len(result))
				return
			}
			for i, file := range result {
				if file != tc.expected[i] {
					t.Errorf("expected file[%d] = %q, got %q", i, tc.expected[i], file)
				}
			}
		})
	}
}

func TestExtractFunctionsFromDiff(t *testing.T) {
	tests := []struct {
		name     string
		diff     string
		expected []string
	}{
		{
			name:     "empty diff",
			diff:     "",
			expected: nil,
		},
		{
			name: "function in hunk header",
			diff: `@@ -10,5 +10,6 @@ func ProcessRequest(ctx context.Context) error {
 	// existing code
+	// new code`,
			expected: []string{"ProcessRequest"},
		},
		{
			name: "added function",
			diff: `+func NewHandler() *Handler {
+	return &Handler{}
+}`,
			expected: []string{"NewHandler"},
		},
		{
			name: "removed function",
			diff: `-func OldFunc() {
-	// old code
-}`,
			expected: []string{"OldFunc"},
		},
		{
			name: "method with receiver",
			diff: `+func (m *Manager) Execute() error {
+	return nil
+}`,
			expected: []string{"Execute"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractFunctionsFromDiff(tc.diff)
			if len(result) != len(tc.expected) {
				t.Errorf("expected %d functions, got %d: %v", len(tc.expected), len(result), result)
				return
			}
			for i, fn := range result {
				if fn != tc.expected[i] {
					t.Errorf("expected func[%d] = %q, got %q", i, tc.expected[i], fn)
				}
			}
		})
	}
}

func TestAreDisjoint(t *testing.T) {
	tests := []struct {
		name     string
		a        []string
		b        []string
		expected bool
	}{
		{
			name:     "both empty",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "one empty",
			a:        []string{"file1.go"},
			b:        nil,
			expected: true,
		},
		{
			name:     "disjoint",
			a:        []string{"file1.go", "file2.go"},
			b:        []string{"file3.go", "file4.go"},
			expected: true,
		},
		{
			name:     "overlapping",
			a:        []string{"file1.go", "file2.go"},
			b:        []string{"file2.go", "file3.go"},
			expected: false,
		},
		{
			name:     "identical",
			a:        []string{"file1.go"},
			b:        []string{"file1.go"},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := areDisjoint(tc.a, tc.b)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestParseMergeResponse(t *testing.T) {
	tests := []struct {
		name        string
		response    string
		expectError bool
		expectFiles int
	}{
		{
			name:        "empty response",
			response:    "",
			expectError: true,
		},
		{
			name:        "no JSON",
			response:    "This is just text without any JSON",
			expectError: true,
		},
		{
			name: "valid response",
			response: `Here is my analysis:
{
  "merged_files": {
    "file1.go": "package main\n\nfunc main() {}\n",
    "file2.go": "package utils\n"
  },
  "reasoning": "Merged both changes successfully"
}`,
			expectError: false,
			expectFiles: 2,
		},
		{
			name: "JSON with extra text",
			response: `Let me resolve this conflict.

{
  "merged_files": {
    "internal/handler.go": "package internal\n\nfunc Handle() error { return nil }\n"
  },
  "reasoning": "Combined the error handling from both branches"
}

That should work.`,
			expectError: false,
			expectFiles: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseMergeResponse(tc.response)
			if tc.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if len(result.MergedFiles) != tc.expectFiles {
				t.Errorf("expected %d files, got %d", tc.expectFiles, len(result.MergedFiles))
			}
		})
	}
}

func TestSemanticMergerCanAutoMerge(t *testing.T) {
	merger := &SemanticMerger{
		repoPath: "/tmp/test",
	}

	tests := []struct {
		name     string
		diff1    string
		diff2    string
		expected bool
	}{
		{
			name:     "empty diffs",
			diff1:    "",
			diff2:    "",
			expected: true, // Empty slices are disjoint
		},
		{
			name: "disjoint files",
			diff1: `diff --git a/file1.go b/file1.go
--- a/file1.go
+++ b/file1.go
@@ -1 +1 @@
-old
+new`,
			diff2: `diff --git a/file2.go b/file2.go
--- a/file2.go
+++ b/file2.go
@@ -1 +1 @@
-old
+new`,
			expected: true,
		},
		{
			name: "same file different functions",
			diff1: `diff --git a/handler.go b/handler.go
--- a/handler.go
+++ b/handler.go
@@ -10,5 +10,6 @@ func FuncA() {
 	// code
+	// more code`,
			diff2: `diff --git a/handler.go b/handler.go
--- a/handler.go
+++ b/handler.go
@@ -50,5 +50,6 @@ func FuncB() {
 	// code
+	// more code`,
			expected: true, // Different functions
		},
		{
			name: "same file same function",
			diff1: `diff --git a/handler.go b/handler.go
--- a/handler.go
+++ b/handler.go
@@ -10,5 +10,6 @@ func SharedFunc() {
 	// code A
+	// more code A`,
			diff2: `diff --git a/handler.go b/handler.go
--- a/handler.go
+++ b/handler.go
@@ -10,5 +10,6 @@ func SharedFunc() {
 	// code B
+	// more code B`,
			expected: false, // Same function
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := merger.CanAutoMerge(tc.diff1, tc.diff2)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestSemanticMergeResultFields(t *testing.T) {
	// Test that SemanticMergeResult has the expected fields
	result := SemanticMergeResult{
		Success:     true,
		MergedFiles: []string{"file1.go", "file2.go"},
		NeedsHuman:  false,
		Reason:      "Merged successfully",
	}

	if !result.Success {
		t.Error("expected Success to be true")
	}
	if len(result.MergedFiles) != 2 {
		t.Errorf("expected 2 merged files, got %d", len(result.MergedFiles))
	}
	if result.NeedsHuman {
		t.Error("expected NeedsHuman to be false")
	}
	if result.Reason != "Merged successfully" {
		t.Errorf("unexpected reason: %s", result.Reason)
	}
}

func TestNewSemanticMerger(t *testing.T) {
	merger := NewSemanticMerger(nil, "/tmp/test-repo")

	if merger == nil {
		t.Fatal("expected non-nil merger")
	}
	if merger.repoPath != "/tmp/test-repo" {
		t.Errorf("expected repoPath '/tmp/test-repo', got %q", merger.repoPath)
	}
}

func TestCanAutoMerge_StrictConditions(t *testing.T) {
	merger := &SemanticMerger{
		repoPath: "/tmp/test",
	}

	tests := []struct {
		name     string
		diff1    string
		diff2    string
		expected bool
		reason   string
	}{
		{
			name: "completely disjoint directories",
			diff1: `diff --git a/internal/api/handler.go b/internal/api/handler.go
--- a/internal/api/handler.go
+++ b/internal/api/handler.go
@@ -1 +1 @@
-old
+new`,
			diff2: `diff --git a/pkg/models/user.go b/pkg/models/user.go
--- a/pkg/models/user.go
+++ b/pkg/models/user.go
@@ -1 +1 @@
-old
+new`,
			expected: true,
			reason:   "files in different directories should be disjoint",
		},
		{
			name: "test files vs source files",
			diff1: `diff --git a/internal/handler.go b/internal/handler.go
--- a/internal/handler.go
+++ b/internal/handler.go
@@ -5,5 +5,6 @@ func Handle() {
 	// code
+	// addition`,
			diff2: `diff --git a/internal/handler_test.go b/internal/handler_test.go
--- a/internal/handler_test.go
+++ b/internal/handler_test.go
@@ -10,5 +10,6 @@ func TestHandle() {
 	// test code
+	// new test`,
			expected: true,
			reason:   "source and test files should be disjoint",
		},
		{
			name: "overlapping file - same function modified",
			diff1: `diff --git a/internal/service.go b/internal/service.go
--- a/internal/service.go
+++ b/internal/service.go
@@ -10,5 +10,6 @@ func Process() {
 	// branch A change
+	doA()`,
			diff2: `diff --git a/internal/service.go b/internal/service.go
--- a/internal/service.go
+++ b/internal/service.go
@@ -10,5 +10,6 @@ func Process() {
 	// branch B change
+	doB()`,
			expected: false,
			reason:   "same function modified in both - needs semantic merge",
		},
		{
			name: "multiple files - one overlaps same function",
			diff1: `diff --git a/file1.go b/file1.go
--- a/file1.go
+++ b/file1.go
@@ -1 +1 @@
-old
+new
diff --git a/shared.go b/shared.go
--- a/shared.go
+++ b/shared.go
@@ -10,5 +10,6 @@ func SharedFunc() {
 	// code
+	doA()`,
			diff2: `diff --git a/file2.go b/file2.go
--- a/file2.go
+++ b/file2.go
@@ -1 +1 @@
-old
+new
diff --git a/shared.go b/shared.go
--- a/shared.go
+++ b/shared.go
@@ -10,5 +10,6 @@ func SharedFunc() {
 	// code
+	doB()`,
			expected: false,
			reason:   "shared.go overlaps with same function - needs semantic merge",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := merger.CanAutoMerge(tc.diff1, tc.diff2)
			if result != tc.expected {
				t.Errorf("%s: expected %v, got %v", tc.reason, tc.expected, result)
			}
		})
	}
}

func TestExtractFilesFromDiff_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		diff     string
		expected int
	}{
		{
			name:     "whitespace only",
			diff:     "   \n\n\t\t  ",
			expected: 0,
		},
		{
			name: "malformed diff header",
			diff: `diff --git broken
+++ something`,
			expected: 0,
		},
		{
			name: "binary file diff",
			diff: `diff --git a/image.png b/image.png
Binary files a/image.png and b/image.png differ`,
			expected: 1,
		},
		{
			name: "new file mode",
			diff: `diff --git a/newfile.go b/newfile.go
new file mode 100644
index 0000000..abc1234
--- /dev/null
+++ b/newfile.go
@@ -0,0 +1,5 @@
+package main
+
+func main() {
+}`,
			expected: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractFilesFromDiff(tc.diff)
			if len(result) != tc.expected {
				t.Errorf("expected %d files, got %d: %v", tc.expected, len(result), result)
			}
		})
	}
}
