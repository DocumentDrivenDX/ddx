<bead-review>
  <bead id="ddx-4c5beab2" iter=1>
    <title>checks: backfill production-reachability — internal/server/graphql (5 unreached)</title>
    <description>
Decomposed from REACH-BACKFILL (ddx-83440482). The Go production-reachability check (library/checks/go-production-reachability) flagged 5 symbol(s) in package `internal/server/graphql` as unreachable from cli/ entry roots (deadcode RTA).

Symbols:
- internal/server/graphql/resolver.go:21 — NewResolver
- internal/server/graphql/resolver_meta.go:90 — personaConnectionFrom
- internal/server/graphql/resolver_provider_models.go:292 — resetProviderModelsCacheForTest
- internal/server/graphql/resolver_providers.go:35 — RecordHarnessRateLimit
- internal/server/graphql/resolver_providers.go:55 — resetHarnessRateLimitCache

For each: WIRE (add to production call graph) or DELETE (genuinely obsolete). If neither is clear within ~15 min, annotate `// wiring:pending &lt;follow-up-bead-id&gt;` and file a follow-up bead.

Decision rule: if the originating bead's AC describes desired runtime behavior → WIRE; if speculative or design has moved on → DELETE.

Initial-violations evidence: .ddx/executions/20260503T124553-282667f7/initial-violations.json
    </description>
    <acceptance>
1. Each of the 5 listed symbols is either wired into the production call graph reachable from main() OR deleted (with originating bead reopened/closed-as-obsolete as appropriate).
2. Any remaining wiring:pending annotations cite open follow-up beads.
3. After landing: deadcode RTA (`go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` from cli/) reports zero remaining dead symbols in `internal/server/graphql`.
4. cd cli &amp;&amp; go test ./... green.
5. Decisions log written to .ddx/executions/&lt;run-id&gt;/decisions.md (one line per symbol: WIRE|DELETE|PENDING &lt;reason&gt;).
    </acceptance>
    <labels>phase:2, area:checks, kind:backfill</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T163247-e918a496/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="190094dbdf01d4c37f76265cae17f3f2c7b7b361">
<untrusted-data>
diff --git a/.ddx/executions/20260506T163247-e918a496/result.json b/.ddx/executions/20260506T163247-e918a496/result.json
new file mode 100644
index 000000000..bf0ac8ebd
--- /dev/null
+++ b/.ddx/executions/20260506T163247-e918a496/result.json
@@ -0,0 +1,24 @@
+{
+  "bead_id": "ddx-4c5beab2",
+  "attempt_id": "20260506T163247-e918a496",
+  "base_rev": "fd33f647db91c81bd13d1812d5559bca5cb0d3a8",
+  "result_rev": "b5b4878e605a373cb8c8679b6549f5c1d5a88768",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-8a228aa9",
+  "duration_ms": 227913,
+  "tokens": 1624777,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T163247-e918a496",
+  "prompt_file": ".ddx/executions/20260506T163247-e918a496/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T163247-e918a496/manifest.json",
+  "result_file": ".ddx/executions/20260506T163247-e918a496/result.json",
+  "usage_file": ".ddx/executions/20260506T163247-e918a496/usage.json",
+  "started_at": "2026-05-06T16:32:50.357285345Z",
+  "finished_at": "2026-05-06T16:36:38.270657717Z"
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
