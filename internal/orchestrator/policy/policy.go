// Package policy defines configurable policy parameters for orchestrator behavior.
// This centralizes magic numbers and threshold values that were previously scattered
// across implementation files, enabling configuration and testing.
package policy

import "time"

// Config contains all configurable policy parameters for the orchestrator.
// These values control scheduling, collision detection, and review triggers.
type Config struct {
	// Scheduling policies
	Scheduling SchedulingPolicy

	// Collision detection policies
	Collision CollisionPolicy

	// Second review trigger policies
	Review ReviewPolicy

	// Override gate policies
	Override OverridePolicy

	// Loop policies
	Loop LoopPolicy

	// Merge policies
	Merge MergePolicy
}

// SchedulingPolicy controls task scheduling behavior.
type SchedulingPolicy struct {
	// RootTouchingPatterns are keywords indicating a task might modify root-level files.
	// Used in greenfield mode to serialize tasks that might conflict on package.json, etc.
	RootTouchingPatterns []string
}

// CollisionPolicy controls file collision detection.
type CollisionPolicy struct {
	// HotspotThreshold is the number of file touches before a file is considered a hotspot.
	// Hotspot files require serialized access.
	HotspotThreshold int

	// MaxAgentsPerTopLevel is the maximum concurrent agents allowed in the same top-level directory.
	MaxAgentsPerTopLevel int
}

// ReviewPolicy controls when second reviews are triggered.
type ReviewPolicy struct {
	// LargeDiffThreshold is the line count at which a diff is considered large.
	LargeDiffThreshold int

	// CrossCuttingThreshold is the package count at which changes are cross-cutting.
	CrossCuttingThreshold int
}

// OverridePolicy controls Scout question override behavior.
type OverridePolicy struct {
	// BlockedAfterNAttempts is the number of failed attempts before allowing Scout questions.
	BlockedAfterNAttempts int

	// ProtectedAreaDetected enables questions when protected areas are detected.
	ProtectedAreaDetected bool
}

// LoopPolicy controls run loop behavior.
type LoopPolicy struct {
	// PollInterval is the delay between schedule checks when no tasks are ready.
	PollInterval time.Duration

	// SpawnStagger is the delay between spawning parallel agents to avoid CLI contention.
	SpawnStagger time.Duration
}

// MergePolicy controls merge queue behavior.
type MergePolicy struct {
	// QueueBufferSize is the buffer size for the merge queue channel.
	QueueBufferSize int
}

// Default returns the default policy configuration.
func Default() *Config {
	return &Config{
		Scheduling: SchedulingPolicy{
			RootTouchingPatterns: []string{
				"package.json", "tsconfig", "eslint", "prettier", "workspace",
				"npm init", "npm install", "yarn", "pnpm",
				"dependencies", "devDependencies",
				"root", "project structure", "initialize", "setup",
				"monorepo", "workspaces",
			},
		},
		Collision: CollisionPolicy{
			HotspotThreshold:     3,
			MaxAgentsPerTopLevel: 2,
		},
		Review: ReviewPolicy{
			LargeDiffThreshold:    200,
			CrossCuttingThreshold: 3,
		},
		Override: OverridePolicy{
			BlockedAfterNAttempts: 5,
			ProtectedAreaDetected: true,
		},
		Loop: LoopPolicy{
			PollInterval: 100 * time.Millisecond,
			SpawnStagger: 2 * time.Second,
		},
		Merge: MergePolicy{
			QueueBufferSize: 100,
		},
	}
}

// Validate checks that policy values are within acceptable ranges.
func (c *Config) Validate() error {
	if c.Collision.HotspotThreshold < 1 {
		c.Collision.HotspotThreshold = 3
	}
	if c.Collision.MaxAgentsPerTopLevel < 1 {
		c.Collision.MaxAgentsPerTopLevel = 2
	}
	if c.Review.LargeDiffThreshold < 10 {
		c.Review.LargeDiffThreshold = 200
	}
	if c.Review.CrossCuttingThreshold < 1 {
		c.Review.CrossCuttingThreshold = 3
	}
	if c.Override.BlockedAfterNAttempts < 1 {
		c.Override.BlockedAfterNAttempts = 5
	}
	if c.Loop.PollInterval < 10*time.Millisecond {
		c.Loop.PollInterval = 100 * time.Millisecond
	}
	if c.Loop.SpawnStagger < 100*time.Millisecond {
		c.Loop.SpawnStagger = 2 * time.Second
	}
	if c.Merge.QueueBufferSize < 1 {
		c.Merge.QueueBufferSize = 100
	}
	return nil
}
