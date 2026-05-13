# Phase 02: Design

The architecture phase where we transform requirements into a concrete technical plan with contracts, tests, and implementation strategy.

## Purpose

The Design phase transforms validated business requirements from Frame into a comprehensive technical architecture. This phase focuses on making and documenting architectural decisions, defining system structure, and establishing contracts that will guide implementation.

## Key Principle

**Architecture Before Implementation**: Design establishes the system's structure, makes key technical decisions, and defines all interfaces before any code is written. This ensures a solid foundation and prevents architectural drift.

## Input Gate

Before starting Design, verify Frame outputs:
- [ ] PRD reviewed and approved
- [ ] Success metrics clearly defined
- [ ] User stories with acceptance criteria
- [ ] Principles document established
- [ ] All stakeholders aligned on scope
- [ ] Security requirements documented and approved
- [ ] Threat model completed with risk assessment
- [ ] Compliance requirements identified and mapped

## Artifacts

### 1. Solution Design
**Location**: `docs/helix/02-design/solution-designs/SD-XXX-[name].md`

Bridges business requirements to technical implementation:
- **Requirements Mapping**: Transform functional/non-functional requirements to technical capabilities
- **Solution Approaches**: Multiple alternatives with trade-offs
- **Domain Model**: Core entities and business rules
- **Component Decomposition**: Breaking down the system
- **Technology Rationale**: Why specific technologies were chosen

Use a solution design when the scope is feature-level or cross-component. It
is the design artifact that explains the chosen approach for a feature. Do not
use it for one bounded story slice.

### 1a. Technical Design
**Location**: `docs/helix/02-design/technical-designs/TD-XXX-[name].md`

Bridges one user story to implementation details:
- **Acceptance Criteria Mapping**: How one story will be satisfied
- **Component Changes**: Specific changes for one bounded slice
- **Interfaces and Data**: Story-scoped contracts and schema impact
- **Testing Approach**: How the story will be verified
- **Implementation Sequence**: Practical order for building the slice

Use a technical design when the scope is one story or one bounded vertical
slice. It should inherit the broader architecture from the parent solution
design instead of redefining feature-level structure.

`ux-design` remains retired as a standalone HELIX artifact. Put user-facing
flow rationale, wireframes, and interaction trade-offs in the governing
solution design or technical design, and use a proof of concept when the main
risk is validating an interactive workflow end to end.

`auth-design` is retired as a standalone HELIX artifact. Put authentication
and authorization security posture — trust boundaries, identity, session
handling, token security, and access control policy — in `security-architecture`,
which has a dedicated "Identity and Access" section for this purpose. Use a
`solution-design` when the auth subsystem is a first-class implementation
target (e.g., a custom identity provider or multi-tenant RBAC engine) that
needs a full feature-level design artifact of its own.

### 2. Architecture Diagrams
**Location**: `docs/helix/02-design/architecture.md`

Visual system documentation using C4 model:
- **System Context**: How the system fits in the larger ecosystem
- **Container Diagram**: Major architectural components
- **Component Diagrams**: Internal structure of containers
- **Deployment Diagram**: Infrastructure and deployment
- **Data Flow**: How information moves through the system

### 3. Contracts
**Location**: `docs/helix/02-design/contracts/CONTRACT-XXX-[name].md`

Defines normative interface and schema specifications that another team can
implement against directly:
- **CLI Contracts**: Command structure, options, input/output formats
- **HTTP/API Contracts**: Endpoints, request/response schemas, error codes
- **Library Contracts**: Public functions, parameters, return types
- **Protocol and Event Contracts**: Message shapes, sequencing, compatibility rules
- **Data Contracts**: JSON schemas, validation rules, units, enums, formulas
- **Error Contracts**: Error codes, messages, retry behavior, recovery actions

Use a contract when the document is the authoritative field-by-field,
schema-level, or interface-level specification. Put decision rationale in ADRs,
feature-level approach in solution designs, and one-slice execution planning in
technical designs.

`CONTRACT-XXX` is the canonical prefix. `API-XXX` remains an allowed subtype
for API-specific contracts when that narrower label improves clarity.

### 4. Architecture Decision Records (ADRs)
**Location**: `docs/helix/02-design/adr/ADR-XXX-[title].md`

Documents significant architectural decisions:
- **Decision Context**: Problem and constraints
- **Alternatives Considered**: Options with pros/cons
- **Decision Rationale**: Why this approach was chosen
- **Consequences**: Trade-offs and impacts
- **Success Criteria**: How to validate the decision

### 5. Security Architecture
**Location**: `docs/helix/02-design/security-architecture.md`

Defines the design-level security posture that implementation and testing must
follow:
- **Trust Boundaries**: Component and integration boundaries that require controls
- **Control Mapping**: Preventive, detective, and recovery controls tied to threats
- **Identity and Access**: Authentication, authorization, sessions, and token handling
- **Data Protection**: Encryption, secrets handling, and sensitive-data controls
- **Verification Hooks**: Testable expectations for security tests and reviews

Use `security-architecture` when the document is the canonical place for
design-level security decisions. Keep requirements in Frame's
`security-requirements` and `threat-model`; keep schema and lifecycle details
in `data-design`.

### 6. Data Design
**Location**: `docs/helix/02-design/data-design.md`

Comprehensive data architecture:
- **Conceptual Model**: Business entities and relationships
- **Logical Model**: Database schema design
- **Data Patterns**: Soft deletes, audit trails, versioning
- **Access Patterns**: Common queries and optimization
- **Migration Strategy**: Moving from current to target state

`data-protection` is not a separate design artifact. Put design-level privacy,
encryption, handling, and monitoring controls in `security-architecture`; keep
regulatory and retention obligations in Frame's `compliance-requirements`; use
`data-design` only for schema, storage, and lifecycle consequences.

### Technical Investigation Artifacts (Optional - When Technical Uncertainty Exists)

When significant technical unknowns exist about architecture, technology choices, integration complexity, or implementation approaches, technical investigation artifacts can be used to reduce risk and validate approaches before committing to detailed design.

#### 7. Technical Spike (Optional)
**Artifact Location**: `artifacts/tech-spike/`
**Output Location**: `docs/helix/02-design/spikes/SPIKE-XXX-[name].md`

Time-boxed technical investigation for unknowns:
- **When to Use**: Technical approach unclear, architecture decision risk, implementation complexity unknown
- **Objectives**: Specific technical questions that need answers
- **Investigation Methods**: Prototyping, benchmarking, comparative analysis, expert consultation
- **Time Budget**: Strict time boundaries (typically 1-5 days)
- **Evidence-Based Findings**: Concrete results with measurements and data
- **Actionable Recommendations**: Clear next steps for design decisions

**Triggers for Technical Spike**:
- "Which technology/approach should we use?" (needs validation)
- "Can this architecture handle our requirements?" (feasibility question)
- "How complex will integration with System X be?" (complexity assessment)
- Unknown performance characteristics or scalability limits
- Novel technical approaches requiring validation

#### 8. Proof of Concept (Optional)
**Artifact Location**: `artifacts/proof-of-concept/`
**Output Location**: `docs/helix/02-design/proofs-of-concept/POC-XXX-[name].md`

Minimal working implementation to validate technical concepts:
- **When to Use**: High-risk technical approach, novel architecture, complex integration, end-to-end validation needed
- **Working Implementation**: Functional system demonstrating core concept
- **End-to-End Validation**: Complete workflows tested from input to output
- **Production Readiness Assessment**: What would be needed for production deployment
- **Performance Characteristics**: Measured system behavior under realistic conditions
- **Integration Strategy**: Validated approach for connecting with other systems

**Triggers for Proof of Concept**:
- High-risk or novel architectural approaches
- Complex system integration requirements
- Performance requirements need validation
- User experience concepts require testing
- Technology stack viability needs demonstration

### Technical Investigation Workflow Integration

#### When Technical Investigation is Needed
Technical investigation artifacts should be considered when:

1. **Technical Uncertainty**: Unknown implementation complexity or approach viability
2. **High-Risk Decisions**: Architecture choices with significant impact if wrong
3. **Novel Technology**: Unproven or unfamiliar technical approaches
4. **Complex Integration**: Integration complexity or compatibility unknown
5. **Performance Critical**: Performance characteristics unknown or requirements stringent

#### Investigation-Informed Design Process
```mermaid
graph TD
    A[Frame Requirements Available] --> B{Technical Uncertainty Exists?}
    B -->|Yes| C[Identify Technical Questions]
    B -->|No| D[Standard Design Artifacts]
    C --> E{Scope of Investigation}
    E -->|Quick Questions| F[Technical Spike]
    E -->|Complex Validation| G[Proof of Concept]
    F --> H[Spike Findings]
    G --> I[PoC Results]
    H --> J{Sufficient Confidence?}
    I --> J
    J -->|No| K[Additional Investigation]
    J -->|Yes| D
    K --> E
    D --> L[Solution Design & Architecture]
```

#### Integration with Standard Design Artifacts
Technical investigation findings directly inform design artifacts:

- **Spike Findings → Architecture Decisions**: Technical validation becomes ADR rationale
- **PoC Results → Solution Design**: Proven approaches inform implementation strategy
- **Performance Data → Contracts**: Measured characteristics define API performance requirements
- **Integration Testing → Data Design**: Validated integration patterns inform data architecture

#### Time Management for Technical Investigations
- **Technical Spike**: 1-5 days maximum, strictly time-boxed
- **Proof of Concept**: 1-2 weeks, focused on core concept validation
- **Decision Points**: Clear criteria for when investigation provides sufficient evidence
- **Integration Time**: Budget 1-2 days to integrate findings into design artifacts

## Artifact Selection Guide

Understanding which artifact to use is critical for maintaining clear documentation. This guide helps you choose the right artifact type for your design documentation needs.

### Quick Decision Tree

When deciding which artifact to use, ask these questions in order:

1. **Is this a fundamental architectural decision that would be expensive to change?**
   - YES → **ADR** (e.g., "Use GraphQL for internal APIs", "Adopt microservices architecture")
   - NO → Continue to question 2

2. **Are you defining a normative interface or schema another team could implement against directly?**
   - YES → **Contract** (e.g., "Telemetry export payload schema", "CLI mutation surface", "Execution document format")
   - NO → Continue to question 3

3. **Are you evaluating or selecting specific technologies/libraries?**
   - YES → **Tech Spike** (e.g., "Redis vs Hazelcast for caching", "Caliban vs Sangria for GraphQL")
   - NO → Continue to question 4

4. **Are you defining how to implement an architectural approach?**
   - YES → **Solution Design** (e.g., "GraphQL federation architecture", "Database sharding strategy")
   - NO → Consider if this belongs in Build phase as an Implementation Guide

### Artifact Boundaries and Relationships

#### ADRs - Architectural Decisions (WHY)
**Purpose**: Document fundamental decisions that shape the system architecture
**Scope**: Protocol choices, architectural patterns, system boundaries, data strategies
**Review Cycle**: Only when requirements fundamentally change
**Example**: "We will separate internal and external API surfaces because they have different SLAs, security models, and evolution rates"

#### Contracts - Normative Interfaces (WHAT EXACTLY)
**Purpose**: Define the exact external surface another team can implement against directly
**Scope**: CLI/API/library surfaces, event schemas, protocol messages, validation rules, precedence, and error semantics
**Review Cycle**: When the governed interface changes
**Example**: "Telemetry export payload schema including exact field names, units, enums, precedence, and error behavior"

#### Tech Spikes - Technology Selection (WHAT)
**Purpose**: Evaluate and select specific technologies to implement architectural decisions
**Scope**: Library comparisons, performance testing, feasibility studies
**Review Cycle**: When new versions released or better alternatives emerge
**Example**: "After evaluating Caliban, Sangria, and GraphQL-Java, we selected Caliban for its native ZIO integration"

#### Solution Designs - Implementation Architecture (HOW)
**Purpose**: Define how to build the system using chosen technologies
**Scope**: Component design, integration patterns, data flows, deployment architecture
**Review Cycle**: When implementation reveals better patterns
**Example**: "Internal GraphQL will use Apollo Federation to combine Caliban services with PostGraphile"

### Artifact Flow Example

Here's how artifacts build on each other:

```
1. ADR-012: "Use GraphQL for internal APIs"
   ↓ (Architectural decision made)
2. SPIKE-003: "GraphQL library evaluation for Scala"
   ↓ (Caliban selected)
3. SD-005: "Internal GraphQL federation implementation"
   ↓ (Feature-level approach defined)
4. CONTRACT-004: "Internal GraphQL schema and error contract"
   ↓ (Normative interface defined)
5. Build Phase: Implementation guides and code
```

### Common Mistakes to Avoid

❌ **Don't put technology selection in ADRs**
- Wrong: ADR titled "Use Caliban for GraphQL"
- Right: ADR "Use GraphQL for internal APIs" + Spike "GraphQL library selection"

❌ **Don't repeat architectural rationale in Tech Spikes**
- Wrong: Tech Spike explaining why GraphQL is better than REST
- Right: Tech Spike comparing GraphQL libraries assuming GraphQL is already chosen

❌ **Don't put implementation details in Solution Designs**
- Wrong: Solution Design with code snippets and configuration values
- Right: Solution Design with architecture diagrams and patterns

### Cross-Referencing

Each artifact should reference related artifacts:
- ADRs should note which Tech Spikes validate the decision
- Tech Spikes should reference the ADR they support
- Solution Designs should reference both the ADR (why) and Tech Spike (what technology)
- Contracts should reference the ADR or design that gives them scope
- ADRs, solution designs, and technical designs should reference contracts rather than duplicating normative fields or schemas

### Reference
For complete artifact boundary definitions, see [ADR-011: Design Phase Artifact Boundaries](/docs/helix/02-design/adr/adr-011-design-artifact-boundaries.md) in the DDX documentation.

## Process Flow

```mermaid
graph TD
    A[Review Frame Outputs] --> B[Create Solution Design]
    B --> C[Design Architecture]
    C --> D[Create Diagrams]
    D --> E[Define Contracts]
    E --> F[Design Data Model]
    F --> G[Design Security]
    G --> H[Document ADRs]
    H --> I[Validate Against Principles]
    I --> J{Principles Met?}
    J -->|No| K[Simplify Design]
    K --> C
    J -->|Yes| L[Technical Review]
    L --> M{Approved?}
    M -->|No| C
    M -->|Yes| N[Gate to Test Phase]
```

## Human vs AI Responsibilities

### Human Responsibilities
- **Architecture Decisions**: Choose technologies and patterns
- **Trade-off Analysis**: Balance competing constraints
- **Risk Assessment**: Identify technical challenges
- **Contract Definition**: Define external interfaces
- **Review and Approval**: Validate design decisions

### AI Assistant Responsibilities
- **Contract Generation**: Create detailed interface specifications
- **Test Case Design**: Generate comprehensive test scenarios
- **Consistency Validation**: Check alignment with requirements
- **Documentation**: Structure technical specifications
- **Pattern Suggestions**: Recommend proven solutions
- **Technical Analysis**: Synthesize spike and PoC findings into actionable insights
- **Risk Assessment**: Identify technical risks from multiple implementation approaches
- **Performance Analysis**: Process benchmark data and identify optimization opportunities

## Design Principles Enforcement

### Core Principles Checklist
- [ ] **Library-First**: Each feature is a standalone library
- [ ] **CLI Interface**: All functionality exposed via text interface
- [ ] **Test-First**: Tests specified before implementation
- [ ] **Simplicity**: Maximum 3 major components
- [ ] **Anti-Abstraction**: Direct framework usage, no wrappers
- [ ] **Integration Testing**: Real environments over mocks

### Complexity Budget
Track design complexity to prevent over-engineering:

| Component | Complexity Points | Justification |
|-----------|------------------|---------------|
| Component 1 | 1 | Core requirement |
| Component 2 | 1 | Essential integration |
| Component 3 | 1 | (Only if justified) |
| **Total** | **≤ 3** | Must stay within budget |

## Quality Gates

Before proceeding to Test phase:

### Completeness Checklist
- [ ] All contracts fully specified with examples
- [ ] Solution design bridges requirements to technical approach
- [ ] Architecture diagram shows data flow
- [ ] Technology choices justified
- [ ] ADRs document key decisions
- [ ] No [TO BE DEFINED] markers remain

#### Technical Investigation Completion (When Applicable)
- [ ] Technical spikes completed within time budget with evidence-based findings
- [ ] Proof of concepts demonstrate end-to-end functionality
- [ ] Investigation findings integrated into architecture and design decisions
- [ ] Technical risks identified and mitigation strategies documented
- [ ] Performance characteristics measured and documented
- [ ] Integration complexity assessed and approaches validated

### Validation Questions
1. **Contract Clarity**: Could another team implement from these contracts alone?
2. **Decision Rationale**: Are all architectural decisions justified with ADRs?
3. **Simplicity**: Have we used the minimum viable architecture?
4. **Principles**: Does the design comply with all project principles?
5. **Feasibility**: Can this be built within time and resource constraints?

## Common Pitfalls

### ❌ Avoid These Mistakes

1. **Over-Engineering**
   - Bad: Complex microservices for simple CRUD
   - Good: Monolithic library with CLI interface

2. **Implementation Details in Contracts**
   - Bad: "Calls internal processData() function"
   - Good: "Accepts JSON, returns transformed JSON"

3. **Mock-Heavy Testing**
   - Bad: Mocking all external dependencies
   - Good: Testing with real databases and services

4. **Abstraction Layers**
   - Bad: Custom wrapper around framework
   - Good: Direct framework usage

5. **Missing Error Cases**
   - Bad: Only happy path in contracts
   - Good: Comprehensive error scenarios

## Success Criteria

The Design phase is complete when:

1. **Solution Defined**: Requirements mapped to technical approach
2. **Architecture Documented**: Visual diagrams and component structure clear
3. **Contracts Specified**: All external interfaces defined
4. **Decisions Recorded**: ADRs document key choices with rationale
5. **Data Model Complete**: Schema and patterns designed
6. **Security Addressed**: Threats identified and mitigated

## Next Phase: Test

Once Design is validated, proceed to Test phase where you'll:
- Write comprehensive test specifications
- Implement failing tests (Red phase)
- Define test data and scenarios
- Establish test infrastructure

Remember: Design defines the architecture - Test validates it through executable specifications.

## Tips for Success

1. **Start with Contracts**: Define what before how
2. **Think in Tests**: Every contract needs test cases
3. **Embrace Constraints**: Principles prevent complexity
4. **Iterate on Simplicity**: First design is rarely simple enough
5. **Document Decisions**: Capture why, not just what

## Anti-Patterns to Avoid

### 🚫 Speculative Generality
Don't design for hypothetical future needs. Design for current requirements only.

### 🚫 Framework Religions
Choose tools pragmatically based on requirements, not preferences.

### 🚫 Test Theater
Don't write tests that always pass. Tests must fail first, then pass.

### 🚫 Contract Drift
Implementation must match contracts exactly. No undocumented behavior.

## Using AI Assistance

Design prompts live under `.ddx/plugins/helix/workflows/phases/02-design/artifacts/`.
Open the relevant `prompt.md`, use the adjacent template, and write outputs to
the canonical `docs/helix/02-design/` paths defined in each artifact.

Common entry points:
- `artifacts/solution-design/`
- `artifacts/technical-design/`
- `artifacts/adr/`
- `artifacts/tech-spike/` and `artifacts/proof-of-concept/` for bounded
  investigation work

AI is useful for tradeoff analysis, option comparison, and documentation
drafting. Architectural decisions, acceptance of complexity, and final design
sign-off remain human responsibilities.

The AI excels at generating comprehensive contracts and test cases but human judgment is essential for architectural decisions and technical trade-offs.

## Design Review Checklist

Before approval, ensure:
- [ ] Contracts are complete and unambiguous
- [ ] Every contract has test cases
- [ ] Architecture is as simple as possible
- [ ] All principles are followed (or violations documented)
- [ ] Implementation plan is realistic
- [ ] Team understands and agrees with design

---

*Good design is not when there is nothing left to add, but when there is nothing left to remove.*
