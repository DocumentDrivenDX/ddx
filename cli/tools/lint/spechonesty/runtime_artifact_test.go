package spechonesty

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	fixtureRel := filepath.Join(".ddx", "executions", "fixture", "report.json")
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
	rel := filepath.Join(".ddx", "executions", "offline", "report.json")
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
	art := filepath.Join(tmp, ".ddx", "executions", "fixture", "report.json")
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
