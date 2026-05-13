---
ddx:
  id: "[artifact-id]"
---

# Technical Design: TD-XXX-[story-name]

**User Story**: [[US-XXX]] | **Feature**: [[FEAT-XXX]] | **Solution Design**: [[SD-XXX]]

## Scope

- Story-level design artifact
- Use for one vertical slice or one bounded implementation story
- Must inherit the broader approach from the parent solution design
- Do not redefine cross-component architecture here; that belongs in `SD-XXX`
- Governing artifacts: [User Story, Solution Design, Contracts, Concerns]

## Acceptance Criteria

1. **Given** [precondition], **When** [action], **Then** [expected outcome]
2. **Given** [precondition], **When** [action], **Then** [expected outcome]

## Technical Approach

**Strategy**: [Brief description]

**Key Decisions**:
- [Decision]: [Rationale]

**Trade-offs**:
- [What we gain vs. lose]

## Component Changes

### Modified: [Component Name]
- **Current State**: [What exists]
- **Changes**: [What changes]
- **Files**: `[path]`

### New: [Component Name]
- **Purpose**: [Why needed]
- **Interfaces**: Input: [receives] / Output: [produces]
- **Files**: `[path]`

## API/Interface Design

```yaml
endpoint: /api/v1/[resource]
method: POST
request:
  type: object
  properties:
    field1: string
response:
  type: object
  properties:
    id: string
    status: string
```

## Data Model Changes

```sql
-- New tables or schema modifications
CREATE TABLE [table_name] (
    id UUID PRIMARY KEY,
    [columns]
);
```

## Integration Points

| From | To | Method | Data |
|------|-----|--------|------|
| [Source] | [Target] | [REST/Event/Direct] | [What data] |

### External Dependencies
- **[Service]**: [Usage] | Fallback: [If unavailable]

## Security

- **Authentication**: [Required auth level]
- **Authorization**: [Required permissions]
- **Data Protection**: [Encryption/masking]
- **Threats**: [Specific threats and mitigations]

## Performance

- **Expected Load**: [Requests/sec, data volume]
- **Response Target**: [Milliseconds]
- **Optimizations**: [Caching, indexing, etc.]

## Testing

- [ ] **Unit**: [What to test]
- [ ] **Integration**: [What integrations to test]
- [ ] **API**: [Endpoints to test]
- [ ] **Security**: [Security scenarios]

## Migration & Rollback

- **Backward Compatibility**: [Strategy]
- **Data Migration**: [Required migrations]
- **Feature Toggle**: [Enable/disable mechanism]
- **Rollback**: [Steps to reverse]

## Implementation Sequence

1. [What to build first] -- Files: `[paths]` -- Tests: `[paths]`
2. [What to build next]
3. [Integration and verification]

**Prerequisites**: [Dependencies that must be complete first]

## Risks

| Risk | Prob | Impact | Mitigation |
|------|------|--------|------------|
| [Risk] | H/M/L | H/M/L | [Strategy] |

## Review Checklist

Use this checklist when reviewing a technical design:

- [ ] Acceptance criteria use Given/When/Then format and are verifiable
- [ ] Technical approach inherits from the parent solution design — no contradictions
- [ ] Key decisions have documented rationale
- [ ] Trade-offs are explicit — what we gain and what we lose
- [ ] Component changes clearly describe current state vs. changes
- [ ] API/interface design includes request and response schemas
- [ ] Data model changes include migration SQL
- [ ] Integration points specify fallback behavior for external dependencies
- [ ] Security section addresses authentication, authorization, and data protection
- [ ] Performance targets are numeric with specific metrics
- [ ] Testing section covers unit, integration, API, and security scenarios
- [ ] Migration and rollback strategy is documented
- [ ] Implementation sequence is ordered with file paths and test paths
- [ ] Design is consistent with governing solution design and feature spec
