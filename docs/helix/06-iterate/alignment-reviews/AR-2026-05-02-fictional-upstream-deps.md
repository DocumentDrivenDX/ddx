---
ddx:
  id: AR-2026-05-02-fictional-upstream-deps
  depends_on:
    - FEAT-006
    - FEAT-010
---
# Alignment Review: Fictional Upstream Dependencies in Phase 2 Bead Filing

## Review Metadata

**Review Date**: 2026-05-02
**Scope**: planning process — multi-model plan review interaction with downstream bead filing
**Status**: complete
**Trigger incident**: Phase 2 Story 10 bead batch filed with hard `dep` edges and bead descriptions claiming "blocked on Fizeau CONTRACT-003 extension for `ExcludePinned` + `ProviderID` + `ProviderClass` + `ActualPower` + separate review cost." On user push-back, verification showed those fields were either already shipped in `fizeau v0.9.28` or never necessary.

## What happened

A planning sub-agent (codex review of the Story 10 plan, recorded at `/tmp/story-10-codex.out`) recommended adding several fields to the upstream Fizeau `ExecuteRequest` and `ExecuteResponse` to support reviewer pairing and capability-tier escalation. The recommendation was framed as a "hard upstream dependency" gating six DDx beads (S10_3, S10_5, S10_6, S10_9, S12_1, and indirectly others through coupling).

DDx beads were filed with that framing baked into descriptions, hard `dep` edges connecting them to a wrapper coordination bead (`S10_1`), and the user was told the queue was substantially blocked on upstream work.

Verification against the actual shipped Fizeau state revealed:

| Claimed-needed Fizeau field | Actual state |
|---|---|
| `MinPower` / `MaxPower` on `ExecuteRequest` | Shipped — `agent-79e194aa` closed |
| `Power` metadata on `ListModels` output | Shipped — `agent-da67ebbe` closed |
| Catalog power filtering during routing | Shipped — `agent-7d537e4a` closed |
| Power-aware candidate scoring | Shipped — `agent-005a0a30` closed |
| Routing-policy invariant test contract | Shipped — `agent-de968c76` closed |
| `CostUSD` on `Final` event | Shipped — present in `ServiceFinalData` for many releases |
| `ExcludePinned` field on `ExecuteRequest` | Never needed — design rejected; reviewer "different provider" is best-effort emergent from `MinPower` bump |
| `ProviderID` / `ProviderClass` on response | Never needed — `ServiceRoutingActual.{Harness,Provider,Model}` already returned |
| Separate review cost field | Never needed — `CostUSD` already accumulates per-call |
| `Power int` on `ServiceRoutingActual` | **Genuinely missing** — only real new ask |

The only legitimate net-new upstream ask was a small, observability-shaped epic — `agent-3bb96bf5` in the Fizeau working tree at `/home/erik/Projects/agent` — adding:
- `Role string` and `CorrelationID string` (pure observability) to `ExecuteRequest` and `RouteRequest`
- `Power int` to `ServiceRoutingActual` (so DDx can compute `next.MinPower = previous.Power + rung_step` without importing catalog code)
- Documented reserved-key collision rules for `Metadata["role"]` / `Metadata["correlation_id"]`
- Echo to `Final` + `RoutingDecision` event metadata + session-log header

All other "blocked on Fizeau" framing in the Story 10 batch was scaffolding around fictional fields.

## Root cause

When a planning sub-agent generates a review against an under-specified plan, it can confidently recommend upstream API additions that are either already shipped or never necessary. The sub-agent's review reads as authoritative; downstream bead descriptions inherit the framing without independent verification. The pattern propagates because:

1. The DDx-side bead author (planning agent or human) trusts the codex review without cross-checking the upstream module's `go.mod` version, contract spec, and historical bead state
2. Multiple downstream beads inherit the same framing in a single batch, multiplying the surface area of fiction
3. Hard `dep` edges to a wrapper coordination bead make the fiction look load-bearing in the queue

## Process change adopted

When a planning sub-agent recommends "this needs an upstream contract extension":

1. **Verify shipped state before propagating.** Check the imported version in `cli/go.mod`, the upstream contract spec at `~/Projects/agent/docs/helix/02-design/contracts/CONTRACT-003-ddx-agent-service.md` (and equivalents for other upstream modules), and historical bead state in the upstream repo (`cd ~/Projects/agent && ddx bead show <referenced-id>`). Confirm each "needed" field is genuinely missing before asserting a downstream block.
2. **Frame upstream deps narrowly.** "Wait for `agent-XYZ` tagged release" is precise; "blocked on Fizeau contract extension" is hand-wavy and turns into invented scope across multiple beads.
3. **Push back on the planning sub-agent.** Codex (and any plan-review agent) does not always check shipped state. When its review is built on a hypothetical upstream world, the right move is to challenge the premise, not file beads against the hypothesis.
4. **Cross-repo bead filing.** When upstream work is genuinely needed, file the precise scope in the upstream repo's bead store as the source of truth. The DDx-side coordination bead becomes a thin "wait for upstream tag + bump go.mod" wrapper, not a fictional contract specification.

## Beads adjusted

| DDx bead | Adjustment |
|---|---|
| `ddx-8f1ac866` (S10_1) | Title + description rewritten to "wait for `agent-3bb96bf5` tagged release + bump `cli/go.mod`" |
| `ddx-51ac997b` (S10_2) | Description corrected: MinPower/MaxPower already in v0.9.28 |
| `ddx-cfedee8e` (S10_3) | Description corrected: ladder uses existing MinPower; reads RoutingActual.Power once it lands via `agent-3bb96bf5` |
| `ddx-2436fa73` (S10_5) | Description corrected: best-effort "different provider" via MinPower bump; no ExcludePinned ask |
| `ddx-a8718bec` (S10_6) | Description corrected: CostUSD already on Final, no upstream change needed |
| `ddx-a8718bec` (S10_6) | Removed fictional dep edge on `ddx-8f1ac866` |
| `ddx-5f1eac4f` (S10_9) | Description corrected: ADR-021 documents Day 1 = pure observability; future Fizeau routing intelligence keyed on Role/CorrelationID is Fizeau-internal |

## Beads filed upstream

In `~/Projects/agent`:

| ID | Title |
|---|---|
| `agent-3bb96bf5` (epic) | CONTRACT-003: add Role + CorrelationID + Power for downstream observability |
| `agent-bc214918` | CONTRACT-003: amend spec |
| `agent-170380c1` | fizeau: add Role + CorrelationID + Power types |
| `agent-be5c1ea6` | fizeau: echo Role + CorrelationID into events + session log header |
| `agent-fcbf43d1` | fizeau: extend Routing Policy Test Contract |
| `agent-744cd55e` | fizeau: tag release; downstream notification |

## Open questions

- Should the multi-model plan-review pattern be amended so codex/opus reviews of plans that touch upstream contracts include an explicit "verify shipped state of named fields" step before producing the review? (See: codex-review prompt template under planning skills.)
- Should the bead filing flow include an automated check that any bead description mentioning `<upstream-repo>` or `<contract>` cross-references an actual upstream bead ID or shipped feature flag?

## References

- `cli/go.mod`: `github.com/DocumentDrivenDX/fizeau v0.9.28`
- `~/Projects/agent/docs/helix/02-design/contracts/CONTRACT-003-ddx-agent-service.md`
- Original Story 10 plan: `/tmp/story-10-final.md`
- Original Story 10 codex review: `/tmp/story-10-codex.out`
