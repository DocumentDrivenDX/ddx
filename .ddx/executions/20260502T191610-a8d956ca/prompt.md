<bead-review>
  <bead id="ddx-e2c5180b" iter=1>
    <title>B14.8b-pre: Wire FederationProvider into hub-mode GraphQL resolver</title>
    <description>
Production wiring gap discovered while attempting B14.8b (Playwright e2e). The graphql.Resolver.Federation field is never assigned in internal/server/server.go (only in resolver_federation_test.go). Consequence: federationNodes/federatedBeads/federatedRuns/federatedProjects always return [] when running with --hub-mode. /federation page shows no rows even when spokes are registered, blocking any e2e test that asserts UI federation state.

Add an adapter on *Server satisfying graphql.FederationProvider (Spokes(), FanOut(ctx, req)) and wire it into the Resolver only when s.hub != nil. Adapter contract:
- Spokes() returns a defensive copy of registry spokes (via locked read of s.hub.registry).
- FanOut() runs federation.NewFanOutClient() with hubDDxVersion/hubSchemaVer pinned from s.hub, applies StatusUpdates back to s.hub.registry under s.hub.mu so subsequent reads observe offline transitions, and persists registry changes via persistFederationLocked.

Add a Go integration test that boots Server.New + EnableHubMode, registers a spoke via /api/federation/register, posts { federationNodes { nodeId status } } to /graphql, and asserts the spoke is returned (proving the production wiring path).
    </description>
    <acceptance>
1. internal/server/server.go injects Federation: &lt;adapter&gt; into ddxgraphql.Resolver only when s.hub != nil (nil-safe when hub mode is off).
2. Adapter type implements graphql.FederationProvider; Spokes returns defensive copy; FanOut applies StatusUpdates under s.hub.mu and persists registry.
3. Integration test posts federationNodes query and asserts registered spoke surfaces.
4. go test ./internal/server/... ./internal/server/graphql/... is green.
    </acceptance>
    <labels>phase:2, story:14</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T191105-edfdc9c0/manifest.json</file>
    <file>.ddx/executions/20260502T191105-edfdc9c0/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="fe8bc378a2878a0f769572e43f9cd228fb81d6f2">
diff --git a/.ddx/executions/20260502T191105-edfdc9c0/manifest.json b/.ddx/executions/20260502T191105-edfdc9c0/manifest.json
new file mode 100644
index 00000000..3e2f61a4
--- /dev/null
+++ b/.ddx/executions/20260502T191105-edfdc9c0/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T191105-edfdc9c0",
+  "bead_id": "ddx-e2c5180b",
+  "base_rev": "f22586168cd2d834c823dc14f947fd714dfa0d93",
+  "created_at": "2026-05-02T19:11:07.088833534Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-e2c5180b",
+    "title": "B14.8b-pre: Wire FederationProvider into hub-mode GraphQL resolver",
+    "description": "Production wiring gap discovered while attempting B14.8b (Playwright e2e). The graphql.Resolver.Federation field is never assigned in internal/server/server.go (only in resolver_federation_test.go). Consequence: federationNodes/federatedBeads/federatedRuns/federatedProjects always return [] when running with --hub-mode. /federation page shows no rows even when spokes are registered, blocking any e2e test that asserts UI federation state.\n\nAdd an adapter on *Server satisfying graphql.FederationProvider (Spokes(), FanOut(ctx, req)) and wire it into the Resolver only when s.hub != nil. Adapter contract:\n- Spokes() returns a defensive copy of registry spokes (via locked read of s.hub.registry).\n- FanOut() runs federation.NewFanOutClient() with hubDDxVersion/hubSchemaVer pinned from s.hub, applies StatusUpdates back to s.hub.registry under s.hub.mu so subsequent reads observe offline transitions, and persists registry changes via persistFederationLocked.\n\nAdd a Go integration test that boots Server.New + EnableHubMode, registers a spoke via /api/federation/register, posts { federationNodes { nodeId status } } to /graphql, and asserts the spoke is returned (proving the production wiring path).",
+    "acceptance": "1. internal/server/server.go injects Federation: \u003cadapter\u003e into ddxgraphql.Resolver only when s.hub != nil (nil-safe when hub mode is off).\n2. Adapter type implements graphql.FederationProvider; Spokes returns defensive copy; FanOut applies StatusUpdates under s.hub.mu and persists registry.\n3. Integration test posts federationNodes query and asserts registered spoke surfaces.\n4. go test ./internal/server/... ./internal/server/graphql/... is green.",
+    "parent": "ddx-a038a090",
+    "labels": [
+      "phase:2",
+      "story:14"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T19:11:05Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3446284",
+      "execute-loop-heartbeat-at": "2026-05-02T19:11:05.777019341Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T191105-edfdc9c0",
+    "prompt": ".ddx/executions/20260502T191105-edfdc9c0/prompt.md",
+    "manifest": ".ddx/executions/20260502T191105-edfdc9c0/manifest.json",
+    "result": ".ddx/executions/20260502T191105-edfdc9c0/result.json",
+    "checks": ".ddx/executions/20260502T191105-edfdc9c0/checks.json",
+    "usage": ".ddx/executions/20260502T191105-edfdc9c0/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-e2c5180b-20260502T191105-edfdc9c0"
+  },
+  "prompt_sha": "5289e4acf106bb2dd53ea9bc9541e07feebb59575342abc8a7d83f578f76582d"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T191105-edfdc9c0/result.json b/.ddx/executions/20260502T191105-edfdc9c0/result.json
new file mode 100644
index 00000000..0ee081d0
--- /dev/null
+++ b/.ddx/executions/20260502T191105-edfdc9c0/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-e2c5180b",
+  "attempt_id": "20260502T191105-edfdc9c0",
+  "base_rev": "f22586168cd2d834c823dc14f947fd714dfa0d93",
+  "result_rev": "f49f1502f959c7c9dbac6fea5ee2322f8d75df16",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-c6c3d329",
+  "duration_ms": 300286,
+  "tokens": 9364,
+  "cost_usd": 1.55549225,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T191105-edfdc9c0",
+  "prompt_file": ".ddx/executions/20260502T191105-edfdc9c0/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T191105-edfdc9c0/manifest.json",
+  "result_file": ".ddx/executions/20260502T191105-edfdc9c0/result.json",
+  "usage_file": ".ddx/executions/20260502T191105-edfdc9c0/usage.json",
+  "started_at": "2026-05-02T19:11:07.089099783Z",
+  "finished_at": "2026-05-02T19:16:07.375756622Z"
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
