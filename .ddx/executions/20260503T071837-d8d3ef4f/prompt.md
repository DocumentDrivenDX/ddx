<bead-review>
  <bead id="ddx-a418a1c1" iter=1>
    <title>ADR-022 step 6: ddx agent doctor reads from server runtime registry; .ddx/workers/ fallback</title>
    <description>
ddx agent doctor reads worker state from server's runtime registry when available; falls back to .ddx/workers/ when not. On-disk format stays as fallback for one alpha release lag. ~150 LOC.
    </description>
    <acceptance>
1. ddx agent doctor calls server worker query when reachable; falls back when not.
2. Output shows freshness + mirror_failures_count when reading from server.
3. Output consistent across both modes.
4. Tests: TestDoctor_ReadsFromServer_WhenAvailable, TestDoctor_FallsBackToOnDisk_WhenServerDown.
5. cd cli &amp;&amp; go test green; lefthook pre-commit passes.
6. WIRED-IN + TRANSITIONS: integration test (TestDoctor_HandlesServerDownUpDown) sequences: server down → run doctor (fallback path) → server up → run doctor (server path) → server down again → run doctor (fallback again). Asserts each transition produces consistent output for the operator. Not just two static modes.
    </acceptance>
    <labels>phase:2, area:cli, area:agent, kind:feature, adr:022</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T065653-c7305315/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="ac0b00029d398ebb77e60b580b89af5bc1826e76">
diff --git a/.ddx/executions/20260503T065653-c7305315/result.json b/.ddx/executions/20260503T065653-c7305315/result.json
new file mode 100644
index 00000000..f0e4a134
--- /dev/null
+++ b/.ddx/executions/20260503T065653-c7305315/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-a418a1c1",
+  "attempt_id": "20260503T065653-c7305315",
+  "base_rev": "2a93a458e6401b9268c44d7d7d1c92a8d9b9700b",
+  "result_rev": "a9117ebdd23ca9b37a18dc57332a1ca7bc7aa1b4",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-c29094d3",
+  "duration_ms": 1296527,
+  "tokens": 77,
+  "cost_usd": 4.81821375,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T065653-c7305315",
+  "prompt_file": ".ddx/executions/20260503T065653-c7305315/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T065653-c7305315/manifest.json",
+  "result_file": ".ddx/executions/20260503T065653-c7305315/result.json",
+  "usage_file": ".ddx/executions/20260503T065653-c7305315/usage.json",
+  "started_at": "2026-05-03T06:56:54.807926352Z",
+  "finished_at": "2026-05-03T07:18:31.335700888Z"
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
