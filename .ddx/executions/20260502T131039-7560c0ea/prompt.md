<bead-review>
  <bead id="ddx-5af9ae07" iter=1>
    <title>artifacts: sort dropdown + staleness chips + URL state contract; client resets after on param change</title>
    <description>
Frontend UI for sort dropdown, staleness filter chips. URL state contract via Story 5's urlState.ts. Client resets pagination cursor on filter/sort/search change.
    </description>
    <acceptance>
1. Sort dropdown visible; URL state round-trips. 2. Staleness chips (fresh/stale/missing) toggle filter. 3. Param change resets cursor. 4. e2e covers sort/filter/search composition.
    </acceptance>
    <labels>phase:2, story:6, area:web, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T130348-342eb3e0/manifest.json</file>
    <file>.ddx/executions/20260502T130348-342eb3e0/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="dd0b14d2904811d4808deb6aabe657e20eaef079">
diff --git a/.ddx/executions/20260502T130348-342eb3e0/manifest.json b/.ddx/executions/20260502T130348-342eb3e0/manifest.json
new file mode 100644
index 00000000..9837d9ef
--- /dev/null
+++ b/.ddx/executions/20260502T130348-342eb3e0/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260502T130348-342eb3e0",
+  "bead_id": "ddx-5af9ae07",
+  "base_rev": "a69e071b9260d387840b13875fc2b7c5e1d874d3",
+  "created_at": "2026-05-02T13:03:49.475763531Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-5af9ae07",
+    "title": "artifacts: sort dropdown + staleness chips + URL state contract; client resets after on param change",
+    "description": "Frontend UI for sort dropdown, staleness filter chips. URL state contract via Story 5's urlState.ts. Client resets pagination cursor on filter/sort/search change.",
+    "acceptance": "1. Sort dropdown visible; URL state round-trips. 2. Staleness chips (fresh/stale/missing) toggle filter. 3. Param change resets cursor. 4. e2e covers sort/filter/search composition.",
+    "parent": "ddx-4728ae0f",
+    "labels": [
+      "phase:2",
+      "story:6",
+      "area:web",
+      "kind:feature"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T13:03:48Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1727028",
+      "execute-loop-heartbeat-at": "2026-05-02T13:03:48.150235364Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T130348-342eb3e0",
+    "prompt": ".ddx/executions/20260502T130348-342eb3e0/prompt.md",
+    "manifest": ".ddx/executions/20260502T130348-342eb3e0/manifest.json",
+    "result": ".ddx/executions/20260502T130348-342eb3e0/result.json",
+    "checks": ".ddx/executions/20260502T130348-342eb3e0/checks.json",
+    "usage": ".ddx/executions/20260502T130348-342eb3e0/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-5af9ae07-20260502T130348-342eb3e0"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T130348-342eb3e0/result.json b/.ddx/executions/20260502T130348-342eb3e0/result.json
new file mode 100644
index 00000000..199d6622
--- /dev/null
+++ b/.ddx/executions/20260502T130348-342eb3e0/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-5af9ae07",
+  "attempt_id": "20260502T130348-342eb3e0",
+  "base_rev": "a69e071b9260d387840b13875fc2b7c5e1d874d3",
+  "result_rev": "d0a5dec0ecf9a377e19ff1009c969710de9bd535",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-5e503866",
+  "duration_ms": 406575,
+  "tokens": 21251,
+  "cost_usd": 2.6569564999999997,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T130348-342eb3e0",
+  "prompt_file": ".ddx/executions/20260502T130348-342eb3e0/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T130348-342eb3e0/manifest.json",
+  "result_file": ".ddx/executions/20260502T130348-342eb3e0/result.json",
+  "usage_file": ".ddx/executions/20260502T130348-342eb3e0/usage.json",
+  "started_at": "2026-05-02T13:03:49.476023574Z",
+  "finished_at": "2026-05-02T13:10:36.051813169Z"
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
