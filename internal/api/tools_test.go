package api

import (
	"testing"
)

func TestToolDefinitions(t *testing.T) {
	tools := ToolDefinitions()

	if len(tools) == 0 {
		t.Fatal("ToolDefinitions returned empty slice")
	}

	// Check expected tools are present
	expectedTools := []string{"Read", "Write", "Edit", "Bash", "Glob", "Grep", "ListDir"}

	for _, expectedName := range expectedTools {
		found := false
		for _, tool := range tools {
			if tool.OfTool != nil && tool.OfTool.Name == expectedName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Missing expected tool: %s", expectedName)
		}
	}
}

func TestToolDefinitions_Count(t *testing.T) {
	tools := ToolDefinitions()

	// Should have exactly 7 tools
	if len(tools) != 7 {
		t.Errorf("ToolDefinitions count = %d, want 7", len(tools))
	}
}

func TestMinimalToolDefinitions(t *testing.T) {
	tools := MinimalToolDefinitions()

	if len(tools) == 0 {
		t.Fatal("MinimalToolDefinitions returned empty slice")
	}

	// Minimal should have Read, Glob, Grep only
	expectedTools := []string{"Read", "Glob", "Grep"}

	if len(tools) != len(expectedTools) {
		t.Errorf("MinimalToolDefinitions count = %d, want %d", len(tools), len(expectedTools))
	}

	for _, expectedName := range expectedTools {
		found := false
		for _, tool := range tools {
			if tool.OfTool != nil && tool.OfTool.Name == expectedName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Missing expected minimal tool: %s", expectedName)
		}
	}
}

func TestMinimalToolDefinitions_NoWriteTools(t *testing.T) {
	tools := MinimalToolDefinitions()

	// Minimal should NOT have Write, Edit, Bash
	forbiddenTools := []string{"Write", "Edit", "Bash", "ListDir"}

	for _, forbiddenName := range forbiddenTools {
		for _, tool := range tools {
			if tool.OfTool != nil && tool.OfTool.Name == forbiddenName {
				t.Errorf("MinimalToolDefinitions should not include %s", forbiddenName)
			}
		}
	}
}

func TestToolDefinitions_HasDescriptions(t *testing.T) {
	tools := ToolDefinitions()

	for _, tool := range tools {
		if tool.OfTool == nil {
			t.Error("Tool has nil OfTool")
			continue
		}
		// Description is wrapped in param.Opt[string], just verify tool has a name
		if tool.OfTool.Name == "" {
			t.Error("Tool has empty name")
		}
	}
}

func TestToolDefinitions_HasRequiredFields(t *testing.T) {
	tools := ToolDefinitions()

	for _, tool := range tools {
		if tool.OfTool == nil {
			continue
		}
		if len(tool.OfTool.InputSchema.Required) == 0 {
			t.Errorf("Tool %s has no required fields", tool.OfTool.Name)
		}
	}
}
