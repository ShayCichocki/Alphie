package protect

import "testing"

func TestMatchGlobPattern(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		pattern  string
		expected bool
	}{
		{"double star matches deep path", "a/b/c/d/file.go", "**/c/**", true},
		{"double star at start", "internal/auth/login.go", "**/auth/**", true},
		{"double star at end", "migrations/001_init.sql", "migrations/**", true},
		{"literal match", "config/settings.yaml", "config/settings.yaml", true},
		{"single star in segment", "internal/auth_handler.go", "internal/auth*", true},
		{"no match - different path", "api/handler.go", "**/auth/**", false},
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
