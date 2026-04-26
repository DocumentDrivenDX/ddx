<bead-review>
  <bead id="ddx-c67969d5" iter=1>
    <title>execute-loop: reopen-retry must reuse initial worker params (harness, model, profile)</title>
    <description>
Re-filed from agent-9465287e in /Users/erik/Projects/agent (closed there as 'tracked upstream'; the agent service is not the failure surface). Originally observed in ddx v0.6.2-alpha1 against agent v0.9.9 during SD-024 work.

## Symptom

Worker spawned with explicit pin:

    ddx work --harness claude

First attempt on a bead routes correctly to claude (e.g. opus-4.7) and produces a result. When the post-merge reviewer returns REQUEST_CHANGES the bead reopens. The next execute-loop iteration retries the bead but the worker's --harness override is NOT propagated to that retry. Tier-attempt events show:

    kind:tier-attempt body=tier=standard harness= model= probe=no viable provider, no viable harness found
    kind:tier-attempt body=tier=smart    harness= model= probe=no viable provider, no viable harness found
    kind:escalation-summary winning_tier=exhausted

Note harness= and model= empty — the retry constructs a service request with Harness="". Smart-routing returns zero candidates at every tier above cheap (cheap suppressed by adaptive min-tier from prior failures). Bead bounces between REQUEST_CHANGES (cycle 1) → execution_failed (cycle 2 with no harness routing) repeatedly.

Concrete instance: ddx-dd7fce7e in /Users/erik/Projects/ddx/.ddx/beads.jsonl shows the pattern at events around 2026-04-25T21:46-21:47Z.

## Principle

**Retries should use the same parameters as the initial work.** A reopened bead's retry must inherit every explicit pin (harness, model, model-ref, profile, provider) that the worker was spawned with. Otherwise the retry is a different request, not a retry — and the work cannot converge.

This is a general invariant, not specific to harness overrides. Any explicit pin the worker was launched with must propagate to every iteration on every bead the worker handles, including post-reopen retries.

## Diagnosis (from agent-9465287e worker analysis)

The agent service surface (CONTRACT-003) is correct. The agent has no tier concept — tier classification and adaptive min-tier floors are ddx-side abstractions layered on top of the agent's per-call routing. Loosening the agent's routing engine to return candidates when the harness is empty would change behavior for every other caller and is not the right lever.

The fix is a few lines of option-propagation in the ddx execute-loop's reopen-retry path: when a bead reopens after REQUEST_CHANGES (or any review-driven reopen), the loop must copy the worker-level explicit pins into the next iteration's ExecuteBeadOptions / service.Execute request, the same way it does on the initial claim.

## In-scope files

- cli/internal/agent/execute_bead_loop.go (or wherever the retry path constructs its execute-bead invocation)
- cli/internal/agent/execute_bead_review.go if review-driven reopens have a separate path
- Regression test asserting the second service.Execute call carries the same Harness, Model, Profile values as the first when the bead was reopened by a reviewer

## Out of scope

- Agent-side routing engine changes — the surface is correct
- Adaptive min-tier behavior tuning (separate concern)
- Tool-loop detection (separate concern, see agent-1bdb9b60 in agent repo)
    </description>
    <acceptance>
1. New regression test cd cli &amp;&amp; go test ./internal/agent/... -run TestExecuteLoopRetryReusesWorkerHarness asserts: spawn execute-loop with --harness claude; bead reaches REQUEST_CHANGES → reopen → next iteration's service.Execute request has Harness="claude" (not empty).
2. Same test covers --model, --profile, and --provider propagation across review-driven reopens.
3. Test covers all reopen sources (review BLOCK, review REQUEST_CHANGES, land_conflict, post_run_check_failed, no_changes if it triggers reopen) — every reopen path must propagate pins.
4. Negative test: worker spawned without --harness; reopen retry continues to use auto-routing (no propagation regression).
5. cd cli &amp;&amp; go test ./... -count=1 passes.
    </acceptance>
    <labels>ddx, kind:bug, area:agent, area:execute-loop, area:retry</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260426T112631-fce6a923/manifest.json</file>
    <file>.ddx/executions/20260426T112631-fce6a923/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="3afedb4c4ca3b4c6a8c192dabbfde45dc2852f29">
diff --git a/.ddx/executions/20260426T112631-fce6a923/manifest.json b/.ddx/executions/20260426T112631-fce6a923/manifest.json
new file mode 100644
index 00000000..982efc2e
--- /dev/null
+++ b/.ddx/executions/20260426T112631-fce6a923/manifest.json
@@ -0,0 +1,99 @@
+{
+  "attempt_id": "20260426T112631-fce6a923",
+  "bead_id": "ddx-c67969d5",
+  "base_rev": "de32a56257ca965d2235133af9833b637c6318b9",
+  "created_at": "2026-04-26T11:26:32.580140629Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-c67969d5",
+    "title": "execute-loop: reopen-retry must reuse initial worker params (harness, model, profile)",
+    "description": "Re-filed from agent-9465287e in /Users/erik/Projects/agent (closed there as 'tracked upstream'; the agent service is not the failure surface). Originally observed in ddx v0.6.2-alpha1 against agent v0.9.9 during SD-024 work.\n\n## Symptom\n\nWorker spawned with explicit pin:\n\n    ddx work --harness claude\n\nFirst attempt on a bead routes correctly to claude (e.g. opus-4.7) and produces a result. When the post-merge reviewer returns REQUEST_CHANGES the bead reopens. The next execute-loop iteration retries the bead but the worker's --harness override is NOT propagated to that retry. Tier-attempt events show:\n\n    kind:tier-attempt body=tier=standard harness= model= probe=no viable provider, no viable harness found\n    kind:tier-attempt body=tier=smart    harness= model= probe=no viable provider, no viable harness found\n    kind:escalation-summary winning_tier=exhausted\n\nNote harness= and model= empty — the retry constructs a service request with Harness=\"\". Smart-routing returns zero candidates at every tier above cheap (cheap suppressed by adaptive min-tier from prior failures). Bead bounces between REQUEST_CHANGES (cycle 1) → execution_failed (cycle 2 with no harness routing) repeatedly.\n\nConcrete instance: ddx-dd7fce7e in /Users/erik/Projects/ddx/.ddx/beads.jsonl shows the pattern at events around 2026-04-25T21:46-21:47Z.\n\n## Principle\n\n**Retries should use the same parameters as the initial work.** A reopened bead's retry must inherit every explicit pin (harness, model, model-ref, profile, provider) that the worker was spawned with. Otherwise the retry is a different request, not a retry — and the work cannot converge.\n\nThis is a general invariant, not specific to harness overrides. Any explicit pin the worker was launched with must propagate to every iteration on every bead the worker handles, including post-reopen retries.\n\n## Diagnosis (from agent-9465287e worker analysis)\n\nThe agent service surface (CONTRACT-003) is correct. The agent has no tier concept — tier classification and adaptive min-tier floors are ddx-side abstractions layered on top of the agent's per-call routing. Loosening the agent's routing engine to return candidates when the harness is empty would change behavior for every other caller and is not the right lever.\n\nThe fix is a few lines of option-propagation in the ddx execute-loop's reopen-retry path: when a bead reopens after REQUEST_CHANGES (or any review-driven reopen), the loop must copy the worker-level explicit pins into the next iteration's ExecuteBeadOptions / service.Execute request, the same way it does on the initial claim.\n\n## In-scope files\n\n- cli/internal/agent/execute_bead_loop.go (or wherever the retry path constructs its execute-bead invocation)\n- cli/internal/agent/execute_bead_review.go if review-driven reopens have a separate path\n- Regression test asserting the second service.Execute call carries the same Harness, Model, Profile values as the first when the bead was reopened by a reviewer\n\n## Out of scope\n\n- Agent-side routing engine changes — the surface is correct\n- Adaptive min-tier behavior tuning (separate concern)\n- Tool-loop detection (separate concern, see agent-1bdb9b60 in agent repo)",
+    "acceptance": "1. New regression test cd cli \u0026\u0026 go test ./internal/agent/... -run TestExecuteLoopRetryReusesWorkerHarness asserts: spawn execute-loop with --harness claude; bead reaches REQUEST_CHANGES → reopen → next iteration's service.Execute request has Harness=\"claude\" (not empty).\n2. Same test covers --model, --profile, and --provider propagation across review-driven reopens.\n3. Test covers all reopen sources (review BLOCK, review REQUEST_CHANGES, land_conflict, post_run_check_failed, no_changes if it triggers reopen) — every reopen path must propagate pins.\n4. Negative test: worker spawned without --harness; reopen retry continues to use auto-routing (no propagation regression).\n5. cd cli \u0026\u0026 go test ./... -count=1 passes.",
+    "labels": [
+      "ddx",
+      "kind:bug",
+      "area:agent",
+      "area:execute-loop",
+      "area:retry"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-26T11:26:31Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "4075646",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"pi\",\"resolved_model\":\"Qwen3.6-27B-MLX-8bit\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-25T23:34:55.348743958Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=pi model=Qwen3.6-27B-MLX-8bit"
+        },
+        {
+          "actor": "ddx",
+          "body": "agent: execute: model \"Qwen3.6-27B-MLX-8bit\" is not supported by harness \"pi\"; supported models: gemini-2.5-flash, gemini-2.5-pro\nresult_rev=ce96aa009393d783df09c6d5cead50b24663f855\nbase_rev=ce96aa009393d783df09c6d5cead50b24663f855\nretry_after=2026-04-26T05:34:55Z",
+          "created_at": "2026-04-25T23:34:55.759496395Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-26T11:21:28.474715477Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260426T110752-4001c777\",\"harness\":\"claude\",\"input_tokens\":74,\"output_tokens\":33640,\"total_tokens\":33714,\"cost_usd\":5.44008,\"duration_ms\":815774,\"exit_code\":0}",
+          "created_at": "2026-04-26T11:21:28.558733575Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=33714 cost_usd=5.4401"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"\",\"resolved_provider\":\"claude\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-04-26T11:21:35.375609134Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "failure_class=review-error: provider_empty\nattempt_count=1\nresult_rev=421d3f730f5dbf075c7b716ab7211d6f35ed189e\n\nreviewer: review-error: provider_empty: reviewer output: unparseable JSON verdict: empty input (raw output 0 bytes; see .ddx/executions/20260426T112135-30f80e20)\nharness=codex\nmodel=gpt-5.4\ninput_bytes=7155\noutput_bytes=0\nelapsed_ms=3056",
+          "created_at": "2026-04-26T11:21:40.651153785Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: provider_empty"
+        },
+        {
+          "actor": "ddx",
+          "body": "merged onto current tip\nresult_rev=421d3f730f5dbf075c7b716ab7211d6f35ed189e\nbase_rev=e8050ed88a0709c717a71b9555673dd130e35cf5",
+          "created_at": "2026-04-26T11:21:40.719943562Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-26T11:26:31.796395478Z",
+      "execute-loop-last-detail": "agent: execute: model \"Qwen3.6-27B-MLX-8bit\" is not supported by harness \"pi\"; supported models: gemini-2.5-flash, gemini-2.5-pro",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-04-26T05:34:55Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260426T112631-fce6a923",
+    "prompt": ".ddx/executions/20260426T112631-fce6a923/prompt.md",
+    "manifest": ".ddx/executions/20260426T112631-fce6a923/manifest.json",
+    "result": ".ddx/executions/20260426T112631-fce6a923/result.json",
+    "checks": ".ddx/executions/20260426T112631-fce6a923/checks.json",
+    "usage": ".ddx/executions/20260426T112631-fce6a923/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-c67969d5-20260426T112631-fce6a923"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260426T112631-fce6a923/result.json b/.ddx/executions/20260426T112631-fce6a923/result.json
new file mode 100644
index 00000000..5ada91e4
--- /dev/null
+++ b/.ddx/executions/20260426T112631-fce6a923/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-c67969d5",
+  "attempt_id": "20260426T112631-fce6a923",
+  "base_rev": "de32a56257ca965d2235133af9833b637c6318b9",
+  "result_rev": "c11710b65643a2a57295ab1a9018706322c0c8c8",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-035ecaef",
+  "duration_ms": 308799,
+  "tokens": 11721,
+  "cost_usd": 1.85586775,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260426T112631-fce6a923",
+  "prompt_file": ".ddx/executions/20260426T112631-fce6a923/prompt.md",
+  "manifest_file": ".ddx/executions/20260426T112631-fce6a923/manifest.json",
+  "result_file": ".ddx/executions/20260426T112631-fce6a923/result.json",
+  "usage_file": ".ddx/executions/20260426T112631-fce6a923/usage.json",
+  "started_at": "2026-04-26T11:26:32.580498337Z",
+  "finished_at": "2026-04-26T11:31:41.379711768Z"
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
