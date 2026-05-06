<bead-review>
  <bead id="ddx-6d5c436e" iter=1>
    <title>bead/axon: implement GraphQL-backed CRUD, deps, and events</title>
    <description>
PROBLEM
After the client boundary exists, AxonBackend still needs real GraphQL CRUD, dependency, and event operations. The parent bead currently asks for too many operations at once and has repeatedly produced no_changes_needs_investigation.

ROOT CAUSE WITH FILE:LINE
- cli/internal/bead/axon_backend.go:123 reads from local collection files, not Axon queries.
- cli/internal/bead/axon_backend.go:175 rewrites local collection files, not Axon mutations.
- cli/internal/bead/store.go higher-level operations depend on RawBackend ReadAll/WriteAll/WithLock semantics, so the GraphQL backend must preserve store behavior while changing storage transport.

PROPOSED FIX
- Implement GraphQL-backed Create, Get, Update, Claim/Unclaim, Close, DepAdd, DepRemove, and AppendEvent behavior through the RawBackend integration path.
- Preserve optimistic/concurrent update expectations from the existing store tests.
- Keep event history split into ddx_bead_events and linked to bead rows through event_of.
- Keep local JSONL backend untouched.

NON-SCOPE
- Backend selection config plumbing.
- Import/export conformance matrix beyond targeted operation tests.
    </description>
    <acceptance>
1. TestAxonBackend_CreateAndGet passes against the GraphQL-backed implementation.
2. TestAxonBackend_UpdateMutatesAndPersists passes against the GraphQL-backed implementation.
3. TestAxonBackend_ClaimAndUnclaim passes against the GraphQL-backed implementation.
4. TestAxonBackend_DepAddRemoveAndTree passes against the GraphQL-backed implementation.
5. TestAxonBackend_AppendEventSplitsIntoEventsCollection passes and verifies event_of links.
6. backend_axon.go / axon_backend.go no longer use local JSONL snapshots as the primary storage path when AxonBackend is selected with a GraphQL client.
7. cd cli &amp;&amp; go test ./internal/bead/... -run "TestAxonBackend_CreateAndGet|TestAxonBackend_UpdateMutatesAndPersists|TestAxonBackend_ClaimAndUnclaim|TestAxonBackend_DepAddRemoveAndTree|TestAxonBackend_AppendEventSplitsIntoEventsCollection" -count=1 passes.
8. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:beads, area:storage, kind:feature, axon, from:ddx-9c5bca8f, triage:no-changes-unjustified</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T065501-c01d5cf1/manifest.json</file>
    <file>.ddx/executions/20260506T065501-c01d5cf1/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="4c1cf81624c32446fc3893b55f6b722660a573a4">
<untrusted-data>
diff --git a/.ddx/executions/20260506T065501-c01d5cf1/manifest.json b/.ddx/executions/20260506T065501-c01d5cf1/manifest.json
new file mode 100644
index 00000000..51c058b3
--- /dev/null
+++ b/.ddx/executions/20260506T065501-c01d5cf1/manifest.json
@@ -0,0 +1,117 @@
+{
+  "attempt_id": "20260506T065501-c01d5cf1",
+  "bead_id": "ddx-6d5c436e",
+  "base_rev": "78abc98ac47cad77a98b1969cc478763a8692192",
+  "created_at": "2026-05-06T06:55:04.059871432Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-6d5c436e",
+    "title": "bead/axon: implement GraphQL-backed CRUD, deps, and events",
+    "description": "PROBLEM\nAfter the client boundary exists, AxonBackend still needs real GraphQL CRUD, dependency, and event operations. The parent bead currently asks for too many operations at once and has repeatedly produced no_changes_needs_investigation.\n\nROOT CAUSE WITH FILE:LINE\n- cli/internal/bead/axon_backend.go:123 reads from local collection files, not Axon queries.\n- cli/internal/bead/axon_backend.go:175 rewrites local collection files, not Axon mutations.\n- cli/internal/bead/store.go higher-level operations depend on RawBackend ReadAll/WriteAll/WithLock semantics, so the GraphQL backend must preserve store behavior while changing storage transport.\n\nPROPOSED FIX\n- Implement GraphQL-backed Create, Get, Update, Claim/Unclaim, Close, DepAdd, DepRemove, and AppendEvent behavior through the RawBackend integration path.\n- Preserve optimistic/concurrent update expectations from the existing store tests.\n- Keep event history split into ddx_bead_events and linked to bead rows through event_of.\n- Keep local JSONL backend untouched.\n\nNON-SCOPE\n- Backend selection config plumbing.\n- Import/export conformance matrix beyond targeted operation tests.",
+    "acceptance": "1. TestAxonBackend_CreateAndGet passes against the GraphQL-backed implementation.\n2. TestAxonBackend_UpdateMutatesAndPersists passes against the GraphQL-backed implementation.\n3. TestAxonBackend_ClaimAndUnclaim passes against the GraphQL-backed implementation.\n4. TestAxonBackend_DepAddRemoveAndTree passes against the GraphQL-backed implementation.\n5. TestAxonBackend_AppendEventSplitsIntoEventsCollection passes and verifies event_of links.\n6. backend_axon.go / axon_backend.go no longer use local JSONL snapshots as the primary storage path when AxonBackend is selected with a GraphQL client.\n7. cd cli \u0026\u0026 go test ./internal/bead/... -run \"TestAxonBackend_CreateAndGet|TestAxonBackend_UpdateMutatesAndPersists|TestAxonBackend_ClaimAndUnclaim|TestAxonBackend_DepAddRemoveAndTree|TestAxonBackend_AppendEventSplitsIntoEventsCollection\" -count=1 passes.\n8. lefthook run pre-commit passes.",
+    "parent": "ddx-9c5bca8f",
+    "labels": [
+      "phase:2",
+      "area:beads",
+      "area:storage",
+      "kind:feature",
+      "axon",
+      "from:ddx-9c5bca8f",
+      "triage:no-changes-unjustified"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T06:55:01Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T03:03:49.423302353Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T025432-85c4069b\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":8248483,\"output_tokens\":38332,\"total_tokens\":8286815,\"cost_usd\":0,\"duration_ms\":554708,\"exit_code\":0}",
+          "created_at": "2026-05-06T03:03:49.634939121Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=8286815 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T03:03:57.115880293Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=fcba84a88be9a31884a472dc319fd0c91d19aa50\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T23:09:01-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=9777\noutput_bytes=0\nelapsed_ms=4204",
+          "created_at": "2026-05-06T03:04:01.827192738Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=fcba84a88be9a31884a472dc319fd0c91d19aa50\nbase_rev=dd5e474ac196f67da4e7a7594655fc8f20e36061",
+          "created_at": "2026-05-06T03:04:02.045972001Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T05:30:12.920686332Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T052908-8ffc0c51\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":611746,\"output_tokens\":5952,\"total_tokens\":617698,\"cost_usd\":0,\"duration_ms\":61664,\"exit_code\":0}",
+          "created_at": "2026-05-06T05:30:13.149527914Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=617698 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "(rationale absent)",
+          "created_at": "2026-05-06T05:30:13.834071329Z",
+          "kind": "no_changes_unjustified",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes_unjustified"
+        },
+        {
+          "actor": "erik",
+          "body": "agent exited without a commit or no_changes_rationale.txt\nresult_rev=854e788d2180506cbb995eaaa70ac989f4c49d14\nbase_rev=854e788d2180506cbb995eaaa70ac989f4c49d14",
+          "created_at": "2026-05-06T05:30:14.255228479Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-06T06:55:01.211780592Z",
+      "execute-loop-no-changes-count": 1,
+      "spec_id": "TD-030"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T065501-c01d5cf1",
+    "prompt": ".ddx/executions/20260506T065501-c01d5cf1/prompt.md",
+    "manifest": ".ddx/executions/20260506T065501-c01d5cf1/manifest.json",
+    "result": ".ddx/executions/20260506T065501-c01d5cf1/result.json",
+    "checks": ".ddx/executions/20260506T065501-c01d5cf1/checks.json",
+    "usage": ".ddx/executions/20260506T065501-c01d5cf1/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-6d5c436e-20260506T065501-c01d5cf1"
+  },
+  "prompt_sha": "c1c376f021d5e0741608476eaf535e4233f6e4c9b299fefb263766e2a07aa766"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T065501-c01d5cf1/result.json b/.ddx/executions/20260506T065501-c01d5cf1/result.json
new file mode 100644
index 00000000..1188bfd7
--- /dev/null
+++ b/.ddx/executions/20260506T065501-c01d5cf1/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-6d5c436e",
+  "attempt_id": "20260506T065501-c01d5cf1",
+  "base_rev": "78abc98ac47cad77a98b1969cc478763a8692192",
+  "result_rev": "2d3482b6ab1ca2d5fe72a4fe337bec72540204f1",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-391dcdb8",
+  "duration_ms": 68409,
+  "tokens": 557029,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T065501-c01d5cf1",
+  "prompt_file": ".ddx/executions/20260506T065501-c01d5cf1/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T065501-c01d5cf1/manifest.json",
+  "result_file": ".ddx/executions/20260506T065501-c01d5cf1/result.json",
+  "usage_file": ".ddx/executions/20260506T065501-c01d5cf1/usage.json",
+  "started_at": "2026-05-06T06:55:04.060218807Z",
+  "finished_at": "2026-05-06T06:56:12.469834895Z"
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
