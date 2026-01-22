// Package protect provides protected area detection including import analysis.
package protect

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ImportPattern defines an import pattern that indicates a protected file.
type ImportPattern struct {
	Language string // "go", "typescript", "python", "rust"
	Pattern  string // regex pattern for import statement
	Reason   string // human-readable reason (e.g., "Cryptography", "Authentication")
}

// SecurityImports defines import patterns that indicate security-sensitive code.
var SecurityImports = []ImportPattern{
	// Go
	{Language: "go", Pattern: `import.*"crypto/`, Reason: "Cryptography"},
	{Language: "go", Pattern: `import.*"golang\.org/x/crypto/`, Reason: "Cryptography"},
	{Language: "go", Pattern: `import.*"github\.com/[^/]+/jwt`, Reason: "JWT authentication"},
	{Language: "go", Pattern: `import.*"golang\.org/x/oauth2`, Reason: "OAuth2 authentication"},
	{Language: "go", Pattern: `import.*"github\.com/[^/]+/bcrypt`, Reason: "Password hashing"},
	{Language: "go", Pattern: `import.*"database/sql`, Reason: "Database access"},

	// TypeScript/JavaScript
	{Language: "typescript", Pattern: `import.*['"]crypto['"]`, Reason: "Cryptography"},
	{Language: "typescript", Pattern: `import.*['"]bcrypt['"]`, Reason: "Password hashing"},
	{Language: "typescript", Pattern: `import.*['"]jsonwebtoken['"]`, Reason: "JWT authentication"},
	{Language: "typescript", Pattern: `import.*['"]passport['"]`, Reason: "Authentication"},
	{Language: "typescript", Pattern: `import.*['"]express-session['"]`, Reason: "Session management"},
	{Language: "typescript", Pattern: `import.*['"]@aws-sdk/client-secrets-manager['"]`, Reason: "Secrets management"},
	{Language: "typescript", Pattern: `from.*['"]crypto['"]`, Reason: "Cryptography"},
	{Language: "typescript", Pattern: `from.*['"]bcrypt['"]`, Reason: "Password hashing"},
	{Language: "typescript", Pattern: `from.*['"]jsonwebtoken['"]`, Reason: "JWT authentication"},
	{Language: "typescript", Pattern: `require\(['"]crypto['"]\)`, Reason: "Cryptography"},
	{Language: "typescript", Pattern: `require\(['"]bcrypt['"]\)`, Reason: "Password hashing"},

	// Python
	{Language: "python", Pattern: `import cryptography`, Reason: "Cryptography"},
	{Language: "python", Pattern: `from cryptography`, Reason: "Cryptography"},
	{Language: "python", Pattern: `import jwt`, Reason: "JWT authentication"},
	{Language: "python", Pattern: `from jwt`, Reason: "JWT authentication"},
	{Language: "python", Pattern: `import secrets`, Reason: "Secrets generation"},
	{Language: "python", Pattern: `import hashlib`, Reason: "Password hashing"},
	{Language: "python", Pattern: `import bcrypt`, Reason: "Password hashing"},
	{Language: "python", Pattern: `from passlib`, Reason: "Password hashing"},
	{Language: "python", Pattern: `import sqlalchemy`, Reason: "Database access"},
	{Language: "python", Pattern: `from django\.contrib\.auth`, Reason: "Authentication"},

	// Rust
	{Language: "rust", Pattern: `use.*ring::`, Reason: "Cryptography"},
	{Language: "rust", Pattern: `use.*crypto::`, Reason: "Cryptography"},
	{Language: "rust", Pattern: `use.*jsonwebtoken`, Reason: "JWT authentication"},
	{Language: "rust", Pattern: `use.*bcrypt`, Reason: "Password hashing"},
	{Language: "rust", Pattern: `use.*argon2`, Reason: "Password hashing"},
	{Language: "rust", Pattern: `use.*diesel`, Reason: "Database access"},
	{Language: "rust", Pattern: `use.*sqlx`, Reason: "Database access"},
}

// ImportDetector scans files for security-related imports.
type ImportDetector struct {
	patterns map[string][]*regexp.Regexp // language -> compiled patterns
}

// NewImportDetector creates a new import detector with compiled patterns.
func NewImportDetector() *ImportDetector {
	id := &ImportDetector{
		patterns: make(map[string][]*regexp.Regexp),
	}

	// Compile patterns for each language
	for _, importPattern := range SecurityImports {
		regex, err := regexp.Compile(importPattern.Pattern)
		if err != nil {
			// Skip invalid patterns
			continue
		}

		id.patterns[importPattern.Language] = append(id.patterns[importPattern.Language], regex)
	}

	return id
}

// HasSecurityImports checks if a file contains security-related imports.
// Returns true if security imports are found, along with the reason.
func (id *ImportDetector) HasSecurityImports(filePath string) (bool, string) {
	// Detect language from file extension
	lang := detectLanguage(filePath)
	if lang == "" {
		return false, ""
	}

	// Get patterns for this language
	patterns, exists := id.patterns[lang]
	if !exists || len(patterns) == 0 {
		return false, ""
	}

	// Read and scan file
	file, err := os.Open(filePath)
	if err != nil {
		return false, ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	maxLinesToScan := 100 // Only scan first 100 lines (imports are usually at top)

	for scanner.Scan() && lineNum < maxLinesToScan {
		lineNum++
		line := scanner.Text()

		// Check each pattern
		for _, pattern := range patterns {
			if pattern.MatchString(line) {
				// Find the corresponding reason
				for _, importPattern := range SecurityImports {
					if importPattern.Language == lang && importPattern.Pattern == pattern.String() {
						return true, importPattern.Reason
					}
				}
				// Fallback if reason not found
				return true, "Security-sensitive import detected"
			}
		}

		// Stop scanning after import section (heuristic)
		if shouldStopScanning(line, lang) {
			break
		}
	}

	return false, ""
}

// detectLanguage determines the programming language from file extension.
func detectLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx", ".js", ".jsx":
		return "typescript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	default:
		return ""
	}
}

// shouldStopScanning determines if we've passed the import section.
// This is a heuristic to avoid scanning entire large files.
func shouldStopScanning(line string, lang string) bool {
	trimmed := strings.TrimSpace(line)

	// Empty lines don't indicate end of imports
	if trimmed == "" {
		return false
	}

	// Comments don't indicate end of imports
	if isComment(trimmed, lang) {
		return false
	}

	switch lang {
	case "go":
		// Go: stop after import block closes
		if trimmed == ")" && !strings.Contains(trimmed, "import") {
			return true
		}
		// Go: stop at first function/type/const/var declaration after imports
		if strings.HasPrefix(trimmed, "func ") ||
			strings.HasPrefix(trimmed, "type ") ||
			strings.HasPrefix(trimmed, "const ") ||
			strings.HasPrefix(trimmed, "var ") {
			return true
		}

	case "typescript":
		// TypeScript: stop at first non-import statement
		if !strings.HasPrefix(trimmed, "import ") &&
			!strings.HasPrefix(trimmed, "from ") &&
			!strings.Contains(trimmed, "require(") &&
			!strings.HasPrefix(trimmed, "export ") {
			// Found a non-import statement
			if strings.Contains(trimmed, "function ") ||
				strings.Contains(trimmed, "class ") ||
				strings.Contains(trimmed, "const ") ||
				strings.Contains(trimmed, "let ") ||
				strings.Contains(trimmed, "var ") {
				return true
			}
		}

	case "python":
		// Python: stop at first non-import statement
		if !strings.HasPrefix(trimmed, "import ") &&
			!strings.HasPrefix(trimmed, "from ") {
			// Found a non-import statement (class, def, etc.)
			if strings.HasPrefix(trimmed, "class ") ||
				strings.HasPrefix(trimmed, "def ") ||
				strings.HasPrefix(trimmed, "@") {
				return true
			}
		}

	case "rust":
		// Rust: stop at first non-use statement
		if !strings.HasPrefix(trimmed, "use ") &&
			!strings.HasPrefix(trimmed, "extern ") {
			// Found a non-import statement
			if strings.HasPrefix(trimmed, "fn ") ||
				strings.HasPrefix(trimmed, "struct ") ||
				strings.HasPrefix(trimmed, "enum ") ||
				strings.HasPrefix(trimmed, "impl ") ||
				strings.HasPrefix(trimmed, "pub fn ") {
				return true
			}
		}
	}

	return false
}

// isComment checks if a line is a comment.
func isComment(line string, lang string) bool {
	switch lang {
	case "go", "typescript", "rust":
		return strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*")
	case "python":
		return strings.HasPrefix(line, "#")
	default:
		return false
	}
}

// GetReason returns a human-readable reason for why a file is protected based on imports.
func GetImportProtectionReason(filePath string) string {
	detector := NewImportDetector()
	hasImports, reason := detector.HasSecurityImports(filePath)
	if hasImports {
		return reason
	}
	return ""
}
