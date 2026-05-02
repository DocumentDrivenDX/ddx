package bead

// CI guard tests for state-machine drift prevention.
//
// These four tests fail loudly the moment the persisted-status enumeration
// drifts between its four sources of truth:
//
//  1. TestSchemaEnumMatchesCanonicalStatuses — the JSON Schema enum at
//     cli/internal/bead/schema/bead-record.schema.json must equal the
//     exported CanonicalStatuses list in types.go (set equality).
//  2. TestStatusLiteralAudit — every persisted-status assignment to a
//     bead.Bead.Status field anywhere under cli/internal/ must use a
//     literal in CanonicalStatuses (or, preferably, the typed StatusXxx
//     constant). Catches drift like writing "needs_investigation" as a
//     status string instead of using a label.
//  3. TestTDDocMatchesSchemaEnum — the persisted-status enumeration block
//     in docs/helix/02-design/technical-designs/TD-031-bead-state-machine.md
//     §2 must equal the schema enum. Forces TD amendment when schema
//     changes.
//  4. TestSchemaCompatRoundTripPreservesLabelsAndEvents — a bd-export
//     fixture under cli/internal/bead/testdata/ must round-trip through
//     unmarshalBead → marshalBead → unmarshalBead with all known fields,
//     labels, dependencies, and events (in Extra) byte-equivalent. This
//     is the ADR-004 bd/br interchange compatibility guard.
//
// Each guard's failure message points at the exact mismatch location. All
// four together run in well under five seconds; CI invokes them through
// the standard `go test ./...` pass — no opt-in flag is required.

import (
	"encoding/json"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

// repoRoot returns the repository root path computed from this test file's
// location (cli/internal/bead/sync_test.go → repo root is three levels up).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	// cli/internal/bead/sync_test.go → repo root
	root, err := filepath.Abs(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	require.NoError(t, err)
	return root
}

// cliDir returns the cli/ directory path (one level under the repo root).
func cliDir(t *testing.T) string {
	return filepath.Join(repoRoot(t), "cli")
}

// schemaEnum reads the persisted-status enum out of the bead-record JSON
// schema. Returned as a sorted, deduplicated slice for stable comparison.
func schemaEnum(t *testing.T) []string {
	t.Helper()
	path := filepath.Join(cliDir(t), "internal", "bead", "schema", "bead-record.schema.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read schema at %s", path)

	var doc struct {
		Properties struct {
			Status struct {
				Enum []string `json:"enum"`
			} `json:"status"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	require.NotEmpty(t, doc.Properties.Status.Enum, "schema status.enum is empty at %s", path)
	out := append([]string(nil), doc.Properties.Status.Enum...)
	sort.Strings(out)
	return out
}

// canonicalSorted returns CanonicalStatuses as a sorted, deduplicated copy.
func canonicalSorted() []string {
	out := append([]string(nil), CanonicalStatuses...)
	sort.Strings(out)
	return out
}

// TestSchemaEnumMatchesCanonicalStatuses guards the bead-record JSON Schema
// status enum against drift from the exported CanonicalStatuses list in
// types.go. They must contain exactly the same set of strings.
func TestSchemaEnumMatchesCanonicalStatuses(t *testing.T) {
	got := schemaEnum(t)
	want := canonicalSorted()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf(`schema/bead-record.schema.json status enum is out of sync with bead.CanonicalStatuses.
  schema enum (cli/internal/bead/schema/bead-record.schema.json): %v
  Go constants (cli/internal/bead/types.go CanonicalStatuses):    %v
Update both sides together — adding a new persisted status also requires upstream bd/br coordination and an ADR-004 amendment.`, got, want)
	}
}

// statusLiteralFinding records a single offending literal location so the
// failure message can point straight at the file and line.
type statusLiteralFinding struct {
	Pos     token.Position
	Literal string
	Context string // "assignment" or "composite literal"
}

// TestStatusLiteralAudit walks every Go source file under cli/internal/
// (using go/packages with full type info) and flags any string literal
// assigned to a bead.Bead.Status field whose value is not in
// CanonicalStatuses. The intent: future code must never write a non-canonical
// string into a persisted bead status. The typed StatusXxx constants are the
// preferred form, but raw literals that match the canonical set are tolerated
// (legacy compatibility); literals outside the set are a hard error.
func TestStatusLiteralAudit(t *testing.T) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes |
			packages.NeedSyntax | packages.NeedTypesInfo,
		Dir:   cliDir(t),
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, "./internal/...")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs, "no packages loaded under cli/internal/...")

	canonical := make(map[string]bool, len(CanonicalStatuses))
	for _, s := range CanonicalStatuses {
		canonical[s] = true
	}

	var findings []statusLiteralFinding

	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}
		for i, file := range pkg.Syntax {
			fname := ""
			if i < len(pkg.CompiledGoFiles) {
				fname = pkg.CompiledGoFiles[i]
			}
			// Skip generated and test files (test files are excluded by
			// Tests:false above, but be defensive about codegen output).
			if strings.HasSuffix(fname, "_gen.go") || strings.Contains(fname, "/generated/") {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.AssignStmt:
					for li, lhs := range x.Lhs {
						sel, ok := lhs.(*ast.SelectorExpr)
						if !ok || sel.Sel == nil || sel.Sel.Name != "Status" {
							continue
						}
						if !receiverIsBead(pkg, sel.X) {
							continue
						}
						if li >= len(x.Rhs) {
							continue
						}
						lit, ok := x.Rhs[li].(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							continue
						}
						val, err := strconv.Unquote(lit.Value)
						if err != nil {
							continue
						}
						if !canonical[val] {
							findings = append(findings, statusLiteralFinding{
								Pos:     pkg.Fset.Position(lit.Pos()),
								Literal: val,
								Context: "assignment to bead.Bead.Status",
							})
						}
					}
				case *ast.CompositeLit:
					if !compositeIsBead(pkg, x) {
						return true
					}
					for _, elt := range x.Elts {
						kv, ok := elt.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						key, ok := kv.Key.(*ast.Ident)
						if !ok || key.Name != "Status" {
							continue
						}
						lit, ok := kv.Value.(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							continue
						}
						val, err := strconv.Unquote(lit.Value)
						if err != nil {
							continue
						}
						if !canonical[val] {
							findings = append(findings, statusLiteralFinding{
								Pos:     pkg.Fset.Position(lit.Pos()),
								Literal: val,
								Context: "composite literal bead.Bead{Status: ...}",
							})
						}
					}
				}
				return true
			})
		}
	}

	if len(findings) > 0 {
		var b strings.Builder
		b.WriteString("non-canonical persisted-status literal(s) assigned to bead.Bead.Status:\n")
		for _, f := range findings {
			b.WriteString("  - ")
			b.WriteString(f.Pos.String())
			b.WriteString(" (")
			b.WriteString(f.Context)
			b.WriteString("): \"")
			b.WriteString(f.Literal)
			b.WriteString("\"\n")
		}
		b.WriteString("Allowed values are bead.CanonicalStatuses: ")
		b.WriteString(strings.Join(canonicalSorted(), ", "))
		b.WriteString("\nUse the typed constants (bead.StatusOpen, bead.StatusInProgress, ...) where possible. ")
		b.WriteString("If you intended to mark execution sub-state (e.g. needs_investigation), encode it as a label or event, not as a persisted status — see TD-031.")
		t.Fatal(b.String())
	}
}

// receiverIsBead reports whether the receiver expression of `recv.Status`
// refers to a value or pointer of type bead.Bead.
func receiverIsBead(pkg *packages.Package, recv ast.Expr) bool {
	if pkg.TypesInfo == nil {
		return false
	}
	tv, ok := pkg.TypesInfo.Types[recv]
	if !ok {
		return false
	}
	return typeIsBead(tv.Type.String())
}

// compositeIsBead reports whether a composite literal constructs a
// bead.Bead value (or pointer thereto).
func compositeIsBead(pkg *packages.Package, cl *ast.CompositeLit) bool {
	if pkg.TypesInfo == nil {
		return false
	}
	tv, ok := pkg.TypesInfo.Types[cl]
	if !ok {
		return false
	}
	return typeIsBead(tv.Type.String())
}

// typeIsBead matches the textual rendering of types.Type.String() against
// the bead.Bead named type, with optional pointer prefix.
func typeIsBead(s string) bool {
	s = strings.TrimPrefix(s, "*")
	return s == "github.com/DocumentDrivenDX/ddx/internal/bead.Bead"
}

// tdEnumPattern matches the persisted-status enumeration code-block line in
// TD-031 §2: `open | in_progress | closed | blocked | proposed | cancelled`.
var tdEnumPattern = regexp.MustCompile(`(?m)^([a-z_]+(?:\s*\|\s*[a-z_]+)+)\s*$`)

// TestTDDocMatchesSchemaEnum guards the TD-031 persisted-status section
// against drift from the schema enum. The TD §2 fenced code block contains
// a single pipe-separated enumeration line; that line must list exactly the
// same set of statuses as the schema.
func TestTDDocMatchesSchemaEnum(t *testing.T) {
	tdPath := filepath.Join(repoRoot(t), "docs", "helix", "02-design", "technical-designs", "TD-031-bead-state-machine.md")
	data, err := os.ReadFile(tdPath)
	require.NoError(t, err, "read TD doc at %s", tdPath)

	// Locate §2 (Persisted Status Enumeration). The check is anchored on the
	// section heading rather than absolute line numbers so the test stays
	// stable under benign edits.
	text := string(data)
	idx := strings.Index(text, "## 2. Persisted Status Enumeration")
	require.NotEqual(t, -1, idx, `TD-031 missing "## 2. Persisted Status Enumeration" heading at %s`, tdPath)
	tail := text[idx:]
	// Limit search to this section by stopping at the next "## " heading.
	if next := strings.Index(tail[len("## 2. Persisted Status Enumeration"):], "\n## "); next != -1 {
		tail = tail[:len("## 2. Persisted Status Enumeration")+next]
	}

	matches := tdEnumPattern.FindStringSubmatch(tail)
	require.NotNil(t, matches, `TD-031 §2 has no pipe-separated status enumeration line at %s — expected something like "open | in_progress | closed | blocked | proposed | cancelled"`, tdPath)

	parts := strings.Split(matches[1], "|")
	got := make([]string, 0, len(parts))
	for _, p := range parts {
		got = append(got, strings.TrimSpace(p))
	}
	sort.Strings(got)

	want := schemaEnum(t)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf(`TD-031 §2 persisted-status enumeration is out of sync with the JSON Schema.
  TD-031 §2 lists: %v
  Schema lists:    %v
Update docs/helix/02-design/technical-designs/TD-031-bead-state-machine.md §2 to match cli/internal/bead/schema/bead-record.schema.json (or vice versa). Adding a status also requires an ADR-004 amendment.`, got, want)
	}
}

// TestSchemaCompatRoundTripPreservesLabelsAndEvents is the ADR-004
// compatibility guard. It loads a bd-export fixture that includes labels,
// dependencies, and events (the latter via Extra), runs it through DDx's
// unmarshal → marshal → unmarshal pipeline, and asserts every field —
// known and unknown — survives byte-equivalent.
func TestSchemaCompatRoundTripPreservesLabelsAndEvents(t *testing.T) {
	fxPath := filepath.Join("testdata", "bd_export_with_labels_events.jsonl")
	data, err := os.ReadFile(fxPath)
	require.NoError(t, err, "read fixture %s", fxPath)
	line := strings.TrimSpace(string(data))
	require.NotEmpty(t, line, "fixture %s is empty", fxPath)

	// Parse the fixture as a generic map so we can compare every field after
	// the round-trip.
	var original map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &original), "fixture is not valid JSON")

	b1, err := unmarshalBead([]byte(line))
	require.NoError(t, err, "first unmarshal")

	out, err := marshalBead(b1)
	require.NoError(t, err, "marshal")

	b2, err := unmarshalBead(out)
	require.NoError(t, err, "second unmarshal")
	assert.Equal(t, b1, b2, "Bead struct must round-trip equal")

	var roundTripped map[string]any
	require.NoError(t, json.Unmarshal(out, &roundTripped), "marshalled output is not valid JSON")

	// Compare every key from the original. unmarshal/marshal may normalize
	// timestamp formatting, so compare time fields by parsed value rather
	// than by string equality.
	timeFields := map[string]bool{"created_at": true, "updated_at": true}
	for k, want := range original {
		got, ok := roundTripped[k]
		if !ok {
			t.Errorf("round-trip dropped field %q (value was %v) — fixture: %s", k, want, fxPath)
			continue
		}
		if timeFields[k] {
			// Compare time strings semantically.
			gs, _ := got.(string)
			ws, _ := want.(string)
			if gs != ws && !sameRFC3339(gs, ws) {
				t.Errorf("round-trip mutated time field %q: got %v, want %v", k, got, want)
			}
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf(`round-trip mutated field %q:
  got:  %#v
  want: %#v
fixture: %s`, k, got, want, fxPath)
		}
	}

	// Spot-check that the structured fields we care about most are intact.
	assert.Equal(t, []string{"area:bead", "kind:guardrail", "phase:2"}, b2.Labels, "labels must round-trip")
	require.NotNil(t, b2.Extra["events"], "events (preserved-extras) must survive round-trip")
	events, ok := b2.Extra["events"].([]any)
	require.True(t, ok, "events must round-trip as a JSON array, got %T", b2.Extra["events"])
	require.Len(t, events, 2, "fixture has two events; both must survive")
}

// sameRFC3339 reports whether two strings represent the same instant when
// parsed as RFC3339 timestamps. Used so the round-trip test does not
// false-fail on cosmetic timestamp re-formatting (e.g. trailing nanos).
func sameRFC3339(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ta, err1 := time.Parse(time.RFC3339Nano, a)
	if err1 != nil {
		ta, err1 = time.Parse(time.RFC3339, a)
	}
	tb, err2 := time.Parse(time.RFC3339Nano, b)
	if err2 != nil {
		tb, err2 = time.Parse(time.RFC3339, b)
	}
	if err1 != nil || err2 != nil {
		return false
	}
	return ta.Equal(tb)
}
