<bead-review>
  <bead id="ddx-8ec3e08e" iter=1>
    <title>Story 6 B1: wire frontend q to server search, fix mediaType wildcard, add sort enum + staleness arg</title>
    <description>
Phase 1 plumbing for Story 6 artifact listing. See /tmp/story-6-final.md.

Changes:
- cli/internal/server/graphql/resolver_artifacts.go: translate 'image/*'-style mediaType arg to a prefix match (HasPrefix on 'image/'); current code uses exact equality so the Images chip never matches image/png or image/svg+xml.
- cli/internal/server/graphql/schema.graphql: add 'sort: ArtifactSort' arg on Query.artifacts with enum values PATH_ASC|PATH_DESC|TITLE_ASC|TITLE_DESC|UPDATED_DESC|UPDATED_ASC|STALENESS, plus 'staleness: String' arg.
- Resolver: implement sort with deterministic (sortKey, id) tie-breaker; implement staleness filter; keep existing search (title+path substring, case-insensitive) unchanged in this bead.
- Frontend routes/nodes/[nodeId]/projects/[projectId]/artifacts/+page.ts: pass URL 'q' as the GraphQL 'search' arg instead of client-side substring filter on already-loaded edges. Drop the client-side fallback so search covers the full set.
- Frontend +page.svelte: when q/sort/mediaType/staleness change, reset 'after' cursor before reloading.

Out of scope: sort dropdown UI, staleness chip UI, URL state for new params, snippet rendering, body-text search, phase/prefix filters.
    </description>
    <acceptance>
- mediaType='image/*' returns image/png and image/svg+xml artifacts (resolver test).
- ArtifactSort enum present in schema; each value sorts deterministically with id tie-breaker (resolver test).
- staleness filter narrows result set (resolver test).
- Frontend artifacts page calls GraphQL with 'search' arg from URL 'q' (no client-side filter remaining).
- Changing q/sort/mediaType/staleness clears 'after' cursor.
- 'cd cli &amp;&amp; go test -v ./internal/server/graphql/...' passes.
- 'cd cli/internal/server/frontend &amp;&amp; bun run test' passes.
    </acceptance>
    <labels>phase:2,  story:6</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T133823-f8df7c6d/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="9978c5ac1e8411afe59db12cb25d19afa36ecea8">
diff --git a/.ddx/executions/20260502T133823-f8df7c6d/result.json b/.ddx/executions/20260502T133823-f8df7c6d/result.json
new file mode 100644
index 00000000..fbeb24bb
--- /dev/null
+++ b/.ddx/executions/20260502T133823-f8df7c6d/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-8ec3e08e",
+  "attempt_id": "20260502T133823-f8df7c6d",
+  "base_rev": "2675f38be08c65dc6312d6c527c50c24f22245ec",
+  "result_rev": "16aaf8b21f5a05e0a22e203bb0d6b68a0aa6bc0e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-fc2439d2",
+  "duration_ms": 332799,
+  "tokens": 13466,
+  "cost_usd": 2.17845775,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T133823-f8df7c6d",
+  "prompt_file": ".ddx/executions/20260502T133823-f8df7c6d/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T133823-f8df7c6d/manifest.json",
+  "result_file": ".ddx/executions/20260502T133823-f8df7c6d/result.json",
+  "usage_file": ".ddx/executions/20260502T133823-f8df7c6d/usage.json",
+  "started_at": "2026-05-02T13:38:25.071084446Z",
+  "finished_at": "2026-05-02T13:43:57.871042785Z"
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
