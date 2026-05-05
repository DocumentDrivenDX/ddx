<bead-review>
  <bead id=".execute-bead-wt-.execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398-20260505T024017-db84a9cd-a9e640e8" iter=1>
    <title>bead(axon): scaffold schema snapshot and generated GraphQL client</title>
    <description>
PROBLEM
TD-030 requires a typed Axon client package, but cli/internal/bead/axon_backend.go:20-26 still describes an in-process JSONL emulation. There is no cli/internal/bead/axon/ package yet, so the pinned schema snapshot, GraphQL operation documents, and genqlient output have nowhere to live.

ROOT CAUSE
cli/internal/bead/axon_backend.go:20-26 keeps the backend on the local JSONL-shaped emulation path, which means the repo currently has no package boundary for schema.graphql, query documents, or generated Go bindings. This leaves the TD-030 GraphQL-only client requirement unimplemented.

PROPOSED FIX
- Create cli/internal/bead/axon/schema.graphql as the pinned introspection snapshot for the Axon contract.
- Add query/mutation documents under cli/internal/bead/axon/queries/ for the bead CRUD surface needed by the package.
- Add genqlient config and generated Go bindings under cli/internal/bead/axon/ so the client can be compiled locally.
- Add a compile-focused test that instantiates the generated client surface and proves the bindings match the local package types.

NON-SCOPE
- WebSocket subscription transport.
- Replacing backend_axon.go with the wire implementation.
- Axon server-side schema or resolver work.
    </description>
    <acceptance>
1. cli/internal/bead/axon/ exists with schema.graphql, query documents, genqlient config, and generated Go bindings.
2. TestAxonClient_SchemaBindingsCompile is added in cli/internal/bead/axon/ and exercises the generated package surface.
3. cd cli &amp;&amp; go test ./internal/bead/axon/... passes.
4. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2,  area:beads,  area:storage,  kind:feature,  blocked-on-upstream:axon-05c1019d,  blocked-on-upstream:axon-82b6f7b2,  spec:TD-030</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T024205-4b26207b/checks/production-reachability.json</file>
    <file>.ddx/executions/20260505T024205-4b26207b/manifest.json</file>
    <file>.ddx/executions/20260505T024205-4b26207b/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="b4862af298464e7cf18e77dfd4139bfee131b447">
diff --git a/.ddx/executions/20260505T024205-4b26207b/checks/production-reachability.json b/.ddx/executions/20260505T024205-4b26207b/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260505T024205-4b26207b/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T024205-4b26207b/manifest.json b/.ddx/executions/20260505T024205-4b26207b/manifest.json
new file mode 100644
index 00000000..f8d49566
--- /dev/null
+++ b/.ddx/executions/20260505T024205-4b26207b/manifest.json
@@ -0,0 +1,41 @@
+{
+  "attempt_id": "20260505T024205-4b26207b",
+  "bead_id": ".execute-bead-wt-.execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398-20260505T024017-db84a9cd-a9e640e8",
+  "base_rev": "cd0fa4966d0f6fa50092dbd9f7b3002cedd8f000",
+  "created_at": "2026-05-05T02:42:07.155572027Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-.execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398-20260505T024017-db84a9cd-a9e640e8",
+    "title": "bead(axon): scaffold schema snapshot and generated GraphQL client",
+    "description": "PROBLEM\nTD-030 requires a typed Axon client package, but cli/internal/bead/axon_backend.go:20-26 still describes an in-process JSONL emulation. There is no cli/internal/bead/axon/ package yet, so the pinned schema snapshot, GraphQL operation documents, and genqlient output have nowhere to live.\n\nROOT CAUSE\ncli/internal/bead/axon_backend.go:20-26 keeps the backend on the local JSONL-shaped emulation path, which means the repo currently has no package boundary for schema.graphql, query documents, or generated Go bindings. This leaves the TD-030 GraphQL-only client requirement unimplemented.\n\nPROPOSED FIX\n- Create cli/internal/bead/axon/schema.graphql as the pinned introspection snapshot for the Axon contract.\n- Add query/mutation documents under cli/internal/bead/axon/queries/ for the bead CRUD surface needed by the package.\n- Add genqlient config and generated Go bindings under cli/internal/bead/axon/ so the client can be compiled locally.\n- Add a compile-focused test that instantiates the generated client surface and proves the bindings match the local package types.\n\nNON-SCOPE\n- WebSocket subscription transport.\n- Replacing backend_axon.go with the wire implementation.\n- Axon server-side schema or resolver work.\n",
+    "acceptance": "1. cli/internal/bead/axon/ exists with schema.graphql, query documents, genqlient config, and generated Go bindings.\n2. TestAxonClient_SchemaBindingsCompile is added in cli/internal/bead/axon/ and exercises the generated package surface.\n3. cd cli \u0026\u0026 go test ./internal/bead/axon/... passes.\n4. lefthook run pre-commit passes.",
+    "parent": "ddx-8d747049",
+    "labels": [
+      "phase:2",
+      " area:beads",
+      " area:storage",
+      " kind:feature",
+      " blocked-on-upstream:axon-05c1019d",
+      " blocked-on-upstream:axon-82b6f7b2",
+      " spec:TD-030"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T02:42:05Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
+      "execute-loop-heartbeat-at": "2026-05-05T02:42:05.056496453Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T024205-4b26207b",
+    "prompt": ".ddx/executions/20260505T024205-4b26207b/prompt.md",
+    "manifest": ".ddx/executions/20260505T024205-4b26207b/manifest.json",
+    "result": ".ddx/executions/20260505T024205-4b26207b/result.json",
+    "checks": ".ddx/executions/20260505T024205-4b26207b/checks.json",
+    "usage": ".ddx/executions/20260505T024205-4b26207b/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-.execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398-20260505T024017-db84a9cd-a9e640e8-20260505T024205-4b26207b"
+  },
+  "prompt_sha": "5106d1bcbe176d2ac53a94343ebcf874a61054efb964a73e612280bb312f85da"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T024205-4b26207b/result.json b/.ddx/executions/20260505T024205-4b26207b/result.json
new file mode 100644
index 00000000..6cdf6ba7
--- /dev/null
+++ b/.ddx/executions/20260505T024205-4b26207b/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": ".execute-bead-wt-.execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398-20260505T024017-db84a9cd-a9e640e8",
+  "attempt_id": "20260505T024205-4b26207b",
+  "base_rev": "cd0fa4966d0f6fa50092dbd9f7b3002cedd8f000",
+  "result_rev": "c22efb90e13109ed97e1e2d64059d16517d8eef9",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-466eaff1",
+  "duration_ms": 317751,
+  "tokens": 3160170,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T024205-4b26207b",
+  "prompt_file": ".ddx/executions/20260505T024205-4b26207b/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T024205-4b26207b/manifest.json",
+  "result_file": ".ddx/executions/20260505T024205-4b26207b/result.json",
+  "usage_file": ".ddx/executions/20260505T024205-4b26207b/usage.json",
+  "started_at": "2026-05-05T02:42:07.155862944Z",
+  "finished_at": "2026-05-05T02:47:24.907840871Z"
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
