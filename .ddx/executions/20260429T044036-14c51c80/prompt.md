<bead-review>
  <bead id="ddx-5538aa5b" iter=1>
    <title>execute-loop: tier resolver should consume catalog instead of requiring per-project model_overrides</title>
    <description>
When agent.routing.model_overrides is not set in a project's .ddx/config.yaml, ResolveTierModelRef in cli/internal/agent/profile_ladder.go:44 returns the literal tier name ("cheap", "standard", "smart") as the model identifier. ResolveRoute is then called with model="cheap"/"standard"/"smart" — strings that match no model id and no named route. Every tier resolves to an empty harness/empty model and is recorded as a tier-attempt with body "no viable harness found". The loop exhausts all tiers and reports "execute-loop: all tiers exhausted — no viable provider found".

This is happening today on a workstation that has bragi/vidar/hel hosting qwen3.6, plus working claude and codex harnesses. execute-bead succeeds with the same providers when --harness/--model are pinned explicitly, proving the underlying routing is fine. Only the tier→model resolution path is broken.

The model catalog (~/.ddx/model-catalog.yaml or built-in defaults, surfaced via `ddx agent catalog show`) ALREADY contains the tier→surface→model mapping that would make this work. The resolver just doesn't consume it. Surfaces and concrete models are listed per tier (e.g. cheap: embedded-openai→qwen3.5-27b, codex→gpt-5.4-mini, claude→claude-haiku-4-5) — the resolver should walk these surfaces in order, picking the first whose harness is healthy and whose model resolves to a reachable provider via model_routes. Per-project model_overrides should be an OPTIONAL override layered on top of catalog defaults, not the only thing that makes tier resolution work.

Caller-side workaround: every project must add agent.routing.model_overrides to .ddx/config.yaml before execute-loop will function. This is silent: there is no diagnostic from `ddx agent doctor`, no warning from `ddx agent catalog show`, and no hint in the "no viable provider" error pointing at the missing config. Users have to read source to understand why a fully-healthy provider/harness fleet produces zero viable tiers.

Repro:
1. Fresh project with .ddx/config.yaml lacking agent.routing.model_overrides
2. Healthy provider config in ~/.config/agent/config.yaml (any qwen/claude/codex)
3. `ddx agent execute-loop --once --local --no-adaptive-min-tier`
4. Every tier-attempt event records "no viable harness found"; loop exits with 'all tiers exhausted'

Related symptom — the adaptive min-tier promotion uses cheap-tier success rate to skip cheap, but with the bug above no cheap-tier attempt has ever produced a successful harness invocation, so the trailing window is biased to 0% by design rather than by actual model performance.
    </description>
    <acceptance>
AC1. ResolveTierModelRef (or its replacement) consults the model catalog as the default source of tier→model mapping when agent.routing.model_overrides is not set, returning the first surface→model pair whose harness is available and whose model resolves to a healthy provider.

AC2. `ddx agent execute-loop --once --local --no-adaptive-min-tier` succeeds against a fresh project (no model_overrides in .ddx/config.yaml) when the catalog has at least one tier with a reachable harness/model/provider chain.

AC3. When EVERY catalog surface for the resolved tier is unreachable, the tier-attempt event records the concrete reason for each surface tried (e.g. 'codex: harness not installed', 'claude: provider cooldown', 'embedded-openai: model qwen3.6 has no reachable provider'), not a generic 'no viable harness found' with empty fields.

AC4. `ddx agent doctor` (or `ddx agent catalog show`) emits a warning when a tier's catalog entries reference a model id that has no matching named route AND no provider /v1/models entry across configured providers, so the user discovers the gap before invoking execute-loop.

AC5. Per-project agent.routing.model_overrides remains a supported override that wins over catalog defaults — backwards compatible.
    </acceptance>
    <labels>area:agent, area:routing, kind:design-flaw</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T041456-2c6831a6/manifest.json</file>
    <file>.ddx/executions/20260429T041456-2c6831a6/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="351dc52260be4f5d45484fc8a8d77500fb35f48b">
diff --git a/.ddx/executions/20260429T041456-2c6831a6/manifest.json b/.ddx/executions/20260429T041456-2c6831a6/manifest.json
new file mode 100644
index 00000000..b5c11fa8
--- /dev/null
+++ b/.ddx/executions/20260429T041456-2c6831a6/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260429T041456-2c6831a6",
+  "bead_id": "ddx-5538aa5b",
+  "base_rev": "ac7636bbaa3c8c72f3c04b2df0a671cff1b465ae",
+  "created_at": "2026-04-29T04:14:57.078111785Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-5538aa5b",
+    "title": "execute-loop: tier resolver should consume catalog instead of requiring per-project model_overrides",
+    "description": "When agent.routing.model_overrides is not set in a project's .ddx/config.yaml, ResolveTierModelRef in cli/internal/agent/profile_ladder.go:44 returns the literal tier name (\"cheap\", \"standard\", \"smart\") as the model identifier. ResolveRoute is then called with model=\"cheap\"/\"standard\"/\"smart\" — strings that match no model id and no named route. Every tier resolves to an empty harness/empty model and is recorded as a tier-attempt with body \"no viable harness found\". The loop exhausts all tiers and reports \"execute-loop: all tiers exhausted — no viable provider found\".\n\nThis is happening today on a workstation that has bragi/vidar/hel hosting qwen3.6, plus working claude and codex harnesses. execute-bead succeeds with the same providers when --harness/--model are pinned explicitly, proving the underlying routing is fine. Only the tier→model resolution path is broken.\n\nThe model catalog (~/.ddx/model-catalog.yaml or built-in defaults, surfaced via `ddx agent catalog show`) ALREADY contains the tier→surface→model mapping that would make this work. The resolver just doesn't consume it. Surfaces and concrete models are listed per tier (e.g. cheap: embedded-openai→qwen3.5-27b, codex→gpt-5.4-mini, claude→claude-haiku-4-5) — the resolver should walk these surfaces in order, picking the first whose harness is healthy and whose model resolves to a reachable provider via model_routes. Per-project model_overrides should be an OPTIONAL override layered on top of catalog defaults, not the only thing that makes tier resolution work.\n\nCaller-side workaround: every project must add agent.routing.model_overrides to .ddx/config.yaml before execute-loop will function. This is silent: there is no diagnostic from `ddx agent doctor`, no warning from `ddx agent catalog show`, and no hint in the \"no viable provider\" error pointing at the missing config. Users have to read source to understand why a fully-healthy provider/harness fleet produces zero viable tiers.\n\nRepro:\n1. Fresh project with .ddx/config.yaml lacking agent.routing.model_overrides\n2. Healthy provider config in ~/.config/agent/config.yaml (any qwen/claude/codex)\n3. `ddx agent execute-loop --once --local --no-adaptive-min-tier`\n4. Every tier-attempt event records \"no viable harness found\"; loop exits with 'all tiers exhausted'\n\nRelated symptom — the adaptive min-tier promotion uses cheap-tier success rate to skip cheap, but with the bug above no cheap-tier attempt has ever produced a successful harness invocation, so the trailing window is biased to 0% by design rather than by actual model performance.",
+    "acceptance": "AC1. ResolveTierModelRef (or its replacement) consults the model catalog as the default source of tier→model mapping when agent.routing.model_overrides is not set, returning the first surface→model pair whose harness is available and whose model resolves to a healthy provider.\n\nAC2. `ddx agent execute-loop --once --local --no-adaptive-min-tier` succeeds against a fresh project (no model_overrides in .ddx/config.yaml) when the catalog has at least one tier with a reachable harness/model/provider chain.\n\nAC3. When EVERY catalog surface for the resolved tier is unreachable, the tier-attempt event records the concrete reason for each surface tried (e.g. 'codex: harness not installed', 'claude: provider cooldown', 'embedded-openai: model qwen3.6 has no reachable provider'), not a generic 'no viable harness found' with empty fields.\n\nAC4. `ddx agent doctor` (or `ddx agent catalog show`) emits a warning when a tier's catalog entries reference a model id that has no matching named route AND no provider /v1/models entry across configured providers, so the user discovers the gap before invoking execute-loop.\n\nAC5. Per-project agent.routing.model_overrides remains a supported override that wins over catalog defaults — backwards compatible.",
+    "labels": [
+      "area:agent",
+      "area:routing",
+      "kind:design-flaw"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T04:14:56Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "131811",
+      "execute-loop-heartbeat-at": "2026-04-29T04:14:56.405700261Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T041456-2c6831a6",
+    "prompt": ".ddx/executions/20260429T041456-2c6831a6/prompt.md",
+    "manifest": ".ddx/executions/20260429T041456-2c6831a6/manifest.json",
+    "result": ".ddx/executions/20260429T041456-2c6831a6/result.json",
+    "checks": ".ddx/executions/20260429T041456-2c6831a6/checks.json",
+    "usage": ".ddx/executions/20260429T041456-2c6831a6/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-5538aa5b-20260429T041456-2c6831a6"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T041456-2c6831a6/result.json b/.ddx/executions/20260429T041456-2c6831a6/result.json
new file mode 100644
index 00000000..727540c9
--- /dev/null
+++ b/.ddx/executions/20260429T041456-2c6831a6/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-5538aa5b",
+  "attempt_id": "20260429T041456-2c6831a6",
+  "base_rev": "ac7636bbaa3c8c72f3c04b2df0a671cff1b465ae",
+  "result_rev": "afa6e8e88e90d59cb0e61bd4c9967d0b8fa942c7",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-f61475d0",
+  "duration_ms": 1535609,
+  "tokens": 34,
+  "cost_usd": 7.5364235,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T041456-2c6831a6",
+  "prompt_file": ".ddx/executions/20260429T041456-2c6831a6/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T041456-2c6831a6/manifest.json",
+  "result_file": ".ddx/executions/20260429T041456-2c6831a6/result.json",
+  "usage_file": ".ddx/executions/20260429T041456-2c6831a6/usage.json",
+  "started_at": "2026-04-29T04:14:57.078373617Z",
+  "finished_at": "2026-04-29T04:40:32.688366797Z"
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
