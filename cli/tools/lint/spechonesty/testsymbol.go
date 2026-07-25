// Go Test* evidence-target resolver for Complete/Implemented documents
// (phase2-doc-truth-plan WB-1 steps 3-4).
//
// After the Verification mapping-parser exposes row targets, this pass
// resolves mapped Test* evidence targets to actual Go test function
// symbols in the referenced package/file scope. A test-file path without
// the specific Test* symbol it proves is not coverage.
//
// Static checks, runtime artifacts, command allowlists, coverage
// cardinality, and waivers are sibling children. Read-only: never
// mutates documents or fixtures.
package spechonesty

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// TestSymbolFindingKind classifies a Go test-symbol resolution failure.
type TestSymbolFindingKind string

const (
	// FindingMissingTestSymbol is emitted when a mapped Test* target does
	// not exist in the referenced package/file scope.
	FindingMissingTestSymbol TestSymbolFindingKind = "missing_test_symbol"
	// FindingFileOnlyTestEvidence is emitted when evidence is only a Go
	// test file path without a specific Test* target.
	FindingFileOnlyTestEvidence TestSymbolFindingKind = "file_only_test_evidence"
)

// TestSymbolFinding is one diagnostic from the Go test-symbol resolver.
type TestSymbolFinding struct {
	// Path is the document path recorded on the diagnostic.
	Path string
	// Line is the 1-based Verification mapping row line (0 when unknown).
	Line int
	// RequirementRef is the requirement/anchor the row covers.
	RequirementRef string
	// EvidenceTarget is the raw evidence cell that failed resolution.
	EvidenceTarget string
	// Symbol is the Test* identifier when known (empty for file-only rows).
	Symbol string
	// Kind classifies the failure.
	Kind TestSymbolFindingKind
	// Severity is always SeverityError for Complete/Implemented target failures.
	Severity FindingSeverity
	// Message is a human-readable description naming the missing symbol
	// or explaining that file-only evidence is not target coverage.
	Message string
}

// TestSymbolInput is the document-level input for the Go test-symbol resolver.
// Callers supply status and rows from the status and Verification parsers;
// RepoRoot is the filesystem root used to locate Go packages and test files.
type TestSymbolInput struct {
	// Path is the document path recorded on diagnostics.
	Path string
	// Status is the normalized base document status.
	Status DocStatus
	// StatusLine is the 1-based line of the status stamp (0 → 1).
	StatusLine int
	// Rows is the well-formed Verification mapping row set.
	Rows []VerificationRow
	// RepoRoot is the repository (or fixture) root against which package
	// and file scopes are resolved. Required for filesystem resolution.
	RepoRoot string
}

var (
	// testSymbolIdentRe is a Go exported test function identifier.
	testSymbolIdentRe = regexp.MustCompile(`^Test[A-Za-z0-9_]+$`)
	// goTestPkgRe extracts the package path from a `go test ./pkg` command.
	goTestPkgRe = regexp.MustCompile(`(?i)\bgo\s+test\b(?:\s+-[^\s]+)*\s+(\./[^\s]+|[^\s-]+[^\s]*)`)
	// goTestRunRe extracts -run TestName from a go test command.
	goTestRunRe = regexp.MustCompile(`(?i)-run(?:=|\s+)([^\s]+)`)
)

// CheckTestSymbolResolution resolves mapped Test* evidence targets for
// Complete/Implemented documents against Go sources under RepoRoot.
//
// Rules:
//   - Non-Complete/Implemented statuses → no findings.
//   - Evidence that is only a Go test-file path (no Test* symbol) →
//     FindingFileOnlyTestEvidence (file-only evidence is not target coverage).
//   - Evidence naming a Test* symbol that does not exist in the referenced
//     package/file scope → FindingMissingTestSymbol naming the symbol.
//   - Evidence naming an existing Test* symbol in scope → no diagnostic.
//   - Non-test evidence (static checks, runtime artifacts) is ignored.
//
// Read-only: opens files for parse only; never writes.
func CheckTestSymbolResolution(in TestSymbolInput) []TestSymbolFinding {
	if !IsCompleteStatus(in.Status) {
		return nil
	}
	if len(in.Rows) == 0 {
		return nil
	}

	var index *testSymbolIndex
	var findings []TestSymbolFinding

	for _, row := range in.Rows {
		target := strings.TrimSpace(row.EvidenceTarget)
		if target == "" {
			continue
		}

		parsed := parseTestEvidenceTarget(target, row.Command)
		switch parsed.kind {
		case evidenceKindIgnore:
			continue
		case evidenceKindFileOnly:
			findings = append(findings, TestSymbolFinding{
				Path:           in.Path,
				Line:           row.Line,
				RequirementRef: row.RequirementRef,
				EvidenceTarget: target,
				Kind:           FindingFileOnlyTestEvidence,
				Severity:       SeverityError,
				Message: fmt.Sprintf(
					"%s: file-only test evidence %q is not target coverage; name the specific Test* symbol the file proves",
					in.Path, target,
				),
			})
			continue
		case evidenceKindTestSymbol:
			if index == nil {
				index = buildTestSymbolIndex(in.RepoRoot)
			}
			if index.has(parsed.scope, parsed.symbol) {
				continue
			}
			findings = append(findings, TestSymbolFinding{
				Path:           in.Path,
				Line:           row.Line,
				RequirementRef: row.RequirementRef,
				EvidenceTarget: target,
				Symbol:         parsed.symbol,
				Kind:           FindingMissingTestSymbol,
				Severity:       SeverityError,
				Message: fmt.Sprintf(
					"%s: missing test symbol %q (requirement %q); no matching Test* function in referenced package/file scope",
					in.Path, parsed.symbol, row.RequirementRef,
				),
			})
		}
	}
	return findings
}

// CheckDocumentTestSymbols parses status and Verification rows for content
// and runs the Go test-symbol resolver against repoRoot. Convenience for
// fixture tests; read-only over both the markdown content and the repo tree.
func CheckDocumentTestSymbols(path, content, repoRoot string) []TestSymbolFinding {
	status := ParseDocumentStatusMarkdown(path, content)
	model := ParseVerificationMarkdown(path, content)
	return CheckTestSymbolResolution(TestSymbolInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Rows:       model.Rows,
		RepoRoot:   repoRoot,
	})
}

type evidenceKind int

const (
	evidenceKindIgnore evidenceKind = iota
	evidenceKindFileOnly
	evidenceKindTestSymbol
)

type parsedTestEvidence struct {
	kind   evidenceKind
	symbol string
	// scope is a file path (*.go) or package directory relative to RepoRoot,
	// or empty when the symbol may resolve anywhere under the root.
	scope string
}

// parseTestEvidenceTarget classifies a Verification evidence cell.
// Command may supply package scope via `go test ./pkg` when the cell is a bare Test*.
func parseTestEvidenceTarget(target, command string) parsedTestEvidence {
	target = strings.TrimSpace(target)
	if target == "" {
		return parsedTestEvidence{kind: evidenceKindIgnore}
	}

	// Strip surrounding backticks / bold common in markdown cells.
	target = strings.Trim(target, "`* ")

	// Static checks and non-test named checks are sibling territory.
	if strings.HasPrefix(strings.ToLower(target), "check:") {
		return parsedTestEvidence{kind: evidenceKindIgnore}
	}

	// path:TestSymbol or path#TestSymbol (file or package scope).
	if sym, scope, ok := splitScopedTestSymbol(target); ok {
		return parsedTestEvidence{kind: evidenceKindTestSymbol, symbol: sym, scope: scope}
	}

	// Bare Test* identifier.
	if testSymbolIdentRe.MatchString(target) {
		scope := packageScopeFromCommand(command)
		return parsedTestEvidence{kind: evidenceKindTestSymbol, symbol: target, scope: scope}
	}

	// File-only Go test path (with optional line/column suffix noise).
	if isGoTestFilePath(target) {
		return parsedTestEvidence{kind: evidenceKindFileOnly, scope: normalizePath(target)}
	}

	// Other paths/artifacts are not this resolver's concern.
	return parsedTestEvidence{kind: evidenceKindIgnore}
}

func splitScopedTestSymbol(target string) (symbol, scope string, ok bool) {
	// Prefer the rightmost separator so Windows-ish drive letters are not special-cased;
	// scopes here are repo-relative POSIX paths.
	for _, sep := range []string{":", "#"} {
		if i := strings.LastIndex(target, sep); i > 0 && i < len(target)-1 {
			left := strings.TrimSpace(target[:i])
			right := strings.TrimSpace(target[i+1:])
			if testSymbolIdentRe.MatchString(right) && left != "" {
				// Reject URL-like schemes (http:, https:) — not package scopes.
				if strings.Contains(left, "://") {
					continue
				}
				return right, normalizePath(left), true
			}
		}
	}
	// pkg.TestSymbol — only when the right side is Test* and left looks like a path/package
	// (contains / or is a simple package name, not a sentence).
	if i := strings.LastIndex(target, "."); i > 0 && i < len(target)-1 {
		left := strings.TrimSpace(target[:i])
		right := strings.TrimSpace(target[i+1:])
		if testSymbolIdentRe.MatchString(right) && looksLikePackageOrFileScope(left) {
			return right, normalizePath(left), true
		}
	}
	return "", "", false
}

func looksLikePackageOrFileScope(s string) bool {
	if s == "" {
		return false
	}
	// File path.
	if strings.HasSuffix(s, ".go") {
		return true
	}
	// Relative package path.
	if strings.HasPrefix(s, "./") || strings.Contains(s, "/") {
		return true
	}
	// Single path segment that is a valid package-like identifier.
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func isGoTestFilePath(target string) bool {
	// Strip trailing :line or :line:col that may appear in citations.
	p := target
	if i := strings.LastIndex(p, ":"); i > 0 {
		suffix := p[i+1:]
		if isDigits(suffix) || isLineCol(suffix) {
			p = p[:i]
		}
	}
	p = normalizePath(p)
	base := filepath.Base(p)
	if !strings.HasSuffix(base, ".go") {
		return false
	}
	// Treat any *.go citation without a Test* symbol as file-only when it
	// looks like a path (contains / or ends with _test.go). Production docs
	// cite *_test.go files; also reject non-test .go file-only citations the
	// same way when path-like, so bare prose is not misclassified.
	if strings.HasSuffix(base, "_test.go") {
		return true
	}
	return strings.Contains(p, "/") && strings.HasSuffix(base, ".go")
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isLineCol(s string) bool {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return false
	}
	return isDigits(parts[0]) && isDigits(parts[1])
}

func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, "`\"'")
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	return filepath.ToSlash(p)
}

func packageScopeFromCommand(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	// Prefer an explicit -run package context via go test ./pkg
	m := goTestPkgRe.FindStringSubmatch(command)
	if m == nil {
		return ""
	}
	pkg := m[1]
	// Skip flags mistaken as packages.
	if strings.HasPrefix(pkg, "-") {
		return ""
	}
	return normalizePath(pkg)
}

// testSymbolIndex maps Test* names to the files (and package dirs) that define them.
type testSymbolIndex struct {
	// bySymbol lists relative file paths that define each symbol.
	bySymbol map[string][]string
	// fileHas[file][symbol] for exact file-scope lookups.
	fileHas map[string]map[string]bool
}

func (idx *testSymbolIndex) has(scope, symbol string) bool {
	if idx == nil || symbol == "" {
		return false
	}
	symbol = strings.TrimSpace(symbol)
	scope = normalizePath(scope)

	if scope == "" {
		_, ok := idx.bySymbol[symbol]
		return ok
	}

	// File scope: scope ends with .go
	if strings.HasSuffix(scope, ".go") {
		if m := idx.fileHas[scope]; m != nil && m[symbol] {
			return true
		}
		// Also try with/without leading segments if callers used absolute-ish paths.
		for file, m := range idx.fileHas {
			if m[symbol] && (file == scope || strings.HasSuffix(file, "/"+scope) || strings.HasSuffix(scope, "/"+file)) {
				return true
			}
		}
		return false
	}

	// Package directory scope: any defining file under that directory.
	scope = strings.TrimSuffix(scope, "/")
	for _, file := range idx.bySymbol[symbol] {
		dir := filepath.ToSlash(filepath.Dir(file))
		if dir == scope || strings.HasPrefix(dir, scope+"/") || strings.HasSuffix(dir, "/"+scope) {
			return true
		}
		// Bare package name match on the last path segment.
		if filepath.Base(dir) == scope || filepath.Base(dir) == filepath.Base(scope) {
			// Prefer exact: ./pkg matches .../pkg
			if scope == filepath.Base(dir) || strings.HasSuffix(dir, "/"+strings.TrimPrefix(scope, "./")) {
				return true
			}
		}
	}
	return false
}

func buildTestSymbolIndex(repoRoot string) *testSymbolIndex {
	idx := &testSymbolIndex{
		bySymbol: make(map[string][]string),
		fileHas:  make(map[string]map[string]bool),
	}
	if strings.TrimSpace(repoRoot) == "" {
		return idx
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return idx
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return idx
	}

	fset := token.NewFileSet()
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			// Skip common heavy/irrelevant trees.
			if name == "vendor" || name == "node_modules" || name == ".git" || name == "testdata" {
				// Still allow walking our own fixture testdata when repoRoot IS that tree;
				// only skip nested testdata dirs one level down from non-root.
				if path != root {
					return fs.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		// Skip build-tag heavy files? Parse all; parser ignores body errors lightly.
		syms := parseTestSymbolsInFile(fset, path)
		if len(syms) == 0 {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		if idx.fileHas[rel] == nil {
			idx.fileHas[rel] = make(map[string]bool)
		}
		for _, sym := range syms {
			idx.fileHas[rel][sym] = true
			idx.bySymbol[sym] = append(idx.bySymbol[sym], rel)
		}
		return nil
	})
	return idx
}

func parseTestSymbolsInFile(fset *token.FileSet, path string) []string {
	// Parse only the file; SkipObjectResolution keeps this cheap and read-only.
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		// Best-effort: try reading bytes if path-based parse fails in odd FS cases.
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		f, err = parser.ParseFile(fset, path, data, parser.SkipObjectResolution)
		if err != nil {
			return nil
		}
	}
	var out []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		name := fn.Name.Name
		if !strings.HasPrefix(name, "Test") {
			continue
		}
		// Match go test's discovery: top-level func Test*(t *testing.T) roughly;
		// accept any Test* function (including TestMain) for resolution purposes
		// except require at least "Test" + one more character already enforced.
		if !testSymbolIdentRe.MatchString(name) {
			continue
		}
		// Methods (receivers) are not go test entrypoints.
		if fn.Recv != nil {
			continue
		}
		out = append(out, name)
	}
	return out
}
