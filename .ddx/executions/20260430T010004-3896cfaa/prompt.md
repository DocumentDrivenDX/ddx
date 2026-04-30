<bead-review>
  <bead id="ddx-7eab13a6" iter=1>
    <title>bead-id resolver: malformed IDs when ddx bead create runs from inside an execute-bead worktree (+ flag whether workers should create beads in-band at all)</title>
    <description>
## Two-layer problem

### Layer 1 (mechanical bug)

When a worker invokes `ddx bead create` from inside its execute-bead worktree, the resulting bead IDs are malformed. Reproduced today on this project:

  /home/erik/.local/bin/ddx bead ready  → 11 entries with IDs like
  `.execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-e630aeae`

These were the read-coverage child beads spawned from the ddx-44236615 worker (commit ebeb2464). They have valid descriptions, AC, priorities, and labels — only the ID field is mangled. The pattern `.execute-bead-wt-&lt;parent&gt;-&lt;timestamp&gt;-&lt;short&gt;-&lt;random&gt;` matches the worktree directory name format used by `ddx agent execute-bead`. Most likely failure path: the bead-id resolver consults `os.Getwd()` (or equivalent) to derive a project context prefix, sees the worktree dir, and uses it instead of the real project root + a fresh `ddx-&lt;8hex&gt;` short hash.

Concrete impact:
- 50+ char IDs are unwieldy in CLI commands (`ddx bead show &lt;id&gt;`, `ddx bead dep add ...`).
- Cross-project readers and the bead store assume `ddx-&lt;8hex&gt;` prefix matching for tooling (filtering, GraphQL adapters, MCP responses) — these long IDs may sort or filter unexpectedly.
- The IDs encode ephemeral attempt timestamps, which leaks worker-internal state into a permanent identifier.
- 12 such beads exist in the tracker as of this filing (grep `.execute-bead-wt-` in `.ddx/beads.jsonl`).

### Layer 2 (design question — separate decision)

Even if the IDs were correctly formed `ddx-&lt;8hex&gt;`, it is not obvious that `ddx bead create` from inside an execute-bead worker is the right pattern at all. Two alternatives the design has not articulated a position on:

**(a) Append to the parent bead.** When the worker discovers sub-tasks, it appends them as structured items in the parent's description, notes, or a new `kind:discovered-subtask` event. The parent bead becomes the durable record of "what this run found"; future runs (or the operator) can decompose if/when the work warrants its own tracker entries.

**(b) Surface via the execute-bead result.** The agent emits a structured `discovered_subtasks: [...]` field on result.json; the loop / operator decides whether to file new beads. Keeps tracker mutations gated by an explicit decision point.

**(c) Create new beads in-band (current behavior).** The worker calls `ddx bead create` directly. Pro: fastest decomposition. Con: sub-tasks land in the queue without operator review; can flood the queue if the agent over-decomposes; complicates "what was THIS run's contribution" auditing.

Today's behavior is (c) but undocumented. The 11 read-coverage children landing as a single agent's decomposition is exactly the failure mode (c) makes easy: one analysis bead spawned 11 implementation beads in a single run, all P0, all without operator review. The work may be valid — but the pattern needs a position.

## In-scope outcomes

This bead's job is to surface both layers; the implementation work decomposes into follow-ups.

1. **Concrete fix for layer 1**: bead-id resolver should normalize worktree paths to the real project root before allocating an ID. Test: `ddx bead create` invoked with cwd=`&lt;repo&gt;/.execute-bead-wt-...` produces `ddx-&lt;8hex&gt;`, not `.execute-bead-wt-...-&lt;random&gt;`.
2. **Mass repair for the 12 existing malformed beads**: either rewrite their IDs to fresh `ddx-&lt;8hex&gt;` (with a redirect or alias map for any external references), or close them as deferred and re-file with proper IDs. Decision deferred until layer 2 is settled.
3. **Layer 2 decision**: file a separate design bead (or amendment to FEAT-006 / FEAT-010) that picks (a), (b), or (c) and documents the rationale. This bead does NOT pre-decide; it captures the question.

## In-scope files (layer 1 fix)

- `cli/cmd/bead_create.go` (or wherever bead-create resolves project context).
- `cli/internal/bead/store.go` or `cli/internal/bead/id.go` if ID generation lives there.
- A unit test that drops cwd into a worktree-shaped dir and asserts the resulting bead id matches `^ddx-[0-9a-f]{8}$`.

(Locate via: `grep -rn "execute-bead-wt\|FindProjectRoot\|projectRoot" cli/internal/bead/ cli/cmd/bead_*.go`.)

## Out of scope

- The 11 read-coverage child beads' actual implementation work — that's separate beads.
- Default behavior change for whether workers can/should create beads — that's the layer-2 design bead.
- GraphQL or MCP changes to handle long IDs — fix the source instead.

## Acceptance

Layer-1 only (this bead):

1. `cd cli &amp;&amp; go test ./cmd/... ./internal/bead/...` passes after the fix.
2. New unit test in `cli/internal/bead/` (or `cli/cmd/`) that simulates a bead-create call from `cwd=&lt;project&gt;/.execute-bead-wt-ddx-XXXXX-&lt;timestamp&gt;-&lt;random&gt;` and asserts the new bead's `id` field matches `^ddx-[0-9a-f]{8}$`. Test fails before the fix, passes after.
3. `grep -E '"id":"\.execute-bead-wt-' .ddx/beads.jsonl` returns either 0 (mass repair) or 12 with a recorded decision in this bead's notes that the existing 12 are deferred to a follow-up bead.
4. A separate bead is filed (or referenced existing one updated) that captures the layer-2 design question (a/b/c). Reference its ID in this bead's events.

## Why one bead, not an epic

Layer 1 is a tight 1-3 file fix. Layer 2 is a design discussion that should be its own artifact, not a sibling task. Splitting them keeps each bounded.
    </description>
    <acceptance>
1. cd cli &amp;&amp; go test ./cmd/... ./internal/bead/... passes after the fix.
2. A new unit test simulates `ddx bead create` from cwd=`&lt;project&gt;/.execute-bead-wt-ddx-XXXXX-&lt;timestamp&gt;-&lt;random&gt;` and asserts the new bead's id field matches `^ddx-[0-9a-f]{8}$`. Test fails on current main and passes after the fix.
3. `grep -E '"id":"\.execute-bead-wt-' .ddx/beads.jsonl` returns 0, OR returns 12 with a recorded decision in this bead's notes that the existing 12 are deferred to a separate repair bead.
4. A layer-2 design bead is filed (or an existing one updated) that captures the question of whether workers should create beads in-band (option c), append to the parent (option a), or surface via execute-bead result (option b). Its ID is referenced in this bead's events.
    </acceptance>
    <labels>area:bead, area:agent, kind:bug, kind:design</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="e1e5f7cfd8c9656260461a7e63d3e67883bdd77a">
commit e1e5f7cfd8c9656260461a7e63d3e67883bdd77a
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 29 21:00:01 2026 -0400

    chore: add execution evidence [20260430T005604-]

diff --git a/.ddx/executions/20260430T005604-c425377f/manifest.json b/.ddx/executions/20260430T005604-c425377f/manifest.json
new file mode 100644
index 00000000..69b3de89
--- /dev/null
+++ b/.ddx/executions/20260430T005604-c425377f/manifest.json
@@ -0,0 +1,119 @@
+{
+  "attempt_id": "20260430T005604-c425377f",
+  "bead_id": "ddx-7eab13a6",
+  "base_rev": "d723f41601b7eda940afbee42696b56bc570287c",
+  "created_at": "2026-04-30T00:56:05.26895335Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-7eab13a6",
+    "title": "bead-id resolver: malformed IDs when ddx bead create runs from inside an execute-bead worktree (+ flag whether workers should create beads in-band at all)",
+    "description": "## Two-layer problem\n\n### Layer 1 (mechanical bug)\n\nWhen a worker invokes `ddx bead create` from inside its execute-bead worktree, the resulting bead IDs are malformed. Reproduced today on this project:\n\n  /home/erik/.local/bin/ddx bead ready  → 11 entries with IDs like\n  `.execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-e630aeae`\n\nThese were the read-coverage child beads spawned from the ddx-44236615 worker (commit ebeb2464). They have valid descriptions, AC, priorities, and labels — only the ID field is mangled. The pattern `.execute-bead-wt-\u003cparent\u003e-\u003ctimestamp\u003e-\u003cshort\u003e-\u003crandom\u003e` matches the worktree directory name format used by `ddx agent execute-bead`. Most likely failure path: the bead-id resolver consults `os.Getwd()` (or equivalent) to derive a project context prefix, sees the worktree dir, and uses it instead of the real project root + a fresh `ddx-\u003c8hex\u003e` short hash.\n\nConcrete impact:\n- 50+ char IDs are unwieldy in CLI commands (`ddx bead show \u003cid\u003e`, `ddx bead dep add ...`).\n- Cross-project readers and the bead store assume `ddx-\u003c8hex\u003e` prefix matching for tooling (filtering, GraphQL adapters, MCP responses) — these long IDs may sort or filter unexpectedly.\n- The IDs encode ephemeral attempt timestamps, which leaks worker-internal state into a permanent identifier.\n- 12 such beads exist in the tracker as of this filing (grep `.execute-bead-wt-` in `.ddx/beads.jsonl`).\n\n### Layer 2 (design question — separate decision)\n\nEven if the IDs were correctly formed `ddx-\u003c8hex\u003e`, it is not obvious that `ddx bead create` from inside an execute-bead worker is the right pattern at all. Two alternatives the design has not articulated a position on:\n\n**(a) Append to the parent bead.** When the worker discovers sub-tasks, it appends them as structured items in the parent's description, notes, or a new `kind:discovered-subtask` event. The parent bead becomes the durable record of \"what this run found\"; future runs (or the operator) can decompose if/when the work warrants its own tracker entries.\n\n**(b) Surface via the execute-bead result.** The agent emits a structured `discovered_subtasks: [...]` field on result.json; the loop / operator decides whether to file new beads. Keeps tracker mutations gated by an explicit decision point.\n\n**(c) Create new beads in-band (current behavior).** The worker calls `ddx bead create` directly. Pro: fastest decomposition. Con: sub-tasks land in the queue without operator review; can flood the queue if the agent over-decomposes; complicates \"what was THIS run's contribution\" auditing.\n\nToday's behavior is (c) but undocumented. The 11 read-coverage children landing as a single agent's decomposition is exactly the failure mode (c) makes easy: one analysis bead spawned 11 implementation beads in a single run, all P0, all without operator review. The work may be valid — but the pattern needs a position.\n\n## In-scope outcomes\n\nThis bead's job is to surface both layers; the implementation work decomposes into follow-ups.\n\n1. **Concrete fix for layer 1**: bead-id resolver should normalize worktree paths to the real project root before allocating an ID. Test: `ddx bead create` invoked with cwd=`\u003crepo\u003e/.execute-bead-wt-...` produces `ddx-\u003c8hex\u003e`, not `.execute-bead-wt-...-\u003crandom\u003e`.\n2. **Mass repair for the 12 existing malformed beads**: either rewrite their IDs to fresh `ddx-\u003c8hex\u003e` (with a redirect or alias map for any external references), or close them as deferred and re-file with proper IDs. Decision deferred until layer 2 is settled.\n3. **Layer 2 decision**: file a separate design bead (or amendment to FEAT-006 / FEAT-010) that picks (a), (b), or (c) and documents the rationale. This bead does NOT pre-decide; it captures the question.\n\n## In-scope files (layer 1 fix)\n\n- `cli/cmd/bead_create.go` (or wherever bead-create resolves project context).\n- `cli/internal/bead/store.go` or `cli/internal/bead/id.go` if ID generation lives there.\n- A unit test that drops cwd into a worktree-shaped dir and asserts the resulting bead id matches `^ddx-[0-9a-f]{8}$`.\n\n(Locate via: `grep -rn \"execute-bead-wt\\|FindProjectRoot\\|projectRoot\" cli/internal/bead/ cli/cmd/bead_*.go`.)\n\n## Out of scope\n\n- The 11 read-coverage child beads' actual implementation work — that's separate beads.\n- Default behavior change for whether workers can/should create beads — that's the layer-2 design bead.\n- GraphQL or MCP changes to handle long IDs — fix the source instead.\n\n## Acceptance\n\nLayer-1 only (this bead):\n\n1. `cd cli \u0026\u0026 go test ./cmd/... ./internal/bead/...` passes after the fix.\n2. New unit test in `cli/internal/bead/` (or `cli/cmd/`) that simulates a bead-create call from `cwd=\u003cproject\u003e/.execute-bead-wt-ddx-XXXXX-\u003ctimestamp\u003e-\u003crandom\u003e` and asserts the new bead's `id` field matches `^ddx-[0-9a-f]{8}$`. Test fails before the fix, passes after.\n3. `grep -E '\"id\":\"\\.execute-bead-wt-' .ddx/beads.jsonl` returns either 0 (mass repair) or 12 with a recorded decision in this bead's notes that the existing 12 are deferred to a follow-up bead.\n4. A separate bead is filed (or referenced existing one updated) that captures the layer-2 design question (a/b/c). Reference its ID in this bead's events.\n\n## Why one bead, not an epic\n\nLayer 1 is a tight 1-3 file fix. Layer 2 is a design discussion that should be its own artifact, not a sibling task. Splitting them keeps each bounded.",
+    "acceptance": "1. cd cli \u0026\u0026 go test ./cmd/... ./internal/bead/... passes after the fix.\n2. A new unit test simulates `ddx bead create` from cwd=`\u003cproject\u003e/.execute-bead-wt-ddx-XXXXX-\u003ctimestamp\u003e-\u003crandom\u003e` and asserts the new bead's id field matches `^ddx-[0-9a-f]{8}$`. Test fails on current main and passes after the fix.\n3. `grep -E '\"id\":\"\\.execute-bead-wt-' .ddx/beads.jsonl` returns 0, OR returns 12 with a recorded decision in this bead's notes that the existing 12 are deferred to a separate repair bead.\n4. A layer-2 design bead is filed (or an existing one updated) that captures the question of whether workers should create beads in-band (option c), append to the parent (option a), or surface via execute-bead result (option b). Its ID is referenced in this bead's events.",
+    "labels": [
+      "area:bead",
+      "area:agent",
+      "kind:bug",
+      "kind:design"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-30T00:56:04Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2278885",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-29T22:02:48.370930131Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260429T215419-7600dda3\",\"harness\":\"claude\",\"model\":\"sonnet\",\"input_tokens\":35,\"output_tokens\":15621,\"total_tokens\":15656,\"cost_usd\":1.1730871000000003,\"duration_ms\":507667,\"exit_code\":0}",
+          "created_at": "2026-04-29T22:02:48.465717142Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=15656 cost_usd=1.1731 model=sonnet"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"sonnet\",\"resolved_provider\":\"claude\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-04-29T22:02:52.775820422Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "erik",
+          "body": "REQUEST_CHANGES\nFix in store.go correctly threads workingDir through detectPrefix and resolves the worktree-cwd contamination. However, the new test does not simulate the bug scenario: it never chdirs into a worktree-shaped path, so it would pass on main too — failing AC #2's 'fails-before-fix, passes-after' requirement. Layer-2 bead and deferral note are acceptable.\nartifact: .ddx/executions/20260429T215419-7600dda3/reviewer-stream.log\nharness=claude\nmodel=opus\ninput_bytes=18364\noutput_bytes=1732\nelapsed_ms=39934",
+          "created_at": "2026-04-29T22:03:34.906410296Z",
+          "kind": "review",
+          "source": "ddx agent execute-loop",
+          "summary": "REQUEST_CHANGES"
+        },
+        {
+          "actor": "",
+          "body": "",
+          "created_at": "2026-04-29T22:03:35.007920786Z",
+          "kind": "reopen",
+          "source": "",
+          "summary": "review: REQUEST_CHANGES"
+        },
+        {
+          "actor": "erik",
+          "body": "post-merge review: REQUEST_CHANGES\nFix in store.go correctly threads workingDir through detectPrefix and resolves the worktree-cwd contamination. However, the new test does not simulate the bug scenario: it never chdirs into a worktree-shaped path, so it would pass on main too — failing AC #2's 'fails-before-fix, passes-after' requirement. Layer-2 bead and deferral note are acceptable.\nresult_rev=8cecc08fb79bf09c812fd87226dfad429457527d\nbase_rev=e2c801cbfa064c9b7b2b5de23a2a996527b74dd1",
+          "created_at": "2026-04-29T22:03:35.101108194Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "review_request_changes"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-29T23:29:15.366252468Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-29T23:29:15.483729427Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-29T23:29:15.572142875Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "erik",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-04-29T23:29:15.745850567Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-30T00:56:04.389767975Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260430T005604-c425377f",
+    "prompt": ".ddx/executions/20260430T005604-c425377f/prompt.md",
+    "manifest": ".ddx/executions/20260430T005604-c425377f/manifest.json",
+    "result": ".ddx/executions/20260430T005604-c425377f/result.json",
+    "checks": ".ddx/executions/20260430T005604-c425377f/checks.json",
+    "usage": ".ddx/executions/20260430T005604-c425377f/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-7eab13a6-20260430T005604-c425377f"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260430T005604-c425377f/result.json b/.ddx/executions/20260430T005604-c425377f/result.json
new file mode 100644
index 00000000..8b162fc4
--- /dev/null
+++ b/.ddx/executions/20260430T005604-c425377f/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-7eab13a6",
+  "attempt_id": "20260430T005604-c425377f",
+  "base_rev": "d723f41601b7eda940afbee42696b56bc570287c",
+  "result_rev": "c86b20995b9b191c55007c190520cc2bc18a62af",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-22cc4abe",
+  "duration_ms": 234753,
+  "tokens": 10836,
+  "cost_usd": 1.3640000000000003,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260430T005604-c425377f",
+  "prompt_file": ".ddx/executions/20260430T005604-c425377f/prompt.md",
+  "manifest_file": ".ddx/executions/20260430T005604-c425377f/manifest.json",
+  "result_file": ".ddx/executions/20260430T005604-c425377f/result.json",
+  "usage_file": ".ddx/executions/20260430T005604-c425377f/usage.json",
+  "started_at": "2026-04-30T00:56:05.269374308Z",
+  "finished_at": "2026-04-30T01:00:00.02244817Z"
+}
\ No newline at end of file
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

## Your task

Examine the diff and each acceptance-criteria (AC) item. For each item assign one grade:

- **APPROVE** — fully and correctly implemented; cite the specific file path and line that proves it.
- **REQUEST_CHANGES** — partially implemented or has fixable minor issues.
- **BLOCK** — not implemented, incorrectly implemented, or the diff is insufficient to evaluate.

Overall verdict rule:
- All items APPROVE → **APPROVE**
- Any item BLOCK → **BLOCK**
- Otherwise → **REQUEST_CHANGES**

## Required output format

Respond with a structured review using exactly this layout (replace placeholder text):

---
## Review: ddx-7eab13a6 iter 1

### Verdict: APPROVE | REQUEST_CHANGES | BLOCK

### AC Grades

| # | Item | Grade | Evidence |
|---|------|-------|----------|
| 1 | &lt;AC item text, max 60 chars&gt; | APPROVE | path/to/file.go:42 — brief note |
| 2 | &lt;AC item text, max 60 chars&gt; | BLOCK   | — not found in diff |

### Summary

&lt;1–3 sentences on overall implementation quality and any recurring theme in findings.&gt;

### Findings

&lt;Bullet list of REQUEST_CHANGES and BLOCK findings. Each finding must name the specific file, function, or test that is missing or wrong — specific enough for the next agent to act on without re-reading the entire diff. Omit this section entirely if verdict is APPROVE.&gt;
  </instructions>
</bead-review>
