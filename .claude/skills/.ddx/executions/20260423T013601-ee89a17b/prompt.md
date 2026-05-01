<bead-review>
  <bead id="ddx-15f7ee0b" iter=1>
    <title>Test runs pollute ~/.local/share/ddx/server-state.json with thousands of /tmp/Test.../001 projects</title>
    <description>
## Observed

`~/.local/share/ddx/server-state.json` on a developer machine contains **7,343 registered projects**. Composition:

- **6,936 named "001"** — all are Go test temp dirs like `/tmp/TestAgentCapabilitiesCommandJSON1451829170/001`, `/tmp/TestAgentCheckSuccess3666443068/001`.
- **344 named "002"** — same pattern in tests that use `002/`.
- **~60 real projects** (ddx, helix, axon, plus per-branch worktrees).
- Only 135 of the 6,936 are marked unreachable; the rest are "alive" because their `/tmp` dir still exists or because `LastSeen` is recent.
- State file size: 1.8 MB.

## Root cause

1. Go tests that exercise the server (or anything that wires up a `ServerState`) call `RegisterProject` with a temp-dir path. The state file they write to is **not isolated** — it defaults to the shared XDG location (`serverAddrDir()` at `cli/internal/server/server.go:67`) unless the test explicitly overrides `XDG_DATA_HOME` or constructs state via the `tc.StateDir` injection path.
2. There is no test-side deregistration on teardown. Temp dirs get cleaned up by `t.TempDir()`'s own finalizer, but the project entry in the production state file stays.
3. The reachability sweep (`ServerState.migrate` at `cli/internal/server/state.go:85-148`) tombstones missing-path entries and drops them 24h later. With thousands of entries being added per test run, the rate of addition outpaces the 24h GC.

## Perf amplifier (cross-cutting)

Both `GetBeadSnapshots` (`state_graphql.go:49-105`) and `GetAgentSessionsGraphQL` (`state_graphql.go:476-489`) iterate every registered project per request. With N=7343 that's ~7k `os.Stat` + `store.ReadAll` attempts per bead-list or sessions query, mostly against dead paths. This is part of why `ddx-9ce6842a` (beads perf) and `ddx-2ceb02fa` (sessions feed) feel slow in practice even before their own N·M bugs are considered.

## Proposed direction

Three independently-landable fixes:

### Fix A — Test isolation (highest leverage)

- Audit every test that calls `ServerState` / `RegisterProject` / boots a server. Each one must point at an isolated state dir (`t.TempDir()` + `XDG_DATA_HOME` override, or direct `tc.StateDir` injection).
- Add a linting test: parse `cli/internal/server/**/*_test.go` for constructions of `ServerState` or HTTP server test helpers and assert each uses an isolated state dir. Fail CI otherwise.
- This is the real fix. The rest is damage mitigation.

### Fix B — Smarter sweep (defense in depth)

- Sweep: any path under `/tmp`, `/private/tmp`, `/var/folders/*/T/` (macOS test temp root), or matching `/tmp/Test*` should be treated as a **candidate test-dir project** and dropped immediately on first sweep that finds it missing — no 24h tombstone.
- Sweep should also drop entries where `Path` contains any `Test[A-Z]\w+\d+` segment (the Go testing convention), regardless of whether the dir currently exists. These are never real projects.
- Unit test: seed a state with mixed real + fake-test-dir entries, run sweep, assert only the real ones remain.

### Fix C — Manual cleanup CLI

- `ddx server state prune` — read state file, apply Fix B's rules, write backup to `~/.local/share/ddx/server-state.json.bak-&lt;timestamp&gt;`, write cleaned state, print summary (X dropped, Y kept).
- Useful for recovery on machines that have already accumulated pollution — like the reporter's. Keep even after Fix A lands.
- Dry-run flag prints the summary without writing.

## Out of scope

- Moving state to SQLite.
- Per-user project management UI. (Follow-up if the projects list becomes a real operator surface.)
- Renaming "001" away from a confusing default. The name field is just `filepath.Base(path)`; fixing Fix A makes this question moot.
    </description>
    <acceptance>
**User story:** As a developer running `go test`, my test runs do not leave behind registered projects in my real `~/.local/share/ddx/server-state.json`. When a machine has already been polluted, I can prune it with one command. The server sweep is aggressive enough that obvious test-dir pollution does not survive a restart.

**Acceptance criteria:**

1. **Fix A — test isolation enforced by CI.**
   - Every existing test that instantiates `ServerState` or boots a server helper is migrated to an isolated state dir. No test writes to `~/.local/share/ddx/server-state.json`.
   - A lint test (Go or shell) parses `cli/internal/server/**/*_test.go` (and any other directories that wire up `ServerState`) and fails if it finds a construction that does not pass an explicit isolated state path or override `XDG_DATA_HOME` first. Test cases added for both the allowed-pattern and the disallowed-pattern.

2. **Fix B — sweep recognizes test-dir pollution.**
   - `ServerState.migrate` drops entries whose `Path` matches any of:
     - prefix `/tmp/`, `/private/tmp/`, `/var/folders/`
     - regex `/Test[A-Z][A-Za-z0-9_]*\d+/` anywhere in the path
   - Missing paths matching these patterns are dropped on first sweep (no 24h tombstone).
   - Reachable paths matching these patterns are still dropped (they are never real projects — the test dir may exist transiently but is never the user's registered project).
   - Unit test seeds a fixture state with one real project, one `/tmp/TestFoo123/001` missing, one `/tmp/TestBar456/002` still on disk, and one `/private/tmp/TestMac789/001`; asserts only the real project survives.

3. **Fix C — pruning CLI.**
   - `ddx server state prune [--dry-run] [--state &lt;path&gt;]` applies the Fix B rules to an on-disk state file.
   - Writes `server-state.json.bak-&lt;YYYYMMDDTHHMMSS&gt;` before overwriting.
   - Prints summary: `Pruned X of Y entries (Z kept)`. Dry-run prints the summary without writing.
   - Integration test uses a fixture state containing 10 real + 100 fake entries, runs the command, asserts 10 remain and a backup file with exactly 110 entries was written.

4. **Perf amplifier — regression test.**
   - A test asserts `GetBeadSnapshots` does **not** call `os.Stat` on any project already tombstoned or pattern-filtered. (Implementation detail: sweep on startup + skip tombstoned in the hot loop.)
   - If Fix A+B are in place, a fresh `go test ./...` run followed by a server restart produces a state file with ≤ 20 projects on a typical dev machine. Asserted via a CI job that runs the tests, starts the server briefly, inspects the state file, and fails if test-dir patterns are present.

5. **Backward compatibility.**
   - The prune CLI is safe to run on an existing polluted state file. The sweep upgrade is safe on restart — no data loss for real projects.
   - On upgrade, a note is logged: "Pruned X phantom test-dir projects from state file."

6. **Cross-reference.**
   - Bead notes link to `ddx-9ce6842a` (beads perf) and `ddx-2ceb02fa` (sessions feed) as the two surfaces whose perf problems are amplified by this bug. Those beads' fixture sizing assumptions were correct for a clean state file; the operator experience will still feel slow on a polluted machine until Fix C is run.
    </acceptance>
    <labels>feat-020, feat-008, testing, hygiene, perf-amplifier</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="034fbf8fe4bd674b75a43eb060307111edfe97d8">
commit 034fbf8fe4bd674b75a43eb060307111edfe97d8
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 21:35:59 2026 -0400

    chore: add execution evidence [20260423T010616-]

diff --git a/.ddx/executions/20260423T010616-b2d3b939/result.json b/.ddx/executions/20260423T010616-b2d3b939/result.json
new file mode 100644
index 00000000..578236cd
--- /dev/null
+++ b/.ddx/executions/20260423T010616-b2d3b939/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-15f7ee0b",
+  "attempt_id": "20260423T010616-b2d3b939",
+  "base_rev": "be036de7bf38f331013539b1d8e8cbd53cac33d1",
+  "result_rev": "af88d432737f3bcd5a8cd614ec8e4b731cac0b16",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-66c955d5",
+  "duration_ms": 1781372,
+  "tokens": 87591,
+  "cost_usd": 15.080633749999993,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260423T010616-b2d3b939",
+  "prompt_file": ".ddx/executions/20260423T010616-b2d3b939/prompt.md",
+  "manifest_file": ".ddx/executions/20260423T010616-b2d3b939/manifest.json",
+  "result_file": ".ddx/executions/20260423T010616-b2d3b939/result.json",
+  "usage_file": ".ddx/executions/20260423T010616-b2d3b939/usage.json",
+  "started_at": "2026-04-23T01:06:17.137422925Z",
+  "finished_at": "2026-04-23T01:35:58.510189402Z"
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
## Review: ddx-15f7ee0b iter 1

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
