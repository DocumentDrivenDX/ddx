---
ddx:
  id: TD-006
  depends_on:
    - FEAT-006
---
# Technical Design: Agent Invocation Activity and Native Session References

## File Layout

For the JSONL backend, DDx invocation activity records and any optional
attachments live under the configured `session_log_dir` (default
`.ddx/agent-logs`) and map to:

```text
<session_log_dir>/
├── agent-sessions.jsonl
└── agent-sessions.d/
    └── <session-id>/
        ├── correlation.json
        ├── prompt.txt        # optional, only when DDx owns the body
        ├── response.txt      # optional, only when DDx owns the body
        ├── stdout.log        # optional
        └── stderr.log        # optional
```

Each row in `agent-sessions.jsonl` is one DDx invocation activity record.
Older legacy JSONL session rows remain valid inputs during migration.

## Record Format

```json
{
  "id": "as-1a2b3c4d",
  "issue_type": "agent_session",
  "status": "closed",
  "title": "codex session 2026-04-04T15:10:00Z",
  "timestamp": "2026-04-04T15:10:00Z",
  "harness": "codex",
  "model": "o3-mini",
  "prompt_len": 128,
  "tokens": 1234,
  "duration_ms": 842,
  "exit_code": 0,
  "native": {
    "owner": "codex",
    "session_id": "019d73da-5dab-7292-95a3-aa3200f3ed89",
    "log_ref": "/home/user/.codex/sessions/2026/04/04/rollout-...jsonl"
  },
  "attachments": {
    "correlation": "agent-sessions.d/as-1a2b3c4d/correlation.json"
  },
  "correlation": {
    "bead_id": "ddx-bd674042",
    "workflow": "helix"
  }
}
```

## Write Mechanism

Invocation activity writes must be safe under concurrent agents.

1. Resolve the named `agent-sessions` collection and attachment root.
2. Acquire the collection lock.
3. Write any optional DDx-owned large bodies into a temporary attachment
   directory.
4. Publish the attachment directory into its final location.
5. Create one bead-backed activity row containing metadata plus native session,
   trace, or attachment references.
6. Release the lock.

The activity collection is append-only for new records. No existing row is
rewritten during normal operation.

## Compatibility Algorithm

When reading an invocation:

1. Load the bead-backed activity row from the collection.
2. Fall back to legacy JSONL rows when reading historical session data.
3. Accept missing prompt, response, stderr, or correlation data.
4. Accept missing native session or trace references.
5. Preserve unknown fields on round-trip where possible.
6. Render a placeholder only when a caller asks for a field that the stored
   record did not capture.

## Inspection Algorithm

- `ddx agent log` sorts activity rows by timestamp descending and renders
  metadata first so operators can scan recent activity quickly.
- `ddx agent log <session-id>` finds the exact activity row and emits the
  stored record, including native session or trace references and any optional
  DDx-owned bodies.
- Server/API detail endpoints mirror the same reader to avoid drift.

## Redaction Policy

Redaction occurs before optional DDx-owned bodies are written.

- If a configured redaction rule matches a prompt or response substring, DDx
  replaces the matched value with a deterministic placeholder before writing
  the activity row and any DDx-owned attachments.
- The inspection path does not try to reconstruct redacted data.
- The record should keep enough metadata to explain that redaction occurred.

## Failure Modes

- Missing log directory: create it lazily on first write.
- Corrupt legacy row: skip the row during listing, but surface an error in
  detail mode when the requested session cannot be parsed.
- Missing native log reference: still return the DDx activity metadata and note
  that the native source is unavailable.
- Partial write: prevented by the file lock plus temp attachment publication
  before row creation.
