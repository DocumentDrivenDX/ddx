---
ddx:
  id: SD-025
  depends_on:
    - FEAT-005
    - FEAT-006
    - FEAT-010
---
# Solution Design: Three-Layer Run Substrate

## Purpose

This sketch turns FEAT-010's three-layer run architecture into an implementation
shape. It is intentionally below the feature spec and above code-level storage
types: enough detail to keep implementation beads aligned without re-opening the
routing boundary in FEAT-006.

The substrate has exactly three layers:

| Layer | Public verb | Meaning |
| --- | --- | --- |
| 1 | `ddx run` | one upstream agent invocation |
| 2 | `ddx try <bead>` | one bead attempt in an isolated worktree |
| 3 | `ddx work` | one mechanical queue-drain loop |

Each higher layer references child records from the layer beneath it. Comparison,
replay, benchmark, review, and artifact-regeneration workflows are compositions
over these layers, not new run kinds.

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
- terminal disposition: `drained`, `blocked`, `deferred`, `no_progress`, or
  `signal`

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
| `signal` | SIGINT/SIGTERM received after in-flight attempt finalizes |

The layer-3 evaluation log records the candidate bead, previous outcome,
retry eligibility, requested power bounds, passthrough envelope presence, and
the condition that fired. The log may inspect DDx-owned attempt outcomes and
agent typed status; it must not branch on concrete provider/model identity.

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
