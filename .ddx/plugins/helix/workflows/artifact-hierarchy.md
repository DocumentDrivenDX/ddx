---
ddx:
  id: helix.workflow.artifact-hierarchy
  depends_on:
    - helix.workflow
---
# HELIX Artifact Hierarchy and Naming Conventions

*Understanding how artifacts flow through the HELIX workflow*

## Overview

The HELIX workflow uses a consistent artifact naming system that enables:
- **Traceability**: Track artifacts from requirements to deployment
- **Vertical Slicing**: Implement stories independently
- **Parallel Development**: Multiple stories in different phases
- **Clear Organization**: Predictable file locations

The workflow also supports a project-level **Parking Lot** registry at
`docs/helix/parking-lot.md` for deferred and future work. Any artifact can be
marked with `ddx.parking_lot: true` to keep it out of dependency graphs and
the main PRD flow while remaining in its normal directory.

## Scope Boundary

This document explains authority order, artifact relationships, naming, and
traceability. It does not define ready-queue logic, loop control, or how to
select execution work.

For execution behavior, follow [EXECUTION.md](EXECUTION.md) and the bounded
action prompts. For tracker conventions, see `ddx bead --help` (DDx FEAT-004).

## Canonical Authority Order

Artifact flow and artifact authority are related but not identical. When two
HELIX artifacts disagree, this is the canonical authority order:

1. **Product Vision**
2. **PRD**
3. **Feature Specifications and User Stories**
4. **Architecture and ADRs**
5. **Solution Designs and Technical Designs**
6. **Test Plans and Executable Tests**
7. **Implementation Plans**
8. **Source Code and Build Artifacts**

### Notes

- Feature specifications and user stories refine the PRD and remain above downstream design and implementation artifacts.
- Tests govern Build phase execution because they are executable specifications, but they are still derived from Frame and Design artifacts.
- Source code must conform to higher-order artifacts; it does not redefine them.
- Issues are not part of the canonical authority order. They are execution
  records derived from authoritative artifacts.

## Artifact Types and Relationships

### Canonical Artifacts Plus Execution Issues

```mermaid
graph TD
    subgraph "Project Level"
        PRD[PRD - Product Requirements]
        ARCH[Architecture]
        PRINCIPLES[Principles]
        IMP[Implementation Plan]
    end

    subgraph "Feature Level (Epics)"
        FEAT[FEAT-XXX - Feature Spec]
        SD[SD-XXX - Solution Design]
    end

    subgraph "Story Level (Vertical Slices)"
        US[US-XXX - User Story]
        TD[TD-XXX - Technical Design]
        TP[TP-XXX - Test Plan]
    end

    subgraph "Execution Layer (Non-Canonical)"
        BB[Build Issues]
        DB[Deploy Issues]
    end

    subgraph "Iterate Outputs"
        MD[metrics-dashboard]
        SM[security-metrics]
        IB[improvement-backlog]
    end

    PRD --> FEAT
    FEAT --> US
    FEAT --> SD
    SD --> TD
    US --> TD --> TP
    IMP --> BB
    TP --> BB --> DB
    DB --> MD
    DB --> SM
    MD --> IB
    SM --> IB
```

## Story-Level Progression and Execution

Each user story progresses through all phases independently:

### Naming Pattern
Canonical story document artifacts use `{Prefix}-{Number}-{descriptive-name}.md`.
Build and deploy execution use native tracker issue IDs. A story enters
ITERATE when all matching `phase:deploy` issues are complete and no matching
deploy issue remains not closed. Shared iterate outputs stay project- or
iteration-level context, while tracker-backed follow-on work adds
story-specific evidence when needed.

### Phase Progression
```
Frame:   US-036-list-mcp-servers.md
Design:  TD-036-list-mcp-servers.md
Test:    TP-036-list-mcp-servers.md
Build:   issue `hx-a3f2dd` labeled `helix`, `phase:build`, `story:US-036`
Deploy:  issue `hx-b4c9e1` labeled `helix`, `phase:deploy`, `story:US-036`
Iterate: all `phase:deploy` issue(s) for `story:US-036` are complete and no
         matching deploy issue remains not closed; optional tracker follow-on
         work may remain linked to US-036
Context: `metrics-dashboard.md`, `security-metrics.md` (when relevant), and
         `improvement-backlog.md` provide shared iteration-wide context
```

### Artifact Descriptions

| Surface | Artifact Type | Phase | Purpose |
|--------|--------------|-------|---------|
| US | User Story | Frame | Defines WHAT needs to be built |
| TD | Technical Design | Design | Details HOW to build it |
| TP | Test Plan | Test | Specifies tests to verify it |
| ISSUE | Build / Deploy Issue | Build / Deploy | Tracks scoped execution work in the built-in tracker |
| Iterate outputs | `metrics-dashboard`, `security-metrics`, `improvement-backlog`, plus tracker follow-on work | Iterate | Shared iteration context and prioritized next work after story-level ITERATE is established by completed deploy issue(s), without a separate numbered story report |

## Feature-Level Progression (Epics)

Features represent collections of related stories:

### Naming Pattern
```
Frame:   FEAT-001-mcp-server-management.md
Design:  SD-001-mcp-management.md
```

### Relationships
- One feature (FEAT) contains multiple user stories (US)
- One solution design (SD) guides multiple technical designs (TD)

## Directory Structure

All HELIX artifacts are under `docs/helix/` to support multiple workflows:

```
docs/
└── helix/                              # HELIX workflow artifacts
    ├── 00-discover/
    │   ├── product-vision.md          # Optional project-level discovery
    │   ├── business-case.md
    │   ├── competitive-analysis.md
    │   └── opportunity-canvas.md
    ├── 01-frame/
    │   ├── prd.md                     # Project-level
    │   ├── principles.md               # Project-level
    │   ├── features/
    │   │   └── FEAT-XXX-*.md          # Feature-level
    │   └── user-stories/
    │       └── US-XXX-*.md            # Story-level
    ├── 02-design/
    │   ├── architecture.md            # Project-level
    │   ├── solution-designs/
    │   │   └── SD-XXX-*.md           # Feature-level
    │   └── technical-designs/
    │       └── TD-XXX-*.md           # Story-level
    ├── 03-test/
    │   ├── test-plan.md               # Project-level
    │   └── test-plans/
    │       └── TP-XXX-*.md           # Story-level (NEW)
    ├── 04-build/
    │   ├── implementation-plan.md     # Project-level
    ├── 05-deploy/
    │   ├── deployment-checklist.md    # Project-level
    │   ├── monitoring-setup.md        # Project-level
    │   ├── runbook.md                 # Project-level
    │   └── release-notes.md           # Project-level
    └── 06-iterate/
        ├── metrics-dashboard.md       # Project-level
        ├── security-metrics.md        # Project-level
        ├── improvement-backlog.md     # Project-level
        ├── alignment-reviews/
        │   └── AR-YYYY-MM-DD-*.md    # Cross-phase reconciliation reports
        └── backfill-reports/
            └── BF-YYYY-MM-DD-*.md    # Research-first backfill reports

.helix/                               # Built-in tracker workspace
```

## Cross-References

Each artifact references its dependencies:

### Story-Level References
```markdown
# TD-036-list-mcp-servers.md
**User Story**: [[US-036-list-mcp-servers]]
**Parent Feature**: [[FEAT-001-mcp-server-management]]
**Solution Design**: [[SD-001-mcp-management]]
```

### Feature-Level References
```markdown
# SD-001-helix-supervisory-control.md
**Feature**: [[FEAT-001-helix-supervisory-control]]
**PRD**: [[helix.prd]]
**ADR**: [[ADR-001]]
```

### Traceability Chain
```
FEAT-001 → SD-001 → US-036 → TD-036 → TP-036 → build issue(s) → deploy issue(s) → all deploy issue(s) complete with no matching deploy issue remaining not closed + optional follow-on tracker work (ITERATE evidence)
         ↓
         US-037 → TD-037 → TP-037 → build issue(s) → deploy issue(s) → all deploy issue(s) complete with no matching deploy issue remaining not closed + optional follow-on tracker work (ITERATE evidence)
         ↓
         US-038 → TD-038 → TP-038 → build issue(s) → deploy issue(s) → all deploy issue(s) complete with no matching deploy issue remaining not closed + optional follow-on tracker work (ITERATE evidence)
```

## Naming Rules

### Consistency Rules
1. **Number stays constant**: 036 throughout all phases
2. **Name stays constant**: "list-mcp-servers" throughout
3. **Only canonical story document prefixes change**: US → TD → TP
4. **Build, Deploy, and Iterate use tracker issues and phase docs**: execution is tracked in the built-in tracker, and iterate outcomes land in canonical iterate outputs rather than a numbered HELIX story file

### Valid Examples
✅ `US-001-initialize-ddx.md`
✅ `TD-001-initialize-ddx.md`
✅ `FEAT-014-obsidian-integration.md`

### Invalid Examples
❌ `US-1-init.md` (number must be 3 digits)
❌ `td-001-initialize.md` (prefix must be uppercase)
❌ `US-001-init-ddx.md` (name changed between artifacts)

## State Detection

The workflow state is determined by which artifacts exist:

### Story State Detection
```yaml
If exists US-036: Story is in FRAME
If exists TD-036: Story is in DESIGN
If exists TP-036: Story is in TEST
If open HELIX build issues exist for story US-036: Story is in BUILD
If any HELIX deploy issue for story US-036 is not closed, including status: in_progress: Story is in DEPLOY
If all deploy issues for story US-036 are complete and no matching deploy issue remains not closed: Story is in ITERATE
Linked tracker follow-on work adds iterate evidence when present; shared
iterate outputs provide iteration context but are not queried by story ID
```

### Feature State Detection
```yaml
If all stories have US: Feature is in FRAME
If all stories have TD: Feature is in DESIGN
If all stories have tests passing: Feature is in BUILD
```

## Benefits

### 1. Vertical Slicing
Each story can be deployed independently:
- US-036 could be in production (ITERATE)
- US-037 could be in testing (TEST)
- US-038 could be in design (DESIGN)

### 2. Clear Traceability
Follow any requirement through its lifecycle:
```bash
grep -r "US-001" docs/helix/  # Find all artifacts for story 001
```

### 3. Parallel Development
Multiple team members can work on different stories:
- Developer A: Implementing US-036 (BUILD)
- Developer B: Designing US-037 (DESIGN)
- Developer C: Writing tests for US-038 (TEST)

### 4. No State File Needed
State is derived from artifacts:
- No `.helix-state.yml` to maintain
- Git history shows state changes
- Self-healing (state always reflects reality)

## Migration Path

For existing projects:

### Option 1: New Stories Only
- Keep existing artifacts as-is
- Use new naming for new stories
- Both systems coexist

### Option 2: Gradual Migration
- Rename artifacts as they're updated
- Start with active stories
- Complete over time

### Option 3: Full Migration
- Batch rename all artifacts
- Update all references
- Clean cutover

## Examples

### Example 1: MCP Server Management Feature

```
Feature Level:
  FEAT-001-mcp-server-management.md
  SD-001-mcp-management.md

Story Level:
  US-036-list-mcp-servers.md → ... → deploy issue(s) → all deploy issue(s) complete with no matching deploy issue remaining not closed + optional follow-on tracker work
  US-037-install-mcp-server.md → ... → deploy issue(s) → all deploy issue(s) complete with no matching deploy issue remaining not closed + optional follow-on tracker work
  US-038-configure-mcp-server.md → ... → deploy issue(s) → all deploy issue(s) complete with no matching deploy issue remaining not closed + optional follow-on tracker work
```

### Example 2: Story in Multiple Phases

```
Monday:   Create US-041-user-authentication.md (FRAME)
Tuesday:  Create TD-041-user-authentication.md (DESIGN)
Wednesday: Create TP-041-user-authentication.md (TEST)
Thursday: Create build issue(s) for US-041 (BUILD)
Friday:   Create deploy issue(s) for US-041 (DEPLOY)
Next Week: Complete all deploy issue(s) and leave no matching deploy issue not closed; story enters ITERATE and any follow-on work is captured in tracker-backed iteration evidence
```

## Commands

Use the current HELIX queue controls with this hierarchy:

```bash
# Inspect the current queue
ddx bead ready --json

# Execute one ready issue
helix build

# Decide the next action when the queue drains
helix check

# Run the wrapper loop
helix run
```

---

*This hierarchy enables true agile development with independently deployable story slices while maintaining full traceability.*
