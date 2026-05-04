<bead-review>
  <bead id="ddx-d1dae2dd" iter=1>
    <title>agent: invoke pre-dispatch lint hook before claim</title>
    <description>
PROBLEM
  ddx try must invoke bead-quality lint after selecting a candidate and before claiming it, but the execute-bead loop currently has no lint hook invocation or warn/block policy at that point.

ROOT CAUSE
  cli/internal/agent/execute_bead_loop.go:640-714 runs PreClaimHook and RoutePreflight before Store.Claim. cli/internal/agent/execute_bead_loop.go:668-710 is the current pre-claim RoutePreflight gate; cli/internal/agent/execute_bead_loop.go:714 calls Store.Claim. cli/internal/agent/execute_bead_loop.go:29 now exposes ExecuteBeadLoopRuntime.PreDispatchLintHook from closed contract bead ddx-c8dc1146, but no loop path invokes it and no path emits bead-quality.lint events.

PROPOSED FIX
  - In cli/internal/agent/execute_bead_loop.go, invoke runtime.PreDispatchLintHook(ctx, candidate.ID) after RoutePreflight succeeds and before Store.Claim(candidate.ID, assignee).
  - Default behavior is WARN-ONLY: append a bead event kind bead-quality.lint containing score, rationale, suggested_fixes, waivers_applied, and any warning details, then continue to claim regardless of score.
  - Add opt-in BLOCK behavior from resolved config key bead-quality.lint.block_threshold_score. Only a valid hook response with Score below threshold refuses dispatch; infrastructure errors fail open.
  - On block, emit operator-readable log/loop event text citing bead-lifecycle SKILL.md guidance and skip claiming that bead. The skipped bead must be marked attempted for this Run so the loop moves on instead of spinning on the same candidate.
  - Treat hook errors, missing skill/harness, bad JSON surfaced through the hook, and timeouts as warning events and proceed with claim.

NON-SCOPE
  - Defining hook/result types; closed in ddx-c8dc1146.
  - Implementing the runner-backed lint hook body; covered by ddx-68706ef9.
  - Post-attempt triage and OutcomeReason handling; covered by ddx-4375292b.
  - ddx work integration.

PARENT
  ddx-e1a576a7.

DEPS
  No deps. ddx-c8dc1146 is closed and its hook contract is available in cli/internal/agent/quality_hooks.go and ExecuteBeadLoopRuntime.
    </description>
    <acceptance>
1. PreDispatchLintHook fires after RoutePreflight success and before Store.Claim in cli/internal/agent/execute_bead_loop.go; if RoutePreflight rejects, lint is not invoked for that candidate.
2. TestLoop_LintHook_FiresPreClaim proves hook execution happens before claim by observing hook/claim ordering.
3. TestLintHook_LowScore_WarnDoesNotBlock proves a low score appends a bead-quality.lint event and the loop still claims/dispatches by default.
4. TestLintHook_BlockMode_RefusesDispatchOnLowScore proves bead-quality.lint.block_threshold_score blocks only when the hook returns a valid score below threshold, with no Store.Claim call for that bead.
5. TestLintHook_SkillMissing_ProceedsWithWarning, TestLintHook_BadJSON_ProceedsWithWarning, and TestLintHook_Timeout_ProceedsWithWarning prove hook infrastructure failures append warnings and proceed with claim.
6. WIRED-IN: git grep PreDispatchLintHook cli/internal/agent shows the execute-loop invocation site, not just the runtime field declaration.
7. cd cli &amp;&amp; go test ./internal/agent/... green.
8. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:agent, kind:feature, reliability, bead-quality, adr:023</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260504T213703-9d218d8a/checks/production-reachability.json</file>
    <file>.ddx/executions/20260504T213703-9d218d8a/manifest.json</file>
    <file>.ddx/executions/20260504T213703-9d218d8a/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="9b2f822996928d3c5cf752d3216f9f622a790a1b">
diff --git a/.ddx/executions/20260504T213703-9d218d8a/checks/production-reachability.json b/.ddx/executions/20260504T213703-9d218d8a/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260504T213703-9d218d8a/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260504T213703-9d218d8a/manifest.json b/.ddx/executions/20260504T213703-9d218d8a/manifest.json
new file mode 100644
index 00000000..d0151c7e
--- /dev/null
+++ b/.ddx/executions/20260504T213703-9d218d8a/manifest.json
@@ -0,0 +1,40 @@
+{
+  "attempt_id": "20260504T213703-9d218d8a",
+  "bead_id": "ddx-d1dae2dd",
+  "base_rev": "e78de91b45112786a030b4f253da0e84121b46b7",
+  "created_at": "2026-05-04T21:37:05.298786501Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-d1dae2dd",
+    "title": "agent: invoke pre-dispatch lint hook before claim",
+    "description": "PROBLEM\n  ddx try must invoke bead-quality lint after selecting a candidate and before claiming it, but the execute-bead loop currently has no lint hook invocation or warn/block policy at that point.\n\nROOT CAUSE\n  cli/internal/agent/execute_bead_loop.go:636-653 runs PreClaimHook before attempted bookkeeping. cli/internal/agent/execute_bead_loop.go:657-684 runs RoutePreflight between nextCandidate and Claim, which is the documented pre-claim gate location, but there is no sibling PreDispatchLintHook call. cli/internal/agent/execute_bead_loop.go:290-309 exposes AppendEvent/Update store operations that can record bead-quality events, but no loop path emits bead-quality.lint.\n\nPROPOSED FIX\n  - In cli/internal/agent/execute_bead_loop.go, invoke runtime.PreDispatchLintHook after nextCandidate and before Store.Claim, adjacent to RoutePreflight.\n  - Default behavior is WARN-ONLY: append a bead event kind bead-quality.lint containing score, rationale, suggested_fixes, waivers_applied, and any warning details, then continue to claim regardless of score.\n  - Add opt-in BLOCK behavior from resolved config key bead-quality.lint.block_threshold_score: only valid hook responses with Score below threshold refuse dispatch; infrastructure errors fail open.\n  - On block, emit operator-readable log/loop event text citing the bead-lifecycle SKILL.md guidance and skip claiming that bead.\n  - Treat hook errors, missing skill/harness, bad JSON surfaced through the hook, and timeouts as warning events and proceed.\n\nNON-SCOPE\n  - Defining the hook/result types; depends on child ddx-c8dc1146.\n  - Implementing the runner-backed lint hook body; covered by sibling lint implementation child.\n  - Post-attempt triage and OutcomeReason handling.\n  - ddx work integration.\n\nPARENT\n  ddx-e1a576a7.\n\nDEPS\n  - ddx-c8dc1146: shared runtime hook/result types must exist first.\n  - ddx-e1a576a7: parent decomposition edge requested by the parent attempt instructions.",
+    "acceptance": "1. PreDispatchLintHook fires after nextCandidate and before Store.Claim in cli/internal/agent/execute_bead_loop.go, adjacent to RoutePreflight.\n2. TestLoop_LintHook_FiresPreClaim proves hook execution happens before claim by observing hook/claim ordering.\n3. TestLintHook_LowScore_WarnDoesNotBlock proves a low score appends a bead-quality.lint event and the loop still claims/dispatches by default.\n4. TestLintHook_BlockMode_RefusesDispatchOnLowScore proves bead-quality.lint.block_threshold_score blocks only when the hook returns a valid score below threshold, with no Store.Claim call.\n5. TestLintHook_SkillMissing_ProceedsWithWarning, TestLintHook_BadJSON_ProceedsWithWarning, and TestLintHook_Timeout_ProceedsWithWarning prove hook infrastructure failures append warnings and proceed with claim.\n6. WIRED-IN: git grep PreDispatchLintHook cli/internal/agent shows the execute-loop invocation site.\n7. cd cli \u0026\u0026 go test ./internal/agent/... green.\n8. lefthook run pre-commit passes.",
+    "parent": "ddx-e1a576a7",
+    "labels": [
+      "phase:2",
+      "area:agent",
+      "kind:feature",
+      "reliability",
+      "bead-quality",
+      "adr:023"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-04T21:37:03Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3748035",
+      "execute-loop-heartbeat-at": "2026-05-04T21:37:03.108529731Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260504T213703-9d218d8a",
+    "prompt": ".ddx/executions/20260504T213703-9d218d8a/prompt.md",
+    "manifest": ".ddx/executions/20260504T213703-9d218d8a/manifest.json",
+    "result": ".ddx/executions/20260504T213703-9d218d8a/result.json",
+    "checks": ".ddx/executions/20260504T213703-9d218d8a/checks.json",
+    "usage": ".ddx/executions/20260504T213703-9d218d8a/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-d1dae2dd-20260504T213703-9d218d8a"
+  },
+  "prompt_sha": "0822b44a9308b31ed0124f14385d4808f12457fd60fdadd603849f153e488f5e"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260504T213703-9d218d8a/result.json b/.ddx/executions/20260504T213703-9d218d8a/result.json
new file mode 100644
index 00000000..476d5c92
--- /dev/null
+++ b/.ddx/executions/20260504T213703-9d218d8a/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-d1dae2dd",
+  "attempt_id": "20260504T213703-9d218d8a",
+  "base_rev": "e78de91b45112786a030b4f253da0e84121b46b7",
+  "result_rev": "c074c015cd544065865672ce9b818e1b497a7662",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "session_id": "eb-608bb9ea",
+  "duration_ms": 543876,
+  "tokens": 9557094,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260504T213703-9d218d8a",
+  "prompt_file": ".ddx/executions/20260504T213703-9d218d8a/prompt.md",
+  "manifest_file": ".ddx/executions/20260504T213703-9d218d8a/manifest.json",
+  "result_file": ".ddx/executions/20260504T213703-9d218d8a/result.json",
+  "usage_file": ".ddx/executions/20260504T213703-9d218d8a/usage.json",
+  "started_at": "2026-05-04T21:37:05.299234042Z",
+  "finished_at": "2026-05-04T21:46:09.175441011Z"
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
