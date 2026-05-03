<bead-review>
  <bead id="ddx-ea76e1b8" iter=1>
    <title>checks: ship Go production-reachability check (library/checks/go-production-reachability)</title>
    <description>
Ship the Go-specific production-reachability check as a library resource conforming to the REACH-PROTO check protocol. NOT installed by default for arbitrary projects (Go-opinion would leak through ddx init). This project (ddx itself) opts in by adding .ddx/checks/production-reachability.yaml.

DESIGN
- Location: library/checks/go-production-reachability/ (alongside library/personas/, library/patterns/)
- Implementation: a small Go binary or shell script invoking golang.org/x/tools/cmd/deadcode rooted at configurable entry packages.
- Default entry roots for ddx self-use: cli/cmd/..., cli/internal/server/graphql/..., cli/internal/server/http/...
- Logic:
  1. Read DIFF_BASE..DIFF_HEAD from env. Compute set of new Go symbols (functions, methods, types) added by the diff via AST parse of changed files.
  2. Run deadcode (or callgraph) rooted at the configured entry packages, EXCLUDING _test.go from roots and from reachability.
  3. For each new symbol not reachable from any entry root → violation { file, line, symbol, kind: "unreachable", detail: "no path from entry roots: &lt;list&gt;" }
  4. Recognize // wiring:pending &lt;bead-id&gt; annotation as escape hatch for individual symbols. The check verifies the named bead exists and is open via local beads.jsonl read; otherwise the annotation is rejected.
  5. Write ${EVIDENCE_DIR}/production-reachability.json with the protocol-defined schema.
- Config: library/checks/go-production-reachability/check.yaml template that operators copy to .ddx/checks/ and customize entry_roots for their project.

DOGFOOD
- This project installs it: write .ddx/checks/production-reachability.yaml referencing the library check + cli's entry roots.
- The next ddx try on any bead touching cli/**/*.go will exercise the check.

NOT IN SCOPE
- Auto-install during ddx init (would leak Go-opinion). Operators opt in manually or via ddx install check (future, separate bead).
- Coverage-based "exercised via CLI" check (entered_via flavor). Static reachability is sufficient for the immediate problem.
- Python/TS/Rust analogues. Future contributions; protocol supports them.

DEP: REACH-PROTO (above)
    </description>
    <acceptance>
1. library/checks/go-production-reachability/ exists with: README.md (usage), check.yaml (template), implementation (binary or script).
2. Implementation reads DIFF_BASE/DIFF_HEAD/PROJECT_ROOT/EVIDENCE_DIR from env; computes new symbols from diff; runs deadcode-rooted reachability; writes result.json conforming to REACH-PROTO schema.
3. // wiring:pending &lt;bead-id&gt; annotation parsed; bead existence + open-status verified against .ddx/beads.jsonl; invalid annotation = violation.
4. _test.go files excluded from both root selection and reachability calculation (so a unit-test-only reference does NOT satisfy the check).
5. Self-test: synthetic project with one unwired exported function returns status=block with that symbol in violations.
6. Self-test: same project with the function wired into cmd handler returns status=pass.
7. Self-test: same project with // wiring:pending annotation citing an open bead returns status=pass; annotation citing a closed/missing bead returns status=block.
8. This project installs it: .ddx/checks/production-reachability.yaml committed; ddx ac run on any current bead succeeds (or BLOCKs with the known unwired functions — see REACH-BACKFILL for resolution).
9. cd cli &amp;&amp; go test green; lefthook pre-commit passes.
10. Evidence file at .ddx/executions/&lt;run-id&gt;/production-reachability.json shows the check ran and its findings.
    </acceptance>
    <labels>phase:2, area:checks, area:library, kind:check-impl, prevention</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T123048-397071dc/manifest.json</file>
    <file>.ddx/executions/20260503T123048-397071dc/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="3e58316310e9cf2122afed341ec0ca0a63b6f66d">
diff --git a/.ddx/executions/20260503T123048-397071dc/result.json b/.ddx/executions/20260503T123048-397071dc/result.json
new file mode 100644
index 00000000..d5a8df88
--- /dev/null
+++ b/.ddx/executions/20260503T123048-397071dc/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-ea76e1b8",
+  "attempt_id": "20260503T123048-397071dc",
+  "base_rev": "eec3052eee409114c949529a7eafec84840b69be",
+  "result_rev": "1605816fbc23b99d769e8ee8b391f1aefc9ec3d5",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-eda8dfc9",
+  "duration_ms": 799429,
+  "tokens": 33645,
+  "cost_usd": 3.8090468999999993,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T123048-397071dc",
+  "prompt_file": ".ddx/executions/20260503T123048-397071dc/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T123048-397071dc/manifest.json",
+  "result_file": ".ddx/executions/20260503T123048-397071dc/result.json",
+  "usage_file": ".ddx/executions/20260503T123048-397071dc/usage.json",
+  "started_at": "2026-05-03T12:30:49.976708767Z",
+  "finished_at": "2026-05-03T12:44:09.405942916Z"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260503T123048-397071dc/manifest.json b/.ddx/executions/20260503T123048-397071dc/manifest.json
new file mode 100644
index 00000000..bb37750d
--- /dev/null
+++ b/.ddx/executions/20260503T123048-397071dc/manifest.json
@@ -0,0 +1,901 @@
+{
+  "attempt_id": "20260503T123048-397071dc",
+  "bead_id": "ddx-ea76e1b8",
+  "base_rev": "eec3052eee409114c949529a7eafec84840b69be",
+  "created_at": "2026-05-03T12:30:49.975023185Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-ea76e1b8",
+    "title": "checks: ship Go production-reachability check (library/checks/go-production-reachability)",
+    "description": "Ship the Go-specific production-reachability check as a library resource conforming to the REACH-PROTO check protocol. NOT installed by default for arbitrary projects (Go-opinion would leak through ddx init). This project (ddx itself) opts in by adding .ddx/checks/production-reachability.yaml.\n\nDESIGN\n- Location: library/checks/go-production-reachability/ (alongside library/personas/, library/patterns/)\n- Implementation: a small Go binary or shell script invoking golang.org/x/tools/cmd/deadcode rooted at configurable entry packages.\n- Default entry roots for ddx self-use: cli/cmd/..., cli/internal/server/graphql/..., cli/internal/server/http/...\n- Logic:\n  1. Read DIFF_BASE..DIFF_HEAD from env. Compute set of new Go symbols (functions, methods, types) added by the diff via AST parse of changed files.\n  2. Run deadcode (or callgraph) rooted at the configured entry packages, EXCLUDING _test.go from roots and from reachability.\n  3. For each new symbol not reachable from any entry root → violation { file, line, symbol, kind: \"unreachable\", detail: \"no path from entry roots: \u003clist\u003e\" }\n  4. Recognize // wiring:pending \u003cbead-id\u003e annotation as escape hatch for individual symbols. The check verifies the named bead exists and is open via local beads.jsonl read; otherwise the annotation is rejected.\n  5. Write ${EVIDENCE_DIR}/production-reachability.json with the protocol-defined schema.\n- Config: library/checks/go-production-reachability/check.yaml template that operators copy to .ddx/checks/ and customize entry_roots for their project.\n\nDOGFOOD\n- This project installs it: write .ddx/checks/production-reachability.yaml referencing the library check + cli's entry roots.\n- The next ddx try on any bead touching cli/**/*.go will exercise the check.\n\nNOT IN SCOPE\n- Auto-install during ddx init (would leak Go-opinion). Operators opt in manually or via ddx install check (future, separate bead).\n- Coverage-based \"exercised via CLI\" check (entered_via flavor). Static reachability is sufficient for the immediate problem.\n- Python/TS/Rust analogues. Future contributions; protocol supports them.\n\nDEP: REACH-PROTO (above)",
+    "acceptance": "1. library/checks/go-production-reachability/ exists with: README.md (usage), check.yaml (template), implementation (binary or script).\n2. Implementation reads DIFF_BASE/DIFF_HEAD/PROJECT_ROOT/EVIDENCE_DIR from env; computes new symbols from diff; runs deadcode-rooted reachability; writes result.json conforming to REACH-PROTO schema.\n3. // wiring:pending \u003cbead-id\u003e annotation parsed; bead existence + open-status verified against .ddx/beads.jsonl; invalid annotation = violation.\n4. _test.go files excluded from both root selection and reachability calculation (so a unit-test-only reference does NOT satisfy the check).\n5. Self-test: synthetic project with one unwired exported function returns status=block with that symbol in violations.\n6. Self-test: same project with the function wired into cmd handler returns status=pass.\n7. Self-test: same project with // wiring:pending annotation citing an open bead returns status=pass; annotation citing a closed/missing bead returns status=block.\n8. This project installs it: .ddx/checks/production-reachability.yaml committed; ddx ac run on any current bead succeeds (or BLOCKs with the known unwired functions — see REACH-BACKFILL for resolution).\n9. cd cli \u0026\u0026 go test green; lefthook pre-commit passes.\n10. Evidence file at .ddx/executions/\u003crun-id\u003e/production-reachability.json shows the check ran and its findings.",
+    "parent": "ddx-e34994e2",
+    "labels": [
+      "phase:2",
+      "area:checks",
+      "area:library",
+      "kind:check-impl",
+      "prevention"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-03T12:30:48Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "463338",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-05-03T03:10:42.713398818Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "erik",
+          "body": "exit status 1\nresult_rev=db90b88709366250f00682cdd1a5fa842027ac9c\nbase_rev=db90b88709366250f00682cdd1a5fa842027ac9c\nretry_after=2026-05-03T09:10:42Z",
+          "created_at": "2026-05-03T03:10:42.823261686Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:10:48.26191923Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:11:34.227718175Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:12:29.408038086Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:13:31.262816545Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:14:44.737988969Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:16:07.232615618Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:17:43.269159056Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:19:37.759913783Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:21:34.730770328Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:23:31.512606605Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:25:28.464697797Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:27:25.427267553Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:29:22.561701008Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:31:19.607363868Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:33:16.575274395Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:35:13.244181398Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:37:10.26128752Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:39:07.016606224Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:41:03.95908966Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:43:00.683388Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:44:57.376005152Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:46:54.363661757Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:48:51.236201097Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:50:48.293330069Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:52:45.493812189Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:54:42.45888245Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:56:39.249429334Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:58:36.471969406Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:00:33.839501371Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:02:30.882693835Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:04:28.229687295Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:06:25.005869272Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:08:22.139854232Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:10:19.339202205Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:12:16.273684546Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:14:13.549542733Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:16:10.641263978Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:18:07.631843149Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:20:04.833626687Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:22:01.649431047Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:23:58.931336223Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:25:56.420269802Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:27:53.440847377Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:29:50.650754223Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:31:47.824859857Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:33:44.98453343Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:35:42.234826174Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:37:39.688500213Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:39:37.123658794Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:41:34.644066805Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:43:32.004338355Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:45:29.351433833Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:47:26.492713883Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:49:24.033216738Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:51:21.65495773Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:53:16.569723514Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:55:14.259289752Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:57:11.504932493Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:59:08.760435147Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:01:06.19889773Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:03:05.92343941Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:05:05.448116955Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:07:06.888235363Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:09:06.027673509Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:11:03.358816415Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:13:00.768685252Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:14:57.74038862Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:16:56.573320438Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:18:55.711719662Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:20:55.417405605Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:22:55.414746654Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:24:52.878157922Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:26:50.20770086Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:28:48.127249278Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:30:46.544388119Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:32:43.804378064Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:34:41.262489801Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:36:39.121393848Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:38:36.61915782Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:40:36.256402005Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:42:33.655668067Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:44:31.731344219Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:46:30.04481565Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:48:27.695445936Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:50:26.32681877Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:52:24.181293168Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:54:21.373075742Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:56:18.577874588Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:58:15.988309366Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:00:14.301600188Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:02:12.661998901Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:04:10.746554064Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:06:08.61076677Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:08:06.789627181Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:10:04.939585892Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:12:04.563007977Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:14:02.765066809Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:16:00.719669704Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:17:58.738367799Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:19:56.929401559Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:21:55.171239511Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:23:53.228839011Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:25:51.839524825Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:27:49.942855766Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:29:47.927555535Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-03T12:30:48.366850997Z",
+      "execute-loop-last-detail": "exit status 1",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-05-03T09:10:42Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260503T123048-397071dc",
+    "prompt": ".ddx/executions/20260503T123048-397071dc/prompt.md",
+    "manifest": ".ddx/executions/20260503T123048-397071dc/manifest.json",
+    "result": ".ddx/executions/20260503T123048-397071dc/result.json",
+    "checks": ".ddx/executions/20260503T123048-397071dc/checks.json",
+    "usage": ".ddx/executions/20260503T123048-397071dc/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-ea76e1b8-20260503T123048-397071dc"
+  },
+  "prompt_sha": "ddadeda84c774369b4549992ecf9d8f234a7fa55d79ffda36a224505c108e6a8"
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
