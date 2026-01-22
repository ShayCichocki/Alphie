package main

import (
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/ShayCichocki/alphie/internal/learning"
)

var (
	learnSearchQuery  string
	learnDeleteID     string
	learnConcept      string
)

var learnCmd = &cobra.Command{
	Use:   "learn [CAO triple | show <id>]",
	Short: "Manage learnings in the CAO format",
	Long: `Manage learnings stored as CAO (Condition-Action-Outcome) triples.

A learning captures actionable knowledge in the format:
  WHEN <condition> DO <action> RESULT <outcome>

Usage:
  alphie learn                           # List recent learnings
  alphie learn "WHEN X DO Y RESULT Z"    # Add a new learning
  alphie learn --search "query"          # Search learnings
  alphie learn --concept build           # List by concept
  alphie learn show <id>                 # Show learning details
  alphie learn --delete <id>             # Delete a learning

Examples:
  alphie learn "WHEN tests fail with timeout DO increase test timeout RESULT tests pass"
  alphie learn --search "timeout"
  alphie learn show lr-abc123`,
	Args: cobra.MaximumNArgs(2),
	RunE: runLearn,
}

func init() {
	learnCmd.Flags().StringVarP(&learnSearchQuery, "search", "s", "", "Search learnings by query")
	learnCmd.Flags().StringVarP(&learnDeleteID, "delete", "d", "", "Delete learning by ID")
	learnCmd.Flags().StringVarP(&learnConcept, "concept", "c", "", "Filter learnings by concept")
}

func runLearn(cmd *cobra.Command, args []string) error {
	// Initialize the learning store
	store, err := learning.NewLearningStore(learning.GlobalDBPath())
	if err != nil {
		return fmt.Errorf("failed to open learning store: %w", err)
	}
	defer store.Close()

	// Run migrations to ensure schema exists
	if err := store.Migrate(); err != nil {
		return fmt.Errorf("failed to migrate learning store: %w", err)
	}

	// Handle --delete flag
	if learnDeleteID != "" {
		return deleteLearning(store, learnDeleteID)
	}

	// Handle --search flag
	if learnSearchQuery != "" {
		return searchLearnings(store, learnSearchQuery)
	}

	// Handle --concept flag
	if learnConcept != "" {
		return listByConcept(store, learnConcept)
	}

	// Handle subcommand: show <id>
	if len(args) >= 1 && args[0] == "show" {
		if len(args) < 2 {
			return fmt.Errorf("usage: alphie learn show <id>")
		}
		return showLearning(store, args[1])
	}

	// Handle positional arg: add a new learning
	if len(args) == 1 {
		return addLearning(store, args[0])
	}

	// No args: list recent learnings
	return listRecentLearnings(store)
}

// addLearning parses and adds a new CAO learning
func addLearning(store *learning.LearningStore, input string) error {
	cao, err := learning.ParseCAO(input)
	if err != nil {
		return fmt.Errorf("invalid CAO format: %w\n\nExpected: WHEN <condition> DO <action> RESULT <outcome>", err)
	}

	id := fmt.Sprintf("lr-%s", uuid.New().String()[:8])
	lr := &learning.Learning{
		ID:          id,
		Condition:   cao.Condition,
		Action:      cao.Action,
		Outcome:     cao.Outcome,
		Scope:       "repo",
		OutcomeType: "neutral",
		CreatedAt:   time.Now(),
	}

	if err := store.Create(lr); err != nil {
		return fmt.Errorf("failed to save learning: %w", err)
	}

	fmt.Printf("Learning added: %s\n", id)
	printLearning(lr)
	return nil
}

// searchLearnings searches for learnings matching the query
func searchLearnings(store *learning.LearningStore, query string) error {
	results, err := store.Search(query)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No learnings found matching query.")
		return nil
	}

	fmt.Printf("Found %d learning(s):\n\n", len(results))
	for _, lr := range results {
		printLearningCompact(lr)
	}
	return nil
}

// listByConcept lists learnings tagged with a specific concept
func listByConcept(store *learning.LearningStore, concept string) error {
	// For now, search by concept name as a keyword
	// Full concept support would require querying the learning_concepts table
	results, err := store.Search(concept)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		fmt.Printf("No learnings found for concept: %s\n", concept)
		return nil
	}

	fmt.Printf("Learnings related to '%s':\n\n", concept)
	for _, lr := range results {
		printLearningCompact(lr)
	}
	return nil
}

// listRecentLearnings lists the most recent learnings
func listRecentLearnings(store *learning.LearningStore) error {
	results, err := store.List(10)
	if err != nil {
		return fmt.Errorf("failed to list learnings: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No learnings stored yet.")
		fmt.Println("\nAdd a learning with:")
		fmt.Println("  alphie learn \"WHEN <condition> DO <action> RESULT <outcome>\"")
		return nil
	}

	fmt.Printf("Recent learnings (%d):\n\n", len(results))
	for _, lr := range results {
		printLearningCompact(lr)
	}
	return nil
}

// showLearning displays detailed information about a specific learning
func showLearning(store *learning.LearningStore, id string) error {
	lr, err := store.Get(id)
	if err != nil {
		return fmt.Errorf("failed to get learning: %w", err)
	}

	if lr == nil {
		return fmt.Errorf("learning not found: %s", id)
	}

	printLearningDetailed(lr)
	return nil
}

// deleteLearning removes a learning by ID
func deleteLearning(store *learning.LearningStore, id string) error {
	// First check if learning exists
	lr, err := store.Get(id)
	if err != nil {
		return fmt.Errorf("failed to check learning: %w", err)
	}
	if lr == nil {
		return fmt.Errorf("learning not found: %s", id)
	}

	if err := store.Delete(id); err != nil {
		return fmt.Errorf("failed to delete learning: %w", err)
	}

	fmt.Printf("Deleted learning: %s\n", id)
	return nil
}

// printLearning prints a learning in the standard format
func printLearning(lr *learning.Learning) {
	fmt.Printf("[%s] WHEN: %s\n", lr.ID, lr.Condition)
	fmt.Printf("         DO: %s\n", lr.Action)
	fmt.Printf("         RESULT: %s\n", lr.Outcome)
	fmt.Printf("         Triggers: %d\n", lr.TriggerCount)
}

// printLearningCompact prints a compact learning summary
func printLearningCompact(lr *learning.Learning) {
	fmt.Printf("[%s] WHEN: %s\n", lr.ID, truncate(lr.Condition, 60))
	fmt.Printf("         DO: %s\n", truncate(lr.Action, 60))
	fmt.Printf("         RESULT: %s\n", truncate(lr.Outcome, 60))
	fmt.Printf("         Triggers: %d\n\n", lr.TriggerCount)
}

// printLearningDetailed prints full details about a learning
func printLearningDetailed(lr *learning.Learning) {
	fmt.Printf("ID:           %s\n", lr.ID)
	fmt.Printf("Scope:        %s\n", lr.Scope)
	fmt.Printf("Outcome Type: %s\n", lr.OutcomeType)
	fmt.Printf("Created:      %s\n", lr.CreatedAt.Format(time.RFC3339))
	if !lr.LastTriggered.IsZero() {
		fmt.Printf("Last Triggered: %s\n", lr.LastTriggered.Format(time.RFC3339))
	}
	fmt.Printf("Triggers:     %d\n", lr.TriggerCount)
	if lr.CommitHash != "" {
		fmt.Printf("Commit:       %s\n", lr.CommitHash)
	}
	fmt.Println()
	fmt.Printf("WHEN:   %s\n", lr.Condition)
	fmt.Printf("DO:     %s\n", lr.Action)
	fmt.Printf("RESULT: %s\n", lr.Outcome)
}

// truncate shortens a string to max length, adding ellipsis if needed
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// Ensure we have os imported for potential future use
var _ = os.Exit
