<bead-review>
  <bead id="ddx-aec9d68c" iter=1>
    <title>adaptive min-tier: provide a trivial reset; current state is sticky and unresettable</title>
    <description>
execute-loop's adaptive min-tier promotion (escalation.AdaptiveMinTierThreshold etc.) silently skips cheap tier when trailing cheap-tier success rate falls under threshold. Today the only visible signal is a single log line:

  adaptive min-tier: skipping cheap tier (trailing success rate 0.00 over 9 attempts; threshold 0.20) — min-tier=standard

There is no documented or discoverable way to reset this state. Issues with the current design:

1. STATE LOCATION IS NOT DOCUMENTED. The trailing-window metric is computed from .ddx/agent-logs/routing-outcomes.jsonl per-project (and possibly other sources — rotating that file alone did NOT clear the 0% verdict in a recent test, so additional state may live elsewhere). A user who wants to reset cannot find authoritative documentation of where the state lives.

2. NO RESET CLI. There is no `ddx agent route-status reset`, no `ddx agent metrics clear`, no `ddx agent doctor --reset-adaptive`. The only path is to edit/truncate JSONL files by hand, which is fragile and breaks compatibility with any future schema.

3. SILENT NOISE. Every cheap-tier failure permanently affects future routing without surfacing to the user. If a configuration bug (e.g. the model_overrides issue tracked as ddx-5538aa5b) caused 9 immediate failures, the adaptive logic now blocks cheap tier even after the configuration is fixed. The user has no way to know 'this is stale state, not real performance signal' without reading source.

4. MITIGATION VIA --no-adaptive-min-tier IS NOT A FIX, IT'S AN OVERRIDE. The flag bypasses the gating but also defeats the legitimate purpose of the metric. We need ability to RESET the metric and re-evaluate from a clean slate, not permanently disable the feature.

5. THE THRESHOLD/WINDOW DEFAULTS ARE AGGRESSIVE. Trailing window of 9-50 attempts at threshold 0.20 means a single cheap-tier provider misconfiguration (which is easy to hit, see ddx-5538aa5b) condemns cheap-tier for many subsequent invocations. Meanwhile EVERY configuration error is recorded as cheap-tier failure even when the failure was 'no viable provider' rather than 'model produced bad output'.

Requested behavior:

- A first-class CLI: `ddx agent route-status reset [--scope cheap|all]` that clears the adaptive metric for the current project, with a confirm prompt by default.
- A flag on `ddx agent doctor` that displays current adaptive state per tier (success rate, attempt count, threshold, current floor) so users can SEE the problem.
- An option to record only EXECUTION outcomes (model produced output) in adaptive metrics, NOT no-viable-provider/configuration failures — those should be tracked separately and never count against a tier's reputation.
- Documented location and schema for the metric store, so users who want to inspect or surgically edit understand the on-disk format.
    </description>
    <acceptance>
AC1. `ddx agent route-status reset` (or equivalent verb) exists and clears the trailing-window adaptive metric for the current project, returning min-tier evaluation to a clean baseline. Output names every file/store touched.

AC2. `ddx agent doctor` (or `ddx agent route-status`) prints current adaptive state per tier: window size, trailing attempts, success rate, threshold, current effective floor, and whether the tier is currently being skipped. Diagnostic output is enough to debug a 'why is cheap being skipped' scenario without reading source.

AC3. Routing failures classified as 'no viable provider', 'harness not installed', or 'configuration error' are recorded but EXCLUDED from the adaptive success-rate computation — only failures that reached an actual model invocation count.

AC4. Documentation in docs/ describes the adaptive min-tier mechanism, the metric store path and schema, the reset workflow, and the relationship between --no-adaptive-min-tier (temporary bypass) and reset (permanent state clear).

AC5. After running the reset CLI, a fresh `execute-loop --once --local` evaluates cheap tier on its own merits without the prior trailing-window verdict carrying over.
    </acceptance>
    <labels>area:agent, area:routing, kind:design-flaw, ux</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T162702-2c963978/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="ec38e39e5fc4e16ad3ab5d46ca860da3c37136bb">
diff --git a/.ddx/executions/20260429T162702-2c963978/result.json b/.ddx/executions/20260429T162702-2c963978/result.json
new file mode 100644
index 00000000..11359d98
--- /dev/null
+++ b/.ddx/executions/20260429T162702-2c963978/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-aec9d68c",
+  "attempt_id": "20260429T162702-2c963978",
+  "base_rev": "a95c92e1ec3a3c429261a0a3843444080b6621ae",
+  "result_rev": "cd0f5ed9ed8231ae932dc91db4fa317ca1ebfd60",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-43a934f2",
+  "duration_ms": 1100851,
+  "tokens": 51398,
+  "cost_usd": 3.1451620000000005,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T162702-2c963978",
+  "prompt_file": ".ddx/executions/20260429T162702-2c963978/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T162702-2c963978/manifest.json",
+  "result_file": ".ddx/executions/20260429T162702-2c963978/result.json",
+  "usage_file": ".ddx/executions/20260429T162702-2c963978/usage.json",
+  "started_at": "2026-04-29T16:27:03.433679234Z",
+  "finished_at": "2026-04-29T16:45:24.285294695Z"
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
