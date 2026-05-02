<bead-review>
  <bead id="ddx-cb2eb7e3" iter=1>
    <title>Migration: split current beads.jsonl into active + archive + attachments (ADR-004 step 5)</title>
    <description>
One-shot migration tool that splits the existing 5.4MB .ddx/beads.jsonl into the active+archive+attachments layout introduced by earlier steps. Implemented as 'ddx bead migrate-archive' (or equivalent subcommand). Idempotent: re-running on an already-migrated repo is a no-op. Dry-run flag prints what would move without writing. Uses the same archive API as ddx bead archive, so logic is shared.

Depends on ddx-f7f09b6e, ddx-cd1f0f7e, and the archive command in ddx-&lt;archive-id&gt;.
    </description>
    <acceptance>
1. Migration command exists with --dry-run flag. 2. Running it on a fresh checkout of this repo (with the 5.4MB beads.jsonl) produces a beads.jsonl under 4MB, a beads-archive.jsonl with the moved closed beads, and attachment sidecars for their events. 3. Re-running is a no-op (no changes, exit 0). 4. ddx bead show works for migrated beads (active and archived). 5. ddx bead list and ready/blocked still produce sensible output post-migration. 6. Test runs the migration on a synthetic large fixture and verifies the split.
    </acceptance>
    <labels>area:beads, area:storage, kind:tooling, adr:004</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T050110-b24c47a7/manifest.json</file>
    <file>.ddx/executions/20260502T050110-b24c47a7/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="a77bba973dd4992a1331d925fdf60aa500a8728d">
diff --git a/.ddx/executions/20260502T050110-b24c47a7/manifest.json b/.ddx/executions/20260502T050110-b24c47a7/manifest.json
new file mode 100644
index 00000000..8b6d259b
--- /dev/null
+++ b/.ddx/executions/20260502T050110-b24c47a7/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260502T050110-b24c47a7",
+  "bead_id": "ddx-cb2eb7e3",
+  "base_rev": "78e54b6b2eef3fed8f7aec6526cf493d7780cfd8",
+  "created_at": "2026-05-02T05:01:12.260952195Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-cb2eb7e3",
+    "title": "Migration: split current beads.jsonl into active + archive + attachments (ADR-004 step 5)",
+    "description": "One-shot migration tool that splits the existing 5.4MB .ddx/beads.jsonl into the active+archive+attachments layout introduced by earlier steps. Implemented as 'ddx bead migrate-archive' (or equivalent subcommand). Idempotent: re-running on an already-migrated repo is a no-op. Dry-run flag prints what would move without writing. Uses the same archive API as ddx bead archive, so logic is shared.\n\nDepends on ddx-f7f09b6e, ddx-cd1f0f7e, and the archive command in ddx-\u003carchive-id\u003e.",
+    "acceptance": "1. Migration command exists with --dry-run flag. 2. Running it on a fresh checkout of this repo (with the 5.4MB beads.jsonl) produces a beads.jsonl under 4MB, a beads-archive.jsonl with the moved closed beads, and attachment sidecars for their events. 3. Re-running is a no-op (no changes, exit 0). 4. ddx bead show works for migrated beads (active and archived). 5. ddx bead list and ready/blocked still produce sensible output post-migration. 6. Test runs the migration on a synthetic large fixture and verifies the split.",
+    "parent": "ddx-0c0565f3",
+    "labels": [
+      "area:beads",
+      "area:storage",
+      "kind:tooling",
+      "adr:004"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T05:01:10Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1329405",
+      "execute-loop-heartbeat-at": "2026-05-02T05:01:10.950439674Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T050110-b24c47a7",
+    "prompt": ".ddx/executions/20260502T050110-b24c47a7/prompt.md",
+    "manifest": ".ddx/executions/20260502T050110-b24c47a7/manifest.json",
+    "result": ".ddx/executions/20260502T050110-b24c47a7/result.json",
+    "checks": ".ddx/executions/20260502T050110-b24c47a7/checks.json",
+    "usage": ".ddx/executions/20260502T050110-b24c47a7/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-cb2eb7e3-20260502T050110-b24c47a7"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T050110-b24c47a7/result.json b/.ddx/executions/20260502T050110-b24c47a7/result.json
new file mode 100644
index 00000000..335d0cad
--- /dev/null
+++ b/.ddx/executions/20260502T050110-b24c47a7/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-cb2eb7e3",
+  "attempt_id": "20260502T050110-b24c47a7",
+  "base_rev": "78e54b6b2eef3fed8f7aec6526cf493d7780cfd8",
+  "result_rev": "a6cb953401328343e463f9fdaf3c53649a579930",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-cbdd5fea",
+  "duration_ms": 301280,
+  "tokens": 13165,
+  "cost_usd": 1.64855225,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T050110-b24c47a7",
+  "prompt_file": ".ddx/executions/20260502T050110-b24c47a7/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T050110-b24c47a7/manifest.json",
+  "result_file": ".ddx/executions/20260502T050110-b24c47a7/result.json",
+  "usage_file": ".ddx/executions/20260502T050110-b24c47a7/usage.json",
+  "started_at": "2026-05-02T05:01:12.261261864Z",
+  "finished_at": "2026-05-02T05:06:13.541875046Z"
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
