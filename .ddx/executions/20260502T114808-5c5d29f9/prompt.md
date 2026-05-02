<bead-review>
  <bead id="ddx-0c0565f3" iter=1>
    <title>Implement ADR-004: bead-backed collection abstraction with archive + attachments</title>
    <description>
ADR-004 (Accepted) decided to generalize .ddx/beads.jsonl into a named-collection abstraction with separate archive and attachment storage, but the implementation hasn't shipped. Today the single beads.jsonl carries the entire history (5.4MB and growing) and the events array on closed beads is the dominant size driver. This epic implements the architecture per ADR-004.

Decisions to make during implementation (carry over from this conversation):
- Archival trigger: file-size threshold (e.g. &gt;4MB), age-based (closed &gt;30d), or count-based (&gt;500 closed). ADR-004 doesn't pick one.
- Whether closed-bead events arrays move into attachment storage (.ddx/attachments/&lt;bead-id&gt;/events.jsonl) or stay inline in beads-archive.
- Migration semantics for the existing beads.jsonl (split in place vs explicit migrate command).

References: ADR-004, SD-004, FEAT-004, recent observation that beads.jsonl crossed the 5MB lefthook threshold during the 2026-05 redesign drain.
    </description>
    <acceptance>
1. All child beads closed. 2. .ddx/beads.jsonl can be kept under a configurable size threshold via active+archive split. 3. Existing tooling (ddx bead list/show/ready/blocked, ddx work) continues to work transparently across active and archived beads. 4. bd/br interchange tests still green. 5. Migration tool exists and runs cleanly on the current 5.4MB beads.jsonl.
    </acceptance>
    <notes>
Decomposed 2026-05-02 into children: ddx-2f453147 (collection abstraction), ddx-f7f09b6e (beads-archive read-through), ddx-cd1f0f7e (attachment sidecar for events), ddx-8fcfe2a7 (ddx bead archive command + size trigger, default &gt;4MB), ddx-cb2eb7e3 (migration tool for current 5.4MB beads.jsonl), ddx-9f7a04f4 (bd/br external-backend support for non-default collections). Decisions baked in: archival trigger defaults to file-size &gt;4MB on closed beads; closed-bead events move to .ddx/attachments/&lt;bead-id&gt;/events.jsonl; migration is an explicit 'ddx bead migrate-archive' command sharing logic with the archive command.

REVIEW:BLOCK

Diff contains only execution evidence files (manifest.json, result.json) for an attempt. No actual implementation of ADR-004 (collection abstraction, archive, attachments, migration tool) is present. AC items 2-5 are unimplemented.
    </notes>
    <labels>area:beads, area:storage, kind:refactor, adr:004</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T114559-db0755dd/manifest.json</file>
    <file>.ddx/executions/20260502T114559-db0755dd/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="40bf9c0273b497d8f7218ac63401677906d6b38b">
diff --git a/.ddx/executions/20260502T114559-db0755dd/manifest.json b/.ddx/executions/20260502T114559-db0755dd/manifest.json
new file mode 100644
index 00000000..c886e85b
--- /dev/null
+++ b/.ddx/executions/20260502T114559-db0755dd/manifest.json
@@ -0,0 +1,87 @@
+{
+  "attempt_id": "20260502T114559-db0755dd",
+  "bead_id": "ddx-0c0565f3",
+  "base_rev": "2238ec89204dc349ec7ec31a39d0d5e6734b6b98",
+  "created_at": "2026-05-02T11:46:00.781623492Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-0c0565f3",
+    "title": "Implement ADR-004: bead-backed collection abstraction with archive + attachments",
+    "description": "ADR-004 (Accepted) decided to generalize .ddx/beads.jsonl into a named-collection abstraction with separate archive and attachment storage, but the implementation hasn't shipped. Today the single beads.jsonl carries the entire history (5.4MB and growing) and the events array on closed beads is the dominant size driver. This epic implements the architecture per ADR-004.\n\nDecisions to make during implementation (carry over from this conversation):\n- Archival trigger: file-size threshold (e.g. \u003e4MB), age-based (closed \u003e30d), or count-based (\u003e500 closed). ADR-004 doesn't pick one.\n- Whether closed-bead events arrays move into attachment storage (.ddx/attachments/\u003cbead-id\u003e/events.jsonl) or stay inline in beads-archive.\n- Migration semantics for the existing beads.jsonl (split in place vs explicit migrate command).\n\nReferences: ADR-004, SD-004, FEAT-004, recent observation that beads.jsonl crossed the 5MB lefthook threshold during the 2026-05 redesign drain.",
+    "acceptance": "1. All child beads closed. 2. .ddx/beads.jsonl can be kept under a configurable size threshold via active+archive split. 3. Existing tooling (ddx bead list/show/ready/blocked, ddx work) continues to work transparently across active and archived beads. 4. bd/br interchange tests still green. 5. Migration tool exists and runs cleanly on the current 5.4MB beads.jsonl.",
+    "labels": [
+      "area:beads",
+      "area:storage",
+      "kind:refactor",
+      "adr:004"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T11:45:59Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1727028",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[],\"requested_harness\":\"claude\"}",
+          "created_at": "2026-05-02T03:52:55.111037217Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260502T034935-c7f9d929\",\"harness\":\"claude\",\"input_tokens\":26,\"output_tokens\":12454,\"total_tokens\":12480,\"cost_usd\":1.068339,\"duration_ms\":197167,\"exit_code\":0}",
+          "created_at": "2026-05-02T03:52:55.228370086Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=12480 cost_usd=1.0683"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"\",\"resolved_provider\":\"claude\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-02T03:52:59.643370436Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "erik",
+          "body": "Diff contains only execution evidence files (manifest.json, result.json) for an attempt. No actual implementation of ADR-004 (collection abstraction, archive, attachments, migration tool) is present. AC items 2-5 are unimplemented.\nharness=claude\nmodel=opus\ninput_bytes=8229\noutput_bytes=1029\nelapsed_ms=12380",
+          "created_at": "2026-05-02T03:53:12.235776564Z",
+          "kind": "review",
+          "source": "ddx agent execute-loop",
+          "summary": "BLOCK"
+        },
+        {
+          "actor": "",
+          "body": "",
+          "created_at": "2026-05-02T03:53:12.358498762Z",
+          "kind": "reopen",
+          "source": "",
+          "summary": "review: BLOCK"
+        },
+        {
+          "actor": "erik",
+          "body": "post-merge review: BLOCK (flagged for human)\nDiff contains only execution evidence files (manifest.json, result.json) for an attempt. No actual implementation of ADR-004 (collection abstraction, archive, attachments, migration tool) is present. AC items 2-5 are unimplemented.\nresult_rev=26e13b242ae1a79e492b0c2ffc349ab457b40b74\nbase_rev=442dba0257eb136b1600474a359d93735b47f28c",
+          "created_at": "2026-05-02T03:53:12.47695888Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "review_block"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-02T11:45:59.493495051Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T114559-db0755dd",
+    "prompt": ".ddx/executions/20260502T114559-db0755dd/prompt.md",
+    "manifest": ".ddx/executions/20260502T114559-db0755dd/manifest.json",
+    "result": ".ddx/executions/20260502T114559-db0755dd/result.json",
+    "checks": ".ddx/executions/20260502T114559-db0755dd/checks.json",
+    "usage": ".ddx/executions/20260502T114559-db0755dd/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-0c0565f3-20260502T114559-db0755dd"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T114559-db0755dd/result.json b/.ddx/executions/20260502T114559-db0755dd/result.json
new file mode 100644
index 00000000..5bf19fcb
--- /dev/null
+++ b/.ddx/executions/20260502T114559-db0755dd/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-0c0565f3",
+  "attempt_id": "20260502T114559-db0755dd",
+  "base_rev": "2238ec89204dc349ec7ec31a39d0d5e6734b6b98",
+  "result_rev": "58bc9071825677d3a67f5d190945aa3818f0a0da",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-d2c5cb34",
+  "duration_ms": 123165,
+  "tokens": 4787,
+  "cost_usd": 0.6069997500000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T114559-db0755dd",
+  "prompt_file": ".ddx/executions/20260502T114559-db0755dd/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T114559-db0755dd/manifest.json",
+  "result_file": ".ddx/executions/20260502T114559-db0755dd/result.json",
+  "usage_file": ".ddx/executions/20260502T114559-db0755dd/usage.json",
+  "started_at": "2026-05-02T11:46:00.781908117Z",
+  "finished_at": "2026-05-02T11:48:03.947310382Z"
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
