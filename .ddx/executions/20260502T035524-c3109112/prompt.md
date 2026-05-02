<bead-review>
  <bead id="ddx-947cb331" iter=1>
    <title>Author TD-NNN technical design for bead-backed collection abstraction</title>
    <description>
ADR-004 is at architecture level; the implementation needs a TD that picks specifics: (a) collection registry shape, (b) archival trigger policy and parameters, (c) attachment storage layout (.ddx/attachments/&lt;bead-id&gt;/events.jsonl vs inline in beads-archive), (d) migration semantics for the existing 5.4MB beads.jsonl, (e) read-path semantics across active+archive (lazy load? merged-view abstraction?), (f) bd/br interchange compatibility for archive collection. Pick a free TD-NNN ID; depends_on ADR-004, SD-004, FEAT-004.
    </description>
    <acceptance>
1. docs/helix/02-design/technical-designs/TD-&lt;NNN&gt;-bead-collection-abstraction.md exists with sections for each decision (a)-(f) above. 2. Frontmatter declares depends_on: [ADR-004, SD-004, FEAT-004]. 3. Picks concrete defaults for the archival trigger and attachment layout. 4. Includes migration plan for existing beads.jsonl. 5. ddx doc audit shows the new TD with no broken edges.
    </acceptance>
    <labels>area:beads, kind:doc, phase:design</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T035312-575c488b/manifest.json</file>
    <file>.ddx/executions/20260502T035312-575c488b/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="1f723f5be17776452b5851412d39b95717bbe837">
diff --git a/.ddx/executions/20260502T035312-575c488b/manifest.json b/.ddx/executions/20260502T035312-575c488b/manifest.json
new file mode 100644
index 00000000..5a1784ad
--- /dev/null
+++ b/.ddx/executions/20260502T035312-575c488b/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T035312-575c488b",
+  "bead_id": "ddx-947cb331",
+  "base_rev": "500873d3ccecd7d4e0b8e5670b5e2b59d764d033",
+  "created_at": "2026-05-02T03:53:14.338287506Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-947cb331",
+    "title": "Author TD-NNN technical design for bead-backed collection abstraction",
+    "description": "ADR-004 is at architecture level; the implementation needs a TD that picks specifics: (a) collection registry shape, (b) archival trigger policy and parameters, (c) attachment storage layout (.ddx/attachments/\u003cbead-id\u003e/events.jsonl vs inline in beads-archive), (d) migration semantics for the existing 5.4MB beads.jsonl, (e) read-path semantics across active+archive (lazy load? merged-view abstraction?), (f) bd/br interchange compatibility for archive collection. Pick a free TD-NNN ID; depends_on ADR-004, SD-004, FEAT-004.",
+    "acceptance": "1. docs/helix/02-design/technical-designs/TD-\u003cNNN\u003e-bead-collection-abstraction.md exists with sections for each decision (a)-(f) above. 2. Frontmatter declares depends_on: [ADR-004, SD-004, FEAT-004]. 3. Picks concrete defaults for the archival trigger and attachment layout. 4. Includes migration plan for existing beads.jsonl. 5. ddx doc audit shows the new TD with no broken edges.",
+    "parent": "ddx-0c0565f3",
+    "labels": [
+      "area:beads",
+      "kind:doc",
+      "phase:design"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T03:53:12Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1329405",
+      "execute-loop-heartbeat-at": "2026-05-02T03:53:12.863458045Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T035312-575c488b",
+    "prompt": ".ddx/executions/20260502T035312-575c488b/prompt.md",
+    "manifest": ".ddx/executions/20260502T035312-575c488b/manifest.json",
+    "result": ".ddx/executions/20260502T035312-575c488b/result.json",
+    "checks": ".ddx/executions/20260502T035312-575c488b/checks.json",
+    "usage": ".ddx/executions/20260502T035312-575c488b/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-947cb331-20260502T035312-575c488b"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T035312-575c488b/result.json b/.ddx/executions/20260502T035312-575c488b/result.json
new file mode 100644
index 00000000..382b5d34
--- /dev/null
+++ b/.ddx/executions/20260502T035312-575c488b/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-947cb331",
+  "attempt_id": "20260502T035312-575c488b",
+  "base_rev": "500873d3ccecd7d4e0b8e5670b5e2b59d764d033",
+  "result_rev": "243b7a7b71831958632580ec77ea45f2688c20cd",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-128fb1b8",
+  "duration_ms": 125187,
+  "tokens": 7909,
+  "cost_usd": 0.6180875,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T035312-575c488b",
+  "prompt_file": ".ddx/executions/20260502T035312-575c488b/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T035312-575c488b/manifest.json",
+  "result_file": ".ddx/executions/20260502T035312-575c488b/result.json",
+  "usage_file": ".ddx/executions/20260502T035312-575c488b/usage.json",
+  "started_at": "2026-05-02T03:53:14.338586089Z",
+  "finished_at": "2026-05-02T03:55:19.525613499Z"
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
