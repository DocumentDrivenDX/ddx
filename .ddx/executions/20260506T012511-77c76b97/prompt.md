<bead-review>
  <bead id="ddx-00a8ed79" iter=1>
    <title>cli: support root --version flag</title>
    <description>
PROBLEM
  `ddx --version` exits with an unknown-flag error even though `ddx version` works. Operators commonly guess top-level `--version` for modern CLIs; DDx should support both forms so discovery works without removing or changing the existing `version` subcommand.

ROOT CAUSE
  cli/cmd/command_factory.go:124-127 registers root persistent flags for `--config`, `--verbose`/`-v`, and `--library-base-path`, but no root `--version` flag exists. cli/cmd/command_factory.go:450-466 defines the `version` subcommand and prints the desired version/commit/build output there only. cli/cmd/command_factory.go:130-147 runs root PersistentPreRunE for normal command execution; a root `--version` path must print version info and exit successfully before ordinary command validation/update checks make the flag behave like a missing command.

PROPOSED FIX
  - Add a root persistent `--version` boolean flag in cli/cmd/command_factory.go near the other root flags.
  - Factor the existing version-printing body from the `version` subcommand into a shared helper so `ddx version` and `ddx --version` return the same concise DDx version, commit, and build output.
  - Wire root flag handling so `ddx --version` exits 0, writes to command stdout, does not require a subcommand, and does not consume `-v` because `-v` remains the verbose flag.
  - Add focused tests in cli/cmd/version_acceptance_test.go for `TestRootVersionFlag_PrintsVersionAndBuildInfo` and `TestVersionCommand_StillPrintsVersionAndBuildInfo` (or update the existing acceptance test with those exact subtests).

NON-SCOPE
  - Do not remove or rename `ddx version`.
  - Do not change `-v`; it remains verbose output.
  - Do not alter `ddx status`, plugin version checks, installation/update behavior, or release metadata formatting beyond sharing the existing version output helper.

PARENT
  This is a root-level CLI ergonomics bug; no suitable open CLI umbrella bead exists.

DEPS
  No deps.
    </description>
    <acceptance>
1. `ddx --version` exits 0 and prints the same DDx version, Commit, and Built lines currently printed by `ddx version`.
2. `ddx version` remains supported and keeps printing DDx version, Commit, and Built lines.
3. `-v` remains the existing verbose flag; this bead does not introduce `ddx -v` as a version alias.
4. TestRootVersionFlag_PrintsVersionAndBuildInfo in cli/cmd/version_acceptance_test.go (or an equivalently named subtest under TestAcceptance_US008_CheckDDxVersion) verifies root `--version` output and exit success.
5. TestVersionCommand_StillPrintsVersionAndBuildInfo in cli/cmd/version_acceptance_test.go (or the existing updated acceptance subtest) verifies the `version` subcommand still works after the helper extraction.
6. `cd cli &amp;&amp; go test ./cmd/... -run 'TestRootVersionFlag_PrintsVersionAndBuildInfo|TestVersionCommand_StillPrintsVersionAndBuildInfo|TestAcceptance_US008_CheckDDxVersion' -count=1` passes.
7. `lefthook run pre-commit` passes.
    </acceptance>
    <labels>phase:2, area:cli, kind:fix, ux, discovery</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T011801-6a3687e3/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T011801-6a3687e3/manifest.json</file>
    <file>.ddx/executions/20260506T011801-6a3687e3/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="e23cf3e487a5cac8fcb4ea2dcf16a106a06f96dd">
<untrusted-data>
diff --git a/.ddx/executions/20260506T011801-6a3687e3/checks/production-reachability.json b/.ddx/executions/20260506T011801-6a3687e3/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260506T011801-6a3687e3/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T011801-6a3687e3/manifest.json b/.ddx/executions/20260506T011801-6a3687e3/manifest.json
new file mode 100644
index 00000000..028af162
--- /dev/null
+++ b/.ddx/executions/20260506T011801-6a3687e3/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260506T011801-6a3687e3",
+  "bead_id": "ddx-00a8ed79",
+  "base_rev": "849d82c3ba7523ecd58d89de76af1ff8221d39e8",
+  "created_at": "2026-05-06T01:18:03.283859444Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-00a8ed79",
+    "title": "cli: support root --version flag",
+    "description": "PROBLEM\n  `ddx --version` exits with an unknown-flag error even though `ddx version` works. Operators commonly guess top-level `--version` for modern CLIs; DDx should support both forms so discovery works without removing or changing the existing `version` subcommand.\n\nROOT CAUSE\n  cli/cmd/command_factory.go:124-127 registers root persistent flags for `--config`, `--verbose`/`-v`, and `--library-base-path`, but no root `--version` flag exists. cli/cmd/command_factory.go:450-466 defines the `version` subcommand and prints the desired version/commit/build output there only. cli/cmd/command_factory.go:130-147 runs root PersistentPreRunE for normal command execution; a root `--version` path must print version info and exit successfully before ordinary command validation/update checks make the flag behave like a missing command.\n\nPROPOSED FIX\n  - Add a root persistent `--version` boolean flag in cli/cmd/command_factory.go near the other root flags.\n  - Factor the existing version-printing body from the `version` subcommand into a shared helper so `ddx version` and `ddx --version` return the same concise DDx version, commit, and build output.\n  - Wire root flag handling so `ddx --version` exits 0, writes to command stdout, does not require a subcommand, and does not consume `-v` because `-v` remains the verbose flag.\n  - Add focused tests in cli/cmd/version_acceptance_test.go for `TestRootVersionFlag_PrintsVersionAndBuildInfo` and `TestVersionCommand_StillPrintsVersionAndBuildInfo` (or update the existing acceptance test with those exact subtests).\n\nNON-SCOPE\n  - Do not remove or rename `ddx version`.\n  - Do not change `-v`; it remains verbose output.\n  - Do not alter `ddx status`, plugin version checks, installation/update behavior, or release metadata formatting beyond sharing the existing version output helper.\n\nPARENT\n  This is a root-level CLI ergonomics bug; no suitable open CLI umbrella bead exists.\n\nDEPS\n  No deps.",
+    "acceptance": "1. `ddx --version` exits 0 and prints the same DDx version, Commit, and Built lines currently printed by `ddx version`.\n2. `ddx version` remains supported and keeps printing DDx version, Commit, and Built lines.\n3. `-v` remains the existing verbose flag; this bead does not introduce `ddx -v` as a version alias.\n4. TestRootVersionFlag_PrintsVersionAndBuildInfo in cli/cmd/version_acceptance_test.go (or an equivalently named subtest under TestAcceptance_US008_CheckDDxVersion) verifies root `--version` output and exit success.\n5. TestVersionCommand_StillPrintsVersionAndBuildInfo in cli/cmd/version_acceptance_test.go (or the existing updated acceptance subtest) verifies the `version` subcommand still works after the helper extraction.\n6. `cd cli \u0026\u0026 go test ./cmd/... -run 'TestRootVersionFlag_PrintsVersionAndBuildInfo|TestVersionCommand_StillPrintsVersionAndBuildInfo|TestAcceptance_US008_CheckDDxVersion' -count=1` passes.\n7. `lefthook run pre-commit` passes.",
+    "labels": [
+      "phase:2",
+      "area:cli",
+      "kind:fix",
+      "ux",
+      "discovery"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T01:18:01Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "execute-loop-heartbeat-at": "2026-05-06T01:18:01.110867766Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T011801-6a3687e3",
+    "prompt": ".ddx/executions/20260506T011801-6a3687e3/prompt.md",
+    "manifest": ".ddx/executions/20260506T011801-6a3687e3/manifest.json",
+    "result": ".ddx/executions/20260506T011801-6a3687e3/result.json",
+    "checks": ".ddx/executions/20260506T011801-6a3687e3/checks.json",
+    "usage": ".ddx/executions/20260506T011801-6a3687e3/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-00a8ed79-20260506T011801-6a3687e3"
+  },
+  "prompt_sha": "1c46c8216a2a93b1b39d9e1396d1b542ac2e06cc59a74d4dd1f308ec8663e6ae"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T011801-6a3687e3/result.json b/.ddx/executions/20260506T011801-6a3687e3/result.json
new file mode 100644
index 00000000..937b1946
--- /dev/null
+++ b/.ddx/executions/20260506T011801-6a3687e3/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-00a8ed79",
+  "attempt_id": "20260506T011801-6a3687e3",
+  "base_rev": "849d82c3ba7523ecd58d89de76af1ff8221d39e8",
+  "result_rev": "b7f71c8617581c8d56b60dd4235019e6f60dc29e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-a77f2014",
+  "duration_ms": 420652,
+  "tokens": 5552771,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T011801-6a3687e3",
+  "prompt_file": ".ddx/executions/20260506T011801-6a3687e3/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T011801-6a3687e3/manifest.json",
+  "result_file": ".ddx/executions/20260506T011801-6a3687e3/result.json",
+  "usage_file": ".ddx/executions/20260506T011801-6a3687e3/usage.json",
+  "started_at": "2026-05-06T01:18:03.284290568Z",
+  "finished_at": "2026-05-06T01:25:03.93710519Z"
+}
\ No newline at end of file
</untrusted-data>
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
