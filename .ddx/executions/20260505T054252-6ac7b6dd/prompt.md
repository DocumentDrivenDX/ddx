<bead-review>
  <bead id="ddx-cb63cdfc" iter=1>
    <title>Migration tool: .ddx/beads.jsonl + archive → axon backend</title>
    <description>
PROBLEM
Beads stored in .ddx/beads.jsonl and .ddx/beads-archive.jsonl cannot be migrated to the axon backend without a dedicated migration command. Operators switching to the axon backend must manually reconstruct state or risk data loss.

ROOT CAUSE
- cli/cmd/ does not contain a 'ddx bead migrate' subcommand or --to flag.
- cli/internal/bead/ has backend_jsonl.go (JSONL reader) but no migration path to write into the axon Backend interface (ddx-95ec5ed5, a dep).
- Without an idempotent migration, re-running risks duplicates; without a round-trip check, data loss is silent.

PROPOSED FIX
- Add ddx bead migrate --to axon command under cli/cmd/ reading .ddx/beads.jsonl and .ddx/beads-archive.jsonl and writing losslessly into the axon backend via the Backend interface.
- Idempotent: re-running on an already-migrated store is a no-op (dedup by bead ID).
- Does not delete source files (operator removes after verification).
- Add test fixture under cli/internal/bead/testdata/ covering: representative beads.jsonl + archive.

NON-SCOPE
- Axon backend implementation (ddx-95ec5ed5, a dep).
- Migration to backends other than axon.
- Automatic deletion of source files.
    </description>
    <acceptance>
1. ddx bead migrate --to axon command exists and is documented in --help.
2. After migration, every bead (including events and archive entries) round-trips: ddx bead export | diff against pre-migration export = empty.
3. Idempotent: running twice produces no duplicate beads.
4. Test fixture at cli/internal/bead/testdata/ covers representative beads.jsonl + archive.
5. TestMigrate_AxonBackend_RoundTrip verifies post-migration export matches pre-migration export.
6. TestMigrate_AxonBackend_Idempotent verifies re-run does not duplicate beads.
7. cd cli &amp;&amp; go test ./internal/bead/... green.
8. lefthook run pre-commit passes.
    </acceptance>
    <notes>
REVIEW:BLOCK

Diff contains only execution evidence files (manifest.json, result.json) under .ddx/executions/. No implementation of the migrate --to axon command, no test fixtures, no source code changes. Cannot evaluate AC against missing implementation.
    </notes>
    <labels>phase:2, area:beads, area:storage, kind:tool, backend-migration</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T053906-68f842f3/checks/production-reachability.json</file>
    <file>.ddx/executions/20260505T053906-68f842f3/manifest.json</file>
    <file>.ddx/executions/20260505T053906-68f842f3/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="2cb35b162ee7ba8b27ed50d0943b39d849705225">
diff --git a/.ddx/executions/20260505T053906-68f842f3/checks/production-reachability.json b/.ddx/executions/20260505T053906-68f842f3/checks/production-reachability.json
new file mode 100644
index 00000000..89e73251
--- /dev/null
+++ b/.ddx/executions/20260505T053906-68f842f3/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no non-test Go files changed"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T053906-68f842f3/manifest.json b/.ddx/executions/20260505T053906-68f842f3/manifest.json
new file mode 100644
index 00000000..fc8255d1
--- /dev/null
+++ b/.ddx/executions/20260505T053906-68f842f3/manifest.json
@@ -0,0 +1,116 @@
+{
+  "attempt_id": "20260505T053906-68f842f3",
+  "bead_id": "ddx-cb63cdfc",
+  "base_rev": "ee56f4cec7fdb48b28f82b5760f3e41617441e76",
+  "created_at": "2026-05-05T05:39:08.872573643Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-cb63cdfc",
+    "title": "Migration tool: .ddx/beads.jsonl + archive → axon backend",
+    "description": "PROBLEM\nBeads stored in .ddx/beads.jsonl and .ddx/beads-archive.jsonl cannot be migrated to the axon backend without a dedicated migration command. Operators switching to the axon backend must manually reconstruct state or risk data loss.\n\nROOT CAUSE\n- cli/cmd/ does not contain a 'ddx bead migrate' subcommand or --to flag.\n- cli/internal/bead/ has backend_jsonl.go (JSONL reader) but no migration path to write into the axon Backend interface (ddx-95ec5ed5, a dep).\n- Without an idempotent migration, re-running risks duplicates; without a round-trip check, data loss is silent.\n\nPROPOSED FIX\n- Add ddx bead migrate --to axon command under cli/cmd/ reading .ddx/beads.jsonl and .ddx/beads-archive.jsonl and writing losslessly into the axon backend via the Backend interface.\n- Idempotent: re-running on an already-migrated store is a no-op (dedup by bead ID).\n- Does not delete source files (operator removes after verification).\n- Add test fixture under cli/internal/bead/testdata/ covering: representative beads.jsonl + archive.\n\nNON-SCOPE\n- Axon backend implementation (ddx-95ec5ed5, a dep).\n- Migration to backends other than axon.\n- Automatic deletion of source files.",
+    "acceptance": "1. ddx bead migrate --to axon command exists and is documented in --help.\n2. After migration, every bead (including events and archive entries) round-trips: ddx bead export | diff against pre-migration export = empty.\n3. Idempotent: running twice produces no duplicate beads.\n4. Test fixture at cli/internal/bead/testdata/ covers representative beads.jsonl + archive.\n5. TestMigrate_AxonBackend_RoundTrip verifies post-migration export matches pre-migration export.\n6. TestMigrate_AxonBackend_Idempotent verifies re-run does not duplicate beads.\n7. cd cli \u0026\u0026 go test ./internal/bead/... green.\n8. lefthook run pre-commit passes.",
+    "parent": "ddx-5d49b14e",
+    "labels": [
+      "phase:2",
+      "area:beads",
+      "area:storage",
+      "kind:tool",
+      "backend-migration"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T05:39:06Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[],\"requested_harness\":\"claude\"}",
+          "created_at": "2026-05-03T22:30:28.346622195Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260503T221716-47fdb078\",\"harness\":\"claude\",\"input_tokens\":446,\"output_tokens\":37626,\"total_tokens\":38072,\"cost_usd\":4.720472749999999,\"duration_ms\":789960,\"exit_code\":0}",
+          "created_at": "2026-05-03T22:30:28.446161806Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=38072 cost_usd=4.7205"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"\",\"resolved_provider\":\"claude\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-03T22:30:33.945631219Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "erik",
+          "body": "Diff contains only execution evidence files (manifest.json, result.json) under .ddx/executions/. No implementation of the migrate --to axon command, no test fixtures, no source code changes. Cannot evaluate AC against missing implementation.\nharness=claude\nmodel=opus\ninput_bytes=6258\noutput_bytes=1213\nelapsed_ms=66864",
+          "created_at": "2026-05-03T22:31:40.988539386Z",
+          "kind": "review",
+          "source": "ddx agent execute-loop",
+          "summary": "BLOCK"
+        },
+        {
+          "actor": "",
+          "body": "",
+          "created_at": "2026-05-03T22:31:41.091419285Z",
+          "kind": "reopen",
+          "source": "",
+          "summary": "review: BLOCK"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"action\":\"re_attempt_with_context\",\"mode\":\"review_block\"}",
+          "created_at": "2026-05-03T22:31:41.175688286Z",
+          "kind": "triage-decision",
+          "source": "ddx agent execute-loop",
+          "summary": "review_block: re_attempt_with_context"
+        },
+        {
+          "actor": "erik",
+          "body": "post-merge review: BLOCK (flagged for human)\nDiff contains only execution evidence files (manifest.json, result.json) under .ddx/executions/. No implementation of the migrate --to axon command, no test fixtures, no source code changes. Cannot evaluate AC against missing implementation.\nresult_rev=58b460018431ae6beb881f50f9085acc7dae3814\nbase_rev=823ced625935a40a12172a0a073a221ae18e6520",
+          "created_at": "2026-05-03T22:31:41.329645928Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "review_block"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\",\"fallback_chain\":[]}",
+          "created_at": "2026-05-04T22:01:25.680501144Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "erik",
+          "body": "exit status 1\nresult_rev=45737b718ab3ad97d98e96a3aaa2a441307db946\nbase_rev=45737b718ab3ad97d98e96a3aaa2a441307db946\nretry_after=2026-05-05T04:01:26Z",
+          "created_at": "2026-05-04T22:01:26.40943215Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T05:39:06.742674787Z",
+      "execute-loop-last-detail": "exit status 1",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-05-05T04:01:26Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T053906-68f842f3",
+    "prompt": ".ddx/executions/20260505T053906-68f842f3/prompt.md",
+    "manifest": ".ddx/executions/20260505T053906-68f842f3/manifest.json",
+    "result": ".ddx/executions/20260505T053906-68f842f3/result.json",
+    "checks": ".ddx/executions/20260505T053906-68f842f3/checks.json",
+    "usage": ".ddx/executions/20260505T053906-68f842f3/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-cb63cdfc-20260505T053906-68f842f3"
+  },
+  "prompt_sha": "113a3d11cf59c853fe05ede3d5c584ce97497da91a87f33872249e93711bb7ee"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T053906-68f842f3/result.json b/.ddx/executions/20260505T053906-68f842f3/result.json
new file mode 100644
index 00000000..deb5d141
--- /dev/null
+++ b/.ddx/executions/20260505T053906-68f842f3/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-cb63cdfc",
+  "attempt_id": "20260505T053906-68f842f3",
+  "base_rev": "ee56f4cec7fdb48b28f82b5760f3e41617441e76",
+  "result_rev": "744b0edfa8fe188ac59854053d1c6e8ca9d34ec8",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-8883e77b",
+  "duration_ms": 216609,
+  "tokens": 2240408,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T053906-68f842f3",
+  "prompt_file": ".ddx/executions/20260505T053906-68f842f3/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T053906-68f842f3/manifest.json",
+  "result_file": ".ddx/executions/20260505T053906-68f842f3/result.json",
+  "usage_file": ".ddx/executions/20260505T053906-68f842f3/usage.json",
+  "started_at": "2026-05-05T05:39:08.872940434Z",
+  "finished_at": "2026-05-05T05:42:45.482884887Z"
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
