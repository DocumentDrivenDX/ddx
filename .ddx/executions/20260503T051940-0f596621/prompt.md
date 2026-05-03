<bead-review>
  <bead id="ddx-65543530" iter=1>
    <title>runs: canonicalize Run data projection (fix state_runs.go bundle layering; lossless AgentSession join)</title>
    <description>
state_runs.go:164 currently synthesizes execution bundles as RunLayerRun; per the three-layer model bundles should be RunLayerTry. Fix the layer assignment. Add lossless join with AgentSession (which carries billingMode, cached tokens, prompt/response/stderr, outcome — NOT in Run today). User decision: keep AgentSession as backing store under layer=run.
    </description>
    <acceptance>
1. state_runs.go: bundles → RunLayerTry; agent sessions → RunLayerRun. 2. Run resolver returns lossless join (Run + AgentSession fields when applicable). 3. Existing runs tests pass; add new test for layering correctness. 4. cd cli &amp;&amp; go test ./internal/server/... passes.
    </acceptance>
    <notes>
REVIEW:BLOCK

Diff contains only an execution result.json artifact; no changes to state_runs.go, no resolver join with AgentSession, no new layering test. Cannot verify any AC item from this diff.

**`cachedTokens` is a dangling schema field.** `cli/internal/server/graphql/schema.graphql:1178` and `cli/internal/server/graphql/models.go` (Run.CachedTokens) declare the field, but `sessionEntryToRun` in `cli/internal/server/state_runs.go` never assigns it, and `agent.SessionIndexEntry` has no corresponding source field. Either add `CachedTokens` to `SessionIndexEntry` (sourced from harness usage) and project it in `sessionEntryToRun`, or drop the schema field until a backing source exists — shipping a non-nullable-looking attribute that is permanently null is a regression on the lossless-join contract.
**`prompt`/`response`/`stderr` not projected onto run-layer Runs.** The bead body lists these as AgentSession fields not present on Run today. `SessionIndexEntry` doesn't carry them, but the related `Session` struct in `cli/internal/agent/session_index.go:66-70` has `Prompt/Response/Stderr`. `sessionEntryToRun` should hydrate these (likely via the existing session lookup path used elsewhere) so a `run(id:)` query against a run-layer record is truly lossless against AgentSession.
**Test coverage gap matches the projection gap.** `state_runs_test.go` should additionally assert `cachedTokens` (or remove it) and `prompt`/`response`/`stderr` round-trip on the orphan-session case once those projections land.
    </notes>
    <labels>phase:2, story:8, area:server, kind:fix</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T021932-d0a2c34c/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="c51d8f7391d0c773f93cbce95eac51c8eba758ca">
diff --git a/.ddx/executions/20260503T021932-d0a2c34c/result.json b/.ddx/executions/20260503T021932-d0a2c34c/result.json
new file mode 100644
index 00000000..9b1752c3
--- /dev/null
+++ b/.ddx/executions/20260503T021932-d0a2c34c/result.json
@@ -0,0 +1,19 @@
+{
+  "bead_id": "ddx-65543530",
+  "attempt_id": "20260503T021932-d0a2c34c",
+  "base_rev": "a0ff44349f47b1088a265153ff658797f061c683",
+  "result_rev": "1b68b43b95abd9696e4e77faf46ffbb6e80caaba",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-44a4411d",
+  "duration_ms": 10802049,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T021932-d0a2c34c",
+  "prompt_file": ".ddx/executions/20260503T021932-d0a2c34c/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T021932-d0a2c34c/manifest.json",
+  "result_file": ".ddx/executions/20260503T021932-d0a2c34c/result.json",
+  "started_at": "2026-05-03T02:19:34.010051813Z",
+  "finished_at": "2026-05-03T05:19:36.060032115Z"
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
