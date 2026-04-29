<bead-review>
  <bead id="ddx-c7081f89" iter=1>
    <title>routing-unify-1: align workers.go default escalation semantics with the --escalate opt-in CLI path</title>
    <description>
## Goal

Align `cli/internal/server/workers.go`'s default-path escalation semantics with the migration that already shipped on the CLI path via bead `ddx-87fb72c2` (epic `ddx-fdd3ea36`, see `docs/migrations/routing-config.md`). Today the two paths disagree:

- **CLI path** `cli/cmd/agent_cmd.go:1792`: `escalationEnabled := escalate &amp;&amp; harness == "" &amp;&amp; model == ""` — escalation is opt-in via the `--escalate` flag.
- **Server path** `cli/internal/server/workers.go:650`: `escalationEnabled := spec.Harness == "" &amp;&amp; spec.Model == ""` — escalation is implicitly on whenever neither harness nor model is pinned. No `--escalate` equivalent plumbed through the worker spec.

Because `ddx work` submits to the server worker by default (not `--local`), the user-facing surface still runs the tier loop + `ResolveTierCandidates` cooldown probe + `escalation.AdaptiveMinTier` filter on every dispatch — even though the migration declared the tier ladder opt-in. This is the proximate cause of the "all tiers exhausted — no viable provider" failure observed on `worker-20260429T033325-bedf` against this project today, and was confirmed by a control probe in this session: `ddx work --once --local` against `ddx-fdd3ea36` (no flags, no pin) reproduces "all tiers exhausted" while `ddx work --once --local --no-adaptive-min-tier` succeeds end-to-end on the cheap tier (qwen/qwen3.6-35b-a3b @ lmstudio-bragi, zero cost).

## Concrete change

Plumb an explicit escalation-opt-in flag through the worker spec, matching the CLI path's `--escalate` semantics:

1. Add a boolean field to `WorkerSpec` (in `cli/internal/server/workers.go` or wherever `WorkerSpec` is defined) named e.g. `Escalate`. Default `false`.
2. The `--escalate` flag on `ddx work` (already exists for the CLI path; verify it's plumbed to the worker submission, add the wiring if not) sets `WorkerSpec.Escalate = true`.
3. Change `cli/internal/server/workers.go:650` from:

       escalationEnabled := spec.Harness == "" &amp;&amp; spec.Model == ""

   to:

       escalationEnabled := spec.Escalate &amp;&amp; spec.Harness == "" &amp;&amp; spec.Model == ""

4. Update `cli/internal/server/workers_test.go` to assert: with `spec.Escalate=false` (default), `executor` follows the `singleTierAttempt` path at line 692-702 — no `ResolveProfileLadder` call, no tier loop, no `AdaptiveMinTier` evaluation. With `spec.Escalate=true`, the existing tier-loop behavior is preserved.

That is all. Do not delete any helper functions in this bead — `ResolveProfileLadder`, `ResolveTierModelRef`, `AdaptiveMinTier` all remain (they're still consumed via the `--escalate` opt-in path). Their deletion is reserved for a separate cleanup bead after this lands and there's evidence no caller uses them.

## In-scope files

- `cli/internal/server/workers.go` — line ~650, plus any `WorkerSpec` struct definition (could be in this file or `cli/internal/server/types.go` / `models.go` — locate via `grep -nE "type WorkerSpec\b" cli/`).
- `cli/internal/server/workers_test.go` — add the two assertions above.
- The `ddx work --escalate` flag wiring on the worker-submit path. If `--escalate` is currently CLI-only (i.e. only consulted in the `--local` branch), this bead also plumbs it through `cli/cmd/agent_cmd.go`'s server-submit code path. Locate via `grep -nE "GetBool\\(\"escalate\"\\)|--escalate" cli/cmd/`.
- Possibly `cli/internal/server/server.go` if `WorkerSpec` is constructed there from an HTTP/MCP request body.

## Out of scope (do NOT touch)

- `cli/internal/agent/profile_ladder.go` — keep all helpers; deletion is a separate bead.
- `cli/internal/escalation/escalation.go::AdaptiveMinTier` — keep; deletion is a separate bead.
- Default profile change from `default` to `local` — separate bead (`routing-unify-2`).
- `cli/cmd/agent_cmd.go:1792` — already correct; no change.
- `docs/migrations/routing-config.md` — already correct; describes the migration this bead completes for the server path.
- Any work on `cli/internal/agent` or `agent` SDK — agent already exposes everything DDx needs.

## Acceptance

1. `cd cli &amp;&amp; go test ./internal/server/... ./internal/agent/...` passes.
2. `cd /home/erik/Projects/ddx &amp;&amp; /home/erik/.local/bin/ddx work --once --local` (no flags, no pin) against this project resolves and dispatches a single-call attempt. Verifiable by:
   - The bead receives at most one `kind:routing` event from the new flow (matches the `singleTierAttempt` pattern), not multiple `kind:tier-attempt skipped` events from the tier loop.
   - The execute-loop log line `adaptive min-tier: skipping cheap tier ...` does NOT appear.
3. `cd /home/erik/Projects/ddx &amp;&amp; /home/erik/.local/bin/ddx work --once --local --escalate` against this project preserves the existing tier-loop behavior. Verifiable: at least one `kind:tier-attempt` event appears, the `adaptive min-tier` line still prints when applicable.
4. Unit test in `cli/internal/server/workers_test.go` asserts: with `spec.Escalate=false`, `agent.ResolveProfileLadder` is not called (use the existing call-counter test seam in `cli/internal/agent/profile_ladder.go::ResolveProfileLadderCallCount`); with `spec.Escalate=true`, the call counter advances.
5. Operator policy preserved: `ddx work --harness X` and `ddx work --model Y` continue to work and bypass escalation regardless of `--escalate` (the existing `&amp;&amp; spec.Harness == "" &amp;&amp; spec.Model == ""` predicate handles this).
6. The migration doc `docs/migrations/routing-config.md` is updated to mention that the worker path now matches the CLI path semantics, OR a one-line cross-reference is added if its current text is sufficient.

## Why this is a tight bead

It is a single 1-line behavioral change in `workers.go:650` plus the worker-spec field plumbing. The supervisor checkpoint mandate is "one bead/fix at a time"; this is the smallest meaningful step toward unification given the current tree state. Do not bundle helper deletions, default-profile changes, or schema cleanups — those are reserved for follow-up beads.

## Rollback

If the change breaks anything: revert the workers.go diff. The pre-change behavior was the prior implicit-on default; reverting restores it. Worker-spec field can stay (additive, default false). Migration doc edit is independent.
    </description>
    <acceptance>
1. cd cli &amp;&amp; go test ./internal/server/... ./internal/agent/... passes
2. cd /home/erik/Projects/ddx &amp;&amp; /home/erik/.local/bin/ddx work --once --local (no flags, no pin) successfully dispatches a single-call attempt; the dispatched bead's events show at most one kind:routing event, no kind:tier-attempt events, and no "adaptive min-tier: skipping cheap tier" log line on stderr
3. cd /home/erik/Projects/ddx &amp;&amp; /home/erik/.local/bin/ddx work --once --local --escalate preserves the existing tier-loop behavior: at least one kind:tier-attempt event is emitted, ResolveProfileLadder is called (verifiable via agent.ResolveProfileLadderCallCount() in tests)
4. Unit test in cli/internal/server/workers_test.go asserts: with spec.Escalate=false, agent.ResolveProfileLadderCallCount() does not advance during executor invocation; with spec.Escalate=true, it advances
5. Operator pin preserved: cd cli &amp;&amp; go test ./internal/server -run "TestWorker.*Harness|TestWorker.*Model" passes; --harness and --model flags continue to bypass escalation
6. docs/migrations/routing-config.md cross-references that the server worker path now matches the CLI path semantics (one-line addition or a brief paragraph)
    </acceptance>
    <labels>area:routing, area:server, kind:refactor, phase:build</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T164539-86e4f11a/manifest.json</file>
    <file>.ddx/executions/20260429T164539-86e4f11a/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="39fcbbc94c2013521a2e7d62592977c83489f6ff">
diff --git a/.ddx/executions/20260429T164539-86e4f11a/manifest.json b/.ddx/executions/20260429T164539-86e4f11a/manifest.json
new file mode 100644
index 00000000..82ab25a2
--- /dev/null
+++ b/.ddx/executions/20260429T164539-86e4f11a/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260429T164539-86e4f11a",
+  "bead_id": "ddx-c7081f89",
+  "base_rev": "cee9d7a58c7126615771e89fc29660dfd1013bf6",
+  "created_at": "2026-04-29T16:45:39.839512102Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-c7081f89",
+    "title": "routing-unify-1: align workers.go default escalation semantics with the --escalate opt-in CLI path",
+    "description": "## Goal\n\nAlign `cli/internal/server/workers.go`'s default-path escalation semantics with the migration that already shipped on the CLI path via bead `ddx-87fb72c2` (epic `ddx-fdd3ea36`, see `docs/migrations/routing-config.md`). Today the two paths disagree:\n\n- **CLI path** `cli/cmd/agent_cmd.go:1792`: `escalationEnabled := escalate \u0026\u0026 harness == \"\" \u0026\u0026 model == \"\"` — escalation is opt-in via the `--escalate` flag.\n- **Server path** `cli/internal/server/workers.go:650`: `escalationEnabled := spec.Harness == \"\" \u0026\u0026 spec.Model == \"\"` — escalation is implicitly on whenever neither harness nor model is pinned. No `--escalate` equivalent plumbed through the worker spec.\n\nBecause `ddx work` submits to the server worker by default (not `--local`), the user-facing surface still runs the tier loop + `ResolveTierCandidates` cooldown probe + `escalation.AdaptiveMinTier` filter on every dispatch — even though the migration declared the tier ladder opt-in. This is the proximate cause of the \"all tiers exhausted — no viable provider\" failure observed on `worker-20260429T033325-bedf` against this project today, and was confirmed by a control probe in this session: `ddx work --once --local` against `ddx-fdd3ea36` (no flags, no pin) reproduces \"all tiers exhausted\" while `ddx work --once --local --no-adaptive-min-tier` succeeds end-to-end on the cheap tier (qwen/qwen3.6-35b-a3b @ lmstudio-bragi, zero cost).\n\n## Concrete change\n\nPlumb an explicit escalation-opt-in flag through the worker spec, matching the CLI path's `--escalate` semantics:\n\n1. Add a boolean field to `WorkerSpec` (in `cli/internal/server/workers.go` or wherever `WorkerSpec` is defined) named e.g. `Escalate`. Default `false`.\n2. The `--escalate` flag on `ddx work` (already exists for the CLI path; verify it's plumbed to the worker submission, add the wiring if not) sets `WorkerSpec.Escalate = true`.\n3. Change `cli/internal/server/workers.go:650` from:\n\n       escalationEnabled := spec.Harness == \"\" \u0026\u0026 spec.Model == \"\"\n\n   to:\n\n       escalationEnabled := spec.Escalate \u0026\u0026 spec.Harness == \"\" \u0026\u0026 spec.Model == \"\"\n\n4. Update `cli/internal/server/workers_test.go` to assert: with `spec.Escalate=false` (default), `executor` follows the `singleTierAttempt` path at line 692-702 — no `ResolveProfileLadder` call, no tier loop, no `AdaptiveMinTier` evaluation. With `spec.Escalate=true`, the existing tier-loop behavior is preserved.\n\nThat is all. Do not delete any helper functions in this bead — `ResolveProfileLadder`, `ResolveTierModelRef`, `AdaptiveMinTier` all remain (they're still consumed via the `--escalate` opt-in path). Their deletion is reserved for a separate cleanup bead after this lands and there's evidence no caller uses them.\n\n## In-scope files\n\n- `cli/internal/server/workers.go` — line ~650, plus any `WorkerSpec` struct definition (could be in this file or `cli/internal/server/types.go` / `models.go` — locate via `grep -nE \"type WorkerSpec\\b\" cli/`).\n- `cli/internal/server/workers_test.go` — add the two assertions above.\n- The `ddx work --escalate` flag wiring on the worker-submit path. If `--escalate` is currently CLI-only (i.e. only consulted in the `--local` branch), this bead also plumbs it through `cli/cmd/agent_cmd.go`'s server-submit code path. Locate via `grep -nE \"GetBool\\\\(\\\"escalate\\\"\\\\)|--escalate\" cli/cmd/`.\n- Possibly `cli/internal/server/server.go` if `WorkerSpec` is constructed there from an HTTP/MCP request body.\n\n## Out of scope (do NOT touch)\n\n- `cli/internal/agent/profile_ladder.go` — keep all helpers; deletion is a separate bead.\n- `cli/internal/escalation/escalation.go::AdaptiveMinTier` — keep; deletion is a separate bead.\n- Default profile change from `default` to `local` — separate bead (`routing-unify-2`).\n- `cli/cmd/agent_cmd.go:1792` — already correct; no change.\n- `docs/migrations/routing-config.md` — already correct; describes the migration this bead completes for the server path.\n- Any work on `cli/internal/agent` or `agent` SDK — agent already exposes everything DDx needs.\n\n## Acceptance\n\n1. `cd cli \u0026\u0026 go test ./internal/server/... ./internal/agent/...` passes.\n2. `cd /home/erik/Projects/ddx \u0026\u0026 /home/erik/.local/bin/ddx work --once --local` (no flags, no pin) against this project resolves and dispatches a single-call attempt. Verifiable by:\n   - The bead receives at most one `kind:routing` event from the new flow (matches the `singleTierAttempt` pattern), not multiple `kind:tier-attempt skipped` events from the tier loop.\n   - The execute-loop log line `adaptive min-tier: skipping cheap tier ...` does NOT appear.\n3. `cd /home/erik/Projects/ddx \u0026\u0026 /home/erik/.local/bin/ddx work --once --local --escalate` against this project preserves the existing tier-loop behavior. Verifiable: at least one `kind:tier-attempt` event appears, the `adaptive min-tier` line still prints when applicable.\n4. Unit test in `cli/internal/server/workers_test.go` asserts: with `spec.Escalate=false`, `agent.ResolveProfileLadder` is not called (use the existing call-counter test seam in `cli/internal/agent/profile_ladder.go::ResolveProfileLadderCallCount`); with `spec.Escalate=true`, the call counter advances.\n5. Operator policy preserved: `ddx work --harness X` and `ddx work --model Y` continue to work and bypass escalation regardless of `--escalate` (the existing `\u0026\u0026 spec.Harness == \"\" \u0026\u0026 spec.Model == \"\"` predicate handles this).\n6. The migration doc `docs/migrations/routing-config.md` is updated to mention that the worker path now matches the CLI path semantics, OR a one-line cross-reference is added if its current text is sufficient.\n\n## Why this is a tight bead\n\nIt is a single 1-line behavioral change in `workers.go:650` plus the worker-spec field plumbing. The supervisor checkpoint mandate is \"one bead/fix at a time\"; this is the smallest meaningful step toward unification given the current tree state. Do not bundle helper deletions, default-profile changes, or schema cleanups — those are reserved for follow-up beads.\n\n## Rollback\n\nIf the change breaks anything: revert the workers.go diff. The pre-change behavior was the prior implicit-on default; reverting restores it. Worker-spec field can stay (additive, default false). Migration doc edit is independent.",
+    "acceptance": "1. cd cli \u0026\u0026 go test ./internal/server/... ./internal/agent/... passes\n2. cd /home/erik/Projects/ddx \u0026\u0026 /home/erik/.local/bin/ddx work --once --local (no flags, no pin) successfully dispatches a single-call attempt; the dispatched bead's events show at most one kind:routing event, no kind:tier-attempt events, and no \"adaptive min-tier: skipping cheap tier\" log line on stderr\n3. cd /home/erik/Projects/ddx \u0026\u0026 /home/erik/.local/bin/ddx work --once --local --escalate preserves the existing tier-loop behavior: at least one kind:tier-attempt event is emitted, ResolveProfileLadder is called (verifiable via agent.ResolveProfileLadderCallCount() in tests)\n4. Unit test in cli/internal/server/workers_test.go asserts: with spec.Escalate=false, agent.ResolveProfileLadderCallCount() does not advance during executor invocation; with spec.Escalate=true, it advances\n5. Operator pin preserved: cd cli \u0026\u0026 go test ./internal/server -run \"TestWorker.*Harness|TestWorker.*Model\" passes; --harness and --model flags continue to bypass escalation\n6. docs/migrations/routing-config.md cross-references that the server worker path now matches the CLI path semantics (one-line addition or a brief paragraph)",
+    "labels": [
+      "area:routing",
+      "area:server",
+      "kind:refactor",
+      "phase:build"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T16:45:37Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T16:45:37.052993926Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T164539-86e4f11a",
+    "prompt": ".ddx/executions/20260429T164539-86e4f11a/prompt.md",
+    "manifest": ".ddx/executions/20260429T164539-86e4f11a/manifest.json",
+    "result": ".ddx/executions/20260429T164539-86e4f11a/result.json",
+    "checks": ".ddx/executions/20260429T164539-86e4f11a/checks.json",
+    "usage": ".ddx/executions/20260429T164539-86e4f11a/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-c7081f89-20260429T164539-86e4f11a"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T164539-86e4f11a/result.json b/.ddx/executions/20260429T164539-86e4f11a/result.json
new file mode 100644
index 00000000..59961317
--- /dev/null
+++ b/.ddx/executions/20260429T164539-86e4f11a/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-c7081f89",
+  "attempt_id": "20260429T164539-86e4f11a",
+  "base_rev": "cee9d7a58c7126615771e89fc29660dfd1013bf6",
+  "result_rev": "c8a0515bf508c5b84116dc0440ca328098268077",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-9a78acad",
+  "duration_ms": 870356,
+  "tokens": 22961,
+  "cost_usd": 1.4463004499999996,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T164539-86e4f11a",
+  "prompt_file": ".ddx/executions/20260429T164539-86e4f11a/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T164539-86e4f11a/manifest.json",
+  "result_file": ".ddx/executions/20260429T164539-86e4f11a/result.json",
+  "usage_file": ".ddx/executions/20260429T164539-86e4f11a/usage.json",
+  "started_at": "2026-04-29T16:45:39.840007393Z",
+  "finished_at": "2026-04-29T17:00:10.196137028Z"
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
