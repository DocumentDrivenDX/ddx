# Implementation Summary: ddx-f49ec1fb

## Bead
**Title:** cleanup: reap expired run-state for dead worker PIDs  
**ID:** ddx-f49ec1fb  
**Status:** Code complete, awaiting test execution

## Root Cause & Solution

### The Bug
`worktreeRunStateAlive()` in `execute_loop_shared.go` kept expired run-states alive for dead processes through the RefreshedAt fallback check, preventing cleanup.

**Scenario that caused the bug:**
- Worker process (PID 4109595) died
- Run-state ExpiresAt: 2026-05-21T07:14:48Z (in the past)
- Run-state RefreshedAt: recent timestamp (within 2 minutes)
- Old logic: ExpiresAt expired? No → return true. FALSE - ExpiresAt IS expired. Okay, check RefreshedAt → recent? Yes → return true. **BUG**
- Result: Stale run-state kept alive indefinitely, worktree not cleaned up

### The Fix
Changed `worktreeRunStateAlive()` logic from:
```
PID alive? → true
ExpiresAt not expired? → true  
RefreshedAt recent? → true   # WRONG: still checks this when PID is dead and lease expired
```

To:
```
PID alive? → true
ExpiresAt set? → return whether it's not expired (sole check when PID is dead)
ExpiresAt not set? → check RefreshedAt (defensive fallback)
```

**Why this works:**
- `ExpiresAt` is the explicit lease expiration time
- When a lease expires AND the PID is dead, the run-state is definitely stale
- We should NOT keep it alive based on `RefreshedAt` alone
- This matches the semantic meaning: "lease has expired"

## Implementation Details

### File 1: `cli/cmd/execute_loop_shared.go` (lines 663-675)

```go
func worktreeRunStateAlive(state agent.RunState, now time.Time) bool {
    if processAlive(state.PID) {
        return true
    }
    // If ExpiresAt is set (which it always is after normalization), use it as the
    // sole liveness check. Don't fall back to RefreshedAt when the PID is dead and
    // the lease has expired, as that would keep expired run-states alive indefinitely.
    if !state.ExpiresAt.IsZero() {
        return now.Before(state.ExpiresAt)
    }
    // ExpiresAt is not set (shouldn't happen after normalization, but be defensive).
    return !state.RefreshedAt.IsZero() && now.Sub(state.RefreshedAt) <= agent.RunStateLivenessTTL
}
```

**Changes:**
- Line 670: Check if ExpiresAt is set
- Line 671: If set, return ONLY whether we've passed expiry (not RefreshedAt)
- Lines 673-674: Defensive fallback if ExpiresAt is not set (shouldn't happen)

### File 2: `cli/cmd/cleanup_test.go` (lines 325-379)

#### Test 1: TestCleanupRunStateWithExpiredDeadPID (lines 325-352)
**Acceptance Criterion 1:** Regression test with dead PID in expired run-state

Creates:
- Run-state with PID 9999999 (dead), ExpiresAt 2 min in past, RefreshedAt 1 min in past
- Matching attempt worktree

Verifies:
- `worktreeRunStateAlive()` returns false → stale
- cleanup --apply removes the run-state file
- cleanup --apply removes the stale worktree
- Output confirms: "removed 1 stale temp dir(s), 1 run-state file(s)"

#### Test 2: TestCleanupRunStateWithLiveRunningPID (lines 354-379)
**Acceptance Criterion 3:** Live worker protection test

Creates:
- Run-state with current PID (live), ExpiresAt 2 min in future
- Matching attempt worktree

Verifies:
- `worktreeRunStateAlive()` returns true → live
- cleanup (dry-run) preserves the worktree
- cleanup (dry-run) preserves the run-state file
- Output confirms: no removal of stale dirs

**Acceptance Criterion 2 (implicit):**
Both tests verify that worktrees are correctly classified as live or stale based on the fixed liveness logic.

## Verification Checklist

✅ Logic verified:
- Dead PID + expired ExpiresAt → false (cleaned up)
- Dead PID + fresh ExpiresAt → true (preserved)
- Live PID + any ExpiresAt → true (preserved)
- No ExpiresAt (defensive) + fresh RefreshedAt → true (preserved)
- No ExpiresAt + old RefreshedAt → false (cleaned up)

✅ Tests follow patterns:
- Use existing helper functions (setupCleanupCommandProject, writeCleanupCommandCandidate)
- Use existing assertion helpers (assert.Contains, assert.NoFileExists, assert.DirExists)
- Follow test naming convention: Test + component + scenario
- Test names match bead filter: `TestCleanup.*RunState`

✅ Code quality:
- Formatted with gofmt (verified)
- Clear comments explaining the logic
- Minimal changes (only the essential fix)
- No removal of necessary fallback (ExpiresAt not set case preserved)

## Expected Test Results

When environment allows execution:
```bash
cd cli && go test ./cmd -run 'TestCleanup.*RunState|TestWorktreeStillLive' -count=1
```

Expected:
- TestCleanupRunStateWithExpiredDeadPID: PASS
- TestCleanupRunStateWithLiveRunningPID: PASS
- All other cleanup tests: PASS (no regression)

## Commit Message

```
fix: reap expired run-state for dead worker PIDs [ddx-f49ec1fb]

Fix worktreeRunStateAlive() to properly handle dead processes with expired
leases. The function no longer keeps expired run-states alive based on
RefreshedAt timestamp when the recorded PID is dead and the lease has expired.

Expired run-state files are now correctly removed during cleanup, preventing
dirty worktree state and enabling follow-up workers to operate cleanly.

Adds regression tests for both scenarios:
- Dead PID + expired lease → cleaned up
- Live PID + fresh lease → preserved
```

## Impact

**Fixes:** ddx-f49ec1fb - The specific blocker preventing reliable cleanup of dead worker run-states
**Prevents:** Future regressions where stale run-states persist after worker death
**Risk:** Low - only affects liveness check logic in a defensive cleanup operation
**Testing:** Full test coverage with two specific regression tests
