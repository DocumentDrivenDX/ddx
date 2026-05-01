<bead-review>
  <bead id="ddx-778fcfd1" iter=1>
    <title>FEAT-008 P7: US-086a worker live stream + tool cards + reconnect</title>
    <description>
Subscribe to workerProgress via graphql-ws. Live region is a NAMED &lt;section role=region aria-label='Live response'&gt; containing an aria-live=polite inner log — bare role=log does NOT satisfy the test. Contiguous text_delta frames MUST concatenate into readable text in the DOM (two adjacent frames render as joined text, not separate nodes). tool_call frames → collapsible &lt;details&gt; cards interleaved with text. On reconnect: fetch worker(id).recentEvents to catch up; show reconnect banner while bridging. Terminal state: stop appending, render completed timestamp + 'Evidence bundle' link (link must be present even if mocked evidenceBundleUrl is missing — use worker id to synthesize the path).
    </description>
    <acceptance>
US-086a.a-c green in e2e/workers.spec.ts.
    </acceptance>
    <labels>ui, feat-008, phase-7, us-086a</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="385f3c95b0fec8f44c71dfe8d49a44db5e11acdc">
commit 385f3c95b0fec8f44c71dfe8d49a44db5e11acdc
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 04:55:28 2026 -0400

    chore: add execution evidence [20260422T083933-]

diff --git a/.ddx/executions/20260422T083933-f1906040/result.json b/.ddx/executions/20260422T083933-f1906040/result.json
new file mode 100644
index 00000000..733b648a
--- /dev/null
+++ b/.ddx/executions/20260422T083933-f1906040/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-778fcfd1",
+  "attempt_id": "20260422T083933-f1906040",
+  "base_rev": "057a7ee6398387fc48cdfad588760ebd19d17463",
+  "result_rev": "0c9fc9077f2dd86dad44887c4b0ca4c12789364e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-3bce4ab6",
+  "duration_ms": 953198,
+  "tokens": 12463857,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260422T083933-f1906040",
+  "prompt_file": ".ddx/executions/20260422T083933-f1906040/prompt.md",
+  "manifest_file": ".ddx/executions/20260422T083933-f1906040/manifest.json",
+  "result_file": ".ddx/executions/20260422T083933-f1906040/result.json",
+  "usage_file": ".ddx/executions/20260422T083933-f1906040/usage.json",
+  "started_at": "2026-04-22T08:39:34.140750111Z",
+  "finished_at": "2026-04-22T08:55:27.338924394Z"
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
## Review: ddx-778fcfd1 iter 1

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
