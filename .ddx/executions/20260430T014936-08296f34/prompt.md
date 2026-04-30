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

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="b06d40ead414e9bf8b477132ed1e3646cff0249f">
commit b06d40ead414e9bf8b477132ed1e3646cff0249f
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 29 21:49:34 2026 -0400

    chore: add execution evidence [20260430T014806-]

diff --git a/.ddx/executions/20260430T014806-69daebee/manifest.json b/.ddx/executions/20260430T014806-69daebee/manifest.json
new file mode 100644
index 00000000..02c459dd
--- /dev/null
+++ b/.ddx/executions/20260430T014806-69daebee/manifest.json
@@ -0,0 +1,119 @@
+{
+  "attempt_id": "20260430T014806-69daebee",
+  "bead_id": "ddx-ba7334e6",
+  "base_rev": "6ab991929a606d6d9d4215dbb4348b6f33f144ae",
+  "created_at": "2026-04-30T01:48:06.976527185Z",
+  "requested": {
+    "harness": "claude",
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
+      "claimed-at": "2026-04-30T01:48:05Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2278885",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-29T23:22:10.870055396Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260429T230740-8b27cef3\",\"harness\":\"claude\",\"model\":\"sonnet\",\"input_tokens\":66,\"output_tokens\":39193,\"total_tokens\":39259,\"cost_usd\":2.79193645,\"duration_ms\":869915,\"exit_code\":0}",
+          "created_at": "2026-04-29T23:22:10.959608718Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=39259 cost_usd=2.7919 model=sonnet"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"sonnet\",\"resolved_provider\":\"claude\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-04-29T23:22:14.782655129Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "erik",
+          "body": "REQUEST_CHANGES\nImplementation of `ddx sync` and `--watch`, allowlist enforcement, no-destructive-flags, structured abort, and doctor integration are all present with tests. Missing: FEAT-NNN-sync.md spec under docs/helix/01-frame/features/ (AC8).\nartifact: .ddx/executions/20260429T230740-8b27cef3/reviewer-stream.log\nharness=claude\nmodel=opus\ninput_bytes=12759\noutput_bytes=2042\nelapsed_ms=40149",
+          "created_at": "2026-04-29T23:22:59.117945309Z",
+          "kind": "review",
+          "source": "ddx agent execute-loop",
+          "summary": "REQUEST_CHANGES"
+        },
+        {
+          "actor": "",
+          "body": "",
+          "created_at": "2026-04-29T23:22:59.213964666Z",
+          "kind": "reopen",
+          "source": "",
+          "summary": "review: REQUEST_CHANGES"
+        },
+        {
+          "actor": "erik",
+          "body": "post-merge review: REQUEST_CHANGES\nImplementation of `ddx sync` and `--watch`, allowlist enforcement, no-destructive-flags, structured abort, and doctor integration are all present with tests. Missing: FEAT-NNN-sync.md spec under docs/helix/01-frame/features/ (AC8).\nresult_rev=25cb9b127252c0631cd0922e35bbcbd0b4b40b6a\nbase_rev=d417b6d010c2a41ae9a699278b2122ad9a7a6cdc",
+          "created_at": "2026-04-29T23:22:59.303990987Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "review_request_changes"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-29T23:29:29.090478667Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-29T23:29:29.196681638Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-29T23:29:29.288008958Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "erik",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-04-29T23:29:29.465177896Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-30T01:48:05.980668333Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260430T014806-69daebee",
+    "prompt": ".ddx/executions/20260430T014806-69daebee/prompt.md",
+    "manifest": ".ddx/executions/20260430T014806-69daebee/manifest.json",
+    "result": ".ddx/executions/20260430T014806-69daebee/result.json",
+    "checks": ".ddx/executions/20260430T014806-69daebee/checks.json",
+    "usage": ".ddx/executions/20260430T014806-69daebee/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-ba7334e6-20260430T014806-69daebee"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260430T014806-69daebee/result.json b/.ddx/executions/20260430T014806-69daebee/result.json
new file mode 100644
index 00000000..6cc3ffa3
--- /dev/null
+++ b/.ddx/executions/20260430T014806-69daebee/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-ba7334e6",
+  "attempt_id": "20260430T014806-69daebee",
+  "base_rev": "6ab991929a606d6d9d4215dbb4348b6f33f144ae",
+  "result_rev": "9cbe944262a3af0ee5de885c92d483602fb78fdf",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-acb1d1a1",
+  "duration_ms": 83426,
+  "tokens": 4108,
+  "cost_usd": 0.516055,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260430T014806-69daebee",
+  "prompt_file": ".ddx/executions/20260430T014806-69daebee/prompt.md",
+  "manifest_file": ".ddx/executions/20260430T014806-69daebee/manifest.json",
+  "result_file": ".ddx/executions/20260430T014806-69daebee/result.json",
+  "usage_file": ".ddx/executions/20260430T014806-69daebee/usage.json",
+  "started_at": "2026-04-30T01:48:06.976965102Z",
+  "finished_at": "2026-04-30T01:49:30.403874348Z"
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
## Review: ddx-ba7334e6 iter 1

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
