<bead-review>
  <bead id="ddx-cbd09cde" iter=1>
    <title>bead/axon: add Axon backend conformance and import/export coverage</title>
    <description>
PROBLEM
Once GraphQL-backed operations exist, the backend needs parity coverage against JSONL so worker execution can trust AxonBackend without silently diverging on import/export, events, dependencies, and store behavior.

ROOT CAUSE WITH FILE:LINE
- cli/internal/bead/axon_backend_test.go contains Axon-specific tests, but the parent acceptance requires JSONL import/export round trip and package-level conformance.
- cli/internal/bead/benchmark_backends_test.go and related store tests exercise backend behavior primarily through JSONL paths.
- Without a parameterized conformance pass, AxonBackend can pass targeted CRUD tests but still diverge from RawBackend expectations.

PROPOSED FIX
- Parameterize the existing backend/store conformance suite so JSONL and Axon exercise the same core operations where practical.
- Add Axon import/export round-trip coverage.
- Document any intentional Axon-only limitation in test names and bead notes, not hidden in implementation comments.

NON-SCOPE
- Adding new backend selection config plumbing.
- UI or server integration.
    </description>
    <acceptance>
1. TestAxonBackend_JSONLImportExportRoundTrip passes against the GraphQL-backed implementation.
2. Backend conformance tests run against both JSONL and Axon for create/get/update/claim/deps/events/close behavior.
3. Any Axon-only limitation has an explicit skipped test or assertion message with a follow-up bead ID; no silent gaps remain.
4. cd cli &amp;&amp; go test ./internal/bead/... passes.
5. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:beads, area:storage, kind:test, axon, from:ddx-9c5bca8f</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T080714-a2d53676/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T080714-a2d53676/manifest.json</file>
    <file>.ddx/executions/20260506T080714-a2d53676/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="79f10c8ca6524ef81ca5ae94bd5c989a4503a031">
<untrusted-data>
diff --git a/.ddx/executions/20260506T080714-a2d53676/checks/production-reachability.json b/.ddx/executions/20260506T080714-a2d53676/checks/production-reachability.json
new file mode 100644
index 00000000..89e73251
--- /dev/null
+++ b/.ddx/executions/20260506T080714-a2d53676/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no non-test Go files changed"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T080714-a2d53676/manifest.json b/.ddx/executions/20260506T080714-a2d53676/manifest.json
new file mode 100644
index 00000000..1d20b93a
--- /dev/null
+++ b/.ddx/executions/20260506T080714-a2d53676/manifest.json
@@ -0,0 +1,41 @@
+{
+  "attempt_id": "20260506T080714-a2d53676",
+  "bead_id": "ddx-cbd09cde",
+  "base_rev": "4a059810e78589d022b7d295c0059eb01a73facd",
+  "created_at": "2026-05-06T08:07:17.12221943Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-cbd09cde",
+    "title": "bead/axon: add Axon backend conformance and import/export coverage",
+    "description": "PROBLEM\nOnce GraphQL-backed operations exist, the backend needs parity coverage against JSONL so worker execution can trust AxonBackend without silently diverging on import/export, events, dependencies, and store behavior.\n\nROOT CAUSE WITH FILE:LINE\n- cli/internal/bead/axon_backend_test.go contains Axon-specific tests, but the parent acceptance requires JSONL import/export round trip and package-level conformance.\n- cli/internal/bead/benchmark_backends_test.go and related store tests exercise backend behavior primarily through JSONL paths.\n- Without a parameterized conformance pass, AxonBackend can pass targeted CRUD tests but still diverge from RawBackend expectations.\n\nPROPOSED FIX\n- Parameterize the existing backend/store conformance suite so JSONL and Axon exercise the same core operations where practical.\n- Add Axon import/export round-trip coverage.\n- Document any intentional Axon-only limitation in test names and bead notes, not hidden in implementation comments.\n\nNON-SCOPE\n- Adding new backend selection config plumbing.\n- UI or server integration.",
+    "acceptance": "1. TestAxonBackend_JSONLImportExportRoundTrip passes against the GraphQL-backed implementation.\n2. Backend conformance tests run against both JSONL and Axon for create/get/update/claim/deps/events/close behavior.\n3. Any Axon-only limitation has an explicit skipped test or assertion message with a follow-up bead ID; no silent gaps remain.\n4. cd cli \u0026\u0026 go test ./internal/bead/... passes.\n5. lefthook run pre-commit passes.",
+    "parent": "ddx-9c5bca8f",
+    "labels": [
+      "phase:2",
+      "area:beads",
+      "area:storage",
+      "kind:test",
+      "axon",
+      "from:ddx-9c5bca8f"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T08:07:14Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "execute-loop-heartbeat-at": "2026-05-06T08:07:14.323500075Z",
+      "spec_id": "TD-030"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T080714-a2d53676",
+    "prompt": ".ddx/executions/20260506T080714-a2d53676/prompt.md",
+    "manifest": ".ddx/executions/20260506T080714-a2d53676/manifest.json",
+    "result": ".ddx/executions/20260506T080714-a2d53676/result.json",
+    "checks": ".ddx/executions/20260506T080714-a2d53676/checks.json",
+    "usage": ".ddx/executions/20260506T080714-a2d53676/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-cbd09cde-20260506T080714-a2d53676"
+  },
+  "prompt_sha": "f2a1e32d59bb4ee3658d3173182f75c51c5b6d2ca732abd5b1752316860cba74"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T080714-a2d53676/result.json b/.ddx/executions/20260506T080714-a2d53676/result.json
new file mode 100644
index 00000000..555b4c1e
--- /dev/null
+++ b/.ddx/executions/20260506T080714-a2d53676/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-cbd09cde",
+  "attempt_id": "20260506T080714-a2d53676",
+  "base_rev": "4a059810e78589d022b7d295c0059eb01a73facd",
+  "result_rev": "46f2e1ba8043e3ed6342238739fb034a58c255e6",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-5a62cdc4",
+  "duration_ms": 289774,
+  "tokens": 3236503,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T080714-a2d53676",
+  "prompt_file": ".ddx/executions/20260506T080714-a2d53676/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T080714-a2d53676/manifest.json",
+  "result_file": ".ddx/executions/20260506T080714-a2d53676/result.json",
+  "usage_file": ".ddx/executions/20260506T080714-a2d53676/usage.json",
+  "started_at": "2026-05-06T08:07:17.122505013Z",
+  "finished_at": "2026-05-06T08:12:06.897264902Z"
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
