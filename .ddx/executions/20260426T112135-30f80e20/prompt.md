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
    <file>.ddx/executions/20260426T110752-4001c777/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="421d3f730f5dbf075c7b716ab7211d6f35ed189e">
diff --git a/.ddx/executions/20260426T110752-4001c777/result.json b/.ddx/executions/20260426T110752-4001c777/result.json
new file mode 100644
index 00000000..e8f22ce9
--- /dev/null
+++ b/.ddx/executions/20260426T110752-4001c777/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-c67969d5",
+  "attempt_id": "20260426T110752-4001c777",
+  "base_rev": "e8050ed88a0709c717a71b9555673dd130e35cf5",
+  "result_rev": "d9dfff911b06e17a3f85ca3c1796c27cb4c90ede",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-83b34a7c",
+  "duration_ms": 815774,
+  "tokens": 33714,
+  "cost_usd": 5.44008,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260426T110752-4001c777",
+  "prompt_file": ".ddx/executions/20260426T110752-4001c777/prompt.md",
+  "manifest_file": ".ddx/executions/20260426T110752-4001c777/manifest.json",
+  "result_file": ".ddx/executions/20260426T110752-4001c777/result.json",
+  "usage_file": ".ddx/executions/20260426T110752-4001c777/usage.json",
+  "started_at": "2026-04-26T11:07:52.697457017Z",
+  "finished_at": "2026-04-26T11:21:28.472361854Z"
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
