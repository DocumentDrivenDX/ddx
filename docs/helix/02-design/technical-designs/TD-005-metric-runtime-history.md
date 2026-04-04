---
ddx:
  id: TD-005
  depends_on:
    - FEAT-005
    - FEAT-010
    - SD-005
---
# Technical Design: Metric Runtime and History

## File Layout

For the JSONL backend, the logical execution collections map to separate files:

```text
.ddx/
├── exec-definitions.jsonl        # bead-backed execution definition records
├── exec-runs.jsonl               # bead-backed execution run records
└── exec-runs.d/
    └── <run-id>/
        ├── result.json
        ├── stdout.log
        └── stderr.log
```

Metric-specific inspection is a projection over the generic execution store.
No separate `.ddx/metrics/` runtime store is required.

Other backends preserve the same logical collections even if they do not map
them to JSONL files directly. Their physical storage layout is owned by the
backend implementation.

## Execution Definition Format

Each definition record is a bead-schema row in the `exec-definitions`
collection.

```json
{
  "id": "exec-metric-startup-time@1",
  "issue_type": "exec_definition",
  "status": "open",
  "title": "Execution definition for MET-001 startup time",
  "labels": ["artifact:MET-001", "executor:command"],
  "artifact_ids": ["MET-001"],
  "executor": {
    "kind": "command",
    "command": ["ddx", "bead", "create"],
    "cwd": ".",
    "env": {
      "DDX_DISABLE_UPDATE_CHECK": "1"
    }
  },
  "result": {
    "metric": {
      "unit": "ms",
      "value_path": "$.duration_ms"
    }
  },
  "evaluation": {
    "comparison": "lower-is-better",
    "thresholds": {
      "warn_ms": 20,
      "ratchet_ms": 30
    }
  },
  "active": true,
  "created_at": "2026-04-04T15:00:00Z"
}
```

Rules:
- The execution definition is metadata, not executable code.
- The linked `artifact_ids` set must include exactly one `MET-*` artifact.
- `status=open` means the definition is active; `status=closed` means the
  definition is retired but preserved.
- Only one active definition should exist per metric at a time.
- Historical definitions remain readable for comparison and auditing.

## Execution Run Format

Each execution creates one execution-run bead row in the `exec-runs`
collection plus optional sidecar files under `exec-runs.d/<run-id>/`.

```json
{
  "id": "exec-metric-startup-time@2026-04-04T15:01:00Z",
  "issue_type": "exec_run",
  "status": "closed",
  "title": "Execution run for MET-001 startup time",
  "labels": ["artifact:MET-001", "executor:command", "kind:metric"],
  "run_status": "success",
  "definition_id": "exec-metric-startup-time@1",
  "artifact_ids": ["MET-001"],
  "started_at": "2026-04-04T15:01:00Z",
  "finished_at": "2026-04-04T15:01:00Z",
  "result": {
    "metric": {
      "artifact_id": "MET-001",
      "definition_id": "exec-metric-startup-time@1",
      "observed_at": "2026-04-04T15:01:00Z",
      "status": "pass",
      "value": 14.6,
      "unit": "ms",
      "samples": [14.6],
      "comparison": {
        "baseline": 20,
        "delta": -5.4,
        "direction": "lower-is-better"
      }
    }
  },
  "exit_code": 0,
  "attachments": {
    "stdout": "exec-runs.d/exec-metric-startup-time@2026-04-04T15:01:00Z/stdout.log",
    "stderr": "exec-runs.d/exec-metric-startup-time@2026-04-04T15:01:00Z/stderr.log",
    "result": "exec-runs.d/exec-metric-startup-time@2026-04-04T15:01:00Z/result.json"
  },
  "provenance": {
    "git_rev": "abc123",
    "ddx_version": "0.1.0"
  }
}
```

The execution-run row is the searchable metadata/index record. `result.json`
stores the structured metric payload referenced by the row. `stdout.log` and
`stderr.log` store raw captured output bodies when present.

Migration policy:
- New writes target the bead-backed `exec-definitions` and `exec-runs`
  collections.
- Legacy `.ddx/exec/definitions/` and `.ddx/exec/runs/` data remains readable
  as a fallback during migration.
- `ddx metric` projections resolve through `ddx exec`; they do not write to a
  separate authoritative `.ddx/metrics/` runtime store.

The bead-envelope `status` tracks record lifecycle. Domain execution outcome is
stored separately as `run_status` to avoid redefining the shared bead field.

## Write Mechanism

Execution definition and run writes must be safe under concurrent agents.

1. Resolve the named bead-backed collection (`exec-definitions` or `exec-runs`).
2. Create any large payloads in a temporary attachment directory.
3. Acquire the collection lock.
4. Publish the attachment directory into its final location.
5. Create or update the bead-schema row in the collection using the existing
   bead store write path.
6. Release the lock.

This reuses the bead store semantics for metadata writes while keeping large
payloads out of the collection row. A crash may leave an orphan attachment
directory, but it must not publish a partial row.

## Validation Algorithm

`ddx exec validate <definition-id>` performs:

1. Resolve the linked `MET-*` artifact.
2. Resolve the execution definition by ID.
3. Verify the execution-definition bead row is valid for the collection.
4. Verify the executor configuration is present and executable.
5. Validate metric-specific result and evaluation fields.
6. Report actionable errors with the artifact ID, definition ID, and backing
   collection that failed.

## Run Algorithm

`ddx exec run <definition-id>` performs:

1. Resolve the execution definition and linked `MET-*` artifact.
2. Spawn the command in the configured working directory.
3. Capture exit code, stdout, stderr, and elapsed duration.
4. Normalize the measured value(s) into `result.metric`.
5. Compute comparison output against the configured baseline.
6. Write the generic execution-run row and referenced attachments atomically
   enough that published rows never point at missing files.

## Compare and Trend

- Metric compare reads generic execution history, filters by `MET-*` artifact
  or definition ID, resolves the requested baseline, and reports a
  deterministic delta.
- Metric history filters execution-run rows by metric-linked artifact and
  prints them in observed order.
- Metric trend aggregates the `result.metric` series without rewriting any
  stored record.
