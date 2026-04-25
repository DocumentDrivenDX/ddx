<bead-review>
  <bead id="ddx-021bd69b" iter=1>
    <title>review: machine-readable JSON verdict contract + replace markdown extractor</title>
    <description>
Upstream report ("ddx execute-loop: post-merge reviewer silently fails on large prompts", suggested fix #5) flags that even when the reviewer returns content, the markdown verdict extractor mis-parses it: when Claude responds with '### Verdict: APPROVE' the extractor pulls 'BLOCK' from the options header line in the prompt template that the model echoes back, logging '{"verdict":"BLOCK"}' for an actual APPROVE. This silently lets regressions land.

FEAT-022 (parent ddx-0c35470e) is scoped to prompt assembly and size invariants; response parsing is out of scope there. File this as a sibling.

Switch the reviewer contract to a machine-readable JSON object emitted as the model's final response:

  {
    "verdict": "APPROVE" | "REQUEST_CHANGES" | "BLOCK",
    "findings": [
      { "severity": "info"|"warn"|"block", "summary": "…", "location": "path:line?" }
    ],
    "summary": "≤300 char human-readable verdict justification"
  }

Replace the existing markdown extractor with a strict JSON parser:
1. Update the reviewer prompt template (post-collapse: agent.BuildReviewPrompt) to instruct the model to emit ONLY a JSON object matching the schema, fenced as ```json … ``` for robustness against models that won't emit raw JSON.
2. Add internal/agent/review_verdict.go with a typed ReviewVerdict struct and a ParseReviewVerdict([]byte) (ReviewVerdict, error) that strips an optional ```json fence and decodes; reject on unknown verdict values.
3. Replace the regex-based extractor call site in execute_bead_review.go with ParseReviewVerdict; on parse error, emit the existing 'unparseable' review-error class (introduced by ddx-70c1d2e2) — do NOT close the bead.
4. Schema-version the contract: include schema_version: 1 in the prompt; the parser tolerates unknown fields but rejects unknown verdict values.

In-scope files:
- cli/internal/agent/execute_bead_review.go (prompt template, extractor call site)
- cli/internal/agent/review_verdict.go (new)
- cli/internal/agent/review_verdict_test.go (new)
- prompt template asset under cli/internal/agent/assets/ if separated

Out of scope:
- Bounded retry / error-class taxonomy (ddx-70c1d2e2 owns this; this bead emits 'unparseable' into that taxonomy)
- Prompt size invariants (FEAT-022)
- Grading verdicts (separate bead if needed; this is review-only)

Cross-reference: This bead's `unparseable` emission feeds into ddx-70c1d2e2's bounded-retry logic. ddx-70c1d2e2 should be reviewed once both land to confirm the unparseable retry budget is sane (the upstream report observed real APPROVE verdicts being lost, so unparseable should retry, not terminal-escalate immediately).
    </description>
    <acceptance>
1. New cli/internal/agent/review_verdict.go defines ReviewVerdict struct with Verdict (string, enum-validated), Findings ([]Finding), Summary (string), and SchemaVersion (int) fields.
2. ParseReviewVerdict accepts: raw JSON; ```json …``` fenced JSON; mixed prose+JSON where the JSON is the last fenced block. Rejects: unknown verdict values; missing verdict field; truncated JSON.
3. cd cli &amp;&amp; go test -run TestParseReviewVerdict ./internal/agent/... passes with one subtest per accepted/rejected case above.
4. cd cli &amp;&amp; go test -run TestReviewerEmitsJSONContract ./internal/agent/... passes: uses a fixture reviewer transcript with '### Verdict: APPROVE' wrapped markdown and asserts the extractor call site rejects it with the 'unparseable' class — confirming the markdown path is gone.
5. cd cli &amp;&amp; go test -run TestReviewerJSONHappyPath ./internal/agent/... passes: a fixture transcript containing the JSON contract parses to a ReviewVerdict with verdict=APPROVE.
6. The reviewer prompt template (in execute_bead_review.go or its asset file) requests JSON-only output and includes a schema example; cd cli &amp;&amp; go test -run TestReviewPromptRequestsJSON ./internal/agent/... passes by string-matching the template.
7. The previously-observed regression — extractor parsing 'BLOCK' from the options header line — has a regression test: cd cli &amp;&amp; go test -run TestReviewerNoOptionsHeaderRegression ./internal/agent/... feeds a transcript that includes only the prompt template's options header echoed back and asserts the parser rejects it (unparseable), not parses it as BLOCK.
    </acceptance>
    <labels>ddx, kind:implementation, area:agent, area:review</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="ed2219bf951ce9b7770ed684fb6c789a51b3344d">
commit ed2219bf951ce9b7770ed684fb6c789a51b3344d
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 25 05:39:11 2026 -0400

    chore: add execution evidence [20260425T092410-]

diff --git a/.ddx/executions/20260425T092410-e9991cad/manifest.json b/.ddx/executions/20260425T092410-e9991cad/manifest.json
new file mode 100644
index 00000000..8ca8ec40
--- /dev/null
+++ b/.ddx/executions/20260425T092410-e9991cad/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260425T092410-e9991cad",
+  "bead_id": "ddx-021bd69b",
+  "base_rev": "7820e2fc78c7c199e63f9d6aea898bf4cf506313",
+  "created_at": "2026-04-25T09:24:11.446777923Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-021bd69b",
+    "title": "review: machine-readable JSON verdict contract + replace markdown extractor",
+    "description": "Upstream report (\"ddx execute-loop: post-merge reviewer silently fails on large prompts\", suggested fix #5) flags that even when the reviewer returns content, the markdown verdict extractor mis-parses it: when Claude responds with '### Verdict: APPROVE' the extractor pulls 'BLOCK' from the options header line in the prompt template that the model echoes back, logging '{\"verdict\":\"BLOCK\"}' for an actual APPROVE. This silently lets regressions land.\n\nFEAT-022 (parent ddx-0c35470e) is scoped to prompt assembly and size invariants; response parsing is out of scope there. File this as a sibling.\n\nSwitch the reviewer contract to a machine-readable JSON object emitted as the model's final response:\n\n  {\n    \"verdict\": \"APPROVE\" | \"REQUEST_CHANGES\" | \"BLOCK\",\n    \"findings\": [\n      { \"severity\": \"info\"|\"warn\"|\"block\", \"summary\": \"…\", \"location\": \"path:line?\" }\n    ],\n    \"summary\": \"≤300 char human-readable verdict justification\"\n  }\n\nReplace the existing markdown extractor with a strict JSON parser:\n1. Update the reviewer prompt template (post-collapse: agent.BuildReviewPrompt) to instruct the model to emit ONLY a JSON object matching the schema, fenced as ```json … ``` for robustness against models that won't emit raw JSON.\n2. Add internal/agent/review_verdict.go with a typed ReviewVerdict struct and a ParseReviewVerdict([]byte) (ReviewVerdict, error) that strips an optional ```json fence and decodes; reject on unknown verdict values.\n3. Replace the regex-based extractor call site in execute_bead_review.go with ParseReviewVerdict; on parse error, emit the existing 'unparseable' review-error class (introduced by ddx-70c1d2e2) — do NOT close the bead.\n4. Schema-version the contract: include schema_version: 1 in the prompt; the parser tolerates unknown fields but rejects unknown verdict values.\n\nIn-scope files:\n- cli/internal/agent/execute_bead_review.go (prompt template, extractor call site)\n- cli/internal/agent/review_verdict.go (new)\n- cli/internal/agent/review_verdict_test.go (new)\n- prompt template asset under cli/internal/agent/assets/ if separated\n\nOut of scope:\n- Bounded retry / error-class taxonomy (ddx-70c1d2e2 owns this; this bead emits 'unparseable' into that taxonomy)\n- Prompt size invariants (FEAT-022)\n- Grading verdicts (separate bead if needed; this is review-only)\n\nCross-reference: This bead's `unparseable` emission feeds into ddx-70c1d2e2's bounded-retry logic. ddx-70c1d2e2 should be reviewed once both land to confirm the unparseable retry budget is sane (the upstream report observed real APPROVE verdicts being lost, so unparseable should retry, not terminal-escalate immediately).",
+    "acceptance": "1. New cli/internal/agent/review_verdict.go defines ReviewVerdict struct with Verdict (string, enum-validated), Findings ([]Finding), Summary (string), and SchemaVersion (int) fields.\n2. ParseReviewVerdict accepts: raw JSON; ```json …``` fenced JSON; mixed prose+JSON where the JSON is the last fenced block. Rejects: unknown verdict values; missing verdict field; truncated JSON.\n3. cd cli \u0026\u0026 go test -run TestParseReviewVerdict ./internal/agent/... passes with one subtest per accepted/rejected case above.\n4. cd cli \u0026\u0026 go test -run TestReviewerEmitsJSONContract ./internal/agent/... passes: uses a fixture reviewer transcript with '### Verdict: APPROVE' wrapped markdown and asserts the extractor call site rejects it with the 'unparseable' class — confirming the markdown path is gone.\n5. cd cli \u0026\u0026 go test -run TestReviewerJSONHappyPath ./internal/agent/... passes: a fixture transcript containing the JSON contract parses to a ReviewVerdict with verdict=APPROVE.\n6. The reviewer prompt template (in execute_bead_review.go or its asset file) requests JSON-only output and includes a schema example; cd cli \u0026\u0026 go test -run TestReviewPromptRequestsJSON ./internal/agent/... passes by string-matching the template.\n7. The previously-observed regression — extractor parsing 'BLOCK' from the options header line — has a regression test: cd cli \u0026\u0026 go test -run TestReviewerNoOptionsHeaderRegression ./internal/agent/... feeds a transcript that includes only the prompt template's options header echoed back and asserts the parser rejects it (unparseable), not parses it as BLOCK.",
+    "labels": [
+      "ddx",
+      "kind:implementation",
+      "area:agent",
+      "area:review"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-25T09:24:10Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "execute-loop-heartbeat-at": "2026-04-25T09:24:10.972399113Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260425T092410-e9991cad",
+    "prompt": ".ddx/executions/20260425T092410-e9991cad/prompt.md",
+    "manifest": ".ddx/executions/20260425T092410-e9991cad/manifest.json",
+    "result": ".ddx/executions/20260425T092410-e9991cad/result.json",
+    "checks": ".ddx/executions/20260425T092410-e9991cad/checks.json",
+    "usage": ".ddx/executions/20260425T092410-e9991cad/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-021bd69b-20260425T092410-e9991cad"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260425T092410-e9991cad/result.json b/.ddx/executions/20260425T092410-e9991cad/result.json
new file mode 100644
index 00000000..e17c9978
--- /dev/null
+++ b/.ddx/executions/20260425T092410-e9991cad/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-021bd69b",
+  "attempt_id": "20260425T092410-e9991cad",
+  "base_rev": "7820e2fc78c7c199e63f9d6aea898bf4cf506313",
+  "result_rev": "25da0945769a20d630744999dcc633f2be0e8258",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-b2bae304",
+  "duration_ms": 899008,
+  "tokens": 43740,
+  "cost_usd": 4.689253000000002,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260425T092410-e9991cad",
+  "prompt_file": ".ddx/executions/20260425T092410-e9991cad/prompt.md",
+  "manifest_file": ".ddx/executions/20260425T092410-e9991cad/manifest.json",
+  "result_file": ".ddx/executions/20260425T092410-e9991cad/result.json",
+  "usage_file": ".ddx/executions/20260425T092410-e9991cad/usage.json",
+  "started_at": "2026-04-25T09:24:11.44705705Z",
+  "finished_at": "2026-04-25T09:39:10.45603671Z"
+}
\ No newline at end of file
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

## Your task

Examine the diff and each acceptance-criteria (AC) item. For each item assign one grade:

- **APPROVE** — fully and correctly implemented; cite the specific file path and line that proves it.
- **REQUEST_CHANGES** — partially implemented or has fixable minor issues.
- **BLOCK** — not implemented, incorrectly implemented, or the diff is insufficient to evaluate.

Overall verdict rule:
- All items APPROVE → **APPROVE**
- Any item BLOCK → **BLOCK**
- Otherwise → **REQUEST_CHANGES**

## Required output format

Respond with a structured review using exactly this layout (replace placeholder text):

---
## Review: ddx-021bd69b iter 1

### Verdict: APPROVE | REQUEST_CHANGES | BLOCK

### AC Grades

| # | Item | Grade | Evidence |
|---|------|-------|----------|
| 1 | &lt;AC item text, max 60 chars&gt; | APPROVE | path/to/file.go:42 — brief note |
| 2 | &lt;AC item text, max 60 chars&gt; | BLOCK   | — not found in diff |

### Summary

&lt;1–3 sentences on overall implementation quality and any recurring theme in findings.&gt;

### Findings

&lt;Bullet list of REQUEST_CHANGES and BLOCK findings. Each finding must name the specific file, function, or test that is missing or wrong — specific enough for the next agent to act on without re-reading the entire diff. Omit this section entirely if verdict is APPROVE.&gt;
  </instructions>
</bead-review>
