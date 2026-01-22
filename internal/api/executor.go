package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ToolExecutor executes tool calls from the Claude API.
type ToolExecutor struct {
	workDir string
}

// NewToolExecutor creates a new tool executor for the given working directory.
func NewToolExecutor(workDir string) *ToolExecutor {
	return &ToolExecutor{workDir: workDir}
}

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	Content string
	IsError bool
}

// Execute runs a tool by name with the given JSON input.
func (e *ToolExecutor) Execute(ctx context.Context, name string, input json.RawMessage) ToolResult {
	switch name {
	case "Read":
		return e.execRead(input)
	case "Write":
		return e.execWrite(input)
	case "Edit":
		return e.execEdit(input)
	case "Bash":
		return e.execBash(ctx, input)
	case "Glob":
		return e.execGlob(input)
	case "Grep":
		return e.execGrep(ctx, input)
	case "ListDir":
		return e.execListDir(input)
	default:
		return ToolResult{Content: fmt.Sprintf("Unknown tool: %s", name), IsError: true}
	}
}

func (e *ToolExecutor) execRead(input json.RawMessage) ToolResult {
	var params struct {
		FilePath string `json:"file_path"`
		Offset   int    `json:"offset"`
		Limit    int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Content: fmt.Sprintf("Invalid parameters: %v", err), IsError: true}
	}

	path := e.resolvePath(params.FilePath)
	content, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("Failed to read file: %v", err), IsError: true}
	}

	lines := strings.Split(string(content), "\n")

	// Handle offset and limit
	start := 0
	if params.Offset > 0 {
		start = params.Offset - 1 // Convert to 0-indexed
		if start >= len(lines) {
			return ToolResult{Content: "Offset beyond end of file", IsError: true}
		}
	}

	end := len(lines)
	if params.Limit > 0 {
		end = min(start+params.Limit, len(lines))
	}

	// Format with line numbers (cat -n style)
	var result strings.Builder
	for i := start; i < end; i++ {
		fmt.Fprintf(&result, "%6d\t%s\n", i+1, lines[i])
	}

	return ToolResult{Content: result.String()}
}

func (e *ToolExecutor) execWrite(input json.RawMessage) ToolResult {
	var params struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Content: fmt.Sprintf("Invalid parameters: %v", err), IsError: true}
	}

	path := e.resolvePath(params.FilePath)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return ToolResult{Content: fmt.Sprintf("Failed to create directory: %v", err), IsError: true}
	}

	if err := os.WriteFile(path, []byte(params.Content), 0644); err != nil {
		return ToolResult{Content: fmt.Sprintf("Failed to write file: %v", err), IsError: true}
	}

	return ToolResult{Content: fmt.Sprintf("Successfully wrote %d bytes to %s", len(params.Content), params.FilePath)}
}

func (e *ToolExecutor) execEdit(input json.RawMessage) ToolResult {
	var params struct {
		FilePath   string `json:"file_path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Content: fmt.Sprintf("Invalid parameters: %v", err), IsError: true}
	}

	path := e.resolvePath(params.FilePath)
	content, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("Failed to read file: %v", err), IsError: true}
	}

	contentStr := string(content)

	// Check for the old string
	count := strings.Count(contentStr, params.OldString)
	if count == 0 {
		return ToolResult{Content: "old_string not found in file", IsError: true}
	}

	// If not replace_all, ensure uniqueness
	if !params.ReplaceAll && count > 1 {
		return ToolResult{
			Content: fmt.Sprintf("old_string found %d times; must be unique or use replace_all=true", count),
			IsError: true,
		}
	}

	// Perform replacement
	var newContent string
	if params.ReplaceAll {
		newContent = strings.ReplaceAll(contentStr, params.OldString, params.NewString)
	} else {
		newContent = strings.Replace(contentStr, params.OldString, params.NewString, 1)
	}

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return ToolResult{Content: fmt.Sprintf("Failed to write file: %v", err), IsError: true}
	}

	if params.ReplaceAll {
		return ToolResult{Content: fmt.Sprintf("Replaced %d occurrences", count)}
	}
	return ToolResult{Content: "Edit successful"}
}

func (e *ToolExecutor) execBash(ctx context.Context, input json.RawMessage) ToolResult {
	var params struct {
		Command     string `json:"command"`
		Timeout     int    `json:"timeout"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Content: fmt.Sprintf("Invalid parameters: %v", err), IsError: true}
	}

	// Default timeout of 2 minutes
	timeout := 120 * time.Second
	if params.Timeout > 0 {
		timeout = time.Duration(params.Timeout) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", params.Command)
	cmd.Dir = e.workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return ToolResult{
				Content: fmt.Sprintf("Command timed out after %v:\n%s", timeout, string(output)),
				IsError: true,
			}
		}
		return ToolResult{
			Content: fmt.Sprintf("%s\nError: %v", string(output), err),
			IsError: true,
		}
	}

	// Truncate very long output
	result := string(output)
	if len(result) > 30000 {
		result = result[:30000] + "\n... (output truncated)"
	}

	return ToolResult{Content: result}
}

func (e *ToolExecutor) execGlob(input json.RawMessage) ToolResult {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Content: fmt.Sprintf("Invalid parameters: %v", err), IsError: true}
	}

	searchPath := e.workDir
	if params.Path != "" {
		searchPath = e.resolvePath(params.Path)
	}

	// Use find for recursive glob since filepath.Glob doesn't support **
	var matches []string
	err := filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if d.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}

		// Simple pattern matching
		matched, _ := filepath.Match(filepath.Base(params.Pattern), d.Name())
		if matched {
			relPath, _ := filepath.Rel(searchPath, path)
			matches = append(matches, relPath)
		}
		return nil
	})

	if err != nil {
		return ToolResult{Content: fmt.Sprintf("Glob error: %v", err), IsError: true}
	}

	if len(matches) == 0 {
		return ToolResult{Content: "No files matched the pattern"}
	}

	return ToolResult{Content: strings.Join(matches, "\n")}
}

func (e *ToolExecutor) execGrep(ctx context.Context, input json.RawMessage) ToolResult {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Glob    string `json:"glob"`
		Context int    `json:"context"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Content: fmt.Sprintf("Invalid parameters: %v", err), IsError: true}
	}

	// Build ripgrep command
	args := []string{"--color=never", "-n"} // No color, show line numbers

	if params.Context > 0 {
		args = append(args, "-C", fmt.Sprintf("%d", params.Context))
	}

	if params.Glob != "" {
		args = append(args, "--glob", params.Glob)
	}

	args = append(args, params.Pattern)

	searchPath := e.workDir
	if params.Path != "" {
		searchPath = e.resolvePath(params.Path)
	}
	args = append(args, searchPath)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "rg", args...)
	output, _ := cmd.CombinedOutput() // rg returns non-zero on no match

	result := string(output)
	if len(result) == 0 {
		return ToolResult{Content: "No matches found"}
	}

	// Truncate very long output
	if len(result) > 30000 {
		result = result[:30000] + "\n... (output truncated)"
	}

	return ToolResult{Content: result}
}

func (e *ToolExecutor) execListDir(input json.RawMessage) ToolResult {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Content: fmt.Sprintf("Invalid parameters: %v", err), IsError: true}
	}

	path := e.resolvePath(params.Path)
	entries, err := os.ReadDir(path)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("Failed to read directory: %v", err), IsError: true}
	}

	var result strings.Builder
	for _, entry := range entries {
		info, _ := entry.Info()
		if info != nil {
			if entry.IsDir() {
				fmt.Fprintf(&result, "d %s/\n", entry.Name())
			} else {
				fmt.Fprintf(&result, "- %s (%d bytes)\n", entry.Name(), info.Size())
			}
		} else {
			fmt.Fprintf(&result, "? %s\n", entry.Name())
		}
	}

	return ToolResult{Content: result.String()}
}

func (e *ToolExecutor) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(e.workDir, path)
}

// FormatToolAction returns a human-readable description of a tool call.
func FormatToolAction(name string, input json.RawMessage) string {
	switch name {
	case "Read":
		var p struct {
			FilePath string `json:"file_path"`
		}
		json.Unmarshal(input, &p)
		return "Reading " + filepath.Base(p.FilePath)
	case "Write":
		var p struct {
			FilePath string `json:"file_path"`
		}
		json.Unmarshal(input, &p)
		return "Writing " + filepath.Base(p.FilePath)
	case "Edit":
		var p struct {
			FilePath string `json:"file_path"`
		}
		json.Unmarshal(input, &p)
		return "Editing " + filepath.Base(p.FilePath)
	case "Bash":
		var p struct {
			Command     string `json:"command"`
			Description string `json:"description"`
		}
		json.Unmarshal(input, &p)
		if p.Description != "" {
			return p.Description
		}
		cmd := strings.Split(p.Command, " ")[0]
		if len(cmd) > 20 {
			cmd = cmd[:17] + "..."
		}
		return "Running " + cmd
	case "Glob":
		var p struct {
			Pattern string `json:"pattern"`
		}
		json.Unmarshal(input, &p)
		return "Searching " + p.Pattern
	case "Grep":
		var p struct {
			Pattern string `json:"pattern"`
		}
		json.Unmarshal(input, &p)
		pat := p.Pattern
		if len(pat) > 15 {
			pat = pat[:12] + "..."
		}
		return "Grep " + pat
	case "ListDir":
		return "Listing directory"
	default:
		return name
	}
}
