---
ddx:
  id: example.research-plan.depositmatch
  depends_on:
    - example.opportunity-canvas.depositmatch
    - example.feasibility-study.depositmatch
    - example.business-case.depositmatch
---

# Research Plan: DepositMatch Pilot Validation

**Research Lead**: Product Lead
**Time Budget**: 2 weeks
**Status**: Example

## Research Objectives

### Primary Research Questions

1. **Question**: Do bookkeeping firms with 5-25 employees spend enough weekly
   time on deposit reconciliation to make DepositMatch a top-three workflow
   problem?
   - **Why Important**: The Business Case assumes reconciliation is a capacity
     bottleneck, not a minor annoyance.
   - **Success Criteria**: At least 4 of 5 interviewed firms report weekly
     reconciliation above 3 hours and can name a recent close-cycle failure
     caused by manual matching.

2. **Question**: Can representative pilot firms provide CSV exports that are
   consistent enough for column mapping and suggested matches?
   - **Why Important**: The Feasibility Study identifies CSV variability as the
     largest technical risk.
   - **Success Criteria**: Sample files from at least 3 firms can be mapped into
     the target import schema with fewer than 2 unsupported required fields per
     firm.

3. **Question**: Will pilot firms pay for a narrow reconciliation workspace
   before bank-feed integrations or ledger writeback exist?
   - **Why Important**: The Business Case pricing and obtainable market are
     low-confidence assumptions.
   - **Success Criteria**: At least 3 of 5 pilot firms agree that $149/month is
     reasonable if the pilot reaches the stated time and accuracy targets.

### Knowledge Gaps

| Gap | Impact | Current Confidence |
|-----|--------|--------------------|
| Actual weekly reconciliation time for target firms | High | Low |
| CSV field variability across accounting systems and bank portals | High | Low |
| Willingness to pay for CSV-first workflow | High | Low |
| Which exception states reviewers need on day one | Medium | Medium |

## Scope

**In Scope**: Interviews with target firms, sample CSV collection, lightweight
workflow observation, pricing reaction, and exception-state discovery.

**Out of Scope**: Full usability testing, production onboarding, bank-feed
integration research, and accounting-ledger writeback.

**Assumptions**: Product Vision target segment and pilot scope remain stable
through the research window.

**Decision Enabled**: Whether the PRD can commit to CSV import, match review,
exception ownership, pilot pricing, and success metrics.

## Research Methods

### Target Customer Interviews

- **Objective**: Address Questions 1 and 3.
- **Approach**: Conduct five 45-minute interviews with bookkeeping firm owners
  or reconciliation leads. Focus on recent close cycles, current tools, time
  spent, failure modes, and pricing reaction.
- **Participants/Sources**: Five U.S.-based bookkeeping firms with 5-25
  employees and recurring small-business clients.
- **Duration**: 1 week for recruiting and interviews.
- **Deliverable**: Interview notes, problem-intensity summary, pricing signal.
- **Decision Use**: Confirms whether to proceed with the pilot PRD or return to
  opportunity discovery.

### CSV Sample Review

- **Objective**: Address Question 2.
- **Approach**: Ask interview participants for anonymized or synthetic samples
  matching their real bank deposit and invoice exports. Map fields into the
  target import shape and record unsupported fields.
- **Participants/Sources**: At least three firms using different accounting or
  bank export patterns.
- **Duration**: 3 days.
- **Deliverable**: Import-compatibility matrix and required field list.
- **Decision Use**: Defines FEAT-001 import requirements and feasibility risk.

### Exception Workflow Probe

- **Objective**: Identify day-one exception states for FEAT-003.
- **Approach**: Walk participants through recent unresolved deposits and ask
  what owner, next action, and evidence they needed.
- **Participants/Sources**: Same interview participants; use recent examples.
- **Duration**: Included in interviews.
- **Deliverable**: Exception-state candidates and vocabulary.
- **Decision Use**: Prevents the PRD from inventing exception states detached
  from real reviewer work.

## Timeline

| Phase | Duration | Activities | Deliverables |
|-------|----------|------------|--------------|
| Planning | 1 day | Finalize screener, interview guide, data-handling plan | Approved plan |
| Investigation | 6 days | Interviews, CSV sample collection, workflow probes | Notes and sample inventory |
| Analysis | 3 days | Synthesize findings, map CSV compatibility, summarize pricing signal | Findings summary |
| Validation | 2 days | Review with product, engineering, and compliance | PRD readiness recommendation |

**Total Duration**: 2 weeks

## Research Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Firms cannot share representative CSVs | Medium | High | Accept anonymized or synthetic samples with real column shape. |
| Interviewees overstate willingness to pay | High | Medium | Ask for pilot commitment and compare against current cost/time. |
| Sample size is too small to generalize | High | Medium | Treat findings as pilot-scope evidence only; require follow-up before scale claims. |
| Compliance review limits data collection | Medium | High | Use synthetic/anonymized samples and avoid live financial data during research. |

## Completion Criteria

- [ ] All three primary research questions answered with evidence or deferred
  with rationale.
- [ ] Interview notes summarize problem intensity and current alternatives.
- [ ] CSV compatibility matrix identifies required fields and unsupported
  cases.
- [ ] Pricing signal is explicit enough to confirm or revise the pilot Business
  Case.
- [ ] PRD readiness recommendation reviewed by product, engineering, and
  compliance.
