<bead-review>
  <bead id="ddx-ab7f6184" iter=1>
    <title>B14.3: Hub HTTP API + --hub-mode gating + ts-net policy + version handshake</title>
    <description>
Implement hub-side HTTP API on ddx-server: POST /api/federation/register (idempotent on stable node_id; reject duplicate node_id from different identity with logged conflict; URL change updates registry), POST /api/federation/heartbeat, GET /api/federation/spokes (list), DELETE /api/federation/spokes/:id (deregister). Gate all routes behind --hub-mode flag. Enforce ts-net/loopback peer policy by default; allow --federation-allow-plain-http opt-out (hub-mode only) with WARN log on each accepted plain-HTTP registration. Version handshake at registration: spoke sends ddx_version, schema_version, graphql_schema_version/capability set. Hub rejects incompatible major/schema; marks compatible-but-newer as 'degraded'. Integration tests against in-process httptest server. See /tmp/story-14-final.md 'Authority' and 'ts-net policy' sections.
    </description>
    <acceptance>
Routes mounted only when --hub-mode. Re-register same node_id replaces by id. URL change updates registry. Duplicate node_id from different identity returns 409 + log. Non-loopback non-ts-net peers refused unless --federation-allow-plain-http. Plain-HTTP path emits WARN per accepted registration. Version mismatch: major/schema reject with 4xx + reason; newer minor accepted with degraded status. Integration tests cover all above paths.
    </acceptance>
    <labels>phase:2, story:14</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="fedc522573f6c186fc3b110bada637cbffa09eca">
commit fedc522573f6c186fc3b110bada637cbffa09eca
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat May 2 13:49:11 2026 -0400

    chore: add execution evidence [20260502T174017-]

diff --git a/.ddx/executions/20260502T174017-b417372b/manifest.json b/.ddx/executions/20260502T174017-b417372b/manifest.json
new file mode 100644
index 00000000..7ba38704
--- /dev/null
+++ b/.ddx/executions/20260502T174017-b417372b/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260502T174017-b417372b",
+  "bead_id": "ddx-ab7f6184",
+  "base_rev": "860912e88f1d000d365be546e6ce926438a21021",
+  "created_at": "2026-05-02T17:40:18.622526818Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-ab7f6184",
+    "title": "B14.3: Hub HTTP API + --hub-mode gating + ts-net policy + version handshake",
+    "description": "Implement hub-side HTTP API on ddx-server: POST /api/federation/register (idempotent on stable node_id; reject duplicate node_id from different identity with logged conflict; URL change updates registry), POST /api/federation/heartbeat, GET /api/federation/spokes (list), DELETE /api/federation/spokes/:id (deregister). Gate all routes behind --hub-mode flag. Enforce ts-net/loopback peer policy by default; allow --federation-allow-plain-http opt-out (hub-mode only) with WARN log on each accepted plain-HTTP registration. Version handshake at registration: spoke sends ddx_version, schema_version, graphql_schema_version/capability set. Hub rejects incompatible major/schema; marks compatible-but-newer as 'degraded'. Integration tests against in-process httptest server. See /tmp/story-14-final.md 'Authority' and 'ts-net policy' sections.",
+    "acceptance": "Routes mounted only when --hub-mode. Re-register same node_id replaces by id. URL change updates registry. Duplicate node_id from different identity returns 409 + log. Non-loopback non-ts-net peers refused unless --federation-allow-plain-http. Plain-HTTP path emits WARN per accepted registration. Version mismatch: major/schema reject with 4xx + reason; newer minor accepted with degraded status. Integration tests cover all above paths.",
+    "parent": "ddx-a038a090",
+    "labels": [
+      "phase:2",
+      "story:14"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T17:40:17Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3037698",
+      "execute-loop-heartbeat-at": "2026-05-02T17:40:17.278532066Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T174017-b417372b",
+    "prompt": ".ddx/executions/20260502T174017-b417372b/prompt.md",
+    "manifest": ".ddx/executions/20260502T174017-b417372b/manifest.json",
+    "result": ".ddx/executions/20260502T174017-b417372b/result.json",
+    "checks": ".ddx/executions/20260502T174017-b417372b/checks.json",
+    "usage": ".ddx/executions/20260502T174017-b417372b/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-ab7f6184-20260502T174017-b417372b"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T174017-b417372b/result.json b/.ddx/executions/20260502T174017-b417372b/result.json
new file mode 100644
index 00000000..997867aa
--- /dev/null
+++ b/.ddx/executions/20260502T174017-b417372b/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-ab7f6184",
+  "attempt_id": "20260502T174017-b417372b",
+  "base_rev": "860912e88f1d000d365be546e6ce926438a21021",
+  "result_rev": "3058f7f93e69c754076fcdf9f11439b59fd6d5d2",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-af4e9126",
+  "duration_ms": 529818,
+  "tokens": 29139,
+  "cost_usd": 3.874682000000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T174017-b417372b",
+  "prompt_file": ".ddx/executions/20260502T174017-b417372b/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T174017-b417372b/manifest.json",
+  "result_file": ".ddx/executions/20260502T174017-b417372b/result.json",
+  "usage_file": ".ddx/executions/20260502T174017-b417372b/usage.json",
+  "started_at": "2026-05-02T17:40:18.62286411Z",
+  "finished_at": "2026-05-02T17:49:08.441203536Z"
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
## Review: ddx-ab7f6184 iter 1

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
