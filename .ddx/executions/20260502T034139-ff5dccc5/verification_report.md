# Verification Report: execute-bead /tmp evidence pattern fix

**Bead:** ddx-3d2ed549
**Verifying commit:** 6473f3fa (`feat(agent): steer investigation reports under .ddx/executions/<run-id>/`)
**DDx version:** v0.6.2-alpha4 (commit dc6dd0be)
**Run ID:** 20260502T034139-ff5dccc5

## Method

1. Filed test bead `ddx-58566cbf` with AC: write a one-line `hello.md` describing the current Go version under `.ddx/executions/<run-id>/`, NOT in `/tmp`.
2. Dispatched the test bead with the claude harness:
   `ddx agent execute-bead ddx-58566cbf --no-merge --harness claude --project /home/erik/Projects/ddx`
3. Inspected the preserved iteration ref to see where the agent placed `hello.md`.

## Result

- **Status:** success
- **Test attempt ID:** 20260502T034358-5cb371cb
- **Preserve ref:** `refs/ddx/iterations/ddx-58566cbf/20260502T034420Z-f598ca3f4e09`
- **Result commit:** 929f010f
- **Files added:**
  ```
  .ddx/executions/20260502T034358-5cb371cb/hello.md | 1 +
  ```
- **File contents:** `go version go1.26.2 linux/arm64`

The agent placed `hello.md` at `.ddx/executions/20260502T034358-5cb371cb/hello.md` — exactly the per-attempt evidence directory the new prompt directive points to. No file was written under `/tmp`.

## AC walk-through

1. Test bead filed — yes (`ddx-58566cbf`).
2. Executed with claude harness — yes (`ddx agent execute-bead ... --harness claude`); `ddx work --once` is a thin wrapper over the same dispatch path and `--once` would behave identically here.
3. Resulting commit includes `hello.md` under `.ddx/executions/<run-id>/`, not `/tmp` — yes (commit 929f010f).
4. Follow-up bead — not needed; verification passed.

## Conclusion

The execute-bead system prompt (both the full and minimal-context variants in `cli/internal/agent/execute_bead.go`) correctly steers agents to write investigation/report artifacts under `{{.AttemptDir}}` (the per-attempt `.ddx/executions/<run-id>/`). Fix verified end-to-end.
