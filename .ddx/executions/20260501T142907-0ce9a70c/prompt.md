<bead-review>
  <bead id="ddx-1171968d" iter=1>
    <title>NavShell: WS banner uses text-xs / font-label-caps mismatch; brand uses text-base raw size</title>
    <description>
src/lib/components/NavShell.svelte has two token inconsistencies:

1. Line 105 (WebSocket disconnected banner): uses font-label-caps (correct font family token) but also text-xs (raw Tailwind size class). Since font-label-caps is the semantic font-family token, the corresponding size should be text-label-caps (11px/700/uppercase). Currently the size and family tokens are split: the banner applies the family via font-label-caps but the size via the generic text-xs.

2. Line 79 (brand 'DDx' link): uses text-base which is a raw Tailwind size class. This should be text-headline-md (16px/600) or text-headline-lg (20px/800) — whichever was intended. Using text-base bypasses the semantic size+weight pairing.

3. Line 83 (Node label): uses text-xs which should be text-body-sm (13px) or text-label-caps (11px) depending on design intent.
    </description>
    <acceptance>
All three usages replaced with semantic tokens: WS banner uses text-label-caps font-label-caps (both together), brand link uses text-headline-md or text-headline-lg, Node label uses text-body-sm or text-label-caps. No raw text-xs or text-base remains in NavShell.
    </acceptance>
    <labels>area:ui, kind:design, design-tokens</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T142754-a1a25fb1/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="aaa8505ae7bcd2be7ed914011430fb66508e4b9d">
diff --git a/.ddx/executions/20260501T142754-a1a25fb1/result.json b/.ddx/executions/20260501T142754-a1a25fb1/result.json
new file mode 100644
index 00000000..02b4be9a
--- /dev/null
+++ b/.ddx/executions/20260501T142754-a1a25fb1/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-1171968d",
+  "attempt_id": "20260501T142754-a1a25fb1",
+  "base_rev": "91187c6ca9a3f69183c76c15985183cd245fde05",
+  "result_rev": "eb29f779ca154231b986ac179f92fb8d798917b4",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-4651c3a0",
+  "duration_ms": 67700,
+  "tokens": 3699,
+  "cost_usd": 0.46928575,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T142754-a1a25fb1",
+  "prompt_file": ".ddx/executions/20260501T142754-a1a25fb1/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T142754-a1a25fb1/manifest.json",
+  "result_file": ".ddx/executions/20260501T142754-a1a25fb1/result.json",
+  "usage_file": ".ddx/executions/20260501T142754-a1a25fb1/usage.json",
+  "started_at": "2026-05-01T14:27:55.166534038Z",
+  "finished_at": "2026-05-01T14:29:02.867456955Z"
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
