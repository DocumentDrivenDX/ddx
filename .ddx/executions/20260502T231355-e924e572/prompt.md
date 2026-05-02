<bead-review>
  <bead id="ddx-6bb51a52" iter=1>
    <title>TD: design axon-backend integration for bead tracker</title>
    <description>
Author a technical design doc for adopting axon as the primary bead-tracker backend. Cover: data-model mapping from current bead schema (id, status, claim, events, deps) to axon collections; claim/lock semantics (replacing flock + git tracker-commit lock); archive policy (events array growth, when/how to externalize per ADR-004); multi-machine concurrency story (Story 14 federation prerequisite); JSONL interchange compatibility for bd/br round-trip; failure modes and migration strategy. Land the doc as .ddx/td/TD-axon-bead-backend.md (or under docs/helix/ if the project convention dictates — check existing TDs first). No code changes.
    </description>
    <acceptance>
1. TD doc lands at the correct in-repo path covering all six topics above. 2. Doc references ADR-004 / SD-004 / FEAT-004 and the lock-contention v1 patch (ddx-da11a34a). 3. Doc proposes a concrete backend interface (Go) for cli/internal/bead/backend.go that both JSONL and axon can implement. 4. Doc identifies the chaos_test.go contract the new backend must satisfy.
    </acceptance>
    <labels>phase:2, area:beads, area:storage, kind:design, backend-migration</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T231106-49f4e558/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="44cf6e9e787f55e24d3bee28b7bb481ec2123fa7">
diff --git a/.ddx/executions/20260502T231106-49f4e558/result.json b/.ddx/executions/20260502T231106-49f4e558/result.json
new file mode 100644
index 00000000..7422f594
--- /dev/null
+++ b/.ddx/executions/20260502T231106-49f4e558/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-6bb51a52",
+  "attempt_id": "20260502T231106-49f4e558",
+  "base_rev": "0a47cf7d7fe0077e0dea5b899a9a8a1ede81c6b7",
+  "result_rev": "9c3cbefb4f1973b6908162730b602899c04f5345",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-05490a3e",
+  "duration_ms": 161571,
+  "tokens": 8197,
+  "cost_usd": 0.8203104999999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T231106-49f4e558",
+  "prompt_file": ".ddx/executions/20260502T231106-49f4e558/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T231106-49f4e558/manifest.json",
+  "result_file": ".ddx/executions/20260502T231106-49f4e558/result.json",
+  "usage_file": ".ddx/executions/20260502T231106-49f4e558/usage.json",
+  "started_at": "2026-05-02T23:11:07.817635933Z",
+  "finished_at": "2026-05-02T23:13:49.389163691Z"
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
