<bead-review>
  <bead id="ddx-3610f1fc" iter=1>
    <title>Remove provider-native parsers and GlobalProviderHealth shadow (atomic)</title>
    <description>
Land agent-b896b406 (provider-native parser removal) + agent-bcd2ff47 (GlobalProviderHealth removal) together. The atomic-pair framing is from the parent bead ddx-7bc0c8d5: splitting creates a window where operator surfaces (doctor, /api/providers, MCP tools, agent usage) read from different sources than execute-loop, so both land in one PR.

## Files to delete / rewrite

- cli/internal/agent/session_evidence.go (357 LOC): Codex native session JSONL, Claude stats/cache, OpenRouter /auth/key, LM Studio /v1/models.
- cli/internal/agent/routing_signals.go (~450 LOC Runner-method variant; loadCodex/Claude/LMStudio helpers).
- cli/internal/agent/routing_signal_adapters.go, routing_signal_http.go.
- cli/internal/agent/claude_quota_cache.go — entire ReadClaudeQuotaSnapshot / WriteClaudeQuotaSnapshot / resolveClaudeQuotaSnapshotPath subsystem.
- cli/internal/agent/state.go — ReadClaudeQuotaRoutingDecision and ClaudeQuotaDecision type.
- cli/internal/escalation/GlobalProviderHealth singleton + ProviderHealthTracker.

## Call sites to migrate

- workers.go:511 (IsHealthy) / :593 (Mark)
- agent_cmd.go:1702 (IsHealthy) / :1791 (Mark)
- agent_cmd.go:680 (routing_signal.CurrentQuota.State == "blocked")
- agent_cmd.go:704 (ReadClaudeQuotaRoutingDecision)

## Migration target

- svc.ListHarnesses — HarnessInfo.Quota / UsageWindows / Account
- svc.ListProviders — ProviderInfo.EndpointStatus
- svc.RouteStatus — RouteStatusReport with RouteCandidateStatus.Healthy
- svc.RecordRouteAttempt — RouteAttempt with status/reason

Vocabulary translation shim in buildProviderSummary / buildProviderDetail maps current "fresh/stale" to upstream's "ok/stale/unavailable" (consumer-side mapping only; not an upstream API change).

## Observable behavior changes (consumer surfaces)

- ddx agent doctor --routing --json: state.claude_quota_decision field disappears.
- ddx agent doctor --routing human text: claude-quota-cache line disappears.
- ProbeHarnessState via state.go.
- ddx agent usage enrichment.
- REST /api/providers.
- MCP ddx_provider_list / ddx_provider_show.

Not affected: SvelteKit frontend providers page uses GraphQL providerStatuses only (no cache fields).
    </description>
    <acceptance>
(1) session_evidence.go, routing_signals.go, routing_signal_adapters.go, routing_signal_http.go, claude_quota_cache.go deleted.
(2) state.go: ReadClaudeQuotaRoutingDecision and ClaudeQuotaDecision removed.
(3) cli/internal/escalation: GlobalProviderHealth singleton and ProviderHealthTracker removed.
(4) workers.go, agent_cmd.go call sites migrated to svc.ListHarnesses / svc.ListProviders / svc.RouteStatus / svc.RecordRouteAttempt.
(5) buildProviderSummary / buildProviderDetail perform fresh/stale → ok/stale/unavailable translation.
(6) go -C cli build ./... and go -C cli test ./... green.
(7) ddx agent doctor --routing and /api/providers exercised manually; behavior matches upstream RouteStatusReport semantics.
    </acceptance>
    <labels>area:agent, area:routing, kind:refactor, phase:build, goal:thin-consumer, workstream:agent-upgrade</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="a16d481bdc08c7f5d60329abd5d5220fb2132deb">
commit a16d481bdc08c7f5d60329abd5d5220fb2132deb
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Mon Apr 20 21:50:38 2026 -0400

    chore: add execution evidence [20260421T013555-]

diff --git a/.ddx/executions/20260421T013555-1be775f2/manifest.json b/.ddx/executions/20260421T013555-1be775f2/manifest.json
new file mode 100644
index 00000000..dd6d7ad5
--- /dev/null
+++ b/.ddx/executions/20260421T013555-1be775f2/manifest.json
@@ -0,0 +1,58 @@
+{
+  "attempt_id": "20260421T013555-1be775f2",
+  "bead_id": "ddx-3610f1fc",
+  "base_rev": "8bd141fbbc0c3d1f3b62399ad3e215aefbd8ba3c",
+  "created_at": "2026-04-21T01:35:55.947067677Z",
+  "requested": {
+    "harness": "codex",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-3610f1fc",
+    "title": "Remove provider-native parsers and GlobalProviderHealth shadow (atomic)",
+    "description": "Land agent-b896b406 (provider-native parser removal) + agent-bcd2ff47 (GlobalProviderHealth removal) together. The atomic-pair framing is from the parent bead ddx-7bc0c8d5: splitting creates a window where operator surfaces (doctor, /api/providers, MCP tools, agent usage) read from different sources than execute-loop, so both land in one PR.\n\n## Files to delete / rewrite\n\n- cli/internal/agent/session_evidence.go (357 LOC): Codex native session JSONL, Claude stats/cache, OpenRouter /auth/key, LM Studio /v1/models.\n- cli/internal/agent/routing_signals.go (~450 LOC Runner-method variant; loadCodex/Claude/LMStudio helpers).\n- cli/internal/agent/routing_signal_adapters.go, routing_signal_http.go.\n- cli/internal/agent/claude_quota_cache.go — entire ReadClaudeQuotaSnapshot / WriteClaudeQuotaSnapshot / resolveClaudeQuotaSnapshotPath subsystem.\n- cli/internal/agent/state.go — ReadClaudeQuotaRoutingDecision and ClaudeQuotaDecision type.\n- cli/internal/escalation/GlobalProviderHealth singleton + ProviderHealthTracker.\n\n## Call sites to migrate\n\n- workers.go:511 (IsHealthy) / :593 (Mark)\n- agent_cmd.go:1702 (IsHealthy) / :1791 (Mark)\n- agent_cmd.go:680 (routing_signal.CurrentQuota.State == \"blocked\")\n- agent_cmd.go:704 (ReadClaudeQuotaRoutingDecision)\n\n## Migration target\n\n- svc.ListHarnesses — HarnessInfo.Quota / UsageWindows / Account\n- svc.ListProviders — ProviderInfo.EndpointStatus\n- svc.RouteStatus — RouteStatusReport with RouteCandidateStatus.Healthy\n- svc.RecordRouteAttempt — RouteAttempt with status/reason\n\nVocabulary translation shim in buildProviderSummary / buildProviderDetail maps current \"fresh/stale\" to upstream's \"ok/stale/unavailable\" (consumer-side mapping only; not an upstream API change).\n\n## Observable behavior changes (consumer surfaces)\n\n- ddx agent doctor --routing --json: state.claude_quota_decision field disappears.\n- ddx agent doctor --routing human text: claude-quota-cache line disappears.\n- ProbeHarnessState via state.go.\n- ddx agent usage enrichment.\n- REST /api/providers.\n- MCP ddx_provider_list / ddx_provider_show.\n\nNot affected: SvelteKit frontend providers page uses GraphQL providerStatuses only (no cache fields).",
+    "acceptance": "(1) session_evidence.go, routing_signals.go, routing_signal_adapters.go, routing_signal_http.go, claude_quota_cache.go deleted.\n(2) state.go: ReadClaudeQuotaRoutingDecision and ClaudeQuotaDecision removed.\n(3) cli/internal/escalation: GlobalProviderHealth singleton and ProviderHealthTracker removed.\n(4) workers.go, agent_cmd.go call sites migrated to svc.ListHarnesses / svc.ListProviders / svc.RouteStatus / svc.RecordRouteAttempt.\n(5) buildProviderSummary / buildProviderDetail perform fresh/stale → ok/stale/unavailable translation.\n(6) go -C cli build ./... and go -C cli test ./... green.\n(7) ddx agent doctor --routing and /api/providers exercised manually; behavior matches upstream RouteStatusReport semantics.",
+    "parent": "ddx-7bc0c8d5",
+    "labels": [
+      "area:agent",
+      "area:routing",
+      "kind:refactor",
+      "phase:build",
+      "goal:thin-consumer",
+      "workstream:agent-upgrade"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-21T01:35:55Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1063103",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-21T01:19:48.151986474Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "no_changes\nresult_rev=e94b2ce107ac5bd773b631e0c7e42e5f91c43e30\nbase_rev=e94b2ce107ac5bd773b631e0c7e42e5f91c43e30\nretry_after=2026-04-21T07:19:48Z",
+          "created_at": "2026-04-21T01:19:48.484345502Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-21T01:35:55.493075625Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260421T013555-1be775f2",
+    "prompt": ".ddx/executions/20260421T013555-1be775f2/prompt.md",
+    "manifest": ".ddx/executions/20260421T013555-1be775f2/manifest.json",
+    "result": ".ddx/executions/20260421T013555-1be775f2/result.json",
+    "checks": ".ddx/executions/20260421T013555-1be775f2/checks.json",
+    "usage": ".ddx/executions/20260421T013555-1be775f2/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-3610f1fc-20260421T013555-1be775f2"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260421T013555-1be775f2/result.json b/.ddx/executions/20260421T013555-1be775f2/result.json
new file mode 100644
index 00000000..2da77619
--- /dev/null
+++ b/.ddx/executions/20260421T013555-1be775f2/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-3610f1fc",
+  "attempt_id": "20260421T013555-1be775f2",
+  "base_rev": "8bd141fbbc0c3d1f3b62399ad3e215aefbd8ba3c",
+  "result_rev": "884b60773c262614252798ca49a82483eff61471",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-414c8ef8",
+  "duration_ms": 881479,
+  "tokens": 5180260,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260421T013555-1be775f2",
+  "prompt_file": ".ddx/executions/20260421T013555-1be775f2/prompt.md",
+  "manifest_file": ".ddx/executions/20260421T013555-1be775f2/manifest.json",
+  "result_file": ".ddx/executions/20260421T013555-1be775f2/result.json",
+  "usage_file": ".ddx/executions/20260421T013555-1be775f2/usage.json",
+  "started_at": "2026-04-21T01:35:55.947370094Z",
+  "finished_at": "2026-04-21T01:50:37.427204161Z"
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
## Review: ddx-3610f1fc iter 1

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
