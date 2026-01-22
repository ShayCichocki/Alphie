// Package architect provides architecture document parsing and analysis.
package architect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

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

// ParserCache stores parsed specs by file content hash.
type ParserCache struct {
	mu      sync.RWMutex
	entries map[string]*CachedSpec // key = SHA256 hash
}

// CachedSpec represents a cached parsed specification.
type CachedSpec struct {
	Spec     *ArchSpec
	FilePath string
	FileHash string
	ParsedAt time.Time
}

// Parser extracts structured features from markdown architecture documents using Claude.
type Parser struct {
	// extractionPrompt is the prompt template used to extract features.
	extractionPrompt string
	// cache stores parsed specs by content hash
	cache *ParserCache
	// enableCache controls whether caching is enabled
	enableCache bool
}

// NewParser creates a new Parser with default settings.
func NewParser() *Parser {
	return &Parser{
		extractionPrompt: defaultExtractionPrompt,
		cache: &ParserCache{
			entries: make(map[string]*CachedSpec),
		},
		enableCache: true,
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
- Use EXACTLY the feature IDs and names from the document
- Do NOT infer or generate criteria - only extract explicitly stated criteria
- BE DETERMINISTIC: Always extract the same features in the same order
- Extract ALL features, requirements, and specifications from the document
- Ensure the JSON is valid and complete
- Do not include any text before or after the JSON object

Document to parse:
`

// xmlExtractionPrompt is the prompt used to extract features from XML documents.
const xmlExtractionPrompt = `You are an architecture document parser. Analyze the following XML document and extract all features, requirements, and specifications.

For each feature/requirement you identify, extract:
1. ID: A unique identifier (use existing IDs from XML attributes/tags, or generate ones like F001, F002, etc.)
2. Name: A short descriptive name for the feature
3. Description: The full description of the feature
4. Criteria: What constitutes full implementation (optional)

Parse XML elements, attributes, and nested structures. Common patterns:
- <feature id="F001" name="...">description</feature>
- <requirement>...</requirement>
- <spec>...</spec>
- Or any custom XML schema

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
- Use EXACTLY the feature IDs and names from the XML
- Do NOT infer or generate criteria - only extract explicitly stated criteria
- BE DETERMINISTIC: Always extract the same features in the same order
- Extract ALL features, requirements, and specifications from the XML
- Handle nested elements and attributes appropriately
- Ensure the JSON is valid and complete
- Do not include any text before or after the JSON object

XML document to parse:
`

// Parse extracts features from an architecture document (markdown or XML).
// It reads the file at docPath and uses Claude to extract structured features.
func (p *Parser) Parse(ctx context.Context, docPath string, claude agent.ClaudeRunner) (*ArchSpec, error) {
	// Read the document
	content, err := os.ReadFile(docPath)
	if err != nil {
		return nil, fmt.Errorf("read document: %w", err)
	}

	// Validate content is not empty
	if len(strings.TrimSpace(string(content))) == 0 {
		return nil, fmt.Errorf("document is empty")
	}

	// Compute content hash for caching
	hash := computeSHA256(content)

	// Check cache
	if p.enableCache && p.cache != nil {
		if cached := p.cache.Get(hash); cached != nil {
			return cached.Spec, nil // Cache hit
		}
	}

	// Cache miss - proceed with parsing
	// Select prompt based on file extension
	promptTemplate := p.extractionPrompt
	if strings.HasSuffix(strings.ToLower(docPath), ".xml") {
		promptTemplate = xmlExtractionPrompt
	}

	// Build the prompt
	prompt := promptTemplate + string(content)

	// Start Claude process with temperature=0 for deterministic parsing
	temp := 0.0
	opts := &agent.StartOptions{
		Temperature: &temp,
	}
	if err := claude.StartWithOptions(prompt, "", opts); err != nil {
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

	// Store in cache after successful parse
	if p.enableCache && p.cache != nil {
		p.cache.Set(hash, &CachedSpec{
			Spec:     spec,
			FilePath: docPath,
			FileHash: hash,
			ParsedAt: time.Now(),
		})
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

// computeSHA256 computes the SHA256 hash of the given data.
func computeSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// Get retrieves a cached spec by hash.
func (c *ParserCache) Get(hash string) *CachedSpec {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.entries[hash]
}

// Set stores a spec in the cache by hash.
func (c *ParserCache) Set(hash string, spec *CachedSpec) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[hash] = spec
}
