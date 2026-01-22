// Package structure provides directory structure analysis and guidance.
package structure

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// CacheFileName is the name of the structure cache file
	CacheFileName = ".alphie/structure_cache.json"
	// CacheMaxAge is how long before we re-analyze the structure (24 hours)
	CacheMaxAge = 24 * time.Hour
)

// StructureAnalyzer analyzes repository directory structure and caches the results.
type StructureAnalyzer struct {
	repoPath string
	rules    *StructureRules
}

// NewAnalyzer creates a new StructureAnalyzer for the given repository.
func NewAnalyzer(repoPath string) *StructureAnalyzer {
	return &StructureAnalyzer{
		repoPath: repoPath,
		rules:    nil,
	}
}

// AnalyzeRepository scans the repository and identifies directory structure patterns.
// Results are cached in .alphie/structure_cache.json for performance.
func (a *StructureAnalyzer) AnalyzeRepository() error {
	// Check if we have a valid cache
	if a.loadCache() {
		return nil
	}

	// Perform fresh analysis
	rules, err := a.analyzeDirectoryStructure()
	if err != nil {
		return err
	}

	a.rules = rules

	// Cache the results
	if err := a.saveCache(); err != nil {
		// Log but don't fail - caching is not critical
		// Error will be silently ignored
	}

	return nil
}

// GetRules returns the analyzed structure rules.
// Returns nil if AnalyzeRepository hasn't been called.
func (a *StructureAnalyzer) GetRules() *StructureRules {
	return a.rules
}

// loadCache attempts to load cached structure rules.
// Returns true if cache was loaded successfully and is still valid.
func (a *StructureAnalyzer) loadCache() bool {
	cachePath := filepath.Join(a.repoPath, CacheFileName)

	// Check if cache file exists
	info, err := os.Stat(cachePath)
	if err != nil {
		return false
	}

	// Check if cache is too old
	if time.Since(info.ModTime()) > CacheMaxAge {
		return false
	}

	// Load cache file
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return false
	}

	// Parse cache
	var rules StructureRules
	if err := json.Unmarshal(data, &rules); err != nil {
		return false
	}

	a.rules = &rules
	return true
}

// saveCache saves the structure rules to the cache file.
func (a *StructureAnalyzer) saveCache() error {
	if a.rules == nil {
		return nil
	}

	cachePath := filepath.Join(a.repoPath, CacheFileName)

	// Ensure .alphie directory exists
	alphieDir := filepath.Dir(cachePath)
	if err := os.MkdirAll(alphieDir, 0755); err != nil {
		return err
	}

	// Update timestamp
	a.rules.Timestamp = time.Now().Unix()

	// Marshal to JSON
	data, err := json.MarshalIndent(a.rules, "", "  ")
	if err != nil {
		return err
	}

	// Write to file
	return os.WriteFile(cachePath, data, 0644)
}

// analyzeDirectoryStructure walks the repository and identifies common patterns.
func (a *StructureAnalyzer) analyzeDirectoryStructure() (*StructureRules, error) {
	rules := &StructureRules{
		Rules: []StructureRule{},
	}

	// Track directories and their files
	dirFiles := make(map[string][]string)

	// Walk the repository
	err := filepath.Walk(a.repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}

		// Skip hidden directories and common ignore patterns
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".alphie" {
				return filepath.SkipDir
			}
			return nil
		}

		// Only track code files
		if !isCodeFile(path) {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(a.repoPath, path)
		if err != nil {
			return nil
		}

		// Get directory
		dir := filepath.Dir(relPath)
		if dir == "." {
			dir = ""
		}

		// Add to directory files
		dirFiles[dir] = append(dirFiles[dir], relPath)

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Analyze patterns for directories with multiple files
	for dir, files := range dirFiles {
		if len(files) < 2 {
			continue // Skip directories with only one file
		}

		// Determine the primary file type
		ext := getCommonExtension(files)
		if ext == "" {
			continue
		}

		// Create a rule for this directory
		pattern := filepath.Join(dir, "*"+ext)
		description := describeDirectory(dir)

		// Take up to 3 examples
		examples := files
		if len(examples) > 3 {
			examples = examples[:3]
		}

		rule := StructureRule{
			Pattern:     pattern,
			Description: description,
			Examples:    examples,
			Directory:   dir,
		}

		rules.Rules = append(rules.Rules, rule)
	}

	return rules, nil
}

// isCodeFile checks if a file is a code file based on extension.
func isCodeFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	codeExts := []string{
		".go", ".js", ".ts", ".jsx", ".tsx", ".py", ".rb", ".java",
		".c", ".cpp", ".h", ".hpp", ".rs", ".php", ".swift", ".kt",
	}

	for _, codeExt := range codeExts {
		if ext == codeExt {
			return true
		}
	}
	return false
}

// getCommonExtension returns the most common extension in the file list.
func getCommonExtension(files []string) string {
	extCount := make(map[string]int)

	for _, file := range files {
		ext := filepath.Ext(file)
		extCount[ext]++
	}

	// Find the most common extension
	maxCount := 0
	commonExt := ""
	for ext, count := range extCount {
		if count > maxCount {
			maxCount = count
			commonExt = ext
		}
	}

	return commonExt
}

// describeDirectory generates a human-readable description for a directory.
func describeDirectory(dir string) string {
	if dir == "" {
		return "Root directory files"
	}

	// Extract the last component for description
	parts := strings.Split(dir, string(filepath.Separator))
	lastPart := parts[len(parts)-1]

	// Capitalize and add "files"
	return strings.Title(lastPart) + " files"
}
