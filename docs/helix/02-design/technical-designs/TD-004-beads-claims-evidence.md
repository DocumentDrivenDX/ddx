---
ddx:
  id: TD-004
  depends_on:
    - FEAT-004
    - SD-004
---
# Technical Design: Bead Claims and Execution Evidence

## Purpose

This design closes the remaining gap between the bead feature spec and the
runtime contract: claims must support explicit assignees, and bead history must
support append-only execution evidence.

## Contract Summary

- `ddx bead update <id> --claim [--assignee NAME]`
- `ddx bead update <id> --unclaim`
- `ddx bead evidence add <id> ...`
- `ddx bead evidence list <id> [--json]`

The storage model uses the existing JSONL snapshot writer. The change does not
introduce a new file, database, or background service.

## Claim Resolution

Claiming a bead performs one atomic snapshot rewrite under the store lock.

1. Load the bead snapshot.
2. Resolve the assignee in this order:
   1. explicit CLI `--assignee`
   2. runtime caller identity
   3. `ddx`
3. Set `status=in_progress`.
4. Record `assignee`, `claimed-at`, and `claimed-pid`.
5. Rewrite the snapshot atomically.

Unclaiming performs the inverse mutation:

1. Load the bead snapshot.
2. Set `status=open`.
3. Clear `assignee`, `claimed-at`, and `claimed-pid`.
4. Rewrite the snapshot atomically.

Claim metadata is advisory. The store does not enforce exclusivity beyond the
serialized write path.

## Execution Evidence

Execution evidence is stored on each bead as an ordered `events` array in
`Extra["events"]`.

Recommended event schema:

```json
{
  "kind": "summary",
  "summary": "Completed the migration",
  "body": "Expanded details or multiline note",
  "actor": "alice",
  "created_at": "2026-04-04T15:00:00Z",
  "source": "ddx bead evidence add"
}
```

Rules:

- Append-only: prior entries are never mutated or removed.
- Stable order: entries remain in insertion order.
- Read-only consumers must see the full history.
- Queue derivation ignores evidence content.

## Migration

Existing beads without `Extra["events"]` continue to load normally.

- `notes` remains supported for backward compatibility.
- New evidence entries are appended to `events`.
- No automatic rewrite of old `notes` content is required.

## API Exposure

The MCP and HTTP bead payloads should expose the `events` metadata unchanged.
The server does not interpret the entries beyond preserving and returning
them.

## Verification Targets

- Claim/unclaim paths resolve assignee correctly.
- Evidence appends preserve order across concurrent writers.
- JSONL writes remain atomic and leave no partial records.
- `show --json` returns the full evidence history.
- `list`, `ready`, `blocked`, and `status` ignore evidence for queue logic.
