<bead-review>
  <bead id="ddx-65543530" iter=1>
    <title>runs: canonicalize Run data projection (fix state_runs.go bundle layering; lossless AgentSession join)</title>
    <description>
state_runs.go:164 currently synthesizes execution bundles as RunLayerRun; per the three-layer model bundles should be RunLayerTry. Fix the layer assignment. Add lossless join with AgentSession (which carries billingMode, cached tokens, prompt/response/stderr, outcome — NOT in Run today). User decision: keep AgentSession as backing store under layer=run.
    </description>
    <acceptance>
1. state_runs.go: bundles → RunLayerTry; agent sessions → RunLayerRun. 2. Run resolver returns lossless join (Run + AgentSession fields when applicable). 3. Existing runs tests pass; add new test for layering correctness. 4. cd cli &amp;&amp; go test ./internal/server/... passes.
    </acceptance>
    <labels>phase:2, story:8, area:server, kind:fix</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="ffba0262001df7021bebaa74421078032fa1597a">
commit ffba0262001df7021bebaa74421078032fa1597a
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat May 2 22:16:07 2026 -0400

    chore: add execution evidence [20260503T020341-]

diff --git a/.ddx/executions/20260503T020341-8fe56f98/result.json b/.ddx/executions/20260503T020341-8fe56f98/result.json
new file mode 100644
index 00000000..35350b9f
--- /dev/null
+++ b/.ddx/executions/20260503T020341-8fe56f98/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-65543530",
+  "attempt_id": "20260503T020341-8fe56f98",
+  "base_rev": "fe3f22236cd513b1f895589e41576bb703d34921",
+  "result_rev": "0b607fc363b5314b2a89deeec08ff73cd8bc3a19",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-aef669b2",
+  "duration_ms": 741592,
+  "tokens": 26836,
+  "cost_usd": 5.042985250000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T020341-8fe56f98",
+  "prompt_file": ".ddx/executions/20260503T020341-8fe56f98/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T020341-8fe56f98/manifest.json",
+  "result_file": ".ddx/executions/20260503T020341-8fe56f98/result.json",
+  "usage_file": ".ddx/executions/20260503T020341-8fe56f98/usage.json",
+  "started_at": "2026-05-03T02:03:42.601377665Z",
+  "finished_at": "2026-05-03T02:16:04.194303938Z"
+}
\ No newline at end of file
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

## Your task

Examine the diff and each acceptance-criteria (AC) item. For each item assign one grade:

- **APPROVE** — fully and correctly implemented; cite the specific file path and line that proves it.
- **REQUEST_CHANGES** — partially implemented or has fixable minor issues.
- **BLOCK** — not implemented, incorrectly implemented, or the diff is insufficient to evaluate.

Overall verdict rule:
- All items APPROVE → **APPROVE**
- Any item BLOCK → **BLOCK**
- Otherwise → **REQUEST_CHANGES**

## Required output format

Respond with a structured review using exactly this layout (replace placeholder text):

---
## Review: ddx-65543530 iter 1

### Verdict: APPROVE | REQUEST_CHANGES | BLOCK

### AC Grades

| # | Item | Grade | Evidence |
|---|------|-------|----------|
| 1 | &lt;AC item text, max 60 chars&gt; | APPROVE | path/to/file.go:42 — brief note |
| 2 | &lt;AC item text, max 60 chars&gt; | BLOCK   | — not found in diff |

### Summary

&lt;1–3 sentences on overall implementation quality and any recurring theme in findings.&gt;

### Findings

&lt;Bullet list of REQUEST_CHANGES and BLOCK findings. Each finding must name the specific file, function, or test that is missing or wrong — specific enough for the next agent to act on without re-reading the entire diff. Omit this section entirely if verdict is APPROVE.&gt;
  </instructions>
</bead-review>
