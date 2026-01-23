package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	initForce          bool
	initNoGit          bool
	initProjectName    string
	initWithConfigs    bool
	initSkipClaudeCheck bool
)

var initCmd = &cobra.Command{
	Use:   "init [directory]",
	Short: "Initialize an Alphie project",
	Long: `Initialize a directory for use with Alphie.

This command sets up everything needed to run Alphie:
  - Verifies prerequisites (git, claude CLI)
  - Initializes git repository if needed
  - Creates .alphie directory structure
  - Optionally creates example configuration files

The directory argument is optional and defaults to the current directory.

Examples:
  alphie init              # Initialize current directory
  alphie init ./myproject  # Initialize specific directory
  alphie init --force      # Reinitialize even if already set up
  alphie init --no-git     # Skip git initialization
  alphie init --with-configs  # Create example tier config files`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInit,
}

func init() {
	initCmd.Flags().BoolVar(&initForce, "force", false, "Reinitialize even if already set up")
	initCmd.Flags().BoolVar(&initNoGit, "no-git", false, "Skip git initialization")
	initCmd.Flags().StringVar(&initProjectName, "project-name", "", "Override auto-detected project name")
	initCmd.Flags().BoolVar(&initWithConfigs, "with-configs", false, "Create example tier configuration files")
	initCmd.Flags().BoolVar(&initSkipClaudeCheck, "skip-claude-check", false, "Skip Claude CLI availability check")
}

func runInit(cmd *cobra.Command, args []string) error {
	// Step 1: Resolve target directory
	targetDir := "."
	if len(args) > 0 {
		targetDir = args[0]
	}

	absPath, err := filepath.Abs(targetDir)
	if err != nil {
		return fmt.Errorf("resolving absolute path: %w", err)
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(absPath, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", absPath, err)
	}

	// Change to target directory
	if err := os.Chdir(absPath); err != nil {
		return fmt.Errorf("changing to directory %s: %w", absPath, err)
	}

	fmt.Printf("Initializing Alphie in %s...\n\n", absPath)

	// Step 2: Check if already initialized
	alphieDir := filepath.Join(absPath, ".alphie")
	if _, err := os.Stat(alphieDir); err == nil && !initForce {
		fmt.Printf("Directory already initialized. Use --force to reinitialize.\n")
		return nil
	}

	// Step 3: Verify prerequisites
	if err := checkGitInstalled(); err != nil {
		printStatus("✗", "Git not found", color.FgRed)
		return err
	}
	printStatus("✓", "Git found", color.FgGreen)

	if !initSkipClaudeCheck {
		if err := CheckClaudeCLI(); err != nil {
			printStatus("✗", "Claude Code CLI not found", color.FgRed)
			return err
		}
		printStatus("✓", "Claude Code CLI found", color.FgGreen)
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		printStatus("⚠", "ANTHROPIC_API_KEY not set (you can set it later)", color.FgYellow)
	} else {
		printStatus("✓", "ANTHROPIC_API_KEY is set", color.FgGreen)
	}

	// Step 4: Git initialization (unless --no-git)
	if !initNoGit {
		if err := initGitRepo(absPath); err != nil {
			return err
		}
	} else {
		fmt.Println("Skipping git initialization (--no-git flag)")
	}

	// Step 5: Create .alphie structure
	// alphieDir already defined above
	if err := os.MkdirAll(alphieDir, 0755); err != nil {
		return fmt.Errorf("creating .alphie directory: %w", err)
	}

	logsDir := filepath.Join(alphieDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("creating .alphie/logs directory: %w", err)
	}
	printStatus("✓", "Created .alphie directory structure", color.FgGreen)

	// State and learning databases removed - keeping stateless

	// Step 6: Update .gitignore
	if !initNoGit {
		if err := updateGitignore(absPath); err != nil {
			return fmt.Errorf("updating .gitignore: %w", err)
		}
		printStatus("✓", "Updated .gitignore with Alphie entries", color.FgGreen)
	}

	// Step 7: Create config files (if --with-configs)
	if initWithConfigs {
		if err := createExampleConfigs(absPath); err != nil {
			return fmt.Errorf("creating example configs: %w", err)
		}
		printStatus("✓", "Created example tier configurations in configs/", color.FgGreen)

		if err := createProjectConfig(absPath); err != nil {
			return fmt.Errorf("creating project config: %w", err)
		}
		printStatus("✓", "Created .alphie.yaml template", color.FgGreen)
	}

	// Step 8: Success message
	projectName := initProjectName
	if projectName == "" {
		projectName = detectProjectName(absPath)
	}

	branch := "main"
	if !initNoGit {
		if b, err := getCurrentBranch(); err == nil {
			branch = b
		}
	}

	fmt.Printf("\n%s Alphie initialization complete!\n\n", color.GreenString("✓"))
	fmt.Println("Next steps:")
	if apiKey == "" {
		fmt.Println("  1. Set your API key:")
		fmt.Println("     export ANTHROPIC_API_KEY=your-key-here")
		fmt.Println()
	}
	fmt.Println("  2. Run Alphie:")
	fmt.Println("     alphie run \"your task here\"")
	fmt.Println("     # or: alphie (for interactive mode)")
	fmt.Println()
	fmt.Println("  3. Learn more:")
	fmt.Println("     alphie --help")
	fmt.Println()
	fmt.Println("Project details:")
	fmt.Printf("  Project name: %s\n", projectName)
	fmt.Printf("  Repository: %s\n", absPath)
	if !initNoGit {
		fmt.Printf("  Main branch: %s\n", branch)
	}

	return nil
}

// checkGitInstalled checks if git is installed
func checkGitInstalled() error {
	_, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git not found in PATH\n\n" +
			"Alphie requires git to manage code changes.\n\n" +
			"Install git with:\n" +
			"  - macOS: brew install git\n" +
			"  - Ubuntu/Debian: sudo apt-get install git\n" +
			"  - Other: https://git-scm.com/downloads")
	}
	return nil
}

// initGitRepo initializes git repository and ensures basic requirements
func initGitRepo(repoPath string) error {
	// Check if .git exists
	gitDir := filepath.Join(repoPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		// Initialize git repo
		cmd := exec.Command("git", "init")
		cmd.Dir = repoPath
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git init failed: %s\n%s", err, string(output))
		}
		printStatus("✓", "Initialized git repository", color.FgGreen)
	} else {
		printStatus("✓", "Git repository exists", color.FgGreen)
	}

	// Check for commits
	hasCommits, err := hasAnyCommits(repoPath)
	if err != nil {
		return fmt.Errorf("checking for commits: %w", err)
	}

	if !hasCommits {
		if err := ensureInitialCommit(repoPath); err != nil {
			return fmt.Errorf("creating initial commit: %w", err)
		}
		printStatus("✓", "Created initial commit", color.FgGreen)
	} else {
		printStatus("✓", "Git repository has commits", color.FgGreen)
	}

	// Ensure main/master branch exists
	if err := ensureMainBranch(repoPath); err != nil {
		return fmt.Errorf("ensuring main branch: %w", err)
	}
	printStatus("✓", "Main branch exists", color.FgGreen)

	return nil
}

// hasAnyCommits checks if the repository has any commits
func hasAnyCommits(repoPath string) (bool, error) {
	cmd := exec.Command("git", "rev-list", "-n", "1", "--all")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Exit code 128 typically means no commits
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 128 {
			return false, nil
		}
		return false, fmt.Errorf("git rev-list failed: %s", string(output))
	}

	// If output is empty, no commits
	return len(strings.TrimSpace(string(output))) > 0, nil
}

// ensureInitialCommit creates an initial commit if needed
func ensureInitialCommit(repoPath string) error {
	// Create or update .gitignore
	gitignorePath := filepath.Join(repoPath, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		// Create minimal .gitignore
		content := "# Alphie\n.alphie/logs/\nalphie\n"
		if err := os.WriteFile(gitignorePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("creating .gitignore: %w", err)
		}
	}

	// Add and commit
	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = repoPath
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %s\n%s", err, string(output))
	}

	// Allow empty commit in case nothing to add
	commitCmd := exec.Command("git", "commit", "--allow-empty", "-m", "Initial commit")
	commitCmd.Dir = repoPath
	if output, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit failed: %s\n%s", err, string(output))
	}

	return nil
}

// ensureMainBranch ensures the primary branch is named "main".
// If "master" exists but "main" doesn't, renames master to main.
// This ensures greenfield mode always has a consistent "main" branch.
func ensureMainBranch(repoPath string) error {
	// Check if main already exists
	mainCmd := exec.Command("git", "rev-parse", "--verify", "main")
	mainCmd.Dir = repoPath
	if err := mainCmd.Run(); err == nil {
		return nil // main exists, we're done
	}

	// Check if master exists - if so, rename it to main
	masterCmd := exec.Command("git", "rev-parse", "--verify", "master")
	masterCmd.Dir = repoPath
	if err := masterCmd.Run(); err == nil {
		// master exists, rename it to main for consistency
		renameCmd := exec.Command("git", "branch", "-M", "main")
		renameCmd.Dir = repoPath
		if output, err := renameCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("renaming master to main: %s\n%s", err, string(output))
		}
		return nil
	}

	// Neither exists - rename current branch to main
	renameCmd := exec.Command("git", "branch", "-M", "main")
	renameCmd.Dir = repoPath
	if output, err := renameCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("creating main branch: %s\n%s", err, string(output))
	}

	return nil
}

// getCurrentBranch returns the current git branch
func getCurrentBranch() (string, error) {
	cmd := exec.Command("git", "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// updateGitignore adds Alphie entries to .gitignore if not present
func updateGitignore(repoPath string) error {
	gitignorePath := filepath.Join(repoPath, ".gitignore")

	// Read existing .gitignore or create new
	var existingContent string
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existingContent = string(data)
	}

	// Check if Alphie entries already exist
	alphieEntries := []string{
		".alphie/learnings.db*",
		".alphie/logs/",
		"alphie",
	}

	needsUpdate := false
	for _, entry := range alphieEntries {
		if !strings.Contains(existingContent, entry) {
			needsUpdate = true
			break
		}
	}

	if !needsUpdate {
		return nil // Already has all entries
	}

	// Append Alphie entries
	var newContent strings.Builder
	newContent.WriteString(existingContent)

	if len(existingContent) > 0 && !strings.HasSuffix(existingContent, "\n") {
		newContent.WriteString("\n")
	}

	newContent.WriteString("\n# Alphie\n")
	for _, entry := range alphieEntries {
		if !strings.Contains(existingContent, entry) {
			newContent.WriteString(entry + "\n")
		}
	}

	return os.WriteFile(gitignorePath, []byte(newContent.String()), 0644)
}

// createExampleConfigs creates example configuration files
// Note: Tier configs have been removed as part of simplification.
// This function is kept for future config file examples.
func createExampleConfigs(repoPath string) error {
	// Tier configs removed - no longer needed
	// This function is kept as a placeholder for future config examples
	return nil
}

// createProjectConfig creates .alphie.yaml template
func createProjectConfig(repoPath string) error {
	configPath := filepath.Join(repoPath, ".alphie.yaml")

	// Check if already exists
	if _, err := os.Stat(configPath); err == nil {
		return nil // Already exists, don't overwrite
	}

	template := `# Alphie Project Configuration
# This file overrides defaults from ~/.config/alphie/config.yaml

# defaults:
#   tier: builder
#   token_budget: 100000

# quality_gates:
#   test: true
#   build: true
#   lint: true
#   typecheck: true

# timeouts:
#   scout: 5m
#   builder: 15m
#   architect: 30m
`

	return os.WriteFile(configPath, []byte(template), 0644)
}

// detectProjectName detects project name from directory
func detectProjectName(repoPath string) string {
	// Try to get from git remote
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	cmd.Dir = repoPath
	if output, err := cmd.Output(); err == nil {
		// Extract repo name from URL
		url := strings.TrimSpace(string(output))
		// Remove .git suffix
		url = strings.TrimSuffix(url, ".git")
		// Get last component
		parts := strings.Split(url, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	// Fallback to directory name
	return filepath.Base(repoPath)
}

// printStatus prints a status line with color
func printStatus(symbol, message string, colorAttr color.Attribute) {
	c := color.New(colorAttr)
	fmt.Printf("%s %s\n", c.Sprint(symbol), message)
}
