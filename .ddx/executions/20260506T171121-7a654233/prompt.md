<bead-review>
  <bead id="ddx-ef2a7638" iter=1>
    <title>cli: expose DDx execution cleanup command and report</title>
    <description>
PROBLEM
Operators currently have no first-class command to inspect and clean DDx-owned stale execution resources. During the May 6 incident, cleanup required manual shell inspection of /tmp/ddx-exec-wt, git worktree list, and inode usage, which is error-prone and can accidentally delete non-DDx or preserved state.

ROOT CAUSE WITH FILE:LINE
- cli/cmd/command_factory.go:561-562 registers top-level work/try commands but there is no ddx cleanup or doctor cleanup command that invokes the execution cleanup manager.
- cli/cmd/doctor.go:111-202 checks general repo health but does not expose DDx execution temp worktree/liveness cleanup.
- cli/internal/agent/execute_bead.go:257-279 defines DDx temp worktree locations, but operators must discover and prune them manually.
- docs/helix/02-design/solution-designs/SD-025-task-execution-lifecycle.md:240-244 requires an explicit operator command such as ddx cleanup or ddx doctor cleanup that runs the same manager and reports what it removed.

PROPOSED FIX
- Add an operator command, preferably ddx cleanup unless existing CLI conventions strongly favor ddx doctor cleanup.
- Support dry-run and apply behavior if consistent with existing command style; at minimum produce a clear human summary and JSON output.
- Wire the command to the shared cleanup manager, not a separate shell/script implementation.
- Report removed registered worktrees, unregistered dirs, liveness files, warnings, and resource observations; never scan outside configured DDx roots.

NON-SCOPE
- Do not add destructive broad system cleanup outside DDx-owned roots.
- Do not remove complete .ddx/runs or .ddx/executions evidence.
- Do not require the command to fix provider quota/auth/network failures.
    </description>
    <acceptance>
1. TestCleanupCommand_DryRunReportsWithoutDeleting verifies dry-run lists stale DDx resources and leaves them on disk.
2. TestCleanupCommand_ApplyRemovesStaleDDxResources verifies apply mode invokes the shared cleanup manager and removes stale temp worktrees/liveness files.
3. TestCleanupCommand_JSONShape verifies JSON output includes removed counts, warnings, bytes/inodes when available, and blocked errors.
4. TestCleanupCommand_DoesNotRemovePreservedEvidence verifies preserved attempts and complete evidence bundles survive command execution.
5. The command is registered in the root CLI and documented in docs/agent-execute.md or adjacent operator docs.
6. cd cli &amp;&amp; go test ./cmd/... ./internal/agent/... -run "TestCleanupCommand|TestExecutionCleanup" -count=1 passes.
7. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:cli, area:agent, kind:implementation, reliability, spec:FEAT-010, spec:SD-025, cleanup, operator-ux</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T170034-43fc95ff/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T170034-43fc95ff/result.json</file>
  </changed-files>

  <governing>
    <ref id="SD-025" path="docs/helix/02-design/solution-designs/SD-025-task-execution-lifecycle.md" title="Solution Design: Task Execution Lifecycle">
      <content>
<untrusted-data>
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
finish the TD-031 interruption path: preserve available evidence, remove
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
</untrusted-data>
      </content>
    </ref>
  </governing>

  <diff rev="3142e13e4bee4e03b397842df83b38450dec33b6">
<untrusted-data>
diff --git a/.ddx/executions/20260506T170034-43fc95ff/checks/production-reachability.json b/.ddx/executions/20260506T170034-43fc95ff/checks/production-reachability.json
new file mode 100644
index 000000000..246408be7
--- /dev/null
+++ b/.ddx/executions/20260506T170034-43fc95ff/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T170034-43fc95ff/result.json b/.ddx/executions/20260506T170034-43fc95ff/result.json
new file mode 100644
index 000000000..e1167793b
--- /dev/null
+++ b/.ddx/executions/20260506T170034-43fc95ff/result.json
@@ -0,0 +1,24 @@
+{
+  "bead_id": "ddx-ef2a7638",
+  "attempt_id": "20260506T170034-43fc95ff",
+  "base_rev": "593684cbb142be731f4d482cadf60a06437e9763",
+  "result_rev": "32c0d0a91a3ba8b32a5c255db3b61d6e2a59fb9c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-c714f57c",
+  "duration_ms": 633978,
+  "tokens": 7639164,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T170034-43fc95ff",
+  "prompt_file": ".ddx/executions/20260506T170034-43fc95ff/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T170034-43fc95ff/manifest.json",
+  "result_file": ".ddx/executions/20260506T170034-43fc95ff/result.json",
+  "usage_file": ".ddx/executions/20260506T170034-43fc95ff/usage.json",
+  "started_at": "2026-05-06T17:00:36.913253103Z",
+  "finished_at": "2026-05-06T17:11:10.891924495Z"
+}
\ No newline at end of file
</untrusted-data>
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

For each acceptance-criteria (AC) item, decide whether it is implemented correctly, then assign one overall verdict:

- APPROVE — every AC item is fully and correctly implemented.
- REQUEST_CHANGES — some AC items are partial or have fixable minor issues.
- BLOCK — at least one AC item is not implemented or incorrectly implemented; or the diff is insufficient to evaluate.

## Required output format (schema_version: 1)

Respond with EXACTLY one JSON object as your final response, fenced as a single ```json … ``` code block. Do not include any prose outside the fenced block. The JSON must match this schema:

```json
{
  "schema_version": 1,
  "verdict": "APPROVE",
  "summary": "≤300 char human-readable verdict justification",
  "findings": [
    { "severity": "info", "summary": "what is wrong or notable", "location": "path/to/file.go:42" }
  ]
}
```

Rules:
- "verdict" must be exactly one of "APPROVE", "REQUEST_CHANGES", "BLOCK".
- "severity" must be exactly one of "info", "warn", "block".
- Output the JSON object inside ONE fenced ```json … ``` block. No additional prose, no extra fences, no markdown headings.
- Do not echo this template back. Do not write the words APPROVE, REQUEST_CHANGES, or BLOCK anywhere except as the JSON value of the verdict field.
  </instructions>
</bead-review>
