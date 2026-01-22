// Package decompose provides task decomposition for user requests.
package decompose

import (
	"fmt"
	"strings"

	"github.com/ShayCichocki/alphie/internal/merge"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// CoalesceSetupTasks merges SETUP tasks that have overlapping critical files.
// This prevents merge conflicts when multiple setup tasks try to modify the same
// config files (package.json, tsconfig.json, etc.).
//
// The algorithm:
// 1. Separate SETUP tasks from non-SETUP tasks
// 2. Group SETUP tasks that share critical files
// 3. Merge each group into a single consolidated task
// 4. Update dependencies to point to the merged task
func CoalesceSetupTasks(tasks []*models.Task) []*models.Task {
	if len(tasks) <= 1 {
		return tasks
	}

	// Separate SETUP from other tasks
	var setupTasks []*models.Task
	var otherTasks []*models.Task

	for _, task := range tasks {
		if task.TaskType == models.TaskTypeSetup {
			setupTasks = append(setupTasks, task)
		} else {
			otherTasks = append(otherTasks, task)
		}
	}

	// If 0 or 1 SETUP tasks, nothing to coalesce
	if len(setupTasks) <= 1 {
		return tasks
	}

	// Build groups of SETUP tasks that share critical files
	groups := groupByOverlappingCritical(setupTasks)

	// Merge each group into a single task
	var mergedSetup []*models.Task
	idMapping := make(map[string]string) // old ID -> new ID

	for _, group := range groups {
		if len(group) == 1 {
			// No merging needed
			mergedSetup = append(mergedSetup, group[0])
			continue
		}

		merged := mergeSetupGroup(group)
		mergedSetup = append(mergedSetup, merged)

		// Track ID mappings for dependency updates
		for _, original := range group {
			idMapping[original.ID] = merged.ID
		}
	}

	// Combine results
	result := append(mergedSetup, otherTasks...)

	// Update dependencies: if a task depends on a merged task, point to the merged ID
	for _, task := range result {
		task.DependsOn = updateDependencies(task.DependsOn, idMapping)
	}

	return result
}

// groupByOverlappingCritical groups tasks that have overlapping critical files.
// Uses union-find style grouping.
func groupByOverlappingCritical(tasks []*models.Task) [][]*models.Task {
	n := len(tasks)
	if n == 0 {
		return nil
	}

	// parent[i] points to the representative of i's group
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}

	// Find with path compression
	var find func(i int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}

	// Union
	union := func(i, j int) {
		pi, pj := find(i), find(j)
		if pi != pj {
			parent[pi] = pj
		}
	}

	// Check all pairs for critical file overlap
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if merge.HasCriticalFileOverlap(tasks[i].FileBoundaries, tasks[j].FileBoundaries) {
				union(i, j)
			}
		}
	}

	// Build groups
	groupMap := make(map[int][]*models.Task)
	for i, task := range tasks {
		root := find(i)
		groupMap[root] = append(groupMap[root], task)
	}

	var groups [][]*models.Task
	for _, group := range groupMap {
		groups = append(groups, group)
	}

	return groups
}

// mergeSetupGroup combines multiple SETUP tasks into one.
func mergeSetupGroup(group []*models.Task) *models.Task {
	if len(group) == 0 {
		return nil
	}
	if len(group) == 1 {
		return group[0]
	}

	// Use first task as base
	base := group[0]

	// Combine titles
	var titles []string
	for _, t := range group {
		titles = append(titles, t.Title)
	}

	// Combine descriptions
	var descriptions []string
	for i, t := range group {
		descriptions = append(descriptions, fmt.Sprintf("## Part %d: %s\n%s", i+1, t.Title, t.Description))
	}

	// Combine file boundaries (deduplicate)
	boundarySet := make(map[string]bool)
	for _, t := range group {
		for _, b := range t.FileBoundaries {
			boundarySet[b] = true
		}
	}
	var allBoundaries []string
	for b := range boundarySet {
		allBoundaries = append(allBoundaries, b)
	}

	// Combine acceptance criteria
	var criteria []string
	for _, t := range group {
		if t.AcceptanceCriteria != "" {
			criteria = append(criteria, fmt.Sprintf("- %s: %s", t.Title, t.AcceptanceCriteria))
		}
	}

	// Combine verification intents
	var verifications []string
	for _, t := range group {
		if t.VerificationIntent != "" {
			verifications = append(verifications, t.VerificationIntent)
		}
	}

	// Combine dependencies (from all tasks, excluding merged task IDs)
	mergedIDs := make(map[string]bool)
	for _, t := range group {
		mergedIDs[t.ID] = true
	}
	depSet := make(map[string]bool)
	for _, t := range group {
		for _, dep := range t.DependsOn {
			if !mergedIDs[dep] {
				depSet[dep] = true
			}
		}
	}
	var allDeps []string
	for dep := range depSet {
		allDeps = append(allDeps, dep)
	}

	return &models.Task{
		ID:                 base.ID, // Keep first task's ID as the representative
		Title:              "Project Setup: " + strings.Join(titles, " + "),
		Description:        strings.Join(descriptions, "\n\n"),
		TaskType:           models.TaskTypeSetup,
		FileBoundaries:     allBoundaries,
		AcceptanceCriteria: strings.Join(criteria, "\n"),
		VerificationIntent: strings.Join(verifications, "; "),
		DependsOn:          allDeps,
		Status:             models.TaskStatusPending,
		CreatedAt:          base.CreatedAt,
	}
}

// updateDependencies replaces old task IDs with new merged IDs.
func updateDependencies(deps []string, mapping map[string]string) []string {
	if len(mapping) == 0 {
		return deps
	}

	seen := make(map[string]bool)
	var updated []string

	for _, dep := range deps {
		newDep := dep
		if mapped, ok := mapping[dep]; ok {
			newDep = mapped
		}
		if !seen[newDep] {
			seen[newDep] = true
			updated = append(updated, newDep)
		}
	}

	return updated
}
