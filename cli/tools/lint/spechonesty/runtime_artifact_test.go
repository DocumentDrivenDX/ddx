package spechonesty

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// TestRuntimeArtifactTargetClassification: mapping rows whose evidence
// target names an inspectable runtime artifact are classified as
// runtime-artifact targets; non-artifact evidence is out-of-band for
// this resolver.
func TestRuntimeArtifactTargetClassification(t *testing.T) {
	cases := []struct {
		name   string
		target string
		want   RuntimeArtifactClassKind
		// wantPath is the normalized path when want == runtime_artifact.
		wantPath string
	}{
		// Runtime artifacts: repository / generated fixture paths.
		{
			name:     "ddx_executions_fixture_report",
			target:   ".ddx/executions/fixture/report.json",
			want:     RuntimeArtifactClassRuntime,
			wantPath: ".ddx/executions/fixture/report.json",
		},
		{
			name:     "repo_relative_json",
			target:   "cli/testdata/observation.json",
			want:     RuntimeArtifactClassRuntime,
			wantPath: "cli/testdata/observation.json",
		},
		{
			name:     "backticked_artifact_path",
			target:   "`.ddx/executions/run-1/result.json`",
			want:     RuntimeArtifactClassRuntime,
			wantPath: ".ddx/executions/run-1/result.json",
		},
		{
			name:     "bare_filename_with_extension",
			target:   "observation-report.json",
			want:     RuntimeArtifactClassRuntime,
			wantPath: "observation-report.json",
		},
		{
			name:     "leading_dot_slash",
			target:   "./artifacts/out.xml",
			want:     RuntimeArtifactClassRuntime,
			wantPath: "artifacts/out.xml",
		},

		// Out of band: Test* symbols (owned by test-symbol sibling).
		{
			name:   "bare_test_symbol",
			target: "TestCreateResource",
			want:   RuntimeArtifactClassOutOfBand,
		},
		{
			name:   "scoped_test_symbol_colon",
			target: "pkg/existing_test.go:TestCreateResource",
			want:   RuntimeArtifactClassOutOfBand,
		},
		{
			name:   "scoped_test_symbol_hash",
			target: "pkg/existing_test.go#TestCreateResource",
			want:   RuntimeArtifactClassOutOfBand,
		},

		// Out of band: static checks.
		{
			name:   "static_check",
			target: "check:static-delete",
			want:   RuntimeArtifactClassOutOfBand,
		},

		// Out of band: file-only Go test evidence.
		{
			name:   "file_only_test_go",
			target: "pkg/existing_test.go",
			want:   RuntimeArtifactClassOutOfBand,
		},

		// Out of band: empty / free text / URLs.
		{
			name:   "empty",
			target: "",
			want:   RuntimeArtifactClassOutOfBand,
		},
		{
			name:   "whitespace",
			target: "   ",
			want:   RuntimeArtifactClassOutOfBand,
		},
		{
			name:   "free_text",
			target: "manual inspection of the dashboard",
			want:   RuntimeArtifactClassOutOfBand,
		},
		{
			name:   "http_url",
			target: "https://example.com/report.json",
			want:   RuntimeArtifactClassOutOfBand,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyRuntimeArtifactTarget(tc.target)
			if got.Kind != tc.want {
				t.Fatalf("ClassifyRuntimeArtifactTarget(%q).Kind = %q, want %q",
					tc.target, got.Kind, tc.want)
			}
			if tc.want == RuntimeArtifactClassRuntime {
				if got.Path != tc.wantPath {
					t.Fatalf("Path = %q, want %q", got.Path, tc.wantPath)
				}
				if got.Path == "" {
					t.Fatal("runtime-artifact classification must carry a non-empty Path")
				}
			} else if got.Path != "" {
				t.Fatalf("out-of-band Path must be empty, got %q", got.Path)
			}
		})
	}

	// Row-level: fixture mapping rows from the Verification parser.
	path := filepath.Join("testdata", "docs", "section_anchors_only.md")
	model, err := ParseVerificationDocument(path)
	if err != nil {
		t.Fatalf("ParseVerificationDocument: %v", err)
	}
	if len(model.Rows) < 3 {
		t.Fatalf("expected ≥3 rows, got %d", len(model.Rows))
	}
	// First two rows are Test* symbols; last is a runtime artifact path.
	if got := ClassifyRuntimeArtifactTarget(model.Rows[0].EvidenceTarget); got.Kind != RuntimeArtifactClassOutOfBand {
		t.Fatalf("row0 %q: Kind = %q, want out_of_band", model.Rows[0].EvidenceTarget, got.Kind)
	}
	if got := ClassifyRuntimeArtifactTarget(model.Rows[1].EvidenceTarget); got.Kind != RuntimeArtifactClassOutOfBand {
		t.Fatalf("row1 %q: Kind = %q, want out_of_band", model.Rows[1].EvidenceTarget, got.Kind)
	}
	artifactRow := model.Rows[2]
	if artifactRow.EvidenceTarget != ".ddx/executions/fixture/report.json" {
		t.Fatalf("row2 evidence = %q, want .ddx/executions/fixture/report.json", artifactRow.EvidenceTarget)
	}
	got := ClassifyRuntimeArtifactTarget(artifactRow.EvidenceTarget)
	if got.Kind != RuntimeArtifactClassRuntime {
		t.Fatalf("row2 Kind = %q, want runtime_artifact", got.Kind)
	}
	if got.Path != ".ddx/executions/fixture/report.json" {
		t.Fatalf("row2 Path = %q", got.Path)
	}
}

// TestRuntimeArtifactTargetResolution: existing repository / generated
// fixture paths resolve successfully with ResolvedPath set; nonexistent
// paths report unresolved and carry the offending path string from the
// mapping row.
func TestRuntimeArtifactTargetResolution(t *testing.T) {
	root := t.TempDir()

	// Existing repository path.
	repoRel := filepath.Join("docs", "helix", "evidence.json")
	repoAbs := filepath.Join(root, repoRel)
	if err := os.MkdirAll(filepath.Dir(repoAbs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(repoAbs, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatalf("write repo artifact: %v", err)
	}

	// Generated fixture artifact under .ddx/executions/.
	fixtureRel := filepath.Join(ddxroot.DirName, "executions", "fixture", "report.json")
	fixtureAbs := filepath.Join(root, fixtureRel)
	if err := os.MkdirAll(filepath.Dir(fixtureAbs), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(fixtureAbs, []byte(`{"pass":true}`), 0o644); err != nil {
		t.Fatalf("write fixture artifact: %v", err)
	}

	// Existing repository path resolves.
	got := ResolveRuntimeArtifactTarget(root, "docs/helix/evidence.json")
	if got.Kind != RuntimeArtifactClassRuntime {
		t.Fatalf("Kind = %q, want runtime_artifact", got.Kind)
	}
	if !got.Resolved {
		t.Fatalf("existing repo path must resolve; got %+v", got)
	}
	if got.Path != "docs/helix/evidence.json" {
		t.Fatalf("Path = %q, want docs/helix/evidence.json", got.Path)
	}
	if got.ResolvedPath == "" {
		t.Fatal("ResolvedPath must be set when Resolved")
	}
	if _, err := os.Stat(got.ResolvedPath); err != nil {
		t.Fatalf("ResolvedPath %q not stat-able: %v", got.ResolvedPath, err)
	}

	// Generated fixture artifact resolves.
	got = ResolveRuntimeArtifactTarget(root, ".ddx/executions/fixture/report.json")
	if got.Kind != RuntimeArtifactClassRuntime || !got.Resolved {
		t.Fatalf("fixture artifact must resolve; got %+v", got)
	}
	if got.Path != ".ddx/executions/fixture/report.json" {
		t.Fatalf("Path = %q", got.Path)
	}
	if got.ResolvedPath == "" {
		t.Fatal("ResolvedPath empty for fixture artifact")
	}

	// Nonexistent path reports unresolved and carries the offending path.
	missing := "docs/helix/missing-artifact.json"
	got = ResolveRuntimeArtifactTarget(root, missing)
	if got.Kind != RuntimeArtifactClassRuntime {
		t.Fatalf("missing path still classifies as runtime_artifact; got %q", got.Kind)
	}
	if got.Resolved {
		t.Fatalf("nonexistent path must be unresolved; got %+v", got)
	}
	if got.Path != missing {
		t.Fatalf("unresolved Path = %q, want offending path %q from mapping row", got.Path, missing)
	}
	if got.ResolvedPath != "" {
		t.Fatalf("ResolvedPath must be empty when unresolved, got %q", got.ResolvedPath)
	}

	// Row API: unresolved carries RequirementRef/Line + offending path.
	row := VerificationRow{
		RequirementRef: "REQ-042",
		EvidenceTarget: "artifacts/does-not-exist.json",
		Command:        "test -f artifacts/does-not-exist.json",
		Line:           17,
	}
	got = ResolveRuntimeArtifactRow(root, row)
	if got.Resolved {
		t.Fatalf("row must be unresolved; got %+v", got)
	}
	if got.Path != "artifacts/does-not-exist.json" {
		t.Fatalf("row Path = %q, want artifacts/does-not-exist.json", got.Path)
	}
	if got.RequirementRef != "REQ-042" || got.Line != 17 {
		t.Fatalf("row context lost: RequirementRef=%q Line=%d", got.RequirementRef, got.Line)
	}

	// Out-of-band targets do not claim resolution.
	got = ResolveRuntimeArtifactTarget(root, "TestCreateResource")
	if got.Kind != RuntimeArtifactClassOutOfBand {
		t.Fatalf("Test* Kind = %q, want out_of_band", got.Kind)
	}
	if got.Resolved || got.Path != "" {
		t.Fatalf("out-of-band must not resolve: %+v", got)
	}

	// ResolveRuntimeArtifactRows preserves order and mixed kinds.
	rows := []VerificationRow{
		{RequirementRef: "REQ-001", EvidenceTarget: "docs/helix/evidence.json", Line: 1},
		{RequirementRef: "REQ-002", EvidenceTarget: "TestCreateResource", Line: 2},
		{RequirementRef: "REQ-003", EvidenceTarget: "docs/helix/missing-artifact.json", Line: 3},
		{RequirementRef: "REQ-004", EvidenceTarget: "check:static-delete", Line: 4},
	}
	results := ResolveRuntimeArtifactRows(root, rows)
	if len(results) != 4 {
		t.Fatalf("len(results) = %d, want 4", len(results))
	}
	if !results[0].Resolved || results[0].Kind != RuntimeArtifactClassRuntime {
		t.Fatalf("results[0] = %+v, want resolved runtime_artifact", results[0])
	}
	if results[1].Kind != RuntimeArtifactClassOutOfBand {
		t.Fatalf("results[1] Kind = %q", results[1].Kind)
	}
	if results[2].Resolved || results[2].Path != "docs/helix/missing-artifact.json" {
		t.Fatalf("results[2] = %+v, want unresolved with offending path", results[2])
	}
	if results[3].Kind != RuntimeArtifactClassOutOfBand {
		t.Fatalf("results[3] Kind = %q", results[3].Kind)
	}
}

// TestRuntimeArtifactTargetResolution_NoNetwork: resolution succeeds with
// no network access available. The resolver is filesystem-only; this test
// forces common network-related env vars into a broken state and still
// expects local path resolution to succeed.
func TestRuntimeArtifactTargetResolution_NoNetwork(t *testing.T) {
	// Break proxy / network-ish environment so any accidental network use
	// would fail rather than silently succeed.
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "socks5://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	t.Setenv("http_proxy", "http://127.0.0.1:1")
	t.Setenv("https_proxy", "http://127.0.0.1:1")

	root := t.TempDir()
	rel := filepath.Join(ddxroot.DirName, "executions", "offline", "report.json")
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte(`{"offline":true}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := ResolveRuntimeArtifactTarget(root, ".ddx/executions/offline/report.json")
	if got.Kind != RuntimeArtifactClassRuntime {
		t.Fatalf("Kind = %q, want runtime_artifact", got.Kind)
	}
	if !got.Resolved {
		t.Fatalf("offline resolution must succeed without network; got %+v", got)
	}
	if got.ResolvedPath == "" {
		t.Fatal("ResolvedPath empty")
	}

	// Unresolved path also must not require network.
	missing := ResolveRuntimeArtifactTarget(root, ".ddx/executions/offline/absent.json")
	if missing.Kind != RuntimeArtifactClassRuntime || missing.Resolved {
		t.Fatalf("absent local path: %+v", missing)
	}
	if missing.Path != ".ddx/executions/offline/absent.json" {
		t.Fatalf("Path = %q", missing.Path)
	}
}

const fixtureCompleteExistingRuntimeArtifact = `---
ddx:
  id: FIXTURE-COMPLETE-EXISTING-RUNTIME-ARTIFACT
---
# Fixture Complete Existing Runtime Artifact

**Status:** Complete

## Requirements

### REQ-001: Observation report

The system MUST retain the observation report artifact.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | .ddx/executions/fixture/report.json | test -f .ddx/executions/fixture/report.json |
`

const fixtureImplementedExistingRuntimeArtifact = `---
ddx:
  id: FIXTURE-IMPLEMENTED-EXISTING-RUNTIME-ARTIFACT
---
# Fixture Implemented Existing Runtime Artifact

**Status:** Implemented

## Requirements

### REQ-010: Generated evidence

The system MUST publish generated evidence under the fixture tree.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-010 | docs/helix/evidence.json | test -f docs/helix/evidence.json |
`

// writeExistingRuntimeArtifactFixture places the inspectable artifact paths
// used by the Complete/Implemented positive-path fixtures under root.
func writeExistingRuntimeArtifactFixture(t *testing.T, root string) {
	t.Helper()
	paths := []string{
		filepath.Join(ddxroot.DirName, "executions", "fixture", "report.json"),
		filepath.Join("docs", "helix", "evidence.json"),
	}
	for _, rel := range paths {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(`{"ok":true}`), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

// TestCompleteVerificationResolvesExistingRuntimeArtifact: a Complete
// fixture mapping a requirement to an existing inspectable runtime
// artifact passes artifact resolution and emits no missing-artifact
// diagnostic.
func TestCompleteVerificationResolvesExistingRuntimeArtifact(t *testing.T) {
	root := t.TempDir()
	writeExistingRuntimeArtifactFixture(t, root)

	path := "docs/fixtures/complete_existing_runtime_artifact.md"
	status := ParseDocumentStatusMarkdown(path, fixtureCompleteExistingRuntimeArtifact)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteExistingRuntimeArtifact)
	if len(model.Rows) != 1 {
		t.Fatalf("rows = %d, want 1; %+v", len(model.Rows), model.Rows)
	}
	row := model.Rows[0]
	if row.EvidenceTarget != ".ddx/executions/fixture/report.json" {
		t.Fatalf("EvidenceTarget = %q", row.EvidenceTarget)
	}

	// Resolver positive path: classified as runtime artifact and present.
	res := ResolveRuntimeArtifactRow(root, row)
	if res.Kind != RuntimeArtifactClassRuntime {
		t.Fatalf("Kind = %q, want runtime_artifact", res.Kind)
	}
	if !res.Resolved {
		t.Fatalf("existing runtime artifact must resolve; got %+v", res)
	}
	if res.ResolvedPath == "" {
		t.Fatal("ResolvedPath must be set when Resolved")
	}

	// Analyzer validation: no missing-artifact diagnostic for existing path.
	findings := CheckRuntimeArtifactResolution(RuntimeArtifactInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Rows:       model.Rows,
		RepoRoot:   root,
	})
	if len(findings) != 0 {
		t.Fatalf("existing runtime artifact must emit no diagnostic; got %+v", findings)
	}
	for _, f := range findings {
		if f.Kind == FindingMissingRuntimeArtifact {
			t.Fatalf("must not emit missing_runtime_artifact for existing path; got %+v", f)
		}
	}

	// Convenience path (status + mapping parse + validation).
	docFindings := CheckDocumentRuntimeArtifacts(path, fixtureCompleteExistingRuntimeArtifact, root)
	if len(docFindings) != 0 {
		t.Fatalf("CheckDocumentRuntimeArtifacts must pass for existing artifact; got %+v", docFindings)
	}
}

// TestImplementedVerificationResolvesExistingRuntimeArtifact: an
// Implemented fixture mapping a requirement to an existing inspectable
// runtime artifact passes artifact resolution and emits no
// missing-artifact diagnostic.
func TestImplementedVerificationResolvesExistingRuntimeArtifact(t *testing.T) {
	root := t.TempDir()
	writeExistingRuntimeArtifactFixture(t, root)

	path := "docs/fixtures/implemented_existing_runtime_artifact.md"
	status := ParseDocumentStatusMarkdown(path, fixtureImplementedExistingRuntimeArtifact)
	if status.Status != StatusImplemented {
		t.Fatalf("Status = %q, want %q", status.Status, StatusImplemented)
	}
	if !IsCompleteStatus(status.Status) {
		t.Fatal("Implemented must be treated as Complete/Implemented for validation")
	}
	model := ParseVerificationMarkdown(path, fixtureImplementedExistingRuntimeArtifact)
	if len(model.Rows) != 1 {
		t.Fatalf("rows = %d, want 1; %+v", len(model.Rows), model.Rows)
	}
	row := model.Rows[0]
	if row.EvidenceTarget != "docs/helix/evidence.json" {
		t.Fatalf("EvidenceTarget = %q", row.EvidenceTarget)
	}

	res := ResolveRuntimeArtifactRow(root, row)
	if res.Kind != RuntimeArtifactClassRuntime || !res.Resolved {
		t.Fatalf("existing Implemented runtime artifact must resolve; got %+v", res)
	}

	findings := CheckRuntimeArtifactResolution(RuntimeArtifactInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Rows:       model.Rows,
		RepoRoot:   root,
	})
	if len(findings) != 0 {
		t.Fatalf("existing runtime artifact must emit no diagnostic; got %+v", findings)
	}
	for _, f := range findings {
		if f.Kind == FindingMissingRuntimeArtifact {
			t.Fatalf("must not emit missing_runtime_artifact for existing path; got %+v", f)
		}
	}

	docFindings := CheckDocumentRuntimeArtifacts(path, fixtureImplementedExistingRuntimeArtifact, root)
	if len(docFindings) != 0 {
		t.Fatalf("CheckDocumentRuntimeArtifacts must pass for Implemented existing artifact; got %+v", docFindings)
	}
}

const fixtureCompleteMissingRuntimeArtifact = `---
ddx:
  id: FIXTURE-COMPLETE-MISSING-RUNTIME-ARTIFACT
---
# Fixture Complete Missing Runtime Artifact

**Status:** Complete

## Requirements

### REQ-042: Observation report

The system MUST retain the observation report artifact.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-042 | .ddx/executions/missing/report.json | test -f .ddx/executions/missing/report.json |
`

// TestMissingRuntimeArtifactDiagnosticIdentifiesRequirementID: a missing
// inspectable runtime-artifact mapping diagnostic includes the requirement
// id from the offending Verification row (not inferred from prose).
func TestMissingRuntimeArtifactDiagnosticIdentifiesRequirementID(t *testing.T) {
	root := t.TempDir()
	// Empty repo root: mapped artifact path does not exist.

	path := "docs/fixtures/complete_missing_runtime_artifact.md"
	status := ParseDocumentStatusMarkdown(path, fixtureCompleteMissingRuntimeArtifact)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteMissingRuntimeArtifact)
	if len(model.Rows) != 1 {
		t.Fatalf("rows = %d, want 1; %+v", len(model.Rows), model.Rows)
	}
	if model.Rows[0].RequirementRef != "REQ-042" {
		t.Fatalf("parsed RequirementRef = %q, want REQ-042", model.Rows[0].RequirementRef)
	}

	findings := CheckRuntimeArtifactResolution(RuntimeArtifactInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Rows:       model.Rows,
		RepoRoot:   root,
	})
	if len(findings) != 1 {
		t.Fatalf("expected exactly one missing-runtime-artifact finding; got %+v", findings)
	}
	f := findings[0]
	if f.Kind != FindingMissingRuntimeArtifact {
		t.Fatalf("Kind = %q, want %q", f.Kind, FindingMissingRuntimeArtifact)
	}
	if f.Severity != SeverityError {
		t.Fatalf("Severity = %q, want %q", f.Severity, SeverityError)
	}
	if f.RequirementRef != "REQ-042" {
		t.Fatalf("RequirementRef = %q, want REQ-042 from offending Verification row", f.RequirementRef)
	}
	if !strings.Contains(f.Message, "REQ-042") {
		t.Fatalf("Message must identify requirement id REQ-042; got %q", f.Message)
	}

	// Convenience path must also surface the row identity.
	docFindings := CheckDocumentRuntimeArtifacts(path, fixtureCompleteMissingRuntimeArtifact, root)
	if len(docFindings) != 1 {
		t.Fatalf("CheckDocumentRuntimeArtifacts: expected 1 finding; got %+v", docFindings)
	}
	if docFindings[0].RequirementRef != "REQ-042" {
		t.Fatalf("CheckDocumentRuntimeArtifacts RequirementRef = %q, want REQ-042", docFindings[0].RequirementRef)
	}
}

// TestMissingRuntimeArtifactDiagnosticIdentifiesSourceDocument: a missing
// inspectable runtime-artifact mapping diagnostic includes the source
// document path containing the offending Verification row.
func TestMissingRuntimeArtifactDiagnosticIdentifiesSourceDocument(t *testing.T) {
	root := t.TempDir()

	path := "docs/fixtures/complete_missing_runtime_artifact.md"
	status := ParseDocumentStatusMarkdown(path, fixtureCompleteMissingRuntimeArtifact)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteMissingRuntimeArtifact)
	if model.Path != path {
		t.Fatalf("model.Path = %q, want %q", model.Path, path)
	}

	findings := CheckRuntimeArtifactResolution(RuntimeArtifactInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Rows:       model.Rows,
		RepoRoot:   root,
	})
	if len(findings) != 1 {
		t.Fatalf("expected exactly one missing-runtime-artifact finding; got %+v", findings)
	}
	f := findings[0]
	if f.Kind != FindingMissingRuntimeArtifact {
		t.Fatalf("Kind = %q, want %q", f.Kind, FindingMissingRuntimeArtifact)
	}
	if f.Path != path {
		t.Fatalf("diagnostic Path = %q, want source document %q", f.Path, path)
	}
	if !strings.Contains(f.Message, path) {
		t.Fatalf("Message must identify source document %q; got %q", path, f.Message)
	}

	// Convenience path records the same document path on the diagnostic.
	docFindings := CheckDocumentRuntimeArtifacts(path, fixtureCompleteMissingRuntimeArtifact, root)
	if len(docFindings) != 1 {
		t.Fatalf("CheckDocumentRuntimeArtifacts: expected 1 finding; got %+v", docFindings)
	}
	if docFindings[0].Path != path {
		t.Fatalf("CheckDocumentRuntimeArtifacts Path = %q, want %q", docFindings[0].Path, path)
	}
}

const fixtureCompleteMissingMappedPathText = `---
ddx:
  id: FIXTURE-COMPLETE-MISSING-MAPPED-PATH-TEXT
---
# Fixture Complete Missing Mapped Path Text

**Status:** Complete

## Requirements

### REQ-050: Relative artifact path

The system MUST report the mapped relative path exactly as written.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-050 | ./artifacts/missing/report.json | test -f ./artifacts/missing/report.json |
`

// TestMissingRuntimeArtifactDiagnosticPreservesMappedPathText: a missing
// runtime-artifact diagnostic reports the relative mapped path exactly as
// written in the Verification row, not a cleaned (./ stripped), absolute,
// or inferred replacement.
func TestMissingRuntimeArtifactDiagnosticPreservesMappedPathText(t *testing.T) {
	root := t.TempDir()

	path := "docs/fixtures/complete_missing_mapped_path_text.md"
	const mappedAsWritten = "./artifacts/missing/report.json"

	status := ParseDocumentStatusMarkdown(path, fixtureCompleteMissingMappedPathText)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteMissingMappedPathText)
	if len(model.Rows) != 1 {
		t.Fatalf("rows = %d, want 1; %+v", len(model.Rows), model.Rows)
	}
	row := model.Rows[0]
	if row.EvidenceTarget != mappedAsWritten {
		t.Fatalf("parsed EvidenceTarget = %q, want mapped text %q", row.EvidenceTarget, mappedAsWritten)
	}

	// Resolver cleans ./ away for existence checks; diagnostic must not.
	res := ResolveRuntimeArtifactRow(root, row)
	if res.Kind != RuntimeArtifactClassRuntime {
		t.Fatalf("Kind = %q, want runtime_artifact", res.Kind)
	}
	if res.Resolved {
		t.Fatalf("missing path must be unresolved; got %+v", res)
	}
	if res.Path == mappedAsWritten {
		t.Fatalf("test precondition failed: resolver Path must differ from mapped text (cleaned); both = %q", res.Path)
	}
	if res.Path != "artifacts/missing/report.json" {
		t.Fatalf("resolver Path = %q, want cleaned artifacts/missing/report.json", res.Path)
	}

	findings := CheckRuntimeArtifactResolution(RuntimeArtifactInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Rows:       model.Rows,
		RepoRoot:   root,
	})
	if len(findings) != 1 {
		t.Fatalf("expected exactly one missing-runtime-artifact finding; got %+v", findings)
	}
	f := findings[0]
	if f.Kind != FindingMissingRuntimeArtifact {
		t.Fatalf("Kind = %q, want %q", f.Kind, FindingMissingRuntimeArtifact)
	}
	if f.ArtifactPath != mappedAsWritten {
		t.Fatalf("ArtifactPath = %q, want mapped text %q (not cleaned %q)", f.ArtifactPath, mappedAsWritten, res.Path)
	}
	if f.EvidenceTarget != mappedAsWritten {
		t.Fatalf("EvidenceTarget = %q, want mapped text %q", f.EvidenceTarget, mappedAsWritten)
	}
	if !strings.Contains(f.Message, mappedAsWritten) {
		t.Fatalf("Message must contain mapped path %q; got %q", mappedAsWritten, f.Message)
	}
	if strings.Contains(f.Message, `"`+res.Path+`"`) && res.Path != mappedAsWritten {
		t.Fatalf("Message must not quote cleaned resolver path %q; got %q", res.Path, f.Message)
	}
	// Must not report an absolute path under the temp root.
	if strings.Contains(f.ArtifactPath, root) || strings.Contains(f.Message, root) {
		t.Fatalf("diagnostic must not use absolute/repo-root path; ArtifactPath=%q Message=%q root=%q",
			f.ArtifactPath, f.Message, root)
	}

	docFindings := CheckDocumentRuntimeArtifacts(path, fixtureCompleteMissingMappedPathText, root)
	if len(docFindings) != 1 {
		t.Fatalf("CheckDocumentRuntimeArtifacts: expected 1 finding; got %+v", docFindings)
	}
	if docFindings[0].ArtifactPath != mappedAsWritten {
		t.Fatalf("CheckDocumentRuntimeArtifacts ArtifactPath = %q, want %q", docFindings[0].ArtifactPath, mappedAsWritten)
	}
}

const fixtureCompleteMissingRowVsResolverPath = `---
ddx:
  id: FIXTURE-COMPLETE-MISSING-ROW-VS-RESOLVER-PATH
---
# Fixture Complete Missing Row Vs Resolver Path

**Status:** Complete

## Requirements

### REQ-051: Backticked artifact path

The system MUST preserve backticked mapped path text in the diagnostic.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-051 | ` + "`.ddx/executions/missing/report.json`" + ` | test -f .ddx/executions/missing/report.json |
`

// TestMissingRuntimeArtifactDiagnosticUsesRowPathNotResolverPath: when the
// resolver's cleaned Path differs from the mapped Verification text (e.g.
// markdown backticks around the path), the missing-artifact diagnostic still
// emits the mapped text, not the resolver path.
func TestMissingRuntimeArtifactDiagnosticUsesRowPathNotResolverPath(t *testing.T) {
	root := t.TempDir()

	path := "docs/fixtures/complete_missing_row_vs_resolver_path.md"
	const mappedAsWritten = "`.ddx/executions/missing/report.json`"
	const resolverPath = ".ddx/executions/missing/report.json"

	status := ParseDocumentStatusMarkdown(path, fixtureCompleteMissingRowVsResolverPath)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteMissingRowVsResolverPath)
	if len(model.Rows) != 1 {
		t.Fatalf("rows = %d, want 1; %+v", len(model.Rows), model.Rows)
	}
	row := model.Rows[0]
	if row.EvidenceTarget != mappedAsWritten {
		t.Fatalf("parsed EvidenceTarget = %q, want mapped text %q", row.EvidenceTarget, mappedAsWritten)
	}

	res := ResolveRuntimeArtifactRow(root, row)
	if res.Kind != RuntimeArtifactClassRuntime {
		t.Fatalf("Kind = %q, want runtime_artifact", res.Kind)
	}
	if res.Resolved {
		t.Fatalf("missing path must be unresolved; got %+v", res)
	}
	if res.Path != resolverPath {
		t.Fatalf("resolver Path = %q, want cleaned %q", res.Path, resolverPath)
	}
	if res.Path == mappedAsWritten {
		t.Fatal("test precondition failed: resolver Path must differ from raw mapped text")
	}

	findings := CheckRuntimeArtifactResolution(RuntimeArtifactInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Rows:       model.Rows,
		RepoRoot:   root,
	})
	if len(findings) != 1 {
		t.Fatalf("expected exactly one missing-runtime-artifact finding; got %+v", findings)
	}
	f := findings[0]
	if f.Kind != FindingMissingRuntimeArtifact {
		t.Fatalf("Kind = %q, want %q", f.Kind, FindingMissingRuntimeArtifact)
	}
	if f.ArtifactPath != mappedAsWritten {
		t.Fatalf("ArtifactPath = %q, want mapped row text %q (not resolver path %q)",
			f.ArtifactPath, mappedAsWritten, resolverPath)
	}
	if f.ArtifactPath == res.Path {
		t.Fatalf("ArtifactPath must not equal resolver Path %q", res.Path)
	}
	if f.EvidenceTarget != mappedAsWritten {
		t.Fatalf("EvidenceTarget = %q, want %q", f.EvidenceTarget, mappedAsWritten)
	}
	if !strings.Contains(f.Message, mappedAsWritten) {
		t.Fatalf("Message must contain mapped text %q; got %q", mappedAsWritten, f.Message)
	}
	// Resolver cleaned path must not replace mapped text in ArtifactPath.
	if f.ArtifactPath == resolverPath {
		t.Fatalf("ArtifactPath must not be the cleaned resolver path %q", resolverPath)
	}

	docFindings := CheckDocumentRuntimeArtifacts(path, fixtureCompleteMissingRowVsResolverPath, root)
	if len(docFindings) != 1 {
		t.Fatalf("CheckDocumentRuntimeArtifacts: expected 1 finding; got %+v", docFindings)
	}
	if docFindings[0].ArtifactPath != mappedAsWritten {
		t.Fatalf("CheckDocumentRuntimeArtifacts ArtifactPath = %q, want %q",
			docFindings[0].ArtifactPath, mappedAsWritten)
	}
}

// TestRuntimeArtifactResolver_ReadOnly: resolving every fixture leaves all
// fixture files byte-identical.
func TestRuntimeArtifactResolver_ReadOnly(t *testing.T) {
	// Package testdata fixtures.
	tdRoot := filepath.Join("testdata")
	beforeTD := snapshotFixtures(t, tdRoot)
	if len(beforeTD) == 0 {
		t.Fatal("expected package testdata fixtures")
	}

	// Resolve against package testdata and each markdown document's rows.
	docsDir := filepath.Join("testdata", "docs")
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		t.Fatalf("readdir docs: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		p := filepath.Join(docsDir, e.Name())
		model, err := ParseVerificationDocument(p)
		if err != nil {
			t.Fatalf("ParseVerificationDocument(%s): %v", p, err)
		}
		_ = ResolveRuntimeArtifactRows(tdRoot, model.Rows)
		for _, row := range model.Rows {
			_ = ResolveRuntimeArtifactTarget(tdRoot, row.EvidenceTarget)
			_ = ClassifyRuntimeArtifactTarget(row.EvidenceTarget)
		}
	}

	// Exercise mixed synthetic rows (existing + missing + out-of-band).
	// Create a temporary artifact tree and resolve against it without
	// touching package fixtures.
	tmp := t.TempDir()
	art := filepath.Join(tmp, ddxroot.DirName, "executions", "fixture", "report.json")
	if err := os.MkdirAll(filepath.Dir(art), 0o755); err != nil {
		t.Fatalf("mkdir tmp artifact: %v", err)
	}
	if err := os.WriteFile(art, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write tmp artifact: %v", err)
	}
	beforeTmp := snapshotFixtures(t, tmp)
	_ = ResolveRuntimeArtifactRows(tmp, []VerificationRow{
		{RequirementRef: "REQ-001", EvidenceTarget: ".ddx/executions/fixture/report.json", Line: 1},
		{RequirementRef: "REQ-002", EvidenceTarget: "TestCreateResource", Line: 2},
		{RequirementRef: "REQ-003", EvidenceTarget: "check:static-delete", Line: 3},
		{RequirementRef: "REQ-004", EvidenceTarget: "missing/path.json", Line: 4},
		{RequirementRef: "REQ-005", EvidenceTarget: "pkg/existing_test.go", Line: 5},
	})
	afterTmp := snapshotFixtures(t, tmp)
	if diffs := diffFixtures(beforeTmp, afterTmp); len(diffs) > 0 {
		t.Fatalf("resolver mutated temp fixtures:\n%s", strings.Join(diffs, "\n"))
	}

	afterTD := snapshotFixtures(t, tdRoot)
	if diffs := diffFixtures(beforeTD, afterTD); len(diffs) > 0 {
		t.Fatalf("resolver mutated package testdata fixtures:\n%s", strings.Join(diffs, "\n"))
	}
}
