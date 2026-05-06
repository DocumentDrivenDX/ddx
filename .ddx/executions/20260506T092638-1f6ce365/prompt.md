<bead-review>
  <bead id="ddx-29f02cf4" iter=1>
    <title>bead/axon: wire backend selection through .ddx config</title>
    <description>
PROBLEM
The store factory still treats Axon as an experimental special case and only enables it behind an environment gate. The relevant branch is cli/internal/bead/store.go:117-137, which falls back to JSONL unless DDX_AXON_EXPERIMENTAL is truthy, even when config has backend: axon. The checked-in sample config also does not document the bead_tracker backend block yet (.ddx/config.yaml:1-17).

ROOT CAUSE
cli/internal/bead/store.go:117-137 and .ddx/config.yaml:1-17 do not present Axon as a normal selectable backend. The config type already has a Backend field, but the factory still requires an env override and emits a warning instead of honoring configuration directly.

PROPOSED FIX
- Update the store factory so bead_tracker.backend=axon selects the Axon backend from config without requiring the experimental env var.
- Update the checked-in .ddx/config.yaml example to show the backend and Axon connection block.
- Keep the default backend unchanged when backend is unset.
- Preserve bd/br behavior for the existing external backend path.

NON-SCOPE
- GraphQL client generation.
- Axon backend CRUD internals.
- Subscription transport.
    </description>
    <acceptance>
1. .ddx/config.yaml documents bead_tracker.backend: axon plus the Axon connection block.
2. TestNewStore_DefaultsToJSONL, TestNewStore_SelectsAxonFromConfig, and TestNewStore_PreservesExternalBackends are added or updated.
3. config selection honors axon without requiring DDX_AXON_EXPERIMENTAL, while jsonl remains the default.
4. cd cli &amp;&amp; go test ./internal/bead/... passes.
5. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:beads, area:storage, kind:feature, blocked-on-upstream:axon-05c1019d, blocked-on-upstream:axon-82b6f7b2</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T092212-36d077c0/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T092212-36d077c0/manifest.json</file>
    <file>.ddx/executions/20260506T092212-36d077c0/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="30cc6a51f01547fac0a120e58bed2d5f4eccaeaa">
<untrusted-data>
diff --git a/.ddx/executions/20260506T092212-36d077c0/checks/production-reachability.json b/.ddx/executions/20260506T092212-36d077c0/checks/production-reachability.json
new file mode 100644
index 000000000..246408be7
--- /dev/null
+++ b/.ddx/executions/20260506T092212-36d077c0/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T092212-36d077c0/manifest.json b/.ddx/executions/20260506T092212-36d077c0/manifest.json
new file mode 100644
index 000000000..0e3d8738e
--- /dev/null
+++ b/.ddx/executions/20260506T092212-36d077c0/manifest.json
@@ -0,0 +1,40 @@
+{
+  "attempt_id": "20260506T092212-36d077c0",
+  "bead_id": "ddx-29f02cf4",
+  "base_rev": "521e1c806a51bb800d0baeba067aa49417f912c7",
+  "created_at": "2026-05-06T09:22:15.008991787Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-29f02cf4",
+    "title": "bead/axon: wire backend selection through .ddx config",
+    "description": "PROBLEM\nThe store factory still treats Axon as an experimental special case and only enables it behind an environment gate. The relevant branch is cli/internal/bead/store.go:117-137, which falls back to JSONL unless DDX_AXON_EXPERIMENTAL is truthy, even when config has backend: axon. The checked-in sample config also does not document the bead_tracker backend block yet (.ddx/config.yaml:1-17).\n\nROOT CAUSE\ncli/internal/bead/store.go:117-137 and .ddx/config.yaml:1-17 do not present Axon as a normal selectable backend. The config type already has a Backend field, but the factory still requires an env override and emits a warning instead of honoring configuration directly.\n\nPROPOSED FIX\n- Update the store factory so bead_tracker.backend=axon selects the Axon backend from config without requiring the experimental env var.\n- Update the checked-in .ddx/config.yaml example to show the backend and Axon connection block.\n- Keep the default backend unchanged when backend is unset.\n- Preserve bd/br behavior for the existing external backend path.\n\nNON-SCOPE\n- GraphQL client generation.\n- Axon backend CRUD internals.\n- Subscription transport.",
+    "acceptance": "1. .ddx/config.yaml documents bead_tracker.backend: axon plus the Axon connection block.\n2. TestNewStore_DefaultsToJSONL, TestNewStore_SelectsAxonFromConfig, and TestNewStore_PreservesExternalBackends are added or updated.\n3. config selection honors axon without requiring DDX_AXON_EXPERIMENTAL, while jsonl remains the default.\n4. cd cli \u0026\u0026 go test ./internal/bead/... passes.\n5. lefthook run pre-commit passes.",
+    "parent": "ddx-8d747049",
+    "labels": [
+      "phase:2",
+      "area:beads",
+      "area:storage",
+      "kind:feature",
+      "blocked-on-upstream:axon-05c1019d",
+      "blocked-on-upstream:axon-82b6f7b2"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T09:22:12Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "execute-loop-heartbeat-at": "2026-05-06T09:22:12.206789376Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T092212-36d077c0",
+    "prompt": ".ddx/executions/20260506T092212-36d077c0/prompt.md",
+    "manifest": ".ddx/executions/20260506T092212-36d077c0/manifest.json",
+    "result": ".ddx/executions/20260506T092212-36d077c0/result.json",
+    "checks": ".ddx/executions/20260506T092212-36d077c0/checks.json",
+    "usage": ".ddx/executions/20260506T092212-36d077c0/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-29f02cf4-20260506T092212-36d077c0"
+  },
+  "prompt_sha": "cb6ca51c5579fdd770a7842edc0dbe6c57e4136c077ee0f3f60ca581e6b0b0f9"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T092212-36d077c0/result.json b/.ddx/executions/20260506T092212-36d077c0/result.json
new file mode 100644
index 000000000..7648914f1
--- /dev/null
+++ b/.ddx/executions/20260506T092212-36d077c0/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-29f02cf4",
+  "attempt_id": "20260506T092212-36d077c0",
+  "base_rev": "521e1c806a51bb800d0baeba067aa49417f912c7",
+  "result_rev": "250cfdc09ec86403ac050a2d2c6e96dbf22d42ed",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-487526f6",
+  "duration_ms": 256113,
+  "tokens": 3426181,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T092212-36d077c0",
+  "prompt_file": ".ddx/executions/20260506T092212-36d077c0/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T092212-36d077c0/manifest.json",
+  "result_file": ".ddx/executions/20260506T092212-36d077c0/result.json",
+  "usage_file": ".ddx/executions/20260506T092212-36d077c0/usage.json",
+  "started_at": "2026-05-06T09:22:15.009491203Z",
+  "finished_at": "2026-05-06T09:26:31.122851406Z"
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
