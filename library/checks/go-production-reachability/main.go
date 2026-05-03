// Package main implements the Go-specific production-reachability check
// conforming to the REACH-PROTO check protocol (see cli/internal/checks/protocol.go).
//
// It reports newly-added Go functions/methods that are not reachable from the
// project's configured entry roots, using golang.org/x/tools/cmd/deadcode
// (Rapid Type Analysis) as the underlying reachability engine.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type violation struct {
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
	Symbol string `json:"symbol,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type result struct {
	Status     string      `json:"status"`
	Message    string      `json:"message,omitempty"`
	Violations []violation `json:"violations,omitempty"`
}

func main() {
	var (
		moduleDir     string
		packagesArg   string
		deadcodeBin   string
		beadsFile     string
		checkName     string
	)
	flag.StringVar(&moduleDir, "module-dir", ".", "Directory holding the go.mod from which deadcode is run (relative to PROJECT_ROOT)")
	flag.StringVar(&packagesArg, "packages", "./...", "Comma-separated package patterns passed to deadcode")
	flag.StringVar(&deadcodeBin, "deadcode", "", "Path to deadcode binary; default uses 'go run golang.org/x/tools/cmd/deadcode@v0.42.0'")
	flag.StringVar(&beadsFile, "beads-file", ".ddx/beads.jsonl", "Path to beads.jsonl (relative to PROJECT_ROOT)")
	flag.StringVar(&checkName, "name", "", "Override check name (default from CHECK_NAME env)")
	flag.Parse()

	projectRoot := envOr("PROJECT_ROOT", "")
	evidenceDir := envOr("EVIDENCE_DIR", "")
	diffBase := envOr("DIFF_BASE", "")
	diffHead := envOr("DIFF_HEAD", "HEAD")
	if checkName == "" {
		checkName = envOr("CHECK_NAME", "production-reachability")
	}
	if projectRoot == "" || evidenceDir == "" {
		emitFatal(evidenceDir, checkName, "missing required env: PROJECT_ROOT and EVIDENCE_DIR must be set")
		return
	}
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		emitFatal(evidenceDir, checkName, "create evidence dir: "+err.Error())
		return
	}

	res := run(runArgs{
		ProjectRoot: projectRoot,
		ModuleDir:   moduleDir,
		Packages:    splitCSV(packagesArg),
		DeadcodeBin: deadcodeBin,
		BeadsFile:   filepath.Join(projectRoot, beadsFile),
		DiffBase:    diffBase,
		DiffHead:    diffHead,
		EntryRoots:  splitCSV(packagesArg),
	})

	writeResult(filepath.Join(evidenceDir, checkName+".json"), res)
}

type runArgs struct {
	ProjectRoot string
	ModuleDir   string
	Packages    []string
	EntryRoots  []string
	DeadcodeBin string
	BeadsFile   string
	DiffBase    string
	DiffHead    string
}

func run(a runArgs) result {
	if a.DiffBase == "" || a.DiffBase == a.DiffHead {
		return result{Status: "pass", Message: "no diff range; nothing to evaluate"}
	}

	changedGoFiles, err := gitChangedGoFiles(a.ProjectRoot, a.DiffBase, a.DiffHead)
	if err != nil {
		return result{Status: "error", Message: "git diff: " + err.Error()}
	}
	if len(changedGoFiles) == 0 {
		return result{Status: "pass", Message: "no non-test Go files changed"}
	}

	newSyms, err := newSymbols(a.ProjectRoot, a.DiffBase, changedGoFiles)
	if err != nil {
		return result{Status: "error", Message: "scan new symbols: " + err.Error()}
	}
	if len(newSyms) == 0 {
		return result{Status: "pass", Message: "no new top-level functions or methods added"}
	}

	deadSet, err := runDeadcode(a)
	if err != nil {
		return result{Status: "error", Message: "deadcode: " + err.Error()}
	}

	var violations []violation
	for _, s := range newSyms {
		if !isDead(s, deadSet) {
			continue
		}
		ann := s.Annotation
		if ann != "" {
			ok, why := validatePending(a.BeadsFile, ann)
			if ok {
				continue
			}
			violations = append(violations, violation{
				File:   relPath(a.ProjectRoot, s.AbsFile),
				Line:   s.Line,
				Symbol: s.Symbol,
				Kind:   "invalid_annotation",
				Detail: fmt.Sprintf("// wiring:pending %s rejected: %s", ann, why),
			})
			continue
		}
		violations = append(violations, violation{
			File:   relPath(a.ProjectRoot, s.AbsFile),
			Line:   s.Line,
			Symbol: s.Symbol,
			Kind:   "unreachable",
			Detail: fmt.Sprintf("no path from entry roots: %s", strings.Join(a.EntryRoots, ",")),
		})
	}

	if len(violations) == 0 {
		return result{Status: "pass", Message: fmt.Sprintf("%d new symbol(s) all reachable from entry roots", len(newSyms))}
	}
	return result{
		Status:     "block",
		Message:    fmt.Sprintf("%d new symbol(s) unreachable from entry roots", len(violations)),
		Violations: violations,
	}
}

// symbol describes one newly-added top-level function or method declaration
// from the worktree side of the diff.
type symbol struct {
	AbsFile    string // absolute path
	Line       int    // declaration line in HEAD
	Symbol     string // "Func" or "(*T).Method" or "T.Method"
	Annotation string // bead-id named in `// wiring:pending <bead-id>`, else ""
}

// newSymbols returns top-level FuncDecls present in the worktree (HEAD)
// version of each changed file but absent from the DIFF_BASE version,
// keyed by (file, qualified symbol name). Test files are skipped.
func newSymbols(projectRoot, diffBase string, files []string) ([]symbol, error) {
	var out []symbol
	for _, rel := range files {
		abs := filepath.Join(projectRoot, rel)
		newSrc, err := os.ReadFile(abs)
		if err != nil {
			// File deleted in HEAD — nothing to add.
			continue
		}
		oldSrc, _ := gitShow(projectRoot, diffBase, rel) // empty on add
		oldDecls := declSet(oldSrc, rel)
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, abs, newSrc, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			qn := qualName(fd)
			if _, existed := oldDecls[qn]; existed {
				continue
			}
			pos := fset.Position(fd.Pos())
			out = append(out, symbol{
				AbsFile:    abs,
				Line:       pos.Line,
				Symbol:     qn,
				Annotation: extractPendingBead(fd),
			})
		}
	}
	return out, nil
}

func declSet(src []byte, displayName string) map[string]struct{} {
	out := map[string]struct{}{}
	if len(src) == 0 {
		return out
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, displayName, src, 0)
	if err != nil {
		return out
	}
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok {
			out[qualName(fd)] = struct{}{}
		}
	}
	return out
}

// qualName returns deadcode-style qualified name: "Func" for plain functions,
// "T.Method" for value-receiver methods, "(*T).Method" for pointer-receiver.
func qualName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	recv := fd.Recv.List[0].Type
	switch t := recv.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return fmt.Sprintf("(*%s).%s", id.Name, fd.Name.Name)
		}
	case *ast.Ident:
		return fmt.Sprintf("%s.%s", t.Name, fd.Name.Name)
	case *ast.IndexExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return fmt.Sprintf("%s.%s", id.Name, fd.Name.Name)
		}
	case *ast.IndexListExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return fmt.Sprintf("%s.%s", id.Name, fd.Name.Name)
		}
	}
	return fd.Name.Name
}

// extractPendingBead scans the doc comment block immediately preceding a
// FuncDecl for `// wiring:pending <bead-id>`. Returns the bead id or "".
func extractPendingBead(fd *ast.FuncDecl) string {
	if fd.Doc == nil {
		return ""
	}
	for _, c := range fd.Doc.List {
		txt := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		txt = strings.TrimSpace(strings.TrimPrefix(txt, "/*"))
		txt = strings.TrimSuffix(txt, "*/")
		txt = strings.TrimSpace(txt)
		const tag = "wiring:pending"
		if strings.HasPrefix(txt, tag) {
			rest := strings.TrimSpace(strings.TrimPrefix(txt, tag))
			rest = strings.Fields(rest + " ")[0]
			return rest
		}
	}
	return ""
}

// validatePending checks that bead exists in beads.jsonl and is open.
func validatePending(beadsFile, beadID string) (bool, string) {
	f, err := os.Open(beadsFile)
	if err != nil {
		return false, fmt.Sprintf("cannot read beads file %s: %v", beadsFile, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.ID == beadID {
			if rec.Status == "open" || rec.Status == "in_progress" || rec.Status == "ready" {
				return true, ""
			}
			return false, fmt.Sprintf("bead %s status=%q (must be open)", beadID, rec.Status)
		}
	}
	return false, fmt.Sprintf("bead %s not found in %s", beadID, beadsFile)
}

// deadcodePackage and deadcodeFunction mirror golang.org/x/tools/cmd/deadcode -json schema.
type deadcodePackage struct {
	Path  string             `json:"Path"`
	Funcs []deadcodeFunction `json:"Funcs"`
}

type deadcodeFunction struct {
	Name      string           `json:"Name"`
	Position  deadcodePosition `json:"Position"`
	Generated bool             `json:"Generated"`
}

type deadcodePosition struct {
	File string `json:"File"`
	Line int    `json:"Line"`
	Col  int    `json:"Col"`
}

// runDeadcode invokes deadcode -json from module-dir and returns the set of
// dead function declarations keyed by (absFile, line).
func runDeadcode(a runArgs) (map[deadKey]deadcodeFunction, error) {
	dir := filepath.Join(a.ProjectRoot, a.ModuleDir)
	args := []string{"-json"}
	args = append(args, a.Packages...)

	var cmd *exec.Cmd
	if a.DeadcodeBin != "" {
		cmd = exec.Command(a.DeadcodeBin, args...)
	} else {
		// Look up deadcode on PATH; fall back to `go run`.
		if path, err := exec.LookPath("deadcode"); err == nil {
			cmd = exec.Command(path, args...)
		} else {
			full := append([]string{"run", "golang.org/x/tools/cmd/deadcode@v0.42.0"}, args...)
			cmd = exec.Command("go", full...)
		}
	}
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%v (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	var pkgs []deadcodePackage
	if err := json.Unmarshal(out, &pkgs); err != nil {
		return nil, fmt.Errorf("parse deadcode json: %w", err)
	}
	set := make(map[deadKey]deadcodeFunction)
	for _, p := range pkgs {
		for _, fn := range p.Funcs {
			if strings.HasSuffix(fn.Position.File, "_test.go") {
				continue
			}
			absFile, _ := filepath.Abs(fn.Position.File)
			set[deadKey{File: absFile, Line: fn.Position.Line}] = fn
		}
	}
	return set, nil
}

type deadKey struct {
	File string
	Line int
}

func isDead(s symbol, dead map[deadKey]deadcodeFunction) bool {
	abs, _ := filepath.Abs(s.AbsFile)
	_, ok := dead[deadKey{File: abs, Line: s.Line}]
	return ok
}

// gitChangedGoFiles lists *.go (excluding _test.go) modified or added in
// DIFF_BASE..DIFF_HEAD, paths relative to projectRoot.
func gitChangedGoFiles(projectRoot, base, head string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=AM", base+".."+head, "--", "*.go")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, "_test.go") {
			continue
		}
		files = append(files, line)
	}
	return files, nil
}

// gitShow returns the contents of path at rev, or empty bytes if path did
// not exist at rev (i.e. the file was added in this diff).
func gitShow(projectRoot, rev, path string) ([]byte, error) {
	cmd := exec.Command("git", "show", rev+":"+path)
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, nil
	}
	return out, nil
}

func writeResult(path string, r result) {
	data, _ := json.MarshalIndent(r, "", "  ")
	_ = os.WriteFile(path, data, 0o644)
	fmt.Println(string(data))
}

func emitFatal(evidenceDir, name, msg string) {
	r := result{Status: "error", Message: msg}
	if evidenceDir != "" && name != "" {
		writeResult(filepath.Join(evidenceDir, name+".json"), r)
		return
	}
	data, _ := json.MarshalIndent(r, "", "  ")
	fmt.Fprintln(os.Stderr, string(data))
	os.Exit(0) // protocol: exit 0; status field is authoritative.
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func relPath(root, abs string) string {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return abs
	}
	return rel
}
