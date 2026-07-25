// Runtime-artifact evidence-target classifier and path resolver for
// Verification mapping rows (phase2-doc-truth-plan WB-1 steps 3-4).
//
// Classifies a parsed mapping row's evidence target as an inspectable
// runtime artifact (or out-of-band for this resolver), then resolves
// inspectable targets to a repository path or generated fixture artifact
// path without network access. Pure read-only lookup: returns resolution
// results only — never emits analyzer diagnostics and never writes files.
//
// Diagnostic emission for missing artifacts is a sibling child's job.
// Test* symbols, static checks (check:*), command allowlists, coverage
// cardinality, and waivers are out of scope here.
package spechonesty

import (
	"os"
	"path/filepath"
	"strings"
)

// RuntimeArtifactClassKind is the classification of an evidence target
// relative to this resolver.
type RuntimeArtifactClassKind string

const (
	// RuntimeArtifactClassRuntime marks an inspectable runtime-artifact path.
	RuntimeArtifactClassRuntime RuntimeArtifactClassKind = "runtime_artifact"
	// RuntimeArtifactClassOutOfBand marks targets owned by other resolvers
	// (Test* symbols, static checks, empty/non-path evidence).
	RuntimeArtifactClassOutOfBand RuntimeArtifactClassKind = "out_of_band"
)

// RuntimeArtifactClassification is the result of classifying one evidence target.
type RuntimeArtifactClassification struct {
	// Kind is runtime_artifact or out_of_band.
	Kind RuntimeArtifactClassKind
	// Path is the normalized path string when Kind is runtime_artifact;
	// empty when out of band.
	Path string
	// Target is the original evidence cell after light markdown trimming.
	Target string
}

// RuntimeArtifactResolution is the result of resolving one runtime-artifact
// evidence target against a repository (or fixture) root.
//
// For out-of-band targets, Resolved is false, Path is empty, and Kind is
// out_of_band. For runtime-artifact targets, Path always carries the
// offending/normalized path string from the mapping row; Resolved is true
// only when that path exists under RepoRoot.
type RuntimeArtifactResolution struct {
	// Kind is the classification of the evidence target.
	Kind RuntimeArtifactClassKind
	// Target is the original evidence cell after light markdown trimming.
	Target string
	// Path is the normalized path string from the mapping row when Kind is
	// runtime_artifact (present for both resolved and unresolved cases so
	// callers can name the offending path). Empty when out of band.
	Path string
	// Resolved is true when Kind is runtime_artifact and the path exists
	// under RepoRoot as a file or directory.
	Resolved bool
	// ResolvedPath is the absolute filesystem path when Resolved is true.
	ResolvedPath string
	// RequirementRef and Line are copied from the mapping row when resolving
	// rows; zero values when resolving a bare target string.
	RequirementRef string
	Line           int
}

// ClassifyRuntimeArtifactTarget reports whether evidenceTarget names an
// inspectable runtime artifact path for this resolver.
//
// Out of band (not this resolver's concern):
//   - empty / whitespace-only cells
//   - check:… deterministic static checks
//   - bare or scoped Test* symbols
//   - Go test-file paths without a Test* symbol (file-only test evidence)
//   - non-path free text
//
// Runtime artifact: path-like evidence (repo-relative paths, .ddx/…
// generated fixture artifacts, extension-bearing file names) that is not
// claimed by the Test*/static-check siblings.
//
// Pure: no filesystem or network access.
func ClassifyRuntimeArtifactTarget(evidenceTarget string) RuntimeArtifactClassification {
	target := stripEvidenceMarkup(evidenceTarget)
	if target == "" {
		return RuntimeArtifactClassification{
			Kind:   RuntimeArtifactClassOutOfBand,
			Target: target,
		}
	}

	// Static checks belong to the static-check / evidence-target siblings.
	if strings.HasPrefix(strings.ToLower(target), "check:") {
		return RuntimeArtifactClassification{
			Kind:   RuntimeArtifactClassOutOfBand,
			Target: target,
		}
	}

	// Go Test* symbols (bare or scoped) belong to the test-symbol resolver.
	if _, _, ok := splitScopedTestSymbol(target); ok {
		return RuntimeArtifactClassification{
			Kind:   RuntimeArtifactClassOutOfBand,
			Target: target,
		}
	}
	if testSymbolIdentRe.MatchString(target) {
		return RuntimeArtifactClassification{
			Kind:   RuntimeArtifactClassOutOfBand,
			Target: target,
		}
	}

	// File-only Go test paths are not runtime artifacts; the test-symbol
	// sibling owns the "file-only is not target coverage" diagnostic.
	if isGoTestFilePath(target) {
		return RuntimeArtifactClassification{
			Kind:   RuntimeArtifactClassOutOfBand,
			Target: target,
		}
	}

	if !looksLikeRuntimeArtifactPath(target) {
		return RuntimeArtifactClassification{
			Kind:   RuntimeArtifactClassOutOfBand,
			Target: target,
		}
	}

	path := normalizePath(target)
	return RuntimeArtifactClassification{
		Kind:   RuntimeArtifactClassRuntime,
		Path:   path,
		Target: target,
	}
}

// ResolveRuntimeArtifactTarget classifies evidenceTarget and, when it is a
// runtime artifact, resolves it under repoRoot.
//
// Resolution is local filesystem only: os.Stat against repoRoot. No network
// access, no writes, no diagnostics. Unresolved targets still carry Path as
// the normalized mapping-row path string.
func ResolveRuntimeArtifactTarget(repoRoot, evidenceTarget string) RuntimeArtifactResolution {
	class := ClassifyRuntimeArtifactTarget(evidenceTarget)
	out := RuntimeArtifactResolution{
		Kind:   class.Kind,
		Target: class.Target,
		Path:   class.Path,
	}
	if class.Kind != RuntimeArtifactClassRuntime {
		return out
	}
	return resolveRuntimeArtifactPath(repoRoot, out)
}

// ResolveRuntimeArtifactRow resolves one Verification mapping row's evidence
// target. Copies RequirementRef and Line onto the result for callers that
// later emit diagnostics (sibling child).
func ResolveRuntimeArtifactRow(repoRoot string, row VerificationRow) RuntimeArtifactResolution {
	res := ResolveRuntimeArtifactTarget(repoRoot, row.EvidenceTarget)
	res.RequirementRef = row.RequirementRef
	res.Line = row.Line
	return res
}

// ResolveRuntimeArtifactRows resolves every row independently. Out-of-band
// rows appear in the result with Kind=out_of_band so callers can skip them
// without re-classifying. Never mutates rows or the filesystem under
// repoRoot beyond read-only existence checks.
func ResolveRuntimeArtifactRows(repoRoot string, rows []VerificationRow) []RuntimeArtifactResolution {
	if len(rows) == 0 {
		return nil
	}
	out := make([]RuntimeArtifactResolution, 0, len(rows))
	for _, row := range rows {
		out = append(out, ResolveRuntimeArtifactRow(repoRoot, row))
	}
	return out
}

func resolveRuntimeArtifactPath(repoRoot string, out RuntimeArtifactResolution) RuntimeArtifactResolution {
	if out.Path == "" {
		return out
	}
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		// No root: cannot resolve; report unresolved with the offending path.
		return out
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return out
	}
	// Reject absolute targets that escape the repository root; treat them as
	// path strings relative only when they stay under root, otherwise still
	// attempt a direct existence check under the joined root-relative form.
	candidate := out.Path
	if filepath.IsAbs(candidate) {
		// Absolute paths are only accepted when they already live under root.
		rel, relErr := filepath.Rel(absRoot, candidate)
		if relErr != nil || strings.HasPrefix(rel, "..") {
			// Offending path remains out.Path; unresolved.
			return out
		}
		candidate = rel
	}
	full := filepath.Join(absRoot, filepath.FromSlash(candidate))
	// Clean and re-check confinement after join.
	full = filepath.Clean(full)
	rel, relErr := filepath.Rel(absRoot, full)
	if relErr != nil || strings.HasPrefix(rel, "..") {
		return out
	}
	info, statErr := os.Stat(full)
	if statErr != nil {
		return out
	}
	// Any existing path (file or directory) is inspectable.
	_ = info
	out.Resolved = true
	out.ResolvedPath = full
	return out
}

// stripEvidenceMarkup trims whitespace and common markdown cell wrappers.
func stripEvidenceMarkup(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Strip a single layer of surrounding backticks / bold markers.
	s = strings.Trim(s, "`* ")
	s = strings.TrimSpace(s)
	return s
}

// looksLikeRuntimeArtifactPath reports whether s is a path-like evidence
// target that can name a repository or generated-fixture artifact.
func looksLikeRuntimeArtifactPath(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// Explicit URL schemes are not local inspectable artifacts.
	if strings.Contains(s, "://") {
		return false
	}
	// Path separators → repository- or fixture-relative path.
	if strings.Contains(s, "/") || strings.Contains(s, `\`) {
		return true
	}
	// Dot-relative single segments (.ddx, ./report.json after normalize, etc.).
	if strings.HasPrefix(s, ".") {
		return true
	}
	// Bare filename with an extension (report.json, observation.xml).
	base := s
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if i := strings.LastIndex(base, "."); i > 0 && i < len(base)-1 {
		ext := base[i+1:]
		// Reject pure numeric "extensions" (not file types).
		if isDigits(ext) {
			return false
		}
		// Reject Go source as bare names without a path — those are not
		// "runtime" artifacts; multi-segment .go paths are handled above.
		if strings.EqualFold(ext, "go") {
			return false
		}
		return true
	}
	return false
}
