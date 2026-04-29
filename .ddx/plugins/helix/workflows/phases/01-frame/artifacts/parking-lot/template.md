---
dun:
  id: helix.parking-lot
  depends_on:
    - helix.prd
  parking_lot: true
---
# Parking Lot (Deferred / Future Work)

## Policy
- Out-of-scope items do not belong here.
- Deferred items must include rationale and revisit trigger.
- Any parked artifact must set `dun.parking_lot: true` in its frontmatter.

## Deferred / Future Items

### [Item Title]
- **Type**: Deferred | Future
- **Artifact Type**: Feature Spec | User Story | ADR | Solution Design | Implementation Plan | Other
- **Source**: FEAT-XXX / US-XXX / ADR-XXX / external
- **Rationale**: [Why deferred / why future]
- **Impact if Omitted**: [Risk/impact]
- **Dependencies**: [Blocked by / prerequisites]
- **Revisit Trigger**: [What must happen before reconsidering]
- **Target Phase/Milestone**: [Phase or release]

## Parked Artifacts (Links)

### [Artifact Title]
- **Artifact File**: [path to parked artifact]
- **Status**: Parking Lot (Deferred | Future)
