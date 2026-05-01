<bead-review>
  <bead id="ddx-38d773c1" iter=1>
    <title>website: record bootstrap→beads→work demo for website/static/demos/</title>
    <description>
Author a synthetic asciinema v2 cast file at website/static/demos/08-helix-quickstart.cast showing the bootstrap → beads → work flow. Cast files are plain JSON (one header line + timestamped event lines) — see existing files in website/static/demos/ for format. The cast must cover: (1) ddx init in a fresh project dir, (2) ddx install helix, (3) create 2-3 beads with ddx bead create, (4) ddx work draining the queue with realistic agent dispatch output. Target 60-90s playback at normal speed (timestamp the events accordingly). Width=100 cols, height=28 rows. Update the homepage demo player data-src to reference this file instead of 07-quickstart.cast. See website/layouts/index.html for the player mount element.
    </description>
    <acceptance>
60-90s terminal screencast committed to website/static/demos/ covering: ddx init, ddx install helix, create beads, ddx work draining queue with agent dispatch visible; homepage demo player references this file; old 07-quickstart.cast replaced or archived
    </acceptance>
    <notes>
REVIEW:BLOCK

Diff contains only execution metadata (manifest.json, result.json) — no screencast file under website/static/demos/, no homepage player reference change, and no archival of 07-quickstart.cast. None of the acceptance criteria can be verified from the changed files.
    </notes>
    <labels>area:website, demo</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T115115-02d42b2a/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="0b6e9943391e077561f1ef73e443113c982b4770">
diff --git a/.ddx/executions/20260501T115115-02d42b2a/result.json b/.ddx/executions/20260501T115115-02d42b2a/result.json
new file mode 100644
index 00000000..8f6918ab
--- /dev/null
+++ b/.ddx/executions/20260501T115115-02d42b2a/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-38d773c1",
+  "attempt_id": "20260501T115115-02d42b2a",
+  "base_rev": "a378cc284db7edf1c1d3beee5ce509133b4ad919",
+  "result_rev": "d0511aad45f39e97ffa27be122010a10db58c55c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-d55e31d3",
+  "duration_ms": 95448,
+  "tokens": 6622,
+  "cost_usd": 0.602709,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T115115-02d42b2a",
+  "prompt_file": ".ddx/executions/20260501T115115-02d42b2a/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T115115-02d42b2a/manifest.json",
+  "result_file": ".ddx/executions/20260501T115115-02d42b2a/result.json",
+  "usage_file": ".ddx/executions/20260501T115115-02d42b2a/usage.json",
+  "started_at": "2026-05-01T11:51:16.836189275Z",
+  "finished_at": "2026-05-01T11:52:52.284639408Z"
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
