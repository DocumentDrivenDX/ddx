<bead-review>
  <bead id="ddx-7d7a17c8" iter=1>
    <title>TD-driven reconciliation: FEAT-004/SD-004 docs + Go status constants + IsValidStatusTransition matrix</title>
    <description>
Sibling of ddx-673833f4 (state-machine TD). After the TD lands, this bead reconciles all source-of-truth files to match the TD's canonical state list (the bd/br-compatible 6 — schema is NOT modified).

Mechanical work driven by the TD's decisions:

1. Update FEAT-004 docs/helix/01-frame/features/FEAT-004-beads.md line 65 — change documented status list from 3 values to the canonical 6 (open, in_progress, closed, blocked, proposed, cancelled).
2. Update SD-004 docs/helix/02-design/solution-designs/SD-004-beads-tracker.md — cross-reference the new TD.
3. Audit cli/internal/bead/types.go status constants (lines 59-75) — convert any non-canonical status references to use the canonical Status* constants.
4. Audit cli/internal/agent/ for non-canonical persisted-status writes — convert event-kind/phase/derived-category names that were misused as statuses (per the TD's naming-role decision matrix).
5. Update IsValidStatusTransition (cli/internal/bead/types.go:110-125) to match the TD's transition matrix.

NO functional behavior change. Pure documentation + code-vocabulary alignment to the TD.
    </description>
    <acceptance>
1. FEAT-004 line 65 documents the canonical 6-value status list. 2. SD-004 cross-references the new TD. 3. cli/internal/bead/types.go Status* constants match the canonical 6; no off-list constants exist. 4. No persisted status WRITES in cli/internal/agent/ use string literals outside the canonical 6 (verified via grep). 5. IsValidStatusTransition matches TD transition matrix; existing tests pass. 6. cd cli &amp;&amp; go test ./internal/bead/... ./internal/agent/... passes. 7. ddx doc audit clean.
    </acceptance>
    <labels>phase:2, story:10, area:specs, area:beads, kind:reconciliation</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T191700-036a0d6f/manifest.json</file>
    <file>.ddx/executions/20260502T191700-036a0d6f/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="467010584bd793a3bc88265bac16edff5e3f48e8">
diff --git a/.ddx/executions/20260502T191700-036a0d6f/manifest.json b/.ddx/executions/20260502T191700-036a0d6f/manifest.json
new file mode 100644
index 00000000..64130aa5
--- /dev/null
+++ b/.ddx/executions/20260502T191700-036a0d6f/manifest.json
@@ -0,0 +1,40 @@
+{
+  "attempt_id": "20260502T191700-036a0d6f",
+  "bead_id": "ddx-7d7a17c8",
+  "base_rev": "ae2fd6ba8102457bbfaa9b800354d750188016a9",
+  "created_at": "2026-05-02T19:17:01.391065632Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-7d7a17c8",
+    "title": "TD-driven reconciliation: FEAT-004/SD-004 docs + Go status constants + IsValidStatusTransition matrix",
+    "description": "Sibling of ddx-673833f4 (state-machine TD). After the TD lands, this bead reconciles all source-of-truth files to match the TD's canonical state list (the bd/br-compatible 6 — schema is NOT modified).\n\nMechanical work driven by the TD's decisions:\n\n1. Update FEAT-004 docs/helix/01-frame/features/FEAT-004-beads.md line 65 — change documented status list from 3 values to the canonical 6 (open, in_progress, closed, blocked, proposed, cancelled).\n2. Update SD-004 docs/helix/02-design/solution-designs/SD-004-beads-tracker.md — cross-reference the new TD.\n3. Audit cli/internal/bead/types.go status constants (lines 59-75) — convert any non-canonical status references to use the canonical Status* constants.\n4. Audit cli/internal/agent/ for non-canonical persisted-status writes — convert event-kind/phase/derived-category names that were misused as statuses (per the TD's naming-role decision matrix).\n5. Update IsValidStatusTransition (cli/internal/bead/types.go:110-125) to match the TD's transition matrix.\n\nNO functional behavior change. Pure documentation + code-vocabulary alignment to the TD.",
+    "acceptance": "1. FEAT-004 line 65 documents the canonical 6-value status list. 2. SD-004 cross-references the new TD. 3. cli/internal/bead/types.go Status* constants match the canonical 6; no off-list constants exist. 4. No persisted status WRITES in cli/internal/agent/ use string literals outside the canonical 6 (verified via grep). 5. IsValidStatusTransition matches TD transition matrix; existing tests pass. 6. cd cli \u0026\u0026 go test ./internal/bead/... ./internal/agent/... passes. 7. ddx doc audit clean.",
+    "parent": "ddx-e34994e2",
+    "labels": [
+      "phase:2",
+      "story:10",
+      "area:specs",
+      "area:beads",
+      "kind:reconciliation"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T19:17:00Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3446284",
+      "execute-loop-heartbeat-at": "2026-05-02T19:17:00.0896641Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T191700-036a0d6f",
+    "prompt": ".ddx/executions/20260502T191700-036a0d6f/prompt.md",
+    "manifest": ".ddx/executions/20260502T191700-036a0d6f/manifest.json",
+    "result": ".ddx/executions/20260502T191700-036a0d6f/result.json",
+    "checks": ".ddx/executions/20260502T191700-036a0d6f/checks.json",
+    "usage": ".ddx/executions/20260502T191700-036a0d6f/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-7d7a17c8-20260502T191700-036a0d6f"
+  },
+  "prompt_sha": "6aa91f362ab8c3071b4f6002f1aea203e7c8973d398a6dbfc65645c309302221"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T191700-036a0d6f/result.json b/.ddx/executions/20260502T191700-036a0d6f/result.json
new file mode 100644
index 00000000..2fb8e670
--- /dev/null
+++ b/.ddx/executions/20260502T191700-036a0d6f/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-7d7a17c8",
+  "attempt_id": "20260502T191700-036a0d6f",
+  "base_rev": "ae2fd6ba8102457bbfaa9b800354d750188016a9",
+  "result_rev": "625bd04ed972489272b1c1b26cc105ebac4ec46c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-ae09154e",
+  "duration_ms": 313946,
+  "tokens": 11625,
+  "cost_usd": 1.9842867500000005,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T191700-036a0d6f",
+  "prompt_file": ".ddx/executions/20260502T191700-036a0d6f/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T191700-036a0d6f/manifest.json",
+  "result_file": ".ddx/executions/20260502T191700-036a0d6f/result.json",
+  "usage_file": ".ddx/executions/20260502T191700-036a0d6f/usage.json",
+  "started_at": "2026-05-02T19:17:01.391314256Z",
+  "finished_at": "2026-05-02T19:22:15.337449465Z"
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
