<bead-review>
  <bead id=".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-3e3913a0" iter=1>
    <title>execute-bead: audit and enforce per-request HTTP deadlines on agent provider calls</title>
    <description>
## Root cause RC4 from ddx-0a651925

After RC1 is fixed (caller ctx propagates), provider HTTP calls still need per-request deadlines. A stalled TCP socket that delivers headers and then stops sending body bytes — particularly during streaming token reads — can hang a goroutine for days.

Grepping the `DocumentDrivenDX/agent@v0.3.14` module cache shows short timeouts only on discovery probes (`provider/openai/discovery.go:46,174`, `openai.go:123`). The main `Run` call path inside agent providers appears to inherit whatever ctx DDx passes — which today is `context.Background()` (see RC1).

## Scope

1. Audit `openai`, `anthropic`, `virtual` providers in the agent library for per-request deadline usage.
2. Install a per-request deadline (default ~10-15 min) on each provider call; separate from the wall-clock bound in RC2.
3. For streaming bodies, enforce an **idle-read limit** on the response body so a mid-stream stall cannot hold a goroutine indefinitely (e.g. `http.Transport.ResponseHeaderTimeout` + periodic `SetReadDeadline` on the underlying conn).
4. Surface provider-timeout failures as a distinct status in `ExecuteBeadResult.Detail`.

## Non-goals

- Rewriting providers; minimally invasive change only.
- Changing DDx-side timeout defaults (RC2 covers outer bound).

## Prerequisite

Sibling RC1 bead must be resolved first, otherwise adding deadlines inside providers is useless while DDx still feeds them `context.Background()`.
    </description>
    <acceptance>
- [ ] Audit report (checked-in .md or bead-comment) lists every `http.Client.Do`-equivalent call in agent providers with its current deadline semantics
- [ ] Each main provider call path installs a per-request deadline (default 15m or catalog-configurable)
- [ ] Streaming body reads terminate within a bounded idle period (ReadDeadline or equivalent)
- [ ] Integration test: fake HTTP server accepts the connection, sends headers, then stops responding. Provider call returns within deadline with a distinct error
    </acceptance>
    <labels>ddx, phase:build, kind:bug, area:agent, area:providers, root-cause:rc4</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="b0a4fce4796e6841c9a9686ebeee197fb22a2fa7">
commit b0a4fce4796e6841c9a9686ebeee197fb22a2fa7
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 18 01:48:05 2026 -0400

    chore: add execution evidence [20260418T053050-]

diff --git a/.ddx/executions/20260418T053050-a607d6d5/manifest.json b/.ddx/executions/20260418T053050-a607d6d5/manifest.json
new file mode 100644
index 00000000..5758585f
--- /dev/null
+++ b/.ddx/executions/20260418T053050-a607d6d5/manifest.json
@@ -0,0 +1,40 @@
+{
+  "attempt_id": "20260418T053050-a607d6d5",
+  "bead_id": ".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-3e3913a0",
+  "base_rev": "fc199a6f6f8a17dbb9b38df858e90a11e2bf7358",
+  "created_at": "2026-04-18T05:30:50.428593539Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-3e3913a0",
+    "title": "execute-bead: audit and enforce per-request HTTP deadlines on agent provider calls",
+    "description": "## Root cause RC4 from ddx-0a651925\n\nAfter RC1 is fixed (caller ctx propagates), provider HTTP calls still need per-request deadlines. A stalled TCP socket that delivers headers and then stops sending body bytes — particularly during streaming token reads — can hang a goroutine for days.\n\nGrepping the `DocumentDrivenDX/agent@v0.3.14` module cache shows short timeouts only on discovery probes (`provider/openai/discovery.go:46,174`, `openai.go:123`). The main `Run` call path inside agent providers appears to inherit whatever ctx DDx passes — which today is `context.Background()` (see RC1).\n\n## Scope\n\n1. Audit `openai`, `anthropic`, `virtual` providers in the agent library for per-request deadline usage.\n2. Install a per-request deadline (default ~10-15 min) on each provider call; separate from the wall-clock bound in RC2.\n3. For streaming bodies, enforce an **idle-read limit** on the response body so a mid-stream stall cannot hold a goroutine indefinitely (e.g. `http.Transport.ResponseHeaderTimeout` + periodic `SetReadDeadline` on the underlying conn).\n4. Surface provider-timeout failures as a distinct status in `ExecuteBeadResult.Detail`.\n\n## Non-goals\n\n- Rewriting providers; minimally invasive change only.\n- Changing DDx-side timeout defaults (RC2 covers outer bound).\n\n## Prerequisite\n\nSibling RC1 bead must be resolved first, otherwise adding deadlines inside providers is useless while DDx still feeds them `context.Background()`.",
+    "acceptance": "- [ ] Audit report (checked-in .md or bead-comment) lists every `http.Client.Do`-equivalent call in agent providers with its current deadline semantics\n- [ ] Each main provider call path installs a per-request deadline (default 15m or catalog-configurable)\n- [ ] Streaming body reads terminate within a bounded idle period (ReadDeadline or equivalent)\n- [ ] Integration test: fake HTTP server accepts the connection, sends headers, then stops responding. Provider call returns within deadline with a distinct error",
+    "parent": "ddx-0a651925",
+    "labels": [
+      "ddx",
+      "phase:build",
+      "kind:bug",
+      "area:agent",
+      "area:providers",
+      "root-cause:rc4"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-18T05:30:50Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1833233",
+      "execute-loop-heartbeat-at": "2026-04-18T05:30:50.083349789Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260418T053050-a607d6d5",
+    "prompt": ".ddx/executions/20260418T053050-a607d6d5/prompt.md",
+    "manifest": ".ddx/executions/20260418T053050-a607d6d5/manifest.json",
+    "result": ".ddx/executions/20260418T053050-a607d6d5/result.json",
+    "checks": ".ddx/executions/20260418T053050-a607d6d5/checks.json",
+    "usage": ".ddx/executions/20260418T053050-a607d6d5/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-3e3913a0-20260418T053050-a607d6d5"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260418T053050-a607d6d5/result.json b/.ddx/executions/20260418T053050-a607d6d5/result.json
new file mode 100644
index 00000000..3441d797
--- /dev/null
+++ b/.ddx/executions/20260418T053050-a607d6d5/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-3e3913a0",
+  "attempt_id": "20260418T053050-a607d6d5",
+  "base_rev": "fc199a6f6f8a17dbb9b38df858e90a11e2bf7358",
+  "result_rev": "9b4631ce37403850e0c4bde5f2efa0fd176b5b53",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-b67b87ce",
+  "duration_ms": 1034128,
+  "tokens": 36101,
+  "cost_usd": 7.46391575,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260418T053050-a607d6d5",
+  "prompt_file": ".ddx/executions/20260418T053050-a607d6d5/prompt.md",
+  "manifest_file": ".ddx/executions/20260418T053050-a607d6d5/manifest.json",
+  "result_file": ".ddx/executions/20260418T053050-a607d6d5/result.json",
+  "usage_file": ".ddx/executions/20260418T053050-a607d6d5/usage.json",
+  "started_at": "2026-04-18T05:30:50.428852706Z",
+  "finished_at": "2026-04-18T05:48:04.557785455Z"
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
## Review: .execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-3e3913a0 iter 1

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
