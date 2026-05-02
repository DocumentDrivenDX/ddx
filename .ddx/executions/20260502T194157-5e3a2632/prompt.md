<bead-review>
  <bead id="ddx-861ad6a9" iter=1>
    <title>CI guards: 4 tests for state-machine drift prevention (schema↔Go-constants, status-literal audit, TD-doc↔schema, schema_compat round-trip)</title>
    <description>
Sibling of ddx-673833f4. After the TD + reconciliation bead land, this bead adds 4 CI guard tests that prevent future state-machine drift.

Each test goes in cli/internal/bead/ or a dedicated cli/internal/bead/sync_test.go file:

1. **Schema enum ↔ Go status constants synchronized**: parse cli/internal/bead/schema/bead-record.schema.json status enum; assert matches the exported canonical Status list in cli/internal/bead/types.go. CI fails on mismatch.

2. **Status-literal audit**: scan cli/internal/ Go source files for string literals at persisted-status assignment sites (bead.Status = '...' patterns); assert every such literal is in the canonical list. Catches drift where someone writes 'needs_investigation' as a status string instead of using the typed constant or a label.

3. **TD doc ↔ schema enum synchronized**: parse docs/helix/02-design/technical-designs/TD-NNN-bead-state-machine.md persisted-status section; assert it lists exactly the schema enum. Forces TD amendment when schema changes.

4. **schema_compat_test.go round-trip**: take a fresh bd-export fixture (DDx-side fixture file under cli/internal/bead/testdata/ if not already present); export → import via DDx; assert the round-trip preserves all fields including new labels/events. ADR-004 compatibility regression guard.

Each test must be deterministic, fast (&lt;1s each), and have a clear failure message that points the developer at the exact mismatch location.
    </description>
    <acceptance>
1. All 4 CI guard tests exist as named test functions. 2. Each fails with a clear message when its respective drift is introduced (verified by deliberately breaking each in a throwaway commit, observing the failure, then reverting). 3. cd cli &amp;&amp; go test ./internal/bead/... runs all 4 in &lt;5s total. 4. CI workflow includes these tests (no special opt-in flag). 5. README or test-doc comment explains the role of each guard.
    </acceptance>
    <labels>phase:2, story:10, area:tests, area:ci, kind:guardrail</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T193411-c46ce0f7/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="82acb4f6d99abca7e6f1cb0b9185f4a2ef3743ae">
diff --git a/.ddx/executions/20260502T193411-c46ce0f7/result.json b/.ddx/executions/20260502T193411-c46ce0f7/result.json
new file mode 100644
index 00000000..889d0230
--- /dev/null
+++ b/.ddx/executions/20260502T193411-c46ce0f7/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-861ad6a9",
+  "attempt_id": "20260502T193411-c46ce0f7",
+  "base_rev": "496781e6843ae20f7a5433bd9583933c9542732d",
+  "result_rev": "6a005ea4607c186bf5f243f5e795c13e84e9c696",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-aaa0960a",
+  "duration_ms": 456908,
+  "tokens": 26273,
+  "cost_usd": 2.35275775,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T193411-c46ce0f7",
+  "prompt_file": ".ddx/executions/20260502T193411-c46ce0f7/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T193411-c46ce0f7/manifest.json",
+  "result_file": ".ddx/executions/20260502T193411-c46ce0f7/result.json",
+  "usage_file": ".ddx/executions/20260502T193411-c46ce0f7/usage.json",
+  "started_at": "2026-05-02T19:34:12.966419719Z",
+  "finished_at": "2026-05-02T19:41:49.875136007Z"
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
