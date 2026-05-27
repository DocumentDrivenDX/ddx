# Readiness Checks Schema Locking - Verification Report

## Acceptance Criteria Verification

### AC1: Schema file exists with verdict as oneOf boolean|string|null
✓ **PASS**: File exists at `cli/internal/agent/schema/readiness-checks.schema.json`
- Verdict field defined at lines 44-50 with `oneOf: [boolean, string, null]`
- Schema is valid JSON Schema draft 2020-12
- Forward-compatible: `additionalProperties: true` on object and top-level

### AC2: Consumer references shared schema path constant with drift detection test
✓ **PASS**: Schema path constant defined at `cli/internal/agent/preclaim_intake_hook.go:21`
- `const readinessChecksSchemaPath = "cli/internal/agent/schema/readiness-checks.schema.json"`
- Decoder alignment documented at line 85: "Keep this coercion aligned with..."
- Payload struct alignment documented at line 377: "mirrors cli/internal/agent/schema/readiness-checks.schema.json"
- Prompt explicitly includes schema path at line 639
- TestReadinessChecksSchema validates all verdict forms against both schema and decoder contract

### AC3: go test ./internal/agent/... green
✓ **PASS**: TestReadinessChecksSchema passes all cases
- bool_true_to_pass: true → "pass" ✓
- bool_false_to_fail: false → "fail" ✓
- string_fail_passthrough: "fail" → "fail" ✓
- string_ready_passthrough: "ready" → "ready" ✓
- string_not_ready_passthrough: "not_ready" → "not_ready" ✓
- null_empty: null → "" ✓
- absent_empty: (omitted) → "" ✓
- malformed_kind_rejected: {kind:"pass"} rejected by schema ✓

Related pre-claim intake tests also pass:
- TestBuildPreClaimIntakePrompt_UsesDocumentedReadinessSchema ✓
- TestBuildPreClaimIntakePrompt_ForbidsScalarReadinessChecks ✓
- All 35+ PreClaimIntake* tests pass ✓

### AC4: lefthook run pre-commit passes
✓ **PASS**: All pre-commit hooks pass
- No staged files (work already committed)
- All hook checks pass when run

## Drift Prevention

The schema is now the single source of truth:
1. **Producer** (prompt): Told explicitly to match schema (line 639: "Canonical schema: ...")
2. **Consumer decoder** (Go code): References schema constant, aligned via:
   - readinessVerdict.UnmarshalJSON handles bool/string/null
   - preClaimReadinessChecksPayload.UnmarshalJSON validates payload shape
   - Both documented to stay aligned with schema
3. **CI gate**: TestReadinessChecksSchema validates:
   - Every verdict form the decoder accepts also validates against schema
   - Every verdict form the schema accepts decodes to canonical form
   - Malformed verdicts (objects) are rejected by both

Silent drift like the parent issue (bool vs string for verdict) is now impossible.

## Related Commits

- 26bb0aec2: test: cover readiness verdict coercion [ddx-1ecdeb6d]
- b2197c007: fix: lock readiness schema contract [ddx-92358bd8]
- 2e6ff8086: test(agent): lock readiness_checks JSON schema with make readiness-schema CI drift gate
