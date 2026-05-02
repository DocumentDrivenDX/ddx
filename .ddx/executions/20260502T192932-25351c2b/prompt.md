<bead-review>
  <bead id="ddx-da11a34a" iter=1>
    <title>ddx work: lock around tracker-commit (git add/commit on primary .git) to prevent multi-worker contention</title>
    <description>
Bead-store locking is intact and well-tested (cli/internal/bead/lock_unix.go + lock_windows.go, plus the TestChaos_* family in chaos_test.go covering concurrent append/close/claim/dep/event-append). Those tests confirm the JSONL writes themselves are atomic and never lose data under concurrent workers.

The failure observed in the Phase 2 drain is NOT bead-store contention. It's at cli/internal/agent/execute_bead.go:1050 in the tracker-commit step:

  // Approximate flow at execute_bead.go:1047–1054:
  msg := fmt.Sprintf("chore: update tracker (execute-bead %s)", time.Now().UTC().Format("20060102T150405"))
  // git add .ddx/beads.jsonl                        ← line ~1050 (staging tracker)
  // git commit -m msg                               ← line ~1054 (committing tracker)

These run against the project's primary .git directory (not a per-worker worktree). When two workers ('ddx work --local' running concurrently) reach this step in overlapping windows, both invoke 'git add' and one fails with:

  fatal: Unable to create '/home/erik/Projects/ddx/.git/index.lock': File exists.

The error surfaces to the loop as 'staging tracker: ...' and the bead is dropped from that drain pass. Observed failures in this drain (single-window): ddx-c77a809c, ddx-cd2ecf79, ddx-6f41aa55.

Fix: wrap the tracker-commit pair (and any other git operations on the primary .git from the agent package) in a process-shared file lock, e.g. flock on .ddx/.git-tracker.lock. Reuse the existing flock primitive pattern from cli/internal/bead/lock_unix.go (and lock_windows.go) — same shape, different lockfile.

Out of scope for this bead:
- Bead-store locking (already works; see chaos_test.go)
- Worktree branch merging in the worktree itself (each worker has its own .ddx/worktrees/&lt;id&gt;/ that's already isolated)
- Federation / multi-machine workers (Story 14)
    </description>
    <acceptance>
1. Tracker-commit pair at cli/internal/agent/execute_bead.go:1050+1054 wrapped in process-shared file lock. 2. Lockfile lives at .ddx/.git-tracker.lock. 3. Lock primitive reuses pattern from cli/internal/bead/lock_unix.go + lock_windows.go (flock-based, with stale-lock-by-age fallback). 4. This bead OWNS the lock-error retry policy implementation, including backoff curve, max retries, and stale-lock recovery. ddx-3c154349 (auto-triage) calls into this implementation but does not duplicate the logic. 5. Lock contention behavior aligns with TD-NNN LockContentionContract (per ddx-673833f4): retry-with-backoff is the default response, NOT bead failure. **No persisted status change from lock contention; bd/br compatibility (ADR-004) preserved.** 6. New tests in cli/internal/agent/: TestTrackerCommit_ConcurrentSafety + TestTrackerCommit_StaleLockRecovery + TestTrackerCommit_RetryBackoffPolicy. 7. After fix: re-run drain with 2 workers; zero 'staging tracker' errors over 5+ minutes.
    </acceptance>
    <labels>phase:2, story:10, area:agent, area:git, kind:fix, observed-failure</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T192347-e4ebfb5e/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="f97caec4e3206084ac5b19aa19f2b26d0bcb707d">
diff --git a/.ddx/executions/20260502T192347-e4ebfb5e/result.json b/.ddx/executions/20260502T192347-e4ebfb5e/result.json
new file mode 100644
index 00000000..224fe7e1
--- /dev/null
+++ b/.ddx/executions/20260502T192347-e4ebfb5e/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-da11a34a",
+  "attempt_id": "20260502T192347-e4ebfb5e",
+  "base_rev": "d9d1c1b8f9d74d3a4aa5da0cc8f04359fe8d33c6",
+  "result_rev": "ba6882a419e0c119968fb34201d971d545f22035",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-fb853175",
+  "duration_ms": 338673,
+  "tokens": 12158,
+  "cost_usd": 1.4380022499999998,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T192347-e4ebfb5e",
+  "prompt_file": ".ddx/executions/20260502T192347-e4ebfb5e/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T192347-e4ebfb5e/manifest.json",
+  "result_file": ".ddx/executions/20260502T192347-e4ebfb5e/result.json",
+  "usage_file": ".ddx/executions/20260502T192347-e4ebfb5e/usage.json",
+  "started_at": "2026-05-02T19:23:48.966760188Z",
+  "finished_at": "2026-05-02T19:29:27.639816473Z"
+}
\ No newline at end of file
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

For each acceptance-criteria (AC) item, decide whether it is implemented correctly, then assign one overall verdict:

- APPROVE — every AC item is fully and correctly implemented.
- REQUEST_CHANGES — some AC items are partial or have fixable minor issues.
- BLOCK — at least one AC item is not implemented or incorrectly implemented; or the diff is insufficient to evaluate.

## Required output format (schema_version: 1)

Respond with EXACTLY one JSON object as your final response, fenced as a single ```json … ``` code block. Do not include any prose outside the fenced block. The JSON must match this schema:

```json
{
  "schema_version": 1,
  "verdict": "APPROVE",
  "summary": "≤300 char human-readable verdict justification",
  "findings": [
    { "severity": "info", "summary": "what is wrong or notable", "location": "path/to/file.go:42" }
  ]
}
```

Rules:
- "verdict" must be exactly one of "APPROVE", "REQUEST_CHANGES", "BLOCK".
- "severity" must be exactly one of "info", "warn", "block".
- Output the JSON object inside ONE fenced ```json … ``` block. No additional prose, no extra fences, no markdown headings.
- Do not echo this template back. Do not write the words APPROVE, REQUEST_CHANGES, or BLOCK anywhere except as the JSON value of the verdict field.
  </instructions>
</bead-review>
