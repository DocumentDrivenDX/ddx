---
ddx:
  id: read-coverage-audit-2026-04-29
  depends_on:
    - plan-2026-04-29-artifact-and-run-architecture
---

# Read-Coverage Audit — HTTP/MCP Gap Analysis

**Date:** 2026-04-29
**Bead:** ddx-44236615 (artifact-run-arch read-coverage audit)
**Plan:** plan-2026-04-29-artifact-and-run-architecture.md § Bead #20
**Reconciliation:** spot-checked against the current CLI, HTTP, MCP, and GraphQL surfaces on 2026-06-07. The historical G1-G12 backlog is now closed or superseded; no new read-surface bead is required from this audit.

## Scope

Enumerate every CLI-visible read surface → map to HTTP REST and MCP tool coverage → identify gaps. Sequenced by user-visibility impact.

---

## Coverage Matrix

### Bead (Work Item) Reads

| CLI Command | HTTP Route | MCP Tool | Status |
|---|---|---|---|
| `ddx bead list` | `GET /api/beads` | `ddx_list_beads` | implemented |
| `ddx bead show <id>` | `GET /api/beads/{id}` | `ddx_show_bead` | implemented |
| `ddx bead ready` | `GET /api/beads/ready` | `ddx_bead_ready` | implemented |
| `ddx bead status` | `GET /api/beads/status` | `ddx_bead_status` | implemented |
| `ddx bead blocked` | `GET /api/beads/blocked` | `ddx_bead_blocked` | implemented |
| `ddx bead dep tree <id>` | `GET /api/beads/dep/tree/{id}` | `ddx_bead_dep_tree` | implemented |
| `ddx bead evidence list <id>` | `GET /api/beads/{id}/evidence` | `ddx_bead_evidence` | implemented |
| `ddx bead cooldown show <id>` | `GET /api/beads/{id}/cooldown` | `ddx_bead_cooldown` | implemented |
| `ddx bead routing` | `GET /api/beads/{id}/routing` | `ddx_bead_routing` | implemented |

### Document Graph Reads

| CLI Command | HTTP Route | MCP Tool | Status |
|---|---|---|---|
| `ddx doc graph` | `GET /api/docs/graph` | `ddx_doc_graph` | implemented |
| `ddx doc stale` | `GET /api/docs/stale` | `ddx_doc_stale` | implemented |
| `ddx doc show <id>` | `GET /api/docs/{id}` | `ddx_doc_show` | implemented |
| `ddx doc deps <id>` | `GET /api/docs/{id}/deps` | `ddx_doc_deps` | implemented |
| `ddx doc dependents <id>` | `GET /api/docs/{id}/dependents` | `ddx_doc_dependents` | implemented |
| `ddx doc history <id>` | `GET /api/docs/{id}/history` | `ddx_doc_history` | implemented |
| `ddx doc diff <id>` | `GET /api/docs/{id}/diff` | `ddx_doc_diff` | implemented |
| `ddx doc changed` | `GET /api/docs/changed` | `ddx_doc_changed` | implemented |
| `ddx doc audit` | (GraphQL `docGraphIssues`) | n/a | GraphQL-only; out of this audit's HTTP/MCP backlog |

### Agent Service Reads

| CLI Command | HTTP Route | MCP Tool | Status |
|---|---|---|---|
| `legacy agent providers` | `GET /api/providers` | `ddx_provider_list` | implemented |
| `legacy agent route-status` | `GET /api/providers/{harness}` | `ddx_provider_show` | implemented |
| `legacy agent sessions` / `log` | `GET /api/agent/sessions` | `ddx_agent_sessions` | implemented |
| `legacy agent models` | `GET /api/agent/models` | `ddx_agent_models` | implemented |
| `legacy agent catalog show` | `GET /api/agent/models` + `GET /api/providers/{harness}` | n/a | superseded by provider model/status views |
| `legacy agent capabilities` | `GET /api/agent/capabilities` | `ddx_agent_capabilities` | implemented |
| `legacy agent usage` | FEAT-014 provider dashboards / usage summaries | n/a | superseded by FEAT-014 usage surfaces |
| `legacy agent workers` | `GET /api/agent/workers` | `ddx_worker_list` | implemented |
| `ddx server workers list` | `GET /api/agent/workers` | `ddx_worker_list` | implemented |
| `ddx server workers show <id>` | `GET /api/agent/workers/{id}` | `ddx_worker_show` | implemented |
| `ddx server workers log <id>` | `GET /api/agent/workers/{id}/log` | `ddx_worker_log` | implemented |
| `legacy agent metrics route-success` | `GET /api/providers` / provider status summary | `ddx_provider_list` / `ddx_provider_show` | implemented |
| `legacy agent metrics review-outcomes` | `GET /api/metrics/rework` | `ddx_metrics_rework` | implemented |
| `legacy agent metrics cost-efficiency` | `GET /api/metrics/cost` | `ddx_metrics_cost` | implemented |
| `legacy agent executions` | `GET /api/exec/runs` | `ddx_exec_history` | implemented |

### Execution / Run Reads

| CLI Command | HTTP Route | MCP Tool | Status |
|---|---|---|---|
| `ddx exec list` | `GET /api/exec/definitions` | `ddx_exec_definitions` | implemented |
| `ddx exec show <id>` | `GET /api/exec/definitions/{id}` | `ddx_exec_show` | implemented |
| `ddx exec history` | `GET /api/exec/runs` | `ddx_exec_history` | implemented |
| `ddx exec result <id>` | `GET /api/exec/runs/{id}` | `ddx_exec_run` | implemented |
| `ddx exec log <id>` | `GET /api/exec/runs/{id}/log` | `ddx_exec_run_log` | implemented |
| `ddx runs` / `ddx tries` (planned) | GraphQL `runs` / `run` / `runToolCalls` / `runBundleFile` | GraphQL only | superseded by the current layer-aware run substrate |

### Persona Reads

| CLI Command | HTTP Route | MCP Tool | Status |
|---|---|---|---|
| `ddx persona list` | `GET /api/personas` | `ddx_list_personas` | implemented |
| `ddx persona show <role>` | `GET /api/personas/{role}` | `ddx_resolve_persona` | implemented |

### Process Metrics Reads

| CLI Command | HTTP Route | MCP Tool | Status |
|---|---|---|---|
| `ddx metrics summary` | `GET /api/metrics/summary` | `ddx_metrics_summary` | implemented |
| `ddx metrics cost` | `GET /api/metrics/cost` | `ddx_metrics_cost` | implemented |
| `ddx metrics cycle-time` | `GET /api/metrics/cycle-time` | `ddx_metrics_cycle_time` | implemented |
| `ddx metrics rework` | `GET /api/metrics/rework` | `ddx_metrics_rework` | implemented |
| `ddx metric history <id>` | `GET /api/metrics/{id}/history` | `ddx_metric_history` | implemented |
| `ddx metric trend <id>` | `GET /api/metrics/{id}/trend` | `ddx_metric_trend` | implemented |

### Plugin / Registry Reads

| CLI Command | HTTP Route | MCP Tool | Status |
|---|---|---|---|
| `ddx mcp list` | `GET /api/mcp-servers` | `ddx_list_mcp_servers` | implemented |
| `ddx install installed` | `GET /api/plugins` | `ddx_list_plugins` | implemented |
| `ddx install outdated` | local registry state | n/a | local-only; intentionally out of scope for HTTP/MCP |
| `ddx install search` | local registry search | n/a | local-only; intentionally out of scope for HTTP/MCP |

### Other Reads (fully covered or out of scope)

| CLI Command | Coverage | Note |
|---|---|---|
| `ddx bead list/show/ready/status` | HTTP + MCP | full |
| `ddx doc graph/stale/show/deps/history/diff` | HTTP + MCP | full |
| `ddx persona show <role>` | HTTP + MCP | full |
| `legacy agent providers/route-status` | HTTP + MCP | full |
| `legacy agent sessions` | HTTP + MCP | full |
| `ddx exec list/show/history` | HTTP + MCP | full |
| `ddx metrics summary/cost/cycle-time/rework` | HTTP + MCP | full |
| `ddx checkpoint` | local only | git-level; out of scope |
| `ddx skills check` | local only | validation; out of scope |
| `ddx doctor` | local only | health check; out of scope |
| `ddx status` / `ddx version` | local only | binary introspection; out of scope |

---

## Reconciliation Notes

### P1 — Core MCP parity

**G1 · `ddx_doc_dependents` MCP tool** is implemented in HTTP and MCP.

**G2 · MCP exec run detail + log tools** are implemented in HTTP and MCP.

**G3 · `ddx_bead_blocked` + `ddx_bead_dep_tree` MCP tools** are implemented in HTTP and MCP.

**G4 · MCP worker status tools** are implemented in HTTP and MCP.

### P2 — Configuration transparency

**G5 · Agent models / catalog / capabilities HTTP + MCP** is superseded by the current provider-model and provider-status surfaces. `legacy agent models` and `legacy agent capabilities` are implemented via `/api/agent/models`, `/api/agent/capabilities`, `ddx_agent_models`, and `ddx_agent_capabilities`; the old catalog/usage names are no longer active read-surface gaps.

**G6 · `ddx_list_personas` MCP tool** is implemented.

### P3 — Process intelligence

**G7 · MCP process metrics tools** are implemented.

**G8 · Layer-aware run-history HTTP + MCP** is superseded by the current GraphQL run substrate (`runs`, `run`, `runToolCalls`, `runBundleFile`) and the FEAT-008 frontend layer-aware runs views. There is no separate REST/MCP backlog left here.

**G9 · MCP server registry + plugin manifest HTTP + MCP** is partially implemented: `ddx mcp list` and `ddx install installed` are covered by `/api/mcp-servers` and `/api/plugins` plus their MCP mirrors, while `ddx install outdated` and `ddx install search` remain local-only and are intentionally out of scope for this server audit.

### P4 — Completeness

**G10 · `GET /api/docs/changed` REST route** is implemented.

**G11 · Per-metric-id history/trend HTTP + MCP** is implemented.

**G12 · Bead evidence + cooldown + routing HTTP + MCP** is implemented.

---

## Backlog Status

No new bead was filed from this audit. The historical G1-G12 items are either implemented in the current server/CLI/MCP surface or superseded by newer GraphQL/provider-status/read-model surfaces.
