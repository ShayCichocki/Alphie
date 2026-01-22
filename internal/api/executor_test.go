package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewToolExecutor(t *testing.T) {
	executor := NewToolExecutor("/tmp/test")

	if executor == nil {
		t.Fatal("NewToolExecutor returned nil")
	}
	if executor.workDir != "/tmp/test" {
		t.Errorf("workDir = %q, want %q", executor.workDir, "/tmp/test")
	}
}

func TestToolExecutor_UnknownTool(t *testing.T) {
	executor := NewToolExecutor("/tmp")

	result := executor.Execute(context.Background(), "UnknownTool", json.RawMessage(`{}`))

	if !result.IsError {
		t.Error("Expected error for unknown tool")
	}
	if !strings.Contains(result.Content, "Unknown tool") {
		t.Errorf("Error message = %q, should contain 'Unknown tool'", result.Content)
	}
}

func TestToolExecutor_Read(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "line1\nline2\nline3"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	executor := NewToolExecutor(tmpDir)

	input, _ := json.Marshal(map[string]interface{}{
		"file_path": testFile,
	})

	result := executor.Execute(context.Background(), "Read", input)

	if result.IsError {
		t.Fatalf("Read failed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "line1") {
		t.Error("Result should contain file content")
	}
	if !strings.Contains(result.Content, "1\t") {
		t.Error("Result should have line numbers")
	}
}

func TestToolExecutor_Read_NotFound(t *testing.T) {
	executor := NewToolExecutor("/tmp")

	input, _ := json.Marshal(map[string]interface{}{
		"file_path": "/nonexistent/file.txt",
	})

	result := executor.Execute(context.Background(), "Read", input)

	if !result.IsError {
		t.Error("Expected error for nonexistent file")
	}
}

func TestToolExecutor_Read_WithOffset(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "line1\nline2\nline3\nline4\nline5"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	executor := NewToolExecutor(tmpDir)

	input, _ := json.Marshal(map[string]interface{}{
		"file_path": testFile,
		"offset":    3,
		"limit":     2,
	})

	result := executor.Execute(context.Background(), "Read", input)

	if result.IsError {
		t.Fatalf("Read failed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "line3") {
		t.Error("Result should contain line3")
	}
	if !strings.Contains(result.Content, "line4") {
		t.Error("Result should contain line4")
	}
	if strings.Contains(result.Content, "line1") {
		t.Error("Result should not contain line1 (before offset)")
	}
	if strings.Contains(result.Content, "line5") {
		t.Error("Result should not contain line5 (after limit)")
	}
}

func TestToolExecutor_Write(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "newfile.txt")

	executor := NewToolExecutor(tmpDir)

	input, _ := json.Marshal(map[string]interface{}{
		"file_path": testFile,
		"content":   "hello world",
	})

	result := executor.Execute(context.Background(), "Write", input)

	if result.IsError {
		t.Fatalf("Write failed: %s", result.Content)
	}

	// Verify file was created
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("File content = %q, want %q", string(content), "hello world")
	}
}

func TestToolExecutor_Write_CreatesDirs(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "subdir", "nested", "file.txt")

	executor := NewToolExecutor(tmpDir)

	input, _ := json.Marshal(map[string]interface{}{
		"file_path": testFile,
		"content":   "nested content",
	})

	result := executor.Execute(context.Background(), "Write", input)

	if result.IsError {
		t.Fatalf("Write failed: %s", result.Content)
	}

	// Verify file was created with parent dirs
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("File was not created")
	}
}

func TestToolExecutor_Edit(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "edit.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	executor := NewToolExecutor(tmpDir)

	input, _ := json.Marshal(map[string]interface{}{
		"file_path":  testFile,
		"old_string": "world",
		"new_string": "universe",
	})

	result := executor.Execute(context.Background(), "Edit", input)

	if result.IsError {
		t.Fatalf("Edit failed: %s", result.Content)
	}

	// Verify edit was applied
	content, _ := os.ReadFile(testFile)
	if string(content) != "hello universe" {
		t.Errorf("File content = %q, want %q", string(content), "hello universe")
	}
}

func TestToolExecutor_Edit_NotUnique(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "edit.txt")
	if err := os.WriteFile(testFile, []byte("hello hello world"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	executor := NewToolExecutor(tmpDir)

	input, _ := json.Marshal(map[string]interface{}{
		"file_path":  testFile,
		"old_string": "hello",
		"new_string": "hi",
	})

	result := executor.Execute(context.Background(), "Edit", input)

	if !result.IsError {
		t.Error("Expected error for non-unique string")
	}
	if !strings.Contains(result.Content, "must be unique") {
		t.Errorf("Error = %q, should mention 'must be unique'", result.Content)
	}
}

func TestToolExecutor_Edit_ReplaceAll(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "edit.txt")
	if err := os.WriteFile(testFile, []byte("hello hello world"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	executor := NewToolExecutor(tmpDir)

	input, _ := json.Marshal(map[string]interface{}{
		"file_path":   testFile,
		"old_string":  "hello",
		"new_string":  "hi",
		"replace_all": true,
	})

	result := executor.Execute(context.Background(), "Edit", input)

	if result.IsError {
		t.Fatalf("Edit failed: %s", result.Content)
	}

	content, _ := os.ReadFile(testFile)
	if string(content) != "hi hi world" {
		t.Errorf("File content = %q, want %q", string(content), "hi hi world")
	}
}

func TestToolExecutor_Glob(t *testing.T) {
	tmpDir := t.TempDir()
	// Create some files
	os.WriteFile(filepath.Join(tmpDir, "file1.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte(""), 0644)

	executor := NewToolExecutor(tmpDir)

	input, _ := json.Marshal(map[string]interface{}{
		"pattern": "*.go",
		"path":    tmpDir,
	})

	result := executor.Execute(context.Background(), "Glob", input)

	if result.IsError {
		t.Fatalf("Glob failed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "file1.go") {
		t.Error("Result should contain file1.go")
	}
	if !strings.Contains(result.Content, "file2.go") {
		t.Error("Result should contain file2.go")
	}
	if strings.Contains(result.Content, "file.txt") {
		t.Error("Result should not contain file.txt")
	}
}

func TestToolExecutor_ListDir(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte(""), 0644)
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)

	executor := NewToolExecutor(tmpDir)

	input, _ := json.Marshal(map[string]interface{}{
		"path": tmpDir,
	})

	result := executor.Execute(context.Background(), "ListDir", input)

	if result.IsError {
		t.Fatalf("ListDir failed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "file1.txt") {
		t.Error("Result should contain file1.txt")
	}
	if !strings.Contains(result.Content, "subdir") {
		t.Error("Result should contain subdir")
	}
}

func TestToolExecutor_Bash(t *testing.T) {
	executor := NewToolExecutor("/tmp")

	input, _ := json.Marshal(map[string]interface{}{
		"command": "echo hello",
	})

	result := executor.Execute(context.Background(), "Bash", input)

	if result.IsError {
		t.Fatalf("Bash failed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "hello") {
		t.Errorf("Result = %q, should contain 'hello'", result.Content)
	}
}

func TestToolExecutor_Bash_Failure(t *testing.T) {
	executor := NewToolExecutor("/tmp")

	input, _ := json.Marshal(map[string]interface{}{
		"command": "exit 1",
	})

	result := executor.Execute(context.Background(), "Bash", input)

	if !result.IsError {
		t.Error("Expected error for failing command")
	}
}

func TestFormatToolAction_Read(t *testing.T) {
	input, _ := json.Marshal(map[string]interface{}{
		"file_path": "/path/to/file.go",
	})

	action := FormatToolAction("Read", input)

	if !strings.Contains(action, "file.go") {
		t.Errorf("Action = %q, should contain filename", action)
	}
}

func TestFormatToolAction_Write(t *testing.T) {
	input, _ := json.Marshal(map[string]interface{}{
		"file_path": "/path/to/output.txt",
	})

	action := FormatToolAction("Write", input)

	if !strings.Contains(action, "output.txt") {
		t.Errorf("Action = %q, should contain filename", action)
	}
}

func TestFormatToolAction_Bash(t *testing.T) {
	input, _ := json.Marshal(map[string]interface{}{
		"command": "go build ./...",
	})

	action := FormatToolAction("Bash", input)

	// Action is formatted as "Running <first word of command>"
	if !strings.Contains(action, "Running") {
		t.Errorf("Action = %q, should contain 'Running'", action)
	}
}

func TestFormatToolAction_Unknown(t *testing.T) {
	input := json.RawMessage(`{}`)

	action := FormatToolAction("UnknownTool", input)

	if action != "UnknownTool" {
		t.Errorf("Action = %q, want 'UnknownTool'", action)
	}
}

func TestToolResult_Fields(t *testing.T) {
	result := ToolResult{
		Content: "test content",
		IsError: true,
	}

	if result.Content != "test content" {
		t.Errorf("Content = %q, want %q", result.Content, "test content")
	}
	if !result.IsError {
		t.Error("IsError should be true")
	}
}
