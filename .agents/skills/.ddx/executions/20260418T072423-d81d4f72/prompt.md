<bead-review>
  <bead id="ddx-4802a7b3" iter=1>
    <title>Expose retry-parked beads through ddx bead blocked</title>
    <description>
CONTRACT-001 currently assumes HELIX can learn about retry-parked/suspended execution work through DDx blocker surfacing, but `ddx bead blocked` only returns dependency-blocked open beads. ReadyExecution already honors `execute-loop-retry-after` and execute-loop writes that field, yet the operator-facing/query surface does not classify or emit parked beads.

Implement blocker surfacing for retry-suppressed beads without forcing HELIX to reimplement DDx cooldown logic.
    </description>
    <acceptance>
- [ ] `ddx bead blocked --json` includes retry-suppressed open beads with their existing bead fields plus deterministic blocker metadata naming the blocker kind and next eligible timestamp
- [ ] A fixture with one dep-blocked bead and one retry-parked bead proves both are surfaced distinctly in JSON output
- [ ] Non-JSON output distinguishes dependency blockers from retry/cooldown blockers without dropping existing dep output
- [ ] Existing `ReadyExecution()` cooldown filtering behavior remains unchanged
- [ ] Targeted acceptance test covers retry-after surfacing end to end
    </acceptance>
    <labels>ddx, area:tracker, area:cli, area:execution-boundary</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="570ae403d8e417bbdaf757d2ce6e187ea10cfcf8">
commit 570ae403d8e417bbdaf757d2ce6e187ea10cfcf8
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 18 03:24:21 2026 -0400

    chore: add execution evidence [20260418T071413-]

diff --git a/.ddx/executions/20260418T071413-1bb811c0/manifest.json b/.ddx/executions/20260418T071413-1bb811c0/manifest.json
new file mode 100644
index 00000000..aab92469
--- /dev/null
+++ b/.ddx/executions/20260418T071413-1bb811c0/manifest.json
@@ -0,0 +1,58 @@
+{
+  "attempt_id": "20260418T071413-1bb811c0",
+  "bead_id": "ddx-4802a7b3",
+  "base_rev": "86d0d50f2ce49098df27c57c8dee417d298700f4",
+  "created_at": "2026-04-18T07:14:13.694022632Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-4802a7b3",
+    "title": "Expose retry-parked beads through ddx bead blocked",
+    "description": "CONTRACT-001 currently assumes HELIX can learn about retry-parked/suspended execution work through DDx blocker surfacing, but `ddx bead blocked` only returns dependency-blocked open beads. ReadyExecution already honors `execute-loop-retry-after` and execute-loop writes that field, yet the operator-facing/query surface does not classify or emit parked beads.\n\nImplement blocker surfacing for retry-suppressed beads without forcing HELIX to reimplement DDx cooldown logic.",
+    "acceptance": "- [ ] `ddx bead blocked --json` includes retry-suppressed open beads with their existing bead fields plus deterministic blocker metadata naming the blocker kind and next eligible timestamp\n- [ ] A fixture with one dep-blocked bead and one retry-parked bead proves both are surfaced distinctly in JSON output\n- [ ] Non-JSON output distinguishes dependency blockers from retry/cooldown blockers without dropping existing dep output\n- [ ] Existing `ReadyExecution()` cooldown filtering behavior remains unchanged\n- [ ] Targeted acceptance test covers retry-after surfacing end to end",
+    "labels": [
+      "ddx",
+      "area:tracker",
+      "area:cli",
+      "area:execution-boundary"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-18T07:14:13Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1833233",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"agent\",\"resolved_model\":\"Qwen3.6-35B-A3B-4bit\",\"route_reason\":\"direct-override\",\"fallback_chain\":[],\"base_url\":\"http://vidar:1235/v1\"}",
+          "created_at": "2026-04-18T00:51:07.566461747Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=agent model=Qwen3.6-35B-A3B-4bit reason=direct-override"
+        },
+        {
+          "actor": "erik",
+          "body": "agent: provider error: unexpected end of JSON input\nresult_rev=ca9470b633731c66bfce2483087d6d7ce7682e7b\nbase_rev=ca9470b633731c66bfce2483087d6d7ce7682e7b\nretry_after=2026-04-18T06:51:07Z",
+          "created_at": "2026-04-18T00:51:07.690788778Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-18T07:14:13.368410911Z",
+      "execute-loop-last-detail": "agent: provider error: unexpected end of JSON input",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-04-18T06:51:07Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260418T071413-1bb811c0",
+    "prompt": ".ddx/executions/20260418T071413-1bb811c0/prompt.md",
+    "manifest": ".ddx/executions/20260418T071413-1bb811c0/manifest.json",
+    "result": ".ddx/executions/20260418T071413-1bb811c0/result.json",
+    "checks": ".ddx/executions/20260418T071413-1bb811c0/checks.json",
+    "usage": ".ddx/executions/20260418T071413-1bb811c0/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-4802a7b3-20260418T071413-1bb811c0"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260418T071413-1bb811c0/result.json b/.ddx/executions/20260418T071413-1bb811c0/result.json
new file mode 100644
index 00000000..d579d70d
--- /dev/null
+++ b/.ddx/executions/20260418T071413-1bb811c0/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-4802a7b3",
+  "attempt_id": "20260418T071413-1bb811c0",
+  "base_rev": "86d0d50f2ce49098df27c57c8dee417d298700f4",
+  "result_rev": "7027779bbc4f75c85b157dcbc8396bf0608f5420",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-0fc7b937",
+  "duration_ms": 607492,
+  "tokens": 28135,
+  "cost_usd": 3.7153664999999982,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260418T071413-1bb811c0",
+  "prompt_file": ".ddx/executions/20260418T071413-1bb811c0/prompt.md",
+  "manifest_file": ".ddx/executions/20260418T071413-1bb811c0/manifest.json",
+  "result_file": ".ddx/executions/20260418T071413-1bb811c0/result.json",
+  "usage_file": ".ddx/executions/20260418T071413-1bb811c0/usage.json",
+  "started_at": "2026-04-18T07:14:13.694267841Z",
+  "finished_at": "2026-04-18T07:24:21.186500653Z"
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
## Review: ddx-4802a7b3 iter 1

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
