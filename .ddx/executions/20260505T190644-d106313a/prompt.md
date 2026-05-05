<bead-review>
  <bead id="ddx-b669bb9f" iter=1>
    <title>C6: single commitOutcome helper deletes 18 store-error exit paths + handleOutcomeStoreError</title>
    <description>
PROBLEM
The execute-bead loop body (cli/internal/agent/execute_bead_loop.go:875-1036) has 18 store-error exit paths — every bead outcome write can hard-exit the drain loop. This violates the loop's contract: a single bead's store failure should never kill the entire drain worker.

ROOT CAUSE
cli/internal/agent/execute_bead_loop.go:875-1036 contains ~14 direct error returns triggered by store write failures, all routing through handleOutcomeStoreError. Each returns a hard error to Drain instead of scheduling a cooldown and continuing. The helper handleOutcomeStoreError itself is also a terminal-exit path. The 18 exit paths (14 store-error sites + handleOutcomeStoreError variants) are the dominant smell: 18 of 24 returns in the loop body are store-error paths.

PROPOSED FIX
- Add cli/internal/agent/work/commit.go defining commitOutcome(ctx, store, beadID, op func() error).
- commitOutcome calls op(); on error, calls SetExecutionCooldown with 'loop-error' ParkReason and returns nil (never hard-errors to Drain).
- Delete handleOutcomeStoreError.
- Replace all 14 callsites in execute_bead_loop.go (pre-refactor coordinates: 628, 740, 763, 800, 834, 850, 869, 877, 905, 918, 963, 979, 1014, 1026) with commitOutcome.

NON-SCOPE
- Logic changes to what's stored (only how store errors are handled).
- execute_bead_loop.go file rename (separate bead ddx-1d867ec1).

INTERSECTIONS
- P1: store-write failure must not terminate the whole drain loop.
- P3: loop-error fallback should be observable as a structured degradation event.
- P4: a write failure on one bead must not affect later beads.
    </description>
    <acceptance>
1. cli/internal/agent/work/commit.go (or cli/internal/agent/execute_bead_loop.go equivalent) defines commitOutcome(ctx, store, beadID string, op func() error) error.
2. handleOutcomeStoreError deleted from execute_bead_loop.go.
3. All 14 callsites in execute_bead_loop.go (pre-refactor: 628, 740, 763, 800, 834, 850, 869, 877, 905, 918, 963, 979, 1014, 1026) replaced with commitOutcome.
4. TestCommitOutcome_StoreErrorContinuesLoop verifies that a store write failure schedules cooldown and returns nil (loop continues).
5. TestCommitOutcome_StoreError_SchedulesCooldown_NotExit verifies cooldown event is recorded with ParkReason=loop-error.
6. C0 fixture diff: store-glitch-during-close lifecycle byte-identical event stream (loop-error event present, no hard exit).
7. cd cli &amp;&amp; go test ./internal/agent/... green.
8. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, refactor, kind:refactor</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T185505-2726ffeb/checks/production-reachability.json</file>
    <file>.ddx/executions/20260505T185505-2726ffeb/manifest.json</file>
    <file>.ddx/executions/20260505T185505-2726ffeb/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="407cca5b0dc262e0a47ec202a2c2d0bf5257f408">
<untrusted-data>
diff --git a/.ddx/executions/20260505T185505-2726ffeb/checks/production-reachability.json b/.ddx/executions/20260505T185505-2726ffeb/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260505T185505-2726ffeb/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T185505-2726ffeb/manifest.json b/.ddx/executions/20260505T185505-2726ffeb/manifest.json
new file mode 100644
index 00000000..c6bf264a
--- /dev/null
+++ b/.ddx/executions/20260505T185505-2726ffeb/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260505T185505-2726ffeb",
+  "bead_id": "ddx-b669bb9f",
+  "base_rev": "6dc3f78203c42b05c429491b229ae0ea8007453b",
+  "created_at": "2026-05-05T18:55:08.106524693Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-b669bb9f",
+    "title": "C6: single commitOutcome helper deletes 18 store-error exit paths + handleOutcomeStoreError",
+    "description": "PROBLEM\nThe execute-bead loop body (cli/internal/agent/execute_bead_loop.go:875-1036) has 18 store-error exit paths — every bead outcome write can hard-exit the drain loop. This violates the loop's contract: a single bead's store failure should never kill the entire drain worker.\n\nROOT CAUSE\ncli/internal/agent/execute_bead_loop.go:875-1036 contains ~14 direct error returns triggered by store write failures, all routing through handleOutcomeStoreError. Each returns a hard error to Drain instead of scheduling a cooldown and continuing. The helper handleOutcomeStoreError itself is also a terminal-exit path. The 18 exit paths (14 store-error sites + handleOutcomeStoreError variants) are the dominant smell: 18 of 24 returns in the loop body are store-error paths.\n\nPROPOSED FIX\n- Add cli/internal/agent/work/commit.go defining commitOutcome(ctx, store, beadID, op func() error).\n- commitOutcome calls op(); on error, calls SetExecutionCooldown with 'loop-error' ParkReason and returns nil (never hard-errors to Drain).\n- Delete handleOutcomeStoreError.\n- Replace all 14 callsites in execute_bead_loop.go (pre-refactor coordinates: 628, 740, 763, 800, 834, 850, 869, 877, 905, 918, 963, 979, 1014, 1026) with commitOutcome.\n\nNON-SCOPE\n- Logic changes to what's stored (only how store errors are handled).\n- execute_bead_loop.go file rename (separate bead ddx-1d867ec1).\n\nINTERSECTIONS\n- P1: store-write failure must not terminate the whole drain loop.\n- P3: loop-error fallback should be observable as a structured degradation event.\n- P4: a write failure on one bead must not affect later beads.",
+    "acceptance": "1. cli/internal/agent/work/commit.go (or cli/internal/agent/execute_bead_loop.go equivalent) defines commitOutcome(ctx, store, beadID string, op func() error) error.\n2. handleOutcomeStoreError deleted from execute_bead_loop.go.\n3. All 14 callsites in execute_bead_loop.go (pre-refactor: 628, 740, 763, 800, 834, 850, 869, 877, 905, 918, 963, 979, 1014, 1026) replaced with commitOutcome.\n4. TestCommitOutcome_StoreErrorContinuesLoop verifies that a store write failure schedules cooldown and returns nil (loop continues).\n5. TestCommitOutcome_StoreError_SchedulesCooldown_NotExit verifies cooldown event is recorded with ParkReason=loop-error.\n6. C0 fixture diff: store-glitch-during-close lifecycle byte-identical event stream (loop-error event present, no hard exit).\n7. cd cli \u0026\u0026 go test ./internal/agent/... green.\n8. lefthook run pre-commit passes.",
+    "parent": "ddx-5cb6e6cd",
+    "labels": [
+      "phase:2",
+      "refactor",
+      "kind:refactor"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T18:55:05Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3918937",
+      "execute-loop-heartbeat-at": "2026-05-05T18:55:05.434302614Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T185505-2726ffeb",
+    "prompt": ".ddx/executions/20260505T185505-2726ffeb/prompt.md",
+    "manifest": ".ddx/executions/20260505T185505-2726ffeb/manifest.json",
+    "result": ".ddx/executions/20260505T185505-2726ffeb/result.json",
+    "checks": ".ddx/executions/20260505T185505-2726ffeb/checks.json",
+    "usage": ".ddx/executions/20260505T185505-2726ffeb/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-b669bb9f-20260505T185505-2726ffeb"
+  },
+  "prompt_sha": "e35abec18dbbc7f958757893ba89055f87be2dd85be8d1143393c76d68bfcc53"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T185505-2726ffeb/result.json b/.ddx/executions/20260505T185505-2726ffeb/result.json
new file mode 100644
index 00000000..082ab033
--- /dev/null
+++ b/.ddx/executions/20260505T185505-2726ffeb/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-b669bb9f",
+  "attempt_id": "20260505T185505-2726ffeb",
+  "base_rev": "6dc3f78203c42b05c429491b229ae0ea8007453b",
+  "result_rev": "1d0be4c0a9c25aecc377d1a77b02021d89fb619c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-6472fc18",
+  "duration_ms": 687969,
+  "tokens": 9053123,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T185505-2726ffeb",
+  "prompt_file": ".ddx/executions/20260505T185505-2726ffeb/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T185505-2726ffeb/manifest.json",
+  "result_file": ".ddx/executions/20260505T185505-2726ffeb/result.json",
+  "usage_file": ".ddx/executions/20260505T185505-2726ffeb/usage.json",
+  "started_at": "2026-05-05T18:55:08.106956484Z",
+  "finished_at": "2026-05-05T19:06:36.076906914Z"
+}
\ No newline at end of file
</untrusted-data>
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
