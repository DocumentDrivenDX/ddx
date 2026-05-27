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
