---
ddx:
  id: TP-002
  depends_on:
    - FEAT-002
    - FEAT-008
    - FEAT-020
    - FEAT-021
    - FEAT-026
    - FEAT-029
    - SD-019
    - ADR-006
    - ADR-028
---
# Test Plan: DDx Server and Web UI

**ID:** TP-002
**Features:** FEAT-002 (Server), FEAT-008 (Web UI), FEAT-020 (Node State),
FEAT-021 (Dashboard UI), FEAT-026 (Federation), FEAT-029 (Managed-Node Remote
Control), SD-019 (Host+User Multi-Project Topology)

> **Historical reference** — describes the pre-2026-04-14 React stack. This plan is superseded by current-stack test plans; retained rows below are historical only.
**Status:** Superseded historical reference

## Scope

End-to-end testing of the DDx server HTTP API, MCP tools, and embedded web
UI. Tests run against a live `ddx server` instance — a per-user host daemon
holding its state at `~/.local/share/ddx/server-state.json` — with real
project data (documents, beads, personas, execution definitions) from one or
more project roots. Coverage includes host+user isolation across registered
projects, concurrent project access, and work worker lifecycle
reported through autonomous worker probes and server-derived worker views.

The remote-access trust boundary is ADR-006: localhost and ts-net peers are
trusted for v1, and write-capable paths carry the identity envelope required
for future identity policy. Federation and managed-node coverage must verify
the envelope and provenance, not merely HTTP success.

## Test Infrastructure

| Component | Tool | Location |
|-----------|------|----------|
| Go unit tests | `go test` | `cli/internal/server/server_test.go` |
| E2E functional tests | Playwright | `cli/internal/server/frontend/e2e/app.spec.ts` |
| Visual regression | Playwright screenshots | `cli/internal/server/frontend/e2e/screenshots.spec.ts` |
| Demo recording | Playwright video | `cli/internal/server/frontend/e2e/demo-recording.spec.ts` |
| Multi-project coverage | Playwright | `cli/internal/server/frontend/e2e/projects.spec.ts` |
| Config (functional) | Playwright | `cli/internal/server/frontend/playwright.config.ts` |
| Config (demo) | Playwright | `cli/internal/server/frontend/playwright.demo.config.ts` |

### Running

```bash
cd cli/internal/server/frontend

# Install browsers (first time)
bunx playwright install chromium

# Functional e2e tests
bun run test:e2e

# Demo video recording
bun run demo:record
# Output: demo-output/
```

The Playwright configs auto-start `ddx server --port 18080` via `webServer`.
Multi-project fixtures point the server at an isolated `XDG_DATA_HOME` so
the host+user state file at `~/.local/share/ddx/server-state.json` is
scoped to the test run, and either seed `server.projects` in config or drive
`POST /api/projects/register` to populate the registry with several project
roots. That gives request routing, the UI project picker, host+user
isolation, and concurrent-project behaviors a shared fixture to exercise in
one run.

## Measurement Methodology

Artifact-listing performance is measured as a browser/UI contract, not as a
general benchmark suite. The intent is to lock the FEAT-008 gating numbers only
after the B7-C1 baseline run has been captured in `docs/helix/06-iterate/perf/`.
The baseline run publishes the locked gating numbers; until then, the FEAT-008
budgets remain provisional design targets.

| Metric | What to record | Where it lives |
|--------|----------------|----------------|
| First usable content | Time from navigation start to the artifact list becoming visible and interactive on the reference fixture | `cli/internal/server/frontend/e2e/artifacts.spec.ts` |
| Search/filter latency | Time from a category filter change or a search input change, measured separately, to the updated list settling while the list is already loaded | `cli/internal/server/frontend/e2e/artifacts.spec.ts` |
| Steady-state DOM rows | Maximum visible artifact rows in the default list state before the contract needs to be revisited | FEAT-008 contract + perf baseline report |

Method:

1. Run the browser flow against the reference fixture and record first usable
   content from navigation start to the artifact list becoming visible and
   interactive.
2. With the list already loaded, measure a category filter change and a search
   input change separately, then record the settle time for each interaction.
3. Count visible artifact rows in the default list state and treat that number
   as the steady-state DOM ceiling for the current contract.
4. Publish the measured numbers in `docs/helix/06-iterate/perf/` and keep this
   plan aligned with the same report.
5. Use the B7-C1 baseline report as the source of truth for the locked gating
   numbers. Until that baseline exists, the FEAT-008 budgets remain provisional
   design targets rather than pass/fail thresholds.

Revisit the contract before shipping if the artifact browser design adds any
of the following:

- infinite scroll
- auto-prefetch
- 500-1000 visible DOM rows in the default list state
- more than 1000 visible DOM rows in the default list state
- sticky group headers

The current implementation model assumes the artifact list remains flat and
non-virtualized. If that assumption changes, update both the FEAT-008 section
and this measurement methodology together.

## Test Cases

### TC-001: Dashboard

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-001.1 | Dashboard loads | `h1` contains "Dashboard" | Implemented |
| TC-001.2 | Document count card | Card shows numeric count > 0 | Implemented |
| TC-001.3 | Bead status card | Shows Ready, In Progress, Open, Closed counts | Implemented |
| TC-001.4 | Stale docs card | Shows numeric count | Implemented |
| TC-001.5 | Server health card | Shows status "ok" | Implemented |
| TC-001.6 | Navigate to Documents | "Browse" link navigates to `/documents` | Implemented |
| TC-001.7 | Navigate to Beads | "View board" link navigates to `/beads` | Implemented |
| TC-001.8 | Navigate to Graph | "View graph" link navigates to `/graph` | Implemented |

### TC-002: Documents Page

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-002.1 | Document list loads | Left panel shows document entries | Implemented |
| TC-002.2 | Type filter | Selecting a type filters the list | Implemented |
| TC-002.3 | Search filter | Typing in search narrows the list | Implemented |
| TC-002.4 | View document | Clicking a document shows rendered markdown in right panel | Implemented |
| TC-002.5 | Document path display | Path shown in monospace above content | Implemented |
| TC-002.6 | Edit button | "Edit" button switches to textarea with raw content | Implemented |
| TC-002.7 | Cancel edit | "Cancel" returns to rendered view without saving | Implemented |
| TC-002.8 | Empty state | "Select a document" placeholder when nothing selected | Implemented |

### TC-003: Beads Kanban Board

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-003.1 | Kanban loads | Three columns: OPEN, IN PROGRESS, CLOSED visible | Implemented |
| TC-003.2 | Bead cards render | Cards show title, ID, priority, labels | Implemented |
| TC-003.3 | Search beads | Search input filters cards across columns | Implemented |
| TC-003.4 | Clear search | Clearing search restores full board | Implemented |
| TC-003.5 | Select bead | Clicking card opens detail panel on right | Implemented |
| TC-003.6 | Detail shows fields | Detail panel shows title, ID, status, priority, labels, description, acceptance | Implemented |
| TC-003.7 | Close detail | X button closes detail panel | Implemented |
| TC-003.8 | Create bead | "+ New Bead" opens modal with title, type, priority, labels, description, acceptance fields | Implemented |
| TC-003.9 | Create bead submit | Submitting modal creates bead, card appears in OPEN column | Implemented |
| TC-003.10 | Claim bead | "Claim" button on open bead moves it to IN PROGRESS | Implemented |
| TC-003.11 | Unclaim bead | "Unclaim" button on in-progress bead moves it back to OPEN | Implemented (`e2e/beads.spec.ts` TC-003.11) |
| TC-003.12 | Close bead | "Close" button on in-progress bead moves it to CLOSED | Historical reference only — no `beadClose` mutation in `schema.graphql`; no Close button in `BeadDetail.svelte`. File an implementation bead before re-scheduling this test. |
| TC-003.13 | Reopen bead | "Re-open" on closed bead shows reason input, confirms reopens | Historical reference only — `beadReopen` mutation exists in `schema.graphql` but `BeadDetail.svelte` exposes no Reopen button. |
| TC-003.14 | Drag and drop | Dragging a card between columns updates status | Historical reference only — no drag-drop UI in the beads page today. |
| TC-003.15 | Dependency display | Detail panel shows dependency list with check/circle status | Planned |

### TC-004: Document Graph

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-004.1 | Graph loads | Page renders without error | Implemented |
| TC-004.2 | Nodes visible | Graph contains document nodes | Planned |

### TC-005: Agent Sessions

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-005.1 | Page loads | Agent sessions page renders | Implemented |
| TC-005.2 | Session list | Shows recent agent sessions if any exist | Planned |

### TC-006: Personas

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-006.1 | Persona list loads | Left panel shows persona entries | Implemented |
| TC-006.2 | Select persona | Clicking shows persona content in right panel | Implemented |
| TC-006.3 | Role badges | Persona cards show role badges | Implemented |
| TC-006.4 | Tag badges | Persona cards show tag badges | Planned |

### TC-007: Navigation

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-007.1 | Nav links | All 6 nav links visible: Dashboard, Documents, Beads, Graph, Agent, Personas | Implemented |
| TC-007.2 | Active state | Current page link is visually highlighted | Planned |
| TC-007.3 | SPA routing | All routes work without full page reload | Implemented |

### TC-008: HTTP API

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-008.1 | Health endpoint | `GET /api/health` returns `{"status":"ok"}` | Implemented |
| TC-008.2 | Documents list | `GET /api/documents` returns array | Implemented |
| TC-008.3 | Beads list | `GET /api/beads` returns array | Implemented |
| TC-008.4 | Beads status | `GET /api/beads/status` returns counts object | Implemented |
| TC-008.5 | Personas list | `GET /api/personas` returns array | Implemented |
| TC-008.6 | Doc graph | `GET /api/docs/graph` returns array | Implemented |

### TC-009: Demo Video

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-009.1 | Video captures all pages | Demo visits Dashboard, Documents, Beads, Graph, Agent, Personas | Implemented |
| TC-009.2 | Document interaction | Demo selects and reads a document | Implemented |
| TC-009.3 | Bead interaction | Demo searches beads, selects one, views detail | Implemented |
| TC-009.4 | Bead creation | Demo creates a new bead via the form | Implemented |
| TC-009.5 | Persona interaction | Demo selects a persona and views content | Implemented |
| TC-009.6 | Video quality | 1280x720, readable text, smooth pacing | Implemented |
| TC-009.7 | Video file produced | `demo-output/` contains a `.webm` video file | Implemented |

### TC-010: Project Registry and Scoped Routing

Ownership is split across layers. HTTP API, registry shape, singleton
fallback, isolation, and MCP coverage are Go-side — owned by
`cli/internal/server/server_test.go` and its companions. The UI project
picker is SvelteKit-side — owned by `cli/internal/server/frontend/e2e/navigation.spec.ts`.
Dedicated `projects.spec.ts` is intentionally not restored post-Svelte
migration; the equivalent coverage is split across `navigation.spec.ts`
(project picker) and the Go tests (HTTP + MCP contract).

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-010.1 | Registry loads | `GET /api/projects` lists each configured project root with a default marker | Implemented — `cli/internal/server/server_test.go` |
| TC-010.2 | Scoped API requests | `GET /api/projects/:project/beads` and sibling routes resolve the selected project context | Implemented — `cli/internal/server/server_test.go` |
| TC-010.3 | UI project picker | The web UI shows a project picker when more than one project is registered | Implemented — `cli/internal/server/frontend/e2e/navigation.spec.ts` TC-004, TC-005 |
| TC-010.4 | Singleton fallback | A single-project server still serves the legacy unscoped routes and dashboard | Implemented — `cli/internal/server/server_test.go` |
| TC-010.5 | Isolation | A malformed or missing project root reports degraded status without blocking healthy sibling projects | Implemented — `cli/internal/server/server_test.go` |
| TC-010.6 | Registry shape | Duplicate project ids fail registry loading before serving partial context | Planned — Go server tests |
| TC-010.7 | MCP registry listing | `ddx_list_projects` lists the registered projects and marks the default project | Implemented — `cli/internal/server/server_test.go` |
| TC-010.8 | MCP project lookup | `ddx_show_project` resolves the selected project context and returns the matching project metadata | Implemented — `cli/internal/server/server_test.go` |
| TC-010.9 | MCP scoped tool call | A project-aware MCP tool call runs against the selected project and returns that project's data | Implemented — `cli/internal/server/server_test.go` |

### TC-011: Host+User State and Node Identity

Verifies that `ddx-server` runs as a per-user host daemon with state at
`~/.local/share/ddx/server-state.json` and writes `~/.local/share/ddx/server.addr`,
per FEAT-020. These cases are owned by the Go server tests in
`cli/internal/server/server_test.go` (and companion `node_state_test.go`).

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-011.1 | State file location | Server writes `server-state.json` under `XDG_DATA_HOME/ddx` (not inside `.ddx/server/`) | Implemented |
| TC-011.2 | Addr file location | Server writes `server.addr` with URL, node name, and node ID under `XDG_DATA_HOME/ddx` | Implemented |
| TC-011.3 | Node identity endpoint | `GET /api/node` returns a stable `node-<hash>` ID derived from hostname or `DDX_NODE_NAME` | Implemented |
| TC-011.4 | State survives restart | Projects registered before a restart are still returned by `GET /api/projects` after restart | Implemented |
| TC-011.5 | CLI auto-registration | Running `ddx bead list` in a fresh project directory causes that project to appear in `GET /api/projects` within 1s | Planned |
| TC-011.6 | Single instance per host | A second `ddx server` start overwrites the addr file and the first instance does not continue serving the addr | Implemented |

### TC-012: Host+User Project Isolation and Concurrency

Verifies that one host+user server can serve multiple projects concurrently
without cross-project leakage, per SD-019. Owned entirely by the Go server
tests in `cli/internal/server/server_test.go` — isolation is a server
contract, not a UI surface.

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-012.1 | Bead isolation | `GET /api/projects/proj-a/beads` and `GET /api/projects/proj-b/beads` return disjoint bead sets from each project's own store | Implemented — `cli/internal/server/server_test.go` |
| TC-012.2 | Document isolation | A document present only in project A is not visible via project B's documents endpoint | Implemented — `cli/internal/server/server_test.go` |
| TC-012.3 | Concurrent requests | Parallel requests against different registered projects complete successfully without racing on adapters or caches | Implemented — `cli/internal/server/server_test.go` |
| TC-012.4 | Degraded project isolation | A malformed project root is reported as degraded in `GET /api/projects` while sibling projects continue serving | Implemented — `cli/internal/server/server_test.go` |
| TC-012.5 | Cache namespace | A cached lookup in project A does not surface the same key in project B | Implemented — `cli/internal/server/server_test.go` |

### TC-013: Execute-Loop Worker Lifecycle

Verifies that worker state is surfaced through autonomous worker reports and
server-derived views, per ADR-022. Any local supervisor used to start work is
an implementation detail; the bead store remains the claim authority.

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-013.1 | Worker start | `StartExecuteLoop` against a registered project creates a worker record and starts a goroutine | Implemented |
| TC-013.2 | Live logs | `Logs` returns streaming log output while the worker is running | Implemented |
| TC-013.3 | Worker stop | `Stop` cancels the running worker and the on-disk record transitions to a terminal state | Implemented |
| TC-013.4 | Worker scope | A worker started for project A writes worker records and execution artifacts only under project A's `.ddx/workers/` and `.ddx/executions/<attempt-id>/` directories | Implemented |
| TC-013.5 | Replay-backed attempts | Runtime metrics (harness, model, tokens, cost, base_rev, result_rev) are persisted into the project's `.ddx/executions/<attempt-id>/` bundle per FEAT-014 | Planned |
| TC-013.6 | Concurrent workers | Workers for two different registered projects run in parallel without cross-project filesystem writes | Implemented |

### TC-014: ts-net Identity Envelope

Verifies ADR-006 identity capture for localhost, ts-net, forwarded, and
managed-node commands.

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-014.1 | Localhost envelope | A localhost write records `immediate_actor_kind=localhost`, origin actor, node id, project id, and request id | Planned |
| TC-014.2 | ts-net envelope | A ts-net write records Tailscale user/node identity without requiring an API key | Planned |
| TC-014.3 | Forwarded provenance | A hub-forwarded write preserves origin actor and forwarding path through to the owning node's audit event | Planned |
| TC-014.4 | Future policy seam | GraphQL/REST write handlers accept and expose envelope fields without changing command payload shapes | Planned |

### TC-015: Read Federation

Verifies FEAT-026 hub/spoke pull aggregation.

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-015.1 | Hub with no spokes | `/federation` renders hub status and an empty spoke list | Planned |
| TC-015.2 | Spoke registration | A spoke registers and heartbeats to the hub with version/capability metadata | Planned |
| TC-015.3 | Federated reads | Hub combined bead/run/project views merge local and spoke rows with node/project badges | Planned |
| TC-015.4 | Stale/offline/degraded | Slow, unreachable, stale, and version-skewed spokes produce partial results and visible status badges | Planned |

### TC-016: Worker Single Pane and Server-Managed Work

Verifies FEAT-021 US-100/US-101. This is the basic "server runs work and the
operator sees it" contract. The fixture uses local JSONL-backed projects and
existing autonomous workers; Axon and 2k-scale performance are out of scope.

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-016.1 | Node-wide workers | `/nodes/:nodeId/workers` lists workers from at least two registered projects with project badge, worker id, state, current bead id/title, phase, route/model, elapsed time, and freshness | Planned |
| TC-016.2 | Worker detail | Clicking a worker opens detail with recent phase events, captured output, prompt, attempt id, current bead link, and stop action | Planned |
| TC-016.3 | Start multiple workers | Project workers page starts `count=2` autonomous `ddx work` workers through `startWorker`; both rows appear and the queue/worker summary updates | Planned |
| TC-016.4 | Worker cap | Starting beyond `.ddx/config.yaml workers.max_count` is refused or disabled with an operator-visible cap reason | Planned |
| TC-016.5 | Stop worker | Stop action reaches a stopping/terminal state and records a lifecycle/audit event | Planned |
| TC-016.6 | Live updates | Worker row updates current bead and phase without page reload via subscription or polling fallback | Planned |

### TC-017: Federated Worker Control and Owner-Targeted Writes

Verifies FEAT-026 US-097b and FEAT-021 US-102. The hub is a single pane of glass,
but the owning node executes every mutation locally.

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-017.1 | Federated worker list | Hub `/nodes/:hubId/workers?scope=federation` lists hub and spoke workers with node/project badges and freshness | Planned |
| TC-017.2 | Start spoke worker | Starting a worker for a spoke project from the hub forwards to exactly that spoke; the worker is created on the spoke, not the hub | Planned |
| TC-017.3 | Stop spoke worker | Stopping a spoke worker from the hub forwards to the owning spoke and the hub view reflects the terminal state | Planned |
| TC-017.4 | Offline refusal | Commands for an offline spoke are refused with an operator-visible reason and do not create phantom local rows | Planned |
| TC-017.5 | Idempotency | Retrying a forwarded command with the same request id returns the original result without launching duplicates or appending duplicate events | Planned |

### TC-018: Cross-Project Bead Queue Mutations

Verifies FEAT-021 US-103. Queue stewardship must be possible from the web UI for
any registered project.

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-018.1 | Create bead | Creating a bead with title, description, acceptance, priority, type, and labels persists it in the selected project only | Planned |
| TC-018.2 | Edit bead | Editing title, labels, priority, description, acceptance, or status persists and updates the row without cross-project leakage | Planned |
| TC-018.3 | Lifecycle actions | Approve, block, cancel, and reopen mutate lifecycle state, append events, and update queue summary | Planned |
| TC-018.4 | Federated bead write | Hub create/edit for a spoke project forwards to the owning spoke and appears in federated reads with origin/forwarding audit metadata | Planned |
| TC-018.5 | Invalid state refusal | Empty required reason/note, stale state, or non-owned target is rejected with a visible error | Planned |

### TC-019: Cross-Project Spec Editing

Verifies FEAT-021 US-104. Spec editing is scoped to project documents,
especially `docs/helix/**`.

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-019.1 | Save FEAT doc | Editing and saving a `docs/helix/**` document writes through `documentWrite`, refreshes rendered content, and stays in the selected project | Planned |
| TC-019.2 | Path confinement | Absolute paths and `../` traversal writes are rejected and no file is written | Planned |
| TC-019.3 | Stale write refusal | Saving after the document changed externally fails with a conflict/stale-write message instead of overwriting | Planned |
| TC-019.4 | Federated document write | Hub save for a spoke project forwards to the owning spoke and preserves origin/forwarding audit metadata | Planned |
| TC-019.5 | Graph/staleness refresh | After a spec save, document graph/staleness views refresh or expose a clear refresh affordance | Planned |

### TC-020: Managed-Node Remote Control

Verifies FEAT-029 / ADR-028 outbound managed-node behavior. The fixture must
start the managed node without an inbound ts-net listener; all hub visibility
comes from the outbound channel and pushed state.

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-020.1 | Outbound registration | A managed node dials the hub by DNS/MagicDNS name and registers node/project/capability metadata | Planned |
| TC-020.2 | Derived state | Hub UI renders managed-node projects, beads, workers, runs, and logs from snapshots/events/backfill | Planned |
| TC-020.3 | Remote worker start | Hub command starts a local autonomous worker on the managed node and progress appears in the hub UI | Planned |
| TC-020.4 | Remote worker stop/cancel | Hub command requests local stop/cancel; managed node records the command and worker honors it at a safe point | Planned |
| TC-020.5 | Operator prompt write-through | Hub submit/approve/cancel for an operator prompt persists on the managed node with origin/forwarding audit | Planned |
| TC-020.6 | Idempotency | Retrying a mutating command with the same request id returns the original result without duplicate state | Planned |
| TC-020.7 | Conflict rejection | A command targeting stale or changed local state is rejected and the rejection is shown in the hub UI | Planned |
| TC-020.8 | Offline refusal | Hub refuses new commands for an offline managed node and renders stale derived state clearly | Planned |

### TC-021: Hermetic E2E Gate

Verifies that server/web/federation e2e tests are suitable as reliability
gates.

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-021.1 | Fixture isolation | E2E tests run against temp projects and isolated `XDG_DATA_HOME`, not the developer's live DDx state | Planned |
| TC-021.2 | No tracked dirtiness | Running the e2e suite leaves `git status --short` clean except for pre-existing unrelated changes | Planned |
| TC-021.3 | Quarantined known failures | Not-yet-implemented UI features are skipped or isolated, not part of the reliability gate | Planned |
| TC-021.4 | Screenshot determinism | Visual baselines are pinned or regenerated only by an explicit update flow | Planned |

## Out of Scope

- MCP transport-level testing (covered by Go unit tests)
- Identity policy beyond ADR-006's trusted localhost/ts-net v1 boundary
- General performance benchmarks outside the artifact-listing measurement contract
- Mobile/responsive layout testing
