<bead-review>
  <bead id="ddx-a8939a2b" iter=1>
    <title>S15-2: Bead schema/template/status-model — operator-prompt issueType</title>
    <description>
Extend bead schema validator to accept issueType=operator-prompt. Add 'proposed' status to status model. Add prompt→bead template (auto-AC stub, default labels kind:operator-prompt+source:web-ui, default tier from project policy). Enforce no-self-mutation rule: operator-prompt bead executions cannot create/edit/close other operator-prompt beads. See /tmp/story-15-final.md §Implementation #1 and §Additional security controls last bullet.
    </description>
    <acceptance>
issueType=operator-prompt accepted by validator; new 'proposed' status recognized in state machine with valid transitions (proposed→ready, proposed→cancelled); template applied on create populates labels/tier/auto-AC; structural AC check skipped for issueType=operator-prompt; runtime guard prevents operator-prompt beads from mutating operator-prompt beads (unit tests cover allow/deny matrix).
    </acceptance>
    <labels>phase:2, story:15, kind:schema</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T135003-ba252f19/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="2ea4b7289bd637f1d535c89beffb9ed84b6e1312">
diff --git a/.ddx/executions/20260502T135003-ba252f19/result.json b/.ddx/executions/20260502T135003-ba252f19/result.json
new file mode 100644
index 00000000..9c761b06
--- /dev/null
+++ b/.ddx/executions/20260502T135003-ba252f19/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-a8939a2b",
+  "attempt_id": "20260502T135003-ba252f19",
+  "base_rev": "808c4087971cc16a3f9ca2bb5bbf70e29ea8edab",
+  "result_rev": "81e6d3380725f6c39022094728489a39522b0063",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-a6814f63",
+  "duration_ms": 521619,
+  "tokens": 20588,
+  "cost_usd": 3.8985082500000012,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T135003-ba252f19",
+  "prompt_file": ".ddx/executions/20260502T135003-ba252f19/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T135003-ba252f19/manifest.json",
+  "result_file": ".ddx/executions/20260502T135003-ba252f19/result.json",
+  "usage_file": ".ddx/executions/20260502T135003-ba252f19/usage.json",
+  "started_at": "2026-05-02T13:50:04.281022208Z",
+  "finished_at": "2026-05-02T13:58:45.900158102Z"
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
