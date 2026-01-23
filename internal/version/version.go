package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var versionContent string

// Get returns the current version, with whitespace trimmed
func Get() string {
	return strings.TrimSpace(versionContent)
}
