# Reliability Principles

See this document for the reliability principles (P1–P10) applied to ddx try / ddx work execution.

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

## P8: DDx-Owned Resources Must Be Reclaimed

Every DDx-created temporary execution resource has an owner, liveness signal,
retention policy, and cleanup path. That scope includes leaked DDx test/e2e
scratch roots, execution worktrees, generated test binaries, and run-state or
liveness files. `ddx try` cleans up one attempt inline; `ddx work` cleans up
between attempts and on graceful shutdown; long-running workers also run
conservative background cleanup with a lock and jitter.

Host-level resource exhaustion is not a bead failure and not model
no-progress. If bytes, inodes, writability, or git worktree registration are
unhealthy after one cleanup pass, the work loop stops visibly and claims no more
beads. Continuing to scan the queue under ENOSPC expands the blast radius and
violates P4.

Concrete policy values — recognized DDx-owned scratch prefixes, minimum mtime
age for metadata-less directories (6 hours), soft cleanup trigger (512 MiB free
bytes / 8192 free inodes), and hard stop floor (64 MiB free bytes / 1024 free
inodes) — are defined in FEAT-010 `### Execution Resource Cleanup` and mirrored
in SD-025 `### Roots and ownership` and `### Resource preflight and loop-fatal
handling`. Implementation beads must reference those sections rather than
guessing at values. Test/e2e scratch roots under DDx prefixes (`ddx-test-`,
`ddx-e2e-`) are explicitly in scope for cleanup; treating them as out of scope
is a correctness bug under this principle.

## P9: Network-Free Drain

The single-node drain loop must never block on network I/O. `Land()` performs
only a local worktree-merge into the local target branch under a brief local
lock. `git fetch` and `git push` are never called from `Land()` or from the
`ddx work` queue loop.

This invariant prevents the 30-120 s lock holds that caused ddx-08e90869 (work
landed only on a preserved iteration ref due to tracker lock contention) and
ddx-15a51f8c (push-race remediation). Network operations are operator-driven
(see FEAT-023 `ddx sync`) or future background-deferred, never coupled to the
drain.

Governing requirement: FEAT-010 §"Network-free drain boundary" (functional
requirement 16) and §"Network isolation" (non-functional). Implementation
target: `cli/internal/agent/execute_bead_land.go` `Land()` and
`cli/internal/agent/execute_bead_loop.go` drain loop — neither must call
`git fetch` or `git push`.

## P10: No Skip-And-Exit On Idle Scan

When the picker returns no executable bead, the work loop does not exit. It
diagnoses every non-ready bead in the breakdown with a per-bead reason code,
fires the matching auto-remediation in the same loop iteration, then re-scans.
Every blocker class that the diagnoser surfaces must have either:

- A defined auto-remediation that the loop runs without operator intervention,
  subject to per-bead attempt cap (3), cost budget (`--max-recovery-cost`),
  cooldown, cycle guard, and operator override flag; or
- A defined operator-surface path (`ddx work focus` Section A) that triggers
  ONLY after auto-remediation has provably failed (attempts exhausted, gates
  hit) or where automated remediation is not safe (real dep blocks on open
  work, tracker integrity issues, unknown state).

"Skip and exit" — printing `No execution-eligible beads in the queue` and
returning while non-ready beads still have known root causes the system could
act on — is forbidden. The factory rule: `ddx work` must self-unstick;
operator attention is the exception, not the default.

Beads queued for auto-remediation are surfaced in `ddx work focus` Section B
(count by default, `--verbose` for per-bead detail). Silent hiding of
auto-remediable beads is also forbidden — operators must always be able to
see what the loop is about to do on its next iteration.

Governing requirement: FEAT-010 §"Idle-Path Diagnosis and Auto-Remediation"
and FEAT-004 §"Queue Semantics For Epics" define the closed reason taxonomy,
the safety contract, and the per-class remediation paths. New blocker classes
or new reason codes require those specs to be updated before the diagnoser
ships the code.
