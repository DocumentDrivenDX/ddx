<bead-review>
  <bead id="ddx-d3a4f89c" iter=1>
    <title>layer-2 design: should execute-bead workers create beads in-band?</title>
    <description>
During investigation of ddx-7eab13a6 (bead-id resolver worktree bug), a design question surfaced: is it correct for execute-bead workers to call `ddx bead create` directly in-band?

Three alternatives:

(a) Append to parent bead. Worker appends discovered sub-tasks as structured items in the parent's notes or a new discovered-subtask event. The parent becomes the durable record; future runs or the operator can decompose.

(b) Surface via result. Agent emits a structured discovered_subtasks: [...] field in result.json; the execute-loop / operator decides whether to file new beads. Keeps tracker mutations gated by an explicit decision point.

(c) Create beads in-band (current behavior). Worker calls ddx bead create directly. Pro: fastest decomposition. Con: sub-tasks land in the queue without operator review; can flood the queue on over-decomposition.

Today's behavior is (c) but undocumented. One worker spawned 11 read-coverage children in a single run (ddx-44236615), all P0, without operator review. The work may be valid but the pattern needs a documented position.

Additionally, 22 existing beads have malformed IDs of the form .execute-bead-wt-&lt;parent&gt;-&lt;timestamp&gt;-&lt;random&gt;-&lt;hex&gt; (caused by the layer-1 bug fixed in ddx-7eab13a6). A mass-repair or close+refile decision should be made once the design position is set.

Scope: pick (a), (b), or (c) and document in a spec amendment (FEAT-006 or FEAT-010 if applicable); record the rationale. Also decide whether to repair the 22 existing malformed-ID beads.
    </description>
    <acceptance/>
    <labels>area:bead, area:agent, kind:design</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T232303-1edce776/manifest.json</file>
    <file>.ddx/executions/20260429T232303-1edce776/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="ad42d24fcb5f39f4ab8c43e68a729b6abe466822">
diff --git a/.ddx/executions/20260429T232303-1edce776/manifest.json b/.ddx/executions/20260429T232303-1edce776/manifest.json
new file mode 100644
index 00000000..a973cfea
--- /dev/null
+++ b/.ddx/executions/20260429T232303-1edce776/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260429T232303-1edce776",
+  "bead_id": "ddx-d3a4f89c",
+  "base_rev": "55b56e4de35da5d53a429ba1ef53504fd8fe40bd",
+  "created_at": "2026-04-29T23:23:03.984197281Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-d3a4f89c",
+    "title": "layer-2 design: should execute-bead workers create beads in-band?",
+    "description": "During investigation of ddx-7eab13a6 (bead-id resolver worktree bug), a design question surfaced: is it correct for execute-bead workers to call `ddx bead create` directly in-band?\n\nThree alternatives:\n\n(a) Append to parent bead. Worker appends discovered sub-tasks as structured items in the parent's notes or a new discovered-subtask event. The parent becomes the durable record; future runs or the operator can decompose.\n\n(b) Surface via result. Agent emits a structured discovered_subtasks: [...] field in result.json; the execute-loop / operator decides whether to file new beads. Keeps tracker mutations gated by an explicit decision point.\n\n(c) Create beads in-band (current behavior). Worker calls ddx bead create directly. Pro: fastest decomposition. Con: sub-tasks land in the queue without operator review; can flood the queue on over-decomposition.\n\nToday's behavior is (c) but undocumented. One worker spawned 11 read-coverage children in a single run (ddx-44236615), all P0, without operator review. The work may be valid but the pattern needs a documented position.\n\nAdditionally, 22 existing beads have malformed IDs of the form .execute-bead-wt-\u003cparent\u003e-\u003ctimestamp\u003e-\u003crandom\u003e-\u003chex\u003e (caused by the layer-1 bug fixed in ddx-7eab13a6). A mass-repair or close+refile decision should be made once the design position is set.\n\nScope: pick (a), (b), or (c) and document in a spec amendment (FEAT-006 or FEAT-010 if applicable); record the rationale. Also decide whether to repair the 22 existing malformed-ID beads.",
+    "labels": [
+      "area:bead",
+      "area:agent",
+      "kind:design"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T23:22:59Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T23:22:59.722607085Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T232303-1edce776",
+    "prompt": ".ddx/executions/20260429T232303-1edce776/prompt.md",
+    "manifest": ".ddx/executions/20260429T232303-1edce776/manifest.json",
+    "result": ".ddx/executions/20260429T232303-1edce776/result.json",
+    "checks": ".ddx/executions/20260429T232303-1edce776/checks.json",
+    "usage": ".ddx/executions/20260429T232303-1edce776/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-d3a4f89c-20260429T232303-1edce776"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T232303-1edce776/result.json b/.ddx/executions/20260429T232303-1edce776/result.json
new file mode 100644
index 00000000..1306a593
--- /dev/null
+++ b/.ddx/executions/20260429T232303-1edce776/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-d3a4f89c",
+  "attempt_id": "20260429T232303-1edce776",
+  "base_rev": "55b56e4de35da5d53a429ba1ef53504fd8fe40bd",
+  "result_rev": "e7149f17705fdd1b0f96aff2fe6ab361fb5381f5",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-9d8046f9",
+  "duration_ms": 172153,
+  "tokens": 5677,
+  "cost_usd": 0.49991675,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T232303-1edce776",
+  "prompt_file": ".ddx/executions/20260429T232303-1edce776/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T232303-1edce776/manifest.json",
+  "result_file": ".ddx/executions/20260429T232303-1edce776/result.json",
+  "usage_file": ".ddx/executions/20260429T232303-1edce776/usage.json",
+  "started_at": "2026-04-29T23:23:03.984467198Z",
+  "finished_at": "2026-04-29T23:25:56.137790878Z"
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
