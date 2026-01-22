// Package architect provides architecture document parsing and analysis.
package architect

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ShayCichocki/alphie/internal/agent"
)

// Note: Feature and ArchSpec types are defined in auditor.go

// Section represents a logical section of the architecture document.
type Section struct {
	// Title is the section heading.
	Title string `json:"title"`
	// Content is the raw text content of the section.
	Content string `json:"content"`
	// Level indicates the heading level (1 for H1, 2 for H2, etc.).
	Level int `json:"level"`
}

// Parser extracts structured features from markdown architecture documents using Claude.
type Parser struct {
	// extractionPrompt is the prompt template used to extract features.
	extractionPrompt string
}

// NewParser creates a new Parser with default settings.
func NewParser() *Parser {
	return &Parser{
		extractionPrompt: defaultExtractionPrompt,
	}
}

// defaultExtractionPrompt is the prompt used to extract features from markdown documents.
const defaultExtractionPrompt = `You are an architecture document parser. Analyze the following markdown document and extract all features, requirements, and specifications.

For each feature/requirement you identify, extract:
1. ID: A unique identifier (use existing IDs from the doc, or generate ones like F001, F002, etc.)
2. Name: A short descriptive name for the feature
3. Description: The full description of the feature
4. Criteria: What constitutes full implementation (optional)

Respond with a JSON object in this exact format:
{
  "name": "Specification Name",
  "features": [
    {
      "id": "F001",
      "name": "Feature Name",
      "description": "Full description",
      "criteria": "What defines complete implementation"
    }
  ]
}

IMPORTANT:
- Extract ALL features, requirements, and specifications from the document
- If no explicit criteria exist, infer reasonable ones from the description
- Ensure the JSON is valid and complete
- Do not include any text before or after the JSON object

Document to parse:
`

// Parse extracts features from a markdown architecture document.
// It reads the file at docPath and uses Claude to extract structured features.
func (p *Parser) Parse(ctx context.Context, docPath string, claude agent.ClaudeRunner) (*ArchSpec, error) {
	// Read the markdown document
	content, err := os.ReadFile(docPath)
	if err != nil {
		return nil, fmt.Errorf("read document: %w", err)
	}

	// Validate content is not empty
	if len(strings.TrimSpace(string(content))) == 0 {
		return nil, fmt.Errorf("document is empty")
	}

	// Build the prompt
	prompt := p.extractionPrompt + string(content)

	// Start Claude process
	if err := claude.Start(prompt, ""); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}

	// Collect the response
	var responseBuilder strings.Builder
	for event := range claude.Output() {
		switch event.Type {
		case agent.StreamEventResult:
			responseBuilder.WriteString(event.Message)
		case agent.StreamEventAssistant:
			responseBuilder.WriteString(event.Message)
		case agent.StreamEventError:
			return nil, fmt.Errorf("claude error: %s", event.Error)
		}
	}

	// Wait for process to complete
	if err := claude.Wait(); err != nil {
		return nil, fmt.Errorf("claude process failed: %w", err)
	}

	// Parse the JSON response
	response := responseBuilder.String()
	spec, err := parseResponse(response)
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return spec, nil
}

// parseResponse extracts the ArchSpec from Claude's response.
// It handles cases where the JSON might be wrapped in markdown code blocks.
func parseResponse(response string) (*ArchSpec, error) {
	response = strings.TrimSpace(response)

	// Handle markdown code blocks
	if strings.HasPrefix(response, "```json") {
		response = strings.TrimPrefix(response, "```json")
		if idx := strings.LastIndex(response, "```"); idx != -1 {
			response = response[:idx]
		}
		response = strings.TrimSpace(response)
	} else if strings.HasPrefix(response, "```") {
		response = strings.TrimPrefix(response, "```")
		if idx := strings.LastIndex(response, "```"); idx != -1 {
			response = response[:idx]
		}
		response = strings.TrimSpace(response)
	}

	// Try to find JSON object in the response
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start == -1 || end == -1 || end < start {
		return nil, fmt.Errorf("no valid JSON object found in response")
	}
	response = response[start : end+1]

	var spec ArchSpec
	if err := json.Unmarshal([]byte(response), &spec); err != nil {
		return nil, fmt.Errorf("unmarshal JSON: %w", err)
	}

	// Validate the parsed spec
	if err := validateSpec(&spec); err != nil {
		return nil, fmt.Errorf("validate spec: %w", err)
	}

	return &spec, nil
}

// validateSpec performs basic validation on the parsed ArchSpec.
func validateSpec(spec *ArchSpec) error {
	if spec == nil {
		return fmt.Errorf("spec is nil")
	}

	// Ensure features have required fields
	for i, f := range spec.Features {
		if f.ID == "" {
			return fmt.Errorf("feature at index %d has empty ID", i)
		}
		if f.Name == "" {
			return fmt.Errorf("feature %q has empty name", f.ID)
		}
	}

	return nil
}

// ParseArchDoc is a convenience function that creates a Parser and parses a document.
// This matches the interface specified in the task description.
func ParseArchDoc(ctx context.Context, docPath string, claude agent.ClaudeRunner) (*ArchSpec, error) {
	parser := NewParser()
	return parser.Parse(ctx, docPath, claude)
}
