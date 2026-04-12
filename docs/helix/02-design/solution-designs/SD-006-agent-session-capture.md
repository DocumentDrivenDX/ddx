---
ddx:
  id: SD-006
  depends_on:
    - FEAT-006
---
# Solution Design: Agent Session Capture and Inspection

## Purpose

This design makes agent sessions inspectable end-to-end. The existing agent
runtime already tracks metadata; this tranche adds the prompt and response
bodies, correlation metadata, and compatibility rules needed for full session
review.

## Scope

- Capture full prompt and response bodies for agent invocations.
- Preserve harness, model, token, duration, and exit-code metadata.
- Attach workflow correlation metadata without coupling to HELIX internals.
- Keep old metadata-only session rows readable.
- Define the CLI and API inspection behavior.
- Define redaction and retention policy knobs.

## Storage Boundary

Agent session evidence is stored as bead-backed session records in a dedicated
collection, not in the primary active-work collection.

- A session may reference workflow identifiers such as `bead_id`, but the
  canonical prompt, response, and log bodies live in sidecar files referenced
  by the session record.
- The session metadata row uses the bead envelope plus agent-session fields in
  preserved extras.
- Bead evidence in the primary work queue may summarize a session, but it is
  not the authoritative session store.

## Data Model

### Session record

Session records are bead-backed metadata rows whose attachment fields point at
stored prompt, response, and log bodies.

Required fields:
- `id`
- `timestamp`
- `harness`
- `model`
- `prompt_len`
- `tokens`
- `duration_ms`
- `exit_code`
- references to stored prompt and response bodies

Optional fields:
- `error`
- references to stored stderr and stdout bodies
- `correlation`
- `redaction`
- `prompt_source`

### Correlation metadata

Correlation metadata is a free-form object used by workflow tools to attach
domain identifiers such as `bead_id`, `doc_id`, `workflow`, or `request_id`.
DDx stores the data but does not interpret workflow-specific keys.

## Inspection UX

- `ddx agent log` lists sessions in reverse chronological order.
- `ddx agent log <session-id>` renders the full stored prompt and response.
- The server's agent-session endpoints mirror the same data shape.

## Compatibility

- Existing metadata-only JSONL rows remain valid legacy session entries.
- Missing prompt, response, stderr, or correlation data is treated as absent
  evidence rather than a hard failure.
- The inspection path must not fail when it encounters older log rows during
  migration.

## Redaction and Retention

- Default retention is local and indefinite.
- Redaction is opt-in and config-driven.
- Redaction is applied before persistence so list/detail APIs never expose
  masked content accidentally.

## Behavior

1. The runner resolves the prompt and response for the invocation.
2. The runner records execution metadata, prompt length, token count, and duration.
3. The session writer stores one session row in the dedicated collection and
   writes any large bodies as sidecar files.
4. Session detail reads the stored row and referenced bodies.

All writes remain repository-local and read-only consumers do not mutate
session state.
