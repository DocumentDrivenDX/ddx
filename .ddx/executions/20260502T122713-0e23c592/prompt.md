<bead-review>
  <bead id="ddx-f7b4bbc3" iter=1>
    <title>B2: Tighten execute-bead shared blocks; add guardrail-list comment + invariant/selector/missing-governing tests</title>
    <description>
Tighten the shared instruction blocks extracted by B1 for density per /tmp/story-12-final.md §B2. Step 0: prose -&gt; 4-bullet checklist + 4-step recipe (~120-&gt;~80w). Process: drop verification-command examples; keep these as separate explicit bullets — do not commit red code; commit subject ends with [&lt;bead-id&gt;]; git add &lt;specific-paths&gt; (never -A); commit exactly once when green; stop after commit; do not modify outside bead scope (~150-&gt;~85w). Review-gate: two bullets — review is a gate; address every BLOCKING &lt;review-findings&gt; item, do not declare no_changes with blocking findings open (~80-&gt;~45w). Bead-override: one sentence (~70-&gt;~25w). Constraints tail bullets including '.ddx/executions/ intact' and 'never ddx init' as separate items (~60-&gt;~40w). Add load-bearing-guardrail comment block above each constant per FEAT-022 amendment (full guardrail list in /tmp/story-12-final.md Diagnosis section). Selector at execute_bead.go:1343: agent/fiz/embedded -&gt; Agent variant; claude/codex/opencode/unknown -&gt; Claude variant — must be preserved.
    </description>
    <acceptance>
AC2: Rendered prompt for representative bead is &gt;=30% shorter (word count) than current for both Claude and Agent variants — assert with size-floor test. AC3: New test TestExecuteBeadInstructionsLoadBearingGuardrails asserts every guardrail listed in the file-level comment block appears in the rendered prompt for each (harness, variant); includes selector tests (agent/fiz/embedded -&gt; Agent; claude/codex/opencode/unknown -&gt; Claude). AC4: New invariant tests assert rendered prompts contain [&lt;bead-id&gt;], git add &lt;specific-paths&gt;, NO 'git add -A', no_changes_rationale.txt, .ddx/executions/, ddx bead create, ddx bead dep add, ddx bead update. AC5: New test asserts non-minimal prompts include executeBeadMissingGoverningText; contextBudget == 'minimal' omits governing refs. AC6: All existing prompt tests pass unchanged. AC10: No new per-harness branches beyond Claude/Agent.
    </acceptance>
    <labels>phase:2, story:12, tier:cheap</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T121655-038c230f/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="4e6e0e23c37e30be9a08731602a404aff54fef6e">
diff --git a/.ddx/executions/20260502T121655-038c230f/result.json b/.ddx/executions/20260502T121655-038c230f/result.json
new file mode 100644
index 00000000..e5f9bc79
--- /dev/null
+++ b/.ddx/executions/20260502T121655-038c230f/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-f7b4bbc3",
+  "attempt_id": "20260502T121655-038c230f",
+  "base_rev": "63bb61948a4ccc6e164f532ea75a5cd28d530ecf",
+  "result_rev": "c3a88234e6ffa51afc8938a1321f4ee633c51827",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-e556e04e",
+  "duration_ms": 611932,
+  "tokens": 30271,
+  "cost_usd": 2.9825862499999998,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T121655-038c230f",
+  "prompt_file": ".ddx/executions/20260502T121655-038c230f/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T121655-038c230f/manifest.json",
+  "result_file": ".ddx/executions/20260502T121655-038c230f/result.json",
+  "usage_file": ".ddx/executions/20260502T121655-038c230f/usage.json",
+  "started_at": "2026-05-02T12:16:56.767628284Z",
+  "finished_at": "2026-05-02T12:27:08.700306476Z"
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
