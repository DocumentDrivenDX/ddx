<bead-review>
  <bead id="ddx-ba7334e6" iter=1>
    <title>ddx sync: first-class multi-machine repo sync as a CLI command + daemon mode</title>
    <description>
Today users with DDx checked out on multiple machines manually pull/stash/pop/commit-tracker/push to keep .ddx/beads.jsonl, .ddx/executions/, and committed plugin assets in sync. The dirty-tracker problem is constant because workers churn beads.jsonl every few minutes during execute-loop drains. Manual sync is friction; missing a sync risks divergent tracker state.

There is no good remote-only solution: routines in Anthropic cloud cannot touch a local working tree, and a server-side sync only helps if every endpoint is the server (DDx is per-project local).

REQUESTED FEATURE

1. ddx sync — one-shot command that runs the canonical flow:
   a. git fetch origin
   b. if working tree is dirty in tracked DDx-managed paths only (.ddx/beads.jsonl, .ddx/executions/, .ddx/plugins/), git stash
   c. git pull --rebase=false origin main (merge, never rebase — preserves execute-bead history per the project's standing merge policy)
   d. git stash pop
   e. stage and commit DDx-managed dirty paths (.ddx/beads.jsonl as 'chore: tracker', .ddx/executions/ as 'chore: add execution evidence')
   f. git push origin main; on non-fast-forward, retry from (a) once
   - Refuses to touch any path outside the DDx-managed allowlist.
   - Never uses --force, --no-verify, or destructive flags.
   - Aborts cleanly with a structured exit status if stash-pop conflicts or push fails twice.
   - Exit 0 = clean; non-zero = something needs human attention.

2. ddx sync --watch (or 'ddx sync daemon') — runs the above on an interval (default 15min, configurable). Local-only, runs in foreground or as a backgrounded process.

3. ddx-server can run sync as part of its loop when a project is registered with the server. The server already runs continuously per-project; adding sync to its loop costs almost nothing and turns the server into the canonical multi-machine sync hub.

4. Conflict surface: when sync aborts, doctor should pick up the issue ('ddx doctor' surfaces 'sync aborted at &lt;timestamp&gt;: &lt;reason&gt;') so users see it without having to remember to check.

5. Cross-platform: must work on macOS and Linux at minimum. No symlinks, no bash-isms. Probably implemented in Go using existing git-shell-out helpers (cli/internal/git/).

OUT OF SCOPE
- Conflict resolution beyond stash-pop (that's the preserved-iteration problem; ddx-0097af14 covers).
- Sync of paths outside the DDx-managed allowlist (intentional — never touches user code).
- Multi-remote sync (single origin only for v1).

ACCEPTANCE OF FAILURE: when something can't be auto-resolved (stash-pop conflict, double push fail, divergent base after fetch+merge), 'ddx sync' exits with a structured non-zero code and doctor surfaces it. No retries, no force, no surprise commits.

EVIDENCE THIS IS NEEDED: in the 2026-04-29 dogfood session, the operator manually invoked the sync flow ~6 times in ~3 hours while workers chewed through beads. Without sync, both machines drift; with this command, sync is one keystroke or zero (daemon).
    </description>
    <acceptance>
1. 'ddx sync' command exists and runs the canonical flow described. 2. 'ddx sync --watch [--interval=15m]' runs the flow on an interval until killed. 3. Both modes refuse to touch any path outside the DDx-managed allowlist (.ddx/beads.jsonl, .ddx/executions/, .ddx/plugins/) — verified by test that injects unrelated dirty changes and asserts they survive untouched. 4. Both modes never invoke git with --force, --no-verify, or any destructive flag — verified by test asserting on shell args. 5. Stash-pop conflict aborts with structured exit code; same for double push fail — covered by tests using fake git. 6. Doctor surfaces a recent sync failure — covered by integration test. 7. Cross-platform: tests run green on macOS and Linux CI. 8. Documentation in docs/helix/01-frame/features/ — likely a new FEAT-NNN-sync.md spec lands first per the standing 'specs first → beads' rule.
    </acceptance>
    <labels>area:cli, kind:feature, area:sync, quality-of-life</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T230740-8b27cef3/manifest.json</file>
    <file>.ddx/executions/20260429T230740-8b27cef3/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="25cb9b127252c0631cd0922e35bbcbd0b4b40b6a">
diff --git a/.ddx/executions/20260429T230740-8b27cef3/manifest.json b/.ddx/executions/20260429T230740-8b27cef3/manifest.json
new file mode 100644
index 00000000..462e427a
--- /dev/null
+++ b/.ddx/executions/20260429T230740-8b27cef3/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260429T230740-8b27cef3",
+  "bead_id": "ddx-ba7334e6",
+  "base_rev": "d417b6d010c2a41ae9a699278b2122ad9a7a6cdc",
+  "created_at": "2026-04-29T23:07:40.949292378Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-ba7334e6",
+    "title": "ddx sync: first-class multi-machine repo sync as a CLI command + daemon mode",
+    "description": "Today users with DDx checked out on multiple machines manually pull/stash/pop/commit-tracker/push to keep .ddx/beads.jsonl, .ddx/executions/, and committed plugin assets in sync. The dirty-tracker problem is constant because workers churn beads.jsonl every few minutes during execute-loop drains. Manual sync is friction; missing a sync risks divergent tracker state.\n\nThere is no good remote-only solution: routines in Anthropic cloud cannot touch a local working tree, and a server-side sync only helps if every endpoint is the server (DDx is per-project local).\n\nREQUESTED FEATURE\n\n1. ddx sync — one-shot command that runs the canonical flow:\n   a. git fetch origin\n   b. if working tree is dirty in tracked DDx-managed paths only (.ddx/beads.jsonl, .ddx/executions/, .ddx/plugins/), git stash\n   c. git pull --rebase=false origin main (merge, never rebase — preserves execute-bead history per the project's standing merge policy)\n   d. git stash pop\n   e. stage and commit DDx-managed dirty paths (.ddx/beads.jsonl as 'chore: tracker', .ddx/executions/ as 'chore: add execution evidence')\n   f. git push origin main; on non-fast-forward, retry from (a) once\n   - Refuses to touch any path outside the DDx-managed allowlist.\n   - Never uses --force, --no-verify, or destructive flags.\n   - Aborts cleanly with a structured exit status if stash-pop conflicts or push fails twice.\n   - Exit 0 = clean; non-zero = something needs human attention.\n\n2. ddx sync --watch (or 'ddx sync daemon') — runs the above on an interval (default 15min, configurable). Local-only, runs in foreground or as a backgrounded process.\n\n3. ddx-server can run sync as part of its loop when a project is registered with the server. The server already runs continuously per-project; adding sync to its loop costs almost nothing and turns the server into the canonical multi-machine sync hub.\n\n4. Conflict surface: when sync aborts, doctor should pick up the issue ('ddx doctor' surfaces 'sync aborted at \u003ctimestamp\u003e: \u003creason\u003e') so users see it without having to remember to check.\n\n5. Cross-platform: must work on macOS and Linux at minimum. No symlinks, no bash-isms. Probably implemented in Go using existing git-shell-out helpers (cli/internal/git/).\n\nOUT OF SCOPE\n- Conflict resolution beyond stash-pop (that's the preserved-iteration problem; ddx-0097af14 covers).\n- Sync of paths outside the DDx-managed allowlist (intentional — never touches user code).\n- Multi-remote sync (single origin only for v1).\n\nACCEPTANCE OF FAILURE: when something can't be auto-resolved (stash-pop conflict, double push fail, divergent base after fetch+merge), 'ddx sync' exits with a structured non-zero code and doctor surfaces it. No retries, no force, no surprise commits.\n\nEVIDENCE THIS IS NEEDED: in the 2026-04-29 dogfood session, the operator manually invoked the sync flow ~6 times in ~3 hours while workers chewed through beads. Without sync, both machines drift; with this command, sync is one keystroke or zero (daemon).",
+    "acceptance": "1. 'ddx sync' command exists and runs the canonical flow described. 2. 'ddx sync --watch [--interval=15m]' runs the flow on an interval until killed. 3. Both modes refuse to touch any path outside the DDx-managed allowlist (.ddx/beads.jsonl, .ddx/executions/, .ddx/plugins/) — verified by test that injects unrelated dirty changes and asserts they survive untouched. 4. Both modes never invoke git with --force, --no-verify, or any destructive flag — verified by test asserting on shell args. 5. Stash-pop conflict aborts with structured exit code; same for double push fail — covered by tests using fake git. 6. Doctor surfaces a recent sync failure — covered by integration test. 7. Cross-platform: tests run green on macOS and Linux CI. 8. Documentation in docs/helix/01-frame/features/ — likely a new FEAT-NNN-sync.md spec lands first per the standing 'specs first → beads' rule.",
+    "labels": [
+      "area:cli",
+      "kind:feature",
+      "area:sync",
+      "quality-of-life"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T23:07:38Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T23:07:38.054784335Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T230740-8b27cef3",
+    "prompt": ".ddx/executions/20260429T230740-8b27cef3/prompt.md",
+    "manifest": ".ddx/executions/20260429T230740-8b27cef3/manifest.json",
+    "result": ".ddx/executions/20260429T230740-8b27cef3/result.json",
+    "checks": ".ddx/executions/20260429T230740-8b27cef3/checks.json",
+    "usage": ".ddx/executions/20260429T230740-8b27cef3/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-ba7334e6-20260429T230740-8b27cef3"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T230740-8b27cef3/result.json b/.ddx/executions/20260429T230740-8b27cef3/result.json
new file mode 100644
index 00000000..d2091f6c
--- /dev/null
+++ b/.ddx/executions/20260429T230740-8b27cef3/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-ba7334e6",
+  "attempt_id": "20260429T230740-8b27cef3",
+  "base_rev": "d417b6d010c2a41ae9a699278b2122ad9a7a6cdc",
+  "result_rev": "16ed11ecc5bec6435cfa3e5ec8a91fc41a8f1375",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-a1132ea8",
+  "duration_ms": 869915,
+  "tokens": 39259,
+  "cost_usd": 2.79193645,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T230740-8b27cef3",
+  "prompt_file": ".ddx/executions/20260429T230740-8b27cef3/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T230740-8b27cef3/manifest.json",
+  "result_file": ".ddx/executions/20260429T230740-8b27cef3/result.json",
+  "usage_file": ".ddx/executions/20260429T230740-8b27cef3/usage.json",
+  "started_at": "2026-04-29T23:07:40.949617836Z",
+  "finished_at": "2026-04-29T23:22:10.865005777Z"
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
