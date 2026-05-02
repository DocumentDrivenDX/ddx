<bead-review>
  <bead id="ddx-238880c4" iter=1>
    <title>B14.4: Spoke lifecycle - --hub= flag, idempotent register, jittered heartbeat, URL-change, deregister</title>
    <description>
Implement spoke-side federation lifecycle on ddx-server: --hub=&lt;host&gt; flag triggers registration on startup. Idempotent register-on-start using stable node_id. Heartbeat every 30s with jitter (avoid synchronized beats); spoke marked stale at 2m of missed beats. Detect own URL change between starts and re-register with updated URL. Deregister on graceful shutdown (best-effort). Both --hub-mode and --hub= can be set on the same server (hub_spoke role). Expose node.federation_role in /api/node response: standalone | hub | spoke | hub_spoke. Integration tests with in-process hub. See /tmp/story-14-final.md 'Authority' and 'Discovery' sections.
    </description>
    <acceptance>
--hub= flag registers on startup. Re-start with same node_id is idempotent (replaces, not duplicates). Heartbeat interval 30s ±jitter. URL change between restarts triggers re-registration with new URL. Graceful shutdown sends deregister (best-effort, no error if hub down). hub_spoke role works (both flags). /api/node exposes federation_role. Integration tests cover lifecycle: register → heartbeat → URL-change → deregister.
    </acceptance>
    <labels>phase:2, story:14</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="f574aaa618b847a28209ef49096910f2d3b4cb19">
commit f574aaa618b847a28209ef49096910f2d3b4cb19
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat May 2 14:00:28 2026 -0400

    chore: add execution evidence [20260502T174947-]

diff --git a/.ddx/executions/20260502T174947-31ad40a4/manifest.json b/.ddx/executions/20260502T174947-31ad40a4/manifest.json
new file mode 100644
index 00000000..806f4eb3
--- /dev/null
+++ b/.ddx/executions/20260502T174947-31ad40a4/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260502T174947-31ad40a4",
+  "bead_id": "ddx-238880c4",
+  "base_rev": "6b1d9b60e962acd00f88dcc409c1655aa01e0afd",
+  "created_at": "2026-05-02T17:49:48.320852723Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-238880c4",
+    "title": "B14.4: Spoke lifecycle - --hub= flag, idempotent register, jittered heartbeat, URL-change, deregister",
+    "description": "Implement spoke-side federation lifecycle on ddx-server: --hub=\u003chost\u003e flag triggers registration on startup. Idempotent register-on-start using stable node_id. Heartbeat every 30s with jitter (avoid synchronized beats); spoke marked stale at 2m of missed beats. Detect own URL change between starts and re-register with updated URL. Deregister on graceful shutdown (best-effort). Both --hub-mode and --hub= can be set on the same server (hub_spoke role). Expose node.federation_role in /api/node response: standalone | hub | spoke | hub_spoke. Integration tests with in-process hub. See /tmp/story-14-final.md 'Authority' and 'Discovery' sections.",
+    "acceptance": "--hub= flag registers on startup. Re-start with same node_id is idempotent (replaces, not duplicates). Heartbeat interval 30s ±jitter. URL change between restarts triggers re-registration with new URL. Graceful shutdown sends deregister (best-effort, no error if hub down). hub_spoke role works (both flags). /api/node exposes federation_role. Integration tests cover lifecycle: register → heartbeat → URL-change → deregister.",
+    "parent": "ddx-a038a090",
+    "labels": [
+      "phase:2",
+      "story:14"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T17:49:47Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3037698",
+      "execute-loop-heartbeat-at": "2026-05-02T17:49:47.100555836Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T174947-31ad40a4",
+    "prompt": ".ddx/executions/20260502T174947-31ad40a4/prompt.md",
+    "manifest": ".ddx/executions/20260502T174947-31ad40a4/manifest.json",
+    "result": ".ddx/executions/20260502T174947-31ad40a4/result.json",
+    "checks": ".ddx/executions/20260502T174947-31ad40a4/checks.json",
+    "usage": ".ddx/executions/20260502T174947-31ad40a4/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-238880c4-20260502T174947-31ad40a4"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T174947-31ad40a4/result.json b/.ddx/executions/20260502T174947-31ad40a4/result.json
new file mode 100644
index 00000000..6abe4b08
--- /dev/null
+++ b/.ddx/executions/20260502T174947-31ad40a4/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-238880c4",
+  "attempt_id": "20260502T174947-31ad40a4",
+  "base_rev": "6b1d9b60e962acd00f88dcc409c1655aa01e0afd",
+  "result_rev": "9b8e85f393ef2888349eadb59eb2d49acce1edad",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-00d6304b",
+  "duration_ms": 637334,
+  "tokens": 35945,
+  "cost_usd": 5.514065999999998,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T174947-31ad40a4",
+  "prompt_file": ".ddx/executions/20260502T174947-31ad40a4/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T174947-31ad40a4/manifest.json",
+  "result_file": ".ddx/executions/20260502T174947-31ad40a4/result.json",
+  "usage_file": ".ddx/executions/20260502T174947-31ad40a4/usage.json",
+  "started_at": "2026-05-02T17:49:48.321096514Z",
+  "finished_at": "2026-05-02T18:00:25.65576278Z"
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
## Review: ddx-238880c4 iter 1

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
