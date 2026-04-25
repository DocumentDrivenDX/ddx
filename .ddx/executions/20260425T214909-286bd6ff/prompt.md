<bead-review>
  <bead id="ddx-c14f350e" iter=1>
    <title>fix: agent run/doctor invalid --timeout should silence usage block + add regression test</title>
    <description>
## Report
External report at /tank/home/erik/Downloads/ddx-agent-run-invalid-timeout-exits-zero.md claims `ddx agent run --timeout 60ss ...` exits 0 on bad input, breaking wrapper exit-code inference (canonical caller: helix CLI).

## Verification
Bug does NOT reproduce on current main. cli/cmd/agent_cmd.go:117-120 already returns invalid-timeout error from RunE; cli/main.go:31-43 propagates non-nil cobra errors to exit code 1. Reporter is on stale niflheim/sindri host installs.

## Real gaps
1. No regression test guards the exit-code semantic. cli/cmd/agent_run_profile_test.go only exercises happy-path `--timeout 3s`.
2. SilenceUsage is not set on `agent run` — flag parse errors dump the entire usage block above the error message (the cosmetic part of the reporter's complaint).
3. `agent doctor` (cli/cmd/agent_cmd.go:647) calls time.ParseDuration on its own --timeout flag and shares the same class of bug. Same one-line fix.

## Plan
(a) Set `SilenceUsage: true` on `newAgentRunCommand` (cli/cmd/agent_cmd.go:91 area) and on `newAgentDoctorCommand` (sibling, ~line 647). Per-command scope, not root — matches existing precedent (cli/cmd/init.go:99 sets SilenceUsage only; cli/cmd/doc.go:89,118 set both). Do NOT set SilenceErrors=true; keep cobra's stderr error line.

(b) Add a regression test asserting `agent run --timeout 60ss --harness codex --text "..."` returns a non-nil err from executeCommand whose message contains `invalid timeout`. This is sufficient to catch the regression because main.go:31-43 turns any non-nil err into exit 1 — no subprocess test needed. Place the test in a neutrally named file, not agent_run_profile_test.go (the profile-specific filename is misleading for a generic flag-parse test).

(c) Optional polish: assert the captured output after SilenceUsage does NOT contain `Usage:`/`Flags:` lines.

(d) Do NOT extend coverage to other duration-typed inputs across the CLI; out of scope for this bug.

## Out of scope
- Helix-side fix (already done at /home/erik/Projects/helix/scripts/helix line 70 per reporter).
- Root-level cobra UX changes.
- Validation hardening of other duration flags elsewhere.

## Refinement log
Reviewed by fresh-eyes (general-purpose subagent) and codex (`ddx agent run --harness codex`). Fresh-eyes verified main.go propagates cobra errors faithfully — command-level test is sufficient. Codex pushed for a subprocess exit-code test; rejected because main.go:31 invariant makes the command-level assertion equivalent. Both reviewers agreed per-command SilenceUsage is the right scope (matches init.go/doc.go precedent). Folded in: doctor-command parity, SilenceErrors warning, test-file placement note. Rejected: broader CLI sweep, subprocess test.
    </description>
    <acceptance>
- `SilenceUsage: true` set on both `agent run` and `agent doctor` cobra commands
- `SilenceErrors` left at default (false) — error line still printed by cobra
- New regression test asserts `agent run --timeout 60ss ...` returns non-nil err containing "invalid timeout"
- Test located outside agent_run_profile_test.go
- `cd cli &amp;&amp; go test -v ./cmd` passes
- Manual smoke: `ddx agent run --timeout 60ss --text x` prints only the error line + ` exit 1` (no Usage/Flags block dump)
    </acceptance>
    <labels>cli, agent, testing</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="7eeb8cddc9d54f73d3685766f21325194e9b6424">
commit 7eeb8cddc9d54f73d3685766f21325194e9b6424
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 25 17:49:07 2026 -0400

    chore: add execution evidence [20260425T214601-]

diff --git a/.ddx/executions/20260425T214601-82538bfc/manifest.json b/.ddx/executions/20260425T214601-82538bfc/manifest.json
new file mode 100644
index 00000000..e7e1fe3f
--- /dev/null
+++ b/.ddx/executions/20260425T214601-82538bfc/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260425T214601-82538bfc",
+  "bead_id": "ddx-c14f350e",
+  "base_rev": "93f35d83b94b0401bfff079cc2bab0e4d22350dc",
+  "created_at": "2026-04-25T21:46:01.690859098Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-c14f350e",
+    "title": "fix: agent run/doctor invalid --timeout should silence usage block + add regression test",
+    "description": "## Report\nExternal report at /tank/home/erik/Downloads/ddx-agent-run-invalid-timeout-exits-zero.md claims `ddx agent run --timeout 60ss ...` exits 0 on bad input, breaking wrapper exit-code inference (canonical caller: helix CLI).\n\n## Verification\nBug does NOT reproduce on current main. cli/cmd/agent_cmd.go:117-120 already returns invalid-timeout error from RunE; cli/main.go:31-43 propagates non-nil cobra errors to exit code 1. Reporter is on stale niflheim/sindri host installs.\n\n## Real gaps\n1. No regression test guards the exit-code semantic. cli/cmd/agent_run_profile_test.go only exercises happy-path `--timeout 3s`.\n2. SilenceUsage is not set on `agent run` — flag parse errors dump the entire usage block above the error message (the cosmetic part of the reporter's complaint).\n3. `agent doctor` (cli/cmd/agent_cmd.go:647) calls time.ParseDuration on its own --timeout flag and shares the same class of bug. Same one-line fix.\n\n## Plan\n(a) Set `SilenceUsage: true` on `newAgentRunCommand` (cli/cmd/agent_cmd.go:91 area) and on `newAgentDoctorCommand` (sibling, ~line 647). Per-command scope, not root — matches existing precedent (cli/cmd/init.go:99 sets SilenceUsage only; cli/cmd/doc.go:89,118 set both). Do NOT set SilenceErrors=true; keep cobra's stderr error line.\n\n(b) Add a regression test asserting `agent run --timeout 60ss --harness codex --text \"...\"` returns a non-nil err from executeCommand whose message contains `invalid timeout`. This is sufficient to catch the regression because main.go:31-43 turns any non-nil err into exit 1 — no subprocess test needed. Place the test in a neutrally named file, not agent_run_profile_test.go (the profile-specific filename is misleading for a generic flag-parse test).\n\n(c) Optional polish: assert the captured output after SilenceUsage does NOT contain `Usage:`/`Flags:` lines.\n\n(d) Do NOT extend coverage to other duration-typed inputs across the CLI; out of scope for this bug.\n\n## Out of scope\n- Helix-side fix (already done at /home/erik/Projects/helix/scripts/helix line 70 per reporter).\n- Root-level cobra UX changes.\n- Validation hardening of other duration flags elsewhere.\n\n## Refinement log\nReviewed by fresh-eyes (general-purpose subagent) and codex (`ddx agent run --harness codex`). Fresh-eyes verified main.go propagates cobra errors faithfully — command-level test is sufficient. Codex pushed for a subprocess exit-code test; rejected because main.go:31 invariant makes the command-level assertion equivalent. Both reviewers agreed per-command SilenceUsage is the right scope (matches init.go/doc.go precedent). Folded in: doctor-command parity, SilenceErrors warning, test-file placement note. Rejected: broader CLI sweep, subprocess test.",
+    "acceptance": "- `SilenceUsage: true` set on both `agent run` and `agent doctor` cobra commands\n- `SilenceErrors` left at default (false) — error line still printed by cobra\n- New regression test asserts `agent run --timeout 60ss ...` returns non-nil err containing \"invalid timeout\"\n- Test located outside agent_run_profile_test.go\n- `cd cli \u0026\u0026 go test -v ./cmd` passes\n- Manual smoke: `ddx agent run --timeout 60ss --text x` prints only the error line + ` exit 1` (no Usage/Flags block dump)",
+    "labels": [
+      "cli",
+      "agent",
+      "testing"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-25T21:46:01Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "execute-loop-heartbeat-at": "2026-04-25T21:46:01.076186883Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260425T214601-82538bfc",
+    "prompt": ".ddx/executions/20260425T214601-82538bfc/prompt.md",
+    "manifest": ".ddx/executions/20260425T214601-82538bfc/manifest.json",
+    "result": ".ddx/executions/20260425T214601-82538bfc/result.json",
+    "checks": ".ddx/executions/20260425T214601-82538bfc/checks.json",
+    "usage": ".ddx/executions/20260425T214601-82538bfc/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-c14f350e-20260425T214601-82538bfc"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260425T214601-82538bfc/result.json b/.ddx/executions/20260425T214601-82538bfc/result.json
new file mode 100644
index 00000000..4710587b
--- /dev/null
+++ b/.ddx/executions/20260425T214601-82538bfc/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-c14f350e",
+  "attempt_id": "20260425T214601-82538bfc",
+  "base_rev": "93f35d83b94b0401bfff079cc2bab0e4d22350dc",
+  "result_rev": "69f2b51390de161906931325b1ac28dbc46d6c4e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-a179a75b",
+  "duration_ms": 185355,
+  "tokens": 6585,
+  "cost_usd": 0.9146334999999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260425T214601-82538bfc",
+  "prompt_file": ".ddx/executions/20260425T214601-82538bfc/prompt.md",
+  "manifest_file": ".ddx/executions/20260425T214601-82538bfc/manifest.json",
+  "result_file": ".ddx/executions/20260425T214601-82538bfc/result.json",
+  "usage_file": ".ddx/executions/20260425T214601-82538bfc/usage.json",
+  "started_at": "2026-04-25T21:46:01.691230723Z",
+  "finished_at": "2026-04-25T21:49:07.046651043Z"
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
## Review: ddx-c14f350e iter 1

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
