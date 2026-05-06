<bead-review>
  <bead id="ddx-5681cc57" iter=1>
    <title>perf: 2k fixture + baseline non-gating measurement</title>
    <description>
PROBLEM
No performance baseline exists for the documents/artifacts UI at realistic corpus size (2000 artifacts). Without a baseline, the caching work (ddx-4a7eed8c) cannot demonstrate improvement, and CI has no reference point for regression detection.

ROOT CAUSE
- cli/internal/server/frontend/e2e/fixtures/ does not contain a scale fixture for 2000 artifacts.
- No measurement of first-paint cold/warm latency, scroll smoothness, or search latency at this scale has been recorded.
- ddx-b9993722 (FEAT-008 + TP-002 measurement contract, a dep) established the measurement methodology; this bead applies it to the 2k scale.

PROPOSED FIX
- Create a synthetic 2000-artifact fixture under cli/internal/server/frontend/e2e/fixtures/scale/.
- Measure: first paint cold/warm, scroll smoothness (frame rate under scroll), search latency (50ms goal from parent perf epic).
- Record baseline measurements in .ddx/executions/&lt;run-id&gt;/perf-baseline.md.
- Do NOT add a CI gate (baseline only — gate added in a later bead after caching lands).

NON-SCOPE
- Caching implementation (ddx-4a7eed8c).
- CI gate (deferred).
    </description>
    <acceptance>
1. Synthetic 2000-artifact fixture exists at cli/internal/server/frontend/e2e/fixtures/scale/ (JSON or equivalent format loadable by the dev server).
2. Baseline measurements captured at .ddx/executions/&lt;run-id&gt;/perf-baseline.md including: first-paint cold, first-paint warm, scroll smoothness, search latency.
3. No CI gate added.
4. Fixture loads without crash or timeout in Playwright e2e (test('2k fixture loads without crash', ...)).
5. bun run test:e2e green.
6. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, story:7, area:tests, kind:test</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T043749-7c9fea7a/manifest.json</file>
    <file>.ddx/executions/20260506T043749-7c9fea7a/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="93a0c1235e0641879a0b8520fa4d025b5e3665e0">
<untrusted-data>
diff --git a/.ddx/executions/20260506T043749-7c9fea7a/manifest.json b/.ddx/executions/20260506T043749-7c9fea7a/manifest.json
new file mode 100644
index 00000000..f976c26b
--- /dev/null
+++ b/.ddx/executions/20260506T043749-7c9fea7a/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260506T043749-7c9fea7a",
+  "bead_id": "ddx-5681cc57",
+  "base_rev": "e90842def4b7dc5c54534172c2d2c6ddd50df3a1",
+  "created_at": "2026-05-06T04:37:52.231558147Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-5681cc57",
+    "title": "perf: 2k fixture + baseline non-gating measurement",
+    "description": "PROBLEM\nNo performance baseline exists for the documents/artifacts UI at realistic corpus size (2000 artifacts). Without a baseline, the caching work (ddx-4a7eed8c) cannot demonstrate improvement, and CI has no reference point for regression detection.\n\nROOT CAUSE\n- cli/internal/server/frontend/e2e/fixtures/ does not contain a scale fixture for 2000 artifacts.\n- No measurement of first-paint cold/warm latency, scroll smoothness, or search latency at this scale has been recorded.\n- ddx-b9993722 (FEAT-008 + TP-002 measurement contract, a dep) established the measurement methodology; this bead applies it to the 2k scale.\n\nPROPOSED FIX\n- Create a synthetic 2000-artifact fixture under cli/internal/server/frontend/e2e/fixtures/scale/.\n- Measure: first paint cold/warm, scroll smoothness (frame rate under scroll), search latency (50ms goal from parent perf epic).\n- Record baseline measurements in .ddx/executions/\u003crun-id\u003e/perf-baseline.md.\n- Do NOT add a CI gate (baseline only — gate added in a later bead after caching lands).\n\nNON-SCOPE\n- Caching implementation (ddx-4a7eed8c).\n- CI gate (deferred).",
+    "acceptance": "1. Synthetic 2000-artifact fixture exists at cli/internal/server/frontend/e2e/fixtures/scale/ (JSON or equivalent format loadable by the dev server).\n2. Baseline measurements captured at .ddx/executions/\u003crun-id\u003e/perf-baseline.md including: first-paint cold, first-paint warm, scroll smoothness, search latency.\n3. No CI gate added.\n4. Fixture loads without crash or timeout in Playwright e2e (test('2k fixture loads without crash', ...)).\n5. bun run test:e2e green.\n6. lefthook run pre-commit passes.",
+    "parent": "ddx-97335425",
+    "labels": [
+      "phase:2",
+      "story:7",
+      "area:tests",
+      "kind:test"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T04:37:49Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "execute-loop-heartbeat-at": "2026-05-06T04:37:49.877557229Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T043749-7c9fea7a",
+    "prompt": ".ddx/executions/20260506T043749-7c9fea7a/prompt.md",
+    "manifest": ".ddx/executions/20260506T043749-7c9fea7a/manifest.json",
+    "result": ".ddx/executions/20260506T043749-7c9fea7a/result.json",
+    "checks": ".ddx/executions/20260506T043749-7c9fea7a/checks.json",
+    "usage": ".ddx/executions/20260506T043749-7c9fea7a/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-5681cc57-20260506T043749-7c9fea7a"
+  },
+  "prompt_sha": "9bac810c582beb08247eecf9e7c71fb56d376c651bb9032e4016ad091a4914ea"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T043749-7c9fea7a/result.json b/.ddx/executions/20260506T043749-7c9fea7a/result.json
new file mode 100644
index 00000000..d98e26fb
--- /dev/null
+++ b/.ddx/executions/20260506T043749-7c9fea7a/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-5681cc57",
+  "attempt_id": "20260506T043749-7c9fea7a",
+  "base_rev": "e90842def4b7dc5c54534172c2d2c6ddd50df3a1",
+  "result_rev": "e0d149846b426126cfd601dd214fa65fd5faeacb",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-f45f7852",
+  "duration_ms": 302515,
+  "tokens": 5307073,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T043749-7c9fea7a",
+  "prompt_file": ".ddx/executions/20260506T043749-7c9fea7a/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T043749-7c9fea7a/manifest.json",
+  "result_file": ".ddx/executions/20260506T043749-7c9fea7a/result.json",
+  "usage_file": ".ddx/executions/20260506T043749-7c9fea7a/usage.json",
+  "started_at": "2026-05-06T04:37:52.231893063Z",
+  "finished_at": "2026-05-06T04:42:54.747813783Z"
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
