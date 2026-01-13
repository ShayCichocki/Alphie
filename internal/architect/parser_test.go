package architect

import (
	"testing"
)

func TestParseResponse_ValidJSON(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantErr  bool
		features int
	}{
		{
			name: "simple valid JSON",
			response: `{
				"name": "Test Spec",
				"features": [{"id": "F001", "name": "Test Feature", "description": "A test", "criteria": "Works"}]
			}`,
			wantErr:  false,
			features: 1,
		},
		{
			name: "JSON in markdown code block",
			response: "```json\n" + `{
				"name": "Test",
				"features": [{"id": "F001", "name": "Feature", "description": "Desc", "criteria": ""}]
			}` + "\n```",
			wantErr:  false,
			features: 1,
		},
		{
			name: "JSON with surrounding text",
			response: `Here is the parsed result:
			{"name": "Test", "features": [{"id": "F001", "name": "Feature", "description": "Desc", "criteria": ""}]}
			Hope this helps!`,
			wantErr:  false,
			features: 1,
		},
		{
			name: "multiple features",
			response: `{
				"name": "Multi Feature Spec",
				"features": [
					{"id": "F001", "name": "First", "description": "First feature", "criteria": "A, B"},
					{"id": "F002", "name": "Second", "description": "Second feature", "criteria": "C"}
				]
			}`,
			wantErr:  false,
			features: 2,
		},
		{
			name:     "no JSON in response",
			response: "I couldn't parse the document",
			wantErr:  true,
		},
		{
			name:     "invalid JSON",
			response: `{"features": [{"id": "F001" "name": "Missing comma"}]}`,
			wantErr:  true,
		},
		{
			name:     "empty ID",
			response: `{"name": "Test", "features": [{"id": "", "name": "No ID", "description": "Desc", "criteria": ""}]}`,
			wantErr:  true,
		},
		{
			name:     "empty name",
			response: `{"name": "Test", "features": [{"id": "F001", "name": "", "description": "Desc", "criteria": ""}]}`,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := parseResponse(tt.response)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if len(spec.Features) != tt.features {
				t.Errorf("parseResponse() got %d features, want %d", len(spec.Features), tt.features)
			}
		})
	}
}

func TestValidateSpec(t *testing.T) {
	tests := []struct {
		name    string
		spec    *ArchSpec
		wantErr bool
	}{
		{
			name:    "nil spec",
			spec:    nil,
			wantErr: true,
		},
		{
			name: "valid spec",
			spec: &ArchSpec{
				Name:     "Test Spec",
				Features: []Feature{{ID: "F001", Name: "Test", Description: "Desc"}},
			},
			wantErr: false,
		},
		{
			name: "empty feature ID",
			spec: &ArchSpec{
				Name:     "Test",
				Features: []Feature{{ID: "", Name: "Test", Description: "Desc"}},
			},
			wantErr: true,
		},
		{
			name: "empty feature name",
			spec: &ArchSpec{
				Name:     "Test",
				Features: []Feature{{ID: "F001", Name: "", Description: "Desc"}},
			},
			wantErr: true,
		},
		{
			name: "empty features is valid",
			spec: &ArchSpec{
				Name:     "Empty Spec",
				Features: []Feature{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSpec(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSpec() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFeatureStruct(t *testing.T) {
	// Test that Feature struct has all required fields
	f := Feature{
		ID:          "F001",
		Name:        "Test Feature",
		Description: "A test feature for validation",
		Criteria:    "Must work correctly",
	}

	if f.ID != "F001" {
		t.Error("Feature ID not set correctly")
	}
	if f.Name != "Test Feature" {
		t.Error("Feature Name not set correctly")
	}
	if f.Description != "A test feature for validation" {
		t.Error("Feature Description not set correctly")
	}
	if f.Criteria != "Must work correctly" {
		t.Error("Feature Criteria not set correctly")
	}
}

func TestSectionStruct(t *testing.T) {
	s := Section{
		Title:   "Overview",
		Content: "This is the overview section",
		Level:   1,
	}

	if s.Title != "Overview" {
		t.Error("Section Title not set correctly")
	}
	if s.Content != "This is the overview section" {
		t.Error("Section Content not set correctly")
	}
	if s.Level != 1 {
		t.Error("Section Level not set correctly")
	}
}

func TestArchSpecStruct(t *testing.T) {
	spec := ArchSpec{
		Name: "Test Specification",
		Features: []Feature{
			{ID: "F001", Name: "Feature 1", Description: "Desc 1"},
			{ID: "F002", Name: "Feature 2", Description: "Desc 2"},
		},
	}

	if spec.Name != "Test Specification" {
		t.Errorf("Expected name 'Test Specification', got '%s'", spec.Name)
	}
	if len(spec.Features) != 2 {
		t.Errorf("Expected 2 features, got %d", len(spec.Features))
	}
}

func TestNewParser(t *testing.T) {
	parser := NewParser()
	if parser == nil {
		t.Fatal("NewParser returned nil")
	}
	if parser.extractionPrompt == "" {
		t.Error("Parser extractionPrompt is empty")
	}
}

func TestParseResponse_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantErr  bool
	}{
		{
			name:     "empty string",
			response: "",
			wantErr:  true,
		},
		{
			name:     "only whitespace",
			response: "   \n\t   ",
			wantErr:  true,
		},
		{
			name:     "only curly braces",
			response: "{}",
			wantErr:  false, // Empty spec is valid
		},
		{
			name: "nested code blocks",
			response: "```\n```json\n" + `{"name": "Test", "features": []}` + "\n```\n```",
			wantErr:  false,
		},
		{
			name: "features with empty criteria",
			response: `{
				"name": "Test",
				"features": [{
					"id": "F001",
					"name": "Feature",
					"description": "Desc",
					"criteria": ""
				}]
			}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseResponse(tt.response)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
