<bead-review>
  <bead id="ddx-06b77652" iter=1>
    <title>ADR-style: 7 reliability principles for ddx try / ddx work / sub-agent execution</title>
    <description>
The ddx try / ddx work execution machinery has 9 fail-prone layers (routing pre-flight, cooldown classification, claim CAS, worktree spawn, subprocess lifecycle, outcome adjudication, retry policy, land coordination, review). Today most layers fail-CLOSED — when a layer's specific check rejects, the layer wedges the entire pipeline. The auto-routing rejection bug fixed at commit 3b4f5d58 demonstrated this: one preflight check went strict and the queue locked up for hours.

The 7 principles below capture what each layer should do instead. They are documentation, not code — referenced by every refactor child's AC and by future bead-authoring guidance.

PRINCIPLE STATEMENTS

P1: FAIL-OPEN at every machinery layer. When a layer's specific check rejects a candidate, the layer skips itself and emits a structured event — it does not wedge the pipeline. The auto-routing fix (workers.go:803, commit 3b4f5d58) is the canonical example: preflight is now advisory when no operator pin exists.

P2: SINGLE RESPONSIBILITY per layer. Each layer rejects only on conditions it owns. Routing pre-flight does NOT decide provider availability (fizeau owns); cooldown does NOT decide eligibility (picker owns); etc. Cross-layer concerns are the smell.

P3: OBSERVABLE DEGRADATION. Every fail-open emits a structured event surfaced in the workers panel. Operators see "preflight skipped (no operator pin)" instead of silent acceptance OR endless retry loop.

P4: BOUNDED BLAST RADIUS. A failure on bead X must not affect bead Y. Stay-alive fix at commit 41cb762e established this for preflight rejections (per-bead continue, not loop exit). Extend to all layers.

P5: OPERATOR-VISIBLE STATE. Worker reports current state (idle, claiming, executing, reviewing, blocked-on-X) at all times. No '8 hours running, 0 attempts' mystery state. ADR-022 rev 5 §Probe + freshness state model defines the worker side; UI workers panel surfaces it.

P6: AUTO-RETRY ONLY FOR TRANSIENT CLASSES. Cooldown fires ONLY when the model genuinely couldn't make progress (clean no-changes with rationale). Disrupted, preflight-rejected, network-error, claim-race → no cooldown, return to ready. Existing code: shouldSuppressNoProgress at execute_bead_loop.go:1545 already respects Disrupted (commit 47d8054e).

P7: BEAD = PROMPT. A bead's description + AC must be sufficient context for a competent sub-agent to execute it without hand-curation. Investigation done, file:line citations included, concrete test names specified, explicit non-scope marked. If a sub-agent succeeds where the bead's auto-prompt failed, the BEAD failed (not the executor). Bead-authoring template enforces this; bead-quality audit (forthcoming bead) retrofits existing beads.

NOT IN SCOPE
- Code changes (this bead is doc-only)
- Per-layer reliability bead-quality fixes (separate refactor children)

INTERSECTIONS
- Each refactor child (ddx-c8f79963 C5, ddx-06eb05d8 C7, ddx-9228a484 C9, ddx-848069a3 C8, ddx-b669bb9f C6, ddx-c670ef0a C12) gets an AC line citing the principle(s) it enforces
- Bead-quality audit applies P7 to ~20 existing open beads

EVIDENCE
- ddx-f339c399 documents the 2026-05-04 ddx try ddx-29058e2a --harness=codex no_changes outcome and supports P7 / bead-quality.
    </description>
    <acceptance>
1. docs/helix/06-iterate/reliability-principles.md exists and contains all 7 principles with their statements above (verbatim).
2. README sections of each refactor child (C4, C5, C6, C7, C8, C9, C11, C12, C13) cite the principle(s) the child enforces. Cross-link in those bead descriptions added via ddx bead update.
3. CLAUDE.md and AGENTS.md updated with one-line pointers: 'See docs/helix/06-iterate/reliability-principles.md for the 7 reliability principles applied to ddx try / ddx work execution.'
4. cd cli &amp;&amp; go test ./... still green (no code change; sanity check that nothing broke).
5. lefthook run pre-commit passes.
6. Conventional commit ending [&lt;this-bead-id&gt;].
    </acceptance>
    <notes>
Evidence cross-link: ddx-f339c399 documents the 2026-05-04 ddx try ddx-29058e2a --harness=codex no_changes outcome and serves as evidence for P7 / bead-quality.
    </notes>
    <labels>phase:2, area:agent, area:server, kind:design, reliability, bead-quality</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T074136-36b65cad/manifest.json</file>
    <file>.ddx/executions/20260505T074136-36b65cad/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="ea11d9f534e25c1f31ad2c680ab7065b95a08dcf">
diff --git a/.ddx/executions/20260505T074136-36b65cad/manifest.json b/.ddx/executions/20260505T074136-36b65cad/manifest.json
new file mode 100644
index 00000000..ec499ef2
--- /dev/null
+++ b/.ddx/executions/20260505T074136-36b65cad/manifest.json
@@ -0,0 +1,61 @@
+{
+  "attempt_id": "20260505T074136-36b65cad",
+  "bead_id": "ddx-06b77652",
+  "base_rev": "24359509fbd370840c63c52588d2e722756f0fd8",
+  "created_at": "2026-05-05T07:41:38.634098529Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-06b77652",
+    "title": "ADR-style: 7 reliability principles for ddx try / ddx work / sub-agent execution",
+    "description": "The ddx try / ddx work execution machinery has 9 fail-prone layers (routing pre-flight, cooldown classification, claim CAS, worktree spawn, subprocess lifecycle, outcome adjudication, retry policy, land coordination, review). Today most layers fail-CLOSED — when a layer's specific check rejects, the layer wedges the entire pipeline. The auto-routing rejection bug fixed at commit 3b4f5d58 demonstrated this: one preflight check went strict and the queue locked up for hours.\n\nThe 7 principles below capture what each layer should do instead. They are documentation, not code — referenced by every refactor child's AC and by future bead-authoring guidance.\n\nPRINCIPLE STATEMENTS\n\nP1: FAIL-OPEN at every machinery layer. When a layer's specific check rejects a candidate, the layer skips itself and emits a structured event — it does not wedge the pipeline. The auto-routing fix (workers.go:803, commit 3b4f5d58) is the canonical example: preflight is now advisory when no operator pin exists.\n\nP2: SINGLE RESPONSIBILITY per layer. Each layer rejects only on conditions it owns. Routing pre-flight does NOT decide provider availability (fizeau owns); cooldown does NOT decide eligibility (picker owns); etc. Cross-layer concerns are the smell.\n\nP3: OBSERVABLE DEGRADATION. Every fail-open emits a structured event surfaced in the workers panel. Operators see \"preflight skipped (no operator pin)\" instead of silent acceptance OR endless retry loop.\n\nP4: BOUNDED BLAST RADIUS. A failure on bead X must not affect bead Y. Stay-alive fix at commit 41cb762e established this for preflight rejections (per-bead continue, not loop exit). Extend to all layers.\n\nP5: OPERATOR-VISIBLE STATE. Worker reports current state (idle, claiming, executing, reviewing, blocked-on-X) at all times. No '8 hours running, 0 attempts' mystery state. ADR-022 rev 5 §Probe + freshness state model defines the worker side; UI workers panel surfaces it.\n\nP6: AUTO-RETRY ONLY FOR TRANSIENT CLASSES. Cooldown fires ONLY when the model genuinely couldn't make progress (clean no-changes with rationale). Disrupted, preflight-rejected, network-error, claim-race → no cooldown, return to ready. Existing code: shouldSuppressNoProgress at execute_bead_loop.go:1545 already respects Disrupted (commit 47d8054e).\n\nP7: BEAD = PROMPT. A bead's description + AC must be sufficient context for a competent sub-agent to execute it without hand-curation. Investigation done, file:line citations included, concrete test names specified, explicit non-scope marked. If a sub-agent succeeds where the bead's auto-prompt failed, the BEAD failed (not the executor). Bead-authoring template enforces this; bead-quality audit (forthcoming bead) retrofits existing beads.\n\nNOT IN SCOPE\n- Code changes (this bead is doc-only)\n- Per-layer reliability bead-quality fixes (separate refactor children)\n\nINTERSECTIONS\n- Each refactor child (ddx-c8f79963 C5, ddx-06eb05d8 C7, ddx-9228a484 C9, ddx-848069a3 C8, ddx-b669bb9f C6, ddx-c670ef0a C12) gets an AC line citing the principle(s) it enforces\n- Bead-quality audit applies P7 to ~20 existing open beads\n\nEVIDENCE\n- ddx-f339c399 documents the 2026-05-04 ddx try ddx-29058e2a --harness=codex no_changes outcome and supports P7 / bead-quality.",
+    "acceptance": "1. docs/helix/06-iterate/reliability-principles.md exists and contains all 7 principles with their statements above (verbatim).\n2. README sections of each refactor child (C4, C5, C6, C7, C8, C9, C11, C12, C13) cite the principle(s) the child enforces. Cross-link in those bead descriptions added via ddx bead update.\n3. CLAUDE.md and AGENTS.md updated with one-line pointers: 'See docs/helix/06-iterate/reliability-principles.md for the 7 reliability principles applied to ddx try / ddx work execution.'\n4. cd cli \u0026\u0026 go test ./... still green (no code change; sanity check that nothing broke).\n5. lefthook run pre-commit passes.\n6. Conventional commit ending [\u003cthis-bead-id\u003e].",
+    "parent": "ddx-e34994e2",
+    "labels": [
+      "phase:2",
+      "area:agent",
+      "area:server",
+      "kind:design",
+      "reliability",
+      "bead-quality"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T07:41:36Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\",\"fallback_chain\":[]}",
+          "created_at": "2026-05-04T22:20:59.208009027Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "erik",
+          "body": "exit status 1\nresult_rev=0d05405dd919e77ce5487734852b9ae6462d9f95\nbase_rev=0d05405dd919e77ce5487734852b9ae6462d9f95\nretry_after=2026-05-05T04:20:59Z",
+          "created_at": "2026-05-04T22:20:59.981150814Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T07:41:36.699413469Z",
+      "execute-loop-last-detail": "exit status 1",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-05-05T04:20:59Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T074136-36b65cad",
+    "prompt": ".ddx/executions/20260505T074136-36b65cad/prompt.md",
+    "manifest": ".ddx/executions/20260505T074136-36b65cad/manifest.json",
+    "result": ".ddx/executions/20260505T074136-36b65cad/result.json",
+    "checks": ".ddx/executions/20260505T074136-36b65cad/checks.json",
+    "usage": ".ddx/executions/20260505T074136-36b65cad/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-06b77652-20260505T074136-36b65cad"
+  },
+  "prompt_sha": "0428918b10d656a5dd165e6ecf71e5739a3a9a149f488ed8a9639d6f32eb197a"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T074136-36b65cad/result.json b/.ddx/executions/20260505T074136-36b65cad/result.json
new file mode 100644
index 00000000..4eef497c
--- /dev/null
+++ b/.ddx/executions/20260505T074136-36b65cad/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-06b77652",
+  "attempt_id": "20260505T074136-36b65cad",
+  "base_rev": "24359509fbd370840c63c52588d2e722756f0fd8",
+  "result_rev": "2285f17b89ff1db0dfdd91a7c37ad8e58914399e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-d42cf023",
+  "duration_ms": 255317,
+  "tokens": 4228856,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T074136-36b65cad",
+  "prompt_file": ".ddx/executions/20260505T074136-36b65cad/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T074136-36b65cad/manifest.json",
+  "result_file": ".ddx/executions/20260505T074136-36b65cad/result.json",
+  "usage_file": ".ddx/executions/20260505T074136-36b65cad/usage.json",
+  "started_at": "2026-05-05T07:41:38.634446445Z",
+  "finished_at": "2026-05-05T07:45:53.952240577Z"
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
