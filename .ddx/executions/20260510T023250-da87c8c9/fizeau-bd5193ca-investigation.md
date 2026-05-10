# Investigation: fizeau-bd5193ca — why 4 attempts preserved, none landed

Bead: fizeau-bd5193ca "fix: integrate Gaussian broadening into fizeau visibility fringe model"
Execution: 20260510T023250-da87c8c9

## Summary

All 4 attempts produced valid implementation commits (task_succeeded) but were preserved
by agent.Land() and never merged to master. Root causes:

- Attempt 1: real merge conflict (concurrent commit on master during execution)
- Attempts 2–4: evidence commit failure caused by fizeau's .gitignore covering .ddx/executions/

The operator's cherry-pick of attempt 3 (ee719835) succeeded because the code changes were
valid; the failure was in DDx's evidence-commit infrastructure, not the implementation.

---

## Attempt 1: merge conflict (20260508T175640-b12741e5)

**base_rev**: b88f24ef987c  
**result_rev**: 2c3f896a  
**preserve_ref**: refs/ddx/iterations/fizeau-bd5193ca/20260508T175640-b12741e5-ac79cd13392e

While attempt 1 was executing (17:56–18:12 UTC), commit ac79cd13392e
("chore(beads): add smart route semantics decision bead") was pushed to master by a
concurrent actor. When Land() ran: currentTip(ac79cd13392e) ≠ baseRev(b88f24ef987c) →
merge path → git merge --no-ff conflicted → preserved.

**This is expected behavior** — the DDx land system correctly detected a conflicting
concurrent edit and preserved the iteration for conflict recovery.

---

## Attempts 2–4: evidence commit failure (gitignore root cause)

**Attempt 2**: base=940d8683787c, result=b673f48a, preserve tip=940d8683787c → fast-forward confirmed  
**Attempt 3**: base=e9e33bba16bb, result=ee719835, preserve tip=e9e33bba16bb → fast-forward confirmed  
**Attempt 4**: base=ed0c37701733, result=ebbeb0ad, preserve tip=ed0c37701733 → fast-forward confirmed

For all three: `currentTip == baseRev` so the fast-forward path was taken in landLocked().

### The gitignore problem

fizeau/.gitignore contains:

```
.ddx/executions/
```

This causes `git add` to silently ignore all files under `.ddx/executions/` (no error,
no output, exit 0). The evidence commit mechanism (StageDir → CommitStaged) uses plain
`git add` without `--force`.

### Exact failure chain (per attempt)

1. `commitEvidenceBundleInWorktree` in the execution worktree:
   - Runs `git add -- .ddx/executions/<attemptID>/` → silently stages nothing (gitignored)
   - `git diff --cached --name-only` → empty → returns `""`
   - `evidenceCommittedInWt = false`; `res.ResultRev` stays at implementation commit

2. Deferred evidence publish: copies full evidence dir to projectRoot

3. `singleTierAttempt` calls `SubmitWithPreMergeChecks`:
   - `RunPreMergeChecks`: fizeau has no `.ddx/checks/` directory → `Blocked=false`
   - `localCoord.Submit(BuildLandRequestFromResult(...))` → `agent.Land()`

4. `landLocked` fast-forward path:
   - `gitOps.UpdateRefTo(targetRef, req.ResultRev, currentTip)` → **master advanced** to implementation commit
   - `landingFinalizationWorktree` → temp worktree checked out at master (implementation commit)
   - `preserveIfPostLandGateFails` → no PostLandCommand → passes
   - `copyEvidenceDirForLanding`:
     - `evidenceDirHasTrackedFiles(finalWD, evidenceDir)` → false (gitignored, not in git ls-files)
     - Copies evidence files from projectRoot into finalization worktree filesystem
   - `landEvidence(finalWD, ...)`:
     - `StageDir` → `git add -- .ddx/executions/<attemptID>/` → silently stages nothing (gitignored)
     - `CommitStaged` → nothing cached → returns `sha=""`
     - `evidenceDirHasTrackedFiles(finalWD, evidenceDir)` → false
     - Returns error: "commit evidence: no staged evidence files under .ddx/executions/..."
   - `preserveAfterEvidenceFailure`:
     - Creates preserve ref pointing to `req.ResultRev` (implementation commit)
     - **Rolls back master** to preLandTip (currentTip)
     - Returns `LandResult{Status: "preserved", Reason: "evidence commit failed: ..."}`

5. `ApplyLandResultToExecuteBeadResult`:
   - Reason doesn't start with PreMergeChecksReason
   - Maps to `reasonForStatus = "merge conflict"`
   - `ClassifyExecuteBeadStatus("preserved", 0, "merge conflict")` → **`"land_conflict"`** (escalatable)

6. `runEscalatingSingleTierAttempts` sees escalatable status → escalates to next power tier:
   - Attempt 1 (power floor: none) → attempt 2 (min_power=5)
   - Attempt 2 (min_power=5) → attempt 3 (min_power=7)
   - Attempt 3 (min_power=7) → attempt 4 (min_power=8)
   - Attempt 4 (min_power=8) → attempt 5: routing fails ("no viable routing candidate for pins min_power=9 max_power=8") → exhausted

### Evidence from fizeau refs and events

All preserve refs use `landIterationRef` format (not `PreserveRef` format from
pre-merge checks), confirming these came from `agent.Land()`, not from
`RunPreMergeChecks`.

No `checks-blocked` events appear in fizeau-bd5193ca/events.jsonl — confirming
pre-merge checks never fired.

The implementation commits (b673f48a, ee719835, ebbeb0ad) appear as parents of the
preserve refs, confirming `req.ResultRev` at land time was the implementation commit
(not an evidence commit).

---

## Classification: Real defect in DDx

This is a **real defect**, not a false positive. The pre-merge check system is
innocent. The failure is in the evidence-commit mechanism:

> `StageDir` (= `git add`) does not use `--force`, so projects that gitignore
> `.ddx/executions/` silently produce zero-staged evidence → evidence commit fails →
> Land() preserves instead of landing.

The combination of this with the status-classification bug (evidence-commit-failure
maps to `"land_conflict"` instead of a non-escalatable status) causes the escalation
ladder to run to exhaustion on every single attempt.

Projects that gitignore `.ddx/executions/` (fizeau, and likely many others that follow
the standard DDx .gitignore template) will **never land** until this is fixed.

---

## Why the operator's cherry-pick passed `go test`

The cherry-pick moved commit ee719835 (the implementation changes: Gaussian broadening
integration). That commit contains valid, tested Go code. `go test` validates code
correctness; it has no knowledge of DDx evidence files or the land pipeline. The
failure was entirely in DDx infrastructure after the implementation was complete.

---

## Required follow-up

File bead: `fix: StageDir must use git add --force for evidence directories in projects
that gitignore .ddx/executions/`

Root cause file: `cli/internal/agent/execute_bead_land.go:701` (`StageDir`)

Secondary: `ApplyLandResultToExecuteBeadResult` should map "evidence commit failed"
preservations to a non-escalatable status (e.g. `preserved_needs_review`) rather than
`land_conflict`, so the escalation ladder is not burned on infrastructure failures.
