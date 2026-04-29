<bead-review>
  <bead id="ddx-98e6e9ef" iter=1>
    <title>routing-preflight-gate: typed-incompatibility check before bead Claim</title>
    <description>
Cover D3 from ddx-fdd3ea36. After ResolveRoute returns, DDx verifies the decision's (Harness, Model) is not in the typed-incompatibility set surfaced by upstream. Compatibility data comes from upstream — DDx does NOT duplicate the allow-list. Insertion point: cli/internal/agent/execute_bead_loop.go (current line ~341–352, before Claim). Route error OR preflight rejection exits the loop with a worker-level failure record; no bead is claimed, no burn cycle. Defensive only.
    </description>
    <acceptance>
1. Call sequence at execute_bead_loop.go (pre-Claim): next-candidate → ResolveRoute → preflight gate (new) → Claim → invoke. 2. Test: seed a pathological route (harness with allow-list that excludes the decision's model — fabricated via upstream test seam). Assert beadStore.Claim is NEVER called. 3. Test: worker record shows status: execution_failed with detail naming the rejected (harness, model) pair. 4. Test: no bead has a kind:tier-attempt event from this attempt. 5. No DDx-side allow-list logic — preflight only consumes the typed error / decision-trace surface from upstream.
    </acceptance>
    <labels>feat-006, routing</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T123156-6eefce3d/manifest.json</file>
    <file>.ddx/executions/20260429T123156-6eefce3d/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="115b47c073ae267ebcde8185f17bc3ba1a7d53d2">
diff --git a/.ddx/executions/20260429T123156-6eefce3d/manifest.json b/.ddx/executions/20260429T123156-6eefce3d/manifest.json
new file mode 100644
index 00000000..f60c8afa
--- /dev/null
+++ b/.ddx/executions/20260429T123156-6eefce3d/manifest.json
@@ -0,0 +1,116 @@
+{
+  "attempt_id": "20260429T123156-6eefce3d",
+  "bead_id": "ddx-98e6e9ef",
+  "base_rev": "61dd52e6e98538aef837c2903ffa2840cb9267c6",
+  "created_at": "2026-04-29T12:31:57.982384171Z",
+  "requested": {
+    "harness": "agent",
+    "model": "qwen/qwen3.6-35b-a3b",
+    "provider": "lmstudio-bragi-1234",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-98e6e9ef",
+    "title": "routing-preflight-gate: typed-incompatibility check before bead Claim",
+    "description": "Cover D3 from ddx-fdd3ea36. After ResolveRoute returns, DDx verifies the decision's (Harness, Model) is not in the typed-incompatibility set surfaced by upstream. Compatibility data comes from upstream — DDx does NOT duplicate the allow-list. Insertion point: cli/internal/agent/execute_bead_loop.go (current line ~341–352, before Claim). Route error OR preflight rejection exits the loop with a worker-level failure record; no bead is claimed, no burn cycle. Defensive only.",
+    "acceptance": "1. Call sequence at execute_bead_loop.go (pre-Claim): next-candidate → ResolveRoute → preflight gate (new) → Claim → invoke. 2. Test: seed a pathological route (harness with allow-list that excludes the decision's model — fabricated via upstream test seam). Assert beadStore.Claim is NEVER called. 3. Test: worker record shows status: execution_failed with detail naming the rejected (harness, model) pair. 4. Test: no bead has a kind:tier-attempt event from this attempt. 5. No DDx-side allow-list logic — preflight only consumes the typed error / decision-trace surface from upstream.",
+    "parent": "ddx-fdd3ea36",
+    "labels": [
+      "feat-006",
+      "routing"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T12:31:52Z",
+      "claimed-machine": "sindri",
+      "claimed-pid": "539353",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-29T03:33:28.268835626Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-29T03:33:28.417049924Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-29T03:33:28.536896441Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-04-29T03:33:28.750210339Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"lmstudio-bragi-1234\",\"resolved_model\":\"qwen/qwen3.6-35b-a3b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-29T03:55:39.152636775Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=lmstudio-bragi-1234 model=qwen/qwen3.6-35b-a3b"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260429T034740-115acdc0\",\"harness\":\"agent\",\"provider\":\"lmstudio-bragi-1234\",\"model\":\"qwen/qwen3.6-35b-a3b\",\"input_tokens\":134730,\"output_tokens\":1969,\"total_tokens\":136699,\"cost_usd\":0,\"duration_ms\":475571,\"exit_code\":0}",
+          "created_at": "2026-04-29T03:55:39.445860912Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=136699 model=qwen/qwen3.6-35b-a3b"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=cheap harness=agent model=qwen/qwen3.6-35b-a3b probe=ok\nno_changes",
+          "created_at": "2026-04-29T03:55:40.055503795Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"cheap\",\"harness\":\"agent\",\"model\":\"qwen/qwen3.6-35b-a3b\",\"status\":\"no_changes\",\"cost_usd\":0,\"duration_ms\":475571}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-29T03:55:40.243663074Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=1 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "erik",
+          "body": "no_changes\ntier=cheap\nprobe_result=ok\nresult_rev=06609fe9aa60f101a3a308a0bde726bb2b7c7926\nbase_rev=06609fe9aa60f101a3a308a0bde726bb2b7c7926\nretry_after=2026-04-29T09:55:40Z",
+          "created_at": "2026-04-29T03:55:40.834124384Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-29T12:31:52.24557952Z",
+      "execute-loop-last-detail": "no_changes",
+      "execute-loop-last-status": "no_changes",
+      "execute-loop-no-changes-count": 1,
+      "execute-loop-retry-after": "2026-04-29T09:55:40Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T123156-6eefce3d",
+    "prompt": ".ddx/executions/20260429T123156-6eefce3d/prompt.md",
+    "manifest": ".ddx/executions/20260429T123156-6eefce3d/manifest.json",
+    "result": ".ddx/executions/20260429T123156-6eefce3d/result.json",
+    "checks": ".ddx/executions/20260429T123156-6eefce3d/checks.json",
+    "usage": ".ddx/executions/20260429T123156-6eefce3d/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-98e6e9ef-20260429T123156-6eefce3d"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T123156-6eefce3d/result.json b/.ddx/executions/20260429T123156-6eefce3d/result.json
new file mode 100644
index 00000000..5b6d9a29
--- /dev/null
+++ b/.ddx/executions/20260429T123156-6eefce3d/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-98e6e9ef",
+  "attempt_id": "20260429T123156-6eefce3d",
+  "base_rev": "61dd52e6e98538aef837c2903ffa2840cb9267c6",
+  "result_rev": "4e2d22550b990829e0f5a36e81015a21ce9b2570",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "agent",
+  "provider": "lmstudio-bragi-1234",
+  "model": "qwen/qwen3.6-35b-a3b",
+  "session_id": "eb-8f4583ad",
+  "duration_ms": 761180,
+  "tokens": 511245,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T123156-6eefce3d",
+  "prompt_file": ".ddx/executions/20260429T123156-6eefce3d/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T123156-6eefce3d/manifest.json",
+  "result_file": ".ddx/executions/20260429T123156-6eefce3d/result.json",
+  "usage_file": ".ddx/executions/20260429T123156-6eefce3d/usage.json",
+  "started_at": "2026-04-29T12:31:57.9837706Z",
+  "finished_at": "2026-04-29T12:44:39.164718716Z"
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
