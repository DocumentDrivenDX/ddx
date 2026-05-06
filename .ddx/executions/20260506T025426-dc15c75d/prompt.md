<bead-review>
  <bead id="ddx-208a5bee" iter=1>
    <title>bead/axon: add GraphQL client boundary and row mapping</title>
    <description>
PROBLEM
AxonBackend is still a local JSONL emulation instead of a GraphQL-backed RawBackend. Before mutations can be implemented safely, the backend needs a real client boundary and bead/event row mapping that uses the generated Axon client types.

ROOT CAUSE WITH FILE:LINE
- cli/internal/bead/axon_backend.go:20 explicitly says the implementation serializes collections to JSONL files under &lt;dir&gt;/axon/.
- cli/internal/bead/axon_backend.go:71 defines AxonBackend around BeadsFile, EventsFile, and LockDir instead of a GraphQL client/session.
- cli/internal/bead/axon/generated.go exists, but axon_backend.go does not use the generated client path.

PROPOSED FIX
- Introduce the GraphQL client dependency boundary for AxonBackend using cli/internal/bead/axon generated types.
- Implement bead row and event entity encode/decode helpers against the generated GraphQL shapes, preserving the two-collection model ddx_beads and ddx_bead_events.
- Keep JSONL as the default backend and keep the existing experimental gate behavior unchanged.
- Add unit tests for client construction/mapping that do not require a live Axon server.

NON-SCOPE
- Full CRUD mutation implementation; child bead handles write operations.
- Backend selection config plumbing; existing follow-up handles selection.
- Broad conformance matrix; child bead handles it after operations exist.
    </description>
    <acceptance>
1. AxonBackend has an injectable GraphQL client boundary instead of hard-coding only BeadsFile/EventsFile local snapshots.
2. Mapping helpers cover bead rows and event entities for ddx_beads and ddx_bead_events while preserving event_of linkage and schema version fields.
3. Existing JSONL default backend behavior and DDX_AXON_EXPERIMENTAL gating remain unchanged.
4. TestAxonBackend_GraphQLClientBoundary and TestAxonBackend_GraphQLMapping_BeadsAndEvents verify the client boundary and mapping without a live Axon server.
5. cd cli &amp;&amp; go test ./internal/bead/... -run "TestAxonBackend_GraphQLClientBoundary|TestAxonBackend_GraphQLMapping_BeadsAndEvents|TestAxonExperimental" -count=1 passes.
6. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:beads, area:storage, kind:feature, axon, from:ddx-9c5bca8f</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T025226-3d29d7fd/manifest.json</file>
    <file>.ddx/executions/20260506T025226-3d29d7fd/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="249004c2d05a0b40fda6da5014087cd2db600c8f">
<untrusted-data>
diff --git a/.ddx/executions/20260506T025226-3d29d7fd/manifest.json b/.ddx/executions/20260506T025226-3d29d7fd/manifest.json
new file mode 100644
index 00000000..7c67bfd3
--- /dev/null
+++ b/.ddx/executions/20260506T025226-3d29d7fd/manifest.json
@@ -0,0 +1,83 @@
+{
+  "attempt_id": "20260506T025226-3d29d7fd",
+  "bead_id": "ddx-208a5bee",
+  "base_rev": "1e7c049c3254cbc6e062a84f85c161fee727f67f",
+  "created_at": "2026-05-06T02:52:29.179243349Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-208a5bee",
+    "title": "bead/axon: add GraphQL client boundary and row mapping",
+    "description": "PROBLEM\nAxonBackend is still a local JSONL emulation instead of a GraphQL-backed RawBackend. Before mutations can be implemented safely, the backend needs a real client boundary and bead/event row mapping that uses the generated Axon client types.\n\nROOT CAUSE WITH FILE:LINE\n- cli/internal/bead/axon_backend.go:20 explicitly says the implementation serializes collections to JSONL files under \u003cdir\u003e/axon/.\n- cli/internal/bead/axon_backend.go:71 defines AxonBackend around BeadsFile, EventsFile, and LockDir instead of a GraphQL client/session.\n- cli/internal/bead/axon/generated.go exists, but axon_backend.go does not use the generated client path.\n\nPROPOSED FIX\n- Introduce the GraphQL client dependency boundary for AxonBackend using cli/internal/bead/axon generated types.\n- Implement bead row and event entity encode/decode helpers against the generated GraphQL shapes, preserving the two-collection model ddx_beads and ddx_bead_events.\n- Keep JSONL as the default backend and keep the existing experimental gate behavior unchanged.\n- Add unit tests for client construction/mapping that do not require a live Axon server.\n\nNON-SCOPE\n- Full CRUD mutation implementation; child bead handles write operations.\n- Backend selection config plumbing; existing follow-up handles selection.\n- Broad conformance matrix; child bead handles it after operations exist.",
+    "acceptance": "1. AxonBackend has an injectable GraphQL client boundary instead of hard-coding only BeadsFile/EventsFile local snapshots.\n2. Mapping helpers cover bead rows and event entities for ddx_beads and ddx_bead_events while preserving event_of linkage and schema version fields.\n3. Existing JSONL default backend behavior and DDX_AXON_EXPERIMENTAL gating remain unchanged.\n4. TestAxonBackend_GraphQLClientBoundary and TestAxonBackend_GraphQLMapping_BeadsAndEvents verify the client boundary and mapping without a live Axon server.\n5. cd cli \u0026\u0026 go test ./internal/bead/... -run \"TestAxonBackend_GraphQLClientBoundary|TestAxonBackend_GraphQLMapping_BeadsAndEvents|TestAxonExperimental\" -count=1 passes.\n6. lefthook run pre-commit passes.",
+    "parent": "ddx-9c5bca8f",
+    "labels": [
+      "phase:2",
+      "area:beads",
+      "area:storage",
+      "kind:feature",
+      "axon",
+      "from:ddx-9c5bca8f"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T02:52:26Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T01:50:37.757099057Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T014718-e4871f66\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":2559955,\"output_tokens\":17520,\"total_tokens\":2577475,\"cost_usd\":0,\"duration_ms\":196279,\"exit_code\":0}",
+          "created_at": "2026-05-06T01:50:37.997845494Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=2577475 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T01:50:44.163475837Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=1f7ded2e86362a032b75d79d33411b5869deca87\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T21:55:48-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=9888\noutput_bytes=0\nelapsed_ms=4233",
+          "created_at": "2026-05-06T01:50:48.942875313Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=1f7ded2e86362a032b75d79d33411b5869deca87\nbase_rev=2c004949e5276b49d1b3d67a37d66a729120c3cf",
+          "created_at": "2026-05-06T01:50:49.144075316Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-06T02:52:26.539972482Z",
+      "spec_id": "TD-030"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T025226-3d29d7fd",
+    "prompt": ".ddx/executions/20260506T025226-3d29d7fd/prompt.md",
+    "manifest": ".ddx/executions/20260506T025226-3d29d7fd/manifest.json",
+    "result": ".ddx/executions/20260506T025226-3d29d7fd/result.json",
+    "checks": ".ddx/executions/20260506T025226-3d29d7fd/checks.json",
+    "usage": ".ddx/executions/20260506T025226-3d29d7fd/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-208a5bee-20260506T025226-3d29d7fd"
+  },
+  "prompt_sha": "97e7639ad21fa2171329b6e823366c7afb9ba80985812c1f3d7bfccd36f118ac"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T025226-3d29d7fd/result.json b/.ddx/executions/20260506T025226-3d29d7fd/result.json
new file mode 100644
index 00000000..eac933fb
--- /dev/null
+++ b/.ddx/executions/20260506T025226-3d29d7fd/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-208a5bee",
+  "attempt_id": "20260506T025226-3d29d7fd",
+  "base_rev": "1e7c049c3254cbc6e062a84f85c161fee727f67f",
+  "result_rev": "760edbfca2fdd496d30e8a03e30700453044318f",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-4b887b72",
+  "duration_ms": 110203,
+  "tokens": 802348,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T025226-3d29d7fd",
+  "prompt_file": ".ddx/executions/20260506T025226-3d29d7fd/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T025226-3d29d7fd/manifest.json",
+  "result_file": ".ddx/executions/20260506T025226-3d29d7fd/result.json",
+  "usage_file": ".ddx/executions/20260506T025226-3d29d7fd/usage.json",
+  "started_at": "2026-05-06T02:52:29.179638099Z",
+  "finished_at": "2026-05-06T02:54:19.383625905Z"
+}
\ No newline at end of file
</untrusted-data>
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
