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
    <file>.ddx/executions/20260506T175523-c95a730b/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="618b448b837e5ebf18f36acc35496eae7a48c7c8">
<untrusted-data>
diff --git a/.ddx/executions/20260506T175523-c95a730b/result.json b/.ddx/executions/20260506T175523-c95a730b/result.json
new file mode 100644
index 000000000..1e691dd53
--- /dev/null
+++ b/.ddx/executions/20260506T175523-c95a730b/result.json
@@ -0,0 +1,24 @@
+{
+  "bead_id": "ddx-4c5beab2",
+  "attempt_id": "20260506T175523-c95a730b",
+  "base_rev": "0c71367a099a861412116a968aaac4c135451cd1",
+  "result_rev": "5f5f2e76c7077f3f5991243c116d905c58bc5572",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-7ac80a54",
+  "duration_ms": 201610,
+  "tokens": 3191769,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T175523-c95a730b",
+  "prompt_file": ".ddx/executions/20260506T175523-c95a730b/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T175523-c95a730b/manifest.json",
+  "result_file": ".ddx/executions/20260506T175523-c95a730b/result.json",
+  "usage_file": ".ddx/executions/20260506T175523-c95a730b/usage.json",
+  "started_at": "2026-05-06T17:55:26.255751599Z",
+  "finished_at": "2026-05-06T17:58:47.866746017Z"
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
