# Feature: Artifact Tooling (ADR + Solution Design)

**ID:** FEAT-005
**Status:** Not Started
**Priority:** P1
**Owner:** DDx Team

## Overview

DDx manages the documents that drive development. Two artifact types appear in nearly every document-driven project: Architecture Decision Records (ADRs) and Solution Designs (SDs). The `ddx adr` and `ddx sd` commands provide scaffolding, listing, and validation for these artifacts — so projects get consistent, well-structured design documents without manual boilerplate.

## Problem Statement

**Current situation:** ADRs and solution designs are created manually by copy-pasting from previous examples or templates. There's no tooling to allocate IDs, enforce structure, or validate that required sections exist.

**Pain points:**
- ID allocation is manual and error-prone (duplicate IDs, gaps in sequence)
- No validation that frontmatter or required sections exist until review time
- No standard way to list or query existing decisions and designs
- Templates vary between projects with no shared convention

**Desired outcome:** `ddx adr create` and `ddx sd create` scaffold well-structured documents with correct IDs and frontmatter. Validation catches structural problems early. Listing provides a quick inventory of project decisions and designs.

## Requirements

### Functional

1. **ADR scaffolding** (`ddx adr create "Title"`) — allocate next ID, create file from template with frontmatter
2. **ADR listing** (`ddx adr list`) — scan project for ADRs, display ID/title/status from frontmatter
3. **ADR display** (`ddx adr show <id>`) — show ADR content
4. **ADR validation** (`ddx adr validate`) — check frontmatter, required sections, dependency references
5. **SD scaffolding** (`ddx sd create "Title"`) — allocate next ID, create file from template
6. **SD listing** (`ddx sd list`) — scan project for SDs
7. **SD display** (`ddx sd show <id>`) — show SD content
8. **SD validation** (`ddx sd validate`) — check frontmatter, required sections

### Non-Functional

- **Performance:** Scanning and listing complete in <500ms for projects with up to 200 artifacts
- **Portability:** No external dependencies beyond the `ddx` binary
- **Extensibility:** Output directories and templates are configurable for workflow-specific layouts

## Artifact Formats

### ADR Format

```yaml
---
dun:
  id: ADR-NNN
  depends_on: []        # References to other artifact IDs
---
# ADR-NNN: Title

| Date | Status | Deciders | Confidence |
|------|--------|----------|------------|
| YYYY-MM-DD | Proposed | | |

## Context
## Decision
## Alternatives
## Consequences
## Risks
```

**Required sections:** Context, Decision, Alternatives
**Status values:** Proposed, Accepted, Deprecated, Superseded

### SD Format

```yaml
---
dun:
  id: SD-NNN
  depends_on: []        # References to features, ADRs, etc.
---
# SD-NNN: Title

## Scope
## Acceptance Criteria
## Solution Approaches
## Component Changes
```

**Required sections:** Scope, Acceptance Criteria, Solution Approaches

### Frontmatter Convention

Both artifacts use the `dun` frontmatter namespace:
- `dun.id` — artifact identifier (e.g., `ADR-001`, `SD-003`)
- `dun.depends_on` — array of artifact IDs this document depends on

This convention is shared across all DDx-managed artifacts and enables cross-referencing and dependency validation.

## Configuration

| Setting | Config Path | Env Var | Default |
|---------|------------|---------|---------|
| ADR output directory | `artifact.adr.dir` | `DDX_ADR_DIR` | `docs/adr/` |
| SD output directory | `artifact.sd.dir` | `DDX_SD_DIR` | `docs/designs/` |
| ADR template | `artifact.adr.template` | — | built-in |
| SD template | `artifact.sd.template` | — | built-in |

Workflows override these. For example, HELIX sets ADR dir to `docs/helix/02-design/adr/` and SD dir to `docs/helix/02-design/solution-designs/`.

## CLI Surface

```
ddx adr create "Title" [--depends-on ID,ID] [--status S]
ddx adr list [--status S] [--json]
ddx adr show <id>
ddx adr validate [path|--all]

ddx sd create "Title" [--depends-on ID,ID] [--feature FEAT-NNN]
ddx sd list [--json]
ddx sd show <id>
ddx sd validate [path|--all]
```

## Server Endpoints (FEAT-002 integration)

| MCP Tool | HTTP Endpoint | Description |
|----------|--------------|-------------|
| `ddx_list_adrs` | `GET /api/adrs` | List ADRs with status |
| `ddx_show_adr` | `GET /api/adrs/:id` | Show ADR content |
| `ddx_list_designs` | `GET /api/designs` | List SDs |
| `ddx_show_design` | `GET /api/designs/:id` | Show SD content |

## User Stories

### US-024: Scaffold an ADR
**As a** developer making an architecture decision
**I want** to run `ddx adr create "Use PostgreSQL for persistence"` and get a well-structured ADR
**So that** I don't waste time on boilerplate and IDs

**Acceptance Criteria:**
- Given no ADRs exist, when I run `ddx adr create "Use PostgreSQL"`, then `docs/adr/ADR-001-use-postgresql.md` is created with correct frontmatter and sections
- Given ADR-001 and ADR-003 exist, when I create a new ADR, then it gets ID ADR-004 (next after max)
- Given I specify `--depends-on FEAT-001`, then `dun.depends_on` includes `FEAT-001`

### US-025: List and Validate ADRs
**As a** developer reviewing project decisions
**I want** to list all ADRs and validate their structure
**So that** I can quickly find decisions and catch formatting problems

**Acceptance Criteria:**
- Given ADRs exist, when I run `ddx adr list`, then I see a table with ID, title, status, and date
- Given I run `ddx adr validate --all`, then each ADR is checked for frontmatter and required sections
- Given an ADR is missing the Decision section, when I validate it, then I get a clear error

### US-026: Scaffold a Solution Design
**As a** developer designing a feature implementation
**I want** to run `ddx sd create "User authentication flow"` and get a structured SD
**So that** I start with the right sections and frontmatter

**Acceptance Criteria:**
- Given no SDs exist, when I run `ddx sd create "User auth flow"`, then `docs/designs/SD-001-user-auth-flow.md` is created
- Given I specify `--feature FEAT-002`, then `dun.depends_on` includes `FEAT-002`
- Given I specify `--depends-on ADR-001`, then `dun.depends_on` includes `ADR-001`

### US-027: Validate Solution Designs
**As a** developer checking design quality
**I want** to validate that SDs have required sections and valid frontmatter
**So that** structural problems are caught before review

**Acceptance Criteria:**
- Given I run `ddx sd validate --all`, then each SD is checked for frontmatter and required sections
- Given an SD is missing Acceptance Criteria, when I validate it, then I get a clear error
- Given an SD references a nonexistent dependency, when I validate it, then I get a warning

## Edge Cases

- Create when output directory doesn't exist — create it
- Create with a title that slugifies to empty string — error with clear message
- List when no artifacts exist — show helpful message
- Validate a file that isn't an ADR/SD — skip with warning
- ID gap in existing sequence (ADR-001, ADR-003) — allocate next after max, not fill gaps

## Dependencies

- `.ddx/` directory or DDx-initialized project
- Templates from DDx library (built-in fallback if library not present)

## Out of Scope

- Artifact lifecycle management (draft → governing → realized) — that's workflow-level
- Cross-artifact consistency checks beyond dependency resolution — that's `helix align`
- Generating ADRs or SDs from natural language — that's agent/workflow work
