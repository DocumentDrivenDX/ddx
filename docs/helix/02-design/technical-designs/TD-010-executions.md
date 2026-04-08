---
ddx:
  id: TD-010
  depends_on:
    - FEAT-006
    - FEAT-010
    - ADR-004
---
# Technical Design: Execution Definitions and Runs

## Purpose

This design defines the generic DDx execution substrate introduced by
FEAT-010. It establishes the storage model, collection boundaries, attachment
layout, compatibility rules, and inspection semantics that all execution
specializations must reuse.

Metric- or acceptance-specific behavior is layered on top of this design. The
generic substrate owns:

- execution-definition storage
- immutable execution-run storage
- attachment publication
- read/write compatibility with legacy exec layouts
- CLI and server inspection behavior for definitions, runs, logs, and results

## Definition Sources

Execution definitions have two authoritative sources:

1. **Graph-authored execution documents** — git-tracked markdown files with
   `ddx:` frontmatter (see FEAT-007). These are the preferred source when using
   `ddx agent execute-bead`. DDx discovers them by traversing the document graph
   from the target bead and its governing artifacts. They participate in ordinary
   document indexing and staleness tracking.

2. **Runtime-managed definitions** — machine-readable records stored in the
   `exec-definitions` collection. These are valid for `ddx exec` operations that
   do not require graph discovery.

DDx may maintain an internal indexed or compiled representation of graph-authored
execution documents for runtime speed, but the git-tracked document is the source
of truth. `ddx exec validate/run/history/result/log` resolve and operate on both
sources; graph-authored definitions take precedence when both exist for the same
artifact.

## Storage Model

Executions use named bead-backed collections rather than a bespoke `.ddx/exec/`
metadata format.

- `exec-definitions` stores machine-readable runtime-managed execution definitions
- `exec-runs` stores immutable execution-run metadata rows
- `exec-runs.d/` stores attachment-backed payloads for one run

For the JSONL backend, those logical collections map to:

```text
.ddx/
├── exec-definitions.jsonl
├── exec-runs.jsonl
└── exec-runs.d/
    └── <run-id>/
        ├── result.json
        ├── stdout.log
        └── stderr.log
```

Other backends preserve the same logical collection boundary without DDx
prescribing their physical layout.

Legacy compatibility:

- older definition bundles under `.ddx/exec/definitions/*.json` remain readable
  during migration
- older run bundles under `.ddx/exec/runs/<run-id>/` remain readable during
  migration
- new authoritative writes target the bead-backed collections and attachment
  directories above

## Pre-exec Metric Storage Migration

Prior to the exec substrate (before commit `2647ae4`), the metric store wrote
directly to:

```text
.ddx/
└── metrics/
    ├── definitions/
    │   └── <definition-id>.json    # one JSON file per definition
    └── history.jsonl               # append-only JSONL history log
```

This layout was only present in very early DDx builds before the exec substrate
was introduced. It was never part of a public or tagged release.

Migration status: **no migration is required.**

The current metric store delegates entirely to the exec substrate. It does not
read from `.ddx/metrics/` at all. If an old `.ddx/metrics/` directory is
present, DDx ignores it silently — no crash, no data corruption, no stale reads.

Operators who upgraded from an early pre-release build and want to reclaim disk
space may safely delete `.ddx/metrics/`. Historical metric runs stored there
will no longer appear in `ddx metric history`, but this data loss is acceptable
given that no public release ever wrote to that path.

## Definition Record Shape

Each definition is stored as one bead-backed row in the `exec-definitions`
collection. The shared bead envelope does not gain new top-level fields for
execution; the definition payload lives in preserved extras.

```json
{
  "id": "exec-metric-startup-time@1",
  "issue_type": "exec_definition",
  "status": "open",
  "title": "Execution definition for MET-001",
  "labels": ["artifact:MET-001", "executor:command"],
  "definition": {
    "id": "exec-metric-startup-time@1",
    "artifact_ids": ["MET-001"],
    "executor": {
      "kind": "command",
      "command": ["go", "test", "./..."],
      "cwd": ".",
      "timeout_ms": 30000
    },
    "result": {
      "metric": {
        "unit": "ms",
        "value_path": "$.duration_ms"
      }
    },
    "evaluation": {
      "comparison": "lower-is-better"
    },
    "active": true,
    "created_at": "2026-04-04T15:00:00Z"
  }
}
```

Rules:

- the bead envelope provides shared runtime metadata and backend portability
- the full execution definition is stored inside preserved extras under
  `definition`
- `status=open` means active; `status=closed` means retired but preserved
- one execution definition may link to one or more artifact IDs
- collection readers must ignore retired or malformed definitions rather than
  inventing partial runtime contracts

## Run Record Shape

Each execution produces one bead-backed row in the `exec-runs` collection plus
zero or more sidecar attachments. The shared bead envelope does not gain new
top-level fields for execution; run payloads live in preserved extras.

```json
{
  "id": "exec-metric-startup-time@2026-04-04T15:01:00Z-1",
  "issue_type": "exec_run",
  "status": "closed",
  "title": "Execution run for MET-001",
  "labels": ["artifact:MET-001", "executor:command"],
  "run": {
    "run_id": "exec-metric-startup-time@2026-04-04T15:01:00Z-1",
    "definition_id": "exec-metric-startup-time@1",
    "artifact_ids": ["MET-001"],
    "started_at": "2026-04-04T15:01:00Z",
    "finished_at": "2026-04-04T15:01:01Z",
    "status": "success",
    "exit_code": 0,
    "attachments": {
      "stdout": "exec-runs.d/exec-metric-startup-time@2026-04-04T15:01:00Z-1/stdout.log",
      "stderr": "exec-runs.d/exec-metric-startup-time@2026-04-04T15:01:00Z-1/stderr.log",
      "result": "exec-runs.d/exec-metric-startup-time@2026-04-04T15:01:00Z-1/result.json"
    },
    "provenance": {
      "git_rev": "abc123",
      "ddx_version": "0.1.0"
    }
  },
  "result": {
    "metric": {
      "artifact_id": "MET-001",
      "definition_id": "exec-metric-startup-time@1",
      "observed_at": "2026-04-04T15:01:00Z",
      "status": "pass",
      "value": 14.6,
      "unit": "ms",
      "samples": [14.6]
    }
  }
}
```

Rules:

- the bead envelope `status` tracks row lifecycle, not domain outcome
- domain execution outcome lives in `run.status`
- the full structured result payload may appear inline under `result` and may
  also be published to `attachments.result` for durable inspection
- attachment references are relative, repo-local paths stored in preserved
  extras
- run rows are append-only; later executions never rewrite prior run rows

## Attachment Publication

Large bodies are written outside the collection row:

- stdout
- stderr
- normalized result payload
- future executor-specific payloads

Write order:

1. Resolve the target collection and attachment root.
2. Create the payloads in a temporary attachment directory.
3. Acquire the collection lock.
4. Copy the current collection snapshot to a backup when repair or rewrite
   safety is required.
5. Publish the attachment directory into its final location with an atomic
   rename or backend-equivalent swap.
6. Append or update the bead-backed metadata row in the target collection.
7. Release the lock.

This guarantees published rows do not point at missing attachment paths. A
crash may leave an orphan temporary directory or backup copy, but it must not
leave a partial published row.

## Read and Inspection Semantics

The generic execution substrate owns these inspection paths:

- `ddx exec list [--artifact ID]`
- `ddx exec show <definition-id>`
- `ddx exec validate <definition-id>`
- `ddx exec run <definition-id>`
- `ddx exec history [--artifact ID] [--definition ID]`
- `ddx exec log <run-id>`
- `ddx exec result <run-id>`

Read rules:

- list/show/history prefer bead-backed collection records
- legacy `.ddx/exec/` data remains a read fallback during migration
- detail readers resolve attachments lazily from the referenced paths
- sorting is deterministic:
  - definitions by `created_at` descending, then `id`
  - runs by `started_at` ascending, then `run_id`

Server read APIs must use the same underlying reader logic so CLI and HTTP/MCP
do not drift.

## Validation Algorithm

`ddx exec validate <definition-id>` performs:

1. Load the preferred definition from the collection, falling back to legacy
   storage only when no collection record exists.
2. Verify the definition has an ID and at least one linked artifact ID.
3. Resolve each linked artifact ID through the document graph.
4. Validate the executor kind and executor-specific fields.
5. Validate any specialization-specific result/evaluation fields.
6. Report the failing definition ID, artifact ID, and storage source
   deterministically.

## Run Algorithm

`ddx exec run <definition-id>` performs:

1. Validate the definition and linked artifacts.
2. Invoke the configured executor.
3. Capture stdout, stderr, exit code, timestamps, and provenance.
4. Normalize the structured result payload.
5. Publish attachments.
6. Persist one immutable `exec-runs` metadata row.

The first implementation supports `command` execution. `agent` remains a
defined executor kind and must eventually delegate to `ddx agent` while
preserving the same run/attachment model.

## Failure Modes

- missing linked artifact: validation fails with the missing artifact ID
- unknown executor kind: validation fails before invocation
- attachment publish failure: no metadata row is published
- malformed collection record: the reader reports the concrete collection and
  record ID
- corrupt legacy run bundle: the reader reports the concrete bundle path
