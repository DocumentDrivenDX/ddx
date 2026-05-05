<bead-review>
  <bead id=".execute-bead-wt-.execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398-20260505T024017-db84a9cd-f2dae5c7" iter=1>
    <title>bead(axon): add changeEvents websocket subscription transport</title>
    <description>
PROBLEM
TD-030 requires a GraphQL subscription client for Axon changeEvents, but cli/internal/bead/axon_backend.go:20-26 still only documents a local JSONL emulation. There is no transport helper in cli/internal/bead/axon/ to open a live subscription stream against Axons WebSocket endpoint.

ROOT CAUSE
cli/internal/bead/axon_backend.go:20-26 keeps the backend in-process, so the package currently has no websocket transport boundary where GraphQL subscription setup, event decoding, and stream lifecycle management can live.

PROPOSED FIX
- Add a small subscription transport in cli/internal/bead/axon/ that opens a GraphQL WebSocket connection and subscribes to changeEvents.
- Define a narrow stream helper and event decoding path in the same package so the transport stays isolated from backend_axon.go.
- Add a focused subscription test that uses the helper to receive ordered changeEvents from a test endpoint.

NON-SCOPE
- Schema snapshot and genqlient binding generation.
- Replacing backend_axon.go with a production wire-path implementation.
- Axon server-side subscription schema work.
    </description>
    <acceptance>
1. cli/internal/bead/axon/ contains a websocket subscription helper for changeEvents.
2. TestAxonSubscription_ChangeEventsStream is added in cli/internal/bead/axon/ and receives ordered events from the helper.
3. The transport is wired through the package helper used by the test, not left as dead code.
4. cd cli &amp;&amp; go test ./internal/bead/axon/... passes.
5. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2,  area:beads,  area:storage,  kind:feature,  blocked-on-upstream:axon-05c1019d,  blocked-on-upstream:axon-82b6f7b2,  spec:TD-030</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T031641-18b3d274/manifest.json</file>
    <file>.ddx/executions/20260505T031641-18b3d274/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="e82b2d653ed7d3b3f02413f9709ed886ef600d91">
diff --git a/.ddx/executions/20260505T031641-18b3d274/manifest.json b/.ddx/executions/20260505T031641-18b3d274/manifest.json
new file mode 100644
index 00000000..2966afdc
--- /dev/null
+++ b/.ddx/executions/20260505T031641-18b3d274/manifest.json
@@ -0,0 +1,41 @@
+{
+  "attempt_id": "20260505T031641-18b3d274",
+  "bead_id": ".execute-bead-wt-.execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398-20260505T024017-db84a9cd-f2dae5c7",
+  "base_rev": "fca6bc829d0a8ec4aea96ae3d6b4cc96b6d89442",
+  "created_at": "2026-05-05T03:16:43.360676267Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-.execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398-20260505T024017-db84a9cd-f2dae5c7",
+    "title": "bead(axon): add changeEvents websocket subscription transport",
+    "description": "PROBLEM\nTD-030 requires a GraphQL subscription client for Axon changeEvents, but cli/internal/bead/axon_backend.go:20-26 still only documents a local JSONL emulation. There is no transport helper in cli/internal/bead/axon/ to open a live subscription stream against Axons WebSocket endpoint.\n\nROOT CAUSE\ncli/internal/bead/axon_backend.go:20-26 keeps the backend in-process, so the package currently has no websocket transport boundary where GraphQL subscription setup, event decoding, and stream lifecycle management can live.\n\nPROPOSED FIX\n- Add a small subscription transport in cli/internal/bead/axon/ that opens a GraphQL WebSocket connection and subscribes to changeEvents.\n- Define a narrow stream helper and event decoding path in the same package so the transport stays isolated from backend_axon.go.\n- Add a focused subscription test that uses the helper to receive ordered changeEvents from a test endpoint.\n\nNON-SCOPE\n- Schema snapshot and genqlient binding generation.\n- Replacing backend_axon.go with a production wire-path implementation.\n- Axon server-side subscription schema work.\n",
+    "acceptance": "1. cli/internal/bead/axon/ contains a websocket subscription helper for changeEvents.\n2. TestAxonSubscription_ChangeEventsStream is added in cli/internal/bead/axon/ and receives ordered events from the helper.\n3. The transport is wired through the package helper used by the test, not left as dead code.\n4. cd cli \u0026\u0026 go test ./internal/bead/axon/... passes.\n5. lefthook run pre-commit passes.",
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
+      "claimed-at": "2026-05-05T03:16:41Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
+      "execute-loop-heartbeat-at": "2026-05-05T03:16:41.106012188Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T031641-18b3d274",
+    "prompt": ".ddx/executions/20260505T031641-18b3d274/prompt.md",
+    "manifest": ".ddx/executions/20260505T031641-18b3d274/manifest.json",
+    "result": ".ddx/executions/20260505T031641-18b3d274/result.json",
+    "checks": ".ddx/executions/20260505T031641-18b3d274/checks.json",
+    "usage": ".ddx/executions/20260505T031641-18b3d274/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-.execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398-20260505T024017-db84a9cd-f2dae5c7-20260505T031641-18b3d274"
+  },
+  "prompt_sha": "91603417593c94099964057303b30db9b1a53cbbce121f296d2a3a699334f83c"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T031641-18b3d274/result.json b/.ddx/executions/20260505T031641-18b3d274/result.json
new file mode 100644
index 00000000..1aaee702
--- /dev/null
+++ b/.ddx/executions/20260505T031641-18b3d274/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": ".execute-bead-wt-.execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398-20260505T024017-db84a9cd-f2dae5c7",
+  "attempt_id": "20260505T031641-18b3d274",
+  "base_rev": "fca6bc829d0a8ec4aea96ae3d6b4cc96b6d89442",
+  "result_rev": "a32129dc36e3553a08bcfd9ea7d7beaefc0ca233",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-d81b4ee9",
+  "duration_ms": 52561,
+  "tokens": 414309,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T031641-18b3d274",
+  "prompt_file": ".ddx/executions/20260505T031641-18b3d274/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T031641-18b3d274/manifest.json",
+  "result_file": ".ddx/executions/20260505T031641-18b3d274/result.json",
+  "usage_file": ".ddx/executions/20260505T031641-18b3d274/usage.json",
+  "started_at": "2026-05-05T03:16:43.361006767Z",
+  "finished_at": "2026-05-05T03:17:35.922236175Z"
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
