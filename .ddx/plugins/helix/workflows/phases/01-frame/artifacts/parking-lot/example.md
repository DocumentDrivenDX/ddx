---
dun:
  id: helix.parking-lot
  parking_lot: true
---
# Parking Lot (Deferred / Future Work)

## Purpose
Track deferred and future work without cluttering core HELIX artifacts.

## Policy
- Out of Scope items do not belong here.
- Deferred items must include a rationale and revisit trigger.
- Future items must include a source and expected value.
- Any parked artifact must set `dun.parking_lot: true`.

## Deferred / Future Items

### Just-in-Time Provisioning for SSO
- **Type**: Deferred
- **Artifact Type**: Feature Spec
- **Source**: FEAT-042
- **Rationale**: Reduce auth scope for MVP
- **Impact if Omitted**: Manual provisioning for enterprise customers
- **Dependencies**: SSO base integration
- **Revisit Trigger**: Enterprise onboarding pipeline active
- **Target Phase/Milestone**: Post-MVP
- **Artifact File**: docs/helix/01-frame/features/FEAT-042-sso-jit-provisioning.md
- **Link/Owner**: #142 / Identity Team
- **Last Reviewed**: 2026-01-15

### Secrets Rotation Policy
- **Type**: Future
- **Artifact Type**: ADR
- **Source**: ADR-017
- **Rationale**: Needs infra work and key management decision
- **Impact if Omitted**: Elevated operational risk
- **Dependencies**: KMS provider selection
- **Revisit Trigger**: KMS decision approved
- **Target Phase/Milestone**: Q3 hardening
- **Artifact File**: docs/helix/02-design/adr/ADR-017-secrets-rotation.md
- **Link/Owner**: #189 / Platform Team
- **Last Reviewed**: 2026-01-15

### Support-Driven Audit Trail Export
- **Type**: Future
- **Artifact Type**: Feature Spec
- **Source**: Support ticket #8891
- **Rationale**: Requires data retention policy alignment
- **Impact if Omitted**: Manual exports remain a support burden
- **Dependencies**: Data retention policy update
- **Revisit Trigger**: Policy approved by legal
- **Target Phase/Milestone**: 2026-H2
- **Artifact File**: docs/helix/01-frame/features/FEAT-051-audit-trail-export.md
- **Link/Owner**: #8891 / Support Ops
- **Last Reviewed**: 2026-01-15

## Parked Artifacts (Links)

### FEAT-042 SSO JIT Provisioning
- **Artifact File**: docs/helix/01-frame/features/FEAT-042-sso-jit-provisioning.md
- **Status**: Parking Lot (Deferred)

### ADR-017 Secrets Rotation
- **Artifact File**: docs/helix/02-design/adr/ADR-017-secrets-rotation.md
- **Status**: Parking Lot (Future)
