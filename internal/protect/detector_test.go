package protect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	detector := New()
	if detector == nil {
		t.Fatal("expected non-nil detector")
	}
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

func TestDetector_PatternMatching(t *testing.T) {
	detector := New()

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"auth directory", "internal/auth/login.go", true},
		{"auth root", "auth/handler.go", true},
		{"migrations", "db/migrations/001_create_users.sql", true},
		{"security", "pkg/security/crypto.go", true},
		{"terraform", "infra/terraform/main.tf", true},
		{"k8s", "k8s/deployment.yaml", true},
		{"helm", "deploy/helm/values.yaml", true},
		{"regular file", "internal/handler/api.go", false},
		{"test file", "internal/handler/api_test.go", false},
		{"docs", "docs/README.md", false},
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

func TestDetector_KeywordDetection(t *testing.T) {
	detector := New()

	tests := []struct {
		path     string
		expected bool
	}{
		{"internal/user_auth.go", true},
		{"handlers/login_handler.go", true},
		{"utils/password_hash.go", true},
		{"api/token_service.go", true},
		{"config/secret_manager.go", true},
		{"db/migration_runner.go", true},
		{"pkg/credential_store.go", true},
		{"crypto/encrypt_data.go", true},
		{"auth/jwt_validator.go", true},
		{"api/oauth_handler.go", true},
		{"access/permission_check.go", true},
		{"access/rbac_policy.go", true},
		{"internal/USER_AUTH.go", true},       // case insensitive
		{"api/TokenService.go", true},         // mixed case
		{"internal/user_handler.go", false},   // no keyword
		{"services/order_service.go", false},  // no keyword
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			result := detector.IsProtected(tc.path)
			if result != tc.expected {
				t.Errorf("IsProtected(%q) = %v, expected %v", tc.path, result, tc.expected)
			}
		})
	}
}

func TestDetector_FileTypeDetection(t *testing.T) {
	detector := New()

	tests := []struct {
		path     string
		expected bool
	}{
		{"db/schema.sql", true},
		{"deploy/main.tf", true},
		{"certs/server.pem", true},
		{"secrets/private.key", true},
		{"config/.env", true},
		{"certs/client.p12", true},
		{"certs/server.pfx", true},
		{"java/keystore.jks", true},
		{"certs/server.crt", true},
		{"certs/ca.cer", true},
		{"db/SCHEMA.SQL", true},  // case insensitive
		{"main.go", false},
		{"config/app.yaml", false},
		{"package.json", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			result := detector.IsProtected(tc.path)
			if result != tc.expected {
				t.Errorf("IsProtected(%q) = %v, expected %v", tc.path, result, tc.expected)
			}
		})
	}
}

func TestDetector_AddCustomPatterns(t *testing.T) {
	detector := New()

	if detector.IsProtected("custom/special/file.go") {
		t.Error("expected custom path to not be protected initially")
	}

	detector.AddPattern("**/custom/**")

	if !detector.IsProtected("custom/special/file.go") {
		t.Error("expected custom path to be protected after adding pattern")
	}
}

func TestDetector_AddCustomKeywords(t *testing.T) {
	detector := New()

	if detector.IsProtected("internal/foobar_handler.go") {
		t.Error("expected foobar path to not be protected initially")
	}

	detector.AddKeyword("foobar")

	if !detector.IsProtected("internal/foobar_handler.go") {
		t.Error("expected foobar path to be protected after adding keyword")
	}
}

func TestDetector_AddCustomFileTypes(t *testing.T) {
	detector := New()

	if detector.IsProtected("config/app.xyz") {
		t.Error("expected .xyz file to not be protected initially")
	}

	detector.AddFileType(".xyz")

	if !detector.IsProtected("config/app.xyz") {
		t.Error("expected .xyz file to be protected after adding file type")
	}
}

func TestDetector_LoadConfig(t *testing.T) {
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

	detector := New()

	if err := detector.LoadConfig(configPath); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if !detector.IsProtected("internal/custom_area/file.go") {
		t.Error("expected custom_area pattern to be protected")
	}

	if !detector.IsProtected("internal/customkeyword_handler.go") {
		t.Error("expected customkeyword to be protected")
	}

	if !detector.IsProtected("config/app.custom") {
		t.Error("expected .custom file type to be protected")
	}
}

func TestDetector_PathNormalization(t *testing.T) {
	detector := New()

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"forward slashes", "internal/auth/login.go", true},
		{"backslashes", "internal\\auth\\login.go", true},
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
