---
ddx:
  id: FEAT-005
  depends_on:
    - helix.prd
    - FEAT-001
    - FEAT-002
---
# Feature Specification: FEAT-005 - Execution-Backed Skill Output

**Feature ID**: FEAT-005
**Status**: Draft
**Priority**: P1
**Owner**: HELIX maintainers

## Overview

HELIX skills produce structured output — evolution reports, alignment audits,
review findings, design documents, backfill reports, check decisions, and build
summaries. Today these outputs are either printed to stdout/stderr, written to
ad-hoc files on disk, or embedded in conversation context where they are lost
when the session ends.

This feature makes every HELIX skill invocation a DDx execution run. The skill
output — structured report, raw logs, and terminal status — is captured as an
immutable DDx execution record linked to the governing artifact. This replaces
ad-hoc file output with a queryable, durable, artifact-linked evidence trail.

## Problem Statement

- **Current situation**: HELIX skill outputs are ephemeral. An evolution report
  is printed to the conversation. A review finding is in a session transcript.
  A check decision is a trailer block in stderr. Nothing links these outputs
  to the artifact they evaluated or the bead they operated on.
- **Pain points**:
  1. No durable record of what HELIX did, when, or what it found.
  2. No way to query "show me all review findings for SD-001" or "what did
     the last alignment audit find?"
  3. Ad-hoc output files (when they exist) have no schema, no artifact
     linkage, and no retention policy.
  4. `helix status` cannot report on the history of actions taken — only
     the current run-controller state.
  5. Token usage and timing data are captured per-cycle in the run loop but
     not per-skill-invocation.
- **Desired outcome**: Every HELIX skill invocation creates a DDx execution
  run record. The record captures raw output, structured result data, terminal
  status, timing, and is linked to the relevant artifact ID. Tools can query
  execution history by artifact, skill, status, or recency.

## Design

### Execution definition per skill

Each HELIX skill registers a DDx execution definition. The definition declares:
- **Definition ID**: `helix:<skill-name>` (e.g., `helix:evolve`, `helix:review`)
- **Linked artifact IDs**: Resolved at invocation time from the skill's
  `--scope`, `--artifact`, or `--spec-id` argument
- **Executor kind**: `command` (delegates to `helix <command>`) or `agent`
  (when the skill dispatches via `ddx agent run`)
- **Result schema**: Skill-specific structured output (e.g., evolution report
  has `artifacts_updated`, `issues_created`, `conflicts` fields)

### Run capture flow

```
User invokes skill (CLI or agent)
  → helix CLI resolves execution definition
  → ddx exec run helix:<skill> --artifact <id>
    → captures stdout/stderr as raw logs
    → parses structured trailer/output as result payload
    → records timing, terminal status, provenance
    → persists immutable run record
  → skill output displayed to user as before
```

The user experience does not change — output is still printed. But a durable
record is also persisted.

### Structured output by skill

| Skill | Structured result fields |
|-------|------------------------|
| `evolve` | `artifacts_updated`, `issues_created`, `conflicts`, `evolution_status` |
| `review` | `findings`, `severity_counts`, `files_reviewed`, `verdict` |
| `align` | `artifacts_audited`, `drift_detected`, `recommendations` |
| `check` | `next_action`, `queue_state`, `ready_count`, `blocked_count` |
| `build` | `issue_id`, `files_changed`, `tests_passed`, `terminal_status` |
| `design` | `artifact_id`, `iterations`, `convergence_status` |
| `polish` | `issues_refined`, `acceptance_criteria_added` |
| `triage` | `issues_created`, `spec_ids_referenced` |
| `frame` | `artifacts_created`, `artifacts_updated` |
| `run` | `cycles_completed`, `issues_closed`, `stop_reason`, `token_usage` |
| `status` | `run_state`, `claimed_issue`, `queue_health` |

### Integration with existing observability

The `helix run` loop already captures per-cycle timing and token usage. With
execution-backed output:
- Each `build` dispatch within a run creates its own execution run record.
- Each `review` dispatch creates its own execution run record.
- The `run` skill's own execution record summarizes the full session.
- `helix status` can query execution history to report recent actions.

### Dependency on DDx FEAT-010

This feature depends on DDx FEAT-010 (Executions) being implemented. Until
`ddx exec` is available:
- HELIX should define the execution definitions in a DDx-compatible format
  (ready to register when `ddx exec` ships).
- HELIX can use a lightweight shim that writes run records to
  `.ddx/exec/runs/` in the expected format, migrating to `ddx exec` once
  available.
- Alternatively, HELIX can defer execution capture entirely until DDx FEAT-010
  ships, provided DDx documents the consumer integration contract clearly
  enough for HELIX to plan against.

## Requirements

### Functional Requirements

1. Every HELIX skill invocation must produce a DDx execution run record when
   execution capture is enabled.
2. Each run record must include: raw logs, structured result payload, terminal
   status, timing, and linked artifact ID(s).
3. Execution definitions must be registered per HELIX skill with stable
   definition IDs (`helix:<skill-name>`).
4. Structured result schemas must be skill-specific and machine-parseable.
5. `helix status` must be able to query recent execution history to report
   what actions were taken and their outcomes.
6. The run loop (`helix run`) must create per-dispatch execution records for
   each sub-action it invokes (build, review, align, etc.) in addition to
   its own session-level record.
7. Execution capture must not block skill output — the user sees output
   immediately regardless of whether the run record persists successfully.
8. When DDx FEAT-010 is not available, HELIX must either use a compatible
   shim or skip execution capture gracefully.

### Non-Functional Requirements

- **Durability**: Run records must survive session termination.
- **Performance**: Execution capture overhead must be negligible relative to
  the skill invocation itself.
- **Queryability**: Run records must be inspectable by artifact ID, skill
  name, status, and recency.
- **Immutability**: Once persisted, a run record is never modified.

## User Stories

### US-001: Query skill history for an artifact [FEAT-005]
**As a** HELIX operator
**I want** to see all HELIX actions taken against a specific artifact
**So that** I can understand the evolution of my design over time

**Acceptance Criteria:**
- [ ] Given multiple skills have been invoked against SD-001, when I query
  execution history for SD-001, then I see an ordered list of runs with
  timestamps, skill names, and terminal statuses.
- [ ] Given a specific run, when I inspect it, then I see the raw logs and
  structured result payload.

### US-002: Durable evolution report [FEAT-005]
**As a** HELIX operator
**I want** evolution reports to be durably stored, not just printed
**So that** I can review what changed and when without searching chat history

**Acceptance Criteria:**
- [ ] Given `helix evolve` completes, when I query execution history, then
  the evolution report is available as a structured run record.
- [ ] Given the run record exists, when I inspect its result, then I see
  `artifacts_updated`, `issues_created`, and `conflicts` fields.

### US-003: Run loop observability [FEAT-005]
**As a** HELIX operator monitoring a background run
**I want** `helix status` to show recent actions taken by the run loop
**So that** I can understand what happened without reading raw logs

**Acceptance Criteria:**
- [ ] Given a `helix run` session completed 3 build cycles and 1 review,
  when I run `helix status`, then it shows the recent execution history
  with per-cycle outcomes.

## Edge Cases and Error Handling

- **DDx exec not available**: Skills execute normally but skip execution
  capture. A warning is logged once per session.
- **Execution record write fails**: The skill completes normally. The failure
  is logged but does not affect the skill's exit status.
- **Very large output**: Raw logs are stored as attachments (per DDx FEAT-010
  attachment-backed evidence) rather than inline in the run record.
- **Concurrent runs**: Multiple `helix run` instances each create independent
  execution records. No cross-session deduplication.

## Success Metrics

- Every HELIX skill invocation that produces structured output also produces
  a DDx execution run record.
- `helix status` reports recent execution history alongside run-controller
  state.
- Operators can query "what happened to artifact X" through execution history
  rather than searching chat transcripts or log files.

## Constraints and Assumptions

- DDx FEAT-010 provides the execution record model and storage. HELIX is a
  consumer, not an implementer.
- Execution definitions are HELIX-owned but stored in DDx format.
- The structured result schemas are HELIX-specific and not constrained by DDx
  beyond being valid JSON.

## Dependencies

- **DDx FEAT-010**: Execution definitions and runs (external, in-progress)
- **DDx FEAT-006**: Agent service (for agent-backed executor kind)
- **FEAT-001**: Supervisory control (run loop dispatches sub-actions)
- **FEAT-002**: CLI (execution capture hooks into CLI dispatch)

## Out of Scope

- Visual dashboards for execution history
- Automatic execution definition generation from skill metadata
- Cross-repo execution history aggregation
- Execution-based CI/CD gating

## Open Questions

- Should HELIX register execution definitions at install time (static) or
  resolve them at invocation time (dynamic)? Static is simpler but requires
  re-registration when skills are added.
- What is the right granularity for run-loop execution records? One per
  `helix run` session, one per dispatched sub-action, or both?
- Should the shim approach (write compatible records before `ddx exec` ships)
  be pursued, or should HELIX wait for DDx FEAT-010?
