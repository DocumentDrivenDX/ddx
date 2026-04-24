# Efficacy "Compare" — Redesign In Place

**Date:** 2026-04-24
**Bead:** ddx-6ce6d7a7
**Status:** Decided

## Context

The Efficacy page had a standalone "Compare" button that opened an empty modal asking the user to add "arms" (model + prompt pairs) from scratch. User feedback: the button's purpose was not obvious — the Efficacy page aggregates sessions by `(harness, provider, model)`, so a cold-start "design an A/B experiment" flow sitting next to it created a mental-model mismatch.

The backing feature (`Query.comparisons`, `Mutation.comparisonDispatch`, `ComparisonArmInput`) is a real A/B capability. It is not wrong to have — it is wrong to expose as a cold-start modal colocated with a rollup view.

## Evidence

- **Schema surface.** `schema.graphql:2499-2613` — `comparisonDispatch` and `ComparisonArmInput` already accept optional `harness` and `provider` alongside `model`+`prompt`. Pre-seeding from efficacy row tuples requires no schema change.
- **Resolver.** `cli/internal/server/graphql/resolver_feat008.go:146-189` — dispatches a queued record; trims all four fields.
- **Non-UI references.** `comparisonDispatch` is referenced by the server test (`feat008_test.go`), the agent `compare_adapter.go` plumbing, and design docs (`SD-023`). Nothing outside the UI depends on the *standalone* entry point.
- **Usage check.** No `.ddx/comparisons/` directory exists in this repo; the bead index has no `comparison_id` correlation. The cold-start flow has not been exercised in practice. This supports replacing it rather than preserving parity.

## Decision

**Option 2: Redesign in place.**

The Efficacy page gains a leading checkbox column on the rollup table. Selecting 2+ rows enables a "Compare selected" header button that dispatches a comparison pre-seeded with the selected rows' `(harness, provider, model)` tuples. The user supplies only the prompt. The cold-start empty-modal button is removed.

Why not Option 1 (relocate to `/experiments`): usage data does not justify building a new top-level surface; the feature's natural entry point is the rollup the user is already reading.

Why not Option 3 (remove): the backing capability is cheap to keep and has legitimate A/B use cases. The fix is UX, not deletion.

## Scope

- Frontend: efficacy page selection state, header button disabled/enabled/hidden states, pre-seeded dialog, mutation now passes `harness`/`provider`/`model`/`prompt`.
- Mutation payload: adds `harness` and `provider` per arm (schema already supports).
- Tests: Playwright e2e for the selection → pre-seed flow.
- Out of scope: new `/experiments` page; changes to comparison execution; efficacy perf/source rework (covered by `ddx-0a33bc5f`).

## If this decision changes

If a future bead picks Option 1 or Option 3, replace the checkbox column / header button with either (1) a link to `/experiments` or (3) removal of the Comparisons section and resolver.
