<bead-review>
  <bead id="ddx-a78f836f" iter=1>
    <title>checks: backfill production-reachability — internal/testutils (15 unreached)</title>
    <description>
Decomposed from REACH-BACKFILL (ddx-83440482). The Go production-reachability check (library/checks/go-production-reachability) flagged 15 symbol(s) in package `internal/testutils` as unreachable from cli/ entry roots (deadcode RTA).

Symbols:
- internal/testutils/fixture_repo.go:23 — NewFixtureRepo
- internal/testutils/fixture_repo.go:50 — repoRoot
- internal/testutils/fixture_repo.go:79 — ResolveDDxBinary
- internal/testutils/fixture_repo.go:88 — resolveDDxBinary
- internal/testutils/testutils.go:21 — NewTestEnvironment
- internal/testutils/testutils.go:45 — TestEnvironment.Cleanup
- internal/testutils/testutils.go:54 — TestEnvironment.HomeDir
- internal/testutils/testutils.go:59 — TestEnvironment.WorkDir
- internal/testutils/testutils.go:64 — TestEnvironment.CreateFile
- internal/testutils/testutils.go:72 — TestEnvironment.CreateHomeFile
- internal/testutils/testutils.go:80 — TestEnvironment.CreateTemplate
- internal/testutils/testutils.go:92 — TestEnvironment.CreateConfig
- internal/testutils/testutils.go:97 — TestEnvironment.CreateGlobalConfig
- internal/testutils/testutils.go:102 — TestEnvironment.AssertFileExists
- internal/testutils/testutils.go:108 — TestEnvironment.AssertFileContent

For each: WIRE (add to production call graph) or DELETE (genuinely obsolete). If neither is clear within ~15 min, annotate `// wiring:pending &lt;follow-up-bead-id&gt;` and file a follow-up bead.

Decision rule: if the originating bead's AC describes desired runtime behavior → WIRE; if speculative or design has moved on → DELETE.

Initial-violations evidence: .ddx/executions/20260503T124553-282667f7/initial-violations.json
    </description>
    <acceptance>
1. Each of the 15 listed symbols is either wired into the production call graph reachable from main() OR deleted (with originating bead reopened/closed-as-obsolete as appropriate).
2. Any remaining wiring:pending annotations cite open follow-up beads.
3. After landing: deadcode RTA (`go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` from cli/) reports zero remaining dead symbols in `internal/testutils`.
4. cd cli &amp;&amp; go test ./... green.
5. Decisions log written to .ddx/executions/&lt;run-id&gt;/decisions.md (one line per symbol: WIRE|DELETE|PENDING &lt;reason&gt;).
    </acceptance>
    <labels>phase:2, area:checks, kind:backfill</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260507T022739-fbdd4715/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="1fab01fa3c7f9d66002d2f74b5689f32e8196abd">
<untrusted-data>
diff --git a/.ddx/executions/20260507T022739-fbdd4715/result.json b/.ddx/executions/20260507T022739-fbdd4715/result.json
new file mode 100644
index 000000000..eb5540d28
--- /dev/null
+++ b/.ddx/executions/20260507T022739-fbdd4715/result.json
@@ -0,0 +1,24 @@
+{
+  "bead_id": "ddx-a78f836f",
+  "attempt_id": "20260507T022739-fbdd4715",
+  "base_rev": "c1f82b406aa1fe472ffda4e668bde7f8ccd8e31f",
+  "result_rev": "522c1746b4ce257521638a217e9ac8424edddf7b",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-f751c4b2",
+  "duration_ms": 60110,
+  "tokens": 416067,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260507T022739-fbdd4715",
+  "prompt_file": ".ddx/executions/20260507T022739-fbdd4715/prompt.md",
+  "manifest_file": ".ddx/executions/20260507T022739-fbdd4715/manifest.json",
+  "result_file": ".ddx/executions/20260507T022739-fbdd4715/result.json",
+  "usage_file": ".ddx/executions/20260507T022739-fbdd4715/usage.json",
+  "started_at": "2026-05-07T02:27:42.742574893Z",
+  "finished_at": "2026-05-07T02:28:42.853522129Z"
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
