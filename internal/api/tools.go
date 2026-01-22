package api

import (
	"github.com/anthropics/anthropic-sdk-go"
)

// ToolDefinitions returns the tool schemas for Claude API calls.
// These mirror the tools available in Claude Code CLI.
func ToolDefinitions() []anthropic.ToolUnionParam {
	return []anthropic.ToolUnionParam{
		{
			OfTool: &anthropic.ToolParam{
				Name:        "Read",
				Description: anthropic.String("Read a file from the filesystem. Returns file contents with line numbers."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"file_path": map[string]interface{}{
							"type":        "string",
							"description": "Absolute path to the file to read",
						},
						"offset": map[string]interface{}{
							"type":        "integer",
							"description": "Line number to start reading from (1-indexed, optional)",
						},
						"limit": map[string]interface{}{
							"type":        "integer",
							"description": "Maximum number of lines to read (optional)",
						},
					},
					Required: []string{"file_path"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "Write",
				Description: anthropic.String("Write content to a file. Creates parent directories if needed."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"file_path": map[string]interface{}{
							"type":        "string",
							"description": "Absolute path to the file to write",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "Content to write to the file",
						},
					},
					Required: []string{"file_path", "content"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "Edit",
				Description: anthropic.String("Edit a file by replacing text. The old_string must be unique unless replace_all is true."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"file_path": map[string]interface{}{
							"type":        "string",
							"description": "Absolute path to the file to edit",
						},
						"old_string": map[string]interface{}{
							"type":        "string",
							"description": "The exact text to find and replace",
						},
						"new_string": map[string]interface{}{
							"type":        "string",
							"description": "The text to replace it with",
						},
						"replace_all": map[string]interface{}{
							"type":        "boolean",
							"description": "If true, replace all occurrences (default: false)",
						},
					},
					Required: []string{"file_path", "old_string", "new_string"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "Bash",
				Description: anthropic.String("Execute a bash command and return the output."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"command": map[string]interface{}{
							"type":        "string",
							"description": "The bash command to execute",
						},
						"timeout": map[string]interface{}{
							"type":        "integer",
							"description": "Timeout in milliseconds (optional, default 120000)",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "Description of what this command does",
						},
					},
					Required: []string{"command"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "Glob",
				Description: anthropic.String("Find files matching a glob pattern."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"pattern": map[string]interface{}{
							"type":        "string",
							"description": "Glob pattern to match (e.g., '**/*.go', 'src/**/*.ts')",
						},
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Directory to search in (optional, defaults to working directory)",
						},
					},
					Required: []string{"pattern"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "Grep",
				Description: anthropic.String("Search file contents using regex patterns. Uses ripgrep for performance."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"pattern": map[string]interface{}{
							"type":        "string",
							"description": "Regex pattern to search for",
						},
						"path": map[string]interface{}{
							"type":        "string",
							"description": "File or directory to search in (optional)",
						},
						"glob": map[string]interface{}{
							"type":        "string",
							"description": "Glob pattern to filter files (e.g., '*.go')",
						},
						"context": map[string]interface{}{
							"type":        "integer",
							"description": "Number of context lines to show around matches",
						},
					},
					Required: []string{"pattern"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "ListDir",
				Description: anthropic.String("List contents of a directory."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Directory path to list",
						},
					},
					Required: []string{"path"},
				},
			},
		},
	}
}

// MinimalToolDefinitions returns a reduced set of tools for simple tasks
// like decomposition or review that don't need full file manipulation.
func MinimalToolDefinitions() []anthropic.ToolUnionParam {
	return []anthropic.ToolUnionParam{
		{
			OfTool: &anthropic.ToolParam{
				Name:        "Read",
				Description: anthropic.String("Read a file from the filesystem."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"file_path": map[string]interface{}{
							"type":        "string",
							"description": "Absolute path to the file to read",
						},
					},
					Required: []string{"file_path"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "Glob",
				Description: anthropic.String("Find files matching a glob pattern."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"pattern": map[string]interface{}{
							"type":        "string",
							"description": "Glob pattern to match",
						},
					},
					Required: []string{"pattern"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "Grep",
				Description: anthropic.String("Search file contents using regex."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"pattern": map[string]interface{}{
							"type":        "string",
							"description": "Regex pattern to search for",
						},
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Path to search in",
						},
					},
					Required: []string{"pattern"},
				},
			},
		},
	}
}
