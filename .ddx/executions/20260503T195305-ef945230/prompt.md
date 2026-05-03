<bead-review>
  <bead id="ddx-eb7d5981" iter=1>
    <title>TD: axon-backend for bead tracker — schema mapping, claim/lock semantics, archive policy</title>
    <description>
Author docs/helix/02-design/technical-designs/TD-&lt;NNN&gt;-axon-bead-backend.md.

Source material (consume in order):
1. /tmp/ddx-axon-backend-plan.md — full integration plan with Axon's external interface mapped + GraphQL pivot + locked decisions
2. Cross-repo tracking bead axon-82b6f7b2 — full DDx-on-axon adoption checklist (links to all required axon-side capabilities)

LOCKED DECISIONS (do not relitigate; bake into TD):
1. Deployment: separate axon-server, operator-managed. DDx does NOT spawn or supervise Axon.
2. Auth: localhost-only by default; ts-net for remote. Axon already supports ts-net per its serve.rs (Tailscale mode + LocalAPI whois + role tags). Reuses ADR-006.
3. Events storage: separate ddx_bead_events collection with event_of links (Option B). Two collections in Axon.
4. Offline posture: refuse all bead ops when Axon unreachable. No local cache, no WAL.
5. Wire transport: GraphQL only via genqlient-generated Go client. NOT gRPC. Subscriptions over WebSocket for live updates.

Cross-repo dependencies (tracked in axon-82b6f7b2):
- axon-05c1019d (P2): GraphQL pattern-query support — needed for single-query ddx bead ready/blocked. Day 1 ships with two-phase fallback.
- FEAT-013 secondary indexes (P1 Draft in axon): needed for status-field query performance at &gt;1000 beads.
- FEAT-017 schema evolution (P1 Draft in axon): needed for clean DDx schema upgrades over time. Day 1 ships with DDx-side lazy migration on read.
- ts-net auth: ALREADY SHIPPED in axon. No coordination needed.
- GraphQL CRUD + subscriptions: ALREADY SHIPPED in axon.
- Multi-namespace/database: ALREADY SHIPPED.

Adoption gates (per axon-82b6f7b2):
- Alpha: requires axon-05c1019d + everything already-shipped. Acceptable to ship with table-scan perf.
- Beta: requires FEAT-013 (indexes) for &gt;1000-bead performance.
- GA: requires FEAT-013 + FEAT-017 + axon-05c1019d all landed.

Defaults the TD can adopt without rechecking with operator (revisit only if implementation surfaces a problem):
- Schema versioning: DDx-side lazy migrate on read until FEAT-017 lands; switch to axon-native then.
- ddx bead ready Day 1: two-phase GraphQL query.
- Backup/DR: operator-managed via Axon's own backing-store DR.

Output: TD-&lt;NNN&gt;-axon-bead-backend.md with sections for: GraphQL schema mapping (DDx bead → Axon entity); operation mapping (DDx CLI → GraphQL queries/mutations/subscriptions); deployment + config; concurrency model (OCC via expectedVersion); migration plan; test plan; cross-repo coordination notes (links to axon-82b6f7b2 + FEAT-013/017).
    </description>
    <acceptance>
1. TD-&lt;NNN&gt;-axon-bead-backend.md exists with all sections. 2. depends_on: [ADR-004, SD-004, FEAT-004]. 3. All five locked decisions reflected exactly. 4. Wire transport explicitly GraphQL-only with rationale. 5. Cross-references the plan at /tmp/ddx-axon-backend-plan.md as 'background' AND axon-82b6f7b2 as the cross-repo coordination tracker. 6. Adoption gates (alpha/beta/GA) documented with which axon-side beads gate each. 7. ddx doc audit clean. 8. TD picks a concrete TD-NNN ID.
    </acceptance>
    <labels>phase:2, area:beads, area:specs, kind:design</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T194722-416d6e3a/manifest.json</file>
    <file>.ddx/executions/20260503T194722-416d6e3a/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="29aa43bd47c01231a106d4472a38c9aec1fadac2">
diff --git a/.ddx/executions/20260503T194722-416d6e3a/manifest.json b/.ddx/executions/20260503T194722-416d6e3a/manifest.json
new file mode 100644
index 00000000..21920c07
--- /dev/null
+++ b/.ddx/executions/20260503T194722-416d6e3a/manifest.json
@@ -0,0 +1,55 @@
+{
+  "attempt_id": "20260503T194722-416d6e3a",
+  "bead_id": "ddx-eb7d5981",
+  "base_rev": "c94e337b0afb4249c7fdce13709de35cbc08c31a",
+  "created_at": "2026-05-03T19:47:23.727011373Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-eb7d5981",
+    "title": "TD: axon-backend for bead tracker — schema mapping, claim/lock semantics, archive policy",
+    "description": "Author docs/helix/02-design/technical-designs/TD-\u003cNNN\u003e-axon-bead-backend.md.\n\nSource material (consume in order):\n1. /tmp/ddx-axon-backend-plan.md — full integration plan with Axon's external interface mapped + GraphQL pivot + locked decisions\n2. Cross-repo tracking bead axon-82b6f7b2 — full DDx-on-axon adoption checklist (links to all required axon-side capabilities)\n\nLOCKED DECISIONS (do not relitigate; bake into TD):\n1. Deployment: separate axon-server, operator-managed. DDx does NOT spawn or supervise Axon.\n2. Auth: localhost-only by default; ts-net for remote. Axon already supports ts-net per its serve.rs (Tailscale mode + LocalAPI whois + role tags). Reuses ADR-006.\n3. Events storage: separate ddx_bead_events collection with event_of links (Option B). Two collections in Axon.\n4. Offline posture: refuse all bead ops when Axon unreachable. No local cache, no WAL.\n5. Wire transport: GraphQL only via genqlient-generated Go client. NOT gRPC. Subscriptions over WebSocket for live updates.\n\nCross-repo dependencies (tracked in axon-82b6f7b2):\n- axon-05c1019d (P2): GraphQL pattern-query support — needed for single-query ddx bead ready/blocked. Day 1 ships with two-phase fallback.\n- FEAT-013 secondary indexes (P1 Draft in axon): needed for status-field query performance at \u003e1000 beads.\n- FEAT-017 schema evolution (P1 Draft in axon): needed for clean DDx schema upgrades over time. Day 1 ships with DDx-side lazy migration on read.\n- ts-net auth: ALREADY SHIPPED in axon. No coordination needed.\n- GraphQL CRUD + subscriptions: ALREADY SHIPPED in axon.\n- Multi-namespace/database: ALREADY SHIPPED.\n\nAdoption gates (per axon-82b6f7b2):\n- Alpha: requires axon-05c1019d + everything already-shipped. Acceptable to ship with table-scan perf.\n- Beta: requires FEAT-013 (indexes) for \u003e1000-bead performance.\n- GA: requires FEAT-013 + FEAT-017 + axon-05c1019d all landed.\n\nDefaults the TD can adopt without rechecking with operator (revisit only if implementation surfaces a problem):\n- Schema versioning: DDx-side lazy migrate on read until FEAT-017 lands; switch to axon-native then.\n- ddx bead ready Day 1: two-phase GraphQL query.\n- Backup/DR: operator-managed via Axon's own backing-store DR.\n\nOutput: TD-\u003cNNN\u003e-axon-bead-backend.md with sections for: GraphQL schema mapping (DDx bead → Axon entity); operation mapping (DDx CLI → GraphQL queries/mutations/subscriptions); deployment + config; concurrency model (OCC via expectedVersion); migration plan; test plan; cross-repo coordination notes (links to axon-82b6f7b2 + FEAT-013/017).",
+    "acceptance": "1. TD-\u003cNNN\u003e-axon-bead-backend.md exists with all sections. 2. depends_on: [ADR-004, SD-004, FEAT-004]. 3. All five locked decisions reflected exactly. 4. Wire transport explicitly GraphQL-only with rationale. 5. Cross-references the plan at /tmp/ddx-axon-backend-plan.md as 'background' AND axon-82b6f7b2 as the cross-repo coordination tracker. 6. Adoption gates (alpha/beta/GA) documented with which axon-side beads gate each. 7. ddx doc audit clean. 8. TD picks a concrete TD-NNN ID.",
+    "parent": "ddx-5d49b14e",
+    "labels": [
+      "phase:2",
+      "area:beads",
+      "area:specs",
+      "kind:design"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-03T19:47:22Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "463338",
+      "closing_commit_sha": "f169974237a4ec599be97250894d81d2e83bbc96",
+      "events": [
+        {
+          "actor": "",
+          "body": "",
+          "created_at": "2026-05-02T18:09:03.561395069Z",
+          "kind": "reopen",
+          "source": "",
+          "summary": "Auto-closed by drain with only execution-evidence files (no implementation). Reopening per the no_changes pattern that auto-triage (ddx-3c154349 itself) should fix. Real work pending."
+        }
+      ],
+      "events_attachment": "ddx-eb7d5981/events.jsonl",
+      "execute-loop-heartbeat-at": "2026-05-03T19:47:22.205850015Z",
+      "execute-loop-last-detail": "exit status 1",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-05-03T09:12:59Z",
+      "session_id": "eb-5a648b9d"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260503T194722-416d6e3a",
+    "prompt": ".ddx/executions/20260503T194722-416d6e3a/prompt.md",
+    "manifest": ".ddx/executions/20260503T194722-416d6e3a/manifest.json",
+    "result": ".ddx/executions/20260503T194722-416d6e3a/result.json",
+    "checks": ".ddx/executions/20260503T194722-416d6e3a/checks.json",
+    "usage": ".ddx/executions/20260503T194722-416d6e3a/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-eb7d5981-20260503T194722-416d6e3a"
+  },
+  "prompt_sha": "2a6fbb6bb638e131b11ca51d86ae29259396fa33ffc86a51bc7e69b9b62c4723"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260503T194722-416d6e3a/result.json b/.ddx/executions/20260503T194722-416d6e3a/result.json
new file mode 100644
index 00000000..338e26d3
--- /dev/null
+++ b/.ddx/executions/20260503T194722-416d6e3a/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-eb7d5981",
+  "attempt_id": "20260503T194722-416d6e3a",
+  "base_rev": "c94e337b0afb4249c7fdce13709de35cbc08c31a",
+  "result_rev": "731c2431fc20fff0face56c1250a3dac1b172253",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-9c3b9c09",
+  "duration_ms": 335914,
+  "tokens": 16209,
+  "cost_usd": 0.9988627500000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T194722-416d6e3a",
+  "prompt_file": ".ddx/executions/20260503T194722-416d6e3a/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T194722-416d6e3a/manifest.json",
+  "result_file": ".ddx/executions/20260503T194722-416d6e3a/result.json",
+  "usage_file": ".ddx/executions/20260503T194722-416d6e3a/usage.json",
+  "started_at": "2026-05-03T19:47:23.727325665Z",
+  "finished_at": "2026-05-03T19:52:59.641700584Z"
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
