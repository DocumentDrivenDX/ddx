<bead-review>
  <bead id="ddx-2c63bb95" iter=1>
    <title>agent: hardcoded 15-minute DefaultProviderRequestTimeout kills thinking-model requests; not configurable</title>
    <description>
Source: cli/internal/agent/serviceconfig.go:30
  const DefaultProviderRequestTimeout = 15 * time.Minute

This is hardcoded, not surfaced through .ddx/config.yaml or any flag, and is applied uniformly to every provider regardless of model class. It is set on every service path:
  - cli/internal/agent/agent_runner_service.go:116  ProviderTimeout: DefaultProviderRequestTimeout
  - cli/internal/agent/service_run.go:153           ProviderTimeout: DefaultProviderRequestTimeout

The comment justifies it as 'Defeats RC4 of ddx-0a651925: a stalled TCP socket that has delivered headers but stopped emitting body bytes would otherwise pin a goroutine until the outer wall-clock (3h) frees it.'

But that scenario is already handled by a separate idle-read timeout immediately below, on line 34:
  const DefaultProviderIdleReadTimeout = 5 * time.Minute

The 5-minute idle-read timeout is exactly the right tool for the stalled-TCP-socket case (no body bytes for 5 min → fail). The 15-minute wall-clock cap on a single request is the WRONG tool — it kills any legitimately long single Chat call regardless of streaming health.

Concrete impact observed today (2026-04-29 axon repo):

1. axon-db7a6d0a worker (qwen3.6-35b-a3b on bragi via lmstudio)
   - status: no_changes
   - duration_ms: 1,762,081 (29 min total — one timeout + retry)
   - log: 'WARN provider error, retrying attempt=1 err="provider request timeout: wall-clock 15m0s" delay=1s'
   - Retry consumed another full 15 min, also timed out, no commits produced.

2. axon-80979cb8 + axon-db7a6d0a + axon-ab2e52e0 (3-worker cascade re-launch)
   - All 3 hit 'provider request timeout: wall-clock 15m0s' on the first attempt
   - User killed the run before retries could waste another 15 min × 3 each

Why thinking models hit this:
qwen3.6-35b-a3b and similar 'thinking' / chain-of-thought reasoning models can spend 5–10+ minutes generating reasoning tokens before producing the first body delta the harness considers progress. Streaming output IS happening (so the idle-read timeout would NOT fire), but the harness is counting wall-clock against the entire request. A single hard problem can blow the 15-minute cap legitimately.

Why arbitrary timeouts are not helping:

a) Same value for cloud providers (claude, codex) and local providers (lmstudio, omlx). Local thinking models routinely need &gt;15 min on hard prompts; claude rarely does. Same cap is wrong for both classes.

b) No way to opt out of the cap per provider, per model, per request, per project. /home/erik/Projects/axon/.ddx/config.yaml has no knob.

c) Retries inherit the same cap. A timeout-prone session retries with the same fate. Burns cycles and tokens.

d) The cap defeats the cascade: when a cheap-tier qwen times out, escalation to standard claude is intended — but on a thinking model the timeout doesn't mean 'broken provider', it means 'this prompt is hard'. We then escalate after wasting 15 min, and the standard tier solves the problem in 2. Net effect of cheap-tier-first becomes negative.
    </description>
    <acceptance>
AC1. ProviderTimeout is configurable per-project via .ddx/config.yaml under agent.routing or agent.endpoints — e.g. agent.endpoints[provider].request_timeout_seconds — and the current 15-minute hardcoded constant becomes a default that can be overridden upward (or removed entirely) per provider.

AC2. ProviderTimeout is configurable per-tier via the model-overrides path: cheap tier on a known thinking model (qwen3.6, deepseek-r1, etc.) defaults to a longer cap (e.g. 60 min or unlimited). Cloud chat models keep the 15-min default.

AC3. The idle-read timeout (DefaultProviderIdleReadTimeout = 5 min) is the primary stalled-TCP-socket defense; documentation in cli/internal/agent/serviceconfig.go and the user-facing config docs explain the relationship between idle-read and wall-clock-request timeouts and when each fires.

AC4. When ProviderTimeout fires, the error message names the exact timeout setting and the path to override it (e.g. 'request_timeout=15m exceeded; configure agent.endpoints.&lt;name&gt;.request_timeout_seconds in .ddx/config.yaml').

AC5. execute-loop / execute-bead --request-timeout DURATION CLI flag exists for one-off overrides during debugging.

AC6. Default for known-thinking-model surfaces (provider.type=lmstudio AND model name matches qwen3.6/qwen-r1/deepseek-r1 etc., or some other declared 'thinking' attribute) is bumped to a sensible value (e.g. 60 min) without requiring per-project config.
    </acceptance>
    <labels>area:agent, area:routing, area:harness, kind:design-flaw</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T205818-1f396b1e/manifest.json</file>
    <file>.ddx/executions/20260429T205818-1f396b1e/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="99375eaf330ea13838d40d2176d193ae3cd3453b">
diff --git a/.ddx/executions/20260429T205818-1f396b1e/manifest.json b/.ddx/executions/20260429T205818-1f396b1e/manifest.json
new file mode 100644
index 00000000..ea9d4692
--- /dev/null
+++ b/.ddx/executions/20260429T205818-1f396b1e/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260429T205818-1f396b1e",
+  "bead_id": "ddx-2c63bb95",
+  "base_rev": "943b4498470135fecc5b5c5aa31a7292eea02426",
+  "created_at": "2026-04-29T20:58:19.632595983Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-2c63bb95",
+    "title": "agent: hardcoded 15-minute DefaultProviderRequestTimeout kills thinking-model requests; not configurable",
+    "description": "Source: cli/internal/agent/serviceconfig.go:30\n  const DefaultProviderRequestTimeout = 15 * time.Minute\n\nThis is hardcoded, not surfaced through .ddx/config.yaml or any flag, and is applied uniformly to every provider regardless of model class. It is set on every service path:\n  - cli/internal/agent/agent_runner_service.go:116  ProviderTimeout: DefaultProviderRequestTimeout\n  - cli/internal/agent/service_run.go:153           ProviderTimeout: DefaultProviderRequestTimeout\n\nThe comment justifies it as 'Defeats RC4 of ddx-0a651925: a stalled TCP socket that has delivered headers but stopped emitting body bytes would otherwise pin a goroutine until the outer wall-clock (3h) frees it.'\n\nBut that scenario is already handled by a separate idle-read timeout immediately below, on line 34:\n  const DefaultProviderIdleReadTimeout = 5 * time.Minute\n\nThe 5-minute idle-read timeout is exactly the right tool for the stalled-TCP-socket case (no body bytes for 5 min → fail). The 15-minute wall-clock cap on a single request is the WRONG tool — it kills any legitimately long single Chat call regardless of streaming health.\n\nConcrete impact observed today (2026-04-29 axon repo):\n\n1. axon-db7a6d0a worker (qwen3.6-35b-a3b on bragi via lmstudio)\n   - status: no_changes\n   - duration_ms: 1,762,081 (29 min total — one timeout + retry)\n   - log: 'WARN provider error, retrying attempt=1 err=\"provider request timeout: wall-clock 15m0s\" delay=1s'\n   - Retry consumed another full 15 min, also timed out, no commits produced.\n\n2. axon-80979cb8 + axon-db7a6d0a + axon-ab2e52e0 (3-worker cascade re-launch)\n   - All 3 hit 'provider request timeout: wall-clock 15m0s' on the first attempt\n   - User killed the run before retries could waste another 15 min × 3 each\n\nWhy thinking models hit this:\nqwen3.6-35b-a3b and similar 'thinking' / chain-of-thought reasoning models can spend 5–10+ minutes generating reasoning tokens before producing the first body delta the harness considers progress. Streaming output IS happening (so the idle-read timeout would NOT fire), but the harness is counting wall-clock against the entire request. A single hard problem can blow the 15-minute cap legitimately.\n\nWhy arbitrary timeouts are not helping:\n\na) Same value for cloud providers (claude, codex) and local providers (lmstudio, omlx). Local thinking models routinely need \u003e15 min on hard prompts; claude rarely does. Same cap is wrong for both classes.\n\nb) No way to opt out of the cap per provider, per model, per request, per project. /home/erik/Projects/axon/.ddx/config.yaml has no knob.\n\nc) Retries inherit the same cap. A timeout-prone session retries with the same fate. Burns cycles and tokens.\n\nd) The cap defeats the cascade: when a cheap-tier qwen times out, escalation to standard claude is intended — but on a thinking model the timeout doesn't mean 'broken provider', it means 'this prompt is hard'. We then escalate after wasting 15 min, and the standard tier solves the problem in 2. Net effect of cheap-tier-first becomes negative.",
+    "acceptance": "AC1. ProviderTimeout is configurable per-project via .ddx/config.yaml under agent.routing or agent.endpoints — e.g. agent.endpoints[provider].request_timeout_seconds — and the current 15-minute hardcoded constant becomes a default that can be overridden upward (or removed entirely) per provider.\n\nAC2. ProviderTimeout is configurable per-tier via the model-overrides path: cheap tier on a known thinking model (qwen3.6, deepseek-r1, etc.) defaults to a longer cap (e.g. 60 min or unlimited). Cloud chat models keep the 15-min default.\n\nAC3. The idle-read timeout (DefaultProviderIdleReadTimeout = 5 min) is the primary stalled-TCP-socket defense; documentation in cli/internal/agent/serviceconfig.go and the user-facing config docs explain the relationship between idle-read and wall-clock-request timeouts and when each fires.\n\nAC4. When ProviderTimeout fires, the error message names the exact timeout setting and the path to override it (e.g. 'request_timeout=15m exceeded; configure agent.endpoints.\u003cname\u003e.request_timeout_seconds in .ddx/config.yaml').\n\nAC5. execute-loop / execute-bead --request-timeout DURATION CLI flag exists for one-off overrides during debugging.\n\nAC6. Default for known-thinking-model surfaces (provider.type=lmstudio AND model name matches qwen3.6/qwen-r1/deepseek-r1 etc., or some other declared 'thinking' attribute) is bumped to a sensible value (e.g. 60 min) without requiring per-project config.",
+    "labels": [
+      "area:agent",
+      "area:routing",
+      "area:harness",
+      "kind:design-flaw"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T20:58:18Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T20:58:18.720347424Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T205818-1f396b1e",
+    "prompt": ".ddx/executions/20260429T205818-1f396b1e/prompt.md",
+    "manifest": ".ddx/executions/20260429T205818-1f396b1e/manifest.json",
+    "result": ".ddx/executions/20260429T205818-1f396b1e/result.json",
+    "checks": ".ddx/executions/20260429T205818-1f396b1e/checks.json",
+    "usage": ".ddx/executions/20260429T205818-1f396b1e/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-2c63bb95-20260429T205818-1f396b1e"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T205818-1f396b1e/result.json b/.ddx/executions/20260429T205818-1f396b1e/result.json
new file mode 100644
index 00000000..79d41368
--- /dev/null
+++ b/.ddx/executions/20260429T205818-1f396b1e/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-2c63bb95",
+  "attempt_id": "20260429T205818-1f396b1e",
+  "base_rev": "943b4498470135fecc5b5c5aa31a7292eea02426",
+  "result_rev": "28afd8e6857bffb956e92e3f591fccb6fddee921",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-2db5279a",
+  "duration_ms": 1043526,
+  "tokens": 31175,
+  "cost_usd": 2.9120964999999996,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T205818-1f396b1e",
+  "prompt_file": ".ddx/executions/20260429T205818-1f396b1e/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T205818-1f396b1e/manifest.json",
+  "result_file": ".ddx/executions/20260429T205818-1f396b1e/result.json",
+  "usage_file": ".ddx/executions/20260429T205818-1f396b1e/usage.json",
+  "started_at": "2026-04-29T20:58:19.632921858Z",
+  "finished_at": "2026-04-29T21:15:43.159792101Z"
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
