<bead-review>
  <bead id="ddx-240f2082" iter=1>
    <title>C1: try.Outcome sealed sum type + Disposition enum + ParkReason vocabulary</title>
    <description>
Introduce cli/internal/agent/try/outcome.go. No behavior change. ParkReason vocabulary enumerated alongside Disposition to prevent string drift.

Disposition enum (closed set):
  Merged | AlreadyDone | Retry | Park | NeedsHuman | LoopError

ParkReason (string-typed, but enumerated): needs_review, decomposition, push_failed, push_conflict, cost_cap, loop_error, no_changes_unverified, no_changes_unjustified, rate_limit_budget_exhausted, quota_paused, lock_contention.

Outcome { BeadID, AttemptID, BaseRev, ResultRev, SessionID, DurationMS, CostUSD, Disposition, Cooldown time.Duration, ParkReason, RecordEvents []bead.BeadEvent }.

Adapter to/from current ExecuteBeadReport (round-trip parity test).
    </description>
    <acceptance>
1. cli/internal/agent/try/outcome.go defines Outcome + Disposition + ParkReason + ParkReasonValid(). 2. Adapter ToOutcome(report ExecuteBeadReport) + FromOutcome inverse. 3. TestOutcome_RoundTripsExecuteBeadReport. 4. ParkReason vocabulary is a closed Go type with String() + a unit test enumerating all valid values. 5. cd cli &amp;&amp; go test ./internal/agent/try/ green. 6. No callers of try.Outcome yet (introduced in C2).
    </acceptance>
    <labels>phase:2, refactor, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T113218-369b94a3/manifest.json</file>
    <file>.ddx/executions/20260503T113218-369b94a3/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="4f1f48547fc8a6e7e95b9be9c0e46070b25058b4">
diff --git a/.ddx/executions/20260503T113218-369b94a3/manifest.json b/.ddx/executions/20260503T113218-369b94a3/manifest.json
new file mode 100644
index 00000000..e5b21144
--- /dev/null
+++ b/.ddx/executions/20260503T113218-369b94a3/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260503T113218-369b94a3",
+  "bead_id": "ddx-240f2082",
+  "base_rev": "cc1fe00dab7d966da064b25254a196101e750af7",
+  "created_at": "2026-05-03T11:32:20.415810322Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-240f2082",
+    "title": "C1: try.Outcome sealed sum type + Disposition enum + ParkReason vocabulary",
+    "description": "Introduce cli/internal/agent/try/outcome.go. No behavior change. ParkReason vocabulary enumerated alongside Disposition to prevent string drift.\n\nDisposition enum (closed set):\n  Merged | AlreadyDone | Retry | Park | NeedsHuman | LoopError\n\nParkReason (string-typed, but enumerated): needs_review, decomposition, push_failed, push_conflict, cost_cap, loop_error, no_changes_unverified, no_changes_unjustified, rate_limit_budget_exhausted, quota_paused, lock_contention.\n\nOutcome { BeadID, AttemptID, BaseRev, ResultRev, SessionID, DurationMS, CostUSD, Disposition, Cooldown time.Duration, ParkReason, RecordEvents []bead.BeadEvent }.\n\nAdapter to/from current ExecuteBeadReport (round-trip parity test).",
+    "acceptance": "1. cli/internal/agent/try/outcome.go defines Outcome + Disposition + ParkReason + ParkReasonValid(). 2. Adapter ToOutcome(report ExecuteBeadReport) + FromOutcome inverse. 3. TestOutcome_RoundTripsExecuteBeadReport. 4. ParkReason vocabulary is a closed Go type with String() + a unit test enumerating all valid values. 5. cd cli \u0026\u0026 go test ./internal/agent/try/ green. 6. No callers of try.Outcome yet (introduced in C2).",
+    "parent": "ddx-5cb6e6cd",
+    "labels": [
+      "phase:2",
+      "refactor",
+      "kind:feature"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-03T11:32:18Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "463338",
+      "execute-loop-heartbeat-at": "2026-05-03T11:32:18.483424507Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260503T113218-369b94a3",
+    "prompt": ".ddx/executions/20260503T113218-369b94a3/prompt.md",
+    "manifest": ".ddx/executions/20260503T113218-369b94a3/manifest.json",
+    "result": ".ddx/executions/20260503T113218-369b94a3/result.json",
+    "checks": ".ddx/executions/20260503T113218-369b94a3/checks.json",
+    "usage": ".ddx/executions/20260503T113218-369b94a3/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-240f2082-20260503T113218-369b94a3"
+  },
+  "prompt_sha": "9e2d23f508d2a854c78e41b76deafb5456ae6e894b00774188a40721d82b701b"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260503T113218-369b94a3/result.json b/.ddx/executions/20260503T113218-369b94a3/result.json
new file mode 100644
index 00000000..c5971911
--- /dev/null
+++ b/.ddx/executions/20260503T113218-369b94a3/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-240f2082",
+  "attempt_id": "20260503T113218-369b94a3",
+  "base_rev": "cc1fe00dab7d966da064b25254a196101e750af7",
+  "result_rev": "4d5cc5a1cf8d2729a70c30f4c7c039e64771529e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-6ae22626",
+  "duration_ms": 299394,
+  "tokens": 14330,
+  "cost_usd": 1.19326925,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T113218-369b94a3",
+  "prompt_file": ".ddx/executions/20260503T113218-369b94a3/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T113218-369b94a3/manifest.json",
+  "result_file": ".ddx/executions/20260503T113218-369b94a3/result.json",
+  "usage_file": ".ddx/executions/20260503T113218-369b94a3/usage.json",
+  "started_at": "2026-05-03T11:32:20.416066363Z",
+  "finished_at": "2026-05-03T11:37:19.811027894Z"
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
