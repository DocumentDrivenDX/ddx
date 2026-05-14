# Triage-Parked Beads Review - 2026-05-06

## Goal

Review open beads parked behind triage labels, unblock the ones whose latest
reason is no longer valid, and record prevention patterns for automation.
This is a legacy migration audit; mentions of old triage labels below describe
historical cleanup inputs, not active lifecycle states.

## Summary

Initial open triage-labeled set: 43 records.

Observed patterns:

| Pattern | Count | Meaning | Action |
|---|---:|---|---|
| provider_unavailable | 23 | Latest actionable event was `execution_failed` with `all tiers exhausted` / `no viable provider found`. | Removed legacy migration label `triage:needs-investigation`; these are retryable transport outages. |
| verification_red_elsewhere | 4 | Targeted work appeared complete, but a required broad gate was red in unrelated tests. | Leave parked until each bead is verified or AC is narrowed. |
| stale_execute_loop_spec | 6 | Old `ExecuteLoopSpec` refactor beads with no execution event, manual triage label, and deprecated work vocabulary. | Leave parked; rewrite or supersede against current `ddx work` / Fizeau boundary design. |
| decomposition_or_epic | 3 | Parent/umbrella work where children or decomposition already own execution. | Leave parked pending completed-epic closure/supersede policy. |
| no_changes_unverified | 5 | Agent reported already-satisfied or no-changes with verification command. | Legacy migration note: these do not block `ddx work` unless paired with the old needs-investigation label; monitor for auto-verify/close. |
| review_block_or_malfunction | 2 | Review blocked or malfunctioned on a no-changes/result path. | Leave parked pending review/triage cleanup. |

Prevention beads filed:

- `ddx-66c98cde` - `work: keep provider outages retryable instead of triage-parked`
- `ddx-c14f6525` - `work: evaluate completed epics for closure`

## Unblocked Provider-Outage Beads

Removed legacy migration label `triage:needs-investigation` from these beads because their latest
actionable event was only a transient provider/routing outage:

| Bead | Priority | Notes |
|---|---:|---|
| `ddx-9f20a4bd` | 0 | No viable provider; now retryable. |
| `ddx-008d288f` | 1 | No viable provider; now retryable. |
| `ddx-9228a484` | 1 | No viable provider; now retryable. |
| `ddx-9c81b20f` | 1 | Removed legacy migration needs-investigation label; retained `triage:no-changes-unverified`. |
| `ddx-fb790086` | 1 | No viable provider; now retryable. |
| `ddx-256af8b5` | 2 | No viable provider; now retryable. |
| `ddx-4fd71cf3` | 2 | No viable provider; now retryable. |
| `ddx-5681cc57` | 2 | No viable provider; now retryable. |
| `ddx-8c273456` | 2 | No viable provider; now retryable. |
| `ddx-91fe7a1a` | 2 | No viable provider; now retryable. |
| `ddx-9df0636c` | 2 | No viable provider; now retryable. |
| `ddx-a78f836f` | 2 | No viable provider; now retryable. |
| `ddx-ae4b7393` | 2 | No viable provider; now retryable. |
| `ddx-d6730314` | 2 | No viable provider; now retryable. |
| `ddx-23cbcb4b` | 3 | No viable provider; now retryable. |
| `ddx-25069ce4` | 3 | No viable provider; now retryable. |
| `ddx-2db0bd7a` | 3 | No viable provider; now retryable. |
| `ddx-59459dd6` | 3 | No viable provider; now retryable. |
| `ddx-a13eb42a` | 3 | No viable provider; now retryable. |
| `ddx-cd42fc05` | 3 | No viable provider; now retryable. |
| `ddx-d01e5017` | 3 | Removed legacy migration needs-investigation label; retained `triage:no-changes-unverified`. |
| `ddx-e140727a` | 3 | Removed legacy migration needs-investigation label; retained `triage:no-changes-unverified`. |
| `ddx-eddb9ab6` | 3 | No viable provider; now retryable. |

## Remaining Parked Beads

### Verification Red Elsewhere

These should be evaluated one by one. The usual unblock is either: run the
focused AC and close if satisfied, fix the external red test, or narrow the
bead AC if the broad suite requirement is now too noisy.

| Bead | Priority | Latest reason |
|---|---:|---|
| `ddx-1c3a78f2` | 0 | Focused missing-skill diagnostic tests passed, but `go test ./internal/agent/...` failed in unrelated deprecated config/cancel-latency tests. |
| `ddx-312adc66` | 0 | Execution-hint surfaces appear present, but broad agent verification hit a pre-existing hanging test. |
| `ddx-6ecd59aa` | 0 | Targeted preflight tests passed, but broader command failed outside bead scope. |
| `ddx-f326f001` | 0 | Focused terminal-result work passed, but `lefthook run pre-commit` failed on unrelated integration coverage. |

### Stale ExecuteLoopSpec Refactor Beads

These have no recent execution event and still describe `work` as a
future implementation target. Do not blindly retry. They should be rewritten or
closed/superseded after reconciling with the current `ddx work` / `ddx try`
surface and the no-agent cleanup decision.

| Bead | Priority |
|---|---:|
| `ddx-1a9cc01f` | 1 |
| `ddx-16722d4e` | 1 |
| `ddx-76cf71f4` | 1 |
| `ddx-89a9c305` | 1 |
| `ddx-da9e9491` | 1 |
| `ddx-ce1d6309` | 1 |

### Decomposition Or Epic

These are structural or superseded-by-child-work cases. The completed-epic
closure bead (`ddx-c14f6525`) should make this class less noisy.

| Bead | Priority | Latest reason |
|---|---:|---|
| `ddx-a9d130d0` | 1 | Parent epic already decomposed; remaining children own execution. |
| `ddx-d7aca866` | 1 | Umbrella ADR-022 epic, not a single executable change. |
| `ddx-8d747049` | 3 | Already decomposed at depth 2 and still too large for one pass. |

### Review Or No-Changes Problems

These need direct inspection of the latest attempt evidence before unblocking.

| Bead | Priority | Label |
|---|---:|---|
| `ddx-7f4cdb7a` | 2 | `triage:no-changes-unjustified` |
| `ddx-9f6baafe` | 2 | `triage:no-changes-unjustified` |
| `ddx-cfedee8e` | 2 | Legacy migration label `triage:needs-investigation`; rationale says scope is pending/superseded by `ddx-9228a484`. |

### No-Changes Unverified Only

Legacy migration note: these labels were advisory unless paired with `triage:needs-investigation`;
`ddx work plan` shows such beads as eligible. The scalable fix is to make
verification-command no-changes evidence auto-close or auto-clear when green.

| Bead | Priority |
|---|---:|
| `.execute-bead-wt-ddx-0e5c3005-20260505T113434-3317d134-7fb75345` | 2 |
| `ddx-9c81b20f` | 1 |
| `ddx-d01e5017` | 3 |
| `ddx-e0be88f6` | 3 |
| `ddx-e140727a` | 3 |

The `.execute-bead-wt-...` ID looks like an execution worktree name rather
than a valid bead ID. It should be inspected with the tracker tooling before
any mutation.

## Monitoring

After this pass, run:

```bash
ddx work plan --limit=30
ddx bead list --status open --json | jq -r 'select(((.labels//[])|map(startswith("triage:"))|any)) | [.id,((.labels//[])|map(select(startswith("triage:")))|join(",")),.title] | @tsv'
```

Expected near-term signal:

- Provider-unavailable beads should re-enter `ddx work plan`.
- New no-viable-provider failures should not add legacy migration label `triage:needs-investigation`
  once `ddx-66c98cde` lands.
- Completed epics should stop appearing as structural no-ready-work noise once
  `ddx-c14f6525` lands.
