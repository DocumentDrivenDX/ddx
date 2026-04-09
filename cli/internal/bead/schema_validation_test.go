package bead

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

func compileBeadSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	schemaPath := filepath.Join(filepath.Dir(currentFile), "schema", "bead-record.schema.json")
	raw, err := os.ReadFile(schemaPath)
	require.NoError(t, err)

	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("bead-record.schema.json", strings.NewReader(string(raw))))

	schema, err := compiler.Compile("bead-record.schema.json")
	require.NoError(t, err)
	return schema
}

func validateJSONAgainstSchema(t *testing.T, schema *jsonschema.Schema, raw string) error {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal([]byte(raw), &v))
	return schema.Validate(v)
}

func TestBeadRecordSchemaValidatesBdExportExample(t *testing.T) {
	schema := compileBeadSchema(t)

	bdExport := `{"id":"bd-test-1x4","title":"test issue","status":"open","priority":2,"issue_type":"task","owner":"user@example.com","created_at":"2026-04-04T01:23:39Z","created_by":"Test User","updated_at":"2026-04-04T01:23:39Z","dependencies":[{"issue_id":"bd-test-1x4","depends_on_id":"bd-test-ioz","type":"blocks","created_at":"2026-04-03T21:23:54Z","created_by":"Test User","metadata":"{}"}],"dependency_count":1,"dependent_count":0,"comment_count":0}`

	require.NoError(t, validateJSONAgainstSchema(t, schema, bdExport))
}

func TestBeadRecordSchemaValidatesDDxCreatedRecord(t *testing.T) {
	schema := compileBeadSchema(t)
	store := newTestStore(t)

	b := &Bead{
		Title:     "DDx created bead",
		IssueType: "task",
		Priority:  2,
		Labels:    []string{"core"},
		Extra: map[string]any{
			"attachment_refs": map[string]any{
				"result": "exec-runs.d/run-1/result.json",
			},
		},
	}
	require.NoError(t, store.Create(b))

	data, err := MarshalBead(*b)
	require.NoError(t, err)

	require.NoError(t, validateJSONAgainstSchema(t, schema, string(data)))
}

func TestBeadRecordSchemaRejectsMissingTitle(t *testing.T) {
	schema := compileBeadSchema(t)

	invalid := `{"id":"ddx-test","status":"open","priority":2,"issue_type":"task","created_at":"2026-04-04T01:23:39Z","updated_at":"2026-04-04T01:23:39Z"}`

	require.Error(t, validateJSONAgainstSchema(t, schema, invalid))
}
