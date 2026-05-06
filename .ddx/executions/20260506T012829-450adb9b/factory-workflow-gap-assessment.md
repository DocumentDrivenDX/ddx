# Retry / Escalation / Review Workflow Gap Assessment

Date: 2026-05-06

## Baseline Specs Reviewed

- `docs/helix/01-frame/features/FEAT-010-executions.md`
- `docs/helix/01-frame/features/FEAT-022-prompt-evidence-assembly.md`
- `docs/helix/02-design/adr/ADR-023-bead-lifecycle-quality-policy.md`
- `docs/helix/02-design/adr/ADR-024-power-escalation-and-review-routing.md`
- `docs/helix/02-design/technical-designs/TD-031-bead-state-machine.md`
- `docs/triage/decomposition.md`

## Gap Summary

1. **Pre-close review gate missing as first-class implementation work.**
   Existing Story 18 beads mostly cover review sessions, UI, GraphQL, and single-review plumbing. The finalized specs require default adversarial pre-close review before `CloseWithEvidence`, two reviewer slots, unanimous evidence-backed approval, review-error retry caps, and repair-cycle traceability.

2. **Intake/actionability is specified but not represented by executable queue work.**
   Existing work covered `PreDispatchLintHook` and an implementer-prompt Step 0 decomposition hint. The specs require a pre-claim intake gate that can safely rewrite, decompose with AC maps, or block ambiguous/lossy work before an implementer claims it.

3. **Stop/retry work had a dependency deadlock and stale semantics.**
   `ddx-9228a484` said it absorbed `ddx-cfedee8e` but depended on it. It also needed updated acceptance criteria for valid-attempt no-progress counting, review-fixable repair cycles, terminal review classifications, and append-only cycle trace data.

4. **Review cost/refusal bead was over-configured.**
   `ddx-f1e12904` still required preferred reviewer and downgrade ladder behavior. ADR-024 makes adversarial review default and avoids configuration-heavy routing; the remaining valid gap is structured budget/refusal behavior that feeds the review-error path.

5. **Obsolete decomposition and escalation beads were still ready.**
   `ddx-e0be88f6`, `ddx-cfedee8e`, and the decomposed parent `ddx-e1a576a7` could still be selected even though replacement work now owns their scope.

6. **Spec cleanup bead depended on a currently noisy doc audit.**
   `ddx-2db0bd7a` requires `ddx doc audit clean`, but the current audit can be polluted by hidden configured roots such as `.agents/skills/docs`. That infrastructure bug is tracked by `ddx-3c6f5bf0`.

## Tracker Changes Made

- Created `ddx-c851c3dd`: default adversarial pre-close review gate.
- Created `ddx-f3bbcfce`: pre-claim intake gate for actionability and AC-preserving decomposition.
- Updated `ddx-9228a484`: StopCondition, valid-attempt retry budget, review repair-cycle escalation, and trace requirements.
- Updated `ddx-f1e12904`: narrowed to review budget/refusal contracts; removed preferred reviewer/downgrade routing from scope.
- Marked `ddx-cfedee8e` superseded by `ddx-9228a484`, set `execution-eligible=false`, and made it depend on `ddx-9228a484`.
- Marked `ddx-e0be88f6` superseded by `ddx-f3bbcfce`, set `execution-eligible=false`, and made it depend on `ddx-f3bbcfce`.
- Marked `ddx-e1a576a7` decomposed/not directly execution-eligible and made it depend on `ddx-f3bbcfce`.
- Removed the deadlocking `ddx-9228a484 -> ddx-cfedee8e` dependency.
- Made `ddx-2db0bd7a` depend on `ddx-3c6f5bf0` and noted the audit-noise blocker.

## Ready-State Recheck

After the first pass, the touched obsolete beads are no longer ready:

- `ddx-cfedee8e` is dependency-blocked by `ddx-9228a484`.
- `ddx-e0be88f6` is dependency-blocked by `ddx-f3bbcfce`.
- `ddx-e1a576a7` is dependency-blocked by `ddx-f3bbcfce`.

The workflow-critical ready replacements are:

- `ddx-c851c3dd`
- `ddx-f3bbcfce`

`ddx-f1e12904` is now blocked behind `ddx-c851c3dd`, so review budget/refusal work follows the gate semantics instead of predefining conflicting routing behavior.

`ddx-2db0bd7a` is now blocked behind `ddx-3c6f5bf0`, so the spec-cleanup bead is not retried until the doc audit can ignore hidden root directories.
