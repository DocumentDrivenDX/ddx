# Alignment Report Structure

Use this section order:

1. Review Metadata
2. Scope and Governing Artifacts
3. Intent Summary
4. Planning Stack Findings
5. Implementation Map
6. Acceptance Criteria Status
7. Gap Register (with Quality Findings)
8. Traceability Matrix
9. Review Issue Summary
10. Execution Issues Generated
11. Issue Coverage Verification
12. Execution Order
13. Open Decisions

## Required Tables

### Acceptance Criteria Status

| Story / Feature | Criterion | Test Reference | Status | Evidence |
|-----------------|-----------|----------------|--------|----------|
| [US-XXX] | [criterion] | [test path or "none"] | [SATISFIED/TESTED_NOT_PASSING/UNTESTED/UNIMPLEMENTED] | [refs] |

### Gap Register

| Area | Classification | Planning Evidence | Implementation Evidence | Resolution Direction | Review Issue | Notes |
|------|----------------|-------------------|--------------------------|----------------------|-------------|-------|

### Traceability Matrix

| Vision Item | Requirement | Feature / Story | Architecture / ADR | Solution / Technical Design | Test Reference | Implementation Plan | Code Status | Classification |
|-------------|-------------|-----------------|--------------------|-----------------------------|----------------|---------------------|-------------|----------------|

### Review Issue Summary

| Review Issue | Functional Area | Status | Key Findings | Recommended Direction |
|-------------|-----------------|--------|--------------|-----------------------|

### Execution Issues Generated

| Issue ID | Type | HELIX Labels | Parent / Source | Goal | Dependencies | Verification |
|---------|------|--------------|-----------------|------|--------------|-------------|

### Issue Coverage Verification

| Gap / Criterion | Covering Issue | Status |
|-----------------|---------------|--------|
| [gap or criterion] | [issue ID or "MISSING"] | [covered/missing/deferred] |
