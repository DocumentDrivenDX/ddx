# Reliability Principles

See this document for the 7 reliability principles applied to ddx try / ddx work execution.

These principles describe how the execution machinery should behave when a layer rejects a candidate, encounters transient failure, or needs to surface degraded state to operators.

## P1: Fail-Open At Every Machinery Layer

When a layer's specific check rejects a candidate, the layer skips itself and emits a structured event. It does not wedge the pipeline.

The auto-routing fix (`workers.go:803`, commit `3b4f5d58`) is the canonical example: preflight is now advisory when no operator pin exists.

## P2: Single Responsibility Per Layer

Each layer rejects only on conditions it owns. Routing pre-flight does NOT decide provider availability (`fizeau` owns); cooldown does NOT decide eligibility (`picker` owns); etc.

Cross-layer concerns are the smell.

## P3: Observable Degradation

Every fail-open emits a structured event surfaced in the workers panel. Operators see "preflight skipped (no operator pin)" instead of silent acceptance OR endless retry loop.

## P4: Bounded Blast Radius

A failure on bead X must not affect bead Y. The stay-alive fix at commit `41cb762e` established this for preflight rejections (per-bead continue, not loop exit). Extend it to all layers.

## P5: Operator-Visible State

Worker reports current state (`idle`, `claiming`, `executing`, `reviewing`, `blocked-on-X`) at all times. No "8 hours running, 0 attempts" mystery state.

ADR-022 rev 5 `§Probe + freshness state model` defines the worker side; the UI workers panel surfaces it.

## P6: Auto-Retry Only For Transient Classes

Cooldown fires ONLY when the model genuinely couldn't make progress (clean no-changes with rationale). Disrupted, preflight-rejected, network-error, claim-race -> no cooldown, return to ready.

Existing code: `shouldSuppressNoProgress` at `execute_bead_loop.go:1545` already respects `Disrupted` (commit `47d8054e`).

## P7: Bead = Prompt

A bead's description + AC must be sufficient context for a competent sub-agent to execute it without hand-curation. Investigation done, file:line citations included, concrete test names specified, explicit non-scope marked.

If a sub-agent succeeds where the bead's auto-prompt failed, the BEAD failed (not the executor). Bead-authoring template enforces this; bead-quality audit (forthcoming bead) retrofits existing beads.

