<bead-review>
  <bead id="ddx-df8b992f" iter=1>
    <title>Story 6 B4b: extend GraphQL search to metadata fields + add ArtifactEdge.snippet + resolver tests</title>
    <description>
Implement metadata + body search per TD-NNN. Depends on B4a.

Changes:
- resolver_artifacts.go: extend Query.artifacts 'search' to scan description and ddxFrontmatter JSON in addition to title+path (Phase 2 of plan). Then add textual-body grep with the size cap and binary skip rule from TD-NNN.
- schema.graphql: add 'snippet: String' field on ArtifactEdge, reusing the contract from existing SearchResult.snippet (panic-free implementation; do not invent a new shape).
- Resolver populates snippet with the matching context window (with match terms marked, e.g. via existing scoring helper in resolver_feat008.go if reusable).
- Tests: resolver_artifacts_test.go covers title/path/description/frontmatter/body matches; snippet rendering correctness; pagination correctness across filtered+sorted+searched set; deterministic tie-breaker ordering.
    </description>
    <acceptance>
- search arg matches title, path, description, ddxFrontmatter JSON, and body text (per TD-NNN precedence).
- ArtifactEdge.snippet returned for matches; uses the SearchResult.snippet contract.
- Size cap and binary skip enforced (test with oversize fixture and binary fixture).
- Pagination remains deterministic under filter+sort+search composition.
- 'cd cli &amp;&amp; go test -v ./internal/server/graphql/...' passes.
    </acceptance>
    <labels>phase:2,  story:6</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T144856-628a40db/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="668cfafaab9450149cb9cfb668deb77a045dd03b">
diff --git a/.ddx/executions/20260502T144856-628a40db/result.json b/.ddx/executions/20260502T144856-628a40db/result.json
new file mode 100644
index 00000000..9aef3c0e
--- /dev/null
+++ b/.ddx/executions/20260502T144856-628a40db/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-df8b992f",
+  "attempt_id": "20260502T144856-628a40db",
+  "base_rev": "de2d673a38e6cb4c4ba98266f88e589d842036de",
+  "result_rev": "90296f0d98cf718b1a7ee94608679466c9a5a072",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-40c8dec2",
+  "duration_ms": 385635,
+  "tokens": 24496,
+  "cost_usd": 3.6715442499999997,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T144856-628a40db",
+  "prompt_file": ".ddx/executions/20260502T144856-628a40db/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T144856-628a40db/manifest.json",
+  "result_file": ".ddx/executions/20260502T144856-628a40db/result.json",
+  "usage_file": ".ddx/executions/20260502T144856-628a40db/usage.json",
+  "started_at": "2026-05-02T14:48:57.624306335Z",
+  "finished_at": "2026-05-02T14:55:23.259472789Z"
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
