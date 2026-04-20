<bead-review>
  <bead id="ddx-a97b10cd" iter=1>
    <title>claude harness: write per-iteration trace into execution bundle's embedded/ dir</title>
    <description>
The embedded ddx-agent harness honors the per-run SessionLogDir override (agent_runner.go:136-138: `logDir := opts.SessionLogDir; if empty fall back to r.Config.SessionLogDir`) so that execute-bead runs drop their JSONL trace inside the bundle at .ddx/executions/&lt;attempt&gt;/embedded/agent-*.jsonl.

The claude harness does NOT honor that override. claude_stream.go:327 reads only r.Config.SessionLogDir, so every claude execute-bead run writes its trace into the runner-wide default (`.ddx/agent-logs/`) and the bundle's embedded/ dir stays empty. execute_bead.go:446 already passes SessionLogDir: embeddedStateDir through RunOptions; it just isn't used by the claude path.

Empirical evidence from axon's 2026-04-15 ADR-018 session: 2 runs with `harness: agent` produced 34-51 MB traces in embedded/; 23 runs with `harness: claude` left embedded/ empty while `.ddx/agent-logs/` grew to 162 MB.

Fix: plumb opts.SessionLogDir into runClaudeStreaming (either accept it as a parameter or read it off resolvedOpts) and prefer it over r.Config.SessionLogDir when non-empty. Matches the embedded harness behavior exactly.

Parity matters: users want per-run forensic traces attached to the bundle regardless of which harness ran the bead.
    </description>
    <acceptance>
- [ ] claude_stream.go:runClaudeStreaming uses opts.SessionLogDir when non-empty, falling back to r.Config.SessionLogDir (matches agent_runner.go:136-138 pattern)
- [ ] A claude-harness execute-bead run writes agent-&lt;sessionID&gt;.jsonl into the bundle's embedded/ dir, not .ddx/agent-logs/
- [ ] Unit test covers the override precedence for the claude path
- [ ] Regression test: existing claude runs without execute-bead context still write to DefaultLogDir
- [ ] Any other harnesses that emit JSONL traces (gemini, codex, etc.) audited and brought to the same override-respecting behavior
    </acceptance>
    <labels>ddx, area:agent, area:observability, kind:parity</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="f56dbe210daec8b83f0b857d6b1278ca18bb41c2">
commit f56dbe210daec8b83f0b857d6b1278ca18bb41c2
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 18 02:17:02 2026 -0400

    chore: add execution evidence [20260418T060325-]

diff --git a/.ddx/executions/20260418T060325-e4e5ad24/manifest.json b/.ddx/executions/20260418T060325-e4e5ad24/manifest.json
new file mode 100644
index 00000000..753377ac
--- /dev/null
+++ b/.ddx/executions/20260418T060325-e4e5ad24/manifest.json
@@ -0,0 +1,58 @@
+{
+  "attempt_id": "20260418T060325-e4e5ad24",
+  "bead_id": "ddx-a97b10cd",
+  "base_rev": "65ae837bad24d9a343b50e673649cdd7302fd7ef",
+  "created_at": "2026-04-18T06:03:26.208930749Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-a97b10cd",
+    "title": "claude harness: write per-iteration trace into execution bundle's embedded/ dir",
+    "description": "The embedded ddx-agent harness honors the per-run SessionLogDir override (agent_runner.go:136-138: `logDir := opts.SessionLogDir; if empty fall back to r.Config.SessionLogDir`) so that execute-bead runs drop their JSONL trace inside the bundle at .ddx/executions/\u003cattempt\u003e/embedded/agent-*.jsonl.\n\nThe claude harness does NOT honor that override. claude_stream.go:327 reads only r.Config.SessionLogDir, so every claude execute-bead run writes its trace into the runner-wide default (`.ddx/agent-logs/`) and the bundle's embedded/ dir stays empty. execute_bead.go:446 already passes SessionLogDir: embeddedStateDir through RunOptions; it just isn't used by the claude path.\n\nEmpirical evidence from axon's 2026-04-15 ADR-018 session: 2 runs with `harness: agent` produced 34-51 MB traces in embedded/; 23 runs with `harness: claude` left embedded/ empty while `.ddx/agent-logs/` grew to 162 MB.\n\nFix: plumb opts.SessionLogDir into runClaudeStreaming (either accept it as a parameter or read it off resolvedOpts) and prefer it over r.Config.SessionLogDir when non-empty. Matches the embedded harness behavior exactly.\n\nParity matters: users want per-run forensic traces attached to the bundle regardless of which harness ran the bead.",
+    "acceptance": "- [ ] claude_stream.go:runClaudeStreaming uses opts.SessionLogDir when non-empty, falling back to r.Config.SessionLogDir (matches agent_runner.go:136-138 pattern)\n- [ ] A claude-harness execute-bead run writes agent-\u003csessionID\u003e.jsonl into the bundle's embedded/ dir, not .ddx/agent-logs/\n- [ ] Unit test covers the override precedence for the claude path\n- [ ] Regression test: existing claude runs without execute-bead context still write to DefaultLogDir\n- [ ] Any other harnesses that emit JSONL traces (gemini, codex, etc.) audited and brought to the same override-respecting behavior",
+    "labels": [
+      "ddx",
+      "area:agent",
+      "area:observability",
+      "kind:parity"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-18T06:03:25Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1833233",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"qwen3.6-35b-a3b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-17T18:23:41.351572671Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=qwen3.6-35b-a3b"
+        },
+        {
+          "actor": "ddx",
+          "body": "execution_failed\nresult_rev=c179c1badaeca388e31d3a7ee2c15e1ddd5810da\nbase_rev=c179c1badaeca388e31d3a7ee2c15e1ddd5810da\nretry_after=2026-04-18T00:23:41Z",
+          "created_at": "2026-04-17T18:23:41.55219909Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-18T06:03:25.860319542Z",
+      "execute-loop-last-detail": "execution_failed",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": ""
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260418T060325-e4e5ad24",
+    "prompt": ".ddx/executions/20260418T060325-e4e5ad24/prompt.md",
+    "manifest": ".ddx/executions/20260418T060325-e4e5ad24/manifest.json",
+    "result": ".ddx/executions/20260418T060325-e4e5ad24/result.json",
+    "checks": ".ddx/executions/20260418T060325-e4e5ad24/checks.json",
+    "usage": ".ddx/executions/20260418T060325-e4e5ad24/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-a97b10cd-20260418T060325-e4e5ad24"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260418T060325-e4e5ad24/result.json b/.ddx/executions/20260418T060325-e4e5ad24/result.json
new file mode 100644
index 00000000..8464daed
--- /dev/null
+++ b/.ddx/executions/20260418T060325-e4e5ad24/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-a97b10cd",
+  "attempt_id": "20260418T060325-e4e5ad24",
+  "base_rev": "65ae837bad24d9a343b50e673649cdd7302fd7ef",
+  "result_rev": "473ed5cef3c5c4cc4eea085f35554993ad51db65",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-a0988d58",
+  "duration_ms": 815859,
+  "tokens": 27350,
+  "cost_usd": 4.292281249999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260418T060325-e4e5ad24",
+  "prompt_file": ".ddx/executions/20260418T060325-e4e5ad24/prompt.md",
+  "manifest_file": ".ddx/executions/20260418T060325-e4e5ad24/manifest.json",
+  "result_file": ".ddx/executions/20260418T060325-e4e5ad24/result.json",
+  "usage_file": ".ddx/executions/20260418T060325-e4e5ad24/usage.json",
+  "started_at": "2026-04-18T06:03:26.209193166Z",
+  "finished_at": "2026-04-18T06:17:02.068562287Z"
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
## Review: ddx-a97b10cd iter 1

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
