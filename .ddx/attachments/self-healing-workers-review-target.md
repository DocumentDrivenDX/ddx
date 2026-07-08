## Target

Design a permanent fix for DDx worker stoppage across multi-project workers.

Observed incidents:
- Desired DDx/Cayce workers stopped being server-managed because stale
  terminal/operator-attention worker state suppressed restart.
- A Cayce `preserved-needs-review` bead with a large-deletion gate stayed
  worker-eligible until manually moved to `status=blocked` with
  `lifecycle-external-blocker-reason`.
- A readiness response returned `rewrite.acceptance` as an array while
  `preClaimIntakePromptRewrite.Acceptance` is a string at
  `cli/internal/agent/preclaim_intake_hook.go:47-49`.
- Snorri previously stopped on EMFILE / too many open files.
- Direct `nohup` workers died around readiness; `setsid ... < /dev/null`
  survived.
- Runtime JSONL telemetry can conflict during concurrent worker sync.

Proposed fix:
1. Add durable worker slots keyed by `project_root + profile + desired_index`.
   Reconcile desired slots against live PID/PGID, heartbeat, current bead,
   attempt, phase, terminal reason, restart generation, and suppression reason.
   Stale terminal rows must never suppress desired restart.
2. Enforce worker lifecycle invariant:
   `claim -> terminal outcome -> close/unclaim/block/propose`.
   Watch readiness decode failures, provider child never starts, provider child
   exits without outcome, phase-empty heartbeat, process death while claimed,
   stale claim with dead PID/session, and repeated claim of one unexecutable
   bead.
3. Add deterministic server meta-scan across configured projects. It may remove
   dead-PID locks, clear stale run-state for dead workers, unclaim stale claims,
   park/block through lifecycle rules, restart missing desired workers, and file
   code-defect beads. It may dispatch an agent only after deterministic repair
   fails.
4. Add typed failure classes:
   `supervisor_stale_terminal_block`, `readiness_decode_unavailable`,
   `stale_claim_after_process_death`,
   `unreviewed_preserved_attempt_reclaimed`,
   `resource_exhausted_emfile`, `direct_background_session_fragility`.
5. Add doctor checks and multi-project integration tests.

## Review Question

Find BLOCKING issues before this is decomposed into beads. Focus on ambiguity,
unsafe automation, lifecycle violations, missing file/test boundaries, races,
or over-large beads.

You are a critic, not a validator. A BLOCKING finding is anything that would
cause implementation rework, a migration hazard, or a spec gap that agents will
interpret differently.

## Output Contract

### Findings

| Severity | Area | Finding |
|---|---|---|
| BLOCKING | <area> | <specific issue with evidence from the target> |
| WARNING | <area> | <specific issue with evidence from the target> |
| NOTE | <area> | <observation> |

### Verdict: APPROVE | REQUEST_CHANGES | BLOCK

### Summary

2-4 sentences.
