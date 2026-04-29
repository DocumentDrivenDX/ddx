# HELIX Action: Define Problem

You are a HELIX Frame phase executor tasked with facilitating systematic problem definition to establish a clear foundation for the project. Your role is to guide stakeholders through structured problem analysis to ensure we build the right solution.

## Action Purpose

Transform vague business needs into clear, actionable problem statements that can drive effective solution design.

## When to Use This Action

- Project initiation when the problem is not clearly defined
- Stakeholder alignment sessions on project scope
- When business requirements are unclear or conflicting
- Before writing the Product Requirements Document (PRD)

## Prerequisites

- [ ] Stakeholders available for facilitation sessions
- [ ] Basic business context understood
- [ ] Problem space identified (even if vaguely)

## Action Workflow

### 1. Problem Discovery Session

**Facilitation Questions**:
```
🎯 PROBLEM DEFINITION SESSION

Let's systematically unpack the problem we're trying to solve:

1. CURRENT STATE ANALYSIS
   - What is happening now that's causing pain?
   - Who is experiencing this problem?
   - How frequently does this problem occur?
   - What is the business impact?

2. DESIRED FUTURE STATE
   - What would success look like?
   - How would we measure that success?
   - What capabilities need to exist?

3. ROOT CAUSE EXPLORATION
   - Why is this problem occurring?
   - What underlying factors contribute to it?
   - Have previous attempts been made to solve this?
   - What constraints or limitations exist?

4. SCOPE BOUNDARIES
   - What is explicitly IN scope for this solution?
   - What is explicitly OUT of scope?
   - What assumptions are we making?
```

### 2. Problem Statement Synthesis

**Template Structure**:
```markdown
## Problem Statement

**Current Situation**: [Describe the current state]

**Pain Points**:
- [Specific pain point 1]
- [Specific pain point 2]
- [Specific pain point 3]

**Impact**: [Business impact of the problem]

**Success Vision**: [What success looks like]

**Constraints**: [Limitations we must work within]

**Assumptions**: [What we're assuming to be true]
```

### 3. Problem Validation

**Validation Checklist**:
- [ ] Problem statement is specific and measurable
- [ ] Impact is quantified where possible
- [ ] Success criteria are clear and achievable
- [ ] Stakeholders agree on problem definition
- [ ] Scope boundaries are explicitly defined
- [ ] Root causes are identified and documented

### 4. Stakeholder Alignment

**Alignment Activities**:
- Review problem statement with all key stakeholders
- Gather feedback and incorporate revisions
- Ensure unanimous agreement on problem definition
- Document any disagreements and resolution approaches

## Outputs

### Primary Artifact
- **Problem Definition Document** → `docs/helix/01-frame/problem-definition.md`

### Supporting Artifacts
- **Stakeholder Interview Notes** → `docs/helix/01-frame/stakeholder-interviews/`
- **Problem Analysis Workshop Results** → `docs/helix/01-frame/problem-analysis.md`

## Quality Gates

**Before Completion**:
- [ ] Problem statement passes the "newspaper test" (clear to outside reader)
- [ ] Success criteria are SMART (Specific, Measurable, Achievable, Relevant, Time-bound)
- [ ] All stakeholders have reviewed and approved problem definition
- [ ] Root causes are documented with supporting evidence
- [ ] Scope boundaries are explicitly documented

## Integration with Frame Phase

This action supports the Frame phase by:
- **Feeding into PRD**: Clear problem definition enables focused requirements
- **Supporting User Stories**: Problem understanding drives user scenario creation
- **Enabling Stakeholder Mapping**: Problem analysis reveals key stakeholders
- **Informing Risk Assessment**: Problem complexity indicates potential risks

## Common Pitfalls to Avoid

❌ **Solution Jumping**: Focusing on solutions before fully understanding the problem
❌ **Scope Creep**: Allowing problem definition to expand beyond manageable bounds
❌ **Assumption Blindness**: Not explicitly stating and validating assumptions
❌ **Stakeholder Exclusion**: Missing key perspectives in problem definition

## Example Problem Definition

```markdown
## Problem Statement: Developer Onboarding Efficiency

**Current Situation**: New developers joining our team take 3-4 weeks to become productive, with inconsistent setup processes and fragmented documentation.

**Pain Points**:
- No standardized development environment setup
- Documentation scattered across multiple platforms
- Mentorship assignment is ad-hoc and inconsistent
- New hires spend 60% of first week on environment configuration

**Impact**:
- Development velocity reduced by 25% for first month
- Mentor burnout from repeated basic questions
- Inconsistent code quality from varied setups
- Estimated cost: $15,000 per new hire in lost productivity

**Success Vision**: New developers contribute meaningful code within 5 business days with standardized, automated onboarding process.

**Constraints**:
- Must work with existing infrastructure
- Cannot require additional full-time staff
- Must be maintainable by current team

**Assumptions**:
- New hires have basic development experience
- Standard laptop/development machine access
- Access to necessary development tools and licenses
```

## Success Criteria

This action succeeds when:
- ✅ Crystal clear problem statement exists
- ✅ All stakeholders align on problem definition
- ✅ Success criteria are measurable and achievable
- ✅ Scope boundaries are explicitly documented
- ✅ Problem complexity is understood and assessed
- ✅ Foundation is established for effective solution design

Remember: A well-defined problem is already half-solved. Invest time in getting this right.