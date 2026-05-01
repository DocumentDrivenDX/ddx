---
ddx:
  id: ADR-004
  depends_on:
    - FEAT-004
    - FEAT-006
    - FEAT-010
---
# ADR-004: Use Bead-Backed Collections for DDx Runtime Storage

## Status

Accepted

## Context

DDx now has multiple local storage needs beyond the primary work queue:

- active work items
- archived work items
- execution definitions and run indexes
- agent session indexes
- future runtime record families

The current bead implementation already provides useful properties:

- a portable bead schema with unknown-field preservation
- interchangeable local backends (`jsonl`, `bd`, `br`)
- locking and atomic rewrite behavior

The current implementation is also too tightly coupled to one specific file,
`.ddx/beads.jsonl`, and to one primary use case, the active tracker.

At the same time, execution runs and agent sessions can carry large payloads
such as prompts, responses, raw logs, and structured result blobs. Those
payloads should not be forced inline into one shared JSONL record.

This decision must stay aligned with the bd/br interchange contract rather than
silently drifting into a DDx-only fork.

Local compatibility anchors:

- [types.go](/home/erik/Projects/ddx/cli/internal/bead/types.go)
- [marshal.go](/home/erik/Projects/ddx/cli/internal/bead/marshal.go)
- [backend_external.go](/home/erik/Projects/ddx/cli/internal/bead/backend_external.go)
- [schema_compat_test.go](/home/erik/Projects/ddx/cli/internal/bead/schema_compat_test.go)
- [bead-record.schema.json](/home/erik/Projects/ddx/cli/internal/bead/schema/bead-record.schema.json)

Upstream reference points:

- `bd` upstream repository: <https://github.com/gastownhall/beads>
- upstream format directory: <https://github.com/gastownhall/beads/tree/main/format>
- upstream Dolt storage notes: <https://github.com/gastownhall/beads/blob/main/docs/DOLT.md>

DDx's external backend adapter currently treats both `bd` and `br` through the
same interchange surface:

- `list --json` for reads
- `import --from jsonl --replace -` for writes

That contract is the minimum compatibility surface this ADR must preserve.

## Decision

DDx will use the bead schema and backend model as the logical storage engine
for runtime records, but it will separate logical collections from one
specific physical file.

1. DDx storage is organized as **named bead-backed collections**.
2. The primary tracker is one collection, not the only collection.
3. JSONL maps a collection to a separate file under `.ddx/`.
4. Other backends (`bd`, `br`) must preserve the same logical collection
   boundary, but their physical storage layout is a backend concern rather than
   a DDx file-layout concern.
5. Large payloads are stored in **sidecar attachment files** referenced by the
   bead-schema record, not inline in the collection row.
6. The base bead envelope must retain bd/br-compatible field names and
   semantics for the shared fields DDx already interoperates on.

## Consequences

### Positive

- One reusable schema and backend family can serve work items, archives,
  execution indexes, and agent-session indexes.
- DDx can add new record families without inventing a new local storage model
  each time.
- Archived beads can move out of the active queue without changing the storage
  engine.
- Execution and agent records can keep lightweight searchable metadata in the
  bead collection while storing large bodies separately.

### Negative

- The current bead store API must be generalized from a fixed `beads.jsonl`
  target to a named collection abstraction.
- Collection-specific semantics must be documented clearly because not every
  bead-backed collection uses queue semantics such as `ready` or `blocked`.
- DDx must not assume that every backend exposes collections as separate files.

## Collection Model

Examples of logical collections:

- `beads` — active work queue
- `beads-archive` — archived work items
- `exec-runs` — execution run metadata records
- `agent-sessions` — agent session metadata records

Each record still uses the bead envelope:

- `id`
- `issue_type`
- `status`
- `title`
- `labels`
- `created_at`
- `updated_at`
- preserved unknown fields for domain-specific data

Shared field names remain aligned with bd/br interchange:

- `issue_type`, not `type`
- `owner`, not `assignee`
- `created_at` / `updated_at`, not `created` / `updated`
- `dependencies`, not `deps`

Collection-specific fields live in preserved extras. For example:

- execution runs may store `definition_id`, `artifact_ids`, `run_status`,
  `provenance`, and attachment references
- agent sessions may store `harness`, `model`, `tokens`, `correlation`, and
  attachment references

## Attachment Model

Large bodies are stored outside the collection row.

Examples:

- prompt text
- response text
- stdout or stderr logs
- structured result JSON

The bead-backed metadata record stores stable references to these payloads.

Attachment references are DDx-specific extensions carried in preserved extras.
They must not rename or redefine the shared bead-envelope fields that bd/br
already understand.

## Technical Guards

This ADR is only acceptable if the compatibility contract is enforced in-repo.

Required guards:

1. The base bead envelope is defined in
   [bead-record.schema.json](/home/erik/Projects/ddx/cli/internal/bead/schema/bead-record.schema.json).
2. Unknown fields remain allowed at the schema level so bd/br computed fields
   and DDx-specific extensions both round-trip.
3. [schema_compat_test.go](/home/erik/Projects/ddx/cli/internal/bead/schema_compat_test.go)
   must keep locking the field names and round-trip behavior against real bd
   export examples.
4. [marshal.go](/home/erik/Projects/ddx/cli/internal/bead/marshal.go) remains
   the canonical field-mapping implementation for DDx bead JSON.
5. Collection-specific runtime records may add their own schemas, but those
   schemas must layer on top of the base bead record rather than fork it.

If upstream bd/br changes the shared JSON field contract, DDx must update the
schema file, compatibility tests, and this ADR together.

## Not Chosen

### Separate bespoke storage formats per subsystem

Rejected because it multiplies local persistence models and weakens backend
reuse.

### Storing all execution and session payloads inline in collection rows

Rejected because prompts, responses, and logs can grow large and would make
shared collection rewrites fragile and expensive.

### Reusing only bead locking code while inventing a new record schema

Rejected because the bead schema is already the portable DDx record envelope
and should remain the common logical model.
