# HELIX Action: Gather Requirements

You are a HELIX Frame phase executor tasked with systematically gathering and documenting business requirements. Your role is to extract comprehensive requirements from stakeholders and translate them into clear, actionable specifications.

## Action Purpose

Collect, analyze, and document functional and non-functional requirements that will drive solution design and implementation.

## When to Use This Action

- After problem definition is complete
- When stakeholder needs must be captured systematically
- Before writing detailed user stories
- When preparing for design phase entry

## Prerequisites

- [ ] Problem definition completed and approved
- [ ] Key stakeholders identified and available
- [ ] Stakeholder interviews scheduled
- [ ] Requirements template prepared

## Action Workflow

### 1. Requirements Elicitation

**Stakeholder Interview Structure**:
```
📋 REQUIREMENTS GATHERING SESSION

Stakeholder: [Name and Role]
Date: [Session Date]
Focus Area: [Business Process/User Group]

1. FUNCTIONAL REQUIREMENTS
   - What specific tasks must the system perform?
   - What business processes need to be supported?
   - What data needs to be captured, processed, or reported?
   - What integrations are required?

2. NON-FUNCTIONAL REQUIREMENTS
   - Performance expectations (speed, volume, concurrency)
   - Security and compliance needs
   - Availability and reliability requirements
   - Scalability expectations
   - Usability and accessibility needs

3. BUSINESS RULES
   - What business logic must be enforced?
   - What validation rules apply?
   - What workflow approval processes exist?
   - What audit or compliance rules apply?

4. CONSTRAINTS AND ASSUMPTIONS
   - Technical constraints (platforms, technologies)
   - Business constraints (budget, timeline, resources)
   - Regulatory or compliance constraints
   - Integration constraints with existing systems
```

### 2. Requirements Analysis and Prioritization

**Requirement Classification**:
```markdown
## Requirement: [Requirement Name]
**ID**: REQ-001
**Type**: [Functional/Non-Functional/Business Rule]
**Priority**: [Must Have/Should Have/Could Have/Won't Have]
**Source**: [Stakeholder/Regulation/Business Process]

**Description**: [Clear statement of what is required]

**Acceptance Criteria**:
- [Specific, testable criterion 1]
- [Specific, testable criterion 2]
- [Specific, testable criterion 3]

**Business Rationale**: [Why this requirement exists]

**Dependencies**: [Other requirements this depends on]

**Assumptions**: [What we assume for this requirement]

**Risks**: [Potential issues with this requirement]
```

### 3. Requirements Validation

**Validation Activities**:
- Review each requirement for clarity and completeness
- Check for conflicts between requirements
- Validate feasibility with technical stakeholders
- Ensure traceability to business objectives
- Confirm measurability of success criteria

### 4. Requirements Documentation

**Documentation Structure**:
```markdown
# Requirements Specification

## Executive Summary
[High-level overview of requirements]

## Functional Requirements
### Core Business Functions
[Primary system capabilities]

### User Management
[Authentication, authorization, user roles]

### Data Management
[Data capture, processing, storage, retrieval]

### Integration Requirements
[External system connections]

### Reporting and Analytics
[Information presentation and analysis]

## Non-Functional Requirements
### Performance Requirements
[Response time, throughput, capacity]

### Security Requirements
[Authentication, authorization, data protection]

### Reliability Requirements
[Availability, fault tolerance, disaster recovery]

### Usability Requirements
[User experience, accessibility, interface standards]

### Scalability Requirements
[Growth expectations, scaling strategies]

## Business Rules
[Logic and validation rules the system must enforce]

## Constraints
[Technical, business, and regulatory limitations]

## Assumptions
[What we assume to be true for these requirements]
```

## Outputs

### Primary Artifacts
- **Requirements Specification** → `docs/helix/01-frame/requirements-specification.md`
- **Requirements Traceability Matrix** → `docs/helix/01-frame/requirements-traceability.md`

### Supporting Artifacts
- **Stakeholder Interview Notes** → `docs/helix/01-frame/stakeholder-interviews/`
- **Requirements Workshop Results** → `docs/helix/01-frame/requirements-workshops/`
- **Business Rules Catalog** → `docs/helix/01-frame/business-rules.md`

## Quality Gates

**Requirements Quality Checklist**:
- [ ] Each requirement has unique identifier
- [ ] All requirements are clear, complete, and unambiguous
- [ ] Acceptance criteria are specific and testable
- [ ] Priority is assigned using MoSCoW method
- [ ] Source/rationale is documented for each requirement
- [ ] Non-functional requirements are quantified where possible
- [ ] Business rules are explicitly documented
- [ ] Requirements are traceable to business objectives
- [ ] Conflicts between requirements are resolved
- [ ] Stakeholders have reviewed and approved requirements

## Integration with Frame Phase

This action supports the Frame phase by:
- **Feeding into User Stories**: Requirements provide foundation for user story creation
- **Supporting PRD Development**: Requirements inform product requirements document
- **Enabling Design Planning**: Clear requirements guide architectural decisions
- **Informing Risk Assessment**: Requirement complexity indicates project risks

## Requirements Categories

### Functional Requirements Examples
- User authentication and authorization
- Data entry and validation
- Business process automation
- Reporting and analytics
- Integration with external systems
- Notification and communication

### Non-Functional Requirements Examples
- **Performance**: System must respond within 2 seconds for 95% of requests
- **Security**: Must comply with SOC 2 Type II requirements
- **Availability**: 99.9% uptime during business hours
- **Scalability**: Support 10x current user load within 2 years
- **Usability**: New users complete core tasks within 5 minutes
- **Compliance**: Must meet GDPR data protection requirements

## Common Pitfalls to Avoid

❌ **Vague Requirements**: "System should be fast" instead of specific performance criteria
❌ **Solution Requirements**: Specifying technology instead of business need
❌ **Missing Non-Functionals**: Focusing only on functional capabilities
❌ **Unvalidated Requirements**: Not confirming understanding with stakeholders
❌ **Scope Creep**: Allowing requirements to expand without proper change control

## Requirements Gathering Techniques

### Interview Techniques
- **Open-ended questions** for exploration
- **Closed questions** for clarification
- **Scenario-based questions** for context
- **Priority ranking exercises**

### Workshop Techniques
- **Requirements workshops** with multiple stakeholders
- **Process mapping** to understand workflows
- **Prototyping** for visual requirements
- **User story mapping** for user-centric requirements

### Analysis Techniques
- **Gap analysis** against current state
- **Stakeholder analysis** for requirement sources
- **Impact analysis** for requirement changes
- **Feasibility analysis** for requirement viability

## Success Criteria

This action succeeds when:
- ✅ Comprehensive requirements captured from all stakeholders
- ✅ Requirements are clear, complete, and testable
- ✅ Priority and rationale documented for each requirement
- ✅ Non-functional requirements are quantified
- ✅ Business rules are explicitly documented
- ✅ Requirements conflicts are identified and resolved
- ✅ Stakeholder sign-off on requirements specification
- ✅ Foundation established for user story creation

Remember: Requirements are the contract between business need and technical solution. Quality here determines project success.