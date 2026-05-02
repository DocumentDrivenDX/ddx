<bead-review>
  <bead id="ddx-673833f4" iter=1>
    <title>TD: bead state machine — design + naming-role decisions (foundational doc)</title>
    <description>
Authoring-only sub-task of the bead state-machine work. Children (filed separately) handle the mechanical reconciliation, CI guards, and housekeeping that follow this design.

This bead OUTPUTS the canonical TD doc that the 5 hygiene beads + the 3 sibling reconciliation beads reference. NO code or schema changes in this bead — those are separate sub-beads.

## Critical constraint (per ADR-004)

The bead-record schema MUST stay compatible with the bd/br interchange contract. Status enum is FIXED at the bd/br canonical 6 (open, in_progress, closed, blocked, proposed, cancelled). DDx-specific execution semantics live in labels, events, or Extra map — NOT in new statuses.

## Today's actual state (verified)

- bead-record.schema.json enum: 6 values (the bd/br canonical set)
- FEAT-004 line 65 docs: 3 values (lags schema)
- Persisted reality: 'ddx bead list --json | jq -r .[].status | sort -u' shows only open/in_progress/closed in active rows today
- Code uses additional names (done, needs_human, pending, ready, review, needs_investigation) but these are NOT persisted statuses — event kinds, terminal phases, derived queue categories, or labels

## Five in-flight hygiene beads each touch state-machine vocabulary (must align with this TD)

- ddx-b24e9630 (no_changes verification): NoChangesContract subsection
- ddx-3c154349 (auto-triage): TriageContract subsection
- ddx-aede917d (drain pause on quota): QuotaPauseContract subsection
- ddx-c6e3db02 (rate-limit retry): RateLimitRetryContract subsection
- ddx-da11a34a (lock contention): LockContentionContract subsection

## Scope (this bead authors the doc only)

Author docs/helix/02-design/technical-designs/TD-NNN-bead-state-machine.md with:

1. Distinguish category of every observed name as one of: persisted bead status (locked to bd/br set) / derived queue category / event kind / terminal phase / claim metadata / label / Extra metadata field / worker state.
2. Persisted status enumeration — confirm the bd/br-compatible 6-value set. Status enum is NOT modified.
3. Transition matrix — table or diagram: which status → which is allowed; who can drive each (operator-via-CLI, drain loop, agent, reviewer); what events fire on each transition.
4. Event vocabulary — unified across hygiene beads (no_changes_*, drain-paused-quota, etc.).
5. Outcome → label/event/Extra mapping — when execute-bead returns merged/already_satisfied/review_block/execution_failed/no_changes_*, the resulting bead state changes (status transition within the 6, label add/remove, event append, Extra field update).
6. Claim semantics — when claimed, when released, stale claim handling.
7. Naming-role decision matrix — every observed name labeled with role + rationale: blocked, in_progress, open, closed, cancelled, proposed (current schema 6), done, pending, ready, review (in code as non-status), needs_human (label today; stays label), needs_investigation (LABEL per bd/br compatibility, NOT status), blocked-on-upstream:&lt;id&gt; (label).
8. Per-hygiene-bead contract subsection — one per bead (NoChangesContract, TriageContract, QuotaPauseContract, RateLimitRetryContract, LockContentionContract): status transitions used (within canonical 6), labels added/removed, events fired, claim behavior, loop interactions.
9. Future-change process — explicit rule: any bead introducing or changing labels/events/outcome semantics/claim handling MUST cite the TD section authorizing it. Adding a new persisted status requires upstream bd/br coordination + ADR-004 amendment.
10. Worker-state enumeration — parallel section: idle / draining / paused-quota / paused-rate-limit / exiting. Worker-state is distinct from bead-state, owned by worker process not bead store.
11. Migration plan placeholder — say 'sibling bead handles survey + execution' (the actual survey runs in the sibling bead).

## What does NOT land here

- Schema/code/docs reconciliation (sibling bead handles)
- CI guard tests (sibling bead handles)
- Hygiene-bead AC TD-NNN substitution + migration survey (sibling bead handles)
    </description>
    <acceptance>
1. docs/helix/02-design/technical-designs/TD-&lt;NNN&gt;-bead-state-machine.md exists with all 11 sections plus 5 per-hygiene-bead contract subsections. 2. depends_on: [FEAT-004, SD-004, ADR-004]. 3. Persisted status list is the bd/br-canonical 6 (no additions). 4. Naming-role decision matrix covers ALL observed names with rationale. 5. Future-change process rule explicit. 6. Worker-state enumeration parallel section present. 7. ddx doc audit clean for the new TD artifact.
    </acceptance>
    <labels>phase:2, story:10, area:specs, kind:design, foundation</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T190445-bbf9f442/manifest.json</file>
    <file>.ddx/executions/20260502T190445-bbf9f442/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="b192a223325bb84409feb85b0995eefd6938f351">
diff --git a/.ddx/executions/20260502T190445-bbf9f442/manifest.json b/.ddx/executions/20260502T190445-bbf9f442/manifest.json
new file mode 100644
index 00000000..0009140b
--- /dev/null
+++ b/.ddx/executions/20260502T190445-bbf9f442/manifest.json
@@ -0,0 +1,40 @@
+{
+  "attempt_id": "20260502T190445-bbf9f442",
+  "bead_id": "ddx-673833f4",
+  "base_rev": "01907ee8fd806764789935c605f1f05de673d592",
+  "created_at": "2026-05-02T19:04:46.399297024Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-673833f4",
+    "title": "TD: bead state machine — design + naming-role decisions (foundational doc)",
+    "description": "Authoring-only sub-task of the bead state-machine work. Children (filed separately) handle the mechanical reconciliation, CI guards, and housekeeping that follow this design.\n\nThis bead OUTPUTS the canonical TD doc that the 5 hygiene beads + the 3 sibling reconciliation beads reference. NO code or schema changes in this bead — those are separate sub-beads.\n\n## Critical constraint (per ADR-004)\n\nThe bead-record schema MUST stay compatible with the bd/br interchange contract. Status enum is FIXED at the bd/br canonical 6 (open, in_progress, closed, blocked, proposed, cancelled). DDx-specific execution semantics live in labels, events, or Extra map — NOT in new statuses.\n\n## Today's actual state (verified)\n\n- bead-record.schema.json enum: 6 values (the bd/br canonical set)\n- FEAT-004 line 65 docs: 3 values (lags schema)\n- Persisted reality: 'ddx bead list --json | jq -r .[].status | sort -u' shows only open/in_progress/closed in active rows today\n- Code uses additional names (done, needs_human, pending, ready, review, needs_investigation) but these are NOT persisted statuses — event kinds, terminal phases, derived queue categories, or labels\n\n## Five in-flight hygiene beads each touch state-machine vocabulary (must align with this TD)\n\n- ddx-b24e9630 (no_changes verification): NoChangesContract subsection\n- ddx-3c154349 (auto-triage): TriageContract subsection\n- ddx-aede917d (drain pause on quota): QuotaPauseContract subsection\n- ddx-c6e3db02 (rate-limit retry): RateLimitRetryContract subsection\n- ddx-da11a34a (lock contention): LockContentionContract subsection\n\n## Scope (this bead authors the doc only)\n\nAuthor docs/helix/02-design/technical-designs/TD-NNN-bead-state-machine.md with:\n\n1. Distinguish category of every observed name as one of: persisted bead status (locked to bd/br set) / derived queue category / event kind / terminal phase / claim metadata / label / Extra metadata field / worker state.\n2. Persisted status enumeration — confirm the bd/br-compatible 6-value set. Status enum is NOT modified.\n3. Transition matrix — table or diagram: which status → which is allowed; who can drive each (operator-via-CLI, drain loop, agent, reviewer); what events fire on each transition.\n4. Event vocabulary — unified across hygiene beads (no_changes_*, drain-paused-quota, etc.).\n5. Outcome → label/event/Extra mapping — when execute-bead returns merged/already_satisfied/review_block/execution_failed/no_changes_*, the resulting bead state changes (status transition within the 6, label add/remove, event append, Extra field update).\n6. Claim semantics — when claimed, when released, stale claim handling.\n7. Naming-role decision matrix — every observed name labeled with role + rationale: blocked, in_progress, open, closed, cancelled, proposed (current schema 6), done, pending, ready, review (in code as non-status), needs_human (label today; stays label), needs_investigation (LABEL per bd/br compatibility, NOT status), blocked-on-upstream:\u003cid\u003e (label).\n8. Per-hygiene-bead contract subsection — one per bead (NoChangesContract, TriageContract, QuotaPauseContract, RateLimitRetryContract, LockContentionContract): status transitions used (within canonical 6), labels added/removed, events fired, claim behavior, loop interactions.\n9. Future-change process — explicit rule: any bead introducing or changing labels/events/outcome semantics/claim handling MUST cite the TD section authorizing it. Adding a new persisted status requires upstream bd/br coordination + ADR-004 amendment.\n10. Worker-state enumeration — parallel section: idle / draining / paused-quota / paused-rate-limit / exiting. Worker-state is distinct from bead-state, owned by worker process not bead store.\n11. Migration plan placeholder — say 'sibling bead handles survey + execution' (the actual survey runs in the sibling bead).\n\n## What does NOT land here\n\n- Schema/code/docs reconciliation (sibling bead handles)\n- CI guard tests (sibling bead handles)\n- Hygiene-bead AC TD-NNN substitution + migration survey (sibling bead handles)",
+    "acceptance": "1. docs/helix/02-design/technical-designs/TD-\u003cNNN\u003e-bead-state-machine.md exists with all 11 sections plus 5 per-hygiene-bead contract subsections. 2. depends_on: [FEAT-004, SD-004, ADR-004]. 3. Persisted status list is the bd/br-canonical 6 (no additions). 4. Naming-role decision matrix covers ALL observed names with rationale. 5. Future-change process rule explicit. 6. Worker-state enumeration parallel section present. 7. ddx doc audit clean for the new TD artifact.",
+    "parent": "ddx-e34994e2",
+    "labels": [
+      "phase:2",
+      "story:10",
+      "area:specs",
+      "kind:design",
+      "foundation"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T19:04:45Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3446284",
+      "execute-loop-heartbeat-at": "2026-05-02T19:04:45.108443529Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T190445-bbf9f442",
+    "prompt": ".ddx/executions/20260502T190445-bbf9f442/prompt.md",
+    "manifest": ".ddx/executions/20260502T190445-bbf9f442/manifest.json",
+    "result": ".ddx/executions/20260502T190445-bbf9f442/result.json",
+    "checks": ".ddx/executions/20260502T190445-bbf9f442/checks.json",
+    "usage": ".ddx/executions/20260502T190445-bbf9f442/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-673833f4-20260502T190445-bbf9f442"
+  },
+  "prompt_sha": "e3f3d2fed6efc7d129dd25be04e6d60849a1848e6867d3e3dce39aeaf2152cd5"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T190445-bbf9f442/result.json b/.ddx/executions/20260502T190445-bbf9f442/result.json
new file mode 100644
index 00000000..f4e740c1
--- /dev/null
+++ b/.ddx/executions/20260502T190445-bbf9f442/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-673833f4",
+  "attempt_id": "20260502T190445-bbf9f442",
+  "base_rev": "01907ee8fd806764789935c605f1f05de673d592",
+  "result_rev": "f424244a62baba32b6eeaab9c32a7217916ef5e3",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-961d10be",
+  "duration_ms": 202373,
+  "tokens": 11897,
+  "cost_usd": 1.0328602500000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T190445-bbf9f442",
+  "prompt_file": ".ddx/executions/20260502T190445-bbf9f442/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T190445-bbf9f442/manifest.json",
+  "result_file": ".ddx/executions/20260502T190445-bbf9f442/result.json",
+  "usage_file": ".ddx/executions/20260502T190445-bbf9f442/usage.json",
+  "started_at": "2026-05-02T19:04:46.399742272Z",
+  "finished_at": "2026-05-02T19:08:08.773727242Z"
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
