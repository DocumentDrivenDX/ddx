<bead-review>
  <bead id="ddx-3f406266" iter=1>
    <title>routing-unify: delegate provider/harness/cost/local-first routing to agent SDK; ddx preserves execution-unit metadata + operator policy</title>
    <description>
## Status update (filed 2026-04-29 12:50 UTC)

The bulk of this unification **already shipped** as `ddx-87fb72c2` (epic `ddx-fdd3ea36`) before this epic was filed:

- `agent.routing.profile_ladders` and `agent.routing.model_overrides` are now opt-in (consulted only with `--escalate` / `--override-model`).
- The default execute path on the CLI (`cli/cmd/agent_cmd.go`) calls `ResolveRoute` once per attempt instead of iterating a tier ladder.
- `agent.routing.default_harness` was removed (hard error if still present).
- `agent.ResolveTierCandidates` (from `ddx-5538aa5b`) replaced the broken literal-tier-name-as-Model pattern; tier resolution now consults the catalog.
- See `docs/migrations/routing-config.md` for the migration the user-facing CLI surface follows.

**Remaining gap (this epic now scopes only this):** the server worker path in `cli/internal/server/workers.go:650` was not updated by `ddx-87fb72c2`. It still derives `escalationEnabled := spec.Harness == "" &amp;&amp; spec.Model == ""` instead of mirroring the CLI's `escalate &amp;&amp; harness == "" &amp;&amp; model == ""`. That mismatch is why `ddx work` (default mode, server-submit) still hits "all tiers exhausted — no viable provider" with `model_overrides` pointing at non-served models, while `ddx work --local --no-adaptive-min-tier` succeeds. Verified live in this session: `worker-20260429T033325-bedf` failed; `--no-adaptive-min-tier` got `ddx-98e6e9ef` to merge at `115b47c0` on local-tier qwen for zero cost.

Sole child: `ddx-c7081f89`. Once that lands, this epic closes.

## Strategic direction (preserved for future reference)

DDx embeds the agent SDK as a Go library to produce traceable execution units: `(bead, input_state) → (output_state, processing_metadata)`. DDx owns the execution unit and its metadata (bead events, cost ledger, escalation summary, evidence directories), and it owns operator policy (`--harness`/`--model`/`--profile`/`--max-cost`/`--no-review`/`--escalate`). It does **not** need to own provider/harness/cost/local-first routing — agent SDK already does that.

| Concern | DDx (now, post-87fb72c2) | Agent SDK |
|---|---|---|
| Profile selection | `RouteRequest.Profile` straight to agent | `default/cheap/standard/smart/local/offline/air-gapped/fast/code-smart/code-high` |
| Per-tier model pinning | opt-in via `--override-model` | `RouteRequest.Model` |
| Escalation on no-provider | opt-in via `--escalate` (CLI path); **not yet on server worker path** | `escalateProfileLadder` walks `ProfileEscalationLadder=[cheap,standard,smart]` on `ErrNoLiveProvider` |
| Cost-aware ranking | n/a (delegated) | `internal/routing/score.go` ranks by `CostClass` (local=0 wins) |
| Adaptive success-rate gating | `escalation.AdaptiveMinTier` (sticky 50-window cheap-tier lockout, only consumed via `--escalate`) | `routing.Inputs.HistoricalSuccess map[harness]float64` |
| Decision trace | bead events `kind:tier-attempt` per RouteCandidate | `RouteDecision.Candidates []RouteCandidate` |

## Optional follow-ups (not in this epic — file separately if/when needed)

These are no longer blockers; track separately if pursued:

- **Default profile = `local`** so the operator-default honors the local-LLM-first goal without an explicit `--profile local`. Currently default is `default` which maps via project config.
- **Inject `routing.Inputs.HistoricalSuccess` per-harness** to replace `escalation.AdaptiveMinTier` with agent-native success-rate routing. Frees DDx from maintaining its own success-window state machine.
- **Delete `cli/internal/agent/profile_ladder.go` helpers** once no caller (CLI or server) consumes them. Currently consumed only via `--escalate`; if `--escalate` is dropped or moved into agent, these can go.
- **Delete `cli/internal/escalation/escalation.go::AdaptiveMinTier`** once the HistoricalSuccess injection above is in place.

## Out of scope for this epic

- Changes to agent SDK source — if a gap is found, file an upstream bead under `~/Projects/agent/.ddx/`.
- UI/e2e work — the parent e2e drift epic `ddx-ccdf9cf9` covers that and is independent.
- Server source changes outside `cli/internal/server/workers.go` — no API endpoint changes, no GraphQL schema changes.
    </description>
    <acceptance>
Sole remaining child ddx-c7081f89 closes successfully. Then verify:

1. cd cli &amp;&amp; go test ./internal/server/... ./internal/agent/... passes
2. cd /home/erik/Projects/ddx &amp;&amp; /home/erik/.local/bin/ddx work --once --local (no flags, no pin) successfully dispatches a single-call attempt against the next ready bead; the bead's events show no kind:tier-attempt entries (default path uses single ResolveRoute call)
3. cd /home/erik/Projects/ddx &amp;&amp; /home/erik/.local/bin/ddx work --once --local --escalate preserves the existing tier-loop behavior (kind:tier-attempt entries appear)
4. The bead-event taxonomy preserved on a default-path run: kind:routing fires once, kind:cost fires once, kind:execute-bead fires once for the outcome; kind:tier-attempt and kind:escalation-summary do NOT fire on the default path (they remain reserved for --escalate)
    </acceptance>
    <labels>area:routing, area:agent, kind:refactor, phase:build</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T170036-22cf74ed/manifest.json</file>
    <file>.ddx/executions/20260429T170036-22cf74ed/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="cea2c86cf7af867204961c7d8730331883b3a9d2">
diff --git a/.ddx/executions/20260429T170036-22cf74ed/manifest.json b/.ddx/executions/20260429T170036-22cf74ed/manifest.json
new file mode 100644
index 00000000..46f909db
--- /dev/null
+++ b/.ddx/executions/20260429T170036-22cf74ed/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260429T170036-22cf74ed",
+  "bead_id": "ddx-3f406266",
+  "base_rev": "f3240e8fabfb5fbe2ffaeb25c0520c345d0dca3d",
+  "created_at": "2026-04-29T17:00:37.253830066Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-3f406266",
+    "title": "routing-unify: delegate provider/harness/cost/local-first routing to agent SDK; ddx preserves execution-unit metadata + operator policy",
+    "description": "## Status update (filed 2026-04-29 12:50 UTC)\n\nThe bulk of this unification **already shipped** as `ddx-87fb72c2` (epic `ddx-fdd3ea36`) before this epic was filed:\n\n- `agent.routing.profile_ladders` and `agent.routing.model_overrides` are now opt-in (consulted only with `--escalate` / `--override-model`).\n- The default execute path on the CLI (`cli/cmd/agent_cmd.go`) calls `ResolveRoute` once per attempt instead of iterating a tier ladder.\n- `agent.routing.default_harness` was removed (hard error if still present).\n- `agent.ResolveTierCandidates` (from `ddx-5538aa5b`) replaced the broken literal-tier-name-as-Model pattern; tier resolution now consults the catalog.\n- See `docs/migrations/routing-config.md` for the migration the user-facing CLI surface follows.\n\n**Remaining gap (this epic now scopes only this):** the server worker path in `cli/internal/server/workers.go:650` was not updated by `ddx-87fb72c2`. It still derives `escalationEnabled := spec.Harness == \"\" \u0026\u0026 spec.Model == \"\"` instead of mirroring the CLI's `escalate \u0026\u0026 harness == \"\" \u0026\u0026 model == \"\"`. That mismatch is why `ddx work` (default mode, server-submit) still hits \"all tiers exhausted — no viable provider\" with `model_overrides` pointing at non-served models, while `ddx work --local --no-adaptive-min-tier` succeeds. Verified live in this session: `worker-20260429T033325-bedf` failed; `--no-adaptive-min-tier` got `ddx-98e6e9ef` to merge at `115b47c0` on local-tier qwen for zero cost.\n\nSole child: `ddx-c7081f89`. Once that lands, this epic closes.\n\n## Strategic direction (preserved for future reference)\n\nDDx embeds the agent SDK as a Go library to produce traceable execution units: `(bead, input_state) → (output_state, processing_metadata)`. DDx owns the execution unit and its metadata (bead events, cost ledger, escalation summary, evidence directories), and it owns operator policy (`--harness`/`--model`/`--profile`/`--max-cost`/`--no-review`/`--escalate`). It does **not** need to own provider/harness/cost/local-first routing — agent SDK already does that.\n\n| Concern | DDx (now, post-87fb72c2) | Agent SDK |\n|---|---|---|\n| Profile selection | `RouteRequest.Profile` straight to agent | `default/cheap/standard/smart/local/offline/air-gapped/fast/code-smart/code-high` |\n| Per-tier model pinning | opt-in via `--override-model` | `RouteRequest.Model` |\n| Escalation on no-provider | opt-in via `--escalate` (CLI path); **not yet on server worker path** | `escalateProfileLadder` walks `ProfileEscalationLadder=[cheap,standard,smart]` on `ErrNoLiveProvider` |\n| Cost-aware ranking | n/a (delegated) | `internal/routing/score.go` ranks by `CostClass` (local=0 wins) |\n| Adaptive success-rate gating | `escalation.AdaptiveMinTier` (sticky 50-window cheap-tier lockout, only consumed via `--escalate`) | `routing.Inputs.HistoricalSuccess map[harness]float64` |\n| Decision trace | bead events `kind:tier-attempt` per RouteCandidate | `RouteDecision.Candidates []RouteCandidate` |\n\n## Optional follow-ups (not in this epic — file separately if/when needed)\n\nThese are no longer blockers; track separately if pursued:\n\n- **Default profile = `local`** so the operator-default honors the local-LLM-first goal without an explicit `--profile local`. Currently default is `default` which maps via project config.\n- **Inject `routing.Inputs.HistoricalSuccess` per-harness** to replace `escalation.AdaptiveMinTier` with agent-native success-rate routing. Frees DDx from maintaining its own success-window state machine.\n- **Delete `cli/internal/agent/profile_ladder.go` helpers** once no caller (CLI or server) consumes them. Currently consumed only via `--escalate`; if `--escalate` is dropped or moved into agent, these can go.\n- **Delete `cli/internal/escalation/escalation.go::AdaptiveMinTier`** once the HistoricalSuccess injection above is in place.\n\n## Out of scope for this epic\n\n- Changes to agent SDK source — if a gap is found, file an upstream bead under `~/Projects/agent/.ddx/`.\n- UI/e2e work — the parent e2e drift epic `ddx-ccdf9cf9` covers that and is independent.\n- Server source changes outside `cli/internal/server/workers.go` — no API endpoint changes, no GraphQL schema changes.",
+    "acceptance": "Sole remaining child ddx-c7081f89 closes successfully. Then verify:\n\n1. cd cli \u0026\u0026 go test ./internal/server/... ./internal/agent/... passes\n2. cd /home/erik/Projects/ddx \u0026\u0026 /home/erik/.local/bin/ddx work --once --local (no flags, no pin) successfully dispatches a single-call attempt against the next ready bead; the bead's events show no kind:tier-attempt entries (default path uses single ResolveRoute call)\n3. cd /home/erik/Projects/ddx \u0026\u0026 /home/erik/.local/bin/ddx work --once --local --escalate preserves the existing tier-loop behavior (kind:tier-attempt entries appear)\n4. The bead-event taxonomy preserved on a default-path run: kind:routing fires once, kind:cost fires once, kind:execute-bead fires once for the outcome; kind:tier-attempt and kind:escalation-summary do NOT fire on the default path (they remain reserved for --escalate)",
+    "labels": [
+      "area:routing",
+      "area:agent",
+      "kind:refactor",
+      "phase:build"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T17:00:34Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T17:00:34.493590259Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T170036-22cf74ed",
+    "prompt": ".ddx/executions/20260429T170036-22cf74ed/prompt.md",
+    "manifest": ".ddx/executions/20260429T170036-22cf74ed/manifest.json",
+    "result": ".ddx/executions/20260429T170036-22cf74ed/result.json",
+    "checks": ".ddx/executions/20260429T170036-22cf74ed/checks.json",
+    "usage": ".ddx/executions/20260429T170036-22cf74ed/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-3f406266-20260429T170036-22cf74ed"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T170036-22cf74ed/result.json b/.ddx/executions/20260429T170036-22cf74ed/result.json
new file mode 100644
index 00000000..7ccf83cf
--- /dev/null
+++ b/.ddx/executions/20260429T170036-22cf74ed/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-3f406266",
+  "attempt_id": "20260429T170036-22cf74ed",
+  "base_rev": "f3240e8fabfb5fbe2ffaeb25c0520c345d0dca3d",
+  "result_rev": "13a3e7567df81f21ac42de85f9725e1726f70887",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-725db1ea",
+  "duration_ms": 681093,
+  "tokens": 18967,
+  "cost_usd": 1.1059522000000002,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T170036-22cf74ed",
+  "prompt_file": ".ddx/executions/20260429T170036-22cf74ed/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T170036-22cf74ed/manifest.json",
+  "result_file": ".ddx/executions/20260429T170036-22cf74ed/result.json",
+  "usage_file": ".ddx/executions/20260429T170036-22cf74ed/usage.json",
+  "started_at": "2026-04-29T17:00:37.254129774Z",
+  "finished_at": "2026-04-29T17:11:58.347574538Z"
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
