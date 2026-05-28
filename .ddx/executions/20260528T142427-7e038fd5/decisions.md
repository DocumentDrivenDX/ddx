# Deadcode Disposition Decisions

## Summary
Analyzed 6 orphaned symbols from production-reachability RTA artifact. **1 WIRED**, **5 DELETED**.

## Per-Symbol Decisions

### 1. BuildRepairPrompt (repair_prompt.go:24) → **WIRE**
- **Usage**: Called from `candidate_cycle.go:357` in production repair cycle path
- **Justification**: Core component of the bead repair workflow; must remain reachable
- **Action**: Keep function; ensure reachability from main execute-bead path

### 2. WarmProfileSnapshotForProject (profile_select.go:72) → **DELETE**
- **Usage**: Exported function, zero production calls found
- **Justification**: Orphaned helper with no active consumer
- **Action**: Remove function entirely; no tests depend on it

### 3. SelectCheapestProfile (profile_select.go:255) → **DELETE**
- **Usage**: Called only by tests in `profile_select_test.go` (4 test functions)
- **Justification**: Pure test-utility profile selection; production uses SelectImplementationProfile, SelectStrongestProfile, SelectStandardProfile
- **Tests affected**: 
  - `TestSelectCheapestProfile_LowestBandWithAvailableModel`
  - `TestSelectCheapestProfile_UsesPolicyMetadataWhenModelSnapshotEmpty`
  - `TestSelectCheapestProfile_TieDoesNotPreferLocalPolicy`
  - `TestSelectCheapestProfile_DoesNotSelectRequirementProfile`
- **Action**: Delete function and all 4 test functions

### 4. TestProviderConnectivityViaService (service_run.go:581) → **DELETE**
- **Usage**: Zero production calls; mentioned in test_adapters_test.go comment but never used
- **Justification**: Orphaned legacy adapter function
- **Action**: Remove function entirely

### 5. ValidateEffortForRunViaService (service_run.go:668) → **DELETE**
- **Usage**: Zero production calls found
- **Justification**: Orphaned validation function with no consumer
- **Action**: Remove function entirely

### 6. IsGitUpdateRefCompareAndSwapFailure (triage_dispatch.go:25) → **DELETE**
- **Usage**: Called only from tests (execute_bead_concurrent_predispatch_test.go, execute_bead_report_test.go)
- **Justification**: Pure test-utility error classifier; production code does not depend on it
- **Test affected**: `TestIsGitUpdateRefCompareAndSwapFailure`
- **Action**: Delete function and test function; update execute_bead_concurrent_predispatch_test.go to inline string matching instead

---

## Verification Strategy
1. Delete symbols and dependent tests ✓
2. Run `cd cli && go test ./...` to verify all remaining tests pass ✓
3. Run `lefthook run pre-commit` to verify format/lint ✓
4. Run deadcode checker to confirm no new dead code introduced ✓

## Implementation Summary
**Status**: COMPLETED

All 5 orphaned symbols deleted. Tests updated and passing. Deadcode RTA now clean for target files.
- Commit: bb06471d8 (checks: dispose orphaned profile/service/triage/repair helpers from reachability graph)
- 6 files modified, 5 insertions(+), 178 deletions(-)
- No deadcode hits in internal/agent/(profile_select|service_run|triage_dispatch|repair_prompt).go
- All profile selection tests passing
- Concurrent predispatch tests passing (git error matching inlined)
