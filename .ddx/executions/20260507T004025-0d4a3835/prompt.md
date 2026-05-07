<bead-review>
  <bead id="ddx-ae4b7393" iter=1>
    <title>checks: backfill production-reachability — internal/evidence (8 unreached)</title>
    <description>
Decomposed from REACH-BACKFILL (ddx-83440482). The Go production-reachability check (library/checks/go-production-reachability) flagged 8 symbol(s) in package `internal/evidence` as unreachable from cli/ entry roots (deadcode RTA).

Symbols:
- internal/evidence/read.go:24 — OversizeError.Error
- internal/evidence/read.go:29 — OversizeError.Unwrap
- internal/evidence/read.go:70 — ReadFileHardFail
- internal/evidence/sections.go:51 — FitSections
- internal/evidence/sections.go:139 — capContent
- internal/evidence/sections.go:153 — trimToLineBudget
- internal/evidence/strategy.go:19 — AssembleRefOnly
- internal/evidence/strategy.go:50 — AssembleInline

For each: WIRE (add to production call graph) or DELETE (genuinely obsolete). If neither is clear within ~15 min, annotate `// wiring:pending &lt;follow-up-bead-id&gt;` and file a follow-up bead.

Decision rule: if the originating bead's AC describes desired runtime behavior → WIRE; if speculative or design has moved on → DELETE.

Initial-violations evidence: .ddx/executions/20260503T124553-282667f7/initial-violations.json
    </description>
    <acceptance>
1. Each of the 8 listed symbols is either wired into the production call graph reachable from main() OR deleted (with originating bead reopened/closed-as-obsolete as appropriate).
2. Any remaining wiring:pending annotations cite open follow-up beads.
3. After landing: deadcode RTA (`go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` from cli/) reports zero remaining dead symbols in `internal/evidence`.
4. cd cli &amp;&amp; go test ./... green.
5. Decisions log written to .ddx/executions/&lt;run-id&gt;/decisions.md (one line per symbol: WIRE|DELETE|PENDING &lt;reason&gt;).
    </acceptance>
    <labels>phase:2, area:checks, kind:backfill</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260507T003720-d43fa7ba/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="38f493a1ca1faedff7a7012168ce040deccc46e4">
<untrusted-data>
diff --git a/.ddx/executions/20260507T003720-d43fa7ba/result.json b/.ddx/executions/20260507T003720-d43fa7ba/result.json
new file mode 100644
index 000000000..3be74d79c
--- /dev/null
+++ b/.ddx/executions/20260507T003720-d43fa7ba/result.json
@@ -0,0 +1,24 @@
+{
+  "bead_id": "ddx-ae4b7393",
+  "attempt_id": "20260507T003720-d43fa7ba",
+  "base_rev": "b87634a6de6b12e38cd7a7d416f0119842094aa8",
+  "result_rev": "7255b43dd01402a568b30ce0c2f91d6ad66fa191",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-521d5e05",
+  "duration_ms": 169641,
+  "tokens": 2132901,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260507T003720-d43fa7ba",
+  "prompt_file": ".ddx/executions/20260507T003720-d43fa7ba/prompt.md",
+  "manifest_file": ".ddx/executions/20260507T003720-d43fa7ba/manifest.json",
+  "result_file": ".ddx/executions/20260507T003720-d43fa7ba/result.json",
+  "usage_file": ".ddx/executions/20260507T003720-d43fa7ba/usage.json",
+  "started_at": "2026-05-07T00:37:23.077212043Z",
+  "finished_at": "2026-05-07T00:40:12.718537589Z"
+}
\ No newline at end of file
</untrusted-data>
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
