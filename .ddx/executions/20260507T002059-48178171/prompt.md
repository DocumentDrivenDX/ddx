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
    <file>.ddx/executions/20260507T001835-4e374494/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="a6cfd7ac10b75e74639df8742eb49a26224a14c4">
<untrusted-data>
diff --git a/.ddx/executions/20260507T001835-4e374494/result.json b/.ddx/executions/20260507T001835-4e374494/result.json
new file mode 100644
index 000000000..3d33da126
--- /dev/null
+++ b/.ddx/executions/20260507T001835-4e374494/result.json
@@ -0,0 +1,24 @@
+{
+  "bead_id": "ddx-5681cc57",
+  "attempt_id": "20260507T001835-4e374494",
+  "base_rev": "4c4f6226c72ac4519c5c0927678483fd3d8fc326",
+  "result_rev": "e18c8f458b54ca51345b1562630ca632e1bfc41f",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-d939e1a8",
+  "duration_ms": 129770,
+  "tokens": 2082946,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260507T001835-4e374494",
+  "prompt_file": ".ddx/executions/20260507T001835-4e374494/prompt.md",
+  "manifest_file": ".ddx/executions/20260507T001835-4e374494/manifest.json",
+  "result_file": ".ddx/executions/20260507T001835-4e374494/result.json",
+  "usage_file": ".ddx/executions/20260507T001835-4e374494/usage.json",
+  "started_at": "2026-05-07T00:18:38.481707006Z",
+  "finished_at": "2026-05-07T00:20:48.251748717Z"
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
