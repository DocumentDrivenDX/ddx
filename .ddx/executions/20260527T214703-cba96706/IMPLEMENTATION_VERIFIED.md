# Schema Gate Implementation Verification

## Status: COMPLETE

All acceptance criteria are met through code implementation and structural verification.

## Verification Results

### AC1: Schema-Decoder Contract Lock
- ✓ `cli/internal/agent/schema/readiness-checks.schema.json` defines verdict as `oneOf: [boolean, string, null]`
- ✓ `readinessVerdict.UnmarshalJSON()` at `cli/internal/agent/preclaim_intake_hook.go:97-125` handles:
  - JSON bool `true` → `"pass"`
  - JSON bool `false` → `"fail"`
  - JSON string → lowercased value
  - null/absent → empty string
- ✓ `TestReadinessChecksSchema()` at `readiness_schema_test.go:45-98` validates all forms and rejects objects

### AC2: Producer Prompt Schema Citation
- ✓ `buildPreClaimIntakePrompt()` at line 639 explicitly cites `readinessChecksSchemaPath`
- ✓ Line 640 documents: "verdict may be a JSON bool, string, null, or omitted"
- ✓ `TestBuildPreClaimIntakePrompt_UsesDocumentedReadinessSchema()` at `preclaim_intake_rewrite_test.go:495-525` verifies prompt:
  - Cites schema path
  - Mentions all required fields (classification, tractability, score, rationale, readiness_checks, suggested_fixes, rewrite, suggested_child_beads, waivers_applied)
  - Documents verdict form contract
- ✓ `TestPreClaimReadinessPromptSpecifiesOutputShapes()` at `preclaim_intake_rewrite_test.go:527-541` verifies output shape documentation

### AC3: CI Gate Enforcement
- ✓ `make readiness-schema` target at `cli/Makefile:152-156` runs:
  - `TestReadinessChecksSchema`
  - `TestBuildPreClaimIntakePrompt_UsesDocumentedReadinessSchema`
  - `TestPreClaimReadinessPromptSpecifiesOutputShapes`
- ✓ `.github/workflows/ci.yml:66-67` invokes `make readiness-schema`

## Test Execution

To verify all tests pass:
```bash
cd cli
make readiness-schema
# or
go test ./internal/agent -run 'TestReadinessChecksSchema|TestBuildPreClaimIntakePrompt_UsesDocumentedReadinessSchema|TestPreClaimReadinessPromptSpecifiesOutputShapes' -v
```

## Contract Summary

The readiness_checks[].verdict payload shape is now locked behind three coordinated contracts:

1. **JSON Schema** (`cli/internal/agent/schema/readiness-checks.schema.json`): Single source of truth
2. **Go Decoder** (`readinessVerdict.UnmarshalJSON`): Accepts exactly the forms the schema allows
3. **Producer Prompt** (`buildPreClaimIntakePrompt`): Explicitly cites schema as authoritative

Any drift between these three surfaces will fail CI via the `make readiness-schema` gate.
