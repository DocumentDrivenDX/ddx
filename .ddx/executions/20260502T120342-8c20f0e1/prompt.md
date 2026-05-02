<bead-review>
  <bead id="ddx-bffb23ae" iter=1>
    <title>B3: Add prompt_sha to manifest.json + render-size CI fixture</title>
    <description>
Lightweight v1 measurement per /tmp/story-12-final.md §B3. (1) Add deterministic test fixture that renders both execute-bead variants for a representative bead at each contextBudget and prints word/byte counts as a CI artifact (catches accidental bloat). (2) Add prompt_sha (sha256 of rendered prompt) to manifest.json for every attempt, so existing analytics in resolver_feat008.go can group attempts by prompt_sha for before/after comparison. Treats prompt as opaque — can run in parallel with B1/B2.
    </description>
    <acceptance>
AC7: manifest.json contains prompt_sha for every attempt; size-report fixture emits word/byte counts per (variant, contextBudget) and is wired so CI surfaces it as an artifact.
    </acceptance>
    <labels>phase:2, story:12, tier:cheap</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T115700-ab7bd6bd/manifest.json</file>
    <file>.ddx/executions/20260502T115700-ab7bd6bd/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="e791001aa257447cfc6ab8eeed177ab4d7e451db">
diff --git a/.ddx/executions/20260502T115700-ab7bd6bd/manifest.json b/.ddx/executions/20260502T115700-ab7bd6bd/manifest.json
new file mode 100644
index 00000000..877ce242
--- /dev/null
+++ b/.ddx/executions/20260502T115700-ab7bd6bd/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T115700-ab7bd6bd",
+  "bead_id": "ddx-bffb23ae",
+  "base_rev": "282bac2d9e855d59cca4451af62bfbaa695ee2a4",
+  "created_at": "2026-05-02T11:57:01.486266682Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-bffb23ae",
+    "title": "B3: Add prompt_sha to manifest.json + render-size CI fixture",
+    "description": "Lightweight v1 measurement per /tmp/story-12-final.md §B3. (1) Add deterministic test fixture that renders both execute-bead variants for a representative bead at each contextBudget and prints word/byte counts as a CI artifact (catches accidental bloat). (2) Add prompt_sha (sha256 of rendered prompt) to manifest.json for every attempt, so existing analytics in resolver_feat008.go can group attempts by prompt_sha for before/after comparison. Treats prompt as opaque — can run in parallel with B1/B2.",
+    "acceptance": "AC7: manifest.json contains prompt_sha for every attempt; size-report fixture emits word/byte counts per (variant, contextBudget) and is wired so CI surfaces it as an artifact.",
+    "parent": "ddx-a61bf8ee",
+    "labels": [
+      "phase:2",
+      "story:12",
+      "tier:cheap"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T11:57:00Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1724970",
+      "execute-loop-heartbeat-at": "2026-05-02T11:57:00.24778639Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T115700-ab7bd6bd",
+    "prompt": ".ddx/executions/20260502T115700-ab7bd6bd/prompt.md",
+    "manifest": ".ddx/executions/20260502T115700-ab7bd6bd/manifest.json",
+    "result": ".ddx/executions/20260502T115700-ab7bd6bd/result.json",
+    "checks": ".ddx/executions/20260502T115700-ab7bd6bd/checks.json",
+    "usage": ".ddx/executions/20260502T115700-ab7bd6bd/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-bffb23ae-20260502T115700-ab7bd6bd"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T115700-ab7bd6bd/result.json b/.ddx/executions/20260502T115700-ab7bd6bd/result.json
new file mode 100644
index 00000000..e31f3686
--- /dev/null
+++ b/.ddx/executions/20260502T115700-ab7bd6bd/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-bffb23ae",
+  "attempt_id": "20260502T115700-ab7bd6bd",
+  "base_rev": "282bac2d9e855d59cca4451af62bfbaa695ee2a4",
+  "result_rev": "efbed403d81810fd410ebefb0f3399a8a72cb773",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-671ba0dc",
+  "duration_ms": 397756,
+  "tokens": 17975,
+  "cost_usd": 2.7821322499999988,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T115700-ab7bd6bd",
+  "prompt_file": ".ddx/executions/20260502T115700-ab7bd6bd/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T115700-ab7bd6bd/manifest.json",
+  "result_file": ".ddx/executions/20260502T115700-ab7bd6bd/result.json",
+  "usage_file": ".ddx/executions/20260502T115700-ab7bd6bd/usage.json",
+  "started_at": "2026-05-02T11:57:01.48657139Z",
+  "finished_at": "2026-05-02T12:03:39.242785238Z"
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
