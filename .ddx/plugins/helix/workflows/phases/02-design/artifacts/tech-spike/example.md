---
ddx:
  id: example.tech-spike.depositmatch
  depends_on:
    - example.product-vision.depositmatch
    - example.data-design.depositmatch
    - example.security-architecture.depositmatch
---

# Technical Spike: CSV Formula Neutralization

**Spike ID**: SPIKE-001 | **Lead**: Platform engineer | **Time Budget**: 1 day | **Status**: Completed

## Objective

**Technical Question**: Can DepositMatch safely import and re-export client bank
CSV data without allowing spreadsheet formula injection to survive into exports?

**Goals**:
- [x] Identify the risky cell prefixes and encodings common in CSV injection.
- [x] Test whether import-time normalization alone is enough.
- [x] Compare import-time neutralization with export-time neutralization.

**Success Criteria**: Evidence from fixture files shows which control prevents
formula execution in exported review logs.

**Out of Scope**: Full CSV parser replacement, production import UI, antivirus
scanning, and complete bank-format coverage.

## Hypothesis

**Primary**: Export-time neutralization is required because source values may be
stored for auditability and later exported in a different context.

**Assumptions**:
- Source CSVs are untrusted restricted data.
- Reviewers may open exported logs in spreadsheet software.
- The pilot only needs UTF-8 CSV fixtures from three target bank exports.

**Expected Outcome**: Keep raw source values restricted for audit references,
store normalized values for matching, and neutralize risky cells at every CSV
export boundary.

## Approach

**Method**: Minimal implementation with malicious fixtures.

**Activities**:
| Day | Activity | Objective |
|-----|----------|-----------|
| 1 | Build fixture CSVs with `=`, `+`, `-`, `@`, tab-prefixed, and CR-prefixed values | Exercise known formula-entry patterns |
| 1 | Run import normalization and export generation with and without neutralization | Compare control placement |
| 1 | Open outputs in spreadsheet software and inspect stored values | Confirm whether formulas execute |

## Findings

**FINDING 1**: Import-time schema validation is necessary but insufficient.
- **Evidence**: The parser rejected malformed rows and unsupported encodings,
  but valid text fields containing `=cmd`-style values still remained valid
  strings after normalization.
- **Implications**: Validation protects parser behavior. It does not by itself
  protect downstream spreadsheet interpretation.

**FINDING 2**: Export-time neutralization prevented formula execution in all
pilot fixtures.
- **Evidence**: Prefixing risky exported cells with a single quote caused the
  test spreadsheets to display the value as text for all six fixture patterns.
- **Implications**: Every CSV export path needs the same safe-cell function.

**FINDING 3**: Mutating raw source values at import would weaken auditability.
- **Evidence**: When raw values were rewritten during import, support could no
  longer compare the normalized record against the original bank export without
  a separate source attachment.
- **Implications**: Keep raw source data restricted and retained according to
  policy. Neutralize only when writing customer-controlled CSV output.

### Measurements

| Metric | Import Neutralization | Export Neutralization | Notes |
|--------|-----------------------|-----------------------|-------|
| Blocks formula execution in export fixtures | Yes | Yes | Both worked for tested patterns |
| Preserves raw source evidence | No | Yes | Raw import mutation loses fidelity |
| Centralizes customer-output control | No | Yes | Export helper covers review-log downloads |
| Requires every export path to opt in | No | Yes | Add test coverage for all CSV exporters |

## Analysis

**Hypothesis**: CONFIRMED.
**Rationale**: Formula risk appears when restricted stored values cross into a
customer-controlled spreadsheet context. Export-time neutralization protects
that boundary while preserving raw source evidence for audit and support.

### Risks

| Risk | Prob | Impact | Mitigation |
|------|------|--------|------------|
| Future CSV export bypasses the safe-cell helper | Medium | High | Add a shared export helper and test every CSV exporter |
| Bank-specific encodings introduce new edge cases | Medium | Medium | Expand fixture set when Research Plan sample intake completes |
| Spreadsheet behavior varies by tool/version | Low | Medium | Document tested tools and keep fixture-based regression tests |

## Conclusions

**Primary Conclusion**: DepositMatch should preserve raw source values in
restricted storage and neutralize risky cells at every CSV export boundary.

**Confidence**: Medium.

**Limitations**: The spike used pilot fixture files and two spreadsheet tools.
It did not exhaustively test every bank export format or spreadsheet version.

## Recommendations

**RECOMMENDATION**: Add a shared CSV export helper that neutralizes cells
beginning with formula-risk prefixes, require all CSV exports to use it, and add
security tests for the malicious fixture set.

- **Rationale**: The control sits at the trust boundary where spreadsheet
  interpretation becomes possible and preserves source-data auditability.
- **Next Steps**: Update Security Architecture control mapping, add
  `security-tests` coverage for malicious CSV fixtures, and create a Technical
  Design task for the shared export helper.
- **Concern Impact**: Reinforces the security concern that source CSVs are
  untrusted restricted data. No ADR is needed unless the team chooses to mutate
  source values at import instead.

## Artifacts

- `fixtures/csv/formula-injection/*.csv`: malicious CSV fixture set.
- `notes/csv-export-neutralization.md`: spreadsheet behavior observations and
  tested tool versions.
- `prototype/safe_csv_export.rb`: throwaway export helper used to compare
  neutralization strategies.
