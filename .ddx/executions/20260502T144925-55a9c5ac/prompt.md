<bead-review>
  <bead id="ddx-5d49b14e" iter=1>
    <title>Evaluate and adopt a real backend for the bead tracker (axon primary, bd fallback)</title>
    <description>
The current .ddx/beads.jsonl backend (atomic JSONL writes wrapped in flock; events accumulating per-bead; archive split per ADR-004) is showing strain:

1. Multi-worker contention via the git tracker-commit step (see ddx-da11a34a — the file lock around git add/commit is a v1 patch, not the real fix)
2. Events array per bead is unbounded and dominates archive size
3. Cross-machine concurrency is not addressed (Story 14 federation will require it)
4. The current model bakes one process-local store + git-as-replication; doesn't compose cleanly with a real database

Two backend paths under consideration:

**Axon (PRIMARY, per user inclination)**: integrate as the first DDx-side consumer of axon. User owns the stack; tight integration possible; proves axon's value as a substrate. Requires authoring an axon backend implementing the bead schema's storage contract (per ADR-004 / SD-004 — collection abstraction, claim semantics, atomic writes, JSONL interchange compatibility for bd/br).

**bd / doltdb (FALLBACK)**: delegate to bd which already speaks doltdb (git-for-data). Trivial integration; downside is tracking an external codebase as a hard dependency.

This epic frames the evaluation, designs the integration, prototypes, and migrates. Children break down the work. The lock-contention patch (ddx-da11a34a) remains relevant as stability scaffolding while this plays out — when axon ships and is adopted, the JSONL+git path may retire entirely (or stay as a portable interchange backend per FEAT-004's bd/br semantics).
    </description>
    <acceptance>
1. TD authored for the axon-backend integration (data model mapping, claim/lock semantics, archive policy, multi-machine concurrency story). 2. Axon-backend implementation lands behind a feature flag or selectable backend in cli/internal/bead/backend.go. 3. Migration tool moves an existing .ddx/beads.jsonl + .ddx/beads-archive.jsonl into the axon backend without data loss. 4. Existing chaos_test.go suite passes against the axon backend. 5. bd fallback documented (single artifact under docs/) showing how to wire bd as the backend if axon adoption stalls. 6. Performance: queue ops (ready/blocked/show) at parity or better than JSONL on the current 1100-bead archive scale.
    </acceptance>
    <labels>phase:2, area:beads, area:storage, kind:strategy, backend-migration</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T144455-ed29df8c/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="7f507f5e90963aaca65ea7b375731ac4f2560b46">
diff --git a/.ddx/executions/20260502T144455-ed29df8c/result.json b/.ddx/executions/20260502T144455-ed29df8c/result.json
new file mode 100644
index 00000000..7fe563a7
--- /dev/null
+++ b/.ddx/executions/20260502T144455-ed29df8c/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-5d49b14e",
+  "attempt_id": "20260502T144455-ed29df8c",
+  "base_rev": "0f9bd7ded096b060e10ffec49103ea8376849923",
+  "result_rev": "7a9636dd44793cc642e99744e82f28e574c34ae4",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-265d8c17",
+  "duration_ms": 260420,
+  "tokens": 16767,
+  "cost_usd": 1.6210455,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T144455-ed29df8c",
+  "prompt_file": ".ddx/executions/20260502T144455-ed29df8c/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T144455-ed29df8c/manifest.json",
+  "result_file": ".ddx/executions/20260502T144455-ed29df8c/result.json",
+  "usage_file": ".ddx/executions/20260502T144455-ed29df8c/usage.json",
+  "started_at": "2026-05-02T14:44:58.528534978Z",
+  "finished_at": "2026-05-02T14:49:18.948666059Z"
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
