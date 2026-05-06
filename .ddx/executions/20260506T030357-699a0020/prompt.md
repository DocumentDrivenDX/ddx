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
    <labels>phase:2, area:beads, area:storage, kind:feature, axon, from:ddx-9c5bca8f</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T025432-85c4069b/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T025432-85c4069b/manifest.json</file>
    <file>.ddx/executions/20260506T025432-85c4069b/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="fcba84a88be9a31884a472dc319fd0c91d19aa50">
<untrusted-data>
diff --git a/.ddx/executions/20260506T025432-85c4069b/checks/production-reachability.json b/.ddx/executions/20260506T025432-85c4069b/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260506T025432-85c4069b/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T025432-85c4069b/manifest.json b/.ddx/executions/20260506T025432-85c4069b/manifest.json
new file mode 100644
index 00000000..a4aa98a5
--- /dev/null
+++ b/.ddx/executions/20260506T025432-85c4069b/manifest.json
@@ -0,0 +1,41 @@
+{
+  "attempt_id": "20260506T025432-85c4069b",
+  "bead_id": "ddx-6d5c436e",
+  "base_rev": "dd5e474ac196f67da4e7a7594655fc8f20e36061",
+  "created_at": "2026-05-06T02:54:34.71210321Z",
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
+      "from:ddx-9c5bca8f"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T02:54:31Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "execute-loop-heartbeat-at": "2026-05-06T02:54:31.931462666Z",
+      "spec_id": "TD-030"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T025432-85c4069b",
+    "prompt": ".ddx/executions/20260506T025432-85c4069b/prompt.md",
+    "manifest": ".ddx/executions/20260506T025432-85c4069b/manifest.json",
+    "result": ".ddx/executions/20260506T025432-85c4069b/result.json",
+    "checks": ".ddx/executions/20260506T025432-85c4069b/checks.json",
+    "usage": ".ddx/executions/20260506T025432-85c4069b/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-6d5c436e-20260506T025432-85c4069b"
+  },
+  "prompt_sha": "0881a446874e4976d8d9f123cff61f9e8c20316ab4952355965f6aba4ce08f59"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T025432-85c4069b/result.json b/.ddx/executions/20260506T025432-85c4069b/result.json
new file mode 100644
index 00000000..f5094121
--- /dev/null
+++ b/.ddx/executions/20260506T025432-85c4069b/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-6d5c436e",
+  "attempt_id": "20260506T025432-85c4069b",
+  "base_rev": "dd5e474ac196f67da4e7a7594655fc8f20e36061",
+  "result_rev": "c3238a790823b87dddce22adc92d987e3a3c5adf",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-478f7154",
+  "duration_ms": 554708,
+  "tokens": 8286815,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T025432-85c4069b",
+  "prompt_file": ".ddx/executions/20260506T025432-85c4069b/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T025432-85c4069b/manifest.json",
+  "result_file": ".ddx/executions/20260506T025432-85c4069b/result.json",
+  "usage_file": ".ddx/executions/20260506T025432-85c4069b/usage.json",
+  "started_at": "2026-05-06T02:54:34.712454585Z",
+  "finished_at": "2026-05-06T03:03:49.421101605Z"
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
