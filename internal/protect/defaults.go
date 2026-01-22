// Package protect provides protected area detection for sensitive files.
package protect

// DefaultPatterns defines glob patterns for protected areas.
var DefaultPatterns = []string{
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

// DefaultKeywords defines substrings that indicate protected files.
var DefaultKeywords = []string{
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

// DefaultFileTypes defines file extensions that are protected.
var DefaultFileTypes = []string{
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
