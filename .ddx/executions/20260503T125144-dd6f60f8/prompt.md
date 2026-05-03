<bead-review>
  <bead id="ddx-83440482" iter=1>
    <title>checks: systematic backfill — apply production-reachability across cli/ and resolve all violations</title>
    <description>
Once REACH-PROTO + REACH-GO have landed and ddx self-installs the check, run ddx ac run --check production-reachability against the current cli/ tree and resolve every violation. This is the systematic application phase the user mandated: "go back through the codebase systematically applying it."

ABSORBS
- ddx-09d2990c (WIRE) — the 4 specific unwired functions become a subset of this sweep. Close WIRE as superseded by this bead at file time.
- Any clusters from /tmp/agent-surface-dead-code-audit.md not already covered.
- Codex audit additions: jsonlFallbackForCollection (cli/internal/bead/backend_jsonl.go:90), Store.breakStaleLock (lock.go:70), NewStoreWithBackend (store.go:119), WithSpokeHTTPClient (federation_spoke.go:103), Server.execStore (server.go:2028), GraphQL NewResolver (resolver.go:14).

EXPECTED WORKFLOW
1. Run ddx ac run --check production-reachability against the current worktree. Capture violation list.
2. For each violation, decide: WIRE (add to production code path) or DELETE (function is genuinely obsolete).
3. File one child bead per violation cluster (group by package or by originating story for traceability).
4. Drain the children. Each closes either via wiring PR or deletion PR.
5. Final pass: ddx ac run returns status=pass with no violations and no // wiring:pending annotations remaining (or all annotations cite still-open future beads).

DECISION RULE per violation
- If the symbol's originating bead's AC describes desired runtime behavior → WIRE.
- If the symbol was added speculatively or the design has moved on → DELETE; reopen the originating bead with obsolescence rationale, or close as obsolete.
- If neither is clear within 15 min of investigation → annotate // wiring:pending &lt;follow-up-bead&gt; and file the follow-up.

DEP: REACH-PROTO (ddx-a946c744), REACH-GO (ddx-ea76e1b8)

NOT IN SCOPE
- The protocol or check implementation itself (separate beads).
- Other languages (this project is Go-only at the check layer).
- Auto-run as recurring sweep (could be a future /schedule offer; not built into this bead).
    </description>
    <acceptance>
1. ddx ac run --check production-reachability run on the current cli/ tree; full violation list captured to .ddx/executions/&lt;run-id&gt;/initial-violations.json.
2. One child bead filed per violation cluster (grouped sensibly); each child bead has explicit decision (WIRE or DELETE) with rationale.
3. Each child bead landed via standard execute-bead flow; merge gated by REACH-PROTO check (each landing must pass production-reachability for the symbols it touches).
4. ddx-09d2990c (WIRE) closed as superseded by this bead at filing time; its 4 functions surface as initial violations.
5. Final ddx ac run --check production-reachability returns status=pass with zero violations across cli/.
6. Any remaining // wiring:pending annotations cite open follow-up beads; no orphans.
7. Sweep summary report at .ddx/executions/&lt;final-run-id&gt;/reach-backfill-summary.md: total violations found, wired count, deleted count, follow-ups filed, LOC delta.
8. cd cli &amp;&amp; go test ./... green at every child landing.
    </acceptance>
    <notes>
decomposed into 21 per-package child beads (see .ddx/executions/20260503T124553-282667f7/children.json): ddx-0131ebf0 (cmd), ddx-83b8994f (internal/agent — absorbs the 4 WIRE functions from ddx-09d2990c), ddx-abb40ce5 (agent/escalation), ddx-cd51f35b (agent/testfixtures), ddx-2da07e5c (agent/try), ddx-b42dd3a0 (bead), ddx-91fe7a1a (config), ddx-9df0636c (escalation), ddx-ae4b7393 (evidence), ddx-9f6baafe (exec), ddx-8c273456 (federation), ddx-503c34fa (git), ddx-a7fac0fc (metaprompt), ddx-2850c4dc (metric), ddx-c96fc86c (persona), ddx-d0d8d615 (registry), ddx-90901b22 (server), ddx-4c5beab2 (server/graphql), ddx-895fd8bb (server/perf), ddx-a78f836f (testutils), ddx-7f4cdb7a (update). 283 total dead symbols. Parent stays open until all children land + final ddx ac run reports zero violations.
    </notes>
    <labels>phase:2, area:checks, kind:sweep, prevention</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T124553-282667f7/manifest.json</file>
    <file>.ddx/executions/20260503T124553-282667f7/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="72eaa41c25f6c9c2d9a3b81dcad235f3659bb674">
diff --git a/.ddx/executions/20260503T124553-282667f7/manifest.json b/.ddx/executions/20260503T124553-282667f7/manifest.json
new file mode 100644
index 00000000..ab863ca8
--- /dev/null
+++ b/.ddx/executions/20260503T124553-282667f7/manifest.json
@@ -0,0 +1,39 @@
+{
+  "attempt_id": "20260503T124553-282667f7",
+  "bead_id": "ddx-83440482",
+  "base_rev": "c8f68608e2644d44a1114979e70823fefcee6b53",
+  "created_at": "2026-05-03T12:45:55.51847753Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-83440482",
+    "title": "checks: systematic backfill — apply production-reachability across cli/ and resolve all violations",
+    "description": "Once REACH-PROTO + REACH-GO have landed and ddx self-installs the check, run ddx ac run --check production-reachability against the current cli/ tree and resolve every violation. This is the systematic application phase the user mandated: \"go back through the codebase systematically applying it.\"\n\nABSORBS\n- ddx-09d2990c (WIRE) — the 4 specific unwired functions become a subset of this sweep. Close WIRE as superseded by this bead at file time.\n- Any clusters from /tmp/agent-surface-dead-code-audit.md not already covered.\n- Codex audit additions: jsonlFallbackForCollection (cli/internal/bead/backend_jsonl.go:90), Store.breakStaleLock (lock.go:70), NewStoreWithBackend (store.go:119), WithSpokeHTTPClient (federation_spoke.go:103), Server.execStore (server.go:2028), GraphQL NewResolver (resolver.go:14).\n\nEXPECTED WORKFLOW\n1. Run ddx ac run --check production-reachability against the current worktree. Capture violation list.\n2. For each violation, decide: WIRE (add to production code path) or DELETE (function is genuinely obsolete).\n3. File one child bead per violation cluster (group by package or by originating story for traceability).\n4. Drain the children. Each closes either via wiring PR or deletion PR.\n5. Final pass: ddx ac run returns status=pass with no violations and no // wiring:pending annotations remaining (or all annotations cite still-open future beads).\n\nDECISION RULE per violation\n- If the symbol's originating bead's AC describes desired runtime behavior → WIRE.\n- If the symbol was added speculatively or the design has moved on → DELETE; reopen the originating bead with obsolescence rationale, or close as obsolete.\n- If neither is clear within 15 min of investigation → annotate // wiring:pending \u003cfollow-up-bead\u003e and file the follow-up.\n\nDEP: REACH-PROTO (ddx-a946c744), REACH-GO (ddx-ea76e1b8)\n\nNOT IN SCOPE\n- The protocol or check implementation itself (separate beads).\n- Other languages (this project is Go-only at the check layer).\n- Auto-run as recurring sweep (could be a future /schedule offer; not built into this bead).",
+    "acceptance": "1. ddx ac run --check production-reachability run on the current cli/ tree; full violation list captured to .ddx/executions/\u003crun-id\u003e/initial-violations.json.\n2. One child bead filed per violation cluster (grouped sensibly); each child bead has explicit decision (WIRE or DELETE) with rationale.\n3. Each child bead landed via standard execute-bead flow; merge gated by REACH-PROTO check (each landing must pass production-reachability for the symbols it touches).\n4. ddx-09d2990c (WIRE) closed as superseded by this bead at filing time; its 4 functions surface as initial violations.\n5. Final ddx ac run --check production-reachability returns status=pass with zero violations across cli/.\n6. Any remaining // wiring:pending annotations cite open follow-up beads; no orphans.\n7. Sweep summary report at .ddx/executions/\u003cfinal-run-id\u003e/reach-backfill-summary.md: total violations found, wired count, deleted count, follow-ups filed, LOC delta.\n8. cd cli \u0026\u0026 go test ./... green at every child landing.",
+    "parent": "ddx-e34994e2",
+    "labels": [
+      "phase:2",
+      "area:checks",
+      "kind:sweep",
+      "prevention"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-03T12:45:53Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "463338",
+      "execute-loop-heartbeat-at": "2026-05-03T12:45:53.533663Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260503T124553-282667f7",
+    "prompt": ".ddx/executions/20260503T124553-282667f7/prompt.md",
+    "manifest": ".ddx/executions/20260503T124553-282667f7/manifest.json",
+    "result": ".ddx/executions/20260503T124553-282667f7/result.json",
+    "checks": ".ddx/executions/20260503T124553-282667f7/checks.json",
+    "usage": ".ddx/executions/20260503T124553-282667f7/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-83440482-20260503T124553-282667f7"
+  },
+  "prompt_sha": "4657aa1110c193446937674e425b193a497d1b4789243dcea847ead782699744"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260503T124553-282667f7/result.json b/.ddx/executions/20260503T124553-282667f7/result.json
new file mode 100644
index 00000000..b5348376
--- /dev/null
+++ b/.ddx/executions/20260503T124553-282667f7/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-83440482",
+  "attempt_id": "20260503T124553-282667f7",
+  "base_rev": "c8f68608e2644d44a1114979e70823fefcee6b53",
+  "result_rev": "c11b4a3e1c3987caee123d5517a266a833e46026",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-de0f4d1e",
+  "duration_ms": 343375,
+  "tokens": 15515,
+  "cost_usd": 1.6950332500000003,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T124553-282667f7",
+  "prompt_file": ".ddx/executions/20260503T124553-282667f7/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T124553-282667f7/manifest.json",
+  "result_file": ".ddx/executions/20260503T124553-282667f7/result.json",
+  "usage_file": ".ddx/executions/20260503T124553-282667f7/usage.json",
+  "started_at": "2026-05-03T12:45:55.518912446Z",
+  "finished_at": "2026-05-03T12:51:38.894892393Z"
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
