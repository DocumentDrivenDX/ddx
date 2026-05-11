---
ddx:
  id: TD-004
  depends_on:
    - FEAT-004
    - SD-004
    - TD-027
---
# Technical Design: Execution Evidence

> **Scope note (2026-05-11):** This TD was previously titled "Bead Claims and Execution Evidence" and bundled two concerns. Claim resolution semantics moved to TD-027 §11 (Claim Semantics) as part of the bead-architecture consolidation; this document now focuses solely on the execution-evidence subsystem.

## Purpose

This design specifies how bead history accumulates append-only execution evidence — the artifact log attached to each bead by drain attempts, operators, and post-attempt review. Evidence is keyed by bead and is the durable record of "what happened" against a work item, distinct from the bead's lifecycle status.

## Contract Summary

- `ddx bead evidence add <id> ...` — append an evidence record
- `ddx bead evidence list <id> [--json]` — read evidence history

The storage model uses the bead's `Extra["events"]` field on the JSONL snapshot writer (or the equivalent collection in non-JSONL backends per TD-027). The change does not introduce a new file, database, or background service.

## Execution Evidence

Execution evidence is stored on each bead as an ordered `events` array in `Extra["events"]`.

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

The canonical `kind` vocabulary used across the drain loop and review system is enumerated in TD-027 §13 (Event Vocabulary). New `kind` values require updating that section.

Rules:

- **Append-only**: prior entries are never mutated or removed (see TD-027 §11.2 invariant 7).
- **Stable order**: entries remain in insertion order; timestamps are monotonic per bead (TD-027 §11.2 invariant 8).
- **Read-only consumers must see the full history.**
- **Queue derivation ignores evidence content.** Queue buckets are computed from status, dependency edges, and claim metadata only — never from event content (see TD-027 §2.1).

## Migration

Existing beads without `Extra["events"]` continue to load normally.

- `notes` remains supported for backward compatibility.
- New evidence entries are appended to `events`.
- No automatic rewrite of old `notes` content is required.

## API Exposure

The MCP and HTTP bead payloads should expose the `events` metadata unchanged. The server does not interpret the entries beyond preserving and returning them.

## Verification Targets

- Evidence appends preserve order across concurrent writers.
- JSONL writes remain atomic and leave no partial records.
- `show --json` returns the full evidence history.
- `list`, `ready`, `blocked`, and `status` ignore evidence for queue logic.

## Relationship to TD-027

TD-004 owns the evidence-tracking subsystem at a focused, narrow scope:

- The shape of an evidence record.
- The append-only contract.
- The CLI surface (`evidence add` / `evidence list`).
- API exposure (MCP, HTTP).

TD-027 owns everything else about beads:

- Claim semantics (the previous "Bead Claims" half of TD-004 — now §11 of TD-027).
- Status enum, transitions, queue buckets, naming roles.
- Event vocabulary (the controlled list of `kind` values).
- Outcome → state mapping for drain-loop outcomes.
- Collection registry, archival, attachments, migration, read-path semantics.
- Storage interface (`Backend`, sub-interfaces, `Operation` pattern, module boundary).

A consumer needing the full bead contract reads TD-027. A consumer specifically implementing or auditing the evidence subsystem reads TD-004 alongside TD-027 §12 (Event Vocabulary).
