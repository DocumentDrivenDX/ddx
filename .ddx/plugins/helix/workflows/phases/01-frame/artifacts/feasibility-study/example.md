---
ddx:
  id: example.feasibility-study.depositmatch
  depends_on:
    - example.opportunity-canvas.depositmatch
    - example.business-case.depositmatch
    - example.compliance-requirements.depositmatch
---

# Feasibility Study: DepositMatch CSV-first Pilot

**Feasibility Lead**: Product and engineering leads
**Evaluation Timeframe**: 2 weeks
**Decision Deadline**: 2026-05-31
**Status**: Example

## Executive Summary

### Project Overview

DepositMatch is a focused reconciliation workspace for small bookkeeping firms.
The pilot scope imports CSV bank and invoice exports, suggests matches with
visible evidence, tracks reviewer decisions, and keeps unresolved deposits in a
client-scoped exception queue.

### Recommendation

**Overall Assessment**: CONDITIONALLY FEASIBLE
**Decision**: CONDITIONAL GO
**Rationale**: The CSV-first pilot is technically and operationally feasible if
the first release stays narrow. The highest risks are CSV variability, live
financial-data handling, and willingness to pay, so the project should proceed
only with pilot recruiting, compliance review, and explicit success metrics.
**Confidence**: Medium

## Feasibility Assessment

### Technical

- **Assessment**: FEASIBLE
- **Key requirements**: CSV import and mapping, suggested matching, evidence
  display, firm/client access boundaries, review log export, and exception
  queue.
- **Main risks**: CSV format variability, false-positive matches, and audit log
  integrity.
- **Evidence**: Product Vision defines a narrow workflow; Opportunity Canvas
  keeps v1 out of bank feeds and ledger writeback.

### Business

- **Assessment**: HIGH RISK
- **Market opportunity**: Small bookkeeping firms have a specific weekly
  reconciliation bottleneck, but segment size and pricing are still planning
  assumptions.
- **Value proposition**: Reviewer capacity, visible evidence, and exception
  ownership are differentiated against spreadsheets and generic matching tools.
- **Evidence**: Business Case marks TAM/SAM/SOM and pricing as assumptions;
  Opportunity Canvas requires pilot conversion evidence.

### Operational

- **Assessment**: HIGH RISK
- **Support and deployment needs**: Pilot onboarding, CSV sampling, per-firm
  mapping support, deletion requests, incident response, and support access
  controls.
- **Regulatory requirements**: FTC Safeguards and state privacy applicability
  need counsel review before live client financial data is uploaded.
- **Evidence**: Compliance Requirements identifies financial-data handling,
  retention, vendor, and counsel-review gaps.

### Resource

- **Assessment**: FEASIBLE
- **Budget**: Year-one pilot budget in Business Case: $262,000 across
  development, infrastructure, go-to-market, and operations.
- **Team and timeline**: Three-month pilot build is feasible with a focused
  product/engineering pair and limited support coverage.
- **Evidence**: Business Case bounds the first investment; Opportunity Canvas
  keeps bank feeds, ledger writeback, and automatic approval out of scope.

## Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| CSV exports vary enough to slow onboarding | High | Medium | Recruit pilots across at least three accounting systems and build explicit column mapping. |
| Reviewers distrust suggestions | Medium | High | Show evidence before approval and measure accepted suggestion accuracy. |
| Compliance review expands required controls | Medium | High | Complete legal applicability review before live-data pilot. |
| Pilot firms do not pay at target pricing | Medium | High | Validate willingness to pay before expanding beyond CSV-first scope. |

## Alternatives

### CSV-first Pilot

- **Pros**: Fastest path to validate reviewer trust, time savings, and
  willingness to pay without integration dependencies.
- **Cons**: Requires manual CSV mapping support and does not prove bank-feed
  integration value.
- **Feasibility**: CONDITIONALLY FEASIBLE
- **Decision**: Carry forward

### Bank-feed and Ledger Integration First

- **Pros**: Stronger automation story and richer transaction context.
- **Cons**: Higher integration complexity, slower learning, larger compliance
  and support surface.
- **Feasibility**: HIGH RISK
- **Decision**: Reject for v1

### Do Nothing / Delay

- **Pros**: Avoids compliance and support burden while market assumptions are
  weak.
- **Cons**: Delays learning on the core reviewer-trust problem and leaves pilot
  firms in manual spreadsheet workflows.
- **Feasibility**: FEASIBLE but strategically weak
- **Decision**: Reject

## Decision Framework

| Criterion | Status | Rationale |
|-----------|--------|-----------|
| Technical buildability | Pass | CSV-first scope is bounded and avoids complex integrations. |
| Business value | Risk | Pain is clear, but pricing and obtainable market remain assumptions. |
| Operational supportability | Risk | CSV onboarding and financial-data handling need explicit procedures. |
| Compliance readiness | Risk | Counsel review is required before live-data pilot. |
| Resource availability | Pass | Three-month pilot fits the bounded investment case. |

## Next Steps

1. Confirm five pilot firms and collect sample CSVs before finalizing PRD scope.
2. Complete legal/compliance applicability review for live financial data.
3. Turn the CSV import, evidence-backed matching, exception queue, and review
   log into PRD requirements.
4. Add pilot success gates: reconciliation time below 3 minutes per client,
   accepted suggestion accuracy above 95%, and 3 of 5 pilot firms willing to
   pay target pricing.
