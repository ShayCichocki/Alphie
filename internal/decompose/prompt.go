package decompose

// decompositionPrompt is the prompt template for task decomposition.
const decompositionPrompt = `Break this user request into parallelizable subtasks. Each task should be sized for a single agent to complete.

User request:
%s

Return ONLY a JSON array of tasks with this exact structure (no other text):
[
  {
    "title": "Short task title",
    "description": "Detailed task description",
    "task_type": "SETUP|FEATURE|BUGFIX|REFACTOR",
    "file_boundaries": ["src/auth/", "server/routes/api.ts"],
    "depends_on": ["title of dependency 1", "title of dependency 2"],
    "acceptance_criteria": "Criteria to verify this task is complete",
    "verification_intent": "Concrete verification: what commands/tests prove this works"
  }
]

CRITICAL: File Boundary Rules
- file_boundaries MUST list ALL files/directories this task will modify
- Two tasks with overlapping file_boundaries will be SERIALIZED (run one after another)
- Tasks with NO overlap in file_boundaries can run in PARALLEL
- Be specific: "src/auth/login.ts" not just "src/"
- Include config files that will be touched: package.json, tsconfig.json, etc.
- If a task touches a shared config file, it should likely be the ONLY task or run first

Task Type Classification:
- SETUP: Project scaffolding, configuration, initialization (break into focused, atomic tasks)
- FEATURE: New functionality implementation (can be parallelized if boundaries don't overlap)
- BUGFIX: Fixing existing issues (usually single task)
- REFACTOR: Code restructuring without behavior change

Verification Intent Guidelines:
- verification_intent should describe HOW to verify the task was completed correctly
- Focus on observable outcomes: "tests pass", "endpoint returns 200", "file exists"
- Be specific: "go test ./internal/auth/..." not just "tests pass"
- Include the expected behavior or output where applicable
- This will be used to generate executable verification commands BEFORE implementation

Examples of good verification_intent:
- "Run 'go test ./internal/auth/...' - all tests pass, no new failures"
- "File src/config.yaml exists and contains 'database:' section"
- "Endpoint GET /api/users returns 200 with JSON array"
- "No lint errors in modified files: 'golangci-lint run ./internal/...'"

Guidelines:
- Tasks should be as independent as possible to allow parallel execution
- Only add dependencies when truly necessary (task A must complete before task B)
- Each task should be completable by a single agent in one session (aim for focused, atomic work)
- Acceptance criteria should be specific and verifiable
- Use empty array [] for depends_on if there are no dependencies
- Break work into focused tasks - avoid bundling multiple concerns into one task
- It's OK to have tasks that share files if they're sequential (use depends_on to order them)

⚠️ CRITICAL: FULL IMPLEMENTATIONS REQUIRED ⚠️
- Each task description MUST specify COMPLETE, WORKING implementations
- DO NOT create tasks that produce scaffolding, stubs, or placeholder code
- EVERY function, endpoint, component must be FULLY FUNCTIONAL
- Bad: "Create user authentication endpoints" (too vague - leads to stubs)
- Good: "Implement complete user authentication with bcrypt password hashing, JWT session tokens, and working signup/login/logout endpoints that persist to database"
- Bad: "Set up database layer" (leads to empty function stubs)
- Good: "Implement complete database layer with connection pooling, all CRUD operations for users/boards/cards, transaction support, and error handling"

Examples of FULL implementation task descriptions:
- "Implement complete React board component with drag-and-drop using react-beautiful-dnd, state management via React Context, real-time updates, and proper error boundaries"
- "Build complete REST API with: POST /auth/register (bcrypt hashing, validation), POST /auth/login (JWT generation), GET /auth/me (token verification), all with proper error responses and status codes"
- "Create full database migration system with: up/down migrations, version tracking table, rollback support, and migrations for all entities (users, boards, lists, cards) with proper indexes and foreign keys"`
