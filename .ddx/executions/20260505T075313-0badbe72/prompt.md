<bead-review>
  <bead id="ddx-633b16eb" iter=1>
    <title>cli: 'ddx --version' should warn when installed binary is behind source tree</title>
    <description>
PROBLEM
After rebuilding ddx (make install), the installed binary at ~/.local/bin/ddx is correct. But subsequent commits land in the source tree, and the operator forgets to rebuild. New flags / subcommands appear in source but silently disappear from the operator's CLI surface. Today's session bit this twice: 'ddx work plan --limit' worked, then later returned 'unknown flag: --limit' — not because the feature was broken, but because the binary was stale relative to source.

OBSERVED (2026-05-03 session)
- Built + installed at commit c0f95380; 'ddx work plan --limit' worked.
- ~10 commits later: 'ddx work plan --limit' returned 'unknown flag.'
- Source still had the flag definition; binary was stale.
- Operator wasted time investigating a non-existent regression before remembering to rebuild.

This compounds because:
1. Source can drift hours-to-days ahead of the installed binary during active development.
2. Cobra's 'unknown flag' / 'unknown command' errors give NO hint that the binary is stale.
3. ddx work auto-spawns workers — those workers also use the stale binary, propagating the gap.

ROOT CAUSE
cli/cmd/version.go implements 'ddx version' and 'ddx --version'. It prints the build-time-injected version string (e.g., 'v0.6.2-alpha12-12-g86848d21') from ldflags but does NOT compare against the source tree's current HEAD when invoked from inside a ddx project repo. No stale-binary detection exists in cli/cmd/doctor.go either. The build SHA is injected at link time via main.go -ldflags but never compared against runtime git rev-parse HEAD.

PROPOSED FIX
Extend 'ddx --version' (or add 'ddx doctor' check) to:
1. Detect when invoked from inside a git repo whose origin matches the ddx project (DocumentDrivenDX/ddx).
2. Compare the binary's build SHA (from -ldflags / main.go version injection) against the repo's HEAD.
3. If HEAD is ahead of the binary's SHA, print a warning to stderr: 'WARNING: installed ddx is built from &lt;binary-sha&gt;; source tree HEAD is &lt;head-sha&gt; (&lt;N&gt; commits ahead). Run "make install" to refresh.'
4. Exit 0 (informational, not fatal).

Optional: emit same warning on EVERY ddx command invocation when inside the ddx project (cheap: one git rev-parse).

NON-SCOPE
- Auto-rebuilding (operator should explicitly rebuild).
- Comparing against upstream remote (only local source tree).
- Detection from outside the ddx project repo.
    </description>
    <acceptance>
1. cli/cmd/version.go or cli/cmd/doctor.go detects when invoked from inside a git repo whose origin matches DocumentDrivenDX/ddx.
2. When stale (HEAD ahead of build SHA), prints to stderr: 'WARNING: installed ddx is built from &lt;binary-sha&gt;; source tree HEAD is &lt;head-sha&gt; (&lt;N&gt; commits ahead). Run "make install" to refresh.'
3. Exit code 0 (informational only).
4. TestVersion_StaleWarning_PrintsToStderr verifies warning appears when HEAD is ahead of injected SHA.
5. TestVersion_NoWarningWhenInSync verifies no warning when HEAD matches build SHA.
6. TestVersion_NoWarningOutsideDDxRepo verifies no warning when invoked from a non-ddx repo.
7. TestVersion_HandlesDetachedHead verifies graceful handling of detached HEAD (warning OK or skipped).
8. cd cli &amp;&amp; go test ./cmd/... green.
9. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:cli, kind:fix, observed-failure, reliability, bead-quality</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T074606-cd3f6f77/checks/production-reachability.json</file>
    <file>.ddx/executions/20260505T074606-cd3f6f77/manifest.json</file>
    <file>.ddx/executions/20260505T074606-cd3f6f77/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="3bb43a013e3a8ec4f5b78ce8c4b4613a29d95e40">
diff --git a/.ddx/executions/20260505T074606-cd3f6f77/checks/production-reachability.json b/.ddx/executions/20260505T074606-cd3f6f77/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260505T074606-cd3f6f77/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T074606-cd3f6f77/manifest.json b/.ddx/executions/20260505T074606-cd3f6f77/manifest.json
new file mode 100644
index 00000000..0798d7b1
--- /dev/null
+++ b/.ddx/executions/20260505T074606-cd3f6f77/manifest.json
@@ -0,0 +1,61 @@
+{
+  "attempt_id": "20260505T074606-cd3f6f77",
+  "bead_id": "ddx-633b16eb",
+  "base_rev": "4be42ab22d09338612f44fd590439ee7fd85b98b",
+  "created_at": "2026-05-05T07:46:08.323519052Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-633b16eb",
+    "title": "cli: 'ddx --version' should warn when installed binary is behind source tree",
+    "description": "PROBLEM\nAfter rebuilding ddx (make install), the installed binary at ~/.local/bin/ddx is correct. But subsequent commits land in the source tree, and the operator forgets to rebuild. New flags / subcommands appear in source but silently disappear from the operator's CLI surface. Today's session bit this twice: 'ddx work plan --limit' worked, then later returned 'unknown flag: --limit' — not because the feature was broken, but because the binary was stale relative to source.\n\nOBSERVED (2026-05-03 session)\n- Built + installed at commit c0f95380; 'ddx work plan --limit' worked.\n- ~10 commits later: 'ddx work plan --limit' returned 'unknown flag.'\n- Source still had the flag definition; binary was stale.\n- Operator wasted time investigating a non-existent regression before remembering to rebuild.\n\nThis compounds because:\n1. Source can drift hours-to-days ahead of the installed binary during active development.\n2. Cobra's 'unknown flag' / 'unknown command' errors give NO hint that the binary is stale.\n3. ddx work auto-spawns workers — those workers also use the stale binary, propagating the gap.\n\nROOT CAUSE\ncli/cmd/version.go implements 'ddx version' and 'ddx --version'. It prints the build-time-injected version string (e.g., 'v0.6.2-alpha12-12-g86848d21') from ldflags but does NOT compare against the source tree's current HEAD when invoked from inside a ddx project repo. No stale-binary detection exists in cli/cmd/doctor.go either. The build SHA is injected at link time via main.go -ldflags but never compared against runtime git rev-parse HEAD.\n\nPROPOSED FIX\nExtend 'ddx --version' (or add 'ddx doctor' check) to:\n1. Detect when invoked from inside a git repo whose origin matches the ddx project (DocumentDrivenDX/ddx).\n2. Compare the binary's build SHA (from -ldflags / main.go version injection) against the repo's HEAD.\n3. If HEAD is ahead of the binary's SHA, print a warning to stderr: 'WARNING: installed ddx is built from \u003cbinary-sha\u003e; source tree HEAD is \u003chead-sha\u003e (\u003cN\u003e commits ahead). Run \"make install\" to refresh.'\n4. Exit 0 (informational, not fatal).\n\nOptional: emit same warning on EVERY ddx command invocation when inside the ddx project (cheap: one git rev-parse).\n\nNON-SCOPE\n- Auto-rebuilding (operator should explicitly rebuild).\n- Comparing against upstream remote (only local source tree).\n- Detection from outside the ddx project repo.",
+    "acceptance": "1. cli/cmd/version.go or cli/cmd/doctor.go detects when invoked from inside a git repo whose origin matches DocumentDrivenDX/ddx.\n2. When stale (HEAD ahead of build SHA), prints to stderr: 'WARNING: installed ddx is built from \u003cbinary-sha\u003e; source tree HEAD is \u003chead-sha\u003e (\u003cN\u003e commits ahead). Run \"make install\" to refresh.'\n3. Exit code 0 (informational only).\n4. TestVersion_StaleWarning_PrintsToStderr verifies warning appears when HEAD is ahead of injected SHA.\n5. TestVersion_NoWarningWhenInSync verifies no warning when HEAD matches build SHA.\n6. TestVersion_NoWarningOutsideDDxRepo verifies no warning when invoked from a non-ddx repo.\n7. TestVersion_HandlesDetachedHead verifies graceful handling of detached HEAD (warning OK or skipped).\n8. cd cli \u0026\u0026 go test ./cmd/... green.\n9. lefthook run pre-commit passes.",
+    "parent": "ddx-e34994e2",
+    "labels": [
+      "phase:2",
+      "area:cli",
+      "kind:fix",
+      "observed-failure",
+      "reliability",
+      "bead-quality"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T07:46:06Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\",\"fallback_chain\":[]}",
+          "created_at": "2026-05-04T22:21:52.356736003Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "erik",
+          "body": "exit status 1\nresult_rev=3e07bed26c0b487cb0f002f36582bd0cf126d0ff\nbase_rev=3e07bed26c0b487cb0f002f36582bd0cf126d0ff\nretry_after=2026-05-05T04:21:52Z",
+          "created_at": "2026-05-04T22:21:53.102220484Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T07:46:06.351629408Z",
+      "execute-loop-last-detail": "exit status 1",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-05-05T04:21:52Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T074606-cd3f6f77",
+    "prompt": ".ddx/executions/20260505T074606-cd3f6f77/prompt.md",
+    "manifest": ".ddx/executions/20260505T074606-cd3f6f77/manifest.json",
+    "result": ".ddx/executions/20260505T074606-cd3f6f77/result.json",
+    "checks": ".ddx/executions/20260505T074606-cd3f6f77/checks.json",
+    "usage": ".ddx/executions/20260505T074606-cd3f6f77/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-633b16eb-20260505T074606-cd3f6f77"
+  },
+  "prompt_sha": "d4d8955de5861f9016d0afc58f7d1db34b51c3bdd503622b0f5d7dd78a32b486"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T074606-cd3f6f77/result.json b/.ddx/executions/20260505T074606-cd3f6f77/result.json
new file mode 100644
index 00000000..669d308e
--- /dev/null
+++ b/.ddx/executions/20260505T074606-cd3f6f77/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-633b16eb",
+  "attempt_id": "20260505T074606-cd3f6f77",
+  "base_rev": "4be42ab22d09338612f44fd590439ee7fd85b98b",
+  "result_rev": "8c9432b775152aa9da3fba6be8056ed71cd0a595",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-7bef133f",
+  "duration_ms": 418074,
+  "tokens": 8903691,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T074606-cd3f6f77",
+  "prompt_file": ".ddx/executions/20260505T074606-cd3f6f77/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T074606-cd3f6f77/manifest.json",
+  "result_file": ".ddx/executions/20260505T074606-cd3f6f77/result.json",
+  "usage_file": ".ddx/executions/20260505T074606-cd3f6f77/usage.json",
+  "started_at": "2026-05-05T07:46:08.323832552Z",
+  "finished_at": "2026-05-05T07:53:06.39850601Z"
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
