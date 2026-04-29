<bead-review>
  <bead id="ddx-3bd7396a" iter=1>
    <title>delete deprecated routing config + flags + warnings with prejudice (profile_ladders, model_overrides, --escalate, --override-model, AdaptiveMinTier)</title>
    <description>
## Goal

Delete `agent.routing.profile_ladders`, `agent.routing.model_overrides`, the `--escalate` and `--override-model` flags, all their consumers, and the deprecation warnings. The duplicated routing layer is dead — the corpse should not haunt every config load.

The migration `ddx-87fb72c2` made these opt-in. This bead completes the migration: opt-in becomes opt-out becomes gone.

## Scope (delete with prejudice)

### Config schema
- `cli/internal/config/types.go` — delete `RoutingConfig.ProfileLadders` and `RoutingConfig.ModelOverrides` fields (and `defaultProfileLadders`, `DefaultProfileLadders()`).
- `cli/internal/config/schema/config.schema.json` — delete the `profile_ladders` and `model_overrides` properties.
- `cli/internal/config/config.go` — delete the deprecation warning code paths that detect these fields and print the migration hint. Hard reject any project config that still sets them: clear error pointing at the (then-archived) migration doc, exit non-zero. Same blast radius as `default_harness` already gets.
- `cli/internal/config/routing_migration_test.go` — delete or repurpose to test the new hard-reject behavior.

### CLI flags
- `cli/cmd/agent_cmd.go` — delete `--escalate` and `--override-model` flag definitions and their plumbing into `WorkerSpec` / RouteRequest. The `WorkerSpec.Escalate` field added by `ddx-c7081f89` becomes dead and goes too.
- `cli/cmd/agent_execute_loop_*.go` — same.
- Any flag wiring in the worker submit path (`cli/internal/server/server.go`, `cli/internal/server/workers.go`).

### Helpers and consumers
- `cli/internal/agent/profile_ladder.go` — delete entirely (`ResolveProfileLadder`, `ResolveTierModelRef`, `NormalizeRoutingProfile`'s ladder semantics, the test seam call counters, `DefaultRoutingProfile`).
- `cli/internal/escalation/escalation.go` — delete `AdaptiveMinTier`, `AdaptiveMinTierResult`, `AdaptiveMinTierThreshold`, `AdaptiveMinTierMinSamples`, `TierResolver`, `classifyAttemptTier`, `TierOrder`, `TiersInRange`, `tierIndex`. Keep `EscalationSummary`, `TierAttemptRecord`, `BuildEscalationSummary`, `AppendEscalationSummaryEvent`, `EscalationWinningExhausted`, `EscalatableStatuses`, `ShouldEscalate`, `IsInfrastructureFailure`, `ProviderCooldownDuration`, `FormatTierAttemptBody`, `BeadEventAppender`, `SuccessStatus` — these are still used by other paths (loop status, bead events, cooldown).
- `cli/internal/server/workers.go:658-820` — delete the entire escalation-on branch (the `if !escalationEnabled { ... } else { ...tier loop... }` split). `escalationEnabled` always false; collapse to the single-call path.
- `cli/cmd/agent_route_status.go` — drop the `--adaptive` flag and `reset` subcommand if they target the now-deleted state. Keep the rest of route-status.
- `docs/adaptive-min-tier.md` — delete.

### Documentation
- `docs/migrations/routing-config.md` — delete (or replace with a one-line stub: "agent.routing.profile_ladders and agent.routing.model_overrides were removed in vX.Y.Z; configs that still set them are rejected at load.").
- `cli/internal/server/frontend/README.md`, `CHANGELOG.md` — search and remove any user-facing references to `--escalate` or `--override-model`.

### Tests
- Delete tests that exclusively cover the deleted paths: `*_escalation_*_test.go`, `profile_ladder_test.go`, `routing_migration_test.go`, etc. Where a test covers BOTH a deleted path and a kept path, narrow it to the kept path.
- Add one test that asserts `ddx work --once --local` on a project that still has `routing.profile_ladders` in its config exits non-zero with an error message naming the field.

## Affirmative non-goals

- Do **not** preserve any "transition" behavior. There is no warning-then-error grace period; `ddx-87fb72c2` already shipped the warning. This bead is the deletion.
- Do **not** keep dead `Escalate` / `MinTier` / `MaxTier` / `ModelRef` fields on `WorkerSpec` "for future use". If something needs them later, file a fresh bead.
- Do **not** keep the helpers in case some out-of-tree consumer imports them. DDx is a binary, not a library.
- Do **not** touch agent SDK internals — `ProfileEscalationLadder` and `escalateProfileLadder` live agent-side and stay.

## Out of scope

- Default profile change (separate bead if pursued).
- HistoricalSuccess injection into `RouteRequest.Inputs` (separate bead).
- Bead-id resolver bug (`ddx-7eab13a6`) — already filed.
- The 12 malformed-ID beads — `ddx-7eab13a6` covers their repair.

## Acceptance

1. `cd cli &amp;&amp; go test ./...` passes (full tree, not just routing packages — the kept escalation helpers may have other callers).
2. `grep -rnE 'profile_ladders|model_overrides|ProfileLadders|ModelOverrides|ResolveProfileLadder|ResolveTierModelRef|AdaptiveMinTier|--escalate|--override-model' cli/ docs/ --include='*.go' --include='*.md' --include='*.yaml' --include='*.json'` returns matches only in:
   - this bead's own description (if grepped from .ddx/beads.jsonl)
   - the deletion test fixture / commit message
   - upstream-vendored code under cli/internal/server/frontend/node_modules/ if any
   No matches in source files.
3. `cd /home/erik/Projects/ddx &amp;&amp; /home/erik/.local/bin/ddx work --once --local` against the current project (with `agent.routing` already removed) runs cleanly, no warnings, no flags needed.
4. A test fixture project with `agent.routing.profile_ladders: ...` in its `.ddx/config.yaml` causes any `ddx ...` command that loads config to exit non-zero with a message identifying the field. Same for `agent.routing.model_overrides`.
5. `--escalate` and `--override-model` flags are gone from `--help` output of `ddx work`, `ddx agent run`, `ddx agent execute-bead`, `ddx agent execute-loop`. Verifiable: `/home/erik/.local/bin/ddx work --help 2&gt;&amp;1 | grep -E 'escalate|override-model'` returns nothing.
6. `docs/adaptive-min-tier.md` and `docs/migrations/routing-config.md` are either deleted or replaced with single-line stubs noting the removal.

## Rollback

If the deletion proves too aggressive, revert the deletion commits. The migration doc (`docs/migrations/routing-config.md`) at HEAD before deletion has the full feature description; the git history retains the helpers.
    </description>
    <acceptance>
1. cd cli &amp;&amp; go test ./... passes
2. grep -rnE 'profile_ladders|model_overrides|ProfileLadders|ModelOverrides|ResolveProfileLadder|ResolveTierModelRef|AdaptiveMinTier|--escalate|--override-model' cli/ docs/ --include='*.go' --include='*.md' --include='*.yaml' --include='*.json' returns no matches in DDx source files (vendored node_modules and this bead's own description excluded)
3. cd /home/erik/Projects/ddx &amp;&amp; /home/erik/.local/bin/ddx work --once --local runs cleanly with no deprecation warnings and no flags needed
4. A fixture project carrying agent.routing.profile_ladders OR agent.routing.model_overrides in its .ddx/config.yaml causes any ddx command that loads config to exit non-zero with a message naming the offending field
5. /home/erik/.local/bin/ddx work --help; /home/erik/.local/bin/ddx agent run --help; /home/erik/.local/bin/ddx agent execute-bead --help — none of these show --escalate or --override-model
6. docs/adaptive-min-tier.md and docs/migrations/routing-config.md are either deleted or replaced with single-line removal stubs
    </acceptance>
    <labels>area:routing, area:agent, area:config, kind:cleanup, kind:deletion</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T220335-335a5db3/manifest.json</file>
    <file>.ddx/executions/20260429T220335-335a5db3/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="6125fa05cd2adf6a99a0a9338024ebe4fbc3be4d">
diff --git a/.ddx/executions/20260429T220335-335a5db3/manifest.json b/.ddx/executions/20260429T220335-335a5db3/manifest.json
new file mode 100644
index 00000000..809dd4fd
--- /dev/null
+++ b/.ddx/executions/20260429T220335-335a5db3/manifest.json
@@ -0,0 +1,39 @@
+{
+  "attempt_id": "20260429T220335-335a5db3",
+  "bead_id": "ddx-3bd7396a",
+  "base_rev": "858ff8c218b265a1d920a38b6c0659c9985ca9b6",
+  "created_at": "2026-04-29T22:03:36.532941927Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-3bd7396a",
+    "title": "delete deprecated routing config + flags + warnings with prejudice (profile_ladders, model_overrides, --escalate, --override-model, AdaptiveMinTier)",
+    "description": "## Goal\n\nDelete `agent.routing.profile_ladders`, `agent.routing.model_overrides`, the `--escalate` and `--override-model` flags, all their consumers, and the deprecation warnings. The duplicated routing layer is dead — the corpse should not haunt every config load.\n\nThe migration `ddx-87fb72c2` made these opt-in. This bead completes the migration: opt-in becomes opt-out becomes gone.\n\n## Scope (delete with prejudice)\n\n### Config schema\n- `cli/internal/config/types.go` — delete `RoutingConfig.ProfileLadders` and `RoutingConfig.ModelOverrides` fields (and `defaultProfileLadders`, `DefaultProfileLadders()`).\n- `cli/internal/config/schema/config.schema.json` — delete the `profile_ladders` and `model_overrides` properties.\n- `cli/internal/config/config.go` — delete the deprecation warning code paths that detect these fields and print the migration hint. Hard reject any project config that still sets them: clear error pointing at the (then-archived) migration doc, exit non-zero. Same blast radius as `default_harness` already gets.\n- `cli/internal/config/routing_migration_test.go` — delete or repurpose to test the new hard-reject behavior.\n\n### CLI flags\n- `cli/cmd/agent_cmd.go` — delete `--escalate` and `--override-model` flag definitions and their plumbing into `WorkerSpec` / RouteRequest. The `WorkerSpec.Escalate` field added by `ddx-c7081f89` becomes dead and goes too.\n- `cli/cmd/agent_execute_loop_*.go` — same.\n- Any flag wiring in the worker submit path (`cli/internal/server/server.go`, `cli/internal/server/workers.go`).\n\n### Helpers and consumers\n- `cli/internal/agent/profile_ladder.go` — delete entirely (`ResolveProfileLadder`, `ResolveTierModelRef`, `NormalizeRoutingProfile`'s ladder semantics, the test seam call counters, `DefaultRoutingProfile`).\n- `cli/internal/escalation/escalation.go` — delete `AdaptiveMinTier`, `AdaptiveMinTierResult`, `AdaptiveMinTierThreshold`, `AdaptiveMinTierMinSamples`, `TierResolver`, `classifyAttemptTier`, `TierOrder`, `TiersInRange`, `tierIndex`. Keep `EscalationSummary`, `TierAttemptRecord`, `BuildEscalationSummary`, `AppendEscalationSummaryEvent`, `EscalationWinningExhausted`, `EscalatableStatuses`, `ShouldEscalate`, `IsInfrastructureFailure`, `ProviderCooldownDuration`, `FormatTierAttemptBody`, `BeadEventAppender`, `SuccessStatus` — these are still used by other paths (loop status, bead events, cooldown).\n- `cli/internal/server/workers.go:658-820` — delete the entire escalation-on branch (the `if !escalationEnabled { ... } else { ...tier loop... }` split). `escalationEnabled` always false; collapse to the single-call path.\n- `cli/cmd/agent_route_status.go` — drop the `--adaptive` flag and `reset` subcommand if they target the now-deleted state. Keep the rest of route-status.\n- `docs/adaptive-min-tier.md` — delete.\n\n### Documentation\n- `docs/migrations/routing-config.md` — delete (or replace with a one-line stub: \"agent.routing.profile_ladders and agent.routing.model_overrides were removed in vX.Y.Z; configs that still set them are rejected at load.\").\n- `cli/internal/server/frontend/README.md`, `CHANGELOG.md` — search and remove any user-facing references to `--escalate` or `--override-model`.\n\n### Tests\n- Delete tests that exclusively cover the deleted paths: `*_escalation_*_test.go`, `profile_ladder_test.go`, `routing_migration_test.go`, etc. Where a test covers BOTH a deleted path and a kept path, narrow it to the kept path.\n- Add one test that asserts `ddx work --once --local` on a project that still has `routing.profile_ladders` in its config exits non-zero with an error message naming the field.\n\n## Affirmative non-goals\n\n- Do **not** preserve any \"transition\" behavior. There is no warning-then-error grace period; `ddx-87fb72c2` already shipped the warning. This bead is the deletion.\n- Do **not** keep dead `Escalate` / `MinTier` / `MaxTier` / `ModelRef` fields on `WorkerSpec` \"for future use\". If something needs them later, file a fresh bead.\n- Do **not** keep the helpers in case some out-of-tree consumer imports them. DDx is a binary, not a library.\n- Do **not** touch agent SDK internals — `ProfileEscalationLadder` and `escalateProfileLadder` live agent-side and stay.\n\n## Out of scope\n\n- Default profile change (separate bead if pursued).\n- HistoricalSuccess injection into `RouteRequest.Inputs` (separate bead).\n- Bead-id resolver bug (`ddx-7eab13a6`) — already filed.\n- The 12 malformed-ID beads — `ddx-7eab13a6` covers their repair.\n\n## Acceptance\n\n1. `cd cli \u0026\u0026 go test ./...` passes (full tree, not just routing packages — the kept escalation helpers may have other callers).\n2. `grep -rnE 'profile_ladders|model_overrides|ProfileLadders|ModelOverrides|ResolveProfileLadder|ResolveTierModelRef|AdaptiveMinTier|--escalate|--override-model' cli/ docs/ --include='*.go' --include='*.md' --include='*.yaml' --include='*.json'` returns matches only in:\n   - this bead's own description (if grepped from .ddx/beads.jsonl)\n   - the deletion test fixture / commit message\n   - upstream-vendored code under cli/internal/server/frontend/node_modules/ if any\n   No matches in source files.\n3. `cd /home/erik/Projects/ddx \u0026\u0026 /home/erik/.local/bin/ddx work --once --local` against the current project (with `agent.routing` already removed) runs cleanly, no warnings, no flags needed.\n4. A test fixture project with `agent.routing.profile_ladders: ...` in its `.ddx/config.yaml` causes any `ddx ...` command that loads config to exit non-zero with a message identifying the field. Same for `agent.routing.model_overrides`.\n5. `--escalate` and `--override-model` flags are gone from `--help` output of `ddx work`, `ddx agent run`, `ddx agent execute-bead`, `ddx agent execute-loop`. Verifiable: `/home/erik/.local/bin/ddx work --help 2\u003e\u00261 | grep -E 'escalate|override-model'` returns nothing.\n6. `docs/adaptive-min-tier.md` and `docs/migrations/routing-config.md` are either deleted or replaced with single-line stubs noting the removal.\n\n## Rollback\n\nIf the deletion proves too aggressive, revert the deletion commits. The migration doc (`docs/migrations/routing-config.md`) at HEAD before deletion has the full feature description; the git history retains the helpers.",
+    "acceptance": "1. cd cli \u0026\u0026 go test ./... passes\n2. grep -rnE 'profile_ladders|model_overrides|ProfileLadders|ModelOverrides|ResolveProfileLadder|ResolveTierModelRef|AdaptiveMinTier|--escalate|--override-model' cli/ docs/ --include='*.go' --include='*.md' --include='*.yaml' --include='*.json' returns no matches in DDx source files (vendored node_modules and this bead's own description excluded)\n3. cd /home/erik/Projects/ddx \u0026\u0026 /home/erik/.local/bin/ddx work --once --local runs cleanly with no deprecation warnings and no flags needed\n4. A fixture project carrying agent.routing.profile_ladders OR agent.routing.model_overrides in its .ddx/config.yaml causes any ddx command that loads config to exit non-zero with a message naming the offending field\n5. /home/erik/.local/bin/ddx work --help; /home/erik/.local/bin/ddx agent run --help; /home/erik/.local/bin/ddx agent execute-bead --help — none of these show --escalate or --override-model\n6. docs/adaptive-min-tier.md and docs/migrations/routing-config.md are either deleted or replaced with single-line removal stubs",
+    "labels": [
+      "area:routing",
+      "area:agent",
+      "area:config",
+      "kind:cleanup",
+      "kind:deletion"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T22:03:35Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T22:03:35.531536068Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T220335-335a5db3",
+    "prompt": ".ddx/executions/20260429T220335-335a5db3/prompt.md",
+    "manifest": ".ddx/executions/20260429T220335-335a5db3/manifest.json",
+    "result": ".ddx/executions/20260429T220335-335a5db3/result.json",
+    "checks": ".ddx/executions/20260429T220335-335a5db3/checks.json",
+    "usage": ".ddx/executions/20260429T220335-335a5db3/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-3bd7396a-20260429T220335-335a5db3"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T220335-335a5db3/result.json b/.ddx/executions/20260429T220335-335a5db3/result.json
new file mode 100644
index 00000000..81b2a48d
--- /dev/null
+++ b/.ddx/executions/20260429T220335-335a5db3/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-3bd7396a",
+  "attempt_id": "20260429T220335-335a5db3",
+  "base_rev": "858ff8c218b265a1d920a38b6c0659c9985ca9b6",
+  "result_rev": "9d988853a04a067baa6150c03513ff0ba7e0d63a",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-b4716fb6",
+  "duration_ms": 3085489,
+  "tokens": 109968,
+  "cost_usd": 9.967167550000008,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T220335-335a5db3",
+  "prompt_file": ".ddx/executions/20260429T220335-335a5db3/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T220335-335a5db3/manifest.json",
+  "result_file": ".ddx/executions/20260429T220335-335a5db3/result.json",
+  "usage_file": ".ddx/executions/20260429T220335-335a5db3/usage.json",
+  "started_at": "2026-04-29T22:03:36.533407385Z",
+  "finished_at": "2026-04-29T22:55:02.023370423Z"
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
