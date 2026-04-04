---
ddx:
  id: TD-006
  depends_on:
    - FEAT-006
    - SD-006
---
# Technical Design: Agent Session Capture and Inspection

## File Layout

For the JSONL backend, the session collection and its attachments live under
the configured `session_log_dir` (default `.ddx/agent-logs`) and map to:

```text
<session_log_dir>/
├── agent-sessions.jsonl
└── agent-sessions.d/
    └── <session-id>/
        ├── prompt.txt
        ├── response.txt
        ├── stdout.log
        ├── stderr.log
        └── correlation.json
```

Each row in `agent-sessions.jsonl` is one bead-backed session record. Older
legacy JSONL session rows remain valid inputs during migration.

Other backends preserve the same logical `agent-sessions` collection even if
they do not map it to JSONL files directly. Their physical storage layout is
owned by the backend implementation.

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
  "attachments": {
    "prompt": "agent-sessions.d/as-1a2b3c4d/prompt.txt",
    "response": "agent-sessions.d/as-1a2b3c4d/response.txt",
    "stdout": "agent-sessions.d/as-1a2b3c4d/stdout.log",
    "stderr": "agent-sessions.d/as-1a2b3c4d/stderr.log",
    "correlation": "agent-sessions.d/as-1a2b3c4d/correlation.json"
  },
  "correlation": {
    "bead_id": "ddx-bd674042",
    "workflow": "helix"
  }
}
```

## Write Mechanism

Session writes must be safe under concurrent agents.

1. Resolve the named `agent-sessions` collection and attachment root.
2. Acquire the collection lock.
3. Write any large bodies into a temporary attachment directory.
4. Publish the attachment directory into its final location.
5. Create one bead-backed session row containing attachment references.
6. Release the lock.

The session collection is append-only for new session records. No existing
session row is rewritten during normal operation.

## Compatibility Algorithm

When reading a session:

1. Load the bead-backed session row from the collection.
2. Fall back to legacy JSONL rows when reading historical session data.
3. Accept missing prompt, response, stderr, or correlation data.
4. Preserve unknown fields on round-trip where possible.
5. Render a placeholder only when a caller asks for a field that the stored session did not capture.

## Inspection Algorithm

- `ddx agent log` sorts session rows by timestamp descending and renders metadata first so operators can scan recent activity quickly.
- `ddx agent log <session-id>` finds the exact session row and emits the full stored record, including prompt and response bodies.
- Server/API detail endpoints mirror the same reader to avoid drift.

## Redaction Policy

Redaction occurs before the row is written.

- If a configured redaction rule matches a prompt or response substring, DDx replaces the matched value with a deterministic placeholder before writing the session row and attachments.
- The inspection path does not try to reconstruct redacted data.
- The record should keep enough metadata to explain that redaction occurred.

## Failure Modes

- Missing log directory: create it lazily on first write.
- Corrupt legacy row: skip the row during listing, but surface an error in detail mode when the requested session cannot be parsed.
- Partial write: prevented by the file lock plus temp attachment publication
  before row creation.
