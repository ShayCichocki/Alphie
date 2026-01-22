// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"regexp"
	"strings"
)

// RequestType represents the classification of a user request.
type RequestType string

const (
	// RequestTypeSetup indicates setup/scaffolding work (single agent recommended)
	RequestTypeSetup RequestType = "SETUP"
	// RequestTypeFeature indicates feature implementation (parallel agents OK)
	RequestTypeFeature RequestType = "FEATURE"
	// RequestTypeBugfix indicates bug fixing (single focused agent)
	RequestTypeBugfix RequestType = "BUGFIX"
	// RequestTypeRefactor indicates refactoring work (depends on scope)
	RequestTypeRefactor RequestType = "REFACTOR"
)

// RequestAnalysis contains the result of analyzing a user request.
type RequestAnalysis struct {
	// Type is the classified request type.
	Type RequestType
	// Confidence is how confident we are in the classification (0.0-1.0).
	Confidence float64
	// RecommendQuickMode indicates whether to use single-agent quick mode.
	RecommendQuickMode bool
	// MaxAgents is the recommended maximum number of concurrent agents.
	MaxAgents int
	// Keywords are the matched keywords that influenced the classification.
	Keywords []string
}

// RequestAnalyzer classifies user requests to determine the optimal execution strategy.
type RequestAnalyzer struct {
	setupPatterns    []*regexp.Regexp
	featurePatterns  []*regexp.Regexp
	bugfixPatterns   []*regexp.Regexp
	refactorPatterns []*regexp.Regexp
}

// NewRequestAnalyzer creates a new RequestAnalyzer with default patterns.
func NewRequestAnalyzer() *RequestAnalyzer {
	return &RequestAnalyzer{
		setupPatterns: compilePatterns([]string{
			`\b(setup|set up|set-up)\b`,
			`\b(scaffold|scaffolding)\b`,
			`\b(initialize|initialise|init)\b`,
			`\b(create new|create a new|start new)\b`,
			`\b(configure|configuration)\b`,
			`\b(install|installation)\b`,
			`\b(bootstrap)\b`,
			`\b(new project|new app|new application)\b`,
			`\b(eslint|prettier|typescript|vite|webpack)\s+(config|setup)\b`,
		}),
		featurePatterns: compilePatterns([]string{
			`\b(implement|implementing)\b`,
			`\b(add|adding)\s+(a\s+)?(new\s+)?feature\b`,
			`\b(build|building)\s+(a\s+)?(new\s+)?feature\b`,
			`\b(create|creating)\s+(api|endpoint|route|page|component)\b`,
			`\b(add|adding)\s+(api|endpoint|route|page|component)\b`,
			`\b(implement|add)\s+.*\s+(crud|authentication|auth)\b`,
			`\b(user\s+)?(login|signup|registration)\b`,
			`\b(dashboard|admin\s+panel)\b`,
		}),
		bugfixPatterns: compilePatterns([]string{
			`\b(fix|fixing)\b`,
			`\b(bug|bugs)\b`,
			`\b(debug|debugging)\b`,
			`\b(resolve|resolving)\b`,
			`\b(patch|patching)\b`,
			`\b(repair|repairing)\b`,
			`\b(broken|not working|doesn't work|doesn't work)\b`,
			`\b(error|errors)\b`,
			`\b(issue|issues)\b`,
		}),
		refactorPatterns: compilePatterns([]string{
			`\b(refactor|refactoring)\b`,
			`\b(reorganize|reorganise|reorganizing)\b`,
			`\b(restructure|restructuring)\b`,
			`\b(improve|improving)\s+(code|structure|architecture)\b`,
			`\b(optimize|optimise|optimizing)\b`,
			`\b(clean\s*up|cleanup)\b`,
			`\b(simplify|simplifying)\b`,
			`\b(extract|extracting)\s+(to|into)\b`,
		}),
	}
}

// compilePatterns compiles a slice of pattern strings into regexps.
func compilePatterns(patterns []string) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		if r, err := regexp.Compile("(?i)" + p); err == nil {
			compiled = append(compiled, r)
		}
	}
	return compiled
}

// Analyze classifies a user request and returns an analysis with recommendations.
func (a *RequestAnalyzer) Analyze(request string) *RequestAnalysis {
	lower := strings.ToLower(request)

	// Count matches for each type
	setupMatches := a.countMatches(lower, a.setupPatterns)
	featureMatches := a.countMatches(lower, a.featurePatterns)
	bugfixMatches := a.countMatches(lower, a.bugfixPatterns)
	refactorMatches := a.countMatches(lower, a.refactorPatterns)

	// Collect matched keywords
	var matchedKeywords []string
	matchedKeywords = append(matchedKeywords, a.getMatchedKeywords(lower, a.setupPatterns)...)
	matchedKeywords = append(matchedKeywords, a.getMatchedKeywords(lower, a.featurePatterns)...)
	matchedKeywords = append(matchedKeywords, a.getMatchedKeywords(lower, a.bugfixPatterns)...)
	matchedKeywords = append(matchedKeywords, a.getMatchedKeywords(lower, a.refactorPatterns)...)

	// Determine winner
	maxMatches := max(setupMatches, featureMatches, bugfixMatches, refactorMatches)
	totalMatches := setupMatches + featureMatches + bugfixMatches + refactorMatches

	// Default to FEATURE if no clear winner
	result := &RequestAnalysis{
		Type:       RequestTypeFeature,
		Confidence: 0.5,
		MaxAgents:  4,
		Keywords:   matchedKeywords,
	}

	if maxMatches == 0 {
		// No patterns matched, analyze request length and content
		return a.analyzeByContent(request, result)
	}

	// Calculate confidence as proportion of max to total
	if totalMatches > 0 {
		result.Confidence = float64(maxMatches) / float64(totalMatches)
	}

	switch {
	case setupMatches == maxMatches:
		result.Type = RequestTypeSetup
		result.RecommendQuickMode = true
		result.MaxAgents = 1 // Setup should be single agent
	case bugfixMatches == maxMatches:
		result.Type = RequestTypeBugfix
		result.RecommendQuickMode = true
		result.MaxAgents = 1 // Bugfix should be single focused agent
	case refactorMatches == maxMatches:
		result.Type = RequestTypeRefactor
		result.MaxAgents = 2 // Refactor is moderate parallelism
	case featureMatches == maxMatches:
		result.Type = RequestTypeFeature
		result.MaxAgents = 4 // Feature work can be parallel
	}

	return result
}

// countMatches counts how many patterns match the input.
func (a *RequestAnalyzer) countMatches(input string, patterns []*regexp.Regexp) int {
	count := 0
	for _, p := range patterns {
		if p.MatchString(input) {
			count++
		}
	}
	return count
}

// getMatchedKeywords extracts the matched keywords from input.
func (a *RequestAnalyzer) getMatchedKeywords(input string, patterns []*regexp.Regexp) []string {
	var keywords []string
	for _, p := range patterns {
		if matches := p.FindStringSubmatch(input); len(matches) > 0 {
			keywords = append(keywords, matches[0])
		}
	}
	return keywords
}

// analyzeByContent performs additional analysis when no patterns match.
// Note: This is called when no specific patterns matched, so we default to Feature type.
// We do NOT recommend quick mode here - quick mode is only for explicit SETUP/BUGFIX.
func (a *RequestAnalyzer) analyzeByContent(request string, result *RequestAnalysis) *RequestAnalysis {
	lower := strings.ToLower(request)

	// Check for multiple distinct items (suggests parallel work)
	itemSeparators := []string{",", " and ", "\n-", "\n*", "\n1.", "\n2."}
	itemCount := 1
	for _, sep := range itemSeparators {
		if strings.Contains(lower, sep) {
			itemCount = max(itemCount, strings.Count(lower, sep)+1)
		}
	}

	if itemCount >= 3 {
		result.Type = RequestTypeFeature
		result.MaxAgents = min(itemCount, 4)
	} else {
		// Default to feature with single agent - NOT quick mode
		// Quick mode is only for explicit SETUP or BUGFIX patterns
		result.Type = RequestTypeFeature
		result.MaxAgents = 1
		// DO NOT set RecommendQuickMode = true here
	}

	return result
}

// ShouldUseQuickMode returns whether quick mode should be used based on analysis.
func (a *RequestAnalyzer) ShouldUseQuickMode(analysis *RequestAnalysis) bool {
	// Always recommend quick mode for setup and bugfix
	if analysis.Type == RequestTypeSetup || analysis.Type == RequestTypeBugfix {
		return true
	}

	// Use explicit recommendation
	return analysis.RecommendQuickMode
}

// GetMaxAgents returns the recommended max agents based on analysis.
func (a *RequestAnalyzer) GetMaxAgents(analysis *RequestAnalysis) int {
	return analysis.MaxAgents
}
