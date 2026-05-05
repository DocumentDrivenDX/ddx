<bead-review>
  <bead id="ddx-ae4b7393" iter=1>
    <title>checks: backfill production-reachability — internal/evidence (8 unreached)</title>
    <description>
Decomposed from REACH-BACKFILL (ddx-83440482). The Go production-reachability check (library/checks/go-production-reachability) flagged 8 symbol(s) in package `internal/evidence` as unreachable from cli/ entry roots (deadcode RTA).

Symbols:
- internal/evidence/read.go:24 — OversizeError.Error
- internal/evidence/read.go:29 — OversizeError.Unwrap
- internal/evidence/read.go:70 — ReadFileHardFail
- internal/evidence/sections.go:51 — FitSections
- internal/evidence/sections.go:139 — capContent
- internal/evidence/sections.go:153 — trimToLineBudget
- internal/evidence/strategy.go:19 — AssembleRefOnly
- internal/evidence/strategy.go:50 — AssembleInline

For each: WIRE (add to production call graph) or DELETE (genuinely obsolete). If neither is clear within ~15 min, annotate `// wiring:pending &lt;follow-up-bead-id&gt;` and file a follow-up bead.

Decision rule: if the originating bead's AC describes desired runtime behavior → WIRE; if speculative or design has moved on → DELETE.

Initial-violations evidence: .ddx/executions/20260503T124553-282667f7/initial-violations.json
    </description>
    <acceptance>
1. Each of the 8 listed symbols is either wired into the production call graph reachable from main() OR deleted (with originating bead reopened/closed-as-obsolete as appropriate).
2. Any remaining wiring:pending annotations cite open follow-up beads.
3. After landing: deadcode RTA (`go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` from cli/) reports zero remaining dead symbols in `internal/evidence`.
4. cd cli &amp;&amp; go test ./... green.
5. Decisions log written to .ddx/executions/&lt;run-id&gt;/decisions.md (one line per symbol: WIRE|DELETE|PENDING &lt;reason&gt;).
    </acceptance>
    <labels>phase:2, area:checks, kind:backfill</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T061703-ac727539/checks/production-reachability.json</file>
    <file>.ddx/executions/20260505T061703-ac727539/manifest.json</file>
    <file>.ddx/executions/20260505T061703-ac727539/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="c645af10a20edf68228cfe14d58536ed7304826c">
diff --git a/.ddx/executions/20260505T061703-ac727539/checks/production-reachability.json b/.ddx/executions/20260505T061703-ac727539/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260505T061703-ac727539/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T061703-ac727539/manifest.json b/.ddx/executions/20260505T061703-ac727539/manifest.json
new file mode 100644
index 00000000..6de371ca
--- /dev/null
+++ b/.ddx/executions/20260505T061703-ac727539/manifest.json
@@ -0,0 +1,834 @@
+{
+  "attempt_id": "20260505T061703-ac727539",
+  "bead_id": "ddx-ae4b7393",
+  "base_rev": "a6dfb3651f1a5a03b154ce0453b3bd4a0fc1d0f5",
+  "created_at": "2026-05-05T06:17:05.514714161Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-ae4b7393",
+    "title": "checks: backfill production-reachability — internal/evidence (8 unreached)",
+    "description": "Decomposed from REACH-BACKFILL (ddx-83440482). The Go production-reachability check (library/checks/go-production-reachability) flagged 8 symbol(s) in package `internal/evidence` as unreachable from cli/ entry roots (deadcode RTA).\n\nSymbols:\n- internal/evidence/read.go:24 — OversizeError.Error\n- internal/evidence/read.go:29 — OversizeError.Unwrap\n- internal/evidence/read.go:70 — ReadFileHardFail\n- internal/evidence/sections.go:51 — FitSections\n- internal/evidence/sections.go:139 — capContent\n- internal/evidence/sections.go:153 — trimToLineBudget\n- internal/evidence/strategy.go:19 — AssembleRefOnly\n- internal/evidence/strategy.go:50 — AssembleInline\n\nFor each: WIRE (add to production call graph) or DELETE (genuinely obsolete). If neither is clear within ~15 min, annotate `// wiring:pending \u003cfollow-up-bead-id\u003e` and file a follow-up bead.\n\nDecision rule: if the originating bead's AC describes desired runtime behavior → WIRE; if speculative or design has moved on → DELETE.\n\nInitial-violations evidence: .ddx/executions/20260503T124553-282667f7/initial-violations.json",
+    "acceptance": "1. Each of the 8 listed symbols is either wired into the production call graph reachable from main() OR deleted (with originating bead reopened/closed-as-obsolete as appropriate).\n2. Any remaining wiring:pending annotations cite open follow-up beads.\n3. After landing: deadcode RTA (`go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` from cli/) reports zero remaining dead symbols in `internal/evidence`.\n4. cd cli \u0026\u0026 go test ./... green.\n5. Decisions log written to .ddx/executions/\u003crun-id\u003e/decisions.md (one line per symbol: WIRE|DELETE|PENDING \u003creason\u003e).",
+    "parent": "ddx-83440482",
+    "labels": [
+      "phase:2",
+      "area:checks",
+      "kind:backfill"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T06:17:03Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:50:34.474776443Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:53:21.992996948Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:56:08.134905524Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T12:58:57.38986382Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:01:49.612097441Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:04:44.267724007Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:07:38.820803344Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:10:31.005737239Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:13:23.02678708Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:16:14.933510217Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:19:07.598699709Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:21:59.973875208Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:24:52.758531822Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:27:45.200784335Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:30:38.005629291Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:33:30.351380561Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:36:23.102315195Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:39:16.334921158Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:42:09.42477121Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:45:02.971860043Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:47:55.304024317Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:50:48.153450336Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:53:42.812386711Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:56:40.335065792Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T13:59:37.444020323Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:02:35.149488753Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:05:32.818847742Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:08:30.922900647Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:11:28.995762632Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:14:27.325410588Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:17:25.13288368Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:20:23.243800285Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:23:21.103955288Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:26:19.2202702Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:29:17.756908501Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:32:15.871319953Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:35:14.746672037Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:38:12.451491297Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:41:10.762452072Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:44:08.790153828Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:47:07.942077873Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:50:05.381812105Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:53:03.318370455Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T14:56:01.308710615Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T15:00:35.108524214Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T15:05:36.137451271Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T15:10:37.005795282Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T15:15:38.010631912Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T15:20:39.021890544Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T15:25:40.130530061Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T15:30:41.284932663Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T15:35:42.351306664Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T15:40:43.183386083Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T15:45:43.815689186Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T15:50:44.856362552Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T15:55:45.85187451Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T16:00:46.997548925Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T16:05:48.072664179Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T16:10:53.388483566Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T16:15:55.698577479Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T16:20:57.217213323Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T16:26:11.943371617Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T16:31:21.886290554Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T16:36:45.076994416Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T16:41:59.675304408Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T16:47:14.400355712Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T16:52:28.930634111Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T16:57:43.715431186Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:02:58.275373188Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:08:12.655715638Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:13:36.530674974Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:18:55.427816217Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:24:14.303932931Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:29:33.341637065Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:34:52.205722636Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:40:11.371494486Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:45:30.635366112Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:50:49.463836607Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:56:08.751907814Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:01:28.252702882Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:06:47.855706949Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:12:07.474814848Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:17:27.388545461Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:22:47.239914399Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:28:07.026702305Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:33:26.69727361Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:38:46.432090811Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:44:06.394600644Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:49:26.005391746Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:54:45.883995991Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T19:00:05.847648346Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T19:05:25.563910552Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T19:10:45.446716705Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T19:16:05.155206268Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T19:21:25.110005082Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T19:26:44.949220025Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T19:32:04.837685665Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\",\"fallback_chain\":[]}",
+          "created_at": "2026-05-04T22:04:55.984532903Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "erik",
+          "body": "exit status 1\nresult_rev=7ffd3ac95dd4921ffe7a7fde380892d2f19b9a51\nbase_rev=7ffd3ac95dd4921ffe7a7fde380892d2f19b9a51\nretry_after=2026-05-05T04:04:56Z",
+          "created_at": "2026-05-04T22:04:56.716894995Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T06:17:03.577986608Z",
+      "execute-loop-last-detail": "exit status 1",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-05-05T04:04:56Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T061703-ac727539",
+    "prompt": ".ddx/executions/20260505T061703-ac727539/prompt.md",
+    "manifest": ".ddx/executions/20260505T061703-ac727539/manifest.json",
+    "result": ".ddx/executions/20260505T061703-ac727539/result.json",
+    "checks": ".ddx/executions/20260505T061703-ac727539/checks.json",
+    "usage": ".ddx/executions/20260505T061703-ac727539/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-ae4b7393-20260505T061703-ac727539"
+  },
+  "prompt_sha": "74428f535bf188a5a2469be4a8b2419c54a27e04dddf0143e6fb52466511cc7d"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T061703-ac727539/result.json b/.ddx/executions/20260505T061703-ac727539/result.json
new file mode 100644
index 00000000..3fd587f8
--- /dev/null
+++ b/.ddx/executions/20260505T061703-ac727539/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-ae4b7393",
+  "attempt_id": "20260505T061703-ac727539",
+  "base_rev": "a6dfb3651f1a5a03b154ce0453b3bd4a0fc1d0f5",
+  "result_rev": "e03b2912bcd0f5badd2257c00a849b559339f39a",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-ef135ac4",
+  "duration_ms": 455420,
+  "tokens": 6713169,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T061703-ac727539",
+  "prompt_file": ".ddx/executions/20260505T061703-ac727539/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T061703-ac727539/manifest.json",
+  "result_file": ".ddx/executions/20260505T061703-ac727539/result.json",
+  "usage_file": ".ddx/executions/20260505T061703-ac727539/usage.json",
+  "started_at": "2026-05-05T06:17:05.515327035Z",
+  "finished_at": "2026-05-05T06:24:40.935420056Z"
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
