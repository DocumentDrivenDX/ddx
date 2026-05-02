<bead-review>
  <bead id="ddx-d26f65b2" iter=1>
    <title>Story 6 B3: publish facet contract (TD-NNN) + add phase/prefix filter args + chips wired to Story 5 axes</title>
    <description>
Publish facet contract and add Story-5 axes as filters. Depends on Story 5 final + B2.

Changes:
- docs: create TD-NNN technical-decision document (artifact full-text search &amp; facet contract). Pin URL keys: q, mediaType (single), staleness (single), phase (single), prefix (multi), sort (single). Document deterministic (sortKey, id) ordering. Document Phase axis source: path prefix docs/helix/NN-*/. Document Prefix axis source: id prefix segment (ADR|SD|FEAT|US|RSCH|...). Note that filter narrows; Story 5 grouping organizes the remaining set.
- schema.graphql: add 'phase: String' and 'prefix: [String!]' args on Query.artifacts.
- resolver_artifacts.go: implement phase filter (path prefix match) and multi-value prefix filter (id-prefix segment match).
- Frontend: phase chip + prefix multi-chip wired to URL keys. Compose with existing q/mediaType/staleness/sort.

This bead must NOT proceed until Story 5 categorization axes are finalized and B2 has landed.
    </description>
    <acceptance>
- TD-NNN document committed with URL key reservations and source-of-truth definitions for phase/prefix.
- GraphQL args 'phase' and 'prefix' present and filter results correctly (resolver tests).
- Frontend phase chip and prefix multi-chip persist to URL and compose with other facets (vitest + playwright).
- Story 5 grouping consumes the same axis definitions without renaming.
- All Go and frontend tests pass.
    </acceptance>
    <labels>phase:2,  story:6</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T142942-97921dd9/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="a6ffde44055156aa3f4f61efec10fc59ee458031">
diff --git a/.ddx/executions/20260502T142942-97921dd9/result.json b/.ddx/executions/20260502T142942-97921dd9/result.json
new file mode 100644
index 00000000..964a5b07
--- /dev/null
+++ b/.ddx/executions/20260502T142942-97921dd9/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-d26f65b2",
+  "attempt_id": "20260502T142942-97921dd9",
+  "base_rev": "62b2785982b733f18296cf275601e53bad2ea200",
+  "result_rev": "50346ab1db29a7ca6dec1cddd0f3182623956ae0",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-7d7fcebf",
+  "duration_ms": 562915,
+  "tokens": 31960,
+  "cost_usd": 5.23973725,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T142942-97921dd9",
+  "prompt_file": ".ddx/executions/20260502T142942-97921dd9/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T142942-97921dd9/manifest.json",
+  "result_file": ".ddx/executions/20260502T142942-97921dd9/result.json",
+  "usage_file": ".ddx/executions/20260502T142942-97921dd9/usage.json",
+  "started_at": "2026-05-02T14:29:44.025409919Z",
+  "finished_at": "2026-05-02T14:39:06.940722704Z"
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
