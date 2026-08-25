package spechonesty_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/tools/lint/spechonesty"
)

// moduleRoot resolves the cli/ module root from this test package location.
func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	return root
}

// runSpechonesty executes the spechonesty CLI against docsDir via go run.
func runSpechonesty(t *testing.T, docsDir string) (exitCode int, stdout, stderr string) {
	t.Helper()
	return runSpechonestyArgs(t, docsDir)
}

// runSpechonestyArgs executes the spechonesty CLI via go run with args
// passed through verbatim (flags before the docs-dir positional argument,
// matching Go's flag-parsing stop-at-first-non-flag behavior).
func runSpechonestyArgs(t *testing.T, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	root := moduleRoot(t)
	cmdArgs := append([]string{"run", "-buildvcs=false", "./tools/lint/spechonesty/cmd/spechonesty"}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err == nil {
		return 0, outBuf.String(), errBuf.String()
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), outBuf.String(), errBuf.String()
	}
	t.Fatalf("go run spechonesty: %v\nstdout:\n%s\nstderr:\n%s", err, outBuf.String(), errBuf.String())
	return -1, "", ""
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func hashTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		sum := sha256.Sum256(data)
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("hash tree %s: %v", root, err)
	}
	return out
}

// TestSpechonestyCommandExitsNonZeroOnMissingStatus runs the command against
// a fixture docs tree containing an unstamped SD document and expects a
// non-zero exit with a missing-status diagnostic.
func TestSpechonestyCommandExitsNonZeroOnMissingStatus(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "solution-designs", "SD-999-unstamped.md"),
		"---\n"+
			"ddx:\n"+
			"  id: SD-999\n"+
			"---\n"+
			"# Solution Design: Unstamped Fixture\n\n"+
			"No status stamp is present on this design document.\n")

	code, stdout, stderr := runSpechonesty(t, dir)
	combined := stdout + stderr
	if code == 0 {
		t.Fatalf("expected non-zero exit for unstamped SD; stdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if !strings.Contains(combined, "missing_status") && !strings.Contains(strings.ToLower(combined), "missing status") {
		t.Fatalf("expected missing-status diagnostic, got exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

// TestSpechonestyCommandExitsZeroForStampedDocs runs the command against
// fixture docs with body and frontmatter statuses and expects exit zero.
// TD-998 and ADR-998 are Complete/Implemented with Verification rows, so
// this also exercises the WB-1 step 4 current-revision observation-report
// gate: a passing --report/--revision pair is supplied for both documents'
// canonical ids, matching what a real `spechonesty observe` run would
// produce, so the checked-in Verification table alone is not what makes
// the command pass.
func TestSpechonestyCommandExitsZeroForStampedDocs(t *testing.T) {
	const fixtureRevision = "fixture-rev-998"
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "solution-designs", "SD-998-body-status.md"),
		"# Solution Design: Body Status\n\n"+
			"**Status:** Proposed — free-text qualifier should not fail\n\n"+
			"Body stamp only.\n")
	writeFile(t, filepath.Join(dir, "technical-designs", "TD-998-frontmatter-status.md"),
		"---\n"+
			"ddx:\n"+
			"  id: TD-998\n"+
			"status: Complete\n"+
			"---\n"+
			"# Technical Design: Frontmatter Status\n\n"+
			"Frontmatter stamp only.\n\n"+
			"## Verification\n\n"+
			"| Requirement | Evidence | Command |\n"+
			"|-------------|----------|---------|\n"+
			"| REQ-998 | TestFrontmatterStatus | go test ./tools/lint/spechonesty/... |\n")
	writeFile(t, filepath.Join(dir, "adr", "ADR-998-both.md"),
		"---\n"+
			"status: \"In Progress\"\n"+
			"---\n"+
			"# ADR-998: Both Encodings\n\n"+
			"**Status:** Implemented (body wins when both present)\n\n"+
			"## Verification\n\n"+
			"| Requirement | Evidence | Command |\n"+
			"|-------------|----------|---------|\n"+
			"| REQ-998 | TestBothEncodings | go test ./tools/lint/spechonesty/... |\n")

	reportPath := filepath.Join(dir, "observations.json")
	writeObservationReportFixture(t, reportPath, []spechonesty.ObservationReportRow{
		{
			DocumentID:     "TD-998",
			RequirementRef: "REQ-998",
			Command:        "go test ./tools/lint/spechonesty/...",
			Revision:       fixtureRevision,
			ExitCode:       0,
			Evidence:       "PASS",
		},
		{
			DocumentID:     "ADR-998",
			RequirementRef: "REQ-998",
			Command:        "go test ./tools/lint/spechonesty/...",
			Revision:       fixtureRevision,
			ExitCode:       0,
			Evidence:       "PASS",
		},
	})

	code, stdout, stderr := runSpechonestyArgs(t, "--report="+reportPath, "--revision="+fixtureRevision, dir)
	if code != 0 {
		t.Fatalf("expected exit 0 for stamped docs with passing observations; exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

// writeObservationReportFixture writes rows as a JSON observation report,
// matching the format spechonesty observe produces via WriteObservationReport.
func writeObservationReportFixture(t *testing.T, path string, rows []spechonesty.ObservationReportRow) {
	t.Helper()
	if err := spechonesty.WriteObservationReport(path, rows); err != nil {
		t.Fatalf("write observation report fixture %s: %v", path, err)
	}
}

// TestSpechonestyCommandExitsNonZeroOnZeroEvidence ensures a Complete
// document with no Verification rows now surfaces the zero-evidence
// diagnostic through the docs-directory CLI path.
func TestSpechonestyCommandExitsNonZeroOnZeroEvidence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "notes", "complete-zero-evidence.md"),
		"# Complete Document Without Verification Rows\n\n"+
			"**Status:** Complete\n\n"+
			"The document claims completion but carries no Verification block.\n")

	code, stdout, stderr := runSpechonesty(t, dir)
	combined := stdout + stderr
	if code == 0 {
		t.Fatalf("expected non-zero exit for zero-evidence document; stdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if !strings.Contains(combined, "zero_evidence") && !strings.Contains(strings.ToLower(combined), "zero evidence") {
		t.Fatalf("expected zero-evidence diagnostic, got exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(combined, filepath.Join("notes", "complete-zero-evidence.md")) {
		t.Fatalf("expected diagnostic to name the zero-evidence document, got exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

// TestSpechonestyCommandExitsNonZeroOnMissingStaticCheck ensures the
// docs-directory CLI surfaces a missing static-check target.
func TestSpechonestyCommandExitsNonZeroOnMissingStaticCheck(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "notes", "complete-missing-static-check.md"),
		"# Complete Document With Missing Static Check\n\n"+
			"**Status:** Complete\n\n"+
			"## Verification\n\n"+
			"| Requirement | Evidence | Command |\n"+
			"|-------------|----------|---------|\n"+
			"| REQ-001 | check:phantom-but-named | go run ./tools/lint/deletecheck |\n")

	code, stdout, stderr := runSpechonesty(t, dir)
	combined := stdout + stderr
	if code == 0 {
		t.Fatalf("expected non-zero exit for missing static check; stdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if !strings.Contains(combined, "missing_static_check") && !strings.Contains(strings.ToLower(combined), "missing static check") {
		t.Fatalf("expected missing-static-check diagnostic, got exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(combined, "phantom-but-named") {
		t.Fatalf("expected diagnostic to name the missing check, got exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

// TestSpechonestyCommandExitsNonZeroOnMissingRuntimeArtifact ensures the
// docs-directory CLI surfaces missing inspectable runtime-artifact targets.
func TestSpechonestyCommandExitsNonZeroOnMissingRuntimeArtifact(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "notes", "complete-missing-runtime-artifact.md"),
		"# Complete Document With Missing Runtime Artifact\n\n"+
			"**Status:** Complete\n\n"+
			"## Requirements\n\n"+
			"### REQ-001: Artifact\n\n"+
			"The document claims a runtime artifact exists.\n\n"+
			"## Verification\n\n"+
			"| Requirement | Evidence | Command |\n"+
			"|-------------|----------|---------|\n"+
			"| REQ-001 | .ddx/executions/missing/report.json | test -f .ddx/executions/missing/report.json |\n")

	code, stdout, stderr := runSpechonesty(t, dir)
	combined := stdout + stderr
	if code == 0 {
		t.Fatalf("expected non-zero exit for missing runtime artifact; stdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if !strings.Contains(combined, "missing_runtime_artifact") && !strings.Contains(strings.ToLower(combined), "missing runtime artifact") {
		t.Fatalf("expected missing-runtime-artifact diagnostic, got exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(combined, ".ddx/executions/missing/report.json") {
		t.Fatalf("expected diagnostic to name the missing artifact path, got exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

// TestSpechonestyCommandIsReadOnlyOnCorpusFixture asserts the command does
// not write to a docs/helix-shaped corpus even when it emits diagnostics.
func TestSpechonestyCommandIsReadOnlyOnCorpusFixture(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "01-frame", "features", "FEAT-998-clean.md"),
		"# Feature 998: Clean fixture\n\n"+
			"**Status:** Proposed\n\n"+
			"Clean content for the read-only corpus check.\n")
	writeFile(t, filepath.Join(dir, "02-design", "technical-designs", "TD-999-unstamped.md"),
		"---\n"+
			"ddx:\n"+
			"  id: TD-999\n"+
			"---\n"+
			"# TD-999: Unstamped fixture\n\n"+
			"This design deliberately omits a status stamp.\n")

	before := hashTree(t, dir)
	code, stdout, stderr := runSpechonesty(t, dir)
	after := hashTree(t, dir)

	if code == 0 {
		t.Fatalf("expected non-zero exit for bad corpus fixture; stdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "missing_status") && !strings.Contains(strings.ToLower(combined), "missing status") {
		t.Fatalf("expected missing-status diagnostic, got exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if len(before) != len(after) {
		t.Fatalf("file count changed: before=%d after=%d", len(before), len(after))
	}
	for path, sum := range before {
		got, ok := after[path]
		if !ok {
			t.Fatalf("file removed: %s", path)
		}
		if got != sum {
			t.Fatalf("file content changed: %s", path)
		}
	}
}

// TestSpechonestyCommandExitsZeroOnCleanCorpusFixture exercises a clean
// corpus-shaped fixture and expects a zero exit status with no writes.
func TestSpechonestyCommandExitsZeroOnCleanCorpusFixture(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "01-frame", "features", "FEAT-998-clean.md"),
		"# Feature 998: Clean fixture\n\n"+
			"**Status:** Proposed\n\n"+
			"Clean content for the read-only corpus check.\n")

	before := hashTree(t, dir)
	code, stdout, stderr := runSpechonesty(t, dir)
	after := hashTree(t, dir)

	if code != 0 {
		t.Fatalf("expected exit 0 for clean corpus fixture; exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if len(before) != len(after) {
		t.Fatalf("file count changed: before=%d after=%d", len(before), len(after))
	}
	for path, sum := range before {
		got, ok := after[path]
		if !ok {
			t.Fatalf("file removed: %s", path)
		}
		if got != sum {
			t.Fatalf("file content changed: %s", path)
		}
	}
}

// TestSpechonestyCommandExitsNonZeroOnDuplicateUserStoryID ensures the
// docs-directory CLI surfaces duplicate US-id collisions across feature
// documents and names both offending files in the diagnostic.
func TestSpechonestyCommandExitsNonZeroOnDuplicateUserStoryID(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "01-frame", "features", "FEAT-087-alpha.md"),
		"---\n"+
			"ddx:\n"+
			"  id: FEAT-087\n"+
			"---\n"+
			"# FEAT-087 Alpha\n\n"+
			"**Status:** Proposed\n\n"+
			"## User Stories\n\n"+
			"### US-087: Alpha user story\n\n"+
			"Alpha acceptance criteria.\n")
	writeFile(t, filepath.Join(dir, "01-frame", "features", "FEAT-088-beta.md"),
		"---\n"+
			"ddx:\n"+
			"  id: FEAT-088\n"+
			"---\n"+
			"# FEAT-088 Beta\n\n"+
			"**Status:** Proposed\n\n"+
			"## User Stories\n\n"+
			"### US-087: Beta user story\n\n"+
			"Beta acceptance criteria.\n")

	code, stdout, stderr := runSpechonesty(t, dir)
	combined := stdout + stderr
	if code == 0 {
		t.Fatalf("expected non-zero exit for duplicate US-id; stdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if !strings.Contains(combined, "duplicate_us_id") && !strings.Contains(strings.ToLower(combined), "duplicate user-story id") {
		t.Fatalf("expected duplicate US-id diagnostic, got exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(combined, filepath.Join("01-frame", "features", "FEAT-087-alpha.md")) {
		t.Fatalf("expected diagnostic to name first feature file, got exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(combined, filepath.Join("01-frame", "features", "FEAT-088-beta.md")) {
		t.Fatalf("expected diagnostic to name second feature file, got exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

// TestSpechonestyCommandReportsCorpusWB1Diagnostics exercises the real
// docs/helix corpus and checks the WB-1 diagnostics that motivated this bead.
func TestSpechonestyCommandReportsCorpusWB1Diagnostics(t *testing.T) {
	root := moduleRoot(t)
	docsDir := filepath.Join(root, "..", "docs", "helix")

	code, stdout, stderr := runSpechonesty(t, docsDir)
	combined := stdout + stderr
	if code == 0 {
		t.Fatalf("expected non-zero exit for current docs/helix corpus; stdout:\n%s\nstderr:\n%s", stdout, stderr)
	}

	expectations := []string{
		filepath.Join("docs", "helix", "02-design", "solution-designs", "SD-013-multi-agent-coordination.md") + ":1: missing_status:",
		filepath.Join("docs", "helix", "02-design", "technical-designs", "TD-027-bead-collection-abstraction.md") + ":1: missing_status:",
		filepath.Join("docs", "helix", "02-design", "technical-designs", "TD-040-cross-repo-blocker-recheck.md") + ":3: duplicate_id:",
		filepath.Join("docs", "helix", "01-frame", "features", "FEAT-008-web-ui.md") + ":739: duplicate_us_id:",
		filepath.Join("docs", "helix", "01-frame", "features", "FEAT-020-server-node-state.md"),
		"US-087",
		"US-088",
	}
	for _, want := range expectations {
		if !strings.Contains(combined, want) {
			t.Fatalf("expected corpus diagnostic containing %q, got exit=%d\nstdout:\n%s\nstderr:\n%s", want, code, stdout, stderr)
		}
	}
}

// TestSpechonestyCommandIsReadOnly asserts fixture file contents are
// byte-identical before and after command execution.
func TestSpechonestyCommandIsReadOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "solution-designs", "SD-997-readonly.md"),
		"# Solution Design: Read-Only Check\n\n"+
			"**Status:** Deferred\n\n"+
			"Contents must not change.\n")
	writeFile(t, filepath.Join(dir, "notes", "README.md"),
		"# Non-design note without status\n\n"+
			"Should not be mutated either.\n")

	before := hashTree(t, dir)
	_, _, _ = runSpechonesty(t, dir)
	after := hashTree(t, dir)

	if len(before) != len(after) {
		t.Fatalf("file count changed: before=%d after=%d", len(before), len(after))
	}
	for path, sum := range before {
		got, ok := after[path]
		if !ok {
			t.Fatalf("file removed: %s", path)
		}
		if got != sum {
			t.Fatalf("file content changed: %s", path)
		}
	}
}
