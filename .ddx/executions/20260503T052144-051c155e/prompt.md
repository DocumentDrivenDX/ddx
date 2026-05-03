<bead-review>
  <bead id="ddx-b6ab9c0b" iter=1>
    <title>ADR-022 step 8: FEAT-006 amendment — replace rev 3 worker-contract with rev 5 design</title>
    <description>
FEAT-006 currently describes rev 3's long-lived API client with session token / heartbeat / next-bead / result / disconnect. Replace with rev 5's autonomous-default + best-effort-mirror description. ~200 lines docs.
    </description>
    <acceptance>
1. FEAT-006-agent-service.md §'Worker contract (per ADR-022)' rewritten to match rev 5: autonomous worker, server-probe goroutine, best-effort mirror, freshness states, cancel via bead extra. 2. Stale rev 3 mentions of 'session_token', 'long-poll', 'claim_lease', 'heartbeat_interval' removed. 3. Cross-link to ADR-022 commit 484b9a08. 4. Doc validates per existing HELIX consistency check (if any).
    </acceptance>
    <labels>phase:2, area:docs, kind:feature-update, adr:022</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T051808-499156f2/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="b575f925aa5eb6a8091d60961af310ca0f492416">
diff --git a/.ddx/executions/20260503T051808-499156f2/result.json b/.ddx/executions/20260503T051808-499156f2/result.json
new file mode 100644
index 00000000..5939fdc3
--- /dev/null
+++ b/.ddx/executions/20260503T051808-499156f2/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-b6ab9c0b",
+  "attempt_id": "20260503T051808-499156f2",
+  "base_rev": "5a629b7ff8418dda4056d86d13733e9e7f228562",
+  "result_rev": "d016a7f1d8252efffc38173590087ce36b457186",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-f8008128",
+  "duration_ms": 208682,
+  "tokens": 7048,
+  "cost_usd": 0.8283602500000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T051808-499156f2",
+  "prompt_file": ".ddx/executions/20260503T051808-499156f2/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T051808-499156f2/manifest.json",
+  "result_file": ".ddx/executions/20260503T051808-499156f2/result.json",
+  "usage_file": ".ddx/executions/20260503T051808-499156f2/usage.json",
+  "started_at": "2026-05-03T05:18:10.331115261Z",
+  "finished_at": "2026-05-03T05:21:39.013383341Z"
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
