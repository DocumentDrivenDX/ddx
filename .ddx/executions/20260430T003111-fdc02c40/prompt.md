<bead-review>
  <bead id="ddx-2589e256" iter=1>
    <title>routing-overrides-e2e: --harness/--model/--profile flag plumbing + override e2e tests</title>
    <description>
Cover D5 + AC #5 from ddx-fdd3ea36. CLI surface plumbing: ddx agent run --harness X / --model Y / --profile Z must route through unchanged to ResolveRoute(Profile, Harness, Model) and surface upstream typed errors verbatim. ddx work shares the same plumbing. Touched files (expected): cli/cmd/agent_run.go, cli/cmd/work.go, cli/cmd/agent_execute_bead.go, plus any flag-spec helpers.
    </description>
    <acceptance>
1. ddx agent run --harness claude → ResolveRoute(Profile: "default", Harness: "claude"). E2E test mocks gate failures (binary missing, quota exhausted) and asserts typed error surfaces; no silent substitution. 2. ddx agent run --model opus-4.7 → ResolveRoute(Profile: "default", Model: "opus-4.7"). E2E test seeds 3 providers at different cost with shared model; asserts lowest-cost pick. 3. ddx agent run --harness claude --model opus-4.7 → typed ErrHarnessModelIncompatible when invalid; clean dispatch otherwise. E2E coverage of both branches. 4. ddx agent run --profile local --harness claude → upstream typed error ("claude is non-local, profile=local is local-only"); DDx surfaces it unmodified. E2E test. 5. ddx work shares the same plumbing — at least one e2e test exercises it.
    </acceptance>
    <labels>feat-006, routing</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="bcc65fff0eee33efbb29f16349db1793e438381f">
commit bcc65fff0eee33efbb29f16349db1793e438381f
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 29 20:31:09 2026 -0400

    chore: add execution evidence [20260430T000553-]

diff --git a/.ddx/executions/20260430T000553-496eb22c/manifest.json b/.ddx/executions/20260430T000553-496eb22c/manifest.json
new file mode 100644
index 00000000..f7212ec8
--- /dev/null
+++ b/.ddx/executions/20260430T000553-496eb22c/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260430T000553-496eb22c",
+  "bead_id": "ddx-2589e256",
+  "base_rev": "2b6064563f3e25836185c7a789851a60eb4e3f91",
+  "created_at": "2026-04-30T00:05:54.583059033Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-2589e256",
+    "title": "routing-overrides-e2e: --harness/--model/--profile flag plumbing + override e2e tests",
+    "description": "Cover D5 + AC #5 from ddx-fdd3ea36. CLI surface plumbing: ddx agent run --harness X / --model Y / --profile Z must route through unchanged to ResolveRoute(Profile, Harness, Model) and surface upstream typed errors verbatim. ddx work shares the same plumbing. Touched files (expected): cli/cmd/agent_run.go, cli/cmd/work.go, cli/cmd/agent_execute_bead.go, plus any flag-spec helpers.",
+    "acceptance": "1. ddx agent run --harness claude → ResolveRoute(Profile: \"default\", Harness: \"claude\"). E2E test mocks gate failures (binary missing, quota exhausted) and asserts typed error surfaces; no silent substitution. 2. ddx agent run --model opus-4.7 → ResolveRoute(Profile: \"default\", Model: \"opus-4.7\"). E2E test seeds 3 providers at different cost with shared model; asserts lowest-cost pick. 3. ddx agent run --harness claude --model opus-4.7 → typed ErrHarnessModelIncompatible when invalid; clean dispatch otherwise. E2E coverage of both branches. 4. ddx agent run --profile local --harness claude → upstream typed error (\"claude is non-local, profile=local is local-only\"); DDx surfaces it unmodified. E2E test. 5. ddx work shares the same plumbing — at least one e2e test exercises it.",
+    "parent": "ddx-fdd3ea36",
+    "labels": [
+      "feat-006",
+      "routing"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-30T00:05:53Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2278885",
+      "execute-loop-heartbeat-at": "2026-04-30T00:05:53.748311576Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260430T000553-496eb22c",
+    "prompt": ".ddx/executions/20260430T000553-496eb22c/prompt.md",
+    "manifest": ".ddx/executions/20260430T000553-496eb22c/manifest.json",
+    "result": ".ddx/executions/20260430T000553-496eb22c/result.json",
+    "checks": ".ddx/executions/20260430T000553-496eb22c/checks.json",
+    "usage": ".ddx/executions/20260430T000553-496eb22c/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-2589e256-20260430T000553-496eb22c"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260430T000553-496eb22c/result.json b/.ddx/executions/20260430T000553-496eb22c/result.json
new file mode 100644
index 00000000..0dbc0069
--- /dev/null
+++ b/.ddx/executions/20260430T000553-496eb22c/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-2589e256",
+  "attempt_id": "20260430T000553-496eb22c",
+  "base_rev": "2b6064563f3e25836185c7a789851a60eb4e3f91",
+  "result_rev": "d48275c6bbce6d711005fa729115dc3956ca2af5",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-c6b4c698",
+  "duration_ms": 1512173,
+  "tokens": 42737,
+  "cost_usd": 7.20053075,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260430T000553-496eb22c",
+  "prompt_file": ".ddx/executions/20260430T000553-496eb22c/prompt.md",
+  "manifest_file": ".ddx/executions/20260430T000553-496eb22c/manifest.json",
+  "result_file": ".ddx/executions/20260430T000553-496eb22c/result.json",
+  "usage_file": ".ddx/executions/20260430T000553-496eb22c/usage.json",
+  "started_at": "2026-04-30T00:05:54.583340033Z",
+  "finished_at": "2026-04-30T00:31:06.756824759Z"
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
## Review: ddx-2589e256 iter 1

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
