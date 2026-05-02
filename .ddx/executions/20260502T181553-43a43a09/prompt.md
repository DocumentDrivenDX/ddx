<bead-review>
  <bead id="ddx-586d9431" iter=1>
    <title>B14.6b: Federated GraphQL schema - federationNodes, federatedBeads/Runs/Projects + routing metadata</title>
    <description>
Add federated GraphQL schema/resolvers on the hub: federationNodes (list registered spokes with status/version), federatedBeads, federatedRuns, federatedProjects. Each federated row includes routing metadata: node_id, project_id, project_url/path, write_capability/status. Resolvers use B14.6a fan-out client. Schema designed so existing local views can later migrate to scope: LOCAL | FEDERATION (parallel ship now per codex feedback #8 — do NOT collapse yet). Integration tests against 2 mock spokes.
    </description>
    <acceptance>
Schema additions: federationNodes, federatedBeads, federatedRuns, federatedProjects. Each federated row exposes node_id, project_id, project_url, write_capability/status. Resolvers wired to B14.6a fan-out. Integration tests against 2 mocked spokes verify shape, partial-result, version-skew rendering. Local query types unchanged (parallel-ship).
    </acceptance>
    <labels>phase:2, story:14</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="88e53d2915327e8a9ec5ef4afd77b7c690d18066">
commit 88e53d2915327e8a9ec5ef4afd77b7c690d18066
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat May 2 14:15:51 2026 -0400

    chore: add execution evidence [20260502T180604-]

diff --git a/.ddx/executions/20260502T180604-a2d59986/manifest.json b/.ddx/executions/20260502T180604-a2d59986/manifest.json
new file mode 100644
index 00000000..9ec9e5df
--- /dev/null
+++ b/.ddx/executions/20260502T180604-a2d59986/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260502T180604-a2d59986",
+  "bead_id": "ddx-586d9431",
+  "base_rev": "697de43421479149a4709f4d1df8b715c0346162",
+  "created_at": "2026-05-02T18:06:05.483316705Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-586d9431",
+    "title": "B14.6b: Federated GraphQL schema - federationNodes, federatedBeads/Runs/Projects + routing metadata",
+    "description": "Add federated GraphQL schema/resolvers on the hub: federationNodes (list registered spokes with status/version), federatedBeads, federatedRuns, federatedProjects. Each federated row includes routing metadata: node_id, project_id, project_url/path, write_capability/status. Resolvers use B14.6a fan-out client. Schema designed so existing local views can later migrate to scope: LOCAL | FEDERATION (parallel ship now per codex feedback #8 — do NOT collapse yet). Integration tests against 2 mock spokes.",
+    "acceptance": "Schema additions: federationNodes, federatedBeads, federatedRuns, federatedProjects. Each federated row exposes node_id, project_id, project_url, write_capability/status. Resolvers wired to B14.6a fan-out. Integration tests against 2 mocked spokes verify shape, partial-result, version-skew rendering. Local query types unchanged (parallel-ship).",
+    "parent": "ddx-a038a090",
+    "labels": [
+      "phase:2",
+      "story:14"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T18:06:04Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3037698",
+      "execute-loop-heartbeat-at": "2026-05-02T18:06:04.120203028Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T180604-a2d59986",
+    "prompt": ".ddx/executions/20260502T180604-a2d59986/prompt.md",
+    "manifest": ".ddx/executions/20260502T180604-a2d59986/manifest.json",
+    "result": ".ddx/executions/20260502T180604-a2d59986/result.json",
+    "checks": ".ddx/executions/20260502T180604-a2d59986/checks.json",
+    "usage": ".ddx/executions/20260502T180604-a2d59986/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-586d9431-20260502T180604-a2d59986"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T180604-a2d59986/result.json b/.ddx/executions/20260502T180604-a2d59986/result.json
new file mode 100644
index 00000000..439e571c
--- /dev/null
+++ b/.ddx/executions/20260502T180604-a2d59986/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-586d9431",
+  "attempt_id": "20260502T180604-a2d59986",
+  "base_rev": "697de43421479149a4709f4d1df8b715c0346162",
+  "result_rev": "69e387985cf15581f27f73061d7f33dfc3d08bed",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-aeda3a3d",
+  "duration_ms": 584284,
+  "tokens": 38197,
+  "cost_usd": 4.9006635,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T180604-a2d59986",
+  "prompt_file": ".ddx/executions/20260502T180604-a2d59986/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T180604-a2d59986/manifest.json",
+  "result_file": ".ddx/executions/20260502T180604-a2d59986/result.json",
+  "usage_file": ".ddx/executions/20260502T180604-a2d59986/usage.json",
+  "started_at": "2026-05-02T18:06:05.48358833Z",
+  "finished_at": "2026-05-02T18:15:49.768064252Z"
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
## Review: ddx-586d9431 iter 1

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
