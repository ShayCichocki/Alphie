// Package verification provides pattern-based verification contract enhancement.
package verification

import (
	"strings"
)

// Contract is an alias for VerificationContract for pattern matching.
type Contract = VerificationContract

// Command is an alias for VerificationCommand for pattern matching.
type Command = VerificationCommand

// VerificationPattern defines a reusable verification pattern for common task types.
type VerificationPattern struct {
	// Name of the pattern
	Name string
	// Triggers are keywords that activate this pattern
	Triggers []string
	// Commands are verification commands to add
	Commands []CommandPattern
	// FileConstraints are file-level checks
	FileConstraints FileConstraintPattern
	// Description explains what this pattern verifies
	Description string
}

// CommandPattern represents a verification command template.
type CommandPattern struct {
	Command     string
	Expect      string
	Description string
	Required    bool
}

// FileConstraintPattern represents file-level constraints.
type FileConstraintPattern struct {
	MustExist      []string
	MustNotExist   []string
	MustNotChange  []string
}

// StandardPatterns contains common verification patterns.
var StandardPatterns = []VerificationPattern{
	{
		Name:        "authentication",
		Triggers:    []string{"auth", "login", "password", "session", "jwt", "token"},
		Description: "Authentication and security patterns",
		Commands: []CommandPattern{
			{
				Command:     "grep -rE 'bcrypt|argon2|scrypt|pbkdf2' --include='*.go' --include='*.ts' --include='*.js' --include='*.py' .",
				Expect:      "exit 0",
				Description: "Secure password hashing library used",
				Required:    true,
			},
			{
				Command:     "grep -rE 'session.*timeout|token.*expir|jwt.*expir' --include='*.go' --include='*.ts' --include='*.js' --include='*.py' .",
				Expect:      "exit 0",
				Description: "Session/token expiry implemented",
				Required:    false,
			},
		},
	},
	{
		Name:        "database_migration",
		Triggers:    []string{"migration", "schema", "database", "sql"},
		Description: "Database migration patterns",
		Commands: []CommandPattern{
			{
				Command:     "test -d migrations || test -d db/migrations || test -d sql/migrations",
				Expect:      "exit 0",
				Description: "Migration directory exists",
				Required:    true,
			},
		},
	},
	{
		Name:        "api_endpoint",
		Triggers:    []string{"api", "endpoint", "route", "handler", "controller"},
		Description: "API endpoint patterns",
		Commands: []CommandPattern{
			{
				Command:     "grep -rE 'validate|sanitize|escape' --include='*.go' --include='*.ts' --include='*.js' --include='*.py' .",
				Expect:      "exit 0",
				Description: "Input validation present",
				Required:    true,
			},
			{
				Command:     "grep -rE 'error.*handle|try.*catch|panic.*recover' --include='*.go' --include='*.ts' --include='*.js' --include='*.py' .",
				Expect:      "exit 0",
				Description: "Error handling implemented",
				Required:    true,
			},
		},
	},
	{
		Name:        "configuration",
		Triggers:    []string{"config", "settings", "environment"},
		Description: "Configuration file patterns",
		Commands: []CommandPattern{
			{
				Command:     "test -f config.yaml || test -f config.json || test -f .env.example || test -f config.toml",
				Expect:      "exit 0",
				Description: "Configuration file exists",
				Required:    true,
			},
		},
	},
	{
		Name:        "testing",
		Triggers:    []string{"test", "spec", "unit test", "integration test"},
		Description: "Testing patterns",
		Commands: []CommandPattern{
			{
				Command:     "test -d test || test -d tests || test -d __tests__",
				Expect:      "exit 0",
				Description: "Test directory exists",
				Required:    false,
			},
		},
	},
	{
		Name:        "documentation",
		Triggers:    []string{"readme", "documentation", "docs"},
		Description: "Documentation patterns",
		Commands: []CommandPattern{
			{
				Command:     "test -f README.md",
				Expect:      "exit 0",
				Description: "README exists",
				Required:    true,
			},
			{
				Command:     "wc -l README.md | awk '{if ($1 > 10) exit 0; else exit 1}'",
				Expect:      "exit 0",
				Description: "README has substantial content",
				Required:    false,
			},
		},
	},
	{
		Name:        "error_handling",
		Triggers:    []string{"error", "exception", "panic", "crash"},
		Description: "Error handling patterns",
		Commands: []CommandPattern{
			{
				Command:     "grep -rE 'try|catch|except|panic|recover|error' --include='*.go' --include='*.ts' --include='*.js' --include='*.py' .",
				Expect:      "exit 0",
				Description: "Error handling code present",
				Required:    true,
			},
		},
	},
	{
		Name:        "logging",
		Triggers:    []string{"log", "logging", "logger", "audit"},
		Description: "Logging patterns",
		Commands: []CommandPattern{
			{
				Command:     "grep -rE 'log\\.|logger\\.|logging\\.' --include='*.go' --include='*.ts' --include='*.js' --include='*.py' .",
				Expect:      "exit 0",
				Description: "Logging statements present",
				Required:    false,
			},
		},
	},
	{
		Name:        "security_headers",
		Triggers:    []string{"security", "headers", "cors", "csp"},
		Description: "Security header patterns",
		Commands: []CommandPattern{
			{
				Command:     "grep -rE 'X-Frame-Options|Content-Security-Policy|X-Content-Type-Options' --include='*.go' --include='*.ts' --include='*.js' --include='*.py' .",
				Expect:      "exit 0",
				Description: "Security headers configured",
				Required:    false,
			},
		},
	},
	{
		Name:        "rate_limiting",
		Triggers:    []string{"rate limit", "throttle", "rate-limit"},
		Description: "Rate limiting patterns",
		Commands: []CommandPattern{
			{
				Command:     "grep -rE 'rate.*limit|throttle|limiter' --include='*.go' --include='*.ts' --include='*.js' --include='*.py' .",
				Expect:      "exit 0",
				Description: "Rate limiting implementation present",
				Required:    true,
			},
		},
	},
}

// DetectPatterns analyzes a task description and file boundaries to determine applicable patterns.
func DetectPatterns(taskDescription string, fileBoundaries []string) []VerificationPattern {
	detected := []VerificationPattern{}
	descLower := strings.ToLower(taskDescription)

	// Combine task description and file paths for pattern matching
	searchText := descLower
	for _, boundary := range fileBoundaries {
		searchText += " " + strings.ToLower(boundary)
	}

	for _, pattern := range StandardPatterns {
		// Check if any trigger matches
		matched := false
		for _, trigger := range pattern.Triggers {
			if strings.Contains(searchText, strings.ToLower(trigger)) {
				matched = true
				break
			}
		}

		if matched {
			detected = append(detected, pattern)
		}
	}

	return detected
}

// ApplyPatterns enhances a contract with applicable patterns.
func ApplyPatterns(contract *Contract, patterns []VerificationPattern) {
	if contract == nil {
		return
	}

	// Track existing commands to avoid duplicates
	existingCmds := make(map[string]bool)
	for _, cmd := range contract.Commands {
		existingCmds[cmd.Command] = true
	}

	// Add commands from patterns
	for _, pattern := range patterns {
		for _, cmdPattern := range pattern.Commands {
			// Skip if command already exists
			if existingCmds[cmdPattern.Command] {
				continue
			}

			// Add command
			contract.Commands = append(contract.Commands, Command{
				Command:     cmdPattern.Command,
				Expect:      cmdPattern.Expect,
				Description: cmdPattern.Description + " (pattern: " + pattern.Name + ")",
				Required:    cmdPattern.Required,
			})
			existingCmds[cmdPattern.Command] = true
		}

		// Merge file constraints
		for _, file := range pattern.FileConstraints.MustExist {
			if !contains(contract.FileConstraints.MustExist, file) {
				contract.FileConstraints.MustExist = append(contract.FileConstraints.MustExist, file)
			}
		}
		for _, file := range pattern.FileConstraints.MustNotExist {
			if !contains(contract.FileConstraints.MustNotExist, file) {
				contract.FileConstraints.MustNotExist = append(contract.FileConstraints.MustNotExist, file)
			}
		}
	}
}

// contains checks if a string slice contains a value.
func contains(slice []string, value string) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}
