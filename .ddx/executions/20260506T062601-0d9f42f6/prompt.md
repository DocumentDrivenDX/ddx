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
    <file>.ddx/executions/20260506T062353-771ea315/manifest.json</file>
    <file>.ddx/executions/20260506T062353-771ea315/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="2f96326e781529325cf0546d31a0a8845e1fae6d">
<untrusted-data>
diff --git a/.ddx/executions/20260506T062353-771ea315/manifest.json b/.ddx/executions/20260506T062353-771ea315/manifest.json
new file mode 100644
index 00000000..57147820
--- /dev/null
+++ b/.ddx/executions/20260506T062353-771ea315/manifest.json
@@ -0,0 +1,84 @@
+{
+  "attempt_id": "20260506T062353-771ea315",
+  "bead_id": "ddx-d6730314",
+  "base_rev": "49c2d093664d2dde0a4668e421109ad957537469",
+  "created_at": "2026-05-06T06:23:55.697677931Z",
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
+      "claimed-at": "2026-05-06T06:23:53Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T04:33:24.451530973Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T042824-b7bee959\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":6060430,\"output_tokens\":14745,\"total_tokens\":6075175,\"cost_usd\":0,\"duration_ms\":297896,\"exit_code\":0}",
+          "created_at": "2026-05-06T04:33:24.65114092Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=6075175 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T04:33:31.798270402Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=d7ddc21d832f8fd13ee82714e7be0b0b405a9842\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-06T00:38:36-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=9115\noutput_bytes=0\nelapsed_ms=4188",
+          "created_at": "2026-05-06T04:33:36.520389652Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=d7ddc21d832f8fd13ee82714e7be0b0b405a9842\nbase_rev=72e242521b65997d89d9b0eb9bc854953c681989",
+          "created_at": "2026-05-06T04:33:36.716595058Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-06T06:23:53.141503312Z",
+      "replaces": ".execute-bead-wt-ddx-0e5c3005-20260505T113434-3317d134-7fb75345"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T062353-771ea315",
+    "prompt": ".ddx/executions/20260506T062353-771ea315/prompt.md",
+    "manifest": ".ddx/executions/20260506T062353-771ea315/manifest.json",
+    "result": ".ddx/executions/20260506T062353-771ea315/result.json",
+    "checks": ".ddx/executions/20260506T062353-771ea315/checks.json",
+    "usage": ".ddx/executions/20260506T062353-771ea315/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-d6730314-20260506T062353-771ea315"
+  },
+  "prompt_sha": "12441f9386134e0158e53d4dc2ed12a4d7abe5925514412de62d1b41f91cffc1"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T062353-771ea315/result.json b/.ddx/executions/20260506T062353-771ea315/result.json
new file mode 100644
index 00000000..22f30758
--- /dev/null
+++ b/.ddx/executions/20260506T062353-771ea315/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-d6730314",
+  "attempt_id": "20260506T062353-771ea315",
+  "base_rev": "49c2d093664d2dde0a4668e421109ad957537469",
+  "result_rev": "c00b65b37a06bde081f71e2600bf18b495fb7d05",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-c20cd2fc",
+  "duration_ms": 119406,
+  "tokens": 1104129,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T062353-771ea315",
+  "prompt_file": ".ddx/executions/20260506T062353-771ea315/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T062353-771ea315/manifest.json",
+  "result_file": ".ddx/executions/20260506T062353-771ea315/result.json",
+  "usage_file": ".ddx/executions/20260506T062353-771ea315/usage.json",
+  "started_at": "2026-05-06T06:23:55.698016014Z",
+  "finished_at": "2026-05-06T06:25:55.104536283Z"
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
