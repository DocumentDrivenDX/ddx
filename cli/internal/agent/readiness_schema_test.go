package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compileReadinessChecksSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	schemaPath := filepath.Join(filepath.Dir(currentFile), "schema", "readiness-checks.schema.json")
	raw, err := os.ReadFile(schemaPath)
	require.NoError(t, err)

	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("readiness-checks.schema.json", strings.NewReader(string(raw))))

	// Compile also asserts the schema document itself is well-formed.
	schema, err := compiler.Compile("readiness-checks.schema.json")
	require.NoError(t, err)
	return schema
}

// TestReadinessChecksSchema locks the readiness payload shape behind the shared
// readiness-checks.schema.json so the producer prompt and the Go decoder cannot
// drift the way the parent wedge (ddx-f9ddaa68) did, where the producer emitted
// readiness_checks[].verdict as a JSON bool while the consumer parsed a string.
//
// Every verdict form readinessVerdict.UnmarshalJSON
// (cli/internal/agent/preclaim_intake_hook.go:91-121) accepts must both
//   - validate against the schema (AC1/AC4: tightening the schema to reject any
//     accepted form fails this test), and
//   - decode to the canonical readinessVerdict (AC2: pass/fail/passthrough/empty).
func TestReadinessChecksSchema(t *testing.T) {
	schema := compileReadinessChecksSchema(t)

	// buildPayload returns a full readiness payload whose single readiness_checks
	// entry carries the given raw JSON for verdict, or omits verdict when empty.
	buildPayload := func(verdictJSON string) string {
		entry := `{"reason":"missing_verification","evidence":"AC lacks go test","checkable_before_attempt":true`
		if verdictJSON != "" {
			entry += `,"verdict":` + verdictJSON
		}
		entry += `}`
		return `{"classification":"needs_refine","rationale":"check","readiness_checks":[` + entry + `]}`
	}

	cases := []struct {
		name        string
		verdictJSON string
		want        readinessVerdict
	}{
		{name: "bool_true_to_pass", verdictJSON: `true`, want: "pass"},
		{name: "bool_false_to_fail", verdictJSON: `false`, want: "fail"},
		{name: "string_fail_passthrough", verdictJSON: `"fail"`, want: "fail"},
		{name: "string_ready_passthrough", verdictJSON: `"ready"`, want: "ready"},
		{name: "string_not_ready_passthrough", verdictJSON: `"not_ready"`, want: "not_ready"},
		{name: "null_empty", verdictJSON: `null`, want: ""},
		{name: "absent_empty", verdictJSON: "", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := buildPayload(tc.verdictJSON)

			// AC1/AC4: the schema must accept every form the decoder accepts.
			var v any
			require.NoError(t, json.Unmarshal([]byte(payload), &v))
			require.NoError(t, schema.Validate(v),
				"verdict form %q must validate against readiness-checks.schema.json", tc.verdictJSON)

			// AC2: the same form must decode to the canonical readinessVerdict.
			var classified preClaimReadinessClassificationPromptResult
			require.NoError(t, json.Unmarshal([]byte(payload), &classified))
			require.Len(t, classified.ReadinessChecks.Checks, 1)
			assert.Equal(t, tc.want, classified.ReadinessChecks.Checks[0].Verdict)
		})
	}

	t.Run("malformed_kind_rejected", func(t *testing.T) {
		payload := buildPayload(`{"kind":"pass"}`)

		var v any
		require.NoError(t, json.Unmarshal([]byte(payload), &v))
		require.Error(t, schema.Validate(v), "object verdicts must be rejected by readiness-checks.schema.json")
	})
}

// TestReadinessChecksSchema_CheckableBeforeAttemptRemainsBoolean keeps
// checkable_before_attempt boolean-only in the shared schema while proving the
// robustness decoder diagnoses invalid strings instead of silently widening
// canonical acceptance (ddx-442c48e3, cross-ref ddx-52d1c006).
func TestReadinessChecksSchema_CheckableBeforeAttemptRemainsBoolean(t *testing.T) {
	schema := compileReadinessChecksSchema(t)

	buildPayload := func(checkableJSON string) string {
		entry := `{"reason":"missing_verification","evidence":"AC lacks go test","verdict":"fail"`
		if checkableJSON != "" {
			entry += `,"checkable_before_attempt":` + checkableJSON
		}
		entry += `}`
		return `{"classification":"needs_refine","rationale":"check","readiness_checks":[` + entry + `]}`
	}

	t.Run("boolean_true_validates_and_decodes", func(t *testing.T) {
		payload := buildPayload(`true`)
		var v any
		require.NoError(t, json.Unmarshal([]byte(payload), &v))
		require.NoError(t, schema.Validate(v))

		var classified preClaimReadinessClassificationPromptResult
		require.NoError(t, json.Unmarshal([]byte(payload), &classified))
		require.Len(t, classified.ReadinessChecks.Checks, 1)
		assert.True(t, classified.ReadinessChecks.Checks[0].CheckableBeforeAttempt)
		assert.Empty(t, classified.ReadinessChecks.Malformed)
	})

	t.Run("boolean_false_validates_and_decodes", func(t *testing.T) {
		payload := buildPayload(`false`)
		var v any
		require.NoError(t, json.Unmarshal([]byte(payload), &v))
		require.NoError(t, schema.Validate(v))

		var classified preClaimReadinessClassificationPromptResult
		require.NoError(t, json.Unmarshal([]byte(payload), &classified))
		require.Len(t, classified.ReadinessChecks.Checks, 1)
		assert.False(t, classified.ReadinessChecks.Checks[0].CheckableBeforeAttempt)
		assert.Empty(t, classified.ReadinessChecks.Malformed)
	})

	for _, tc := range []struct {
		name          string
		checkableJSON string
		wantKind      string
	}{
		{name: "string_rejected", checkableJSON: `"yes"`, wantKind: "string"},
		{name: "null_rejected", checkableJSON: `null`, wantKind: "null"},
		{name: "object_rejected", checkableJSON: `{}`, wantKind: "object"},
		{name: "array_rejected", checkableJSON: `[]`, wantKind: "array"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := buildPayload(tc.checkableJSON)

			// Canonical shared schema does not accept non-boolean values.
			var v any
			require.NoError(t, json.Unmarshal([]byte(payload), &v))
			require.Error(t, schema.Validate(v),
				"checkable_before_attempt %s must not validate as canonical output", tc.wantKind)

			// Robustness decoder must not silently widen: non-boolean values do
			// not become accepted boolean truth, and strings become Malformed.
			var classified preClaimReadinessClassificationPromptResult
			require.NoError(t, json.Unmarshal([]byte(payload), &classified),
				"decoder must preserve intake rather than aborting with a raw Go error")
			require.Len(t, classified.ReadinessChecks.Checks, 1)
			assert.False(t, classified.ReadinessChecks.Checks[0].CheckableBeforeAttempt,
				"robustness path must not coerce %s into a true boolean", tc.wantKind)
			if tc.wantKind == "null" {
				// null maps to the zero value; schema rejection is the
				// canonical-output gate, not a nested Malformed diagnostic.
				assert.Empty(t, classified.ReadinessChecks.Malformed)
				return
			}
			assert.Contains(t, classified.ReadinessChecks.Malformed, "readiness_checks[0].checkable_before_attempt")
			assert.Contains(t, classified.ReadinessChecks.Malformed, tc.wantKind)
			assert.NotContains(t, classified.ReadinessChecks.Malformed, "cannot unmarshal")
			assert.NotContains(t, classified.ReadinessChecks.Malformed, "Go struct field")
		})
	}
}
