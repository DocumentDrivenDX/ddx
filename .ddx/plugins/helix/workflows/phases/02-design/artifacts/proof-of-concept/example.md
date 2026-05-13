---
ddx:
  id: example.proof-of-concept.depositmatch
  depends_on:
    - example.feasibility-study.depositmatch
    - example.data-design.depositmatch
    - example.security-requirements.depositmatch
---

# Proof of Concept: CSV Import and Evidence-Backed Match Review

**PoC ID**: POC-001 | **Lead**: Engineering Lead | **Time Budget**: 5 days | **Status**: Completed

## Objective

**Primary Question**: Can DepositMatch import representative bank and invoice
CSVs, normalize records by firm/client, suggest matches with visible evidence,
and preserve reviewer decisions without implementing bank feeds or ledger
writeback?

**Success Criteria**:

- **Functional**: WORKING: Import two CSV files, normalize records, generate
  match suggestions, require reviewer approval, and record decisions.
- **Performance**: VALIDATED: Process 500 deposits and 500 invoices for one
  client in under 10 seconds on a local development machine.
- **Integration**: VALIDATED: Source files, normalized records, suggestions,
  review decisions, and exports follow the Data Design relationships.
- **Security**: VALIDATED: Firm/client scoping is enforced in API calls and no
  raw financial identifiers appear in telemetry fixtures.

**In Scope**: CSV parsing, column mapping, normalized records, deterministic
matching rules, evidence display payload, reviewer approval/rejection, and
audit-log write.

**Out of Scope**: Production UI polish, bank-feed integrations, accounting
writeback, ML matching, support tooling, and live customer data.

## Approach

**Architecture Pattern**: Thin vertical workflow from CSV upload through
review decision, using the pilot Data Design and Security Requirements.

**Key Technologies**:

- **Primary**: Local API harness, PostgreSQL-compatible schema, deterministic
  matching service, fixture CSVs.
- **Integration**: Object-storage stub for source files and audit-log table for
  reviewer actions.

## Implementation

### Architecture Overview

```text
CSV fixtures
  -> import validator
  -> normalized deposit/invoice records
  -> deterministic matching service
  -> review queue payload with evidence
  -> reviewer decision endpoint
  -> append-only review_decision audit record
```

### Core Components

#### Import Validator

- **Purpose**: Validate required columns, size limits, encoding, and formula
  injection before normalization.
- **Implementation**: Schema-driven parser with per-client column mapping and
  rejected-row report.

#### Matching Service

- **Purpose**: Suggest candidate deposit-to-invoice matches with evidence.
- **Implementation**: Deterministic rules using amount equality, payer
  reference similarity, and date proximity.

#### Review Decision Endpoint

- **Purpose**: Require reviewer approval or rejection before match state
  changes.
- **Implementation**: Transaction writes suggestion status and immutable
  review_decision record with actor, timestamp, action, and source references.

### Integration Points

| Integration | Type | Status | Notes |
|-------------|------|--------|-------|
| CSV fixtures | File input | Working | Covers two representative bank/invoice export shapes |
| PostgreSQL-compatible schema | Database | Working | Uses pilot entities from Data Design |
| Object-storage stub | File/object store | Partial | Stores source-file metadata only |
| Audit log | Database table | Working | Captures reviewer actions and source refs |

## Results

### Test Scenarios

| Scenario | Result | Status |
|----------|--------|--------|
| Import valid bank and invoice CSVs | 500 deposits and 500 invoices normalized with no rejected rows | Pass |
| Import CSV with missing required column | Batch rejected with field-level error report | Pass |
| Import formula-injection value | Value neutralized before export payload generation | Pass |
| Generate suggested matches | 417 exact-amount/date-window suggestions produced with evidence payload | Pass |
| Reviewer approves match | Suggestion marked accepted and review_decision row appended | Pass |
| Cross-firm read attempt | API returns 403 for records outside firm scope | Pass |
| Performance baseline | 1,000 rows processed in 6.8 seconds locally | Pass |

### Findings

- **FINDING 1**: IMPLEMENTATION: CSV-first import is feasible for the pilot if
  per-client column mapping is explicit.
- **Evidence**: Two fixture formats normalized into the same deposit/invoice
  schema and produced a consistent review queue.
- **Implications**: FEAT-001 should include a mapping step and rejected-row
  report, not a fixed one-format parser.

- **FINDING 2**: VALIDATED: Deterministic matching is sufficient for first-pass
  pilot suggestions.
- **Evidence**: Amount/date/payer rules produced reviewable suggestions with
  evidence payloads and no automatic acceptance.
- **Implications**: ML matching is unnecessary for v1 and should remain parked.

- **FINDING 3**: WORKING: Audit-log writes can be transactionally tied to
  reviewer decisions.
- **Evidence**: Approval/rejection tests wrote suggestion state and
  review_decision rows together.
- **Implications**: Review logs can support the PR-FAQ trust model if the
  production design preserves append-only decision history.

### Risks

| Risk | Prob | Impact | Mitigation |
|------|------|--------|------------|
| Fixture CSVs are not representative enough | High | Medium | Run Research Plan sample intake before finalizing parser scope |
| Deterministic matching misses split payments | Medium | Medium | Route ambiguous deposits to exception queue; defer split support if needed |
| Import performance changes with production storage | Medium | Low | Add performance test with production-like storage before pilot launch |

## Analysis

**Overall Assessment**: VIABLE WITH CONDITIONS

**Rationale**: The PoC proves the end-to-end CSV-first workflow can work without
bank feeds or ledger writeback. Production design should proceed, but only
after research collects representative sample files and confirms the required
column mapping scope.

## Recommendations

1. Proceed with FEAT-001 CSV Import and Column Mapping -- the PoC validated the
   core path -- Design now.
2. Keep matching deterministic in v1 -- sufficient for reviewable suggestions
   and easier to explain -- Design now.
3. Add rejected-row reports and formula-injection protection to acceptance
   criteria -- both materially affect safety and supportability -- Before build.
4. Keep ML matching, bank feeds, and ledger writeback out of v1 -- not needed to
   validate the trust model -- Parking Lot.

### Follow-up

- [ ] Update FEAT-001 technical design with mapping and rejected-row behavior.
- [ ] Add CSV import security tests for formula injection and malformed files.
- [ ] Add performance test using production-like object storage and database.
- [ ] Confirm sample-file compatibility through the Research Plan before build.
