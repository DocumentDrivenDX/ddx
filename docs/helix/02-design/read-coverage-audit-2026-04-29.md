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

## Scope

Enumerate every CLI-visible read surface → map to HTTP REST and MCP tool coverage → identify gaps. Sequenced by user-visibility impact.

---

## Coverage Matrix

### Bead (Work Item) Reads

| CLI Command | HTTP Route | MCP Tool | Gap? |
|---|---|---|---|
| `ddx bead list` | `GET /api/beads` | `ddx_list_beads` | — |
| `ddx bead show <id>` | `GET /api/beads/{id}` | `ddx_show_bead` | — |
| `ddx bead ready` | `GET /api/beads/ready` | `ddx_bead_ready` | — |
| `ddx bead status` | `GET /api/beads/status` | `ddx_bead_status` | — |
| `ddx bead blocked` | `GET /api/beads/blocked` | **MISSING** | **G3** |
| `ddx bead dep tree <id>` | `GET /api/beads/dep/tree/{id}` | **MISSING** | **G3** |
| `ddx bead evidence list <id>` | **MISSING** | **MISSING** | **G12** |
| `ddx bead cooldown show <id>` | **MISSING** | **MISSING** | **G12** |
| `ddx bead routing` | **MISSING** | **MISSING** | **G12** |

### Document Graph Reads

| CLI Command | HTTP Route | MCP Tool | Gap? |
|---|---|---|---|
| `ddx doc graph` | `GET /api/docs/graph` | `ddx_doc_graph` | — |
| `ddx doc stale` | `GET /api/docs/stale` | `ddx_doc_stale` | — |
| `ddx doc show <id>` | `GET /api/docs/{id}` | `ddx_doc_show` | — |
| `ddx doc deps <id>` | `GET /api/docs/{id}/deps` | `ddx_doc_deps` | — |
| `ddx doc dependents <id>` | `GET /api/docs/{id}/dependents` | **MISSING** | **G1** |
| `ddx doc history <id>` | `GET /api/docs/{id}/history` | `ddx_doc_history` | — |
| `ddx doc diff <id>` | `GET /api/docs/{id}/diff` | `ddx_doc_diff` | — |
| `ddx doc changed` | **MISSING** | `ddx_doc_changed` | **G10** |
| `ddx doc audit` | (GraphQL `docGraphIssues`) | **MISSING** | minor |

### Agent Service Reads

| CLI Command | HTTP Route | MCP Tool | Gap? |
|---|---|---|---|
| `ddx agent providers` | `GET /api/providers` | `ddx_provider_list` | — |
| `ddx agent route-status` | `GET /api/providers/{harness}` | `ddx_provider_show` | — |
| `ddx agent sessions` / `log` | `GET /api/agent/sessions` | `ddx_agent_sessions` | — |
| `ddx agent models` | **MISSING** | **MISSING** | **G5** |
| `ddx agent catalog show` | **MISSING** | **MISSING** | **G5** |
| `ddx agent capabilities` | **MISSING** | **MISSING** | **G5** |
| `ddx agent usage` | **MISSING** | **MISSING** | **G5** |
| `ddx agent workers` | `GET /api/agent/workers` | **MISSING** | **G4** |
| `ddx server workers list` | `GET /api/agent/workers` | **MISSING** | **G4** |
| `ddx server workers show <id>` | `GET /api/agent/workers/{id}` | **MISSING** | **G4** |
| `ddx server workers log <id>` | `GET /api/agent/workers/{id}/log` | **MISSING** | **G4** |
| `ddx agent metrics tier-success` | **MISSING** | **MISSING** | **G7** |
| `ddx agent metrics review-outcomes` | **MISSING** | **MISSING** | **G7** |
| `ddx agent metrics cost-efficiency` | **MISSING** | **MISSING** | **G7** |
| `ddx agent executions` | `GET /api/exec/runs` | `ddx_exec_history` | partial |

### Execution / Run Reads

| CLI Command | HTTP Route | MCP Tool | Gap? |
|---|---|---|---|
| `ddx exec list` | `GET /api/exec/definitions` | `ddx_exec_definitions` | — |
| `ddx exec show <id>` | `GET /api/exec/definitions/{id}` | `ddx_exec_show` | — |
| `ddx exec history` | `GET /api/exec/runs` | `ddx_exec_history` | — |
| `ddx exec result <id>` | `GET /api/exec/runs/{id}` | **MISSING** | **G2** |
| `ddx exec log <id>` | `GET /api/exec/runs/{id}/log` | **MISSING** | **G2** |
| (planned: `ddx runs`, `ddx tries`) | **MISSING** (unified substrate) | **MISSING** | **G8** |

### Persona Reads

| CLI Command | HTTP Route | MCP Tool | Gap? |
|---|---|---|---|
| `ddx persona list` | `GET /api/personas` | **MISSING** | **G6** |
| `ddx persona show <role>` | `GET /api/personas/{role}` | `ddx_resolve_persona` | — |

### Process Metrics Reads

| CLI Command | HTTP Route | MCP Tool | Gap? |
|---|---|---|---|
| `ddx metrics summary` | `GET /api/metrics/summary` | **MISSING** | **G7** |
| `ddx metrics cost` | `GET /api/metrics/cost` | **MISSING** | **G7** |
| `ddx metrics cycle-time` | `GET /api/metrics/cycle-time` | **MISSING** | **G7** |
| `ddx metrics rework` | `GET /api/metrics/rework` | **MISSING** | **G7** |
| `ddx metric history <id>` | **MISSING** | **MISSING** | **G11** |
| `ddx metric trend <id>` | **MISSING** | **MISSING** | **G11** |

### Plugin / Registry Reads

| CLI Command | HTTP Route | MCP Tool | Gap? |
|---|---|---|---|
| `ddx mcp list` | **MISSING** | **MISSING** | **G9** |
| `ddx install installed` | **MISSING** | **MISSING** | **G9** |
| `ddx install outdated` | **MISSING** | **MISSING** | **G9** |
| `ddx install search` | **MISSING** | **MISSING** | **G9** |

### Other Reads (fully covered or out of scope)

| CLI Command | Coverage | Note |
|---|---|---|
| `ddx bead list/show/ready/status` | HTTP + MCP | full |
| `ddx doc graph/stale/show/deps/history/diff` | HTTP + MCP | full |
| `ddx persona show <role>` | HTTP + MCP | full |
| `ddx agent providers/route-status` | HTTP + MCP | full |
| `ddx agent sessions` | HTTP + MCP | full |
| `ddx exec list/show/history` | HTTP + MCP | full |
| `ddx metrics summary/cost/cycle-time/rework` | HTTP only | MCP gap → G7 |
| `ddx checkpoint` | local only | git-level; out of scope |
| `ddx skills check` | local only | validation; out of scope |
| `ddx doctor` | local only | health check; out of scope |
| `ddx status` / `ddx version` | local only | binary introspection; out of scope |

---

## Gap Catalog (sequenced by user-visibility impact)

### P1 — Core MCP parity: agent workflow blockers

**G1 · `ddx_doc_dependents` MCP tool** (FEAT-007)
- HTTP covers this: `GET /api/docs/{id}/dependents` exists
- MCP has `ddx_doc_deps` (upstream direction) but NOT the reverse (`dependents`)
- Impact: Agents doing cascade staleness analysis or "what breaks if I change X" can't perform reverse-dependency lookups via MCP. This is the most common agent reasoning pattern for document graphs.

**G2 · MCP exec run detail + log tools**
- HTTP covers: `GET /api/exec/runs/{id}` (result), `GET /api/exec/runs/{id}/log`
- MCP has `ddx_exec_history` (list runs) but no `ddx_exec_run` (single run) or `ddx_exec_run_log`
- Impact: Agents dispatch runs and then can't check results or logs via MCP without switching to HTTP. Breaks the MCP-only agent workflow feedback loop.

**G3 · `ddx_bead_blocked` + `ddx_bead_dep_tree` MCP tools**
- HTTP covers both: `GET /api/beads/blocked`, `GET /api/beads/dep/tree/{id}`
- MCP missing both
- Impact: Agents managing the bead queue via MCP can't check blocked state or inspect dependency trees. Required for agents doing queue drain planning or dependency-aware scheduling.

**G4 · MCP worker status tools**
- HTTP covers: `GET /api/agent/workers`, `GET /api/agent/workers/{id}`, `GET /api/agent/workers/{id}/log`
- MCP has no worker tools at all
- Impact: Agents coordinating or monitoring parallel workers can't query worker state. Affects multi-agent orchestration patterns.

### P2 — Configuration transparency

**G5 · Agent models / catalog / capabilities HTTP + MCP**
- CLI: `ddx agent models`, `ddx agent catalog show`, `ddx agent capabilities`, `ddx agent usage`
- HTTP: No route for models, catalog, or capabilities (providers/routing is covered)
- MCP: No tools
- Impact: High — there's no server-side way to discover available models, tier assignments, or capability metadata. Automated model selection and cost tiering depend on this. The endpoint-first routing redesign (memory: project_endpoint_routing_design) needs these surfaces.

**G6 · `ddx_list_personas` MCP tool**
- HTTP: `GET /api/personas` covers list
- MCP: `ddx_resolve_persona` resolves by role only; no list tool
- Impact: Agents selecting a persona for a task can't enumerate available options via MCP; must know the role name in advance.

### P3 — Process intelligence

**G7 · MCP process metrics tools**
- HTTP: `/api/metrics/{summary,cost,cycle-time,rework}` all exist
- MCP: No metrics tools at all
- Impact: Cost-aware agents (per cost-tiered-work standing goal) can't query process metrics via MCP without switching to HTTP. Affects FEAT-016 read parity.

**G8 · Layer-aware run-history HTTP + MCP** (FEAT-021 unified substrate)
- No routes exist for the three-layer substrate (`work` → `try` → `run` drill-down)
- Required for FEAT-021 layer-aware dashboard and for agents traversing the execution hierarchy
- Note: this gap will be partly addressed by the three-layer run substrate bead (plan #18); listed here to sequence the server-side read exposure.

**G9 · MCP server registry + plugin manifest HTTP + MCP**
- CLI: `ddx mcp list`, `ddx install installed/outdated/search`
- HTTP + MCP: No routes or tools
- Impact: Agents or UIs can't discover which MCP servers are configured or which plugins are installed without spawning the CLI. Needed for plugin-aware workflows.

### P4 — Completeness

**G10 · `GET /api/docs/changed` REST route**
- MCP has `ddx_doc_changed`; HTTP is missing this route
- Impact: Low — REST clients (not MCP) can't query changed artifacts. Easy add alongside FEAT-007 work.

**G11 · Per-metric-id history/trend HTTP + MCP**
- CLI: `ddx metric history <id>`, `ddx metric trend <id>`
- HTTP: Only aggregate endpoints; no per-metric-id time-series
- MCP: None
- Impact: Low-medium — time-series drill-down is used for trend analysis but not on the critical agent path.

**G12 · Bead evidence + cooldown + routing HTTP + MCP**
- CLI: `ddx bead evidence list <id>`, `ddx bead cooldown show <id>`, `ddx bead routing`
- HTTP + MCP: No coverage
- Impact: Low — operational details primarily used by the execute-loop itself; not on the agent-facing critical path.

---

## Per-Gap Bead List (proposed, sequenced by impact)

| Priority | Gap | Proposed Bead Title | FEAT ref |
|---|---|---|---|
| P1.1 | G1 | `[read-coverage] add ddx_doc_dependents MCP tool` | FEAT-007 |
| P1.2 | G2 | `[read-coverage] add ddx_exec_run + ddx_exec_run_log MCP tools` | FEAT-002/010 |
| P1.3 | G3 | `[read-coverage] add ddx_bead_blocked + ddx_bead_dep_tree MCP tools` | FEAT-004/002 |
| P1.4 | G4 | `[read-coverage] add MCP worker status tools (list/show/log)` | FEAT-002/013 |
| P2.1 | G5 | `[read-coverage] add agent models/catalog/capabilities HTTP + MCP` | FEAT-006 |
| P2.2 | G6 | `[read-coverage] add ddx_list_personas MCP tool` | FEAT-007/002 |
| P3.1 | G7 | `[read-coverage] add MCP process metrics tools` | FEAT-016 |
| P3.2 | G8 | `[read-coverage] add layer-aware run-history HTTP + MCP routes` | FEAT-021/010 |
| P3.3 | G9 | `[read-coverage] add MCP server registry + plugin manifest HTTP + MCP` | FEAT-009/015 |
| P4.1 | G10 | `[read-coverage] add GET /api/docs/changed REST route` | FEAT-007 |
| P4.2 | G11 | `[read-coverage] add per-metric-id history/trend HTTP + MCP` | FEAT-016 |
| P4.3 | G12 | `[read-coverage] add bead evidence + cooldown + routing HTTP + MCP` | FEAT-004 |
