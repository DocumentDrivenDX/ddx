<bead-review>
  <bead id="ddx-3e60fd84" iter=1>
    <title>checks integration: pre_merge hook in execute-bead loop + checks_bypass annotation + e2e</title>
    <description>
Slice 2 of parent ddx-a946c744. Wire the checks runner (built in child slice ddx-aee651ec) into the execute-bead loop as a pre_merge gate, add the checks_bypass bead annotation with event recording, and add e2e tests.

DEPENDS ON
- ddx-aee651ec (checks protocol package + ddx ac CLI)

CONTEXT
See parent ddx-a946c744 for full design. This slice integrates the protocol package into the merge flow and adds the bypass mechanism.

SCOPE
- cli/internal/agent/execute_bead_loop.go — call cli/internal/checks runner for all applicable checks BEFORE the merge-back step; on any status=block/error, abort merge, preserve worktree per existing iteration-preservation conventions, write evidence under .ddx/executions/&lt;run-id&gt;/, record event
- cli/internal/bead schema — checks_bypass annotation: list of {name, reason, bead}; missing reason -&gt; bypass rejected loudly; on bypass, named check is skipped and a bypass event is recorded with the reason
- E2E test 1: fixture .ddx/checks/dummy-fail.yaml with script that always returns status=block; ddx try on a no-op bead aborts merge, preserves worktree, evidence file present, event recorded
- E2E test 2: same fixture + checks_bypass annotation on bead -&gt; merge proceeds, bypass event recorded
- Tests under cli/internal/agent/ and/or cli/cmd/ following existing execute_bead_*_test.go patterns

OUT OF SCOPE
- New checks for any specific language (separate REACH-* beads)
    </description>
    <acceptance>
1. execute_bead_loop pre_merge step runs all applicable checks (filtered by AppliesTo) in parallel before merge-back.
2. Any status=block or status=error aborts the merge and preserves the worktree per existing iteration-preservation conventions.
3. checks_bypass annotation on the bead skips the named check and records an event with the reason; missing reason -&gt; bypass rejected.
4. E2E test: dummy-fail fixture causes merge abort, worktree preserved, evidence written, event recorded.
5. E2E test: dummy-fail + checks_bypass -&gt; merge proceeds, bypass event recorded.
6. Evidence under .ddx/executions/&lt;run-id&gt;/ shows fixture wrote result.json and ddx parsed it correctly.
7. cd cli &amp;&amp; go test ./... green; lefthook pre-commit passes.
    </acceptance>
    <labels>phase:2, area:agent, area:checks, kind:platform, prevention</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T211820-d1d78258/manifest.json</file>
    <file>.ddx/executions/20260503T211820-d1d78258/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="9ac9cda89048770d6814772a60df5c27393dca6b">
diff --git a/.ddx/executions/20260503T211820-d1d78258/result.json b/.ddx/executions/20260503T211820-d1d78258/result.json
new file mode 100644
index 00000000..018f2d12
--- /dev/null
+++ b/.ddx/executions/20260503T211820-d1d78258/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-3e60fd84",
+  "attempt_id": "20260503T211820-d1d78258",
+  "base_rev": "6a64e42544b24287ae5f6e923620e1a9cd500362",
+  "result_rev": "43efd67eb74320cfb45e93ae96b526d1d97812f3",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-24ccadfc",
+  "duration_ms": 1913170,
+  "tokens": 57686,
+  "cost_usd": 13.152555,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T211820-d1d78258",
+  "prompt_file": ".ddx/executions/20260503T211820-d1d78258/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T211820-d1d78258/manifest.json",
+  "result_file": ".ddx/executions/20260503T211820-d1d78258/result.json",
+  "usage_file": ".ddx/executions/20260503T211820-d1d78258/usage.json",
+  "started_at": "2026-05-03T21:18:25.439928969Z",
+  "finished_at": "2026-05-03T21:50:18.610361836Z"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260503T211820-d1d78258/manifest.json b/.ddx/executions/20260503T211820-d1d78258/manifest.json
new file mode 100644
index 00000000..7c7b5ce2
--- /dev/null
+++ b/.ddx/executions/20260503T211820-d1d78258/manifest.json
@@ -0,0 +1,40 @@
+{
+  "attempt_id": "20260503T211820-d1d78258",
+  "bead_id": "ddx-3e60fd84",
+  "base_rev": "6a64e42544b24287ae5f6e923620e1a9cd500362",
+  "created_at": "2026-05-03T21:18:25.439661428Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-3e60fd84",
+    "title": "checks integration: pre_merge hook in execute-bead loop + checks_bypass annotation + e2e",
+    "description": "Slice 2 of parent ddx-a946c744. Wire the checks runner (built in child slice ddx-aee651ec) into the execute-bead loop as a pre_merge gate, add the checks_bypass bead annotation with event recording, and add e2e tests.\n\nDEPENDS ON\n- ddx-aee651ec (checks protocol package + ddx ac CLI)\n\nCONTEXT\nSee parent ddx-a946c744 for full design. This slice integrates the protocol package into the merge flow and adds the bypass mechanism.\n\nSCOPE\n- cli/internal/agent/execute_bead_loop.go — call cli/internal/checks runner for all applicable checks BEFORE the merge-back step; on any status=block/error, abort merge, preserve worktree per existing iteration-preservation conventions, write evidence under .ddx/executions/\u003crun-id\u003e/, record event\n- cli/internal/bead schema — checks_bypass annotation: list of {name, reason, bead}; missing reason -\u003e bypass rejected loudly; on bypass, named check is skipped and a bypass event is recorded with the reason\n- E2E test 1: fixture .ddx/checks/dummy-fail.yaml with script that always returns status=block; ddx try on a no-op bead aborts merge, preserves worktree, evidence file present, event recorded\n- E2E test 2: same fixture + checks_bypass annotation on bead -\u003e merge proceeds, bypass event recorded\n- Tests under cli/internal/agent/ and/or cli/cmd/ following existing execute_bead_*_test.go patterns\n\nOUT OF SCOPE\n- New checks for any specific language (separate REACH-* beads)",
+    "acceptance": "1. execute_bead_loop pre_merge step runs all applicable checks (filtered by AppliesTo) in parallel before merge-back.\n2. Any status=block or status=error aborts the merge and preserves the worktree per existing iteration-preservation conventions.\n3. checks_bypass annotation on the bead skips the named check and records an event with the reason; missing reason -\u003e bypass rejected.\n4. E2E test: dummy-fail fixture causes merge abort, worktree preserved, evidence written, event recorded.\n5. E2E test: dummy-fail + checks_bypass -\u003e merge proceeds, bypass event recorded.\n6. Evidence under .ddx/executions/\u003crun-id\u003e/ shows fixture wrote result.json and ddx parsed it correctly.\n7. cd cli \u0026\u0026 go test ./... green; lefthook pre-commit passes.",
+    "parent": "ddx-a946c744",
+    "labels": [
+      "phase:2",
+      "area:agent",
+      "area:checks",
+      "kind:platform",
+      "prevention"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-03T21:18:20Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "463338",
+      "execute-loop-heartbeat-at": "2026-05-03T21:18:20.949391651Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260503T211820-d1d78258",
+    "prompt": ".ddx/executions/20260503T211820-d1d78258/prompt.md",
+    "manifest": ".ddx/executions/20260503T211820-d1d78258/manifest.json",
+    "result": ".ddx/executions/20260503T211820-d1d78258/result.json",
+    "checks": ".ddx/executions/20260503T211820-d1d78258/checks.json",
+    "usage": ".ddx/executions/20260503T211820-d1d78258/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-3e60fd84-20260503T211820-d1d78258"
+  },
+  "prompt_sha": "8b191913071fde3355e42240b766a11fe788144de458039f3e11d859e3ef0eaf"
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
