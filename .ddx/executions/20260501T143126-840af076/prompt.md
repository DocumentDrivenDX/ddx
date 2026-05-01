<bead-review>
  <bead id="ddx-83bcd4a4" iter=1>
    <title>BeadDetail: h2 title and dt labels use raw text-xl/text-xs instead of semantic tokens</title>
    <description>
src/lib/components/BeadDetail.svelte uses raw Tailwind size classes:

Line 377: h2 uses text-xl — should be text-headline-lg (20px/800) since this is the primary heading of the detail panel
Lines 383, 392, 399, 413, 429, 443, 452, 465, 499: dt elements use text-xs — these are uppercase label/field labels and should use text-label-caps (11px/700/uppercase) since they also carry uppercase and tracking-wide already
Lines 382-383: outer dl uses text-sm — should be text-body-sm (13px)
Lines 479, 481: run/execution row sub-badges use text-[10px] — this is below label-caps (11px) and below the token floor; should be text-label-caps if used as status chips
Line 301: owner span uses text-xs — should be text-body-sm

Claim/Unclaim/Edit/Delete buttons use text-sm — should be text-body-sm.
    </description>
    <acceptance>
h2 uses text-headline-lg. dt uppercase labels use text-label-caps. Body text uses text-body-sm. Sub-badges in runs/executions rows use text-label-caps. No raw text-xs, text-sm, or text-xl remains in BeadDetail.
    </acceptance>
    <labels>area:ui, kind:design, design-tokens</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T142826-d7c8439d/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="ebc9229f59b733b14c8c889775ea700d066f63b6">
diff --git a/.ddx/executions/20260501T142826-d7c8439d/result.json b/.ddx/executions/20260501T142826-d7c8439d/result.json
new file mode 100644
index 00000000..5f7111c3
--- /dev/null
+++ b/.ddx/executions/20260501T142826-d7c8439d/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-83bcd4a4",
+  "attempt_id": "20260501T142826-d7c8439d",
+  "base_rev": "456f43af2207bdc811461c6ab3418c4d71afd85a",
+  "result_rev": "1388ae9824c746045b953a684787761c99016758",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-941bd943",
+  "duration_ms": 174152,
+  "tokens": 10338,
+  "cost_usd": 1.3344052500000003,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T142826-d7c8439d",
+  "prompt_file": ".ddx/executions/20260501T142826-d7c8439d/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T142826-d7c8439d/manifest.json",
+  "result_file": ".ddx/executions/20260501T142826-d7c8439d/result.json",
+  "usage_file": ".ddx/executions/20260501T142826-d7c8439d/usage.json",
+  "started_at": "2026-05-01T14:28:27.769270836Z",
+  "finished_at": "2026-05-01T14:31:21.921513349Z"
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
