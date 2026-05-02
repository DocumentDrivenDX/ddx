<bead-review>
  <bead id="ddx-eb7d5981" iter=1>
    <title>TD: axon-backend for bead tracker — schema mapping, claim/lock semantics, archive policy</title>
    <description>
Author docs/helix/02-design/technical-designs/TD-&lt;NNN&gt;-axon-bead-backend.md.

Source material (consume this first): /tmp/ddx-axon-backend-plan.md — full integration plan with Axon's external interface mapped, schema/operation/concurrency mapping spelled out field-by-field, migration plan, and locked operator decisions.

LOCKED DECISIONS (do not relitigate; bake into TD):
1. Deployment: separate axon-server, operator-managed. DDx does NOT spawn or supervise Axon. Document the .ddx/config.yaml shape + ddx doctor diagnostic.
2. Auth: localhost-only by default; ts-net for remote. Reuse ADR-006 tsnet integration.
3. Events storage: separate ddx_bead_events collection with event_of links (Option B). Two collections in Axon.
4. Offline posture: refuse all bead ops when Axon unreachable. No local cache, no WAL.

Defaults the TD can adopt without rechecking with operator (revisit only if implementation surfaces a problem):
- Schema versioning: lazy migrate on read
- ddx bead ready query: two-phase (status filter + per-candidate traverse) with benchmark-then-optimize escalation path
- Backup/DR: operator-managed via Axon's own backing-store DR; ddx bead export is the logical backup format

Open questions for the Axon team (file as cross-repo coordination if needed):
- Is there a server-side aggregation/computed-view for the ready-queue traversal pattern?
- Does Axon's ts-net story align with ADR-006 (Tailscale tsnet listener) or require something different?

Output: TD-&lt;NNN&gt;-axon-bead-backend.md with sections for: schema mapping (DDx bead → Axon entity); operation mapping (DDx CLI → Axon RPC); deployment + config; concurrency model; migration plan; test plan. depends_on: ADR-004, SD-004, FEAT-004.
    </description>
    <acceptance>
1. TD-&lt;NNN&gt;-axon-bead-backend.md exists with all sections from the source plan. 2. depends_on: [ADR-004, SD-004, FEAT-004]. 3. Locked decisions reflected exactly (no relitigation). 4. Cross-references the plan at /tmp/ddx-axon-backend-plan.md as 'background'. 5. ddx doc audit clean. 6. TD picks a concrete TD-NNN ID (next available).
    </acceptance>
    <labels>phase:2, area:beads, area:specs, kind:design</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T145133-e2e48633/manifest.json</file>
    <file>.ddx/executions/20260502T145133-e2e48633/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="f169974237a4ec599be97250894d81d2e83bbc96">
diff --git a/.ddx/executions/20260502T145133-e2e48633/manifest.json b/.ddx/executions/20260502T145133-e2e48633/manifest.json
new file mode 100644
index 00000000..e1a26deb
--- /dev/null
+++ b/.ddx/executions/20260502T145133-e2e48633/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260502T145133-e2e48633",
+  "bead_id": "ddx-eb7d5981",
+  "base_rev": "c8ea93aeed949f480b4879ac5bb62f836dc00fc5",
+  "created_at": "2026-05-02T14:51:34.348383239Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-eb7d5981",
+    "title": "TD: axon-backend for bead tracker — schema mapping, claim/lock semantics, archive policy",
+    "description": "Author docs/helix/02-design/technical-designs/TD-\u003cNNN\u003e-axon-bead-backend.md.\n\nSource material (consume this first): /tmp/ddx-axon-backend-plan.md — full integration plan with Axon's external interface mapped, schema/operation/concurrency mapping spelled out field-by-field, migration plan, and locked operator decisions.\n\nLOCKED DECISIONS (do not relitigate; bake into TD):\n1. Deployment: separate axon-server, operator-managed. DDx does NOT spawn or supervise Axon. Document the .ddx/config.yaml shape + ddx doctor diagnostic.\n2. Auth: localhost-only by default; ts-net for remote. Reuse ADR-006 tsnet integration.\n3. Events storage: separate ddx_bead_events collection with event_of links (Option B). Two collections in Axon.\n4. Offline posture: refuse all bead ops when Axon unreachable. No local cache, no WAL.\n\nDefaults the TD can adopt without rechecking with operator (revisit only if implementation surfaces a problem):\n- Schema versioning: lazy migrate on read\n- ddx bead ready query: two-phase (status filter + per-candidate traverse) with benchmark-then-optimize escalation path\n- Backup/DR: operator-managed via Axon's own backing-store DR; ddx bead export is the logical backup format\n\nOpen questions for the Axon team (file as cross-repo coordination if needed):\n- Is there a server-side aggregation/computed-view for the ready-queue traversal pattern?\n- Does Axon's ts-net story align with ADR-006 (Tailscale tsnet listener) or require something different?\n\nOutput: TD-\u003cNNN\u003e-axon-bead-backend.md with sections for: schema mapping (DDx bead → Axon entity); operation mapping (DDx CLI → Axon RPC); deployment + config; concurrency model; migration plan; test plan. depends_on: ADR-004, SD-004, FEAT-004.",
+    "acceptance": "1. TD-\u003cNNN\u003e-axon-bead-backend.md exists with all sections from the source plan. 2. depends_on: [ADR-004, SD-004, FEAT-004]. 3. Locked decisions reflected exactly (no relitigation). 4. Cross-references the plan at /tmp/ddx-axon-backend-plan.md as 'background'. 5. ddx doc audit clean. 6. TD picks a concrete TD-NNN ID (next available).",
+    "parent": "ddx-5d49b14e",
+    "labels": [
+      "phase:2",
+      "area:beads",
+      "area:specs",
+      "kind:design"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T14:51:33Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1724970",
+      "execute-loop-heartbeat-at": "2026-05-02T14:51:33.060714445Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T145133-e2e48633",
+    "prompt": ".ddx/executions/20260502T145133-e2e48633/prompt.md",
+    "manifest": ".ddx/executions/20260502T145133-e2e48633/manifest.json",
+    "result": ".ddx/executions/20260502T145133-e2e48633/result.json",
+    "checks": ".ddx/executions/20260502T145133-e2e48633/checks.json",
+    "usage": ".ddx/executions/20260502T145133-e2e48633/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-eb7d5981-20260502T145133-e2e48633"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T145133-e2e48633/result.json b/.ddx/executions/20260502T145133-e2e48633/result.json
new file mode 100644
index 00000000..377cdcf3
--- /dev/null
+++ b/.ddx/executions/20260502T145133-e2e48633/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-eb7d5981",
+  "attempt_id": "20260502T145133-e2e48633",
+  "base_rev": "c8ea93aeed949f480b4879ac5bb62f836dc00fc5",
+  "result_rev": "5740627fcdd9531ac640d70e0df862925fcc505d",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-5a648b9d",
+  "duration_ms": 177131,
+  "tokens": 9502,
+  "cost_usd": 0.8009425,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T145133-e2e48633",
+  "prompt_file": ".ddx/executions/20260502T145133-e2e48633/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T145133-e2e48633/manifest.json",
+  "result_file": ".ddx/executions/20260502T145133-e2e48633/result.json",
+  "usage_file": ".ddx/executions/20260502T145133-e2e48633/usage.json",
+  "started_at": "2026-05-02T14:51:34.348654239Z",
+  "finished_at": "2026-05-02T14:54:31.479950398Z"
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
