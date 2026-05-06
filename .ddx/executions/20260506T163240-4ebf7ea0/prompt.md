<bead-review>
  <bead id="ddx-2850c4dc" iter=1>
    <title>checks: backfill production-reachability — internal/metric (14 unreached)</title>
    <description>
Decomposed from REACH-BACKFILL (ddx-83440482). The Go production-reachability check (library/checks/go-production-reachability) flagged 14 symbol(s) in package `internal/metric` as unreachable from cli/ entry roots (deadcode RTA).

Symbols:
- internal/metric/exec_bridge.go:9 — metricDefinitionToExec
- internal/metric/exec_bridge.go:36 — metricDefinitionFromExec
- internal/metric/exec_bridge.go:62 — metricHistoryToRun
- internal/metric/exec_bridge.go:142 — cloneStringMap
- internal/metric/store.go:26 — Store.Init
- internal/metric/store.go:33 — Store.Validate
- internal/metric/store.go:59 — Store.Run
- internal/metric/store.go:71 — Store.Compare
- internal/metric/store.go:117 — Store.LoadDefinition
- internal/metric/store.go:154 — Store.SaveDefinition
- internal/metric/store.go:167 — Store.AppendHistory
- internal/metric/store.go:195 — Store.loadMetricArtifact
- internal/metric/store.go:210 — selectComparisonTarget
- internal/metric/store.go:226 — comparisonFor

For each: WIRE (add to production call graph) or DELETE (genuinely obsolete). If neither is clear within ~15 min, annotate `// wiring:pending &lt;follow-up-bead-id&gt;` and file a follow-up bead.

Decision rule: if the originating bead's AC describes desired runtime behavior → WIRE; if speculative or design has moved on → DELETE.

Initial-violations evidence: .ddx/executions/20260503T124553-282667f7/initial-violations.json
    </description>
    <acceptance>
1. Each of the 14 listed symbols is either wired into the production call graph reachable from main() OR deleted (with originating bead reopened/closed-as-obsolete as appropriate).
2. Any remaining wiring:pending annotations cite open follow-up beads.
3. After landing: deadcode RTA (`go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` from cli/) reports zero remaining dead symbols in `internal/metric`.
4. cd cli &amp;&amp; go test ./... green.
5. Decisions log written to .ddx/executions/&lt;run-id&gt;/decisions.md (one line per symbol: WIRE|DELETE|PENDING &lt;reason&gt;).
    </acceptance>
    <labels>phase:2, area:checks, kind:backfill</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T162849-7b34fa61/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="5d91db3baa5b304f35c49fc867e000e399778979">
<untrusted-data>
diff --git a/.ddx/executions/20260506T162849-7b34fa61/result.json b/.ddx/executions/20260506T162849-7b34fa61/result.json
new file mode 100644
index 000000000..7ed640db2
--- /dev/null
+++ b/.ddx/executions/20260506T162849-7b34fa61/result.json
@@ -0,0 +1,24 @@
+{
+  "bead_id": "ddx-2850c4dc",
+  "attempt_id": "20260506T162849-7b34fa61",
+  "base_rev": "e6487cc23e7009e865b79d1a97b707c8d6c166c7",
+  "result_rev": "a4aad2934a2020a84f8bc91bdda7b31685c7f788",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-9facfef5",
+  "duration_ms": 220191,
+  "tokens": 1604332,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T162849-7b34fa61",
+  "prompt_file": ".ddx/executions/20260506T162849-7b34fa61/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T162849-7b34fa61/manifest.json",
+  "result_file": ".ddx/executions/20260506T162849-7b34fa61/result.json",
+  "usage_file": ".ddx/executions/20260506T162849-7b34fa61/usage.json",
+  "started_at": "2026-05-06T16:28:52.41199448Z",
+  "finished_at": "2026-05-06T16:32:32.603749452Z"
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
