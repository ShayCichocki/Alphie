// Package merge provides smart merge logic for package manager files.
package merge

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// SmartMergeResult contains the outcome of a smart merge operation.
type SmartMergeResult struct {
	// Success indicates whether all files were merged successfully.
	Success bool
	// MergedFiles maps file paths to their merged content.
	MergedFiles map[string][]byte
	// Conflicts lists files that couldn't be automatically merged.
	Conflicts []string
	// RegenerateCommands lists commands to run after merge (e.g., npm install).
	RegenerateCommands []string
}

// SmartMerge attempts to merge critical files using format-aware logic.
// It handles package.json, go.mod, and other common package manager files.
func SmartMerge(repoPath string, conflictFiles []string, sessionBranch, agentBranch string) (*SmartMergeResult, error) {
	result := &SmartMergeResult{
		MergedFiles: make(map[string][]byte),
	}

	mergeable, regenerate := CategorizeCriticalFiles(conflictFiles)

	for _, lockFile := range regenerate {
		if cmd := GetLockFileCommand(lockFile); cmd != "" {
			result.RegenerateCommands = append(result.RegenerateCommands, cmd)
		}
	}

	for _, file := range mergeable {
		merged, err := smartMergeFile(repoPath, file, sessionBranch, agentBranch)
		if err != nil {
			result.Conflicts = append(result.Conflicts, file)
			continue
		}
		result.MergedFiles[file] = merged
	}

	result.Success = len(result.Conflicts) == 0
	return result, nil
}

func smartMergeFile(repoPath, file, sessionBranch, agentBranch string) ([]byte, error) {
	base := filepath.Base(file)

	switch {
	case base == "package.json":
		return smartMergePackageJSON(repoPath, file, sessionBranch, agentBranch)
	case base == "go.mod":
		return smartMergeGoMod(repoPath, file, sessionBranch, agentBranch)
	case base == "Cargo.toml":
		return smartMergeCargoToml(repoPath, file, sessionBranch, agentBranch)
	case base == "pyproject.toml":
		return smartMergePyprojectToml(repoPath, file, sessionBranch, agentBranch)
	case base == "requirements.txt":
		return smartMergeRequirementsTxt(repoPath, file, sessionBranch, agentBranch)
	case base == "tsconfig.json" || base == "jsconfig.json":
		return smartMergeTSConfig(repoPath, file, sessionBranch, agentBranch)
	case base == ".gitignore":
		return smartMergeGitignore(repoPath, file, sessionBranch, agentBranch)
	default:
		if strings.HasSuffix(file, ".json") {
			return smartMergeGenericJSON(repoPath, file, sessionBranch, agentBranch)
		}
		if strings.HasSuffix(file, ".toml") {
			return smartMergeGenericToml(repoPath, file, sessionBranch, agentBranch)
		}
		return nil, fmt.Errorf("unsupported file format: %s", file)
	}
}

func getFileFromBranch(repoPath, file, branch string) ([]byte, error) {
	cmd := exec.Command("git", "show", branch+":"+file)
	cmd.Dir = repoPath
	return cmd.Output()
}

func smartMergePackageJSON(repoPath, file, sessionBranch, agentBranch string) ([]byte, error) {
	sessionContent, err := getFileFromBranch(repoPath, file, sessionBranch)
	if err != nil {
		sessionContent = []byte("{}")
	}

	agentContent, err := getFileFromBranch(repoPath, file, agentBranch)
	if err != nil {
		return nil, fmt.Errorf("get agent content: %w", err)
	}

	var sessionPkg, agentPkg map[string]interface{}
	if err := json.Unmarshal(sessionContent, &sessionPkg); err != nil {
		sessionPkg = make(map[string]interface{})
	}
	if err := json.Unmarshal(agentContent, &agentPkg); err != nil {
		return nil, fmt.Errorf("parse agent package.json: %w", err)
	}

	result := sessionPkg

	stringMapKeys := []string{"dependencies", "devDependencies", "peerDependencies", "scripts"}
	for _, key := range stringMapKeys {
		result[key] = mergeStringMapsInterface(
			toStringMap(sessionPkg[key]),
			toStringMap(agentPkg[key]),
		)
	}

	if agentWs, ok := agentPkg["workspaces"].([]interface{}); ok {
		sessionWs, _ := sessionPkg["workspaces"].([]interface{})
		result["workspaces"] = mergeArrays(sessionWs, agentWs)
	}

	for key, value := range agentPkg {
		if _, exists := result[key]; !exists {
			result[key] = value
		}
	}

	return json.MarshalIndent(result, "", "  ")
}

func mergeStringMapsInterface(a, b map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range a {
		result[k] = v
	}
	for k, v := range b {
		result[k] = v
	}
	return result
}

func toStringMap(v interface{}) map[string]string {
	result := make(map[string]string)
	if m, ok := v.(map[string]interface{}); ok {
		for k, val := range m {
			if s, ok := val.(string); ok {
				result[k] = s
			}
		}
	}
	return result
}

func mergeArrays(a, b []interface{}) []interface{} {
	seen := make(map[string]bool)
	var strings []string

	for _, v := range a {
		if s, ok := v.(string); ok && !seen[s] {
			seen[s] = true
			strings = append(strings, s)
		}
	}

	for _, v := range b {
		if s, ok := v.(string); ok && !seen[s] {
			seen[s] = true
			strings = append(strings, s)
		}
	}

	sort.Strings(strings)

	result := make([]interface{}, len(strings))
	for i, s := range strings {
		result[i] = s
	}

	return result
}

func smartMergeGoMod(repoPath, file, sessionBranch, agentBranch string) ([]byte, error) {
	sessionContent, err := getFileFromBranch(repoPath, file, sessionBranch)
	if err != nil {
		sessionContent = []byte("")
	}

	agentContent, err := getFileFromBranch(repoPath, file, agentBranch)
	if err != nil {
		return nil, fmt.Errorf("get agent content: %w", err)
	}

	sessionRequires := parseGoModRequires(string(sessionContent))
	agentRequires := parseGoModRequires(string(agentContent))

	merged := make(map[string]string)
	for mod, ver := range sessionRequires {
		merged[mod] = ver
	}
	for mod, ver := range agentRequires {
		merged[mod] = ver
	}

	result := updateGoModRequires(string(agentContent), merged)
	return []byte(result), nil
}

func parseGoModRequires(content string) map[string]string {
	requires := make(map[string]string)

	singlePattern := regexp.MustCompile(`require\s+(\S+)\s+(\S+)`)
	for _, match := range singlePattern.FindAllStringSubmatch(content, -1) {
		if len(match) >= 3 {
			requires[match[1]] = match[2]
		}
	}

	blockPattern := regexp.MustCompile(`require\s*\(([\s\S]*?)\)`)
	for _, match := range blockPattern.FindAllStringSubmatch(content, -1) {
		if len(match) >= 2 {
			lines := strings.Split(match[1], "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					requires[parts[0]] = parts[1]
				}
			}
		}
	}

	return requires
}

func updateGoModRequires(content string, requires map[string]string) string {
	var mods []string
	for mod := range requires {
		mods = append(mods, mod)
	}
	sort.Strings(mods)

	var requireLines []string
	for _, mod := range mods {
		requireLines = append(requireLines, fmt.Sprintf("\t%s %s", mod, requires[mod]))
	}

	blockPattern := regexp.MustCompile(`require\s*\([\s\S]*?\)`)
	newBlock := fmt.Sprintf("require (\n%s\n)", strings.Join(requireLines, "\n"))

	if blockPattern.MatchString(content) {
		return blockPattern.ReplaceAllString(content, newBlock)
	}

	return content + "\n" + newBlock
}

func smartMergeTSConfig(repoPath, file, sessionBranch, agentBranch string) ([]byte, error) {
	return smartMergeGenericJSON(repoPath, file, sessionBranch, agentBranch)
}

func smartMergeGenericJSON(repoPath, file, sessionBranch, agentBranch string) ([]byte, error) {
	sessionContent, err := getFileFromBranch(repoPath, file, sessionBranch)
	if err != nil {
		sessionContent = []byte("{}")
	}

	agentContent, err := getFileFromBranch(repoPath, file, agentBranch)
	if err != nil {
		return nil, fmt.Errorf("get agent content: %w", err)
	}

	var sessionObj, agentObj map[string]interface{}
	json.Unmarshal(sessionContent, &sessionObj)
	if err := json.Unmarshal(agentContent, &agentObj); err != nil {
		return nil, fmt.Errorf("parse agent JSON: %w", err)
	}

	result := deepMerge(sessionObj, agentObj)
	return json.MarshalIndent(result, "", "  ")
}

func deepMerge(a, b map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for k, v := range a {
		result[k] = v
	}

	for k, v := range b {
		if aVal, exists := result[k]; exists {
			aMap, aIsMap := aVal.(map[string]interface{})
			bMap, bIsMap := v.(map[string]interface{})
			if aIsMap && bIsMap {
				result[k] = deepMerge(aMap, bMap)
				continue
			}
		}
		result[k] = v
	}

	return result
}

func smartMergeGitignore(repoPath, file, sessionBranch, agentBranch string) ([]byte, error) {
	sessionContent, _ := getFileFromBranch(repoPath, file, sessionBranch)
	agentContent, err := getFileFromBranch(repoPath, file, agentBranch)
	if err != nil {
		return nil, fmt.Errorf("get agent content: %w", err)
	}

	sessionLines := strings.Split(string(sessionContent), "\n")
	agentLines := strings.Split(string(agentContent), "\n")

	seen := make(map[string]bool)
	var result []string

	for _, line := range sessionLines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			result = append(result, line)
		}
	}

	for _, line := range agentLines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			result = append(result, line)
		}
	}

	return []byte(strings.Join(result, "\n")), nil
}

// ApplySmartMerge writes merged files to disk and runs regeneration commands.
func ApplySmartMerge(repoPath string, result *SmartMergeResult) error {
	for file, content := range result.MergedFiles {
		fullPath := filepath.Join(repoPath, file)

		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}

		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			return fmt.Errorf("write %s: %w", file, err)
		}
	}

	for _, cmdStr := range result.RegenerateCommands {
		parts := strings.Fields(cmdStr)
		if len(parts) == 0 {
			continue
		}

		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Dir = repoPath
		_ = cmd.Run()
	}

	return nil
}

// smartMergeCargoToml merges Cargo.toml files by unioning dependencies.
// Cargo.toml uses TOML format with [dependencies], [dev-dependencies], etc. sections.
func smartMergeCargoToml(repoPath, file, sessionBranch, agentBranch string) ([]byte, error) {
	sessionContent, err := getFileFromBranch(repoPath, file, sessionBranch)
	if err != nil {
		sessionContent = []byte("")
	}

	agentContent, err := getFileFromBranch(repoPath, file, agentBranch)
	if err != nil {
		return nil, fmt.Errorf("get agent content: %w", err)
	}

	// Parse dependencies from both versions
	sessionDeps := parseTomlDependencies(string(sessionContent))
	agentDeps := parseTomlDependencies(string(agentContent))

	// Merge dependencies (agent wins on conflict)
	for section, deps := range agentDeps {
		if _, ok := sessionDeps[section]; !ok {
			sessionDeps[section] = make(map[string]string)
		}
		for pkg, ver := range deps {
			sessionDeps[section][pkg] = ver
		}
	}

	// Rebuild the file with merged dependencies
	result := updateTomlDependencies(string(agentContent), sessionDeps)
	return []byte(result), nil
}

// parseTomlDependencies extracts dependency sections from TOML content.
// Returns map[section]map[package]version (e.g., {"dependencies": {"serde": "1.0"}, "dev-dependencies": {...}})
func parseTomlDependencies(content string) map[string]map[string]string {
	result := make(map[string]map[string]string)
	depSections := []string{"dependencies", "dev-dependencies", "build-dependencies"}

	for _, section := range depSections {
		result[section] = make(map[string]string)
	}

	lines := strings.Split(content, "\n")
	currentSection := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for section header [section]
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			sectionName := strings.Trim(trimmed, "[]")
			currentSection = ""
			for _, ds := range depSections {
				if sectionName == ds {
					currentSection = ds
					break
				}
			}
			continue
		}

		// Parse dependency line: package = "version" or package = { version = "..." }
		if currentSection != "" && strings.Contains(trimmed, "=") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				pkg := strings.TrimSpace(parts[0])
				ver := strings.TrimSpace(parts[1])
				// Handle both simple "version" and { version = "..." } formats
				ver = strings.Trim(ver, `"'`)
				if pkg != "" && ver != "" {
					result[currentSection][pkg] = ver
				}
			}
		}
	}

	return result
}

// updateTomlDependencies updates TOML content with merged dependencies.
// This is a simplified approach that preserves the structure and updates values.
func updateTomlDependencies(content string, deps map[string]map[string]string) string {
	// For each dependency section, ensure all merged packages are present
	// This is a simplified implementation that may not preserve all formatting
	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))
	depSections := []string{"dependencies", "dev-dependencies", "build-dependencies"}

	currentSection := ""
	sectionPackages := make(map[string]bool) // track packages in current section

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for section header
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			// Before moving to new section, add any missing packages from old section
			if currentSection != "" {
				for pkg, ver := range deps[currentSection] {
					if !sectionPackages[pkg] {
						result = append(result, fmt.Sprintf("%s = %q", pkg, ver))
					}
				}
			}

			sectionName := strings.Trim(trimmed, "[]")
			currentSection = ""
			for _, ds := range depSections {
				if sectionName == ds {
					currentSection = ds
					sectionPackages = make(map[string]bool)
					break
				}
			}
			result = append(result, line)
			continue
		}

		// Track and potentially update dependency lines
		if currentSection != "" && strings.Contains(trimmed, "=") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				pkg := strings.TrimSpace(parts[0])
				sectionPackages[pkg] = true
				// Use merged version if available
				if ver, ok := deps[currentSection][pkg]; ok {
					result = append(result, fmt.Sprintf("%s = %q", pkg, ver))
					continue
				}
			}
		}

		result = append(result, line)
	}

	// Handle case where file ends in a dependency section
	if currentSection != "" {
		for pkg, ver := range deps[currentSection] {
			if !sectionPackages[pkg] {
				result = append(result, fmt.Sprintf("%s = %q", pkg, ver))
			}
		}
	}

	return strings.Join(result, "\n")
}

// smartMergePyprojectToml merges pyproject.toml files.
func smartMergePyprojectToml(repoPath, file, sessionBranch, agentBranch string) ([]byte, error) {
	// pyproject.toml is TOML format, similar to Cargo.toml
	// Dependencies are typically in [project.dependencies] or [tool.poetry.dependencies]
	return smartMergeGenericToml(repoPath, file, sessionBranch, agentBranch)
}

// smartMergeGenericToml provides a basic TOML merge by parsing and merging sections.
func smartMergeGenericToml(repoPath, file, sessionBranch, agentBranch string) ([]byte, error) {
	sessionContent, err := getFileFromBranch(repoPath, file, sessionBranch)
	if err != nil {
		sessionContent = []byte("")
	}

	agentContent, err := getFileFromBranch(repoPath, file, agentBranch)
	if err != nil {
		return nil, fmt.Errorf("get agent content: %w", err)
	}

	// For generic TOML, we use a line-based merge approach:
	// - Keep all unique lines from both versions
	// - For duplicate keys, agent version wins
	sessionLines := strings.Split(string(sessionContent), "\n")
	agentLines := strings.Split(string(agentContent), "\n")

	// Track key-value pairs by key
	keyValues := make(map[string]string)
	var result []string

	// First pass: collect all key-value pairs from session
	for _, line := range sessionLines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "=") && !strings.HasPrefix(trimmed, "#") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				keyValues[key] = line
			}
		}
	}

	// Second pass: merge with agent (agent wins)
	for _, line := range agentLines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "=") && !strings.HasPrefix(trimmed, "#") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				keyValues[key] = line
			}
		}
	}

	// Rebuild following agent's structure
	seen := make(map[string]bool)
	for _, line := range agentLines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "=") && !strings.HasPrefix(trimmed, "#") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				if !seen[key] {
					seen[key] = true
					result = append(result, keyValues[key])
				}
				continue
			}
		}
		result = append(result, line)
	}

	// Add any keys from session that weren't in agent
	for key, line := range keyValues {
		if !seen[key] {
			result = append(result, line)
		}
	}

	return []byte(strings.Join(result, "\n")), nil
}

// smartMergeRequirementsTxt merges requirements.txt files by unioning packages.
func smartMergeRequirementsTxt(repoPath, file, sessionBranch, agentBranch string) ([]byte, error) {
	sessionContent, err := getFileFromBranch(repoPath, file, sessionBranch)
	if err != nil {
		sessionContent = []byte("")
	}

	agentContent, err := getFileFromBranch(repoPath, file, agentBranch)
	if err != nil {
		return nil, fmt.Errorf("get agent content: %w", err)
	}

	// Parse packages from both versions
	sessionPkgs := parseRequirements(string(sessionContent))
	agentPkgs := parseRequirements(string(agentContent))

	// Merge (agent wins on conflict)
	for pkg, ver := range agentPkgs {
		sessionPkgs[pkg] = ver
	}

	// Rebuild sorted list
	var packages []string
	for pkg := range sessionPkgs {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)

	var result []string
	for _, pkg := range packages {
		ver := sessionPkgs[pkg]
		if ver != "" {
			result = append(result, pkg+ver)
		} else {
			result = append(result, pkg)
		}
	}

	return []byte(strings.Join(result, "\n")), nil
}

// parseRequirements parses a requirements.txt file into package -> version constraint map.
func parseRequirements(content string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}

		// Parse: package==version, package>=version, package, etc.
		// Find the first version specifier
		specifiers := []string{"==", ">=", "<=", "!=", "~=", ">", "<"}
		pkgEnd := len(line)
		verStart := ""

		for _, spec := range specifiers {
			if idx := strings.Index(line, spec); idx != -1 && idx < pkgEnd {
				pkgEnd = idx
				verStart = line[idx:]
			}
		}

		pkg := strings.TrimSpace(line[:pkgEnd])
		// Normalize package name (lowercase, replace _ with -)
		pkg = strings.ToLower(strings.ReplaceAll(pkg, "_", "-"))

		if pkg != "" {
			result[pkg] = verStart
		}
	}

	return result
}
