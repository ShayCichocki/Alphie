package orchestrator

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"sync"

	"github.com/ShayCichocki/alphie/internal/graph"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// Scheduler coordinates the scheduling of ready tasks to available agent slots.
// It respects the tier's max_agents limit and uses collision detection to avoid
// concurrent modifications to the same files or directories.
type Scheduler struct {
	// graph is the dependency graph of tasks.
	graph *graph.DependencyGraph
	// tier is the agent tier for this scheduler.
	tier models.Tier
	// running maps agent IDs to their agent instances.
	running map[string]*models.Agent
	// maxAgents is the maximum number of concurrent agents allowed.
	maxAgents int
	// collision is the collision checker for avoiding file conflicts.
	collision *CollisionChecker
	// greenfield indicates whether this is a greenfield project.
	// In greenfield mode, tasks that might touch root files are serialized.
	greenfield bool
	// orchestrator is a reference to the parent orchestrator for conflict checking.
	orchestrator *Orchestrator
	// trigger is a channel to signal the scheduler to check for work.
	trigger chan struct{}
	// mu protects all mutable fields.
	mu sync.RWMutex
}

// NewScheduler creates a new Scheduler with the given dependency graph, tier, and max agents limit.
func NewScheduler(graph *graph.DependencyGraph, tier models.Tier, maxAgents int) *Scheduler {
	return &Scheduler{
		graph:     graph,
		tier:      tier,
		running:   make(map[string]*models.Agent),
		maxAgents: maxAgents,
		trigger:   make(chan struct{}, 1),
	}
}

// SetOrchestrator sets a reference to the parent orchestrator for merge conflict checking.
func (s *Scheduler) SetOrchestrator(o *Orchestrator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orchestrator = o
}

// SetCollisionChecker sets the collision checker for this scheduler.
// If not set, collision checking is disabled.
func (s *Scheduler) SetCollisionChecker(cc *CollisionChecker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.collision = cc
}

// SetGreenfield enables greenfield mode, which serializes tasks that might touch root files.
func (s *Scheduler) SetGreenfield(greenfield bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.greenfield = greenfield
}

// Schedule returns a slice of tasks that are ready to be scheduled.
// It considers:
// - Tasks with no unmet dependencies (from the graph)
// - Available agent slots (maxAgents - running count)
// - Collision avoidance rules (if a collision checker is set)
// - Merge conflict blocking (if orchestrator has active conflict)
func (s *Scheduler) Schedule() []*models.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// BLOCK ALL SCHEDULING if there's an active merge conflict
	if s.orchestrator != nil && s.orchestrator.HasMergeConflict() {
		debugLog("[scheduler] BLOCKED: merge conflict active - no tasks scheduled")
		return nil
	}

	// Calculate available slots.
	availableSlots := s.maxAgents - len(s.running)
	if availableSlots <= 0 {
		debugLog("[scheduler] no available slots: maxAgents=%d, running=%d", s.maxAgents, len(s.running))
		return nil
	}

	// Get ready task IDs from the dependency graph.
	readyIDs := s.graph.GetReady()
	debugLog("[scheduler] graph.GetReady() returned %d tasks: %v", len(readyIDs), readyIDs)
	if len(readyIDs) == 0 {
		// Log graph state for debugging
		debugLog("[scheduler] graph has %d total tasks, completed: %v", s.graph.Size(), s.graph.GetCompletedIDs())
		return nil
	}

	// Filter out tasks that are already being worked on.
	var candidates []*models.Task
	for _, id := range readyIDs {
		// Check if this task is already assigned to a running agent.
		alreadyRunning := false
		for _, agent := range s.running {
			if agent.TaskID == id {
				alreadyRunning = true
				break
			}
		}
		if alreadyRunning {
			continue
		}

		task := s.graph.GetTask(id)
		if task != nil {
			candidates = append(candidates, task)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Get running agents for collision and SETUP checks.
	runningAgents := s.getRunningAgentsLocked()

	// Layer 1: SETUP tasks must be sequential.
	// Check if any running agent is working on a SETUP task.
	setupRunning := false
	for _, agent := range runningAgents {
		task := s.graph.GetTask(agent.TaskID)
		if task != nil && task.TaskType == models.TaskTypeSetup {
			setupRunning = true
			debugLog("[scheduler] SETUP task %s is running, will serialize other SETUP tasks", task.ID)
			break
		}
	}

	// Filter by SETUP serialization, critical file conflicts, greenfield root-touching, and collision avoidance.
	var schedulable []*models.Task

	// Track root-touching tasks we're scheduling in this batch (for greenfield serialization)
	schedulingRootTouching := false
	// Track critical files being touched by tasks in this batch (for all modes)
	schedulingCriticalFiles := make(map[string]bool)
	// Track skip reasons for logging
	skipReasons := make(map[string]string)

	for _, task := range candidates {
		// Layer 1: Skip SETUP tasks if one is already running.
		if task.TaskType == models.TaskTypeSetup && setupRunning {
			debugLog("[scheduler] Layer 1: Skipping SETUP task %s (%s) - another SETUP is running", task.ID, task.Title)
			skipReasons[task.ID] = "Layer 1: SETUP serialization"
			continue
		}

		// Layer 2: Critical file conflict check (applies in ALL modes).
		// Tasks touching the same critical config files (package.json, go.mod, etc.) must be serialized.
		if s.collision != nil {
			// Check against running agents
			if s.collision.HasCriticalFileConflict(task, runningAgents, s.graph) {
				criticalFiles := s.collision.GetCriticalFileBoundaries(task)
				debugLog("[scheduler] Layer 2: Skipping task %s (%s) - critical file conflict: %v", task.ID, task.Title, criticalFiles)
				skipReasons[task.ID] = fmt.Sprintf("Layer 2: Critical file conflict on %v", criticalFiles)
				continue
			}

			// Check against tasks we're scheduling in THIS batch
			taskCritical := s.collision.GetCriticalFileBoundaries(task)
			conflictInBatch := false
			var conflictingFile string
			for _, f := range taskCritical {
				if schedulingCriticalFiles[f] {
					debugLog("[scheduler] Layer 2: Skipping task %s (%s) - critical file %s already claimed in this batch", task.ID, task.Title, f)
					conflictingFile = f
					conflictInBatch = true
					break
				}
			}
			if conflictInBatch {
				skipReasons[task.ID] = fmt.Sprintf("Layer 2: Critical file %s claimed in batch", conflictingFile)
				continue
			}
			// Mark these critical files as claimed
			for _, f := range taskCritical {
				schedulingCriticalFiles[f] = true
			}
		}

		// Layer 3: In greenfield mode, also serialize tasks that might touch root files.
		// This is broader than critical file check - includes any root-level files.
		if s.greenfield && s.collision != nil {
			taskTouchesRoot := s.collision.MightTouchRoot(task)

			// Check against running agents
			if s.collision.HasRootTouchingConflict(task, runningAgents, s.graph) {
				debugLog("[scheduler] Layer 3: Skipping task %s (%s) - greenfield root-touching conflict with running agent", task.ID, task.Title)
				skipReasons[task.ID] = "Layer 3: Greenfield root conflict with running agent"
				continue
			}

			// Check against tasks we're scheduling in THIS batch
			// This prevents scheduling multiple root-touching tasks at once
			if taskTouchesRoot && schedulingRootTouching {
				debugLog("[scheduler] Layer 3: Skipping task %s (%s) - greenfield: already scheduling another root-touching task", task.ID, task.Title)
				skipReasons[task.ID] = "Layer 3: Greenfield root conflict in batch"
				continue
			}

			if taskTouchesRoot {
				schedulingRootTouching = true
			}
		}

		// Layer 4: General collision avoidance check.
		if s.collision != nil {
			if !s.collision.CanSchedule(task, runningAgents) {
				debugLog("[scheduler] Layer 4: Skipping task %s (%s) - general file boundary overlap", task.ID, task.Title)
				skipReasons[task.ID] = "Layer 4: File boundary overlap with running agents"
				continue
			}
		}

		schedulable = append(schedulable, task)
	}

	// Log scheduling summary
	if len(skipReasons) > 0 {
		debugLog("[scheduler] Scheduling summary: %d tasks skipped due to collision detection", len(skipReasons))
		for taskID, reason := range skipReasons {
			debugLog("[scheduler]   - Task %s: %s", taskID, reason)
		}
	}
	debugLog("[scheduler] Scheduled %d tasks for execution (max parallelism: %d)", len(schedulable), availableSlots)

	sort.SliceStable(schedulable, func(i, j int) bool {
		return extractMilestoneNumber(schedulable[i]) < extractMilestoneNumber(schedulable[j])
	})

	if len(schedulable) > 0 {
		debugLog("[scheduler] Sorted %d tasks by milestone:", len(schedulable))
		for _, task := range schedulable {
			milestone := extractMilestoneNumber(task)
			if milestone == math.MaxInt {
				debugLog("[scheduler]   - %s (%s) [no milestone]", task.ID, task.Title)
			} else {
				debugLog("[scheduler]   - %s (%s) [M%d]", task.ID, task.Title, milestone)
			}
		}
	}

	if len(schedulable) > availableSlots {
		schedulable = schedulable[:availableSlots]
	}

	return schedulable
}

// OnAgentStart records that an agent has started working on a task.
func (s *Scheduler) OnAgentStart(agent *models.Agent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	debugLog("[scheduler.OnAgentStart] registering agent %s for task %s", agent.ID, agent.TaskID)
	s.running[agent.ID] = agent
	debugLog("[scheduler.OnAgentStart] running map now has %d agents", len(s.running))
}

// OnAgentComplete handles the completion of an agent.
// It removes the agent from the running map.
// If success is true, marks the task complete in the graph (unblocking dependents).
// If success is false, the task remains incomplete and dependents stay blocked.
func (s *Scheduler) OnAgentComplete(agentID string, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	debugLog("[scheduler.OnAgentComplete] looking up agent %s in running map (has %d agents), success=%v", agentID, len(s.running), success)
	agent, ok := s.running[agentID]
	if !ok {
		debugLog("[scheduler.OnAgentComplete] agent %s NOT FOUND in running map", agentID)
		return
	}

	// Only mark the task complete if it succeeded.
	// Failed tasks should NOT unblock their dependents.
	if success {
		debugLog("[scheduler.OnAgentComplete] agent %s was working on task %s, marking complete", agentID, agent.TaskID)
		s.graph.MarkComplete(agent.TaskID)
	} else {
		debugLog("[scheduler.OnAgentComplete] agent %s FAILED task %s, NOT marking complete (dependents remain blocked)", agentID, agent.TaskID)
		// Mark dependent tasks as explicitly blocked with reason
		s.markDependentsBlocked(agent.TaskID)
	}

	// Remove from running agents.
	delete(s.running, agentID)
	debugLog("[scheduler.OnAgentComplete] removed agent %s from running map, now have %d agents", agentID, len(s.running))
}

// GetRunningAgents returns a slice of all currently running agents.
func (s *Scheduler) GetRunningAgents() []*models.Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getRunningAgentsLocked()
}

// getRunningAgentsLocked returns running agents without acquiring the lock.
// Caller must hold s.mu.
func (s *Scheduler) getRunningAgentsLocked() []*models.Agent {
	agents := make([]*models.Agent, 0, len(s.running))
	for _, agent := range s.running {
		agents = append(agents, agent)
	}
	return agents
}

// GetRunningCount returns the number of currently running agents.
func (s *Scheduler) GetRunningCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.running)
}

func extractMilestoneNumber(task *models.Task) int {
	re := regexp.MustCompile(`\bM(\d+)\b`)
	matches := re.FindStringSubmatch(task.Title)
	if len(matches) > 1 {
		num, err := strconv.Atoi(matches[1])
		if err == nil {
			return num
		}
	}
	return math.MaxInt
}

// markDependentsBlocked marks all tasks that depend on the failed task as blocked.
// This provides clear visibility that dependent tasks cannot proceed.
func (s *Scheduler) markDependentsBlocked(failedTaskID string) {
	dependentIDs := s.graph.GetDependents(failedTaskID)
	for _, depID := range dependentIDs {
		task := s.graph.GetTask(depID)
		if task != nil && task.Status == models.TaskStatusPending {
			task.Status = models.TaskStatusBlocked
			task.BlockedReason = "dependency_failed:" + failedTaskID
			debugLog("[scheduler] marked task %s as blocked (depends on failed task %s)", depID, failedTaskID)
		}
	}
}
