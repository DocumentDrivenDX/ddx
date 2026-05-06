<bead-review>
  <bead id="ddx-d6730314" iter=1>
    <title>agent: wrap artifact bodies with untrusted-data delimiters</title>
    <description>
PROBLEM
Prompt-injection boundary markers are not yet explicit around artifact bodies that later flow into prompt assembly. The current server-side artifact readers in `cli/internal/server/state_executions.go:205-217` clamp text for transport, but they do not wrap the body in an explicit untrusted-data envelope for downstream prompt renderers. The review prompt path in `cli/internal/agent/execute_bead.go:1561-1630` only uses XML tags for the bead envelope; it does not mark artifact content with a dedicated boundary contract.

ROOT CAUSE
There is no shared delimiter helper for untrusted artifact bodies. The current prompt builders rely on ad hoc XML wrappers and transport clamping, which are not the same thing as a prompt-injection boundary that clearly distinguishes trusted instructions from untrusted artifact text.

PROPOSED FIX
- Add a small helper (likely in `cli/internal/evidence` or the prompt-rendering package that owns the assembler) that wraps artifact bodies in a canonical untrusted-data delimiter pair.
- Update the prompt-rendering call sites that inline artifact bodies to use the wrapper before interpolation.
- Add a regression test named `TestArtifactBodyDelimitedAsUntrusted` that asserts the delimiter pair appears around the artifact payload and that the raw body is still preserved inside the wrapper.

NON-SCOPE
- Do not alter the transport-level clamp behavior or its `truncated` metadata.
- Do not change review verdict parsing or the FEAT-022 telemetry schema in this slice.
- Do not rewrite unrelated server text egress paths.
    </description>
    <acceptance>
1. A reusable delimiter helper exists for untrusted artifact bodies.
2. Prompt assembly uses the helper at every artifact-body interpolation site in scope.
3. `TestArtifactBodyDelimitedAsUntrusted` proves the delimiter contract and payload preservation.
4. `cd cli &amp;&amp; go test ./internal/server/... ./internal/agent/...` passes.
5. `lefthook run pre-commit` passes.
    </acceptance>
    <labels>phase:2, story:18, area:server, area:agent, kind:feature, security, from:ddx-0e5c3005</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T042824-b7bee959/manifest.json</file>
    <file>.ddx/executions/20260506T042824-b7bee959/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="d7ddc21d832f8fd13ee82714e7be0b0b405a9842">
<untrusted-data>
diff --git a/.ddx/executions/20260506T042824-b7bee959/manifest.json b/.ddx/executions/20260506T042824-b7bee959/manifest.json
new file mode 100644
index 00000000..e1192a81
--- /dev/null
+++ b/.ddx/executions/20260506T042824-b7bee959/manifest.json
@@ -0,0 +1,42 @@
+{
+  "attempt_id": "20260506T042824-b7bee959",
+  "bead_id": "ddx-d6730314",
+  "base_rev": "72e242521b65997d89d9b0eb9bc854953c681989",
+  "created_at": "2026-05-06T04:28:26.552573091Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-d6730314",
+    "title": "agent: wrap artifact bodies with untrusted-data delimiters",
+    "description": "PROBLEM\nPrompt-injection boundary markers are not yet explicit around artifact bodies that later flow into prompt assembly. The current server-side artifact readers in `cli/internal/server/state_executions.go:205-217` clamp text for transport, but they do not wrap the body in an explicit untrusted-data envelope for downstream prompt renderers. The review prompt path in `cli/internal/agent/execute_bead.go:1561-1630` only uses XML tags for the bead envelope; it does not mark artifact content with a dedicated boundary contract.\n\nROOT CAUSE\nThere is no shared delimiter helper for untrusted artifact bodies. The current prompt builders rely on ad hoc XML wrappers and transport clamping, which are not the same thing as a prompt-injection boundary that clearly distinguishes trusted instructions from untrusted artifact text.\n\nPROPOSED FIX\n- Add a small helper (likely in `cli/internal/evidence` or the prompt-rendering package that owns the assembler) that wraps artifact bodies in a canonical untrusted-data delimiter pair.\n- Update the prompt-rendering call sites that inline artifact bodies to use the wrapper before interpolation.\n- Add a regression test named `TestArtifactBodyDelimitedAsUntrusted` that asserts the delimiter pair appears around the artifact payload and that the raw body is still preserved inside the wrapper.\n\nNON-SCOPE\n- Do not alter the transport-level clamp behavior or its `truncated` metadata.\n- Do not change review verdict parsing or the FEAT-022 telemetry schema in this slice.\n- Do not rewrite unrelated server text egress paths.",
+    "acceptance": "1. A reusable delimiter helper exists for untrusted artifact bodies.\n2. Prompt assembly uses the helper at every artifact-body interpolation site in scope.\n3. `TestArtifactBodyDelimitedAsUntrusted` proves the delimiter contract and payload preservation.\n4. `cd cli \u0026\u0026 go test ./internal/server/... ./internal/agent/...` passes.\n5. `lefthook run pre-commit` passes.",
+    "parent": "ddx-42b917fe",
+    "labels": [
+      "phase:2",
+      "story:18",
+      "area:server",
+      "area:agent",
+      "kind:feature",
+      "security",
+      "from:ddx-0e5c3005"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T04:28:23Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "execute-loop-heartbeat-at": "2026-05-06T04:28:23.971201942Z",
+      "replaces": ".execute-bead-wt-ddx-0e5c3005-20260505T113434-3317d134-7fb75345"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T042824-b7bee959",
+    "prompt": ".ddx/executions/20260506T042824-b7bee959/prompt.md",
+    "manifest": ".ddx/executions/20260506T042824-b7bee959/manifest.json",
+    "result": ".ddx/executions/20260506T042824-b7bee959/result.json",
+    "checks": ".ddx/executions/20260506T042824-b7bee959/checks.json",
+    "usage": ".ddx/executions/20260506T042824-b7bee959/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-d6730314-20260506T042824-b7bee959"
+  },
+  "prompt_sha": "1fb6f9e4b533b7b513a77501e4217d76d0d641b737f07e1cc16ce4107142fb58"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T042824-b7bee959/result.json b/.ddx/executions/20260506T042824-b7bee959/result.json
new file mode 100644
index 00000000..5d7034e7
--- /dev/null
+++ b/.ddx/executions/20260506T042824-b7bee959/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-d6730314",
+  "attempt_id": "20260506T042824-b7bee959",
+  "base_rev": "72e242521b65997d89d9b0eb9bc854953c681989",
+  "result_rev": "a4c9d8e375a7d3731e06fa70cf60b4b55bf37c38",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-67934a98",
+  "duration_ms": 297896,
+  "tokens": 6075175,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T042824-b7bee959",
+  "prompt_file": ".ddx/executions/20260506T042824-b7bee959/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T042824-b7bee959/manifest.json",
+  "result_file": ".ddx/executions/20260506T042824-b7bee959/result.json",
+  "usage_file": ".ddx/executions/20260506T042824-b7bee959/usage.json",
+  "started_at": "2026-05-06T04:28:26.552920008Z",
+  "finished_at": "2026-05-06T04:33:24.449413015Z"
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
