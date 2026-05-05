---
ddx:
  id: TD-034
  depends_on:
    - ADR-004
    - FEAT-004
    - TD-027
    - TD-030
  status: draft
---
# Technical Design: bd / Dolt Fallback Bead Backend

## Status

Draft. This note documents the contingency path if Axon adoption stalls
before `bead_tracker.backend: axon` becomes the default backend. It does
not replace TD-030; it explains how to wire bd as the bead backend while
keeping the existing JSONL interchange contract intact.

## Why This Note Exists

FEAT-004 already names `bd` as a supported backend and says DDx shells
out to the external binary for `bd` and `br`
([FEAT-004-beads.md:104-112](../../01-frame/features/FEAT-004-beads.md#L104-L112)).
TD-030, by contrast, is Axon-first and treats `ddx bead export` as the
logical backup format rather than the storage backend
([TD-030-axon-bead-backend.md:14-18](TD-030-axon-bead-backend.md#L14-L18),
[TD-030-axon-bead-backend.md:205-211](TD-030-axon-bead-backend.md#L205-L211)).
The code already has the external-backend seam
([backend_external.go:10-20](../../../../cli/internal/bead/backend_external.go#L10-L20),
[backend_external.go:41-45](../../../../cli/internal/bead/backend_external.go#L41-L45),
[backend_external.go:115-148](../../../../cli/internal/bead/backend_external.go#L115-L148))
and the named-collection abstraction in TD-027
([TD-027-bead-collection-abstraction.md:69-102](TD-027-bead-collection-abstraction.md#L69-L102)),
but there is no single design artifact that tells an operator or
implementer how to fall back to bd if Axon is not ready.

## Fallback Contract

- bd remains an external dependency; DDx does not vendor or reimplement
  Dolt.
- `ddx bead import` and `ddx bead export` stay the interchange boundary.
- The fallback must preserve queue semantics, claim and unclaim
  metadata, and the existing `ddx bead list`, `show`, `ready`, and
  `blocked` behaviors.
- The fallback is per collection. The default `beads` collection keeps
  the direct bd path; non-default collections can route through the
  JSONL adapter when bd has no native scoping.

## Interface Adapter

The adapter is the existing `ExternalBackend` seam in
`cli/internal/bead/backend_external.go`.

- Resolve the requested collection first.
- Use direct bd invocation for the default collection so the bd/br
  interchange contract remains unchanged.
- Use the JSONL-backed fallback for collections bd cannot scope
  natively.
- Keep the public surface limited to `List`, `Get`, `WriteAll`, and
  `WithLock` plus collection resolution; do not introduce a second
  backend API just for the fallback.

This matches the current tests around the seam:
`TestExternalBackendCarriesLogicalCollectionName`,
`TestExternalBackendOpensBeadsArchiveWithFallback`, and
`TestExternalBackendDefaultCollectionHasNoFallback`.

## JSONL Interchange

The fallback does not define a new wire format. It keeps the same JSONL
round-trip already used by DDx and covered by:

- `TestBdSchemaCompatibility`
- `TestBdRoundTrip`
- `TestDdxBeadFieldNames`

Operationally:

- Export serializes bead records as bd-compatible JSONL.
- Import consumes the same JSONL and preserves unknown fields.
- The JSONL shape remains the compatibility contract between DDx and
  bd, just as it already is for `ddx bead import/export`.

## Claim / Lock Translation

Claiming a bead remains a DDx record mutation, not a bd-specific
primitive.

- `claim` writes `status=in_progress`, `owner`, `claimed-at`, and
  `claimed-pid`.
- `unclaim` clears only the claim metadata and must not reopen a closed
  bead.
- Backend locking protects the read-modify-write cycle, while the claim
  state stays in the bead record itself.
- The logical claim rules stay portable across backends, so queue
  derivation continues to work the same way whether the store is JSONL,
  bd, or a fallback path.

The existing claim tests remain the behavioral anchors:
`TestUnclaimDoesNotReopenClosedBead`, `TestHeartbeatReclaimStaleInProgressBead`,
`TestHeartbeatKeepsActiveClaimAlive`, and
`TestAtomicClaimUnderContention`.

## Tradeoffs

- External dependency: operators must install and maintain bd and its
  Dolt runtime.
- Git-for-data semantics: bd inherits Dolt commit and branch behavior,
  which is useful for collaboration but heavier than local JSONL.
- Less native control: DDx cannot assume bd exposes every named
  collection or claim primitive natively.
- Better contingency: the project keeps moving even if Axon adoption
  slips.

## Comparison With TD-030

- TD-030 optimizes for a networked Axon service, GraphQL transport, and
  multi-machine access.
- This note optimizes for a local external backend with minimal DDx
  change.
- TD-030 treats `ddx bead export` as the logical backup path; this note
  treats JSONL as the operational interchange path between DDx and bd.
- Both designs preserve the same bead envelope and the same
  `ddx bead import/export` contract.

## Non-Scope

- No code changes are required by this document.
- This is not a decision to make bd the permanent primary backend.
- This does not change the Axon TD or the current JSONL backend
  implementation.
