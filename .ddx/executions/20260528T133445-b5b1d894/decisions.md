# Dead Code Disposition Decisions

## Summary
All orphaned helper functions from the deadcode RTA analysis have been disposed of. Five functions were identified as unreachable from production code and have been deleted. Dependent tests were updated or removed to maintain test suite consistency.

## Disposition Decisions

### Deleted Functions

1. **WarmProfileSnapshotForProject** (profile_select.go:72) - **DELETE**
   - Reason: No production uses found. Function was intended as an optimization hook but never called.
   - Status: DELETED

2. **SelectCheapestProfile** (profile_select.go:255) - **DELETE**
   - Reason: Only test-only uses in profile_select_test.go. Not called from production code.
   - Impact: Removed 3 dedicated test functions; removed one call from TestSelectProfile_ReturnsEmptyWhenNothingSatisfies
   - Status: DELETED
   - Tests Updated:
     - Deleted: TestSelectCheapestProfile_LowestBandWithAvailableModel
     - Deleted: TestSelectCheapestProfile_UsesPolicyMetadataWhenModelSnapshotEmpty
     - Deleted: TestSelectCheapestProfile_TieDoesNotPreferLocalPolicy
     - Deleted: TestSelectCheapestProfile_DoesNotSelectRequirementProfile
     - Updated: TestSelectProfile_ReturnsEmptyWhenNothingSatisfies (removed SelectCheapestProfile assertion)

3. **TestProviderConnectivityViaService** (service_run.go:581) - **DELETE**
   - Reason: No production uses found. Dead code despite descriptive name suggesting production role.
   - Status: DELETED

4. **ValidateEffortForRunViaService** (service_run.go:668) - **DELETE**
   - Reason: No production uses found. Function is a validation helper with no callers.
   - Status: DELETED

5. **IsGitUpdateRefCompareAndSwapFailure** (triage_dispatch.go:25) - **DELETE**
   - Reason: Only test-only uses. Used only in execute_bead_concurrent_predispatch_test.go and execute_bead_report_test.go.
   - Impact: Inlined error pattern check into test code; removed dedicated test function
   - Status: DELETED
   - Tests Updated:
     - Deleted: TestIsGitUpdateRefCompareAndSwapFailure (execute_bead_report_test.go:15)
     - Updated: execute_bead_concurrent_predispatch_test.go (inlined pattern checks at lines 97 and 176)

## Test Results

All remaining tests pass:
- Profile selection tests (TestSelect*): PASS
- Report and triage tests: PASS
- No test suite regressions

## Verification

Deadcode RTA run post-deletion:
```
go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... | grep -E 'internal/agent/(profile_select|service_run|triage_dispatch|repair_prompt)\.go'
```
Result: No unreachable functions found in target files.

## Notes

- BuildRepairPrompt was mentioned in the original analysis but is NOT dead code (used in candidate_cycle.go:164)
- IsLockContentionError in triage_dispatch.go remains (used in production: readiness_classification.go, execute_bead_status.go)
- All production-critical functions (SelectStrongestProfile, SelectImplementationProfile, etc.) remain intact
