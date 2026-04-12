---
ddx:
  id: API-002
  depends_on:
    - API-001
    - FEAT-008
    - SD-019
---
# API/Interface Contract: Epic Worker Surfaces

## Purpose

Define the server API, worker model, and UI surfaces for epic-scoped execution.
Epic workers differ from single-ticket workers: they own a persistent branch
and worktree, execute child beads sequentially, and land via a merge commit
rather than individual rebase+ff operations.

This contract extends the single-ticket supervisor (API-001) without modifying
it. The two worker modes coexist on the same server.

## Epic Worker Model

An epic worker is a long-lived worker that:

1. Claims one open epic bead
2. Creates a persistent epic branch: `epic/<bead-id>`
3. Creates a persistent worktree at `.ddx/.epic-wt-<bead-id>/`
4. Sequentially executes each execution-eligible child bead inside the epic worktree
5. After each child completes, commits the result on the epic branch
6. When all children are done (or the epic is explicitly finalized), merges the epic branch to the target branch with a regular merge commit
7. Cleans up the epic worktree and branch

### State Machine

```
CLAIMED → BRANCHING → EXECUTING_CHILDREN → MERGING → DONE
                ↑              │
                └── RETRY_CHILD (on child failure, continue with next)
```

- **CLAIMED**: Epic bead claimed, no branch yet
- **BRANCHING**: Creating epic branch and worktree
- **EXECUTING_CHILDREN**: Running child beads one at a time
- **RETRY_CHILD**: A child failed; record result and proceed to next child
- **MERGING**: All children done (or stopped), performing epic merge
- **DONE**: Epic merged or preserved, worker exits

### Child Execution

Each child bead executes as a normal `execute-bead` invocation inside the epic
worktree, with these differences:

- The base revision is the epic branch HEAD (not the project default branch)
- The child result is committed directly to the epic branch
- There is no individual land/preserve step — the child commit stays on the epic branch
- Failed children are recorded but do not block the next child
- The epic worker may be configured to stop on first child failure or continue

### Merge Gate

After all children are processed, the epic worker evaluates a merge gate:

1. Rebase the epic branch onto the current target branch HEAD
2. Run required post-epic checks (if any)
3. If clean: merge the epic branch to the target branch with `git merge --no-ff`
4. If conflicts: preserve the epic branch under `refs/ddx/epics/<bead-id>/` and report `land_conflict`

The merge commit message includes:
- Epic bead ID and title
- Number of children executed
- Number of children succeeded vs failed
- Worker ID and timestamp

## Server API

### Worker Endpoints

All worker endpoints are shared between single-ticket and epic workers. The
worker record includes a `mode` field to distinguish them.

#### `GET /api/agent/workers`

List all workers. Supports filtering:

| Query param | Values | Description |
|---|---|---|
| `mode` | `single`, `epic` | Filter by worker mode |
| `state` | `running`, `exited` | Filter by state |
| `project` | `<project-id>` | Filter by project |

Response includes `mode` field on each worker record.

#### `GET /api/agent/workers/:id`

Show worker details. Epic workers include additional fields:

```json
{
  "mode": "epic",
  "epic_bead_id": "ddx-cf340665",
  "epic_branch": "epic/ddx-cf340665",
  "epic_worktree": ".ddx/.epic-wt-ddx-cf340665",
  "children_total": 5,
  "children_done": 3,
  "children_succeeded": 2,
  "children_failed": 1,
  "children_remaining": 2,
  "current_child": "ddx-abc123"
}
```

Single-ticket workers include `"mode": "single"` and omit the epic fields.

#### `POST /api/agent/workers/execute-epic`

Start an epic worker. Request body:

```json
{
  "epic_bead_id": "ddx-cf340665",
  "project": "ddx",
  "model": "z-ai/glm-5.1",
  "harness": "agent",
  "stop_on_failure": false
}
```

- `stop_on_failure`: if true, stop executing children after the first failure
  (default: false — continue with remaining children)

#### `POST /api/agent/workers/:id/stop`

Gracefully stop a running worker. For epic workers, stops after the current
child completes. Does not abort mid-execution.

### Queue Endpoints

#### `GET /api/beads/ready?mode=epic`

List execution-ready epic beads. An epic is execution-eligible when:

- Status is `open` or `in_progress`
- `execution-eligible` is not explicitly `false`
- Not in cooldown from a prior attempt
- Has at least one execution-eligible child bead

#### `GET /api/beads/:id/children`

List child beads of an epic. Response includes each child's execution status.

### Observability Endpoints

#### `GET /api/agent/workers/:id/children`

For epic workers, list child execution results:

```json
[
  {
    "bead_id": "ddx-abc123",
    "status": "success",
    "detail": "",
    "base_rev": "abc1234",
    "result_rev": "def5678",
    "attempt_id": "20260412T120000-abc"
  },
  {
    "bead_id": "ddx-def456",
    "status": "execution_failed",
    "detail": "iteration_limit",
    "base_rev": "def5678",
    "result_rev": "def5678",
    "attempt_id": "20260412T121500-def"
  }
]
```

#### `GET /api/agent/workers/:id/log`

Same as single-ticket worker log, but also includes child-level progress:

```
▶ epic worker started (model: z-ai/glm-5.1)
  📋 epic: ddx-cf340665 — Design epic-scoped execution and merge workflow
  🌿 branch: epic/ddx-cf340665
  👶 child 1/5: ddx-abc123
  ✓ ddx-abc123 → success (abc1234)
  👶 child 2/5: ddx-def456
  ✗ ddx-def456 → execution_failed (iteration_limit)
  👶 child 3/5: ddx-ghi789
  ...
  🔀 merging epic branch → main
  ✓ epic merged (5 children: 4 succeeded, 1 failed)
```

## CLI Surface

### `ddx agent execute-epic`

```
ddx agent execute-epic <epic-bead-id> [flags]

Flags:
  --model string         Model to use for child execution
  --harness string       Agent harness (default: agent)
  --stop-on-failure      Stop after first child failure
  --local                Run inline instead of submitting to server
```

### `ddx server workers list --mode epic`

Filter worker list to epic workers only.

### `ddx server workers show <id>`

Works for both modes. Epic workers show child progress.

## Worker Record Schema

The existing `WorkerRecord` gains these fields:

```go
type WorkerRecord struct {
    // ... existing fields ...
    Mode            WorkerMode  `json:"mode"`              // "single" or "epic"
    EpicBeadID      string      `json:"epic_bead_id,omitempty"`
    EpicBranch      string      `json:"epic_branch,omitempty"`
    EpicWorktree    string      `json:"epic_worktree,omitempty"`
    ChildrenTotal   int         `json:"children_total,omitempty"`
    ChildrenDone    int         `json:"children_done,omitempty"`
    ChildrenSuccess int         `json:"children_succeeded,omitempty"`
    ChildrenFailed  int         `json:"children_failed,omitempty"`
    CurrentChild    string      `json:"current_child,omitempty"`
}

type WorkerMode string

const (
    WorkerModeSingle WorkerMode = "single"
    WorkerModeEpic   WorkerMode = "epic"
)
```

## Distinction From Single-Ticket Workers

| Aspect | Single-Ticket Worker | Epic Worker |
|---|---|---|
| Queue source | `ReadyExecution()` | `ReadyExecution()` filtered by `issue_type=epic` |
| Beads per run | 1 | All children of 1 epic |
| Branch strategy | Per-attempt hidden ref | Persistent `epic/<id>` branch |
| Worktree lifetime | Created/destroyed per attempt | Persistent for epic duration |
| Landing | Rebase + ff per attempt | Single `--no-ff` merge after all children |
| Failure mode | Single result | Partial: some children may succeed |
| Cooldown | Per-bead retry-after | Per-epic retry-after |

## Acceptance

1. Server API exposes `mode`, epic fields, and child results on worker records
2. `ddx agent execute-epic` creates a persistent epic branch and worktree
3. Child beads execute sequentially inside the epic worktree
4. Epic worker log shows per-child progress with aggregate status
5. Merge gate rebases epic branch and creates merge commit on success
6. Failed epic merges preserve the branch under `refs/ddx/epics/`
7. Single-ticket workers are unaffected — the two modes share infrastructure but not state
