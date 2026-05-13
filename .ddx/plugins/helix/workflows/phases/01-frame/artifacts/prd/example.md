---
ddx:
  id: example.prd.depositmatch
  depends_on:
    - example.product-vision.depositmatch
---
# Product Requirements Document

## Summary

DepositMatch is a reconciliation workspace for small bookkeeping firms. It
imports bank deposits and invoice exports, suggests likely matches, and gives
reviewers an exception queue for deposits that need human judgment. The first
release targets weekly reconciliation for firms serving recurring
small-business clients. Success means reviewers can close most clients in
minutes, trust the evidence behind accepted matches, and keep unresolved
deposits from disappearing into spreadsheets or email.

## Problem and Goals

### Problem

Bookkeeping firms with growing client rosters spend 4-8 hours each week
matching bank deposits to invoices across accounting exports, bank portals,
spreadsheets, and email threads. The work is repetitive, but mistakes are
expensive: a missed split payment or duplicate invoice can delay monthly close
and trigger client follow-up days later.

### Goals

1. Reviewers can reconcile routine deposits from one workspace.
2. Every accepted match has visible evidence and reviewer attribution.
3. Unclear deposits become owned exceptions with a next action.

### Success Metrics

| Metric | Target | Measurement Method |
|--------|--------|--------------------|
| Median reconciliation time | Under 3 minutes per client per week | In-product workflow timing |
| Suggestion acceptance accuracy | 95% of accepted suggestions remain accepted in weekly audit sample | Reviewer audit |
| Exception ownership | 90% of unresolved deposits have owner and next action within one business day | Exception queue report |

### Non-Goals

- Replacing QuickBooks, Xero, or the firm's general ledger.
- Automatically posting journal entries.
- Supporting payroll, inventory, or tax workflows.
- Making irreversible match decisions without reviewer approval.

Deferred items tracked in `docs/helix/parking-lot.md`.

## Users and Scope

### Primary Persona: Maya, Reconciliation Lead

**Role**: Senior bookkeeper responsible for weekly reconciliation across 10-20
small-business clients.
**Goals**: Finish routine matching quickly, catch exceptions early, and leave a
clear audit trail for month-end review.
**Pain Points**: Rebuilding context across exports, losing client follow-up in
email, and repeating the same manual comparisons every week.

### Secondary Persona: Andre, Firm Owner

**Role**: Owner of a 12-person bookkeeping firm.
**Goals**: Increase client capacity without hiring another reviewer and reduce
month-end surprises.
**Pain Points**: Spreadsheet-based processes do not scale, and quality depends
too heavily on individual reviewer habits.

## Requirements

Each requirement traces to the Product Vision goal of reducing routine weekly
reconciliation time while preserving reviewer trust and exception ownership.

### Must Have (P0)

1. Import bank deposit CSV files and invoice export CSV files for a client.
2. Generate match suggestions using amount, date, payer, and invoice metadata.
3. Require reviewer approval before a suggested match becomes accepted.
4. Preserve match evidence, reviewer, timestamp, and source rows.
5. Create an exception for every unmatched or low-confidence deposit.

### Should Have (P1)

1. Support split deposits that pay multiple invoices.
2. Export a client-level reconciliation report.
3. Assign exception owners and due dates.

### Nice to Have (P2)

1. Bank feed integration.
2. Accounting platform API sync.
3. Client-facing question portal.

## Functional Requirements

### Import

- The system accepts CSV uploads for bank deposits and invoice exports.
- The user maps required columns on first import for each client.
- The system rejects files missing amount, date, and identifier columns.

### Match Review

- The system suggests matches with a confidence label and evidence summary.
- The reviewer can accept, reject, split, or flag each suggestion.
- Accepted matches are immutable except through a recorded correction.

### Exceptions

- The system creates an exception for every deposit without an accepted match.
- Each exception has status, owner, next action, and due date.
- Reviewers can export exceptions by client.

## Acceptance Test Sketches

| Requirement | Scenario | Input | Expected Output |
|-------------|----------|-------|-----------------|
| Import CSV files | Reviewer uploads bank and invoice exports | Two valid CSV files for one client | Imported deposits and invoices appear in review queue |
| Generate suggestions | Deposit amount and payer match open invoice | Deposit for 1200.00 from Acme Dental; invoice INV-104 for 1200.00 | High-confidence suggestion links deposit to invoice |
| Require approval | Reviewer views suggested match | Suggested match with evidence | Match remains pending until reviewer accepts |
| Preserve evidence | Reviewer accepts suggestion | Accepted match | Audit log records source rows, reviewer, timestamp, and evidence |
| Create exceptions | Deposit has no likely invoice | Deposit for 847.13 with no matching invoice | Exception is created with status `needs-review` |

## Technical Context

- **Language/Runtime**: TypeScript 5.x on Node 20+
- **Key Libraries**: React 18 for UI, Fastify 5 for API, Papa Parse for CSV
- **Data/Storage**: PostgreSQL 16
- **APIs**: Internal REST API; no external accounting API in v1
- **Platform Targets**: Modern desktop browsers; Chrome, Edge, Firefox, Safari

## Constraints, Assumptions, Dependencies

### Constraints

- **Technical**: CSV import is the only v1 data ingestion path.
- **Business**: First release must support a firm with up to 25 active clients.
- **Legal/Compliance**: Customer financial data must be encrypted at rest and
  excluded from analytics events.

### Assumptions

- Firms can export invoice data from their current accounting system.
- Weekly reconciliation is the first workflow worth optimizing.
- Reviewers will trust suggestions only when evidence is visible.

### Dependencies

- Sample CSV exports from at least three accounting systems.
- Security review for financial-data handling.
- Firm owner approval of audit-log retention policy.

## Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| CSV formats vary too much across clients | High | Medium | Add per-client column mapping and save mappings after first import |
| Suggestions look opaque and reviewers reject them | Medium | High | Show amount, date, payer, and invoice evidence beside every suggestion |
| Split payments are common enough to block adoption | Medium | Medium | Include split deposit support as P1 before paid launch |

## Open Questions

- [ ] Which three accounting exports define the v1 CSV compatibility set? - ask pilot firms.
- [ ] What audit-log retention period do firms require? - ask firm owners and legal reviewer.
- [ ] Should low-confidence suggestions appear in review or go straight to exceptions? - ask pilot reviewers.

## Success Criteria

DepositMatch is successful when pilot firms reconcile routine weekly deposits
from one workspace, reviewers accept at least 95% of audited suggestions, and
unresolved deposits consistently leave a named owner and next action.
