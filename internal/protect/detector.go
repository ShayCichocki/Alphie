package protect

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"
)

// Detector checks if file paths are in protected areas.
// Protected areas require additional oversight (e.g., Scout override gates).
// Uses 4 detection strategies:
// 1. Glob patterns (e.g., **/auth/**)
// 2. Keywords in path (e.g., "auth", "secret")
// 3. File types (e.g., .sql, .pem)
// 4. Security-related imports (e.g., crypto, bcrypt)
type Detector struct {
	patterns       []string
	keywords       []string
	fileTypes      []string
	importDetector *ImportDetector
	mu             sync.RWMutex
}

// alphieConfig represents the .alphie.yaml configuration file structure.
type alphieConfig struct {
	ProtectedAreas struct {
		Patterns  []string `yaml:"patterns"`
		Keywords  []string `yaml:"keywords"`
		FileTypes []string `yaml:"file_types"`
	} `yaml:"protected_areas"`
}

// New creates a new detector with default patterns.
func New() *Detector {
	return &Detector{
		patterns:       append([]string{}, DefaultPatterns...),
		keywords:       append([]string{}, DefaultKeywords...),
		fileTypes:      append([]string{}, DefaultFileTypes...),
		importDetector: NewImportDetector(),
	}
}

// IsProtected checks if a path matches any protected area criteria.
// Uses all 4 detection strategies: patterns, keywords, file types, and imports.
func (d *Detector) IsProtected(path string) bool {
	protected, _ := d.IsProtectedWithReason(path)
	return protected
}

// IsProtectedWithReason checks if a path is protected and returns the reason.
// This is useful for providing detailed feedback to users.
func (d *Detector) IsProtectedWithReason(path string) (bool, string) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	normalizedPath := filepath.ToSlash(path)
	lowerPath := strings.ToLower(normalizedPath)

	// Strategy 1: Glob patterns
	for _, pattern := range d.patterns {
		if matchGlobPattern(normalizedPath, pattern) {
			return true, "Path matches protected pattern: " + pattern
		}
	}

	// Strategy 2: Keywords in path
	for _, keyword := range d.keywords {
		if strings.Contains(lowerPath, strings.ToLower(keyword)) {
			return true, "Path contains protected keyword: " + keyword
		}
	}

	// Strategy 3: File types
	ext := strings.ToLower(filepath.Ext(path))
	for _, protectedExt := range d.fileTypes {
		if ext == strings.ToLower(protectedExt) {
			return true, "File type is protected: " + protectedExt
		}
	}

	// Strategy 4: Security imports (only check actual files, not directories)
	if ext != "" && d.importDetector != nil {
		hasImports, reason := d.importDetector.HasSecurityImports(path)
		if hasImports {
			return true, "Security-sensitive import: " + reason
		}
	}

	return false, ""
}

// AddPattern adds a glob pattern to the protected patterns list.
func (d *Detector) AddPattern(pattern string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.patterns = append(d.patterns, pattern)
}

// AddKeyword adds a keyword to the protected keywords list.
func (d *Detector) AddKeyword(keyword string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.keywords = append(d.keywords, keyword)
}

// AddFileType adds a file extension to the protected file types list.
func (d *Detector) AddFileType(ext string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.fileTypes = append(d.fileTypes, ext)
}

// LoadConfig loads protected area configuration from an .alphie.yaml file.
func (d *Detector) LoadConfig(configPath string) error {
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

	d.patterns = append(d.patterns, config.ProtectedAreas.Patterns...)
	d.keywords = append(d.keywords, config.ProtectedAreas.Keywords...)
	d.fileTypes = append(d.fileTypes, config.ProtectedAreas.FileTypes...)

	return nil
}
