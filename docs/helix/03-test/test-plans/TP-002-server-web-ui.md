---
ddx:
  id: TP-002
  depends_on:
    - FEAT-002
    - FEAT-008
---
# Test Plan: DDx Server and Web UI

**ID:** TP-002
**Features:** FEAT-002 (Server), FEAT-008 (Web UI)
**Status:** Active

## Scope

End-to-end testing of the DDx server HTTP API, MCP tools, and embedded web
UI. Tests run against a live `ddx server` instance with real project data
(documents, beads, personas, execution definitions) from one or more project
roots.

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
Multi-project fixtures use a server config that registers multiple project
roots so request routing and the UI project picker can be exercised in one run.

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
| TC-003.11 | Unclaim bead | "Unclaim" button on in-progress bead moves it back to OPEN | Planned |
| TC-003.12 | Close bead | "Close" button on in-progress bead moves it to CLOSED | Planned |
| TC-003.13 | Reopen bead | "Re-open" on closed bead shows reason input, confirms reopens | Planned |
| TC-003.14 | Drag and drop | Dragging a card between columns updates status | Planned |
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

The MCP registry and project-scoped routing checks in this section remain
planned future coverage and are not yet implemented in
`cli/internal/server/server_test.go` alongside the existing HTTP API and MCP
endpoint coverage. The planned MCP coverage here should exercise project
listing via `ddx_list_projects`, project lookup via `ddx_show_project`, and at
least one project-aware MCP tool call using an explicit project selection.
These cases are owned by the Go server tests in
`cli/internal/server/server_test.go`.

| ID | Test | Acceptance | Status |
|----|------|------------|--------|
| TC-010.1 | Registry loads | `GET /api/projects` lists each configured project root with a default marker | Planned |
| TC-010.2 | Scoped API requests | `GET /api/projects/:project/beads` and sibling routes resolve the selected project context | Planned |
| TC-010.3 | UI project picker | The web UI shows a project picker when more than one project is registered | Planned |
| TC-010.4 | Singleton fallback | A single-project server still serves the legacy unscoped routes and dashboard | Planned |
| TC-010.5 | Isolation | A malformed or missing project root reports degraded status without blocking healthy sibling projects | Planned |
| TC-010.6 | Registry shape | Duplicate project ids fail registry loading before serving partial context | Planned |
| TC-010.7 | MCP registry listing | `ddx_list_projects` lists the registered projects and marks the default project | Planned |
| TC-010.8 | MCP project lookup | `ddx_show_project` resolves the selected project context and returns the matching project metadata | Planned |
| TC-010.9 | MCP scoped tool call | A project-aware MCP tool call runs against the selected project and returns that project's data | Planned |

## Out of Scope

- MCP transport-level testing (covered by Go unit tests)
- Authentication (not yet implemented)
- Performance benchmarks
- Mobile/responsive layout testing
