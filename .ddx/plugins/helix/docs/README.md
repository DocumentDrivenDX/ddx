# Documentation Structure

This document describes the organization of documentation in the HELIX repository, which follows the HELIX workflow phases to ensure consistency and traceability.

## Overview

The documentation structure mirrors the HELIX workflow phases, creating a logical progression from problem definition through deployment. This organization ensures that:

- Documentation flows naturally through the development process
- Artifacts are easy to locate based on their lifecycle phase
- Cross-phase relationships are clear and traceable
- Each phase has dedicated space for its specific outputs

## Directory Structure

In this repository, the canonical HELIX phase artifacts live under `docs/helix/`.

```
docs/
├── README.md                     # This overview document
└── helix/
    ├── 00-discover/             # Phase 00: Discovery and validation
    │   └── product-vision.md    # Product vision for HELIX
    ├── 01-frame/                # Phase 01: Requirements & Problem Definition
    │   ├── concerns.md          # Active project concerns and overrides
    │   ├── prd.md               # Product Requirements Document
    │   └── features/            # Feature specifications (plural)
    │       ├── FEAT-001-[name].md # Feature specification
    │       └── FEAT-011-[name].md # Feature specification
    ├── 02-design/               # Phase 02: Technical Architecture
    │   ├── adr/                 # Architecture Decision Records (plural)
    │   │   ├── ADR-001-[title].md # Architectural decision record
    │   │   └── ADR-002-[title].md # Architectural decision record
    │   ├── contracts/           # API and workflow contracts (plural)
    │   │   ├── API-001-[name].md # API contract specification
    │   │   └── CONTRACT-001-[name].md # Cross-system contract
    │   ├── data-design.md       # Data architecture
    │   ├── security-architecture.md # Security design
    │   ├── solution-designs/    # Solution designs (plural)
    │   │   ├── SD-001-[name].md # Solution design for features
    │   │   └── SD-002-[name].md # Solution design for features
    │   └── technical-designs/   # Technical designs (plural)
    │       ├── TD-002-[name].md # Technical implementation design
    │       └── TD-011-[name].md # Technical implementation design
    ├── 03-test/                 # Phase 03: Test Strategy & Specifications
    │   ├── executions/          # Recorded execution evidence
    │   │   └── README.md        # Execution log conventions
    │   └── test-plans/          # Test plans (plural)
    │       └── TP-002-[name].md # Test plan
    ├── 04-build/                # Phase 04: Implementation Strategy
    │   └── implementation-plan.md # Build strategy and planning
    ├── 05-deploy/               # Phase 05: Deployment & Operations
    │   ├── deployment-checklist.md # Release go/no-go checklist
    │   ├── monitoring-setup.md  # Observability and alerting setup
    │   ├── release-notes.md     # Release communication artifact
    │   └── runbook.md           # Operator response and rollback guide
    └── 06-iterate/              # Phase 06: Post-Deployment & Feedback
        ├── alignment-reviews/   # Alignment and drift review reports
        ├── improvement-backlog.md # Prioritized follow-on work
        ├── metrics/             # Metric definitions and fixtures
        ├── metrics-dashboard.md # Iteration health and outcome summary
        └── security-metrics.md  # Security posture summary
```

## Historical Categories

Before this repository adopted the numbered `docs/helix/` layout, HELIX
materials were commonly grouped into broad categories such as
`architecture/`, `implementation/`, `usage/`, `development/`, and
`references/`.

Those names are historical examples, not canonical paths in this repository,
and this repo does not ship those directories as current `docs/` locations.
In this repo, governed HELIX artifacts live under `docs/helix/`,
authoritative shared workflow references live under `workflows/references/`,
`docs/resources/` contains supporting background material, and other
repository docs live as topic-specific files or directories directly under
`docs/`.

## Naming Conventions

### Identifier Systems

The documentation uses systematic identifiers to ensure traceability across phases:

- **FEAT-XXX**: Feature identifiers (e.g., FEAT-001, FEAT-002)
- **US-XXX**: User Story identifiers (e.g., US-001, US-002)
- **SD-XXX**: Solution Design identifiers (e.g., SD-001, SD-002)
- **API-XXX**: API Contract identifiers (e.g., API-001, API-002)
- **ADR-XXX**: Architecture Decision Record identifiers (e.g., ADR-001, ADR-002)

### File Naming

- **Singular artifacts**: Project-wide documents use descriptive names (e.g., `prd.md`, `security-architecture.md`)
- **Plural artifacts**: Feature-specific documents use ID + name format (e.g., `FEAT-001-user-authentication.md`)
- **Collections**: Directories containing multiple related artifacts use plural names

## Artifact Categories

### Singular vs Plural Artifacts

**Singular Artifacts in this repository**:
- Product Vision (`docs/helix/00-discover/product-vision.md`)
- Project Concerns (`docs/helix/01-frame/concerns.md`)
- Product Requirements Document (`docs/helix/01-frame/prd.md`)
- Data Design (`docs/helix/02-design/data-design.md`)
- Security Architecture (`docs/helix/02-design/security-architecture.md`)
- Implementation Plan (`docs/helix/04-build/implementation-plan.md`)
- Deployment Checklist (`docs/helix/05-deploy/deployment-checklist.md`)
- Monitoring Setup (`docs/helix/05-deploy/monitoring-setup.md`)
- Runbook (`docs/helix/05-deploy/runbook.md`)
- Release Notes (`docs/helix/05-deploy/release-notes.md`)
- Metrics Dashboard (`docs/helix/06-iterate/metrics-dashboard.md`)
- Security Metrics (`docs/helix/06-iterate/security-metrics.md`)
- Improvement Backlog (`docs/helix/06-iterate/improvement-backlog.md`)

**Plural Artifacts in this repository**:
- Feature Specifications (`docs/helix/01-frame/features`)
- Solution Designs (`docs/helix/02-design/solution-designs`)
- Technical Designs (`docs/helix/02-design/technical-designs`)
- API Contracts (`docs/helix/02-design/contracts`)
- Architecture Decision Records (`docs/helix/02-design/adr`)
- Test Plans (`docs/helix/03-test/test-plans`)
- Execution Records (`docs/helix/03-test/executions`)
- Alignment Reviews (`docs/helix/06-iterate/alignment-reviews`)
- Metrics Fixtures (`docs/helix/06-iterate/metrics`)

## Phase-Specific Details

### Frame Phase (01)
**Purpose**: Define what to build and why
**Key Outputs**:
- Business requirements and success metrics
- User needs and personas
- Feature identification and prioritization
- Project constraints and principles

### Design Phase (02)
**Purpose**: Define how to build the solution
**Key Outputs**:
- Technical architecture and component design
- API contracts and interface specifications
- Data models and security design
- Architecture decision rationale

### Test Phase (03)
**Purpose**: Define how to verify the solution
**Key Outputs**:
- Comprehensive test strategy
- Test execution procedures
- Quality gates and coverage targets

### Build Phase (04)
**Purpose**: Implement the solution
**Key Outputs**:
- Implementation strategy and timeline
- Development procedures and standards
- Code and build artifacts

### Deploy Phase (05)
**Purpose**: Release the solution to production
**Key Outputs**:
- Deployment procedures and automation
- Monitoring and observability setup
- Go-live and rollback procedures

### Iterate Phase (06)
**Purpose**: Improve based on feedback
**Key Outputs**:
- Performance analysis and improvements
- User feedback integration
- Next cycle planning

## Traceability

The structure enables clear traceability across phases:

1. **Frame → Design**: Features (FEAT-XXX) become Solution Designs (SD-XXX)
2. **Frame → Design**: User Stories (US-XXX) inform API Contracts (API-XXX)
3. **Design → Test**: Contracts define test specifications
4. **Test → Build**: Tests drive implementation priorities
5. **Build → Deploy**: Implementation artifacts inform deployment

## Usage Guidelines

### For Authors
1. **Start with Frame**: Always begin with business requirements
2. **Follow Phase Order**: Complete prerequisites before moving to next phase
3. **Use Identifiers**: Assign and track FEAT-XXX, US-XXX, etc.
4. **Maintain Traceability**: Link related artifacts across phases

### For Readers
1. **Business Context**: Start in `docs/helix/01-frame/` for what and why
2. **Technical Details**: Move to `docs/helix/02-design/` for how
3. **Quality Assurance**: Check `docs/helix/03-test/` for verification approach
4. **Implementation**: Review `docs/helix/04-build/` for development approach

### For Maintainers
1. **Keep Structure Consistent**: Follow naming conventions
2. **Update Cross-References**: Maintain links between phases
3. **Archive Completed Cycles**: Move old iterations to archive
4. **Document Changes**: Update this README when structure evolves

## Project Documentation Convention

### Using Helix Phases in Projects

When applying the HELIX workflow to your project, organize governed workflow
artifacts under `docs/helix/` and keep the reusable workflow library under
`workflows/phases/`.

The tree below is a generic project example, not a literal map of this
repository:

```
project-root/
├── docs/
│   ├── helix/                 # Project-specific HELIX artifacts
│   │   ├── 00-discover/       # Optional discovery and validation
│   │   ├── 01-frame/          # Problem definition & requirements
│   │   ├── 02-design/         # Architecture & design decisions
│   │   ├── 03-test/           # Test strategies & plans
│   │   ├── 04-build/          # Implementation guidance
│   │   ├── 05-deploy/         # Deployment & operations
│   │   └── 06-iterate/        # Continuous improvement
│   ├── reference/             # Reference documentation
│   ├── operations/            # Operational procedures
│   └── strategy/              # Strategic planning
├── workflows/
│   └── phases/                # Shared HELIX artifact prompts/templates
```

This convention:
- Keeps governed HELIX artifacts separate from operational docs
- Uses numbered prefixes for clear ordering
- Stores project outputs under `docs/helix/`, while shared library assets live
  under `workflows/phases/`
- Allows for project-specific documentation outside the phases

### Why `docs/helix/`?

The `docs/helix/` layout:
1. Matches the active HELIX workflow contract used in this repository
2. Separates phase artifacts from other documentation
3. Maintains consistency across projects using HELIX
4. Keeps the project artifact tree distinct from the shared workflow library in
   `workflows/phases/`

## Tools and Automation

### Validation
Use supported HELIX entrypoints to inspect and reconcile the workflow stack:
```bash
# Review the artifact stack for drift
helix align repo

# Decide the next bounded HELIX action for the repository
helix check repo

# Inspect execution-safe tracked work
ddx bead ready --execution
```

### Templates
The reusable artifact library lives under `workflows/phases/.../artifacts/`.
Use the HELIX commands to create or refine project artifacts, and reference the
library paths directly when you need the underlying template assets:
```bash
# Create or refine frame artifacts for a scope
helix frame auth

# Create or refine a design document for a scope
helix design auth

# Reference the shared artifact template directories directly
workflows/phases/01-frame/artifacts/feature-specification/
workflows/phases/02-design/artifacts/solution-design/
```

## Migration Notes

This structure was established to replace the previous organization. Key changes:

1. **Phase-Based Organization**: Documents organized by lifecycle phase
2. **Systematic Identifiers**: FEAT-XXX, US-XXX naming for traceability
3. **Singular/Plural Distinction**: Clear separation of project-wide vs feature-specific artifacts
4. **Workflow Alignment**: Structure mirrors HELIX workflow phases

For historical context, see the category notes above rather than treating them
as live repository paths.

---

*This documentation structure supports the HELIX workflow principle of clear phase separation while maintaining comprehensive traceability throughout the development lifecycle.*
