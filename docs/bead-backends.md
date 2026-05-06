# Bead Backend Fallback Path

This document describes the bd/DoltDB fallback path for bead storage if the axon rollout stalls or is deferred. It is the practical wiring guide for preserving the `Backend` contract introduced in `ddx-bbdd7564` while contrasting that path with the Axon backend design in [TD-030](helix/02-design/technical-designs/TD-030-axon-bead-backend.md).

## Contract To Preserve

The high-level contract is `cli/internal/bead/backend.go:17-67`.

- `Backend` is the interface callers should target.
- It includes CRUD, claim, list/ready/blocked, dependency operations, event append, archive split, and JSONL import/export.
- `RawBackend` remains the low-level read/write/lock primitive that concrete storage adapters implement underneath `Store`.
- Claim and lock are split on purpose: `Claim` and `Unclaim` mutate bead records, while `WithLock` is the serialization boundary that protects the read-modify-write cycle.

The important distinction is:

- `Backend` is the caller-facing API.
- `RawBackend` is the storage engine boundary.

That separation is what lets DDx keep JSONL as a safe default while swapping in bd, br, or axon behavior without changing call sites.

## Where The bd Path Lives

The bd/br path is wired today through `cli/internal/bead/store.go:88-137` and `cli/internal/bead/backend_external.go:10-149`.

- `NewStore` reads `DDX_BEAD_BACKEND`, then `.ddx/config.yaml`, then falls back to `jsonl`.
- When the backend type is `bd` or `br`, `Store` constructs `ExternalBackend`.
- `ExternalBackend` is the adapter boundary: it shells out to the external binary and uses JSONL for interchange.
- Non-default collections can fall back to `JSONLBackend` when the external tool cannot serve them directly.

If a future implementation wants a more explicit bd adapter, it should live beside these files in `cli/internal/bead/`, for example as `backend_bd.go`, but it should still satisfy `RawBackend` and preserve the same `Backend` surface.

## Contrast With Axon

The axon path is defined in `cli/internal/bead/axon_backend.go:15-105` and selected in `cli/internal/bead/store.go:127-137`.

- `BackendAxon` is a separate backend type.
- It is selected by `bead.backend: axon` or `DDX_BEAD_BACKEND=axon`.
- The current implementation is an in-process emulation that persists two JSONL collections under `.ddx/axon/`.

That makes axon a feature-flagged, repository-local implementation path. The bd fallback is different:

- bd is an external dependency.
- bd is selected by backend configuration rather than an experiment flag.
- bd uses the same JSONL interchange contract for import/export.
- bd inherits DoltDB's git-for-data model, so the storage semantics are repository-backed and branch/commit oriented rather than purely local files.

## Future Wiring Steps For A bd Fallback

If axon adoption stalls and bd becomes the preferred backend, a future implementer should follow these steps:

1. Keep `cli/internal/bead/backend.go:17-67` unchanged so callers continue to program against `Backend`.
2. Keep `Store` as the selector in `cli/internal/bead/store.go:88-137`; do not let command packages reach into storage directly.
3. Route `bead.backend: bd` or `DDX_BEAD_BACKEND=bd` to the bd adapter in `NewStore`.
4. Implement the bd adapter in `cli/internal/bead/` as a `RawBackend`-compatible type, or keep reusing `ExternalBackend` if the shell-out contract remains sufficient.
5. Preserve the JSONL interchange path by keeping `Import(source, filePath)` and `ExportTo(w io.Writer)` on the `Backend` interface.
6. Preserve the JSONL fallback for collections that bd cannot serve natively, using the same pattern currently documented in `backend_external.go`.
7. Keep the chaos/conformance suite pointed at the `Backend` interface so bd and JSONL exercise the same contract.

## Tradeoffs

The bd path is attractive because it aligns with the upstream bead-format ecosystem, but it carries an external dependency:

- the `bd` binary must be installed and available in `PATH`
- the fallback depends on the bd codebase and its DoltDB runtime, not just on DDx's repository
- import/export remains a shell-out boundary
- operator environments need a clear fallback story when `bd` is missing

That is the main difference from axon in this repository:

- axon is self-contained inside DDx but is still experimental
- bd is stable as a storage option but depends on an external tool and DoltDB's git-for-data semantics

## Recommended Fallback Policy

If axon remains unstable, prefer this ordering:

1. Keep JSONL as the default safe path.
2. Use bd for operators who want Dolt-backed storage and already have the binary installed.
3. Keep the Axon backend wired through config and the existing conformance suite until its rollout is proven.

This ordering preserves the existing DDx contract: a single `Backend` interface, JSONL interchange, and a storage layer that can be selected without changing callers.
