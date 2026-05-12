---
ddx:
  id: SD-025
  depends_on:
    - FEAT-005
    - FEAT-006
    - FEAT-010
---
# Solution Design: Task Execution Lifecycle

## Purpose

This design turns FEAT-010's task execution lifecycle into an implementation
shape. It is intentionally below the feature spec and above code-level storage
types: enough detail to keep the `run`, `try`, and `work` layers aligned
without re-opening the routing boundary in FEAT-006.

## Layer Model

Task execution is composed from named layers:

| Layer | Public verb | Meaning |
| --- | --- | --- |
| 1 | `ddx run` | one upstream agent invocation |
| 2 | `ddx try <bead>` | one bead attempt in an isolated worktree |
| 3 | `ddx work` | one mechanical queue-drain loop |

Each higher layer references child records from the layer beneath it. Comparison,
replay, benchmark, review, and artifact-regeneration workflows are compositions
over the task execution lifecycle, not new run kinds.

## Record Shape

Every record lives under `.ddx/runs/<run-id>/record.json`. Large bodies are
attachments in the same directory.

```json
{
  "run_id": "run_20260430T120102Z_01HV...",
  "layer": 2,
  "parent_run_id": "run_20260430T115900Z_01HV...",
  "started_at": "2026-04-30T12:01:02Z",
  "finished_at": "2026-04-30T12:04:12Z",
  "terminal_status": "success",
  "actor": "erik",
  "host": "sindri",
  "git_revision": "abc123",
  "ddx_version": "0.0.0-dev",
  "produces_artifact": null,
  "attachments": [
    {"name": "prompt", "path": "prompt.md", "media_type": "text/markdown"},
    {"name": "result", "path": "result.json", "media_type": "application/json"}
  ],
  "layer1": null,
  "layer2": {
    "bead_id": "ddx-12345678",
    "base_revision": "abc123",
    "result_revision": "def456",
    "worktree_path": ".ddx/worktrees/ddx-12345678-run_...",
    "finalization": "merge",
    "child_run_ids": ["run_20260430T120104Z_01HV..."],
    "gate_summary": "go test ./internal/agent"
  },
  "layer3": null
}
```

Rules:

- `layer` is the discriminator; exactly one of `layer1`, `layer2`, or `layer3`
  is populated.
- `parent_run_id` points upward when a run is nested; child ids live in the
  layer extension for efficient drill-down.
- `terminal_status` is the substrate outcome. Domain-specific detail remains in
  the layer extension and attachments.
- `produces_artifact` is set when a run regenerates an artifact; FEAT-007 reads
  it as provenance for `generated_by`.
- Records are append-only. Correction means writing a new record or a separate
  review/annotation artifact, not editing a historical record.

## Layer Extensions

### Layer 1: invocation atom

Layer 1 stores the DDx request envelope and the agent's typed response fields:

- prompt attachment or prompt reference
- requested `MinPower` and optional `MaxPower`
- opaque passthrough constraints (`harness`, `provider`, `model`) when supplied
- non-routing work facts such as permissions, effort, timeout, and artifact id
- actual model and actual power returned by the agent
- token/cost/duration metadata
- upstream session id and session-log attachment pointer
- structured result attachment

DDx does not persist route decisions as DDx-owned policy. Concrete
harness/provider/model values are recorded only as requested passthrough or
agent-reported facts.

### Layer 2: bead attempt

Layer 2 stores DDx-owned attempt orchestration:

- bead id and governing artifact ids
- base revision and worktree path
- child layer-1 run ids
- commit/result revision, if produced
- finalization mode: `merge`, `preserve`, `no_changes`, or `aborted`
- preserve ref, if any
- gate results and review verdicts
- evidence-bundle attachment refs
- cleanup summary: removed worktree path, preserve reason, partial setup
  cleanup, and resource-preflight result

Layer 2 owns success classification for a bead attempt because DDx owns commits,
merge/preserve state, no-change rationale, post-run gates, and review evidence.
The agent's exit status and actual power are inputs, not the full decision.
Per FEAT-010's network-free drain boundary, this layer performs no network I/O;
origin-sync and push are out-of-band per FEAT-023.

### Layer 3: queue drain

Layer 3 stores the mechanical queue loop:

- queue snapshot at start
- child layer-2 attempt ids in execution order
- retry/escalation decisions with requested `MinPower` changes
- no-progress counter state
- stop-condition evaluation log
- cleanup pass summaries and resource stop details
- terminal disposition: `drained`, `blocked`, `deferred`, `no_progress`,
  `signal`, or `resource_exhausted`

Layer 3 does not file new content-aware work or decide workflow policy.
Supervisory behavior belongs to skills or plugins layered on top.
Per FEAT-010's network-free drain boundary, this layer performs no network I/O;
origin-sync and push are out-of-band per FEAT-023.

## On-Disk Layout

```text
.ddx/
└── runs/
    └── <run-id>/
        ├── record.json
        ├── prompt.md
        ├── result.json
        ├── session.jsonl
        ├── stdout.log
        ├── stderr.log
        └── evidence/
            ├── manifest.json
            └── checks.json
```

Writers create records in a temporary directory and atomically rename into
`.ddx/runs/<run-id>/`. A partially written directory is never published as a
complete run. Attachment paths in `record.json` are relative to the run
directory.

## Legacy Migration

Two existing layouts are read during the migration window:

- `.ddx/exec-runs/` or `.ddx/exec-runs.d/` records become virtual layer-1
  records.
- `.ddx/executions/<attempt-id>/` bundles become virtual layer-2 records.

Migration rules:

1. New writes always target `.ddx/runs/`.
2. Readers expose legacy records through the same `ddx runs` query model.
3. A one-shot rewrite can normalize legacy records into `.ddx/runs/`.
4. After the documented cutoff, fallback readers are removed.

The migration must preserve git-tracked execute-bead evidence. It must not
rewrite historical bead attempt commits or `closing_commit_sha` pointers.

## Layer 3.5: Auto-Recovery

Auto-recovery is a distinct stage that runs **between** Layer 3 (queue drain) and the `status=proposed` operator-review escape. It is triggered when a bead's within-cycle escalation ladder has been exhausted on two or more consecutive drain cycles (`Extra["consecutive_ladder_exhaustions"] >= 2`; see TD-031 §5 (`consecutive_ladder_exhaustions` Policy) field). The bead must be `status=open` and not carry the `recovery:manual` label.

### Trigger

The drain loop evaluates the auto-recovery trigger after each drain cycle before selecting the next candidate. If the trigger condition is met for any `status=open`, execution-eligible bead, the loop claims the bead (or waits for the existing claim to release) and enters the auto-recovery sequence. The trigger does not fire during a running attempt; it fires only between attempts.

### Sequence

1. **Reframe attempt** — dispatch a strong-tier reframer agent (ADR-024 P3; TD-031 §4 (Auto-Recovery Role Catalogue)) with `MinPower` set above the exhausted escalation ceiling. The reframer receives the bead record (description, AC, governing artifact refs) and returns structured edits. If the reframer returns edits, DDx applies them via `ddx bead update` paths, records `reframe-applied` (TD-027 §13), resets `consecutive_ladder_exhaustions` to 0, releases the claim, and returns the bead to `status=open` execution-ready. The bead re-enters the standard drain cycle with a fresh ladder.
2. **Decompose attempt** (only when reframe returned no change or the reframer invocation failed) — dispatch a strong-tier decomposer agent with the same `MinPower` floor. The decomposer returns 2–5 executable child bead specs when the bead can be split under itself. If child depth is exhausted, it returns sibling or replacement bead specs under the nearest safe parent/root. DDx calls `Store.Create` for each generated bead, wires the required dependency or supersession edges, records `decompose-applied` (TD-027 §13), and leaves the oversized bead `status=open` with `Extra["execution-eligible"]=false` when it should no longer be claimed directly. The queue advances through the generated executable work.
3. **Final escape** (only when child decomposition and sibling/replacement decomposition returned no executable work or failed) — DDx records `auto-recovery-failed` (TD-027 §13), moves the bead to `status=proposed`, clears the active claim, and does not schedule another attempt. The operator must resolve the bead via `ddx bead update --status open` (after fixing it) or `ddx bead update --status cancelled`.

### Layer-3 record

The auto-recovery sequence runs within the layer-3 run record as an additional stop-condition evaluation entry. The entry names the bead, the trigger condition (`consecutive_ladder_exhaustions` value), which recovery step was attempted, the outcome, and the cost of any reframer/decomposer invocations. These costs contribute to the drain-level budget cap (FEAT-014).

### Cross-references

- **TD-031 §2** — `reframe_applied`, `decompose_applied`, `auto_recovery_failed`, and `per_bead_budget_exhausted` outcome rows define the status transitions and Extra mutations.
- **ADR-024 P4** — the principle authorizing this stage.
- **ADR-024 Escalation Sequencing** — the strict ordering of within-cycle, cross-cycle, and `status=proposed` escapes.
- **TD-031 §4 (Auto-Recovery Role Catalogue)** — reframer and decomposer dispatch contracts.
- **TD-031 §5 (`consecutive_ladder_exhaustions` Policy)** — the counter that drives the trigger.

## Stop Conditions

`ddx work` evaluates stop conditions only between `ddx try` attempts:

| Disposition | Trigger |
| --- | --- |
| `drained` | no ready beads remain |
| `blocked` | remaining ready beads are terminal for the current policy |
| `deferred` | wall-clock or attempt-count budget exhausted |
| `no_progress` | configured consecutive attempts produced no commit and no merged side effect |
| `signal` | SIGINT/SIGTERM received; first signal cancels in-flight work cooperatively and stops new claims |
| `resource_exhausted` | execution roots are not safely writable or lack required bytes/inodes after cleanup |

The layer-3 evaluation log records the candidate bead, previous outcome,
retry eligibility, requested power bounds, passthrough envelope presence, and
the condition that fired. The log may inspect DDx-owned attempt outcomes and
agent typed status; it must not branch on concrete provider/model identity.

Signal handling is process-root behavior, not a per-command special case.
The root command installs a SIGINT/SIGTERM-backed context. On the first signal,
DDx writes `Cancel received, shutting down gracefully` to stderr/stdout visible
to the operator, cancels the root context, and lets `ddx work` / `ddx try`
finish the TD-027 §12.2 worker-interruption path: preserve available evidence, remove
runtime liveness state, release any non-terminal bead claim, and exit with
layer-3 disposition `signal`. A second signal may hard-abort rather than wait
for cleanup.

## Cleanup Manager

The run substrate includes one DDx-owned cleanup manager used by `ddx try`,
`ddx work`, server-managed workers, and an explicit operator cleanup command.
The manager is scoped to DDx-created execution resources only; it never scans or
removes arbitrary temp directories.

### Roots and ownership

The manager knows the configured temporary worktree root, durable run/evidence
roots, worker liveness root, and project git repository. Every created
temporary execution directory records enough ownership to answer:

- which project created it
- which attempt or worker owns it
- whether the attempt reached published evidence
- whether it was intentionally preserved
- when its liveness signal was last refreshed

During the legacy migration window, the manager handles both
`.ddx/executions/<attempt-id>` and `.ddx/runs/<run-id>` evidence, but it treats
complete evidence bundles as durable data rather than scratch.
DDx-owned cleanup scope also includes DDx-created test and e2e scratch roots,
generated test binaries, and run-state or liveness files. Recognized DDx-owned
scratch prefixes are: `ddx-exec-wt`, `ddx-claim-heartbeats`,
`ddx-metric-keepalive`, `ddx-test-`, and `ddx-e2e-`. Any directory containing
a `cleanup.json` metadata file with matching project ownership is DDx-owned
regardless of its name. A path is eligible for deletion when: (a) it has
`cleanup.json` metadata and the liveness is expired or the owning attempt is
terminal; or (b) it matches a recognized DDx prefix without metadata, the
directory mtime is at least **6 hours** old, and no live PID or active session
is present. The manager preserves published evidence and registered active
worktrees.

### Entry points

Inline cleanup runs inside `ddx try` before and after worktree lifecycle
operations. It removes partial directories from failed setup and removes the
isolated worktree after merge, preserve, no-changes, no-evidence, failed
publish, or cooperative interruption finalization.

Loop cleanup runs inside `ddx work` at startup, between attempts after
setup/finalization failure, periodically while polling, and during graceful
shutdown. It may clean stale resources from prior attempts before the next
claim.

Background cleanup runs occasionally while long-lived DDx processes are alive.
It uses jitter plus a project cleanup lock so concurrent workers do not all
prune at once. It must tolerate another process winning the race to remove a
resource.

An explicit operator command, such as `ddx cleanup` or `ddx doctor cleanup`,
runs the same manager and reports what it removed without requiring a queue
drain.

### Conservative deletion rules

The manager may delete:

- unregistered directories under DDx temp worktree roots whose names and
  metadata identify DDx ownership and whose liveness is absent or stale
- registered git worktrees under DDx temp roots whose owning attempt is
  terminal or whose liveness marker is stale
- DDx-created test and e2e scratch roots that satisfy the same ownership,
  age, and liveness rules
- generated test binaries and run-state or liveness files that are DDx-owned
- stale heartbeat/liveness files for dead PIDs or expired sessions
- partial setup directories that never reached atomic evidence publication
- old non-preserved scratch data past configured retention

The manager must not delete:

- preserved attempt worktrees
- local evidence refs under `refs/ddx/iterations/...`
- complete `.ddx/runs/<id>` or `.ddx/executions/<attempt-id>` evidence
- active worktrees with a live PID/session heartbeat
- paths outside configured DDx roots
- paths that only loosely resemble DDx names without matching ownership
  metadata or registered worktree evidence

### Resource preflight and loop-fatal handling

Before `ddx try` claims a bead or creates a worktree, it checks the configured
temp worktree root and durable evidence root for writability, free bytes, and
free inodes when inode data is available. If preflight fails, `ddx try` runs one
immediate cleanup pass and repeats the check. If the check still fails, the
attempt returns `resource_exhausted` without claiming the bead.

`ddx work` and server-managed workers perform the same cleanup pass before the
first claim and before any later claim when any checked temp or evidence root
falls below the soft cleanup trigger of **512 MiB free bytes** or **8192 free
inodes**. If the temp roots remain below the hard stop floor of **64 MiB free
bytes** and **1024 free inodes** after that cleanup, the loop records host
exhaustion and stops claiming new beads.

If resource exhaustion occurs after a claim or during worktree setup,
`ddx try` records whatever partial evidence is available, removes any partial
unregistered directory it created, releases the claim if the bead did not close,
and returns `resource_exhausted`.

`ddx work` treats `resource_exhausted` as host infrastructure failure. It
records a layer-3 stop-condition entry, prints an operator-visible message such
as `resource exhausted after cleanup; stopping work loop`, and stops claiming
new beads. It must not continue scanning the ready queue in the same process.

Cleanup output is structured. Routine passes are trace/debug or worker events.
Passes that reclaim meaningful bytes or inodes emit an operator-visible summary
including counts. Failures include path, class, and whether the failure blocked
progress.

Expected implementation tests include `TestExecutionCleanup_RemovesStaleDDXScratchDirs`,
`TestWorkResourcePreflight_RunsCleanupBelowSoftFloor`, and
`TestWorkResourcePreflight_StopsBelowHardFloorAfterCleanup`.

## Drill-Down Query Model

The read API is centered on `ddx runs`:

- `ddx runs list --layer 1|2|3 --bead <id> --artifact <id>`
- `ddx runs show <run-id>`
- `ddx runs children <run-id>`
- `ddx runs parents <run-id>`
- `ddx runs log <run-id>`
- `ddx runs result <run-id>`

Server and MCP surfaces mirror those reads. A layer-3 detail page drills down
to child layer-2 attempts; each layer-2 attempt drills down to child layer-1
invocations. The graph is navigated by `parent_run_id` and child id arrays, not
by path guessing.

## Implementation Notes

- Reuse the existing execution attachment discipline where it still fits, but
  do not keep `.ddx/exec-runs` and `.ddx/executions` as peer authoritative
  writers.
- Keep passthrough constraints in one request/record subobject so harness,
  provider, and model do not spread through queue, retry, or scheduling code.
- Treat artifact regeneration as metadata on layer 1 or layer 2, depending on
  whether the generator returns content directly or edits the repo in a
  worktree.
- Add tests at the storage boundary for atomic publish, legacy read
  compatibility, parent/child traversal, and no-progress stop-condition
  recording.
