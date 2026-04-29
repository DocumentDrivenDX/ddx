# Documentation Structure

This document describes the organization of documentation in the DDx project, which follows the HELIX workflow phases to ensure consistency and traceability.

## Overview

The documentation structure mirrors the HELIX workflow phases, creating a logical progression from problem definition through deployment. This organization ensures that:

- Documentation flows naturally through the development process
- Artifacts are easy to locate based on their lifecycle phase
- Cross-phase relationships are clear and traceable
- Each phase has dedicated space for its specific outputs

## Directory Structure

```
docs/
├── README.md                    # This overview document
├── frame/                       # Phase 01: Requirements & Problem Definition
│   ├── prd.md                  # Product Requirements Document
│   ├── principles.md           # Project-specific principles
│   ├── features/               # Feature specifications (plural)
│   │   ├── FEAT-001-[name].md # Feature specification
│   │   ├── FEAT-002-[name].md # Feature specification
│   │   └── feature-registry.md # Central feature tracking
│   └── user-stories/           # User stories (plural)
│       ├── US-001-[name].md   # User story collection
│       └── US-002-[name].md   # User story collection
├── design/                     # Phase 02: Technical Architecture
│   ├── architecture.md         # System architecture diagrams (singular)
│   ├── data-design.md         # Data architecture (singular)
│   ├── security.md            # Security design (singular)
│   ├── solution-designs/       # Solution designs (plural)
│   │   ├── SD-001-[name].md   # Solution design for features
│   │   └── SD-002-[name].md   # Solution design for features
│   ├── contracts/              # API contracts (plural)
│   │   ├── API-001-[name].md  # API contract specification
│   │   └── API-002-[name].md  # API contract specification
│   └── adr/                    # Architecture Decision Records (plural)
│       ├── ADR-001-[title].md # Architectural decision record
│       └── ADR-002-[title].md # Architectural decision record
├── test/                       # Phase 03: Test Strategy & Specifications
│   ├── test-plan.md           # Overall testing strategy (singular)
│   └── test-procedures.md     # Test execution procedures (singular)
├── build/                      # Phase 04: Implementation Strategy
│   ├── implementation-plan.md  # Build strategy and planning (singular)
│   └── build-procedures.md    # Development procedures (singular)
├── deploy/                     # Phase 05: Deployment & Operations
│   └── [TBD]                  # Deployment-specific documentation
└── iterate/                    # Phase 06: Post-Deployment & Feedback
    └── [TBD]                  # Iteration and improvement documentation
```

## Legacy Structure

The following directories contain documentation from the previous organization:

### [Architecture](/docs/architecture/)
Technical architecture, system design, and architectural decision records.

### [Implementation](/docs/implementation/)
Setup guides, configuration documentation, and implementation patterns.

### [Usage](/docs/usage/)
User guides, tutorials, FAQs, and troubleshooting resources.

### [Development](/docs/development/)
Developer documentation, contributing guidelines, and development standards.

### [References](/docs/references/)
External resources, research materials, and third-party documentation.

### [Resources](/docs/resources/)
Shared methodologies, principles, and workflow documentation.

## Naming Conventions

### Identifier Systems

The documentation uses systematic identifiers to ensure traceability across phases:

- **FEAT-XXX**: Feature identifiers (e.g., FEAT-001, FEAT-002)
- **US-XXX**: User Story identifiers (e.g., US-001, US-002)
- **SD-XXX**: Solution Design identifiers (e.g., SD-001, SD-002)
- **API-XXX**: API Contract identifiers (e.g., API-001, API-002)
- **ADR-XXX**: Architecture Decision Record identifiers (e.g., ADR-001, ADR-002)

### File Naming

- **Singular artifacts**: Project-wide documents use descriptive names (e.g., `architecture.md`, `test-plan.md`)
- **Plural artifacts**: Feature-specific documents use ID + name format (e.g., `FEAT-001-user-authentication.md`)
- **Collections**: Directories containing multiple related artifacts use plural names

## Artifact Categories

### Singular vs Plural Artifacts

**Singular Artifacts** (one per project):
- Product Requirements Document (`frame/prd.md`)
- Project Principles (`frame/principles.md`)
- System Architecture (`design/architecture.md`)
- Data Design (`design/data-design.md`)
- Security Design (`design/security.md`)
- Test Plan (`test/test-plan.md`)
- Test Procedures (`test/test-procedures.md`)
- Implementation Plan (`build/implementation-plan.md`)
- Build Procedures (`build/build-procedures.md`)

**Plural Artifacts** (multiple per project):
- Feature Specifications (`frame/features/FEAT-XXX-[name].md`)
- User Stories (`frame/user-stories/US-XXX-[name].md`)
- Solution Designs (`design/solution-designs/SD-XXX-[name].md`)
- API Contracts (`design/contracts/API-XXX-[name].md`)
- Architecture Decision Records (`design/adr/ADR-XXX-[title].md`)

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
1. **Business Context**: Start in `frame/` for what and why
2. **Technical Details**: Move to `design/` for how
3. **Quality Assurance**: Check `test/` for verification approach
4. **Implementation**: Review `build/` for development approach

### For Maintainers
1. **Keep Structure Consistent**: Follow naming conventions
2. **Update Cross-References**: Maintain links between phases
3. **Archive Completed Cycles**: Move old iterations to archive
4. **Document Changes**: Update this README when structure evolves

## Project Documentation Convention

### Using Helix Phases in Projects

When applying the HELIX workflow to your project, organize documentation using the `docs/helix-phases/` convention:

```
project-root/
├── docs/
│   ├── helix-phases/           # HELIX phase documentation
│   │   ├── 01-frame/          # Problem definition & requirements
│   │   ├── 02-design/         # Architecture & design decisions
│   │   ├── 03-test/           # Test strategies & plans
│   │   ├── 04-build/          # Implementation guidance
│   │   ├── 05-deploy/         # Deployment & operations
│   │   └── 06-iterate/        # Continuous improvement
│   ├── reference/             # Reference documentation
│   ├── operations/            # Operational procedures
│   └── strategy/              # Strategic planning
```

This convention:
- Keeps phase documentation separate from operational docs
- Uses numbered prefixes for clear ordering
- Places artifacts directly in phase directories (no `artifacts/` subdirectory)
- Allows for project-specific documentation outside the phases

### Why `helix-phases/`?

The `helix-phases/` naming:
1. Clearly indicates use of the HELIX workflow
2. Separates phase artifacts from other documentation
3. Maintains consistency across projects using HELIX
4. Enables tooling to identify and validate phase structure

## Tools and Automation

### Validation
The structure can be validated using:
```bash
# Check for required artifacts
ddx validate docs-structure

# Verify cross-references
ddx validate traceability

# Generate artifact inventory
ddx list artifacts
```

### Templates
Use workflow templates to create consistent artifacts:
```bash
# Create new feature specification
ddx apply workflows/helix/phases/01-frame/artifacts/feature-specification

# Generate solution design
ddx apply workflows/helix/phases/02-design/artifacts/solution-design
```

## Migration Notes

This structure was established to replace the previous organization. Key changes:

1. **Phase-Based Organization**: Documents organized by lifecycle phase
2. **Systematic Identifiers**: FEAT-XXX, US-XXX naming for traceability
3. **Singular/Plural Distinction**: Clear separation of project-wide vs feature-specific artifacts
4. **Workflow Alignment**: Structure mirrors HELIX workflow phases

For legacy documents, see the directories listed under Legacy Structure above.

---

*This documentation structure supports the HELIX workflow principle of clear phase separation while maintaining comprehensive traceability throughout the development lifecycle.*