# DDx Exec Consumer Integration Guide

This guide explains how HELIX and other tools integrate with `ddx exec`. It covers
registering execution definitions, invoking runs, querying history, and interpreting
structured result schemas — without requiring knowledge of DDx internals.

## Overview

`ddx exec` provides a generic execution substrate: you register a definition that
links a command (or agent invocation) to one or more artifact IDs, run it, and query
the resulting immutable history. Each run captures raw logs, a structured result
payload, and provenance metadata.

## Storage Layout

Executions are stored repo-locally under `.ddx/`:

```
.ddx/
├── exec-definitions.jsonl    # execution definitions
├── exec-runs.jsonl           # immutable run records
└── exec-runs.d/
    └── <run-id>/
        ├── result.json       # structured result payload
        ├── stdout.log        # raw stdout
        └── stderr.log        # raw stderr
```

No hosted service or database is required.

This guide describes the generic `ddx exec` substrate only. It does not govern
the tracked `execute-bead` attempt bundles written under
`.ddx/executions/<attempt-id>/`, which are a separate artifact class used for
implementation replay and commit provenance.

## Registering an Execution Definition

An execution definition is a JSON record stored in `.ddx/exec-definitions.jsonl`.
Each definition links a runnable command to one or more artifact IDs.

**Minimum required fields:**

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
      "command": ["go", "test", "-run", "BenchmarkStartup", "./..."],
      "cwd": ".",
      "timeout_ms": 30000
    },
    "active": true,
    "created_at": "2026-04-04T15:00:00Z"
  }
}
```

Rules:
- `id` must be stable and unique within the repo
- `artifact_ids` must reference artifact IDs in the DDx document graph
- `executor.kind` must be `"command"` or `"agent"`
- `status: "open"` means active; `status: "closed"` means retired but preserved
- Append the JSON object as a single line to `.ddx/exec-definitions.jsonl`

**Optional result projection** (for metric-shaped output):

```json
"result": {
  "metric": {
    "unit": "ms",
    "value_path": "$.duration_ms"
  }
}
```

**Optional evaluation rules:**

```json
"evaluation": {
  "comparison": "lower-is-better",
  "thresholds": {
    "warn_ms": 100,
    "ratchet_ms": 200
  }
}
```

**Validate the definition before use:**

```bash
ddx exec validate exec-metric-startup-time@1
```

This checks the definition schema, resolves each linked artifact ID, and verifies
the executor configuration.

## Listing and Inspecting Definitions

```bash
# List all definitions
ddx exec list

# List definitions linked to a specific artifact
ddx exec list --artifact MET-001

# Show one definition as JSON
ddx exec show exec-metric-startup-time@1 --json
```

JSON output from `ddx exec list --json`:

```json
[
  {
    "id": "exec-metric-startup-time@1",
    "artifact_ids": ["MET-001"],
    "executor": { "kind": "command", "command": ["go", "test", "..."] },
    "active": true,
    "created_at": "2026-04-04T15:00:00Z"
  }
]
```

## Invoking a Run

```bash
ddx exec run <definition-id>
```

Example:

```bash
ddx exec run exec-metric-startup-time@1
```

Default output (one line):

```
exec-metric-startup-time@2026-04-04T15:01:00Z-1  success  0
```

Fields: `run_id  status  exit_code`

**JSON output:**

```bash
ddx exec run exec-metric-startup-time@1 --json
```

Returns a `RunRecord` (see [Run Record Schema](#run-record-schema) below).

Each invocation appends a new immutable row to `.ddx/exec-runs.jsonl`. Prior
runs are never overwritten.

## Querying Run History by Artifact ID

```bash
# All runs linked to MET-001
ddx exec history --artifact MET-001

# All runs for a specific definition
ddx exec history --definition exec-metric-startup-time@1

# Both filters combined
ddx exec history --artifact MET-001 --definition exec-metric-startup-time@1

# JSON output for scripting
ddx exec history --artifact MET-001 --json
```

Default output (one line per run):

```
exec-metric-startup-time@2026-04-04T15:01:00Z-1  exec-metric-startup-time@1  success  0
```

Fields: `run_id  definition_id  status  exit_code`

Runs are ordered by `started_at` ascending, then by `run_id`.

## Inspecting a Specific Run

```bash
# Raw logs
ddx exec log <run-id>

# Raw logs as JSON (stdout and stderr keys)
ddx exec log <run-id> --json

# Structured result payload
ddx exec result <run-id>
```

## Run Record Schema

`ddx exec run --json` and `ddx exec history --json` return `RunRecord` objects:

```json
{
  "run_id": "exec-metric-startup-time@2026-04-04T15:01:00Z-1",
  "definition_id": "exec-metric-startup-time@1",
  "artifact_ids": ["MET-001"],
  "started_at": "2026-04-04T15:01:00Z",
  "finished_at": "2026-04-04T15:01:01Z",
  "status": "success",
  "exit_code": 0,
  "agent_session_id": "",
  "attachments": {
    "stdout": "exec-runs.d/exec-metric-startup-time@2026-04-04T15:01:00Z-1/stdout.log",
    "stderr": "exec-runs.d/exec-metric-startup-time@2026-04-04T15:01:00Z-1/stderr.log",
    "result": "exec-runs.d/exec-metric-startup-time@2026-04-04T15:01:00Z-1/result.json"
  },
  "provenance": {
    "actor": "erik",
    "host": "dev-host",
    "git_rev": "abc123",
    "ddx_version": "0.1.0"
  },
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
        "baseline": 15.0,
        "delta": -0.4,
        "direction": "improved"
      }
    }
  }
}
```

### `status` values

| Value | Meaning |
|-------|---------|
| `success` | Command exited 0 and result parsed successfully |
| `failed` | Command exited non-zero |
| `timed_out` | Execution exceeded `timeout_ms` |
| `errored` | Pre-invocation setup error (bad definition, missing artifact) |

### Attachment paths

`attachments` values are relative, repo-local paths from `.ddx/`. To read the
result payload directly:

```bash
cat .ddx/exec-runs.d/<run-id>/result.json
```

Or use the CLI:

```bash
ddx exec result <run-id>
```

## Structured Result Schemas

### Command executor (no result spec)

When no `result` block is specified in the definition, `ddx exec result` returns:

```json
{
  "stdout": "...",
  "stderr": "...",
  "value": 0,
  "parsed": false
}
```

### Metric projection

When the definition includes a `result.metric` block, DDx extracts the numeric
value from the command output using `value_path` (a JSONPath expression):

```json
{
  "metric": {
    "artifact_id": "MET-001",
    "definition_id": "exec-metric-startup-time@1",
    "observed_at": "2026-04-04T15:01:00Z",
    "status": "pass",
    "value": 14.6,
    "unit": "ms",
    "samples": [14.6],
    "comparison": {
      "baseline": 15.0,
      "delta": -0.4,
      "direction": "improved"
    }
  }
}
```

`comparison` is populated when `evaluation.comparison` is set and a prior run
exists for the same artifact.

### Agent executor

When `executor.kind` is `"agent"`, the run record includes `agent_session_id`
linking back to the DDx agent session. The same attachment layout applies; the
agent's raw output lands in `stdout.log`.

## Example: HELIX Integration Workflow

A typical HELIX acceptance-check workflow:

```bash
# 1. Validate the definition before invoking
ddx exec validate ddx-ac-check-feat010@1

# 2. Run the check
RUN_JSON=$(ddx exec run ddx-ac-check-feat010@1 --json)

# 3. Read outcome
STATUS=$(echo "$RUN_JSON" | jq -r '.status')
RUN_ID=$(echo "$RUN_JSON" | jq -r '.run_id')

# 4. Check pass/fail
if [ "$STATUS" != "success" ]; then
  echo "Check failed: $STATUS"
  ddx exec log "$RUN_ID"
  exit 1
fi

# 5. Query history to confirm the criterion has been stable
ddx exec history --artifact AC-042 --json | jq '[.[] | .status] | unique'
```

## Legacy Compatibility

If `.ddx/exec/definitions/*.json` or `.ddx/exec/runs/<run-id>/` directories exist
from an earlier DDx version, DDx reads them as a fallback when no bead-backed
collection record is found. New writes always target the bead-backed collections.
No migration action is required; old data remains readable.
