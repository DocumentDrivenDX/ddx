<bead-review>
  <bead id="ddx-e3493010" iter=1>
    <title>docs: add 'Resolution Path' developer guide tracing CLI flag -&gt; final HTTP dispatch</title>
    <description>
## Problem

No single document traces the provider/model/harness resolution path from CLI flags to the final HTTP request. SD-015 and SD-023 are SPECS — they describe what should happen in 5 routing modes. The code lives across routing.go, discovery.go, agent_runner.go, runner.go, and agent_cmd.go. A newcomer debugging a routing bug must infer the flow by reading all of them.

The audit for epic ddx-2d974641 turned up a RouteRequest struct missing its Provider field, and a parallel RunOptions.Provider path that bypasses routing — neither of which is documented. This is the kind of hidden structural defect a Resolution Path document would have flagged on first read.

## Fix

Write docs/helix/02-design/solution-designs/SD-015-resolution-path-trace.md (or similar location; coordinate with SD-015's author). The document traces one concrete invocation end-to-end:

    ddx work --once --local --provider vidar-omlx --model-ref qwen/qwen3.6

Walk through every layer, with file:line citations:

1. **CLI parse** — cli/cmd/agent_cmd.go flag extraction (line references).

2. **RouteFlags -&gt; RouteRequest normalization** — routing.go:NormalizeRouteRequest. What each CLI flag maps to; which flags are discarded when they don't match an expected shape; the current gap around --provider.

3. **Discovery probe** — discovery.go:DiscoverProviderModels. When it fires; what it caches; the 30s TTL; how failures are handled.

4. **Candidate planning** — routing.go:BuildCandidatePlans / evaluateCandidate. For each registered harness, the state map lookup and the viability checks.

5. **Fuzzy matching** — discovery.go:FuzzyMatchModel. Pool selection (global vs per-provider), normalization (current gaps), prefix + tiebreak algorithm.

6. **Scoring** — routing.go:scoreCandidate. Per-profile scoring, cost class, historical success rate, provider-affinity bonus (when implemented).

7. **Candidate ranking** — routing.go:RankCandidates / SelectBestCandidate. Non-viable rejection, stable tiebreak.

8. **Dispatch** — runner.go:Run. Harness-specific branches (virtual, agent, script, HTTP-provider-embedded, default exec). For embedded: agent_runner.go:RunAgent -&gt; resolveEmbeddedAgentProvider -&gt; agent library call.

9. **Wire-level request** — ddx-agent builds the HTTP body (system_prompt + messages + tools), sends to the provider. What the server receives.

10. **Response parsing** — streaming or non-streaming decode; where 'unexpected end of JSON input' (ddx-6a5dfe35) surfaces.

Include a SEQUENCE DIAGRAM (mermaid or ASCII) showing the layers.

Include a GAP TABLE at the top listing every currently-missing piece (as of whatever the SD-015 implementation state is) with links to the specific beads tracking each gap.

Keep the document MAINTAINED alongside the code: update it when RouteRequest gains a field, when a new routing mode lands, when fuzzy match gains a normalization step.

## Files likely touched

- docs/helix/02-design/solution-designs/SD-015-resolution-path-trace.md (new)
- docs/helix/01-frame/features/FEAT-006-agent-service.md — add a single-line pointer at the top of the 'Overview' section
- skills/ddx/reference/agents.md — add a one-line pointer in 'Harness / profile / persona dispatch' section so agents consulting the skill can follow up to the full trace when needed

## Out of scope

- Rewriting SD-015 or SD-023 — this doc complements them; they describe intent, this describes current reality.
- Documenting future routing modes that haven't landed.
    </description>
    <acceptance>
1. docs/helix/02-design/solution-designs/SD-015-resolution-path-trace.md exists and documents all 10 layers above with file:line citations current as of the document's creation date.

2. The document contains a sequence diagram showing CLI -&gt; NormalizeRouteRequest -&gt; Discovery -&gt; BuildCandidatePlans -&gt; SelectBestCandidate -&gt; Run -&gt; RunAgent -&gt; agent library -&gt; HTTP.

3. The document contains a 'Current Gaps' table with at least the 4 confirmed gaps from epic ddx-2d974641 and links to the tracking beads.

4. FEAT-006 and skills/ddx/reference/agents.md each have a one-line pointer to the new doc.

5. **CI grep verifies citations resolve** (per opus fresh-eyes review). A CI step (or lefthook hook) parses every file:line citation in SD-015-resolution-path-trace.md and verifies the named symbol/line pattern exists in the current source tree. Prevents the doc from rotting silently as routing.go / discovery.go / types.go evolve. Failing citations block the PR.
    </acceptance>
    <labels>ddx, phase:build, kind:documentation, area:agent, area:routing</labels>
  </bead>

  <governing>
    <ref id="SD-015" path=".claude/worktrees/agent-a0673989/docs/helix/02-design/solution-designs/SD-015-agent-routing-and-catalog-resolution.md" title="Solution Design: Agent Routing and Catalog Resolution">
      <content>
---
ddx:
  id: SD-015
  depends_on:
    - FEAT-001
    - FEAT-006
---
# Solution Design: Agent Routing and Catalog Resolution

## Overview

DDx should route ordinary agent requests by intent and normalized routing
signals, not by harness name alone.
Users primarily express:

- a profile such as `cheap`, `fast`, or `smart`
- a model ref or exact pin such as `qwen3` or `claude-sonnet-4-20250514`
- an effort level such as `high`

DDx then decides which harness should execute the request. Once DDx chooses
the embedded harness, the embedded `ddx-agent` runtime chooses its own
provider/backend internally.

This design keeps the boundary explicit:

- DDx owns cross-harness routing and guardrails
- embedded `ddx-agent` owns shared model-catalog data and provider/backend
  selection inside the embedded runtime

## Routing Inputs

DDx normalizes every agent invocation into a `RouteRequest`:

- `Profile`
- `ModelRef`
- `ModelPin`
- `Effort`
- `Permissions`
- `HarnessOverride`

Interpretation:

- `ModelRef` is a logical catalog ref or alias that should be projected onto
  harness-specific surfaces
- `ModelPin` is an exact concrete model string that bypasses catalog policy
- `HarnessOverride` constrains routing to one harness only

## Resolution Order

1. If `HarnessOverride` is set, evaluate only that harness.
2. Else if `ModelRef` is present, attempt shared-catalog resolution on every
   harness surface.
3. Else if `Profile` is present, resolve the profile and evaluate all harnesses
   that can satisfy it.
4. Else use the configured default profile.
5. If no profile is configured, fall back to the configured default harness.

If a `--model` value fails catalog resolution for all harness surfaces, DDx
reinterprets it as an exact `ModelPin`.

## Shared Catalog Use

DDx consumes the shared embedded-runtime catalog for:

- aliases
- shared profiles
- canonical targets
- deprecation/replacement metadata
- surface-specific concrete model strings

DDx does not own concrete production model tables except as temporary fallback
during migration.

### Surfaces

Initial DDx routing should recognize at least:

- embedded OpenAI-compatible surface
- embedded Anthropic surface
- Codex surface
- Claude Code surface

This means a request such as `qwen3` can legitimately resolve only to the
embedded harness.

## Candidate Planning

DDx evaluates one `CandidatePlan` per harness.

`CanonicalTarget` is the stable attribution key for downstream routing
metrics. When a request resolves only to an exact concrete model pin, DDx
records that concrete model in the same attribution key space so observations
for different resolved models stay separate.

Each plan records:

- `Harness`
- `Surface`
- `RequestedRef`
- `CanonicalTarget`
- `ConcreteModel`
- `SupportsEffort`
- `SupportsPermissions`
- `Installed`
- `Reachable`
- `Authenticated`
- `QuotaState`
- `SignalFreshness`
- `CostClass`
- `EstimatedCostUSD`
- `PerformanceMetrics`
- `RejectReason`
- `Score`

The selected plan should be explainable from this record.

## Harness Capability Model

### Static Capability

Each harness publishes:

- catalog surface
- exact-pin support
- supported effort values
- supported permission modes
- local/cloud classification
- cost class or pricing metadata when known

### Dynamic State

Each harness also has runtime state:

- installed
- reachable
- authenticated
- quota or headroom state: `ok`, `blocked`, or `unknown`
- policy-restricted
- healthy / degraded / unavailable
- last checked timestamp
- signal source and freshness

This state should be cached with TTLs rather than fully reprobed on every run.

## Routing Signal Model

DDx routes using a normalized model composed from multiple signal families:

- **Capability** — whether the harness can satisfy the requested profile,
  model, effort, and permission mode
- **Availability** — installed, reachable, authenticated, policy-allowed
- **Quota/headroom** — current provider limit state when a trustworthy source is
  available; otherwise `unknown`
- **Cost** — provider-reported cost where available, otherwise DDx-owned cost
  estimate or coarse cost class
- **Performance** — minimal DDx-observed metrics such as recent latency and
  recent success/failure
- **Freshness** — when each dynamic signal was last observed

Signal ownership is intentionally split:

- **Provider-native sources** own transcripts, rich session history, and
  current quota/headroom when available
- **DDx** owns only the normalized view and the minimal observed metrics needed
  to compare harnesses at dispatch time
- **Embedded `ddx-agent`** owns its runtime telemetry; DDx consumes references
  and derived metrics rather than re-implementing runtime logging

### Minimal DDx-Owned Routing Metrics

DDx keeps only compact routing facts needed to rank harnesses. It does not
store provider transcripts, provider session stores, or embedded-runtime log
bodies as part of routing state.

#### `RoutingOutcome`

One bounded sample per DDx-observed invocation, used to summarize reliability
and latency.

- `harness`
- `surface`
- `canonical_target`
- `observed_at`
- `success`
- `latency_ms`
- `input_tokens` when available
- `output_tokens` when available
- `cost_usd` when available
- `native_session_id`, `native_log_ref`, `trace_id`, or `span_id` when the
  source provides a reference instead of a body

#### `QuotaSnapshot`

One bounded sample per live-probe or cached quota read, used to model headroom
and subscription pressure.

- `harness`
- `surface`
- `canonical_target`
- `source`
- `observed_at`
- `quota_state`
- `used_percent` when available
- `window_minutes` when available
- `resets_at` when available
- `sample_kind`
  - `native-log`
  - `async-probe`
  - `cache`

#### `BurnSummary`

One derived record per harness/surface/canonical-target group, used to compare
providers without direct billing APIs.

- `harness`
- `surface`
- `canonical_target`
- `observed_at`
- `burn_index` as a relative, unitless score
- `trend`
- `confidence`
- `basis` describing which snapshot deltas and token/cost observations fed the
  score

#### Freshness And Retention

- Outcome samples are fresh for one routing TTL after observation; older
  samples remain inspectable but are demoted behind fresher data.
- Keep a rolling window of the most recent 50 outcome samples or 7 days of
  samples per resolved `canonical_target` or exact model-pin equivalent,
  whichever is smaller.
- Keep quota snapshots for 30 days or one billing window, whichever is
  smaller, and compact older snapshots into the burn summary rather than
  retaining them as raw routing inputs.
- Snapshot freshness is source-specific: provider-native logs are fresh until
  the next known provider write, async probes are fresh until their source TTL,
  and cached values are only as fresh as their cache timestamp.
- Burn calculations restart after a quota reset and use only post-reset
  snapshots for the next accumulation interval.

#### Boundary And Exclusions

- DDx never stores provider prompt/response bodies or full native session
  transcripts in routing metrics.
- DDx never re-implements embedded `ddx-agent` session logs or OTEL storage.
- When embedded telemetry is available, DDx stores only references and
  derived facts such as `session_id`, `trace_id`, `span_id`, and the routing
  outcome samples above.
- Routing logic consumes those references and derived metrics; it does not
  need the underlying transcript or log payloads to rank harnesses.

### Source Precedence

- **Codex current quota/headroom** should come from native Codex session JSONL
  when persistence is enabled. If the log is missing or unreadable, treat the
  value as `unknown` rather than fabricating a headroom state. PTY `/status`
  automation is not the default design.
- **Claude historical usage** should come from `~/.claude/stats-cache.json`.
  That cache is the stable source for account-wide usage history, but not for
  current quota/headroom.
- **Claude current quota/headroom** should use a stable non-PTY source if one
  exists. PTY automation is an explicit fallback of last resort and, if used,
  should update an async snapshot cache rather than block routing on inline
  terminal scraping.
- **embedded `ddx-agent` telemetry** should contribute DDx-observed
  performance, reliability, and provenance metrics, not provider quota state.
- **Performance metrics** should come from DDx-observed runs, including async
  snapshot history when DDx must actively sample a live quota source.

## Candidate Rejection Rules

A candidate must be rejected when any of these are true:

- the harness cannot project the requested profile or model to its catalog surface
- the harness does not support the requested effort level
- the harness does not support the requested permission mode
- the harness is not installed
- the harness is installed but not reachable
- the harness lacks required auth
- the harness is explicitly quota-blocked
- the harness is disabled by config or policy
- the harness cannot accept an exact raw pin when the request bypasses the catalog

Rejected candidates remain inspectable via `ddx agent doctor`, `capabilities`,
and future explain/debug modes.

## Ranking Rules

Valid candidates are ranked by:

1. exactness of model/profile match
2. health and confidence of current state
3. freshness and quality of current routing signals
3. intent:
   - `cheap` prefers lowest-cost viable candidate
   - `fast` prefers fastest viable candidate within acceptable cost bounds
   - `smart` prefers highest-quality viable candidate
4. DDx-observed performance and reliability
5. local over cloud when otherwise equivalent
6. stable tie-breaker order

## Embedded Runtime Boundary

When DDx selects the embedded harness:

- DDx passes the resolved profile/model intent into the embedded runtime
- DDx does not select a concrete provider/backend itself
- embedded `ddx-agent` resolves backend pools, provider type, and strategy

Therefore DDx must never duplicate embedded backend-pool logic.

## CLI and Config Direction

### CLI

Preferred:

```bash
ddx agent run --profile cheap --prompt task.md
ddx agent run --model qwen3 --effort high --prompt task.md
```

Advanced override:

```bash
ddx agent run --harness codex --prompt task.md
```

### Config

```yaml
agent:
  profile: cheap
  harness: ""
  model: ""
  permissions: supervised
```

`harness` remains optional and mostly for operator override or debugging.

## `capabilities` and `doctor`

`ddx agent capabilities <harness>` should evolve to show:

- reasoning levels
- exact-pin support
- effective profile/model mappings
- deprecation/replacement warnings

`ddx agent doctor` should evolve to report:

- installed
- reachable
- authenticated
- quota/headroom state
- degraded vs healthy
- source and freshness for dynamic signals
- whether the embedded harness has at least one viable backend for default
  routing

## Implementation Notes

- The current hardcoded DDx model tables in the agent package are transitional
  only.
- Cross-harness routing belongs in DDx.
- Provider/backend selection belongs in embedded `ddx-agent`.
- DDx must not suppress native persistence for external harnesses by default,
  because native provider stores are part of the routing signal surface.
- Exact-model asks such as `qwen3` must be handled without special cases once
  the shared catalog projections are in place.

## Open Questions

- Should DDx expose a user-visible `--explain-routing` or debug output mode for
  rejected candidates?
- How much coarse pricing metadata should DDx own locally versus delegating to
  `ddx-agent` or harness-specific adapters?
- What stable non-PTY current-quota source, if any, can DDx use for Claude
  Code?
- Should `embedded` remain an alias only, or also be the canonical persisted
  harness name in DDx logs?
      </content>
    </ref>
  </governing>

  <diff rev="790f3b9752c1b3b289fd8f3725291fd5b16ed067">
commit 790f3b9752c1b3b289fd8f3725291fd5b16ed067
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 18 02:47:51 2026 -0400

    chore: add execution evidence [20260418T063750-]

diff --git a/.ddx/executions/20260418T063750-16fb3982/manifest.json b/.ddx/executions/20260418T063750-16fb3982/manifest.json
new file mode 100644
index 00000000..ec0027c7
--- /dev/null
+++ b/.ddx/executions/20260418T063750-16fb3982/manifest.json
@@ -0,0 +1,65 @@
+{
+  "attempt_id": "20260418T063750-16fb3982",
+  "bead_id": "ddx-e3493010",
+  "base_rev": "654b4cdcd9200fa8db6c38d6af9dad2acfbaa779",
+  "created_at": "2026-04-18T06:37:51.048634219Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-e3493010",
+    "title": "docs: add 'Resolution Path' developer guide tracing CLI flag -\u003e final HTTP dispatch",
+    "description": "## Problem\n\nNo single document traces the provider/model/harness resolution path from CLI flags to the final HTTP request. SD-015 and SD-023 are SPECS — they describe what should happen in 5 routing modes. The code lives across routing.go, discovery.go, agent_runner.go, runner.go, and agent_cmd.go. A newcomer debugging a routing bug must infer the flow by reading all of them.\n\nThe audit for epic ddx-2d974641 turned up a RouteRequest struct missing its Provider field, and a parallel RunOptions.Provider path that bypasses routing — neither of which is documented. This is the kind of hidden structural defect a Resolution Path document would have flagged on first read.\n\n## Fix\n\nWrite docs/helix/02-design/solution-designs/SD-015-resolution-path-trace.md (or similar location; coordinate with SD-015's author). The document traces one concrete invocation end-to-end:\n\n    ddx work --once --local --provider vidar-omlx --model-ref qwen/qwen3.6\n\nWalk through every layer, with file:line citations:\n\n1. **CLI parse** — cli/cmd/agent_cmd.go flag extraction (line references).\n\n2. **RouteFlags -\u003e RouteRequest normalization** — routing.go:NormalizeRouteRequest. What each CLI flag maps to; which flags are discarded when they don't match an expected shape; the current gap around --provider.\n\n3. **Discovery probe** — discovery.go:DiscoverProviderModels. When it fires; what it caches; the 30s TTL; how failures are handled.\n\n4. **Candidate planning** — routing.go:BuildCandidatePlans / evaluateCandidate. For each registered harness, the state map lookup and the viability checks.\n\n5. **Fuzzy matching** — discovery.go:FuzzyMatchModel. Pool selection (global vs per-provider), normalization (current gaps), prefix + tiebreak algorithm.\n\n6. **Scoring** — routing.go:scoreCandidate. Per-profile scoring, cost class, historical success rate, provider-affinity bonus (when implemented).\n\n7. **Candidate ranking** — routing.go:RankCandidates / SelectBestCandidate. Non-viable rejection, stable tiebreak.\n\n8. **Dispatch** — runner.go:Run. Harness-specific branches (virtual, agent, script, HTTP-provider-embedded, default exec). For embedded: agent_runner.go:RunAgent -\u003e resolveEmbeddedAgentProvider -\u003e agent library call.\n\n9. **Wire-level request** — ddx-agent builds the HTTP body (system_prompt + messages + tools), sends to the provider. What the server receives.\n\n10. **Response parsing** — streaming or non-streaming decode; where 'unexpected end of JSON input' (ddx-6a5dfe35) surfaces.\n\nInclude a SEQUENCE DIAGRAM (mermaid or ASCII) showing the layers.\n\nInclude a GAP TABLE at the top listing every currently-missing piece (as of whatever the SD-015 implementation state is) with links to the specific beads tracking each gap.\n\nKeep the document MAINTAINED alongside the code: update it when RouteRequest gains a field, when a new routing mode lands, when fuzzy match gains a normalization step.\n\n## Files likely touched\n\n- docs/helix/02-design/solution-designs/SD-015-resolution-path-trace.md (new)\n- docs/helix/01-frame/features/FEAT-006-agent-service.md — add a single-line pointer at the top of the 'Overview' section\n- skills/ddx/reference/agents.md — add a one-line pointer in 'Harness / profile / persona dispatch' section so agents consulting the skill can follow up to the full trace when needed\n\n## Out of scope\n\n- Rewriting SD-015 or SD-023 — this doc complements them; they describe intent, this describes current reality.\n- Documenting future routing modes that haven't landed.",
+    "acceptance": "1. docs/helix/02-design/solution-designs/SD-015-resolution-path-trace.md exists and documents all 10 layers above with file:line citations current as of the document's creation date.\n\n2. The document contains a sequence diagram showing CLI -\u003e NormalizeRouteRequest -\u003e Discovery -\u003e BuildCandidatePlans -\u003e SelectBestCandidate -\u003e Run -\u003e RunAgent -\u003e agent library -\u003e HTTP.\n\n3. The document contains a 'Current Gaps' table with at least the 4 confirmed gaps from epic ddx-2d974641 and links to the tracking beads.\n\n4. FEAT-006 and skills/ddx/reference/agents.md each have a one-line pointer to the new doc.\n\n5. **CI grep verifies citations resolve** (per opus fresh-eyes review). A CI step (or lefthook hook) parses every file:line citation in SD-015-resolution-path-trace.md and verifies the named symbol/line pattern exists in the current source tree. Prevents the doc from rotting silently as routing.go / discovery.go / types.go evolve. Failing citations block the PR.",
+    "parent": "ddx-2d974641",
+    "labels": [
+      "ddx",
+      "phase:build",
+      "kind:documentation",
+      "area:agent",
+      "area:routing"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-18T06:37:50Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1833233",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"Qwen3.6-35B-A3B-4bit\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-18T03:03:27.937852549Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=Qwen3.6-35B-A3B-4bit"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"agent\",\"resolved_model\":\"Qwen3.6-35B-A3B-4bit\",\"route_reason\":\"direct-override\",\"fallback_chain\":[],\"base_url\":\"http://vidar:1235/v1\"}",
+          "created_at": "2026-04-18T04:13:02.249837361Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=agent model=Qwen3.6-35B-A3B-4bit reason=direct-override"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-18T06:37:50.691168994Z",
+      "spec-id": "SD-015"
+    }
+  },
+  "governing": [
+    {
+      "id": "SD-015",
+      "path": "docs/helix/02-design/solution-designs/SD-015-agent-routing-and-catalog-resolution.md",
+      "title": "Solution Design: Agent Routing and Catalog Resolution"
+    }
+  ],
+  "paths": {
+    "dir": ".ddx/executions/20260418T063750-16fb3982",
+    "prompt": ".ddx/executions/20260418T063750-16fb3982/prompt.md",
+    "manifest": ".ddx/executions/20260418T063750-16fb3982/manifest.json",
+    "result": ".ddx/executions/20260418T063750-16fb3982/result.json",
+    "checks": ".ddx/executions/20260418T063750-16fb3982/checks.json",
+    "usage": ".ddx/executions/20260418T063750-16fb3982/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-e3493010-20260418T063750-16fb3982"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260418T063750-16fb3982/result.json b/.ddx/executions/20260418T063750-16fb3982/result.json
new file mode 100644
index 00000000..b2000823
--- /dev/null
+++ b/.ddx/executions/20260418T063750-16fb3982/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-e3493010",
+  "attempt_id": "20260418T063750-16fb3982",
+  "base_rev": "654b4cdcd9200fa8db6c38d6af9dad2acfbaa779",
+  "result_rev": "c029270b4c90fcd109d4f0b70c5511746592e457",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-ef8c1c05",
+  "duration_ms": 600151,
+  "tokens": 28889,
+  "cost_usd": 4.733747350000003,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260418T063750-16fb3982",
+  "prompt_file": ".ddx/executions/20260418T063750-16fb3982/prompt.md",
+  "manifest_file": ".ddx/executions/20260418T063750-16fb3982/manifest.json",
+  "result_file": ".ddx/executions/20260418T063750-16fb3982/result.json",
+  "usage_file": ".ddx/executions/20260418T063750-16fb3982/usage.json",
+  "started_at": "2026-04-18T06:37:51.048972843Z",
+  "finished_at": "2026-04-18T06:47:51.200941975Z"
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
## Review: ddx-e3493010 iter 1

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
