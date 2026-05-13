---
ddx:
  id: example.parking-lot.depositmatch
  parking_lot: true
  depends_on:
    - example.feature-registry.depositmatch
---

# Parking Lot (Deferred / Future Work)

## Purpose

Track DepositMatch work that may matter later without letting it distort the
CSV-first pilot scope.

## Policy

- Rejected items do not belong here; close or cancel them instead.
- Active pilot work does not belong here; track it in the Feature Registry and
  DDx.
- Deferred items must include rationale, owner, and revisit trigger.
- Future items must include source and expected value.
- Any parked artifact must set `ddx.parking_lot: true`.

## Deferred / Future Items

### Bank Feed Integration

- **Type**: Deferred
- **Artifact Type**: Feature Spec
- **Source**: Feasibility Study alternative analysis
- **Rationale**: Higher integration and compliance surface would slow the
  CSV-first pilot.
- **Impact if Omitted**: Pilot users continue exporting CSVs manually.
- **Dependencies**: Pilot proves time savings and willingness to pay.
- **Revisit Trigger**: At least 3 of 5 pilot firms convert at target pricing
  and request bank-feed support.
- **Target Phase/Milestone**: Post-pilot
- **Owner**: Product Lead
- **Last Reviewed**: 2026-05-12

### Accounting Ledger Writeback

- **Type**: Future
- **Artifact Type**: Solution Design
- **Source**: Product Vision not-in-scope boundary
- **Rationale**: Writeback changes the trust, liability, and integration model;
  the pilot only needs review and export.
- **Impact if Omitted**: Reviewers must manually apply approved outcomes in
  their accounting system.
- **Dependencies**: Security architecture, compliance review, and integration
  partner selection.
- **Revisit Trigger**: Pilot customers complete review-log export workflow but
  cite manual ledger update as a blocker to renewal.
- **Target Phase/Milestone**: Post-pilot discovery
- **Owner**: Product / Engineering
- **Last Reviewed**: 2026-05-12

### Automatic Approval

- **Type**: Deferred
- **Artifact Type**: ADR
- **Source**: Opportunity Canvas scope boundary
- **Rationale**: The pilot differentiates on reviewer trust, not invisible
  automation.
- **Impact if Omitted**: Reviewers must approve suggested matches explicitly.
- **Dependencies**: Match accuracy evidence, compliance review, customer risk
  tolerance, and audit-log design.
- **Revisit Trigger**: Accepted suggestion accuracy exceeds 98% for two months
  and pilot firms request supervised automation.
- **Target Phase/Milestone**: Future trust-model review
- **Owner**: Product / Compliance
- **Last Reviewed**: 2026-05-12

## Parked Artifacts (Links)

### FEAT Future Bank Feed Integration

- **Artifact File**: `docs/helix/01-frame/features/FEAT-005-bank-feed-integration.md`
- **Status**: Parking Lot (Deferred)

### ADR Future Automatic Approval

- **Artifact File**: `docs/helix/02-design/adr/ADR-002-automatic-approval.md`
- **Status**: Parking Lot (Deferred)
