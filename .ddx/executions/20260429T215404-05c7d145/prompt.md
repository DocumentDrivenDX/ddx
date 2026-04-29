<bead-review>
  <bead id="ddx-5bf4ee7e" iter=1>
    <title>pre-claim triage: complexity evaluator + auto-splitter for recursive epic decomposition before agent dispatch</title>
    <description>
## Goal

Today, when execute-loop dispatches a coarse bead (an unsplit epic) to an agent, the agent often returns `no_changes` with reasoning like "this is an epic, needs child-bead breakdown, not monolithic execution" — wasting a worker attempt, leaving the bead in a half-owned claimed state, and forcing manual operator intervention.

Concrete recent example: `agent-6c7a4c11` (Harness × Model Matrix Benchmark — initial tranche) on this user's `agent` repo was dispatched as a single bead, the agent declined to execute monolithically, the worker exited without unclaiming. The bead is now stuck claimed by a dead worker with no change applied.

The fix is to run a triage step **before** Claim: evaluate whether the bead is atomic (one PR's worth of work, single AC focus) or decomposable (multiple distinct deliverables, AC items map to separable PRs). If decomposable, run a splitter that produces child beads (parent-linked, AC-distributed), and skip dispatch of the parent. Children themselves re-enter triage — recursive decomposition with a depth cap.

## Concrete change

Insert a `complexityGate` phase in `cli/internal/agent/execute_bead_loop.go` between bead-pick and Claim:

```
next-candidate → ResolveRoute → preflight gate (ddx-98e6e9ef)
  → complexityGate (NEW)
  → if atomic:        Claim → invoke
  → if decomposable:  splitter → file children with parent link → mark parent blocked-by-children → re-pick
  → if ambiguous:     surface to human triage (event + label), don't claim
```

Two prompts, both shipped with the default DDx plugin under `library/prompts/triage/`:

1. **`complexity-eval.md`** — given bead body + AC + deps + history (prior attempts, reviewer BLOCKs), classify `{atomic, decomposable, ambiguous}` with reasoning + confidence.
2. **`bead-split.md`** — given a decomposable bead, emit N child beads (JSON) with: title, AC, parent link, spec-id inheritance, in-scope files subset, labels. Children must collectively cover the parent's AC.

Both prompts use the embedded `agent` harness with profile=default (cheap tier, local-first) — the gate's cost/latency must be &lt;&lt; a wasted dispatch.

## Historical evidence engineering

Both prompts must be engineered against historical evidence, not vibes. Harvest a corpus from `.ddx/beads.jsonl` (this project + the user's `agent` project, which has 5 months of bead history including known epic-rejection cases):

- Beads closed `no_changes` whose status text mentions "epic", "split", "breakdown", "scope", or "monolithic" → positive examples for **decomposable**.
- Beads with `attempt_count &gt;= 3` or repeated `kind:reopen` events from reviewer BLOCKs citing scope → positive examples for **decomposable**.
- Beads that were eventually split manually (parent epic + N child tasks filed in a single tracker batch) → use parent body as splitter input, the actual children as ground-truth output.
- Beads closed cleanly on first attempt with single-file or small-diff commits → positive examples for **atomic**.

Hold out 20% as eval. Report `complexity-eval` accuracy/F1 on classification; report `bead-split` AC-coverage rate (children's AC tokens together cover parent's AC) and child-count distribution vs. ground-truth.

The harvest script (`scripts/triage/harvest-corpus.go` or equivalent) is committed and reproducible.

## Recursion + depth cap

Children re-enter the gate. Default depth cap = 3, configurable via `agent.triage.max_decomposition_depth` in `.ddx/config.yaml`. At cap: surface to human (`kind:triage-overflow` event, `status=blocked`, `label=needs-human-decomposition`). Never silently dispatch a parent that hit cap.

## In-scope files

- `cli/internal/agent/execute_bead_loop.go` — new `complexityGate` phase pre-Claim
- `cli/internal/agent/triage.go` (new) — gate logic, prompt invocation, child filing
- `library/prompts/triage/complexity-eval.md` (new)
- `library/prompts/triage/bead-split.md` (new)
- `cli/internal/agent/triage_test.go` (new) — unit tests against held-out historical corpus
- `library/prompts/triage/eval-corpus.jsonl` (new) — frozen eval slice (training slice can be regenerated)
- `scripts/triage/harvest-corpus.go` (new) — corpus regeneration
- `docs/triage/decomposition.md` (new) — operator docs

## Out of scope (file follow-ups if/when needed)

- Backfill triage on already-claimed in-flight beads (e.g. unsticking agent-6c7a4c11 retroactively) — operational bead, not part of this feature.
- A web UI for visualizing the triage decision per bead — consumer bead.
- Per-team customization of split rules — premature; ship the default and iterate.

## Anti-goals

- Do **not** rewrite an existing bead in place. Splitting always files children; the parent stays as an epic with `status=blocked-by-children`.
- Do **not** make the gate silently optional. If disabled via flag or config, emit a one-time warning per worker boot.
- Do **not** use a model larger than the cheap tier for the gate prompts.
- Do **not** duplicate routing logic in the gate; it should be a thin caller of the agent SDK with the gate's two prompts.

## Why P1

The failure mode it prevents — claimed-but-undeliverable beads — corrupts the queue's claim semantics, wastes operator time, and silently inflates `attempt_count` on epics that will never close. Without this gate, the rest of the routing-cleanup work (`ddx-fdd3ea36` umbrella) ships into a queue with structurally bad inputs.
    </description>
    <acceptance>
1. `complexityGate` phase wired into `cli/internal/agent/execute_bead_loop.go` pre-Claim. Unit test asserts: a known-decomposable bead from the historical corpus does NOT reach `Claim`; a known-atomic bead does.

2. Both prompts (`library/prompts/triage/complexity-eval.md`, `library/prompts/triage/bead-split.md`) exist and pass `go test ./cli/internal/agent -run TriagePrompts`, which exercises them against the held-out eval set via the embedded agent harness.

3. `complexity-eval` classifier achieves &gt;= 80% accuracy on the held-out eval slice. The corpus and eval slice are reproducibly built from `.ddx/beads.jsonl` via `scripts/triage/harvest-corpus.go`; running the script regenerates the same eval set deterministically (sorted, seeded split).

4. `bead-split` produces child beads whose collective AC covers &gt;= 90% of the parent's AC tokens (string-overlap metric documented in `docs/triage/decomposition.md`), measured on the held-out eval slice.

5. Recursion depth cap = 3 by default, configurable via `agent.triage.max_decomposition_depth`. At cap, parent is marked `status=blocked` with `label=needs-human-decomposition` and a `kind:triage-overflow` event is filed. Test asserts: a synthetic always-decomposable bead reaches the cap and the parent is blocked, never dispatched.

6. End-to-end: run `ddx work --once` against a fixture queue containing the historical body of `agent-6c7a4c11` (or an equivalent epic). Assert: (a) gate classifies decomposable, (b) children are filed with parent link, (c) parent is not dispatched, (d) the first atomic child IS dispatched.

7. Operator docs at `docs/triage/decomposition.md` cover: when the gate triggers, how to opt out per-bead (`ddx bead update &lt;id&gt; --label triage:skip`), how to configure depth cap, and how to regenerate the historical corpus.

8. Tracker: parent epic has `kind:triage-decomposed` event listing child IDs; children carry `parent: &lt;id&gt;` and inherit `spec-id` from parent.
    </acceptance>
    <labels>area:dispatch, area:bead-triage, kind:feat, phase:design</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T211911-b89b72eb/manifest.json</file>
    <file>.ddx/executions/20260429T211911-b89b72eb/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="09189f68f3fd49eb0642732243e6b0e8a9a98e88">
diff --git a/.ddx/executions/20260429T211911-b89b72eb/manifest.json b/.ddx/executions/20260429T211911-b89b72eb/manifest.json
new file mode 100644
index 00000000..8a45bd80
--- /dev/null
+++ b/.ddx/executions/20260429T211911-b89b72eb/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260429T211911-b89b72eb",
+  "bead_id": "ddx-5bf4ee7e",
+  "base_rev": "be66b9b30a57070bdbb1ec523d58de3f639fd1e1",
+  "created_at": "2026-04-29T21:19:12.271845242Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-5bf4ee7e",
+    "title": "pre-claim triage: complexity evaluator + auto-splitter for recursive epic decomposition before agent dispatch",
+    "description": "## Goal\n\nToday, when execute-loop dispatches a coarse bead (an unsplit epic) to an agent, the agent often returns `no_changes` with reasoning like \"this is an epic, needs child-bead breakdown, not monolithic execution\" — wasting a worker attempt, leaving the bead in a half-owned claimed state, and forcing manual operator intervention.\n\nConcrete recent example: `agent-6c7a4c11` (Harness × Model Matrix Benchmark — initial tranche) on this user's `agent` repo was dispatched as a single bead, the agent declined to execute monolithically, the worker exited without unclaiming. The bead is now stuck claimed by a dead worker with no change applied.\n\nThe fix is to run a triage step **before** Claim: evaluate whether the bead is atomic (one PR's worth of work, single AC focus) or decomposable (multiple distinct deliverables, AC items map to separable PRs). If decomposable, run a splitter that produces child beads (parent-linked, AC-distributed), and skip dispatch of the parent. Children themselves re-enter triage — recursive decomposition with a depth cap.\n\n## Concrete change\n\nInsert a `complexityGate` phase in `cli/internal/agent/execute_bead_loop.go` between bead-pick and Claim:\n\n```\nnext-candidate → ResolveRoute → preflight gate (ddx-98e6e9ef)\n  → complexityGate (NEW)\n  → if atomic:        Claim → invoke\n  → if decomposable:  splitter → file children with parent link → mark parent blocked-by-children → re-pick\n  → if ambiguous:     surface to human triage (event + label), don't claim\n```\n\nTwo prompts, both shipped with the default DDx plugin under `library/prompts/triage/`:\n\n1. **`complexity-eval.md`** — given bead body + AC + deps + history (prior attempts, reviewer BLOCKs), classify `{atomic, decomposable, ambiguous}` with reasoning + confidence.\n2. **`bead-split.md`** — given a decomposable bead, emit N child beads (JSON) with: title, AC, parent link, spec-id inheritance, in-scope files subset, labels. Children must collectively cover the parent's AC.\n\nBoth prompts use the embedded `agent` harness with profile=default (cheap tier, local-first) — the gate's cost/latency must be \u003c\u003c a wasted dispatch.\n\n## Historical evidence engineering\n\nBoth prompts must be engineered against historical evidence, not vibes. Harvest a corpus from `.ddx/beads.jsonl` (this project + the user's `agent` project, which has 5 months of bead history including known epic-rejection cases):\n\n- Beads closed `no_changes` whose status text mentions \"epic\", \"split\", \"breakdown\", \"scope\", or \"monolithic\" → positive examples for **decomposable**.\n- Beads with `attempt_count \u003e= 3` or repeated `kind:reopen` events from reviewer BLOCKs citing scope → positive examples for **decomposable**.\n- Beads that were eventually split manually (parent epic + N child tasks filed in a single tracker batch) → use parent body as splitter input, the actual children as ground-truth output.\n- Beads closed cleanly on first attempt with single-file or small-diff commits → positive examples for **atomic**.\n\nHold out 20% as eval. Report `complexity-eval` accuracy/F1 on classification; report `bead-split` AC-coverage rate (children's AC tokens together cover parent's AC) and child-count distribution vs. ground-truth.\n\nThe harvest script (`scripts/triage/harvest-corpus.go` or equivalent) is committed and reproducible.\n\n## Recursion + depth cap\n\nChildren re-enter the gate. Default depth cap = 3, configurable via `agent.triage.max_decomposition_depth` in `.ddx/config.yaml`. At cap: surface to human (`kind:triage-overflow` event, `status=blocked`, `label=needs-human-decomposition`). Never silently dispatch a parent that hit cap.\n\n## In-scope files\n\n- `cli/internal/agent/execute_bead_loop.go` — new `complexityGate` phase pre-Claim\n- `cli/internal/agent/triage.go` (new) — gate logic, prompt invocation, child filing\n- `library/prompts/triage/complexity-eval.md` (new)\n- `library/prompts/triage/bead-split.md` (new)\n- `cli/internal/agent/triage_test.go` (new) — unit tests against held-out historical corpus\n- `library/prompts/triage/eval-corpus.jsonl` (new) — frozen eval slice (training slice can be regenerated)\n- `scripts/triage/harvest-corpus.go` (new) — corpus regeneration\n- `docs/triage/decomposition.md` (new) — operator docs\n\n## Out of scope (file follow-ups if/when needed)\n\n- Backfill triage on already-claimed in-flight beads (e.g. unsticking agent-6c7a4c11 retroactively) — operational bead, not part of this feature.\n- A web UI for visualizing the triage decision per bead — consumer bead.\n- Per-team customization of split rules — premature; ship the default and iterate.\n\n## Anti-goals\n\n- Do **not** rewrite an existing bead in place. Splitting always files children; the parent stays as an epic with `status=blocked-by-children`.\n- Do **not** make the gate silently optional. If disabled via flag or config, emit a one-time warning per worker boot.\n- Do **not** use a model larger than the cheap tier for the gate prompts.\n- Do **not** duplicate routing logic in the gate; it should be a thin caller of the agent SDK with the gate's two prompts.\n\n## Why P1\n\nThe failure mode it prevents — claimed-but-undeliverable beads — corrupts the queue's claim semantics, wastes operator time, and silently inflates `attempt_count` on epics that will never close. Without this gate, the rest of the routing-cleanup work (`ddx-fdd3ea36` umbrella) ships into a queue with structurally bad inputs.",
+    "acceptance": "1. `complexityGate` phase wired into `cli/internal/agent/execute_bead_loop.go` pre-Claim. Unit test asserts: a known-decomposable bead from the historical corpus does NOT reach `Claim`; a known-atomic bead does.\n\n2. Both prompts (`library/prompts/triage/complexity-eval.md`, `library/prompts/triage/bead-split.md`) exist and pass `go test ./cli/internal/agent -run TriagePrompts`, which exercises them against the held-out eval set via the embedded agent harness.\n\n3. `complexity-eval` classifier achieves \u003e= 80% accuracy on the held-out eval slice. The corpus and eval slice are reproducibly built from `.ddx/beads.jsonl` via `scripts/triage/harvest-corpus.go`; running the script regenerates the same eval set deterministically (sorted, seeded split).\n\n4. `bead-split` produces child beads whose collective AC covers \u003e= 90% of the parent's AC tokens (string-overlap metric documented in `docs/triage/decomposition.md`), measured on the held-out eval slice.\n\n5. Recursion depth cap = 3 by default, configurable via `agent.triage.max_decomposition_depth`. At cap, parent is marked `status=blocked` with `label=needs-human-decomposition` and a `kind:triage-overflow` event is filed. Test asserts: a synthetic always-decomposable bead reaches the cap and the parent is blocked, never dispatched.\n\n6. End-to-end: run `ddx work --once` against a fixture queue containing the historical body of `agent-6c7a4c11` (or an equivalent epic). Assert: (a) gate classifies decomposable, (b) children are filed with parent link, (c) parent is not dispatched, (d) the first atomic child IS dispatched.\n\n7. Operator docs at `docs/triage/decomposition.md` cover: when the gate triggers, how to opt out per-bead (`ddx bead update \u003cid\u003e --label triage:skip`), how to configure depth cap, and how to regenerate the historical corpus.\n\n8. Tracker: parent epic has `kind:triage-decomposed` event listing child IDs; children carry `parent: \u003cid\u003e` and inherit `spec-id` from parent.",
+    "labels": [
+      "area:dispatch",
+      "area:bead-triage",
+      "kind:feat",
+      "phase:design"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T21:19:09Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T21:19:09.550246681Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T211911-b89b72eb",
+    "prompt": ".ddx/executions/20260429T211911-b89b72eb/prompt.md",
+    "manifest": ".ddx/executions/20260429T211911-b89b72eb/manifest.json",
+    "result": ".ddx/executions/20260429T211911-b89b72eb/result.json",
+    "checks": ".ddx/executions/20260429T211911-b89b72eb/checks.json",
+    "usage": ".ddx/executions/20260429T211911-b89b72eb/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-5bf4ee7e-20260429T211911-b89b72eb"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T211911-b89b72eb/result.json b/.ddx/executions/20260429T211911-b89b72eb/result.json
new file mode 100644
index 00000000..93ee9c80
--- /dev/null
+++ b/.ddx/executions/20260429T211911-b89b72eb/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-5bf4ee7e",
+  "attempt_id": "20260429T211911-b89b72eb",
+  "base_rev": "be66b9b30a57070bdbb1ec523d58de3f639fd1e1",
+  "result_rev": "d87cdf2fc2f943b881e65653f2d44aee20c08e5e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-3786cad3",
+  "duration_ms": 2087346,
+  "tokens": 86140,
+  "cost_usd": 5.198592599999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T211911-b89b72eb",
+  "prompt_file": ".ddx/executions/20260429T211911-b89b72eb/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T211911-b89b72eb/manifest.json",
+  "result_file": ".ddx/executions/20260429T211911-b89b72eb/result.json",
+  "usage_file": ".ddx/executions/20260429T211911-b89b72eb/usage.json",
+  "started_at": "2026-04-29T21:19:12.272252992Z",
+  "finished_at": "2026-04-29T21:53:59.619180495Z"
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
