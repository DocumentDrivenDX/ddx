<bead-review>
  <bead id="ddx-4e3e149f" iter=1>
    <title>Phase 5h: migrate Runner-method-bound files in agent package (claude_stream, virtual, script, grade, agent_runner)</title>
    <description>
Surfaced by ddx-c0f6b19c v2 STOP report. Several files in cli/internal/agent/ define Runner methods (or method-bound logic) that prevent Runner deletion:
- claude_stream.go: runClaudeStreaming, runClaudeWithFallback, finalizeClaudeResult — Runner methods. The new agentHarness path (commit 2d766818) routes claude through service.Execute internally; these may be dead code already. Audit + delete or migrate.
- virtual.go: RunVirtual — Runner method. Service has the virtual harness internally. Delete or migrate.
- script.go: RunScript — Runner method. Service has the script harness internally. Delete or migrate.
- grade.go: Grade — Runner method. Audit usage; migrate to free function.
- agent_runner.go: RunAgent — main entry. Already dispatches to runAgentViaService when feature flag default-on (post ddx-d224671d). May still have legacy fallback paths. Audit.
- agent_runner_service.go: runAgentViaService, useNewAgentPath — these ARE the new path; methods on Runner. Convert to free functions taking DdxAgent.

After this lands, the only remaining Runner-method definitions should be the ones in routing.go, state.go, routing_signals.go, routing_metrics.go — which delete with their files.
    </description>
    <acceptance>
- [ ] None of the listed files define (*Runner). methods
- [ ] Tests pass
- [ ] Functionality preserved (new agentHarness path remains the live dispatch)
    </acceptance>
    <labels>phase:build, area:agent, kind:migration</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="d8b4c9799ad9d7aa226ffa74eeb04a6fa0c25047">
commit d8b4c9799ad9d7aa226ffa74eeb04a6fa0c25047
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sun Apr 19 19:26:05 2026 -0400

    chore: add execution evidence [20260419T231835-]

diff --git a/.ddx/executions/20260419T231835-0d23e8f6/manifest.json b/.ddx/executions/20260419T231835-0d23e8f6/manifest.json
new file mode 100644
index 00000000..69c257c2
--- /dev/null
+++ b/.ddx/executions/20260419T231835-0d23e8f6/manifest.json
@@ -0,0 +1,56 @@
+{
+  "attempt_id": "20260419T231835-0d23e8f6",
+  "bead_id": "ddx-4e3e149f",
+  "base_rev": "6d06b2c4b5c7a75537cd0ce672e101b8f6049352",
+  "created_at": "2026-04-19T23:18:36.113388142Z",
+  "requested": {
+    "harness": "agent",
+    "model": "minimax/minimax-m2.7",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-4e3e149f",
+    "title": "Phase 5h: migrate Runner-method-bound files in agent package (claude_stream, virtual, script, grade, agent_runner)",
+    "description": "Surfaced by ddx-c0f6b19c v2 STOP report. Several files in cli/internal/agent/ define Runner methods (or method-bound logic) that prevent Runner deletion:\n- claude_stream.go: runClaudeStreaming, runClaudeWithFallback, finalizeClaudeResult — Runner methods. The new agentHarness path (commit 2d766818) routes claude through service.Execute internally; these may be dead code already. Audit + delete or migrate.\n- virtual.go: RunVirtual — Runner method. Service has the virtual harness internally. Delete or migrate.\n- script.go: RunScript — Runner method. Service has the script harness internally. Delete or migrate.\n- grade.go: Grade — Runner method. Audit usage; migrate to free function.\n- agent_runner.go: RunAgent — main entry. Already dispatches to runAgentViaService when feature flag default-on (post ddx-d224671d). May still have legacy fallback paths. Audit.\n- agent_runner_service.go: runAgentViaService, useNewAgentPath — these ARE the new path; methods on Runner. Convert to free functions taking DdxAgent.\n\nAfter this lands, the only remaining Runner-method definitions should be the ones in routing.go, state.go, routing_signals.go, routing_metrics.go — which delete with their files.",
+    "acceptance": "- [ ] None of the listed files define (*Runner). methods\n- [ ] Tests pass\n- [ ] Functionality preserved (new agentHarness path remains the live dispatch)",
+    "parent": "ddx-62b04ccd",
+    "labels": [
+      "phase:build",
+      "area:agent",
+      "kind:migration"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-19T23:18:19Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "721226",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"lmstudio\",\"resolved_model\":\"qwen3.5-27b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-19T23:18:27.78774914Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=lmstudio model=qwen3.5-27b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=cheap harness=lmstudio model=qwen3.5-27b probe=ok\nagent: native config provider \"lmstudio\": config: unknown provider \"lmstudio\"",
+          "created_at": "2026-04-19T23:18:27.993453099Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-19T23:18:19.294582556Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260419T231835-0d23e8f6",
+    "prompt": ".ddx/executions/20260419T231835-0d23e8f6/prompt.md",
+    "manifest": ".ddx/executions/20260419T231835-0d23e8f6/manifest.json",
+    "result": ".ddx/executions/20260419T231835-0d23e8f6/result.json",
+    "checks": ".ddx/executions/20260419T231835-0d23e8f6/checks.json",
+    "usage": ".ddx/executions/20260419T231835-0d23e8f6/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-4e3e149f-20260419T231835-0d23e8f6"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260419T231835-0d23e8f6/result.json b/.ddx/executions/20260419T231835-0d23e8f6/result.json
new file mode 100644
index 00000000..3e33f3a1
--- /dev/null
+++ b/.ddx/executions/20260419T231835-0d23e8f6/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-4e3e149f",
+  "attempt_id": "20260419T231835-0d23e8f6",
+  "base_rev": "6d06b2c4b5c7a75537cd0ce672e101b8f6049352",
+  "result_rev": "970e50929c98dc30f5adaa6b1afca1ddd0bc4392",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "agent",
+  "model": "minimax/minimax-m2.7-20260318",
+  "session_id": "eb-f19af169",
+  "duration_ms": 448291,
+  "tokens": 1931237,
+  "cost_usd": 0.17519574000000007,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260419T231835-0d23e8f6",
+  "prompt_file": ".ddx/executions/20260419T231835-0d23e8f6/prompt.md",
+  "manifest_file": ".ddx/executions/20260419T231835-0d23e8f6/manifest.json",
+  "result_file": ".ddx/executions/20260419T231835-0d23e8f6/result.json",
+  "usage_file": ".ddx/executions/20260419T231835-0d23e8f6/usage.json",
+  "started_at": "2026-04-19T23:18:36.113643059Z",
+  "finished_at": "2026-04-19T23:26:04.405034162Z"
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
## Review: ddx-4e3e149f iter 1

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
