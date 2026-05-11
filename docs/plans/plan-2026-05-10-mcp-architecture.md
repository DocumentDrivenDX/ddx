---
ddx:
  id: plan-2026-05-10-mcp-architecture
---
# MCP Server Architecture: DDx-server-MCP into Databricks Assistant

Date: 2026-05-10
Status: Multi-model reviewed (opus); verdict reached. MCP surface is much more developed than expected.

## Question being answered

> Have we defined what interfaces the MCP servers will expose, what workflows they will support? Think of it from the perspective of a Databricks App exposing MCP servers into the Databricks Assistant that allow graph operations on documents, bead updates, etc. Should some or all of these functions be pushed down to Axon instead of living in DDx?

## Verdict (opus review): **(a) DDx-server-MCP only.** The Databricks Assistant talks to ddx-server's MCP. Axon stays a backend reached over GraphQL.

Load-bearing reason: the value proposition the user described — *"first-class knowledge of your team's specs, beads, prompts, doc graph"* — is **document- and workflow-shaped, not entity-shaped**. Doc graph, personas, library, ready/blocked computation, and claim semantics all live in DDx and have no Axon analog.

## What surprised me: there are TWO unrelated MCP surfaces

1. **Server-side MCP endpoint (real, substantial, already built).** `ddx-server` exposes JSON-RPC 2.0 / Streamable-HTTP MCP at `POST /mcp` (`cli/internal/server/server.go:953`, handler at `:3139`). Protocol version `2025-03-26`. **51 tools registered** (`server.go:3181-3805`) spanning documents, beads (read+write+claim+create+update), doc-graph, personas, agents, workers, providers, executions, metrics, projects. Output bodies are clamped via `evidence.ClampOutput` per FEAT-022 §10. Multi-project routing via the `project` arg with singleton-compat fallback.
2. **Library MCP-server catalog (stub).** `library/mcp-servers/` is a YAML catalog of **third-party** MCP servers (filesystem, github, postgres, slack, notion). Browsable via `ddx list`. **There is no `ddx mcp install` command** — `cli/cmd/list.go:288` references it as an example string in help text but no Cobra command exists.

The first is what matters for the Databricks Assistant story. The second is a separate gap (orphan help-text reference).

## What works for the Databricks-Assistant use case (already shipped)

- **Doc graph** — `ddx_doc_graph`, `ddx_doc_deps`, `ddx_doc_dependents`, `ddx_doc_stale`, `ddx_doc_show` (`server.go:3360-3413`). "Specs touching auth area" works with a path/label filter on `ddx_doc_graph`.
- **Bead operations** — `ddx_bead_create`, `ddx_list_beads` (filter by label), `ddx_bead_update` (mark blocked / closed), `ddx_bead_claim` (`server.go:3469-3522`).
- **Library lookup** — `ddx_list_documents`, `ddx_read_document`, `ddx_search`, `ddx_list_personas`, `ddx_resolve_persona`, `ddx_list_mcp_servers`, `ddx_list_plugins` (`:3192-3247, :3787-3804`).

## Why Axon-MCP wouldn't replace this

Axon's MCP would expose raw `ddx_beads` / `ddx_bead_events` collection CRUD. What it cannot serve:

- **Doc graph** — markdown files in the repo; ddx-server indexes them. Axon never sees them. Body-link `[[ID]]` parsing is DDx code.
- **Persona resolution / library / plugin manifest** — none of this lives in Axon.
- **Derived bead computations** — `bead_ready` / `bead_blocked` / `bead_dependency_waiting` / `bead_cooldown` / `bead_evidence` are DDx-side per TD-030 v1; Axon's `isReady` lands later.
- **Agent / worker / exec / metrics tools** — read in-memory worker registry, `.ddx/workers/`, `.ddx/executions/` — not Axon.
- **Workflow-aware writes** — `ddx_bead_claim` is a state-machine transition with append-only event semantics DDx owns. Axon's `bead.rs` is informational only per TD-030.
- **Routing / auth pass-through** — ts-net auth + project scoping resolved in ddx-server.

The hybrid model (Axon-MCP + DDx-MCP both exposed) is technically possible but **bad UX for an Assistant**: tool-selection from natural language frequently picks the wrong one, and auth surface doubles. One MCP per Assistant.

## Gaps to ship a Databricks-App MCP deployment

1. **Auth model** — today `handleMCP` accepts any `POST /mcp` from a ts-net or localhost peer with no per-user identity. Databricks needs OBO (on-behalf-of) header propagation so `ddx_bead_create` attributes to the asking user. Free-text `assignee`/`reporter` args at `server.go:3501,3517` need to honor identity.
2. **Deployment surface** — no Databricks App packaging exists (no app config, no entry script, no Postgres-via-Axon connection sample). ts-net (ADR-006) is the documented auth path; Databricks Apps will need a non-tsnet listener mode behind the App's auth proxy.
3. **Axon connectivity** — TD-030 is "draft" status; the GraphQL client and `bead_tracker.backend=axon` switch are not yet implemented end-to-end (per axon backend audit, `plan-2026-05-10-axon-only-architecture.md`). Without this, `ddx-server` can't read beads from Postgres-backed Axon at all.
4. **Missing MCP tools**:
   - `ddx_doc_sidecar`, `ddx_doc_generated_by` (FEAT-007:177-180 spec'd, not in `mcpTools()`).
   - `ddx_bead_close` convenience (currently must be `ddx_bead_update status=closed`).
   - Label/path filter on `ddx_doc_graph` for "specs touching X area" queries.
5. **`ddx mcp install` CLI** — orphan help-text reference at `cli/cmd/list.go:288`; no Cobra command. Orthogonal to Databricks deployment but a documented gap.
6. **Long-running job model** — `ddx_agent_dispatch` and `ddx_exec_dispatch` exist but are localhost-only (FEAT-002 #42). For Databricks, decide explicitly: leave dispatch out of the App's MCP (Assistant can read but not run), or design a Databricks-App-safe dispatch path (probably via Databricks Jobs).
7. **Worker SSE alternative** — MCP polls `ddx_worker_show` per FEAT-002:211-213. For a notebook UX, document polling cadence or add a `ddx_worker_progress_since(seq)` cursor tool.

## Implications for the broader architecture

- ddx-server's MCP is the right Databricks-Assistant surface; this confirms the GraphQL-collapse rejection in `plan-2026-05-10-axon-only-architecture.md`.
- The bigger blocker for a Databricks deployment isn't MCP design — it's auth (gap 1), Axon production-readiness (gap 3), and packaging (gap 2).
- The MCP surface itself is mostly there; the work is wrapping it for the Databricks deployment context, not designing a new tool surface from scratch.

## Open questions

- For gap 6 (long-running dispatch): is "Assistant can read but not dispatch work" acceptable for v1, with dispatch landing later via Databricks Jobs? Or is dispatch table-stakes for the value prop?
- For gap 1 (auth): does ddx-server need its own user model, or can it lean entirely on Databricks workspace OAuth + the App's auth proxy?
- Do we file gaps 1, 2, 3 as a tracked epic now, or wait until Databricks deployment is a committed direction?
