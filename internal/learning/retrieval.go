// Package learning provides learning retrieval and ranking capabilities.
package learning

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

// RetrievalStore defines the interface for learning retrieval operations.
// This is implemented by *LearningStore in store.go.
type RetrievalStore interface {
	// Search searches learnings using full-text search.
	Search(query string) ([]*Learning, error)
	// SearchByScope searches learnings using full-text search filtered by scope(s).
	SearchByScope(query string, scopes []string) ([]*Learning, error)
	// List returns the most recent learnings up to the specified limit.
	List(limit int) ([]*Learning, error)
	// ListByScope returns the most recent learnings filtered by scope(s).
	ListByScope(scopes []string, limit int) ([]*Learning, error)
	// SearchByCondition searches learnings matching the given condition pattern.
	SearchByCondition(pattern string) ([]*Learning, error)
	// SearchByConditionAndScope searches learnings matching condition pattern filtered by scope(s).
	SearchByConditionAndScope(pattern string, scopes []string) ([]*Learning, error)
	// SearchByPath searches learnings by file path prefix.
	SearchByPath(pathPrefix string) ([]*Learning, error)
	// SearchByPathAndScope searches learnings by file path prefix filtered by scope(s).
	SearchByPathAndScope(pathPrefix string, scopes []string) ([]*Learning, error)
	// IncrementTriggerCount increments the trigger count and updates last triggered time.
	IncrementTriggerCount(id string) error
}

// Retriever queries and ranks learnings for relevance to tasks and errors.
type Retriever struct {
	store RetrievalStore
	// Fields used during ranking to enable BM25 scoring
	queryTerms []string
	avgDocLen  float64
	docFreqs   map[string]int
	totalDocs  int
}

// NewRetriever creates a new Retriever with the given store.
func NewRetriever(store RetrievalStore) *Retriever {
	return &Retriever{store: store}
}

// RetrieveForTask retrieves learnings relevant to a task (from all scopes).
// It extracts keywords from the task description, searches by keywords,
// ranks results by trigger count and recency, and returns
// the top 5 most relevant learnings.
func (r *Retriever) RetrieveForTask(taskDescription string, filePaths []string) ([]*Learning, error) {
	return r.RetrieveForTaskWithScope(taskDescription, filePaths, nil)
}

// RetrieveForTaskWithScope retrieves learnings relevant to a task, filtered by scope(s).
// If scopes is nil or empty, retrieves from all scopes.
// Pass multiple scopes like []string{"repo", "global"} to include learnings from any of those scopes.
func (r *Retriever) RetrieveForTaskWithScope(taskDescription string, filePaths []string, scopes []string) ([]*Learning, error) {
	if r.store == nil {
		return nil, nil
	}

	seen := make(map[string]*Learning)

	// Step 1: Extract keywords from task description
	keywords := r.extractKeywords(taskDescription)

	// Step 2: Search learnings by keywords (joined as query)
	if len(keywords) > 0 {
		// Join keywords with OR for FTS5 query
		query := strings.Join(keywords, " OR ")
		var results []*Learning
		var err error
		if len(scopes) > 0 {
			results, err = r.store.SearchByScope(query, scopes)
		} else {
			results, err = r.store.Search(query)
		}
		if err != nil {
			return nil, err
		}
		for _, l := range results {
			seen[l.ID] = l
		}
	}

	// Step 3: Search by path prefixes if file hints provided
	for _, path := range filePaths {
		prefix := extractPathPrefix(path)
		if prefix != "" {
			var results []*Learning
			var err error
			if len(scopes) > 0 {
				results, err = r.store.SearchByPathAndScope(prefix, scopes)
			} else {
				results, err = r.store.SearchByPath(prefix)
			}
			if err != nil {
				return nil, err
			}
			for _, l := range results {
				seen[l.ID] = l
			}
		}
	}

	// Convert map to slice
	learnings := make([]*Learning, 0, len(seen))
	for _, l := range seen {
		learnings = append(learnings, l)
	}

	// Step 4: Compute BM25 corpus stats and rank by relevance
	r.queryTerms = tokenize(strings.ToLower(taskDescription))
	r.avgDocLen, r.docFreqs = computeCorpusStats(learnings)
	r.totalDocs = len(learnings)
	r.rankLearnings(learnings)

	// Step 5: Return top 5 most relevant
	if len(learnings) > 5 {
		learnings = learnings[:5]
	}

	return learnings, nil
}

// RetrieveForError retrieves learnings relevant to an error message (from all scopes).
// It searches learnings by error message and matches against the Condition field.
func (r *Retriever) RetrieveForError(errorMessage string) ([]*Learning, error) {
	return r.RetrieveForErrorWithScope(errorMessage, nil)
}

// RetrieveForErrorWithScope retrieves learnings relevant to an error message, filtered by scope(s).
// If scopes is nil or empty, retrieves from all scopes.
func (r *Retriever) RetrieveForErrorWithScope(errorMessage string, scopes []string) ([]*Learning, error) {
	if r.store == nil {
		return nil, nil
	}

	// Step 1: Search learnings by error message keywords
	keywords := r.extractKeywords(errorMessage)
	var allResults []*Learning

	if len(keywords) > 0 {
		// Join keywords with OR for FTS5 query
		query := strings.Join(keywords, " OR ")
		var results []*Learning
		var err error
		if len(scopes) > 0 {
			results, err = r.store.SearchByScope(query, scopes)
		} else {
			results, err = r.store.Search(query)
		}
		if err != nil {
			return nil, err
		}
		allResults = append(allResults, results...)
	}

	// Step 2: Search by condition pattern
	var conditionResults []*Learning
	var err error
	if len(scopes) > 0 {
		conditionResults, err = r.store.SearchByConditionAndScope(errorMessage, scopes)
	} else {
		conditionResults, err = r.store.SearchByCondition(errorMessage)
	}
	if err != nil {
		return nil, err
	}

	// Deduplicate by ID
	seen := make(map[string]bool)
	merged := make([]*Learning, 0)
	for _, l := range allResults {
		if !seen[l.ID] {
			seen[l.ID] = true
			merged = append(merged, l)
		}
	}
	for _, l := range conditionResults {
		if !seen[l.ID] {
			seen[l.ID] = true
			merged = append(merged, l)
		}
	}

	// Compute BM25 corpus stats and rank by relevance
	r.queryTerms = tokenize(strings.ToLower(errorMessage))
	r.avgDocLen, r.docFreqs = computeCorpusStats(merged)
	r.totalDocs = len(merged)
	r.rankLearnings(merged)

	return merged, nil
}

// extractKeywords extracts meaningful keywords from text.
// It removes common stop words and returns unique, lowercase keywords.
func (r *Retriever) extractKeywords(text string) []string {
	if text == "" {
		return nil
	}

	// Common programming/English stop words to filter out
	stopWords := map[string]bool{
		"a": true, "an": true, "and": true, "are": true, "as": true,
		"at": true, "be": true, "by": true, "for": true, "from": true,
		"has": true, "have": true, "he": true, "in": true, "is": true,
		"it": true, "its": true, "of": true, "on": true, "or": true,
		"that": true, "the": true, "this": true, "to": true, "was": true,
		"will": true, "with": true, "not": true, "but": true, "you": true,
		"your": true, "can": true, "do": true, "does": true, "did": true,
		"should": true, "would": true, "could": true, "may": true, "might": true,
		"must": true, "shall": true, "need": true, "if": true, "then": true,
		"else": true, "when": true, "where": true, "which": true, "who": true,
		"whom": true, "what": true, "how": true, "why": true, "all": true,
		"any": true, "both": true, "each": true, "few": true, "more": true,
		"most": true, "other": true, "some": true, "such": true, "no": true,
		"nor": true, "only": true, "own": true, "same": true, "so": true,
		"than": true, "too": true, "very": true, "just": true, "also": true,
	}

	// Split on non-word characters
	wordPattern := regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9_]*`)
	words := wordPattern.FindAllString(text, -1)

	// Filter and deduplicate
	seen := make(map[string]bool)
	keywords := make([]string, 0)

	for _, word := range words {
		lower := strings.ToLower(word)
		// Skip short words and stop words
		if len(lower) < 3 || stopWords[lower] {
			continue
		}
		if !seen[lower] {
			seen[lower] = true
			keywords = append(keywords, lower)
		}
	}

	return keywords
}

// rankLearnings ranks learnings by trigger count * recency factor.
// More recently triggered learnings and those with higher trigger counts
// are ranked higher.
func (r *Retriever) rankLearnings(learnings []*Learning) {
	now := time.Now()

	sort.Slice(learnings, func(i, j int) bool {
		scoreI := r.calculateScore(learnings[i], now)
		scoreJ := r.calculateScore(learnings[j], now)
		return scoreI > scoreJ
	})
}

// calculateScore computes a combined relevance score for a learning.
// The score combines:
//   - BM25 semantic relevance (primary signal, weighted heavily)
//   - Trigger count (usage signal)
//   - Recency factor (time decay, 7-day half-life)
//   - Effectiveness factor (success rate, floor at 0.1 to allow recovery)
//
// Formula: (1 + BM25) * sqrt(1 + triggerCount) * recencyFactor * effectivenessFactor
// This ensures BM25 is the dominant ranking factor while still boosting
// frequently used, recently triggered, and effective learnings.
// Ineffective learnings are downranked but not eliminated (0.1 floor).
func (r *Retriever) calculateScore(l *Learning, now time.Time) float64 {
	// BM25 semantic relevance score
	bm25 := 0.0
	if r.totalDocs > 0 && len(r.queryTerms) > 0 {
		bm25 = bm25Score(l, r.queryTerms, r.avgDocLen, r.docFreqs, r.totalDocs)
	}

	// Trigger count signal (use sqrt to dampen effect of very high counts)
	triggerScore := math.Sqrt(1 + float64(l.TriggerCount))

	// Calculate recency factor with 7-day half-life
	daysSinceTrigger := now.Sub(l.LastTriggered).Hours() / 24
	if daysSinceTrigger < 0 {
		daysSinceTrigger = 0
	}

	// Decay formula: factor = 1 / (1 + days/halfLife)
	// This gives ~0.5 weight at 7 days, ~0.33 at 14 days, etc.
	halfLife := 7.0
	recencyFactor := 1.0
	if !l.LastTriggered.IsZero() {
		recencyFactor = 1.0 / (1.0 + daysSinceTrigger/halfLife)
	}

	// Effectiveness factor: success rate with 0.1 floor to allow recovery
	// New learnings (no uses yet) default to 1.0 effectiveness
	effectivenessFactor := math.Max(0.1, l.Effectiveness)

	// Combined score: BM25 is primary, modulated by usage, recency, and effectiveness
	// Adding 1 to BM25 ensures non-matching docs still get ranked by other factors
	return (1 + bm25) * triggerScore * recencyFactor * effectivenessFactor
}

// extractPathPrefix extracts a meaningful path prefix from a file path.
// For example, "internal/learning/store.go" returns "internal/learning".
func extractPathPrefix(path string) string {
	if path == "" {
		return ""
	}

	// Remove leading slashes
	path = strings.TrimPrefix(path, "/")

	// Split into parts
	parts := strings.Split(path, "/")

	// Return directory path (all but the last part if it looks like a file)
	if len(parts) <= 1 {
		return path
	}

	lastPart := parts[len(parts)-1]
	if strings.Contains(lastPart, ".") {
		// Likely a file, return directory
		return strings.Join(parts[:len(parts)-1], "/")
	}

	return path
}

// BM25 parameters - standard values from literature
const (
	bm25K1 = 1.2  // Term frequency saturation parameter
	bm25B  = 0.75 // Length normalization parameter
)

// bm25Score computes a BM25 relevance score for a learning against query terms.
// BM25 is a probabilistic ranking function used in information retrieval.
// Higher scores indicate greater relevance.
func bm25Score(learning *Learning, queryTerms []string, avgDocLen float64, docFreqs map[string]int, totalDocs int) float64 {
	if len(queryTerms) == 0 || totalDocs == 0 {
		return 0
	}

	// Combine condition, action, and outcome into document text
	docText := strings.ToLower(learning.Condition + " " + learning.Action + " " + learning.Outcome)
	docTerms := tokenize(docText)
	docLen := float64(len(docTerms))

	if docLen == 0 {
		return 0
	}

	// Count term frequencies in document
	termFreqs := make(map[string]int)
	for _, term := range docTerms {
		termFreqs[term]++
	}

	score := 0.0
	for _, term := range queryTerms {
		tf := float64(termFreqs[term])
		if tf == 0 {
			continue
		}

		// Document frequency (number of docs containing term)
		df := docFreqs[term]
		if df == 0 {
			df = 1 // Avoid division by zero
		}

		// IDF component: log((N - df + 0.5) / (df + 0.5) + 1)
		idf := math.Log((float64(totalDocs)-float64(df)+0.5)/(float64(df)+0.5) + 1)

		// TF component with length normalization
		lengthNorm := 1 - bm25B + bm25B*(docLen/avgDocLen)
		tfNorm := (tf * (bm25K1 + 1)) / (tf + bm25K1*lengthNorm)

		score += idf * tfNorm
	}

	return score
}

// tokenize splits text into lowercase tokens for BM25 scoring.
func tokenize(text string) []string {
	wordPattern := regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9_]*`)
	words := wordPattern.FindAllString(text, -1)
	tokens := make([]string, 0, len(words))
	for _, word := range words {
		lower := strings.ToLower(word)
		if len(lower) >= 2 { // Include 2+ char tokens for BM25
			tokens = append(tokens, lower)
		}
	}
	return tokens
}

// computeCorpusStats computes document frequencies and average document length.
func computeCorpusStats(learnings []*Learning) (avgDocLen float64, docFreqs map[string]int) {
	docFreqs = make(map[string]int)
	totalLen := 0

	for _, l := range learnings {
		docText := strings.ToLower(l.Condition + " " + l.Action + " " + l.Outcome)
		tokens := tokenize(docText)
		totalLen += len(tokens)

		// Track which terms appear in this document (for DF calculation)
		seen := make(map[string]bool)
		for _, token := range tokens {
			if !seen[token] {
				seen[token] = true
				docFreqs[token]++
			}
		}
	}

	if len(learnings) > 0 {
		avgDocLen = float64(totalLen) / float64(len(learnings))
	}
	return avgDocLen, docFreqs
}
