// Package orchestrator provides task decomposition and coordination.
package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"
)

// defaultPatterns defines glob patterns for protected areas.
// These match common security-sensitive directory structures.
var defaultPatterns = []string{
	"**/auth/**",
	"**/security/**",
	"**/migrations/**",
	"**/infra/**",
	"**/secrets/**",
	"**/credentials/**",
	"**/certs/**",
	"**/keys/**",
	"**/.ssh/**",
	"**/terraform/**",
	"**/helm/**",
	"**/k8s/**",
	"**/kubernetes/**",
}

// defaultKeywords defines substrings that indicate protected files.
var defaultKeywords = []string{
	"auth",
	"login",
	"password",
	"token",
	"secret",
	"key",
	"migration",
	"credential",
	"cert",
	"private",
	"encrypt",
	"decrypt",
	"oauth",
	"jwt",
	"session",
	"permission",
	"acl",
	"rbac",
}

// defaultFileTypes defines file extensions that are protected.
var defaultFileTypes = []string{
	".sql",
	".tf",
	".pem",
	".key",
	".env",
	".p12",
	".pfx",
	".jks",
	".keystore",
	".crt",
	".cer",
}

// ProtectedAreaDetector checks if file paths are in protected areas.
// Protected areas require additional oversight (e.g., Scout override gates).
type ProtectedAreaDetector struct {
	patterns  []string
	keywords  []string
	fileTypes []string
	mu        sync.RWMutex
}

// alphieConfig represents the .alphie.yaml configuration file structure.
type alphieConfig struct {
	ProtectedAreas struct {
		Patterns  []string `yaml:"patterns"`
		Keywords  []string `yaml:"keywords"`
		FileTypes []string `yaml:"file_types"`
	} `yaml:"protected_areas"`
}

// NewProtectedAreaDetector creates a new detector with default patterns.
func NewProtectedAreaDetector() *ProtectedAreaDetector {
	return &ProtectedAreaDetector{
		patterns:  append([]string{}, defaultPatterns...),
		keywords:  append([]string{}, defaultKeywords...),
		fileTypes: append([]string{}, defaultFileTypes...),
	}
}

// IsProtected checks if a path matches any protected area criteria.
// It returns true if the path:
//  1. Matches any glob pattern
//  2. Contains any keyword
//  3. Has a protected file extension
func (d *ProtectedAreaDetector) IsProtected(path string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Normalize path separators for consistent matching.
	normalizedPath := filepath.ToSlash(path)
	lowerPath := strings.ToLower(normalizedPath)

	// Check glob patterns.
	for _, pattern := range d.patterns {
		if matchGlobPattern(normalizedPath, pattern) {
			return true
		}
	}

	// Check keywords in the path.
	for _, keyword := range d.keywords {
		if strings.Contains(lowerPath, strings.ToLower(keyword)) {
			return true
		}
	}

	// Check file extension.
	ext := strings.ToLower(filepath.Ext(path))
	for _, protectedExt := range d.fileTypes {
		if ext == strings.ToLower(protectedExt) {
			return true
		}
	}

	return false
}

// AddPattern adds a glob pattern to the protected patterns list.
func (d *ProtectedAreaDetector) AddPattern(pattern string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.patterns = append(d.patterns, pattern)
}

// AddKeyword adds a keyword to the protected keywords list.
func (d *ProtectedAreaDetector) AddKeyword(keyword string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.keywords = append(d.keywords, keyword)
}

// AddFileType adds a file extension to the protected file types list.
// The extension should include the leading dot (e.g., ".sql").
func (d *ProtectedAreaDetector) AddFileType(ext string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.fileTypes = append(d.fileTypes, ext)
}

// LoadConfig loads protected area configuration from an .alphie.yaml file.
// This appends to existing patterns rather than replacing them.
func (d *ProtectedAreaDetector) LoadConfig(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var config alphieConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Append configuration values to existing lists.
	d.patterns = append(d.patterns, config.ProtectedAreas.Patterns...)
	d.keywords = append(d.keywords, config.ProtectedAreas.Keywords...)
	d.fileTypes = append(d.fileTypes, config.ProtectedAreas.FileTypes...)

	return nil
}

// matchGlobPattern matches a path against a glob pattern with ** support.
// It handles ** (any depth), * (single segment), and literal matches.
func matchGlobPattern(path, pattern string) bool {
	// Split both path and pattern into segments.
	pathParts := strings.Split(path, "/")
	patternParts := strings.Split(pattern, "/")

	return matchParts(pathParts, patternParts)
}

// matchParts recursively matches path segments against pattern segments.
func matchParts(path, pattern []string) bool {
	// Base cases.
	if len(pattern) == 0 {
		return len(path) == 0
	}

	p := pattern[0]
	rest := pattern[1:]

	switch p {
	case "**":
		// ** matches zero or more path segments.
		// If this is the last pattern segment, it matches everything.
		if len(rest) == 0 {
			return true
		}
		// Try matching ** against 0 to len(path) segments.
		for i := 0; i <= len(path); i++ {
			if matchParts(path[i:], rest) {
				return true
			}
		}
		return false

	default:
		// Need at least one path segment to match.
		if len(path) == 0 {
			return false
		}
		// Match current segment.
		if !matchSegment(path[0], p) {
			return false
		}
		// Recurse for remaining segments.
		return matchParts(path[1:], rest)
	}
}

// matchSegment matches a single path segment against a pattern segment.
// It supports * as a wildcard within a single segment.
func matchSegment(segment, pattern string) bool {
	// Simple case: literal match or single *.
	if pattern == "*" {
		return true
	}
	if pattern == segment {
		return true
	}

	// Handle patterns with * wildcards (e.g., "auth*", "*_secret").
	if strings.Contains(pattern, "*") {
		return matchWildcard(segment, pattern)
	}

	return false
}

// matchWildcard matches a segment against a pattern containing * wildcards.
func matchWildcard(s, pattern string) bool {
	// Split pattern by * to get literal parts.
	parts := strings.Split(pattern, "*")

	// Track position in the segment.
	pos := 0

	for i, part := range parts {
		if part == "" {
			continue
		}

		// First part must match at the beginning.
		if i == 0 {
			if !strings.HasPrefix(s, part) {
				return false
			}
			pos = len(part)
			continue
		}

		// Last part must match at the end.
		if i == len(parts)-1 && !strings.HasSuffix(pattern, "*") {
			if !strings.HasSuffix(s, part) {
				return false
			}
			continue
		}

		// Middle parts must exist somewhere after current position.
		idx := strings.Index(s[pos:], part)
		if idx == -1 {
			return false
		}
		pos += idx + len(part)
	}

	return true
}
