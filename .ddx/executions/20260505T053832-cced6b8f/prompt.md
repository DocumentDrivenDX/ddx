<bead-review>
  <bead id="ddx-e7b80a50" iter=1>
    <title>review: server session schema/store/lifecycle + manifest+turns.jsonl evidence contract</title>
    <description>
PROBLEM
No server-side ReviewSession model, persistence layer, or schema exists. Story 18 review mutations and the Story 16 run-detail page both depend on a well-defined review session schema and persistence contract. Without this, both are blocked.

ROOT CAUSE
- cli/internal/server/ contains no ReviewSession struct or persistence layer.
- .ddx/reviews/ directory does not exist as a managed path (no mkdir, no lifecycle helpers).
- The Story 16 page (ddx-4cd64068) needs to read review turns from .ddx/reviews/&lt;session-id&gt;/turns.jsonl; the contract must be agreed before both 16 and 18 can land without merge conflicts.
- ddx-cd2ecf79 (artifact type spec, a dep) must land first for {template, review_prompt, preferred_reviewer} fields.

PROPOSED FIX
- Define ReviewSession struct (fields: id, artifact_id, artifact_sha, artifact_git_rev, system_rubric, template_ref, prompt_ref, status, turns[], cost_usd, max_billable_usd) in cli/internal/server/review_session.go.
- Implement persistence under .ddx/reviews/&lt;session-id&gt;/:
  - manifest.json: session metadata (id, status, cost, refs).
  - turns.jsonl: append-only JSONL of turn records (actor, content, cost_usd, created_at).
  - attachments/: binary attachments (if any).
- Document the manifest.json + turns.jsonl contract explicitly as the Story 16 contract.

NON-SCOPE
- GraphQL mutations (ddx-de278b8d).
- Turn dispatcher (ddx-f7c3d512).
- Frontend rendering.
    </description>
    <acceptance>
1. ReviewSession struct defined in cli/internal/server/review_session.go with all required fields.
2. .ddx/reviews/&lt;session-id&gt;/manifest.json written on session create with correct schema.
3. .ddx/reviews/&lt;session-id&gt;/turns.jsonl written as append-only JSONL.
4. manifest.json + turns.jsonl format documented in source (comment block) as the Story 16 contract.
5. TestReviewSession_CreatePersistsManifest verifies manifest.json written with correct fields.
6. TestReviewSession_AppendTurn_TurnsJsonl verifies turn appended to turns.jsonl.
7. TestReviewSession_RoundTrip verifies schema deserialization round-trip.
8. cd cli &amp;&amp; go test ./internal/server/... green.
9. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, story:18, area:server, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T053132-343e05eb/checks/production-reachability.json</file>
    <file>.ddx/executions/20260505T053132-343e05eb/manifest.json</file>
    <file>.ddx/executions/20260505T053132-343e05eb/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="4d8af63c9e62459f1c0c448c3eca40e245f01ed1">
diff --git a/.ddx/executions/20260505T053132-343e05eb/manifest.json b/.ddx/executions/20260505T053132-343e05eb/manifest.json
new file mode 100644
index 00000000..ec369f1a
--- /dev/null
+++ b/.ddx/executions/20260505T053132-343e05eb/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260505T053132-343e05eb",
+  "bead_id": "ddx-e7b80a50",
+  "base_rev": "27d22cada34cbe9f895f0ca9d778e053a74b4bea",
+  "created_at": "2026-05-05T05:31:34.982859409Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-e7b80a50",
+    "title": "review: server session schema/store/lifecycle + manifest+turns.jsonl evidence contract",
+    "description": "PROBLEM\nNo server-side ReviewSession model, persistence layer, or schema exists. Story 18 review mutations and the Story 16 run-detail page both depend on a well-defined review session schema and persistence contract. Without this, both are blocked.\n\nROOT CAUSE\n- cli/internal/server/ contains no ReviewSession struct or persistence layer.\n- .ddx/reviews/ directory does not exist as a managed path (no mkdir, no lifecycle helpers).\n- The Story 16 page (ddx-4cd64068) needs to read review turns from .ddx/reviews/\u003csession-id\u003e/turns.jsonl; the contract must be agreed before both 16 and 18 can land without merge conflicts.\n- ddx-cd2ecf79 (artifact type spec, a dep) must land first for {template, review_prompt, preferred_reviewer} fields.\n\nPROPOSED FIX\n- Define ReviewSession struct (fields: id, artifact_id, artifact_sha, artifact_git_rev, system_rubric, template_ref, prompt_ref, status, turns[], cost_usd, max_billable_usd) in cli/internal/server/review_session.go.\n- Implement persistence under .ddx/reviews/\u003csession-id\u003e/:\n  - manifest.json: session metadata (id, status, cost, refs).\n  - turns.jsonl: append-only JSONL of turn records (actor, content, cost_usd, created_at).\n  - attachments/: binary attachments (if any).\n- Document the manifest.json + turns.jsonl contract explicitly as the Story 16 contract.\n\nNON-SCOPE\n- GraphQL mutations (ddx-de278b8d).\n- Turn dispatcher (ddx-f7c3d512).\n- Frontend rendering.",
+    "acceptance": "1. ReviewSession struct defined in cli/internal/server/review_session.go with all required fields.\n2. .ddx/reviews/\u003csession-id\u003e/manifest.json written on session create with correct schema.\n3. .ddx/reviews/\u003csession-id\u003e/turns.jsonl written as append-only JSONL.\n4. manifest.json + turns.jsonl format documented in source (comment block) as the Story 16 contract.\n5. TestReviewSession_CreatePersistsManifest verifies manifest.json written with correct fields.\n6. TestReviewSession_AppendTurn_TurnsJsonl verifies turn appended to turns.jsonl.\n7. TestReviewSession_RoundTrip verifies schema deserialization round-trip.\n8. cd cli \u0026\u0026 go test ./internal/server/... green.\n9. lefthook run pre-commit passes.",
+    "parent": "ddx-42b917fe",
+    "labels": [
+      "phase:2",
+      "story:18",
+      "area:server",
+      "kind:feature"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T05:31:32Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
+      "execute-loop-heartbeat-at": "2026-05-05T05:31:32.573449751Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T053132-343e05eb",
+    "prompt": ".ddx/executions/20260505T053132-343e05eb/prompt.md",
+    "manifest": ".ddx/executions/20260505T053132-343e05eb/manifest.json",
+    "result": ".ddx/executions/20260505T053132-343e05eb/result.json",
+    "checks": ".ddx/executions/20260505T053132-343e05eb/checks.json",
+    "usage": ".ddx/executions/20260505T053132-343e05eb/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-e7b80a50-20260505T053132-343e05eb"
+  },
+  "prompt_sha": "45adfc0f06aadad91613fbf7f0186b52806807c0b90bd62168908fadb368a327"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T053132-343e05eb/checks/production-reachability.json b/.ddx/executions/20260505T053132-343e05eb/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260505T053132-343e05eb/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T053132-343e05eb/result.json b/.ddx/executions/20260505T053132-343e05eb/result.json
new file mode 100644
index 00000000..d26f37ab
--- /dev/null
+++ b/.ddx/executions/20260505T053132-343e05eb/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-e7b80a50",
+  "attempt_id": "20260505T053132-343e05eb",
+  "base_rev": "27d22cada34cbe9f895f0ca9d778e053a74b4bea",
+  "result_rev": "a48e8cf759a3df2096dd005cda1e69b6f0d6d5bc",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-9d3eaa4a",
+  "duration_ms": 410997,
+  "tokens": 5035764,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T053132-343e05eb",
+  "prompt_file": ".ddx/executions/20260505T053132-343e05eb/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T053132-343e05eb/manifest.json",
+  "result_file": ".ddx/executions/20260505T053132-343e05eb/result.json",
+  "usage_file": ".ddx/executions/20260505T053132-343e05eb/usage.json",
+  "started_at": "2026-05-05T05:31:34.983172284Z",
+  "finished_at": "2026-05-05T05:38:25.980260391Z"
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
