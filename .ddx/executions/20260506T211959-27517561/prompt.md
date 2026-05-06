<bead-review>
  <bead id="ddx-312adc66" iter=1>
    <title>try: define and enforce bead-level execution hints</title>
    <description>
PROBLEM
DDx has a partial tier-label mechanism, but durable bead-level execution hints are underspecified. Agents can currently copy route-shaped settings into bead metadata without a clear lint/audit contract, and smart-tier usage is not tied to a required justification.

ROOT CAUSE
docs/helix/02-design/technical-designs/TD-037-bead-execution-hints.md:23-39 documents that `ddx try` infers tier from labels today, but arbitrary custom fields are preserved without a routing contract. docs/helix/02-design/technical-designs/TD-037-bead-execution-hints.md:62-92 requires smart justification, and lines 112-161 require durable route-pin rejection plus execution-routing-intent evidence.

SMART JUSTIFICATION: This bead defines and implements the durable execution-hint contract that later prose beads rely on. Weak implementation would let agents persist concrete model or harness pins into queue metadata, directly violating ADR-024.

PROPOSED FIX
- Add typed parsing for `tier:cheap`, `tier:standard`, and `tier:smart` labels.
- Extract and validate a `SMART JUSTIFICATION:` section for smart-tier beads.
- Add bead lifecycle lint findings for missing smart justification and durable route-pin fields/labels.
- Record `execution-routing-intent` evidence with source, requested tier/power, smart justification, actual route facts, and degradation notes.
- Extend agent metrics projection enough to audit smart-hint usage and rejected durable route pins.

NON-SCOPE
- Do not add durable bead-level harness/provider/model/model-ref pins.
- Do not change Fizeau routing internals.
- Do not implement a materialized metrics rollup store.

DEPS
No deps. This must land before the Vale/prose child beads rely on tier hints.
    </description>
    <acceptance>
1. `TestExecutionHintParse_ValidTierLabels` covers `tier:cheap`, `tier:standard`, and `tier:smart` parsing.
2. `TestExecutionHintParse_SmartRequiresJustification` covers missing and present `SMART JUSTIFICATION:` text.
3. `TestExecutionHintLint_RejectsDurableRoutePins` covers forbidden fields/labels such as `execution-model` and `harness:claude`.
4. `TestTryRecordsExecutionRoutingIntent` proves `ddx try` evidence records source `bead_hint`, requested tier, smart justification, and actual route facts when available.
5. `TestAgentMetricsIncludesRoutingIntent` proves the normalized attempt projection can count smart-hinted attempts and rejected route pins.
6. Existing heuristic inference tests still pass when no tier hint exists.
7. cd cli &amp;&amp; go test ./internal/escalation/... ./internal/bead/... ./internal/agent/... ./cmd/... ./internal/agentmetrics/... green.
8. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:execution, area:beads, area:metrics, kind:design, kind:implementation, spec:TD-037, adr:024, tier:smart</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T210004-db8d7e99/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T210004-db8d7e99/manifest.json</file>
    <file>.ddx/executions/20260506T210004-db8d7e99/result.json</file>
  </changed-files>

  <governing>
    <ref id="TD-037" path="docs/helix/02-design/technical-designs/TD-037-bead-execution-hints.md" title="Technical Design: Bead-Level Execution Hints">
      <content>
<untrusted-data>
---
ddx:
  id: TD-037
  depends_on:
    - FEAT-006
    - FEAT-010
    - FEAT-014
    - ADR-024
  status: draft
---
# Technical Design: Bead-Level Execution Hints

## Purpose

DDx needs a durable way to say that a bead needs a stronger agent than the
default queue policy would infer. The mechanism must be portable across
machines and providers, auditable after the fact, and hard to cargo-cult into
concrete harness or model pins.

This design defines bead-level execution hints as abstract power intent. It
does not let beads choose providers, harnesses, or concrete models.

## Current State

`ddx try` already has a partial mechanism:

- Beads preserve unknown custom fields in `Extra`, but `ddx try` does not read
  arbitrary routing custom fields.
- `ddx try` accepts CLI routing flags such as `--profile`, `--min-power`,
  `--max-power`, `--harness`, `--provider`, `--model`, and `--model-ref`.
- When no routing flags are supplied and no project routing config exists,
  `ddx try` calls `escalation.InferTier(bead)`.
- `InferTier` treats labels `tier:smart`, `tier:standard`, and `tier:cheap` as
  explicit tier overrides before falling back to priority, kind, and scope
  heuristics.

That gives DDx a usable short-term path, but it is underspecified. It does not
define when `tier:smart` is justified, how the choice is audited, or how to
reject durable model/harness cargo culting.

## Design

### Durable Hint Surface

DDx recognizes exactly one durable bead-level hint surface in v1:

| Surface | Values | Meaning |
|---|---|---|
| `tier:cheap` label | cheap | Mechanical work where low-cost models should be enough. |
| `tier:standard` label | standard | Ordinary implementation or review work. |
| `tier:smart` label | smart | High-judgment, broad, ambiguous, or architecture-sensitive work. |

The label is a request for abstract execution power. It is not a model name.
`tier:smart` maps to the current smart execution profile only at the DDx
request-construction boundary; Fizeau still owns the concrete route.

DDx should not add bead-level `harness`, `provider`, `model`, or `model-ref`
fields. Those remain operator-supplied CLI passthrough constraints for one
attempt, or project/Fizeau configuration when a workspace intentionally pins
routing policy.

### Smart-Tier Justification

`tier:smart` requires a justification in the bead description. The justification
must name the reason the bead is likely capability-sensitive, using one or more
of these categories:

- cross-cutting behavior or multiple subsystems;
- architecture, API, or data-model decision;
- ambiguous or competing requirements that need reconciliation;
- high-risk review, security, data-loss, or migration concern;
- evaluation of non-deterministic output quality;
- decomposition or planning work where weak reasoning creates downstream waste.

Valid example:

```text
SMART JUSTIFICATION: This bead decides the durable execution-hint contract and
its precedence against ADR-024 routing policy. A weak implementation could
persist concrete model pins into queue metadata.
```

Invalid example:

```text
SMART JUSTIFICATION: Claude worked well last time.
```

The first enforcement step should live in the bead lifecycle lint hook so
authors and agents get feedback before dispatch. A later hardening pass may add
the same validation to `ddx bead create` and `ddx bead update`, but lint is the
minimum gate because all automated execution passes through it.

### Precedence

Execution intent resolves in this order:

1. Explicit CLI flags for the current invocation.
2. Project/Fizeau routing configuration.
3. Durable bead hint label.
4. DDx heuristic inference from priority, kind, issue type, and scope.
5. DDx default.

CLI flags win because they are operator actions for one attempt and are visible
in the command/evidence. Durable bead hints win over heuristics because they are
part of the reviewed work item.

If a project has explicit routing configuration, `ddx try` must not silently
override it with a bead label. The label should still be recorded as requested
intent so operators can audit divergence between requested and actual routing.

### Rejected Durable Route Pins

Bead metadata must not persist concrete route choices. The lint hook should
reject or block execution when a bead includes durable fields or labels that
look like route pins, including:

- `harness`, `agent-harness`, `execution-harness`, `try-harness`;
- `provider`, `agent-provider`, `execution-provider`, `try-provider`;
- `model`, `agent-model`, `execution-model`, `try-model`;
- `model-ref`, `agent-model-ref`, `execution-model-ref`, `try-model-ref`;
- labels such as `harness:claude`, `provider:openai`, or `model:gpt-...`.

The diagnostic should say to use a one-off CLI flag for a reproduction or
explicit operator constraint:

```bash
ddx try <id> --harness claude
```

This keeps durable queue metadata portable and prevents agents from copying
route choices that happened to work once.

### Audit Evidence

Every `ddx try` / `ddx work` attempt should record routing-intent evidence
before execution starts. The evidence should be append-only and attached to the
attempt/run record, not only printed to stdout.

Minimum fields:

| Field | Meaning |
|---|---|
| `bead_id` | Target bead. |
| `attempt_id` | Attempt/run id. |
| `routing_intent_source` | `cli`, `project_config`, `bead_hint`, `heuristic`, or `default`. |
| `requested_tier` | `cheap`, `standard`, `smart`, or empty when not tier-based. |
| `requested_min_power` | Resolved `MinPower`, when available. |
| `requested_max_power` | Resolved `MaxPower`, when available. |
| `smart_justification` | Extracted justification text when `requested_tier=smart`. |
| `actual_harness` | Harness reported by Fizeau/agent after execution. |
| `actual_provider` | Provider reported after execution. |
| `actual_model` | Model reported after execution. |
| `actual_power` | Actual power reported after execution. |
| `routing_intent_degraded` | True when requested intent could not be met or was overridden. |
| `routing_intent_note` | Short reason for degradation or override. |

The evidence event name should be stable, for example
`execution-routing-intent`. Existing attempt result fields already capture some
actual route facts; this event ties those facts back to why DDx requested that
power.

### Metrics

The agent metrics rollup should ingest the routing-intent fields once they are
present in run evidence. At minimum, operators need to answer:

- How many attempts used `tier:smart`?
- Which beads requested smart, and what was the justification?
- What was the success rate and cost of smart-hinted work compared with
  heuristic or default work?
- How often did CLI or project config override a bead hint?
- Which beads carried rejected durable route pins?
- Which authors or agents are adding smart hints repeatedly?

Initial metrics can be implemented by extending the normalized attempt
projection used by TD-032. A materialized rollup is not required.

Suggested dimensions:

| Dimension | Example |
|---|---|
| `routing_intent_source` | `bead_hint` |
| `requested_tier` | `smart` |
| `actual_power_bucket` | `>=80` |
| `degraded` | `true` |
| `bead_author` | `erik` or agent id when available |
| `smart_reason_category` | `architecture` |

Suggested counters:

- attempts by requested tier and source;
- success rate by requested tier;
- cost and token usage by requested tier;
- smart-hint count by bead author/agent;
- rejected durable route-pin count;
- override/degradation count.

### Operator Reporting

`ddx try` output should stay concise, but it should include the source when a
bead hint affects execution:

```text
routing intent: tier=smart source=bead_hint
```

If `tier:smart` is present without justification, the lint failure should be
plain:

```text
bead uses tier:smart but has no SMART JUSTIFICATION section
```

If a durable concrete pin is found:

```text
bead metadata contains execution-model=gpt-5.5; durable model pins are not
allowed. Use ddx try <id> --model ... for one-off debugging.
```

## Implementation Plan

### Bead 1: specify and test hint parsing

Scope:

- Parse `tier:*` labels into a typed execution hint.
- Extract `SMART JUSTIFICATION:` from bead descriptions.
- Detect forbidden durable route-pin fields and labels.

Acceptance:

1. Tests cover valid `tier:cheap`, `tier:standard`, and `tier:smart`.
2. Tests cover missing smart justification.
3. Tests cover forbidden concrete route-pin fields and labels.
4. Existing heuristic inference behavior remains unchanged when no hint exists.

### Bead 2: enforce hints in bead lifecycle lint

Scope:

- Add lint findings for missing smart justification.
- Add lint failures for durable concrete route pins.
- Keep diagnostics actionable.

Acceptance:

1. A smart bead without justification fails lint.
2. A bead with `harness:claude` or `execution-model=...` fails lint.
3. A bead with valid `tier:smart` and justification passes lint.

### Bead 3: record routing-intent evidence

Scope:

- Resolve execution-intent source during `ddx try` / `ddx work`.
- Attach `execution-routing-intent` evidence before execution.
- Update result/evidence tests.

Acceptance:

1. Attempts with `tier:smart` record source `bead_hint`.
2. CLI routing flags record source `cli`.
3. Heuristic inference records source `heuristic`.
4. Actual route facts are linked to requested intent when the attempt finishes.

### Bead 4: expose audit metrics

Scope:

- Extend the TD-032 normalized attempt projection with routing-intent fields.
- Add rollup dimensions/counters for requested tier, source, degradation, and
  rejected pins.

Acceptance:

1. Metrics can report smart-hinted attempt count over a time window.
2. Metrics can list smart-hinted beads and justifications.
3. Metrics can compare success/cost for bead-hinted vs heuristic/default work.

### Bead 5: update bead authoring guidance

Scope:

- Update bead authoring docs and DDx skills.
- Add examples for justified smart hints and rejected cargo-cult pins.

Acceptance:

1. Bead authoring template explains when to use `tier:smart`.
2. DDx skill guidance says not to persist harness/provider/model choices.
3. Skill validation passes.

## Non-Goals

- No durable bead-level concrete harness/provider/model pins.
- No DDx-side concrete route ranking or fallback.
- No change to Fizeau's routing algorithm.
- No materialized metrics store in the first implementation.
- No automatic promotion to `tier:smart` solely because a previous attempt used
  a particular harness or model.

## Open Questions

- Should `tier:smart` without justification be a hard blocker immediately, or
  start as a warning for one release?
- Should `tier:*` remain labels long term, or should DDx eventually expose a
  first-class `execution-tier` field while continuing to read labels for
  compatibility?
- What exact power floor should each tier map to once Fizeau catalog power
  numbers are stable enough to document?
</untrusted-data>
      </content>
    </ref>
  </governing>

  <diff rev="bb7d03ddc26a5a22cde202c021d71415bd72c638">
<untrusted-data>
diff --git a/.ddx/executions/20260506T210004-db8d7e99/checks/production-reachability.json b/.ddx/executions/20260506T210004-db8d7e99/checks/production-reachability.json
new file mode 100644
index 000000000..246408be7
--- /dev/null
+++ b/.ddx/executions/20260506T210004-db8d7e99/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T210004-db8d7e99/manifest.json b/.ddx/executions/20260506T210004-db8d7e99/manifest.json
new file mode 100644
index 000000000..d1a3c438b
--- /dev/null
+++ b/.ddx/executions/20260506T210004-db8d7e99/manifest.json
@@ -0,0 +1,133 @@
+{
+  "attempt_id": "20260506T210004-db8d7e99",
+  "bead_id": "ddx-312adc66",
+  "base_rev": "3e393cc89608ae0d765da41428932ea061d072e9",
+  "created_at": "2026-05-06T21:00:06.894531432Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-312adc66",
+    "title": "try: define and enforce bead-level execution hints",
+    "description": "PROBLEM\nDDx has a partial tier-label mechanism, but durable bead-level execution hints are underspecified. Agents can currently copy route-shaped settings into bead metadata without a clear lint/audit contract, and smart-tier usage is not tied to a required justification.\n\nROOT CAUSE\ndocs/helix/02-design/technical-designs/TD-037-bead-execution-hints.md:23-39 documents that `ddx try` infers tier from labels today, but arbitrary custom fields are preserved without a routing contract. docs/helix/02-design/technical-designs/TD-037-bead-execution-hints.md:62-92 requires smart justification, and lines 112-161 require durable route-pin rejection plus execution-routing-intent evidence.\n\nSMART JUSTIFICATION: This bead defines and implements the durable execution-hint contract that later prose beads rely on. Weak implementation would let agents persist concrete model or harness pins into queue metadata, directly violating ADR-024.\n\nPROPOSED FIX\n- Add typed parsing for `tier:cheap`, `tier:standard`, and `tier:smart` labels.\n- Extract and validate a `SMART JUSTIFICATION:` section for smart-tier beads.\n- Add bead lifecycle lint findings for missing smart justification and durable route-pin fields/labels.\n- Record `execution-routing-intent` evidence with source, requested tier/power, smart justification, actual route facts, and degradation notes.\n- Extend agent metrics projection enough to audit smart-hint usage and rejected durable route pins.\n\nNON-SCOPE\n- Do not add durable bead-level harness/provider/model/model-ref pins.\n- Do not change Fizeau routing internals.\n- Do not implement a materialized metrics rollup store.\n\nDEPS\nNo deps. This must land before the Vale/prose child beads rely on tier hints.",
+    "acceptance": "1. `TestExecutionHintParse_ValidTierLabels` covers `tier:cheap`, `tier:standard`, and `tier:smart` parsing.\n2. `TestExecutionHintParse_SmartRequiresJustification` covers missing and present `SMART JUSTIFICATION:` text.\n3. `TestExecutionHintLint_RejectsDurableRoutePins` covers forbidden fields/labels such as `execution-model` and `harness:claude`.\n4. `TestTryRecordsExecutionRoutingIntent` proves `ddx try` evidence records source `bead_hint`, requested tier, smart justification, and actual route facts when available.\n5. `TestAgentMetricsIncludesRoutingIntent` proves the normalized attempt projection can count smart-hinted attempts and rejected route pins.\n6. Existing heuristic inference tests still pass when no tier hint exists.\n7. cd cli \u0026\u0026 go test ./internal/escalation/... ./internal/bead/... ./internal/agent/... ./cmd/... ./internal/agentmetrics/... green.\n8. lefthook run pre-commit passes.",
+    "parent": "ddx-ccda7a32",
+    "labels": [
+      "phase:2",
+      "area:execution",
+      "area:beads",
+      "area:metrics",
+      "kind:design",
+      "kind:implementation",
+      "spec:TD-037",
+      "adr:024",
+      "tier:smart"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T21:00:04Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2049424",
+      "events": [
+        {
+          "actor": "erik",
+          "body": "{\"rationale\":\"\",\"score\":0,\"suggested_fixes\":null,\"waivers_applied\":null,\"warning\":\"lint hook: missing-harness\"}",
+          "created_at": "2026-05-06T20:56:12.8511039Z",
+          "kind": "bead-quality.lint",
+          "source": "ddx agent execute-loop",
+          "summary": "warning score=0"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"base_rev\":\"\",\"classification\":\"quota\",\"detail\":\"creating isolated worktree: git worktree add: Preparing worktree (detached HEAD 26ecf236e)\\nfatal: cannot create directory at '.claude/skills/library/.agents': No space left on device: exit status 128\",\"rationale\":\"The attempt failed before implementation because `git worktree add` could not create directories due to `No space left on device`. This is an environment/resource failure, not a code or test failure.\",\"recommended_action\":\"retry_after_cleanup\",\"result_rev\":\"\",\"session_id\":\"\",\"status\":\"execution_failed\",\"suggested_amendments\":\"Free disk space or remove stale worktrees, then rerun the bead. No bead content changes are indicated by this report.\",\"suggested_followup_beads\":[]}",
+          "created_at": "2026-05-06T20:56:22.699874821Z",
+          "kind": "bead-quality.triage",
+          "source": "ddx agent execute-loop",
+          "summary": "quota: retry_after_cleanup"
+        },
+        {
+          "actor": "erik",
+          "body": "creating isolated worktree: git worktree add: Preparing worktree (detached HEAD 26ecf236e)\nfatal: cannot create directory at '.claude/skills/library/.agents': No space left on device: exit status 128\noutcome_reason=quota",
+          "created_at": "2026-05-06T20:56:22.922260676Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"rationale\":\"\",\"score\":0,\"suggested_fixes\":null,\"waivers_applied\":null,\"warning\":\"lint hook: missing-harness\"}",
+          "created_at": "2026-05-06T20:56:56.474722376Z",
+          "kind": "bead-quality.lint",
+          "source": "ddx agent execute-loop",
+          "summary": "warning score=0"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"base_rev\":\"\",\"classification\":\"quota\",\"detail\":\"creating isolated worktree: git worktree add: Preparing worktree (detached HEAD b91027349)\\nfatal: cannot create directory at '.claude/skills/library/.agents': No space left on device: exit status 128\",\"rationale\":\"The attempt failed before implementation because `git worktree add` could not create directories due to `No space left on device`. This is an environment/resource failure, not a code or test failure.\",\"recommended_action\":\"retry_after_cleanup\",\"result_rev\":\"\",\"session_id\":\"\",\"status\":\"execution_failed\",\"suggested_amendments\":\"Free disk space or remove stale worktrees, then rerun the bead. No bead content changes are indicated by this report.\",\"suggested_followup_beads\":[]}",
+          "created_at": "2026-05-06T20:57:06.02314434Z",
+          "kind": "bead-quality.triage",
+          "source": "ddx agent execute-loop",
+          "summary": "quota: retry_after_cleanup"
+        },
+        {
+          "actor": "erik",
+          "body": "creating isolated worktree: git worktree add: Preparing worktree (detached HEAD b91027349)\nfatal: cannot create directory at '.claude/skills/library/.agents': No space left on device: exit status 128\noutcome_reason=quota",
+          "created_at": "2026-05-06T20:57:06.412709972Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"rationale\":\"\",\"score\":0,\"suggested_fixes\":null,\"waivers_applied\":null,\"warning\":\"lint hook: missing-harness\"}",
+          "created_at": "2026-05-06T20:57:42.231808322Z",
+          "kind": "bead-quality.lint",
+          "source": "ddx agent execute-loop",
+          "summary": "warning score=0"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"bead_id\":\"ddx-312adc66\",\"body\":\"{\\\"bead_id\\\":\\\"ddx-312adc66\\\",\\\"cleanup_summary\\\":{\\\"project_root\\\":\\\"/Users/erik/Projects/ddx\\\",\\\"temp_root\\\":\\\"/tmp/ddx-exec-wt\\\",\\\"scanned_temp_dirs\\\":1946,\\\"scanned_evidence_dirs\\\":3080,\\\"complete_evidence_dirs\\\":2954,\\\"removed_unregistered_temp_dirs\\\":0,\\\"removed_registered_worktrees\\\":0,\\\"removed_run_state_files\\\":1,\\\"bytes_reclaimed\\\":220,\\\"inodes_reclaimed\\\":1,\\\"observations\\\":[{\\\"path\\\":\\\"/Users/erik/Projects/ddx/.ddx/run-state.json\\\",\\\"class\\\":\\\"removed_run_state\\\",\\\"message\\\":\\\"stale run-state\\\",\\\"bytes\\\":220,\\\"inodes\\\":1},{\\\"path\\\":\\\".ddx/executions/20260411T005226-802deaa1\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T162344-2757c165\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T162848-8d1606ff\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T164126-7e61cfc8\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T164804-a0dfbaa1\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T165740-77adba47\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T171014-ef29d046\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T171350-b9afc859\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T171656-78987c7a\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T172131-58e51afe\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T173142-f640b281\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T183832-f4a94877\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T185052-565dc80e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T190456-1ec9de41\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T190651-b85e382c\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T190840-b25e589b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T191529-82d2f322\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T192526-49fafab7\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T192808-cb37ddc0\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T192812-453c47b7\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T193523-45d1299a\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T193815-26708ceb\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T195303-5b6c0922\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T195437-e3b15adb\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T200240-fe1e028d\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T200949-32150c11\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T201511-697788d9\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T202635-e59e915a\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T202820-f75728b1\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T222147-ba70466e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T222345-082b98d3\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T222523-b2a6ffd5\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T230600-4a2b7413\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T230647-d58f29bb\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T232229-109f2287\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T232317-9e1a3a19\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260411T234102-621d888d\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T000203-78fdeb4b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T004053-ef3dacb7\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T015918-ede0ff09\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T023416-cebe304b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T023733-20da18ea\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T023810-b70791eb\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T024102-285b1112\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T201056-79e754dc\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T201304-996f887b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T203037-f65978cb\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T203219-8611dc8f\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T215031-3cd7d0a4\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T215434-434be67c\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T220919-3f461aaa\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T223238-afc5452e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T224350-56cc2477\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T225520-8e3ab730\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T225813-52591a3a\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T232137-eb37b6a6\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T233935-7a0c3b10\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260412T235146-dadad24b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T000311-3e0f1773\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T000811-fce83fbe\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T001134-0fbff2ae\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T001512-016ba6b8\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T001902-fd674986\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T002116-867e31a3\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T003942-cc4e9de8\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T005200-aa4ecad7\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T014101-e537ef70\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T014520-c8d85259\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T021717-1ba7d848\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T022210-e9ccb014\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T022948-ca285162\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T023735-7cb9e71e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T023941-b6b621ee\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T024958-99c057f0\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T031645-9ebf4776\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T031650-f5ba36d2\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T032219-8374270b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T032603-1fcb52bc\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T032603-7557e1fe\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T125906-1a11c02d\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T132123-51bdee52\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T134203-453c7172\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T140544-6b4034a1\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T141126-52c44a18\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T141735-98f342eb\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T141929-ede30ece\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T143934-2f42fe98\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T144349-a9d6dc35\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T145945-e33a9d4f\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T151612-d792df6e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T151809-648e5ef9\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T153034-7d956f17\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T162447-efe9c33e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T163811-4ccc1cad\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204244-f77a0339\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204247-26d4bf09\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204257-8653f6e2\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204300-9c024789\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204310-5dd97438\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204311-ef864d1e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204313-3a1a685b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204511-c5352e1b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204515-ec142b4e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204516-07f5a3b2\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204518-8424555a\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204520-6aba887d\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204522-89293e6f\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204539-660a45c7\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204541-d3b9edb1\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204543-792c4c0e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204544-85356e4f\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204545-f8982959\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204548-fb100d16\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204549-7aee123d\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204551-523dab83\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T204552-924b067d\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T214025-f68a328b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T214026-c2bc0e97\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T214907-a42abbf6\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T215017-924d79d1\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T215420-e03f9e58\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260413T215908-869920b7\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T030840-ac8f120a\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T033456-705893f8\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T033525-2410b3cb\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T033937-54e37833\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T035512-9a1cf3b6\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T040114-265f7481\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T041536-d9102457\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T041927-95d5114d\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T045407-712b4202\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T050951-6ec3a06b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T051615-d657e642\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T052012-77b85dc6\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T053406-2ea4a719\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T053633-822f2771\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T061844-fbb3406a\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T063427-58ba3504\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T065444-e19d722e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T070408-e04c3213\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T072328-72334472\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T073103-1cebdc41\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T074516-d59da8cf\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T081337-928daf7a\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T082148-b6ff5c97\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T084358-38e40267\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T085552-c4932fc5\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T090951-f5a723ed\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T092747-620f8908\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T093731-81b59d8c\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T095146-cc6a6bd5\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T101322-a7e5a51d\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T183632-0fd517f4\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T192339-18ff581e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T192610-541a0805\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T193416-e6849093\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T194818-c3298886\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T195328-0cb99fe6\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T195531-b8a72298\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T195729-8c11530f\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T200653-08a08e4a\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T201510-d6ca13e1\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T201715-538939a9\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T202446-fc51a51f\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T203413-62bccc55\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T203656-29f10d08\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T204210-6e06446b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T204617-6a351e39\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T204851-10260b9a\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T205149-4605b15c\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T205715-0e4b7b6e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T210624-dbe2537e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T213903-42ba8a86\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T215848-4a1e4817\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T222629-739026bd\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T224458-37674b95\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T225025-1986fbb0\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T225559-87c3d299\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T231403-7acf4688\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T231404-1a67e898\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T231405-fee79642\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T233121-d21f6abd\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T235058-b758a19e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260414T235940-2ab945e0\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T000842-6d8bc1e9\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T001522-24e9a21a\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T002835-04a358cc\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T003935-79817a17\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T005249-1a2f354f\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T010645-96bbad72\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T010948-6530e450\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T011117-5434ab28\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T011331-36e59382\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T012104-9fde3672\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T012939-e548f96b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T013551-60ab22d4\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T015231-c2ffa9ba\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T035232-59494b35\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T040354-792a2739\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T040629-993fc148\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T040630-c7445ab6\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T040703-8adb81a0\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T042450-19120590\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T044256-183ff980\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T045751-302274f4\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T050415-ecedfa7a\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T052610-c3db02b2\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T053655-96cfa2ec\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T053924-05dbebde\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T054253-20cd9673\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T055750-15712870\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T062435-4255f769\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T062542-ad6648f5\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T063516-1ed3d210\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T063928-c396f762\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T064230-5edf57a7\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T064906-3f5d8560\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T065117-d3770426\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T065358-87f6ae04\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T071008-a6447823\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T072233-536841ac\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T073932-77305af3\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T074900-a4fb1db6\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T075929-eed47b07\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T081657-9e3822f2\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T082449-5fc8cbf5\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T083447-c9899645\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T083630-1b069c67\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T083712-f69fc612\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T084529-bff6b78f\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T084818-7a51633e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T085246-c2c99779\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T085527-2084f875\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T090347-f445ea68\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T091108-b5f8f3d7\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T091721-b85585ac\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T092314-b3e9a7a0\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T093332-fa21cc5a\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T094125-eafecd9d\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T094339-672dea9e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T094932-674ae109\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T095537-0e53c6ac\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T100120-1ab2b684\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T100502-0fd1e7b0\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T101014-435ffc05\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T102047-b9dac2c7\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T102452-3a344a76\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T102752-40518e6a\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T103422-bb3a45fa\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T115007-d936cde7\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T120749-0108c0d5\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T120757-158fe57f\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T120806-81cd8eb5\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260415T120814-9514778c\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T044613-0f04e8ce\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T044614-0d079bcd\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T044736-e5405c7f\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T052120-a39d912f\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T124345-913ab52a\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T124346-9d5de82a\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T125427-16c6bdd6\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T125428-23d84ea5\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T125943-1418beba\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T125944-d1254ba9\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T125945-90996348\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T130158-d53a188d\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T130159-04dfe02b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T130718-d626ad84\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T130719-6fd6a70b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T130720-28dbd848\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T180825-2aaff03d\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T181205-b5419982\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T193602-1a4b45bf\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T195403-f1506333\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T195405-2fb489af\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T201324-7957de9f\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T202841-36133ca5\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T202842-3d9d211b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T203015-a3c3572d\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T203015-be7ac4f6\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T203941-8cc5dbfb\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T203942-0bfb1aa4\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T212529-c6fa6346\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T213758-dec6c48e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T214943-1587e182\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T215953-6b515c05\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T215954-caa53b4b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T215955-b5b8dfc2\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T221104-ed0d22b9\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T221105-f85ee523\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T221106-ba9ce13d\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T221422-7dc1994f\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T221424-6289a8f8\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T221424-87150f6f\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T230313-e193942b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T230315-f9f46822\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260416T230316-efbf91c5\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260417T011206-3cfbb46c\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260417T021428-1d6d7eff\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260417T021800-875e86e0\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260417T022759-068f2ba5\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260417T023814-a6807535\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260417T025019-e7dfdf1b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260417T025537-8fcab1d8\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260417T025753-f5eb9e18\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260417T171835-e5f1b6a5\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260417T171843-ae5de58e\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260417T171852-faefd3d8\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260417T182339-81f075dd\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260417T182341-5a99310a\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260417T182640-ecc80289\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260417T194816-38d98bba\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260417T200552-505a00b9\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260418T005000-cfeffe49\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260418T005009-71d5356c\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260418T005017-ac81ac97\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260418T005052-3aad2cfa\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260418T011805-e2b15830\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260418T011848-50b8eb2d\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260418T013722-73a5bd53\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260418T013759-cf8fcc56\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260418T015654-977ef255\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260418T020249-41c2cc27\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260418T021319-5c32287b\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260418T021346-948dfcde\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260418T023901-ba3d8d59\\\",\\\"class\\\":\\\"complete_evidence\\\",\\\"message\\\":\\\"complete evidence preserved\\\"},{\\\"path\\\":\\\".ddx/executions/20260418T030326-2b94449b\\\",\\\"class\\\"\n…[truncated, 746757 bytes]\n0506T134233-f95633fb\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T134238-c8adc766\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T134523-ec2dca21\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T134528-de250041\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T134918-4df98c19\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T134923-d337b677\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T135251-17486baa\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T135256-1d75e7b8\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T135613-b0ce38cb\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T135618-ae9f1984\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T135808-98902ac7\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T135954-bdad87d8\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T140000-ef4e0ae8\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T140129-b42f6985\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T140134-c495fce8\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T140508-bbf728d5\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T140513-bc6087c8\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T141045-834f63c4\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T141050-7a9d90ed\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T141334-9e44a459\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T141339-3059faf8\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T141733-ca2104dd\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T141738-980d8c98\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T141956-d6cc6484\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T142031-4391630a\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T142149-3b93b481\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T142155-81d34838\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T142313-527fbd79\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T142405-ea7d271a\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T142640-468967b4\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T142645-a7272dfc\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T145023-ed0f1d9c\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T145038-f5564f67\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T145150-14954a3f\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T145155-81a3996a\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T145437-314ce0ea\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T145833-dfa41429\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T150012-57a9cb0b\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T150159-5230d201\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T150539-64e6a23c\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T150904-7ee5eebd\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T151142-ee67431d\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T151536-16066906\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T151851-d62a93a8\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T154152-4a9c452c\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T154343-ae2fdd7f\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T154739-dc6ff0dc\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T155440-9c31debb\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T155630-c65112d2\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T155809-36719f72\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T155819-eb742e6b\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T155958-1a83298b\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T160004-dcb78bea\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T160123-7b468ece\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T160143-7c1f3a0f\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T161039-4745079a\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T161129-3ecbe3cc\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T161425-8c0efb51\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T161506-90ff8bcc\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T161645-3bd5e886\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T161717-f18e34ad\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T161959-5dce225e\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T162204-ef6ac6d1\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T162332-8f4b5e5b\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T162434-008e8084\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T162645-4df515d2\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T162849-7b34fa61\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T163038-2d060d83\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T163247-e918a496\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T163353-ba20f527\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T163511-7a594d13\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T163621-e616f678\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T163724-11ea3569\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T163730-b7d62098\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T163903-fb96d6fd\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T164059-5b9f3577\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T164231-3fcc6761\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T164422-0f6774ee\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T164833-bb56c0b0\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T164955-75f475db\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T165037-eedd8843\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T165249-d56ad464\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T165835-ee9e61c2\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T170009-a769e56e\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T170034-43fc95ff\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T170122-3b9a6ef7\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T170325-1dec34ad\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T170543-42845b05\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T170754-d8fff4de\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T171059-330b6d6f\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T171127-57687897\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T171415-2121d06d\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T171419-9643ec64\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T171752-9eaf51c0\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T171757-d404c7b7\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T172022-272b28ba\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T172335-9200bc74\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T172523-0e09eeeb\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T172602-a05fdf74\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T172641-48134d22\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T172809-7a0a7018\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T172936-452214ec\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T173123-e1a2cc6d\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T174057-a21f0388\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T174109-7cb23794\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T174302-cf9b16ae\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T174329-7cffbd58\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T174424-cd9b7dac\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T174637-2156c2f9\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T174753-be83c63b\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T174923-28767743\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T175119-173d50d4\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T175124-8003f8a3\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T175510-510a9490\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T175523-c95a730b\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T175820-6b50a007\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T175903-41ec02c9\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T180150-1000e789\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T180213-d848f356\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T180436-558a2e67\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T181734-f77c255d\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T181741-d70d13e0\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T182018-b82879d1\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T182108-9f0a2b19\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T182209-035c6b0c\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T182334-12f3230e\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T182403-bb67b65f\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T182545-950e34a5\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T182657-3c253895\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T182913-7b856a5f\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T182921-85eba348\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T183150-aab84fd7\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T183209-610097ad\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T183424-db372b8a\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T183441-2f9a30e8\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T183820-0b0e69c1\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T184047-2823137b\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T184139-e699eea1\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T184246-62a07a8f\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T184453-976ea6c9\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T184507-e9ada2fe\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T184812-d92eca28\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T184925-7d5fcfcf\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T185128-3f21f122\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T185244-0c7e24ef\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T185513-2871b449\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T185747-9b0cb280\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T185853-562e47a8\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T190232-73748d69\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T190721-dc1a0aba\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T191135-b2f075f4\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T191539-c4e56137\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T191949-984c1c8b\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T192327-a596d815\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T192714-dc196377\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T193127-c1edb762\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T193549-dfafc1a1\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T193554-4c7a6e00\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T193917-468fb55f\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T194055-3bb95a4a\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T194429-3843fd24\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T194535-6949a2ed\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T195018-f7b68416\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T195429-99977a35\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T195832-534f6a33\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T200326-78e0f5f8\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T200734-c714923f\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T201253-9c25205c\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T201735-b4a95e55\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T202414-537456d8\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T202905-60c53cbd\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T203343-f20f2504\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T203752-6ff73ced\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T204212-d99ac295\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T204621-9619ca31\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T205029-195cc25a\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"},{\"path\":\".ddx/executions/20260506T205538-12817547\",\"class\":\"complete_evidence\",\"message\":\"complete evidence preserved\"}]},\"detail\":\"resource_exhausted\",\"evidence_roots\":[\"/Users/erik/Projects/ddx/.ddx/executions\",\"/Users/erik/Projects/ddx/.ddx/runs\"],\"project_root\":\"/Users/erik/Projects/ddx\",\"root_checks\":[{\"path\":\"/tmp/ddx-exec-wt\",\"writable\":true,\"bytes_free\":19176202240,\"inodes_free\":374,\"notes\":[\"free inodes 374 \\u003c required 1024\"]},{\"path\":\"/Users/erik/Projects/ddx/.ddx/executions\",\"writable\":true,\"bytes_free\":161065740533760,\"inodes_free\":1849203104},{\"path\":\"/Users/erik/Projects/ddx/.ddx/runs\",\"writable\":true,\"bytes_free\":161065740533760,\"inodes_free\":1849203104}],\"status\":\"resource_exhausted\",\"temp_root\":\"/tmp/ddx-exec-wt\"}",
+          "created_at": "2026-05-06T20:57:42.96346171Z",
+          "kind": "resource-exhausted",
+          "source": "ddx agent execute-loop",
+          "summary": "resource exhausted after cleanup; stopping work loop"
+        },
+        {
+          "actor": "erik",
+          "body": "resource_exhausted",
+          "created_at": "2026-05-06T20:57:43.194961391Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "resource_exhausted"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"rationale\":\"\",\"score\":0,\"suggested_fixes\":null,\"waivers_applied\":null,\"warning\":\"lint hook: missing-harness\"}",
+          "created_at": "2026-05-06T21:00:03.746429443Z",
+          "kind": "bead-quality.lint",
+          "source": "ddx agent execute-loop",
+          "summary": "warning score=0"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-06T21:00:04.150632813Z",
+      "spec-id": "TD-037"
+    }
+  },
+  "governing": [
+    {
+      "id": "TD-037",
+      "path": "docs/helix/02-design/technical-designs/TD-037-bead-execution-hints.md",
+      "title": "Technical Design: Bead-Level Execution Hints"
+    }
+  ],
+  "paths": {
+    "dir": ".ddx/executions/20260506T210004-db8d7e99",
+    "prompt": ".ddx/executions/20260506T210004-db8d7e99/prompt.md",
+    "manifest": ".ddx/executions/20260506T210004-db8d7e99/manifest.json",
+    "result": ".ddx/executions/20260506T210004-db8d7e99/result.json",
+    "checks": ".ddx/executions/20260506T210004-db8d7e99/checks.json",
+    "usage": ".ddx/executions/20260506T210004-db8d7e99/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-312adc66-20260506T210004-db8d7e99"
+  },
+  "prompt_sha": "eee50898f7cca6b470fb0befe0c885e36c8ea0ab301af6bc0260ef5875a687b6"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T210004-db8d7e99/result.json b/.ddx/executions/20260506T210004-db8d7e99/result.json
new file mode 100644
index 000000000..cd5ab90fb
--- /dev/null
+++ b/.ddx/executions/20260506T210004-db8d7e99/result.json
@@ -0,0 +1,24 @@
+{
+  "bead_id": "ddx-312adc66",
+  "attempt_id": "20260506T210004-db8d7e99",
+  "base_rev": "3e393cc89608ae0d765da41428932ea061d072e9",
+  "result_rev": "7edcd80fba056a076dcb9eb0e0f096c0c7401e4e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-279220d2",
+  "duration_ms": 1181268,
+  "tokens": 25416438,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T210004-db8d7e99",
+  "prompt_file": ".ddx/executions/20260506T210004-db8d7e99/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T210004-db8d7e99/manifest.json",
+  "result_file": ".ddx/executions/20260506T210004-db8d7e99/result.json",
+  "usage_file": ".ddx/executions/20260506T210004-db8d7e99/usage.json",
+  "started_at": "2026-05-06T21:00:06.895603848Z",
+  "finished_at": "2026-05-06T21:19:48.164257965Z"
+}
\ No newline at end of file
</untrusted-data>
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
