---
dun:
  id: ADR-002
  depends_on:
    - helix.prd
    - ADR-001
---
# ADR-002: HELIX Tracker Write Safety Model

| Date | Status | Deciders | Related | Confidence |
|------|--------|----------|---------|------------|
| 2026-03-27 | Proposed | HELIX maintainers | tracker, installer, tests | High |

## Context

| Aspect | Description |
|--------|-------------|
| Problem | The built-in HELIX tracker stores one JSON object per line in `.helix/issues.jsonl`, but current mutation paths are naive enough that malformed writes or overlapping edits can corrupt the file and break all tracker operations. |
| Current State | Tracker writes are implemented as full-file read/modify/write cycles with no explicit conflict detection contract, no malformed-file recovery contract, and limited metadata mutation support. Real planning work has already reproduced JSONL corruption while refining issues. |
| Requirements | HELIX needs a conservative local tracker that is safe enough for agent-driven issue refinement and concurrent local supervision. The tracker must surface malformed state explicitly, define what concurrency/conflict guarantees are supported, and make metadata mutation available through first-class commands instead of direct file edits. |

## Decision

We will define the HELIX tracker as a conservative file-backed system with
explicit write-safety guarantees, conflict visibility, and fail-closed
corruption handling.

The tracker will not pretend to provide arbitrary multi-writer transactional
semantics. Instead, it will define the supported local execution model,
require explicit detection or prevention of silent lost updates, and make
malformed tracker state a surfaced failure rather than something the rest of
HELIX must guess around.

The supported local model must explicitly include one automated `helix-run`
session progressing execution while another local session refines specs or
tracker issues. The write-safety model therefore has to support not just file
integrity, but also concurrency-visible mutation semantics that let the runner
revalidate issue state at safe boundaries.

**Key Points**: explicit local write model | fail closed on malformed state |
first-class mutation APIs instead of manual JSONL edits

## Alternatives

| Option | Pros | Cons | Evaluation |
|--------|------|------|------------|
| Keep naive JSONL read/modify/write and rely on careful operator usage | Lowest implementation effort | Proven corruption risk, no race semantics, unsafe for agent-driven refinement | Rejected: already failing in real use |
| Replace the tracker immediately with a database-backed system | Stronger concurrency primitives | Much larger implementation jump, higher distribution complexity, changes HELIX surface area substantially | Rejected for now: too large for the immediate hardening goal |
| **Keep JSONL but define and enforce a conservative write-safety contract** | Preserves the current HELIX tracker shape, addresses current failures, enables deterministic testing | Requires explicit conflict/corruption handling and careful write-path design | **Selected: smallest sufficient hardening** |

## Consequences

| Type | Impact |
|------|--------|
| Positive | Tracker behavior becomes specifiable and testable instead of depending on lucky file writes. |
| Positive | HELIX issue refinement can move toward first-class metadata mutation APIs rather than direct JSONL edits. |
| Positive | The tracker becomes a reliable coordination layer between automated execution and interactive refinement. |
| Positive | Malformed tracker state becomes diagnosable instead of cascading into undefined behavior. |
| Negative | Tracker commands may fail more often and earlier when they detect malformed or conflicting state. |
| Negative | Additional deterministic tests and write-path safeguards are required before the contract is real. |
| Neutral | The tracker remains file-backed and local-first for now. |

## Risks

| Risk | Prob | Impact | Mitigation |
|------|------|--------|------------|
| The supported local concurrency model remains underspecified | M | H | Pair this ADR with an explicit tracker contract artifact and deterministic tests |
| The hardening layer adds partial protections but still misses silent lost updates | M | H | Make race and corruption scenarios executable in the harness before claiming safety |
| Metadata mutation expansion broadens the surface faster than the safety model | M | M | Define mutation contract first, then add APIs behind tests |
| File safety improves but `helix-run` still closes stale work after concurrent refinement | M | H | Define pre-claim/pre-close revalidation and issue supersession semantics in the technical design |

## Validation

| Success Metric | Review Trigger |
|----------------|----------------|
| Tracker docs and tests describe the same write-safety and corruption-handling model | Any tracker mutation behavior that is implemented but not captured in the contract or tests |
| Real malformed-state and overlapping-mutation scenarios fail conservatively instead of corrupting the file | A reproduced issue causes invalid JSONL or silent lost updates |
| HELIX issue refinement no longer requires direct JSONL surgery for supported metadata changes | A HELIX workflow still needs manual JSONL edits for normal tracker mutation needs |
| Concurrent local operator refinement is surfaced as queue drift rather than hidden stale execution | `helix-run` claims or closes work after a material tracker change without revalidation |

## References

- [Product Vision](/home/erik/Projects/helix/docs/helix/00-discover/product-vision.md)
- [PRD](/home/erik/Projects/helix/docs/helix/01-frame/prd.md)
- [ADR-001](/home/erik/Projects/helix/docs/helix/02-design/adr/ADR-001-supervisory-control-model.md)
