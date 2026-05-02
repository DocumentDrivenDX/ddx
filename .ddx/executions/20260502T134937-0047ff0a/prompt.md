<bead-review>
  <bead id="ddx-791a4f65" iter=1>
    <title>availability: cached providerModels(name, kind) query + refreshProviderModels mutation</title>
    <description>
Add GraphQL query providerModels(name, kind) returning model identifiers per provider, with a server-side cache (60s TTL by default). Add refreshProviderModels mutation (auth-gated, in-flight guarded, sanitized baseURL). Use a separate cached query rather than bolting onto the 2.5s-polled provider table — fizeau.ListModels does live discovery and would hammer providers.
    </description>
    <acceptance>
1. providerModels query in GraphQL schema. 2. 60s cache; manual refresh via mutation. 3. baseURL sanitized for ts-net safety. 4. In-flight guard prevents duplicate concurrent fetches. 5. Auth: requireTrusted gate.
    </acceptance>
    <labels>phase:2, story:9, area:server, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T134303-e6a2f0d1/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="225929f712f7bbaef2d1b2ef3762fe4bf84eee2b">
diff --git a/.ddx/executions/20260502T134303-e6a2f0d1/result.json b/.ddx/executions/20260502T134303-e6a2f0d1/result.json
new file mode 100644
index 00000000..02c4e61f
--- /dev/null
+++ b/.ddx/executions/20260502T134303-e6a2f0d1/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-791a4f65",
+  "attempt_id": "20260502T134303-e6a2f0d1",
+  "base_rev": "a278f11618f824f2f85c826c4a42acab81198acd",
+  "result_rev": "94bf4527d0b9c326ecbcbfb825ba8b5680cb2531",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-6841d4d4",
+  "duration_ms": 387646,
+  "tokens": 22483,
+  "cost_usd": 3.6492942500000005,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T134303-e6a2f0d1",
+  "prompt_file": ".ddx/executions/20260502T134303-e6a2f0d1/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T134303-e6a2f0d1/manifest.json",
+  "result_file": ".ddx/executions/20260502T134303-e6a2f0d1/result.json",
+  "usage_file": ".ddx/executions/20260502T134303-e6a2f0d1/usage.json",
+  "started_at": "2026-05-02T13:43:05.170607949Z",
+  "finished_at": "2026-05-02T13:49:32.817476943Z"
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
