package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProtectedAreaDetector_PatternMatching(t *testing.T) {
	detector := NewProtectedAreaDetector()

	tests := []struct {
		name     string
		path     string
		expected bool
		reason   string
	}{
		// auth directory patterns
		{
			name:     "auth directory - deep path",
			path:     "internal/auth/login.go",
			expected: true,
			reason:   "files in auth/ should be protected",
		},
		{
			name:     "auth directory - root",
			path:     "auth/handler.go",
			expected: true,
			reason:   "root auth/ should be protected",
		},
		// migrations patterns
		{
			name:     "migrations directory",
			path:     "db/migrations/001_create_users.sql",
			expected: true,
			reason:   "migrations/ should be protected",
		},
		{
			name:     "migrations nested",
			path:     "internal/database/migrations/schema.go",
			expected: true,
			reason:   "nested migrations/ should be protected",
		},
		// security patterns
		{
			name:     "security directory",
			path:     "pkg/security/crypto.go",
			expected: true,
			reason:   "security/ should be protected",
		},
		// infrastructure patterns
		{
			name:     "terraform files",
			path:     "infra/terraform/main.tf",
			expected: true,
			reason:   "terraform/ should be protected",
		},
		{
			name:     "kubernetes directory",
			path:     "k8s/deployment.yaml",
			expected: true,
			reason:   "k8s/ should be protected",
		},
		{
			name:     "helm directory",
			path:     "deploy/helm/values.yaml",
			expected: true,
			reason:   "helm/ should be protected",
		},
		// Non-protected paths
		{
			name:     "regular source file",
			path:     "internal/handler/api.go",
			expected: false,
			reason:   "regular files should not be protected",
		},
		{
			name:     "test file",
			path:     "internal/handler/api_test.go",
			expected: false,
			reason:   "test files should not be protected by default patterns",
		},
		{
			name:     "documentation",
			path:     "docs/README.md",
			expected: false,
			reason:   "docs should not be protected",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := detector.IsProtected(tc.path)
			if result != tc.expected {
				t.Errorf("%s: IsProtected(%q) = %v, expected %v", tc.reason, tc.path, result, tc.expected)
			}
		})
	}
}

func TestProtectedAreaDetector_KeywordDetection(t *testing.T) {
	detector := NewProtectedAreaDetector()

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		// Keyword matches
		{
			name:     "auth keyword in filename",
			path:     "internal/user_auth.go",
			expected: true,
		},
		{
			name:     "login keyword",
			path:     "handlers/login_handler.go",
			expected: true,
		},
		{
			name:     "password keyword",
			path:     "utils/password_hash.go",
			expected: true,
		},
		{
			name:     "token keyword",
			path:     "api/token_service.go",
			expected: true,
		},
		{
			name:     "secret keyword",
			path:     "config/secret_manager.go",
			expected: true,
		},
		{
			name:     "migration keyword",
			path:     "db/migration_runner.go",
			expected: true,
		},
		{
			name:     "credential keyword",
			path:     "pkg/credential_store.go",
			expected: true,
		},
		{
			name:     "encrypt keyword",
			path:     "crypto/encrypt_data.go",
			expected: true,
		},
		{
			name:     "jwt keyword",
			path:     "auth/jwt_validator.go",
			expected: true,
		},
		{
			name:     "oauth keyword",
			path:     "api/oauth_handler.go",
			expected: true,
		},
		{
			name:     "permission keyword",
			path:     "access/permission_check.go",
			expected: true,
		},
		{
			name:     "rbac keyword",
			path:     "access/rbac_policy.go",
			expected: true,
		},
		// Case insensitivity
		{
			name:     "uppercase AUTH",
			path:     "internal/USER_AUTH.go",
			expected: true,
		},
		{
			name:     "mixed case Token",
			path:     "api/TokenService.go",
			expected: true,
		},
		// Non-matches
		{
			name:     "regular handler",
			path:     "internal/user_handler.go",
			expected: false,
		},
		{
			name:     "regular service",
			path:     "services/order_service.go",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := detector.IsProtected(tc.path)
			if result != tc.expected {
				t.Errorf("IsProtected(%q) = %v, expected %v", tc.path, result, tc.expected)
			}
		})
	}
}

func TestProtectedAreaDetector_FileTypeDetection(t *testing.T) {
	detector := NewProtectedAreaDetector()

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		// Protected file types
		{
			name:     "SQL file",
			path:     "db/schema.sql",
			expected: true,
		},
		{
			name:     "Terraform file",
			path:     "deploy/main.tf",
			expected: true,
		},
		{
			name:     "PEM file",
			path:     "certs/server.pem",
			expected: true,
		},
		{
			name:     "Key file",
			path:     "secrets/private.key",
			expected: true,
		},
		{
			name:     "env file",
			path:     "config/.env",
			expected: true,
		},
		{
			name:     "P12 file",
			path:     "certs/client.p12",
			expected: true,
		},
		{
			name:     "PFX file",
			path:     "certs/server.pfx",
			expected: true,
		},
		{
			name:     "JKS file",
			path:     "java/keystore.jks",
			expected: true,
		},
		{
			name:     "certificate file",
			path:     "certs/server.crt",
			expected: true,
		},
		{
			name:     "CER file",
			path:     "certs/ca.cer",
			expected: true,
		},
		// Case insensitivity for extensions
		{
			name:     "uppercase SQL",
			path:     "db/SCHEMA.SQL",
			expected: true,
		},
		// Non-protected file types
		{
			name:     "Go file",
			path:     "main.go",
			expected: false,
		},
		{
			name:     "YAML file",
			path:     "config/app.yaml",
			expected: false,
		},
		{
			name:     "JSON file",
			path:     "package.json",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := detector.IsProtected(tc.path)
			if result != tc.expected {
				t.Errorf("IsProtected(%q) = %v, expected %v", tc.path, result, tc.expected)
			}
		})
	}
}

func TestProtectedAreaDetector_AddCustomPatterns(t *testing.T) {
	detector := NewProtectedAreaDetector()

	// Initially not protected
	if detector.IsProtected("custom/special/file.go") {
		t.Error("expected custom path to not be protected initially")
	}

	// Add custom pattern
	detector.AddPattern("**/custom/**")

	// Now should be protected
	if !detector.IsProtected("custom/special/file.go") {
		t.Error("expected custom path to be protected after adding pattern")
	}
}

func TestProtectedAreaDetector_AddCustomKeywords(t *testing.T) {
	detector := NewProtectedAreaDetector()

	// Initially not protected
	if detector.IsProtected("internal/foobar_handler.go") {
		t.Error("expected foobar path to not be protected initially")
	}

	// Add custom keyword
	detector.AddKeyword("foobar")

	// Now should be protected
	if !detector.IsProtected("internal/foobar_handler.go") {
		t.Error("expected foobar path to be protected after adding keyword")
	}
}

func TestProtectedAreaDetector_AddCustomFileTypes(t *testing.T) {
	detector := NewProtectedAreaDetector()

	// Initially not protected
	if detector.IsProtected("config/app.xyz") {
		t.Error("expected .xyz file to not be protected initially")
	}

	// Add custom file type
	detector.AddFileType(".xyz")

	// Now should be protected
	if !detector.IsProtected("config/app.xyz") {
		t.Error("expected .xyz file to be protected after adding file type")
	}
}

func TestProtectedAreaDetector_LoadConfig(t *testing.T) {
	// Create temporary config file
	tmpDir, err := os.MkdirTemp("", "protected-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, ".alphie.yaml")
	configContent := `protected_areas:
  patterns:
    - "**/custom_area/**"
  keywords:
    - customkeyword
  file_types:
    - ".custom"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	detector := NewProtectedAreaDetector()

	// Load config
	if err := detector.LoadConfig(configPath); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Test custom pattern
	if !detector.IsProtected("internal/custom_area/file.go") {
		t.Error("expected custom_area pattern to be protected")
	}

	// Test custom keyword
	if !detector.IsProtected("internal/customkeyword_handler.go") {
		t.Error("expected customkeyword to be protected")
	}

	// Test custom file type
	if !detector.IsProtected("config/app.custom") {
		t.Error("expected .custom file type to be protected")
	}
}

func TestMatchGlobPattern(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		pattern  string
		expected bool
	}{
		{
			name:     "double star matches deep path",
			path:     "a/b/c/d/file.go",
			pattern:  "**/c/**",
			expected: true,
		},
		{
			name:     "double star at start",
			path:     "internal/auth/login.go",
			pattern:  "**/auth/**",
			expected: true,
		},
		{
			name:     "double star at end",
			path:     "migrations/001_init.sql",
			pattern:  "migrations/**",
			expected: true,
		},
		{
			name:     "literal match",
			path:     "config/settings.yaml",
			pattern:  "config/settings.yaml",
			expected: true,
		},
		{
			name:     "single star in segment",
			path:     "internal/auth_handler.go",
			pattern:  "internal/auth*",
			expected: true,
		},
		{
			name:     "no match - different path",
			path:     "api/handler.go",
			pattern:  "**/auth/**",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := matchGlobPattern(tc.path, tc.pattern)
			if result != tc.expected {
				t.Errorf("matchGlobPattern(%q, %q) = %v, expected %v", tc.path, tc.pattern, result, tc.expected)
			}
		})
	}
}

func TestNewProtectedAreaDetector(t *testing.T) {
	detector := NewProtectedAreaDetector()

	if detector == nil {
		t.Fatal("expected non-nil detector")
	}

	// Check that default patterns are loaded
	if len(detector.patterns) == 0 {
		t.Error("expected default patterns to be loaded")
	}
	if len(detector.keywords) == 0 {
		t.Error("expected default keywords to be loaded")
	}
	if len(detector.fileTypes) == 0 {
		t.Error("expected default file types to be loaded")
	}
}

func TestProtectedAreaDetector_PathNormalization(t *testing.T) {
	detector := NewProtectedAreaDetector()

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "forward slashes",
			path:     "internal/auth/login.go",
			expected: true,
		},
		{
			name:     "backslashes normalized",
			path:     "internal\\auth\\login.go",
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := detector.IsProtected(tc.path)
			if result != tc.expected {
				t.Errorf("IsProtected(%q) = %v, expected %v", tc.path, result, tc.expected)
			}
		})
	}
}
