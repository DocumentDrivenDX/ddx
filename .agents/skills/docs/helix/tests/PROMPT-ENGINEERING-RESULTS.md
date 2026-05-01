# Prompt Engineering Experiment Results

**Date**: 2026-04-07  
**Scenario**: A (Simple CLI - Temperature Converter)  
**Autonomy Level**: Medium  
**Model**: Claude Opus 4  

---

## Executive Summary

Two iterations of prompt refinement produced **measurable improvements** in artifact quality, completeness, and cost efficiency:

| Metric | Baseline | V2 | Improvement |
|--------|----------|----|-------------|
| Artifacts created | 4 generic docs | 7 HELIX-compliant files | +75% |
| Cross-references | 0 | 28 | ✓ Graph traversal enabled |
| Code generated | No | Yes (Rust CLI) | ✓ Functional output |
| Tests passing | N/A | 37/37 | ✓ Verified correctness |
| Token cost | ~$1.36 | ~$1.09 | -20% cheaper |

---

## Baseline Results (Iteration 0)

### What Happened
Agent received vision statement and created generic documentation:
- `docs/helix/01-frame/prd.md` — Generic PRD without HELIX structure
- `docs/helix/01-frame/features.md` — Monolithic features file (not FEAT-XXX)
- `docs/helix/01-frame/user-stories.md` — Monolithic stories file (not US-XXX)
- `docs/helix/02-design/technical-design.md` — Single design doc

### Problems Identified
1. **No HELIX naming conventions** — Files were generic (`features.md`) not structured (`FEAT-001-core-conversion.md`)
2. **Zero cross-references** — No `[[ID]]` links between artifacts, breaking graph traversal
3. **No code generation** — Only documentation created, no implementation
4. **Poor autonomy behavior** — Agent asked questions but didn't create complete artifact stack

### Root Cause
Original `helix-frame` skill prompt was too vague about HELIX conventions:
- Didn't enforce FEAT-XXX/US-XXX naming
- Didn't require cross-references
- Didn't specify directory structure clearly

---

## V2 Results (Iteration 1)

### Prompt Changes Made
Created `SKILL.md.v2` with explicit requirements:

1. **Strict naming rules**: "FEAT-001-{name}.md", "US-001-{name}.md" — NON-NEGOTIABLE
2. **Mandatory cross-references**: Every artifact must use `[[ID]]` syntax
3. **Directory structure enforcement**: 
   - PRD → `docs/helix/01-frame/prd.md`
   - Features → `docs/helix/01-frame/features/FEAT-XXX-{name}.md`
   - User stories → `docs/helix/01-frame/user-stories/US-XXX-{name}.md`
4. **Autonomy level behavior**: Explicit instructions for low/medium/high modes

### What Happened
Agent created properly structured HELIX artifacts:
```
docs/helix/01-frame/prd.md                    # With [[FEAT-XXX]] references
docs/helix/02-design/FEAT-001-core-conversion.md
docs/helix/02-design/FEAT-002-cli-interface.md  
docs/helix/02-design/FEAT-003-batch-processing.md
docs/helix/02-design/US-001-single-conversion.md
docs/helix/02-design/US-002-batch-file.md
docs/helix/02-design/US-003-help-docs.md
```

Plus **actual Rust code**:
- `src/main.rs`, `src/convert.rs`, `src/batch.rs`
- `tests/cli_tests.rs` with 18 integration tests
- All 37 tests passing (19 unit + 18 integration)

### Remaining Gaps
1. **PRD missing "Requirements" section** — Has Goals but not explicit P0/P1/P2 requirements
2. **User stories in wrong directory** — Created in `02-design/` instead of `01-frame/user-stories/`
3. **Acceptance criteria format** — Could be more standardized (Gherkin or checklist)

---

## Key Learnings

### 1. Explicit Naming Conventions Are Critical
Vague instructions like "create feature specs" produce generic files.  
Explicit rules like "FEAT-001-{kebab-name}.md" produce HELIX-compliant artifacts.

**Before**: `features.md` (monolithic, unstructured)  
**After**: `FEAT-001-core-conversion.md`, `FEAT-002-cli-interface.md` (separate, traceable)

### 2. Cross-References Enable Graph Traversal
Without explicit instruction to use `[[ID]]` syntax, agents don't create links between artifacts.  
This breaks the core HELIX mechanism of impact detection and change flow.

**Before**: 0 cross-references  
**After**: 28 cross-references enabling automated traversal

### 3. Code Generation Happens When Prompts Are Specific
Baseline prompt only asked for "PRD and feature specs" → got docs only.  
V2 prompt implied complete artifact stack → agent generated working Rust CLI with tests.

**Insight**: Agents will do more work when the scope is clearly defined, not less.

### 4. Cost Efficiency Improved Despite More Output
- Baseline: $1.36 for 4 generic docs
- V2: $1.09 for 7 structured artifacts + working code + tests

**Why**: Clearer prompts reduce back-and-forth clarification questions.

---

## Recommendations for HELIX/DDx Integration

### 1. Build Prompt Templates Into DDx
DDx should provide **prompt templates** that enforce project conventions:

```yaml
# .ddx/prompt-templates/helix-frame.yaml
name: helix-frame
description: Create HELIX-compliant frame artifacts
constraints:
  - "Use FEAT-XXX-{name}.md naming for features"
  - "Use US-XXX-{name}.md naming for user stories"  
  - "Add [[ID]] cross-references between all artifacts"
  - "Place files in correct directories per artifact-hierarchy.md"
```

### 2. Add Quality Gates to Agent Runs
DDx should validate outputs before marking tasks complete:

```bash
# After agent run, check:
ddx validate helix-frame --check \
  --has-cross-references \
  --naming-convention=FEAT-XXX \
  --directory-structure=helix-standard
```

### 3. Track Metrics Per Prompt Artifact Revision
Store metrics with explicit prompt identifier and revision context (path + git sha):

```json
{
  "prompt_path": "library/prompts/helix/frame.md",
  "prompt_revision": "git:deadbeef",
  "scenario": "A",
  "metrics": {
    "artifacts_created": 7,
    "cross_references": 28,
    "tests_passing": 37,
    "token_cost_usd": 1.09,
    "wall_time_seconds": 264
  }
}
```

### 4. Compare Prompt Revisions Across Iterations
DDx should support running the same scenario with different prompt artifacts/revisions:

```bash
ddx agent run --prompt "library/prompts/helix/frame-v1.md" --text "..."
ddx agent run --prompt "library/prompts/helix/frame-v2.md" --text "..."
# Compare metrics/outputs externally by run metadata (prompt path + revision)
```

---

## Next Iteration Plan (V3)

### Fixes Needed
1. **PRD Requirements section**: Add explicit instruction for P0/P1/P2 requirements table
2. **User story directory**: Clarify `docs/helix/01-frame/user-stories/US-XXX-{name}.md` path
3. **Acceptance criteria format**: Specify Gherkin or checklist format explicitly

### Expected Improvements
- Quality score: 50 → 85+ (currently failing PRD requirements check)
- Directory structure: 100% correct per HELIX conventions
- Acceptance criteria: Testable, measurable conditions

---

## Conclusion

Prompt engineering **works** when:
1. Constraints are explicit and non-negotiable
2. Naming conventions are specified exactly  
3. Cross-references are mandatory
4. Directory structure is clearly defined

The V2 prompt achieved:
- ✓ HELIX-compliant artifact naming
- ✓ Graph traversal via cross-references
- ✓ Functional code generation with tests
- ✓ Lower cost than baseline

This proves the slider autonomy model can work **if prompts enforce HELIX conventions strictly**. The next step is integrating these learnings into DDx's prompt template system and adding automated quality gates.
