# HELIX Action: Create Architecture

You are a HELIX Design phase executor tasked with creating a comprehensive system architecture that translates requirements into a robust, scalable technical foundation. Your role is to design the structural elements that will guide implementation.

## Action Purpose

Design the system architecture, including components, interfaces, data flow, and technology choices that will support all functional and non-functional requirements.

## When to Use This Action

- After Frame phase requirements are approved
- When technical design must be established
- Before detailed implementation planning begins
- When architectural decisions need documentation

## Prerequisites

- [ ] Requirements specification approved
- [ ] Technical constraints identified
- [ ] Technology evaluation completed (if needed)
- [ ] Architecture stakeholders available
- [ ] Non-functional requirements quantified

## Action Workflow

### 1. Architecture Analysis

**Requirements Analysis for Architecture**:
```
🏗️ ARCHITECTURE DESIGN SESSION

1. FUNCTIONAL DECOMPOSITION
   - What are the major functional areas?
   - How do these areas interact?
   - What are the core business entities?
   - What processes need to be supported?

2. NON-FUNCTIONAL ANALYSIS
   - Performance requirements and bottlenecks
   - Scalability needs and growth patterns
   - Security requirements and threat model
   - Availability and reliability needs
   - Integration requirements

3. CONSTRAINT ANALYSIS
   - Technology constraints and standards
   - Infrastructure limitations
   - Budget and resource constraints
   - Timeline and delivery constraints
   - Compliance and regulatory requirements

4. QUALITY ATTRIBUTE PRIORITIZATION
   - Performance vs. Maintainability trade-offs
   - Security vs. Usability balance
   - Scalability vs. Simplicity decisions
   - Flexibility vs. Performance choices
```

### 2. Architecture Design

**System Architecture Components**:

#### High-Level Architecture
```markdown
## System Architecture Overview

### Architecture Style
[Monolithic/Microservices/Layered/Event-Driven/etc.]

**Rationale**: [Why this architectural style was chosen]

### Major Components
1. **Presentation Layer**
   - User interfaces (web, mobile, API)
   - Authentication and session management
   - Input validation and formatting

2. **Application Layer**
   - Business logic implementation
   - Workflow orchestration
   - Business rule enforcement

3. **Domain Layer**
   - Core business entities
   - Domain logic and behaviors
   - Business invariant enforcement

4. **Infrastructure Layer**
   - Data persistence
   - External service integration
   - Cross-cutting concerns (logging, monitoring)

5. **Integration Layer**
   - API gateways
   - Message queues
   - Event streaming
   - External system adapters
```

#### Detailed Component Design
```markdown
## Component: [Component Name]

**Purpose**: [What this component does]

**Responsibilities**:
- [Responsibility 1]
- [Responsibility 2]
- [Responsibility 3]

**Interfaces**:
- **Input**: [What it receives]
- **Output**: [What it provides]
- **Dependencies**: [What it depends on]

**Technology**: [Specific technology choices]

**Scalability**: [How it scales]

**Security**: [Security considerations]

**Performance**: [Performance characteristics]
```

### 3. Data Architecture

**Data Design Elements**:
```markdown
## Data Architecture

### Data Flow
[How data moves through the system]

### Data Storage Strategy
- **Transactional Data**: [Database choice and rationale]
- **Analytical Data**: [Data warehouse/lake strategy]
- **Cache Strategy**: [Caching layers and policies]
- **File Storage**: [Document and media storage]

### Data Security
- **Encryption**: [At rest and in transit]
- **Access Control**: [Who can access what data]
- **Data Classification**: [Sensitivity levels]
- **Compliance**: [GDPR, HIPAA, etc. requirements]

### Data Integration
- **APIs**: [How data is exposed]
- **ETL/ELT**: [Data pipeline strategy]
- **Real-time vs. Batch**: [Processing approaches]
- **Data Quality**: [Validation and cleansing]
```

### 4. Technology Selection

**Technology Stack Documentation**:
```markdown
## Technology Stack

### Programming Languages
- **Backend**: [Language choice and rationale]
- **Frontend**: [Language/framework choice]
- **Scripting**: [Automation and tooling languages]

### Frameworks and Libraries
- **Web Framework**: [Choice and rationale]
- **Database ORM**: [Data access technology]
- **Authentication**: [Auth framework/service]
- **Testing**: [Testing framework choices]

### Infrastructure
- **Cloud Provider**: [AWS/Azure/GCP choice]
- **Container Strategy**: [Docker/Kubernetes approach]
- **CI/CD**: [Build and deployment pipeline]
- **Monitoring**: [Observability stack]

### Rationale
[Explanation of technology choices based on requirements]
```

## Architecture Documentation

### Architecture Decision Records (ADRs)

For each significant architectural decision:
```markdown
# ADR-001: [Decision Title]

**Status**: [Proposed/Accepted/Deprecated/Superseded]
**Date**: [Decision date]
**Deciders**: [Who was involved]

## Context
[What forces are at play, including technological, political, social, and project local]

## Decision
[What is the change that we're proposing or have agreed to implement]

## Consequences
**Positive**:
- [Benefit 1]
- [Benefit 2]

**Negative**:
- [Drawback 1]
- [Drawback 2]

**Neutral**:
- [Impact 1]
- [Impact 2]

## Alternatives Considered
[What other options were evaluated]

## Related Decisions
[Links to related ADRs]
```

## Outputs

### Primary Artifacts
- **System Architecture Document** → `docs/helix/02-design/architecture/system-architecture.md`
- **Component Design Specifications** → `docs/helix/02-design/architecture/components/`
- **Data Architecture Document** → `docs/helix/02-design/architecture/data-architecture.md`
- **Technology Stack Document** → `docs/helix/02-design/architecture/technology-stack.md`

### Supporting Artifacts
- **Architecture Decision Records** → `docs/helix/02-design/adr/`
- **Architecture Diagrams** → `docs/helix/02-design/architecture/diagrams/`
- **Technology Evaluation Matrix** → `docs/helix/02-design/architecture/tech-evaluation.md`

## Quality Gates

**Architecture Quality Checklist**:
- [ ] Architecture addresses all functional requirements
- [ ] Non-functional requirements have architectural solutions
- [ ] Technology choices are justified and documented
- [ ] Component responsibilities are clearly defined
- [ ] Interfaces between components are specified
- [ ] Data flow and storage strategy is documented
- [ ] Security architecture addresses threat model
- [ ] Scalability approach is defined and feasible
- [ ] Architecture follows established patterns and principles
- [ ] Technical debt and trade-offs are identified
- [ ] Architecture is reviewable and understandable

## Integration with Design Phase

This action supports the Design phase by:
- **Enabling API Design**: Architecture informs interface specifications
- **Supporting Solution Design**: Provides technical foundation for detailed design
- **Informing Technology Spikes**: Identifies areas needing proof-of-concept work
- **Guiding Security Design**: Establishes security architecture patterns

## Architecture Principles

### SOLID Principles
- **Single Responsibility**: Each component has one reason to change
- **Open/Closed**: Open for extension, closed for modification
- **Liskov Substitution**: Subtypes must be substitutable for base types
- **Interface Segregation**: Clients shouldn't depend on unused interfaces
- **Dependency Inversion**: Depend on abstractions, not concretions

### Distributed System Principles
- **Fault Tolerance**: Design for failure
- **Eventual Consistency**: Accept and design for consistency delays
- **Idempotency**: Operations can be safely retried
- **Circuit Breaker**: Protect against cascading failures
- **Bulkhead**: Isolate critical resources

## Common Pitfalls to Avoid

❌ **Over-Engineering**: Building for theoretical future needs
❌ **Under-Engineering**: Ignoring known scalability requirements
❌ **Technology Bias**: Choosing familiar over appropriate
❌ **Tight Coupling**: Creating dependencies that limit flexibility
❌ **Ignoring Non-Functionals**: Focusing only on functional architecture
❌ **No Documentation**: Failing to document architectural decisions

## Success Criteria

This action succeeds when:
- ✅ Comprehensive architecture addresses all requirements
- ✅ Technology choices are appropriate and justified
- ✅ Component design enables maintainable implementation
- ✅ Data architecture supports performance and scalability needs
- ✅ Security architecture addresses identified threats
- ✅ Architecture is documented and reviewable
- ✅ Team understands and agrees on architectural approach
- ✅ Foundation is established for detailed design work

Remember: Architecture is about making the right trade-offs early to prevent expensive changes later.