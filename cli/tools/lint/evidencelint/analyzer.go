// Package evidencelint implements the no-unbounded-prompts structural lint
// rule for FEAT-022. It flags four sink patterns that route runtime data
// into agent prompts or prompt-adjacent server text egress without going
// through the internal/evidence primitives:
//
//  1. Assignment to a *.Prompt selector whose receiver type is the
//     agent.RunOptions struct (or a struct embedding it).
//  2. Composite-literal construction of mcpContent{Type: "text", ...}.
//  3. os.ReadFile / ioutil.ReadFile / io/ioutil.ReadFile result flowing
//     directly into a variable whose name matches *prompt* / *Prompt*.
//  4. Calls to response-writer methods (Write, WriteString, Fprintf,
//     Encoder.Encode) inside server packages whose data argument
//     references library / exec / diff / session / persona source
//     packages.
//
// Static/literal prompt fragments compiled into the binary are exempt.
//
// A flagged path may be intentionally exempted by adding an
//
//	// evidence:allow-unbounded reason="..."
//
// comment on the same line as the offending node, or on a comment line
// directly above it. The reason= clause must be present and non-empty.
package evidencelint

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const annotation = "evidence:allow-unbounded"

// Analyzer is the FEAT-022 structural sink analyzer.
var Analyzer = &analysis.Analyzer{
	Name:     "evidencelint",
	Doc:      "flags unbounded prompt sinks per FEAT-022 §3 (no-unbounded-prompts invariant)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// runOptionsTypeNames lists the named types whose .Prompt field
// assignment is a sink. RunOptions is the canonical case; embedding
// types (QuorumOptions, EmbeddedAgentOptions, etc.) are matched
// transitively via field lookup on the embedded type.
var runOptionsTypeNames = map[string]bool{
	"RunOptions": true,
}

// runOptionsPkgSuffixes lists package import-path suffixes whose
// RunOptions named type is the prompt sink we care about. Prevents
// false positives on identically named types in unrelated packages.
var runOptionsPkgSuffixes = []string{"/agent", "agent"}

// mcpContentTypeName is the named type whose composite-literal
// construction with Type:"text" is a sink.
const mcpContentTypeName = "mcpContent"

// mcpContentPkgSuffixes lists package import-path suffixes whose
// mcpContent named type is the prompt-adjacent egress sink.
var mcpContentPkgSuffixes = []string{"/server", "server", "mcppkg"}

// sourcePkgSuffixes lists the source-package import-path suffixes
// considered library/exec/diff/session/persona content for the §10
// bounded-egress rule. Conservative structural match.
var sourcePkgSuffixes = []string{
	"/library", "library",
	"/exec", "exec",
	"/diff", "diff",
	"/session", "session",
	"/persona", "persona",
	"/libdoc", "libdoc",
}

// serverPkgSuffixes lists the package import-path suffixes considered
// HTTP-handler-bearing server packages.
var serverPkgSuffixes = []string{
	"/server", "server",
	"/graphql", "graphql",
}

// responseWriterMethods are method names whose receiver is treated as
// a response-writer sink for pattern 4.
var responseWriterMethods = map[string]bool{
	"Write":       true,
	"WriteString": true,
	"Fprintf":     true,
	"Fprint":      true,
	"Fprintln":    true,
	"Encode":      true,
}

func pkgPathHasSuffix(p string, suffixes []string) bool {
	for _, s := range suffixes {
		if p == s || strings.HasSuffix(p, s) {
			return true
		}
	}
	return false
}

func run(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Collect all annotation comments (file → line → reason).
	allow := collectAllowAnnotations(pass)

	report := func(pos token.Pos, msg string) {
		if isAllowed(pass.Fset, pos, allow) {
			return
		}
		pass.Report(analysis.Diagnostic{
			Pos:     pos,
			Message: msg,
		})
	}

	// Skip the analyzer's own package and tests directory.
	if pass.Pkg != nil {
		path := pass.Pkg.Path()
		if strings.HasSuffix(path, "/tools/lint/evidencelint") ||
			strings.Contains(path, "/tools/lint/evidencelint/") {
			return nil, nil
		}
	}

	// Pattern 1 & 3: AssignStmt walks
	insp.Preorder([]ast.Node{(*ast.AssignStmt)(nil)}, func(n ast.Node) {
		as := n.(*ast.AssignStmt)
		checkPromptAssignment(pass, as, report)
		checkReadFileToPromptVar(pass, as, report)
	})

	// Pattern 2: mcpContent composite literals
	insp.Preorder([]ast.Node{(*ast.CompositeLit)(nil)}, func(n ast.Node) {
		cl := n.(*ast.CompositeLit)
		checkMCPContent(pass, cl, report)
	})

	// Pattern 4: response-writer calls inside server packages
	if pass.Pkg != nil && pkgPathHasSuffix(pass.Pkg.Path(), serverPkgSuffixes) {
		insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
			ce := n.(*ast.CallExpr)
			checkServerWrite(pass, ce, report)
		})
	}

	return nil, nil
}

// --- Pattern 1: assignment to RunOptions.Prompt ----------------------------

func checkPromptAssignment(pass *analysis.Pass, as *ast.AssignStmt, report func(token.Pos, string)) {
	for _, lhs := range as.Lhs {
		sel, ok := lhs.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil || sel.Sel.Name != "Prompt" {
			continue
		}
		// Resolve the type of sel.X.
		t := pass.TypesInfo.TypeOf(sel.X)
		if t == nil {
			continue
		}
		if !typeIsRunOptions(t) {
			continue
		}
		// Allow assignment where the RHS is a constant string expression.
		if len(as.Rhs) == 1 && isConstantString(pass, as.Rhs[0]) {
			continue
		}
		report(sel.Pos(),
			"evidencelint: assignment to RunOptions.Prompt without going through internal/evidence primitives (FEAT-022 §3); add `// evidence:allow-unbounded reason=\"...\"` to suppress")
	}
}

func typeIsRunOptions(t types.Type) bool {
	// Strip pointers.
	for {
		p, ok := t.(*types.Pointer)
		if !ok {
			break
		}
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj == nil {
		return false
	}
	if !runOptionsTypeNames[obj.Name()] {
		// Not RunOptions itself; check if the underlying struct embeds RunOptions.
		st, ok := named.Underlying().(*types.Struct)
		if !ok {
			return false
		}
		for i := 0; i < st.NumFields(); i++ {
			f := st.Field(i)
			if !f.Anonymous() {
				continue
			}
			if typeIsRunOptions(f.Type()) {
				return true
			}
		}
		return false
	}
	if obj.Pkg() == nil {
		return false
	}
	return pkgPathHasSuffix(obj.Pkg().Path(), runOptionsPkgSuffixes)
}

func isConstantString(pass *analysis.Pass, e ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[e]
	if !ok {
		return false
	}
	return tv.Value != nil && tv.Value.Kind().String() == "String"
}

// --- Pattern 2: mcpContent{Type:"text", ...} composite literal -------------

func checkMCPContent(pass *analysis.Pass, cl *ast.CompositeLit, report func(token.Pos, string)) {
	// Use the type-checker's view of the literal itself; this handles
	// elided types in nested array/slice composite literals where
	// cl.Type is nil but the element type is known.
	t := pass.TypesInfo.TypeOf(cl)
	if t == nil {
		return
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return
	}
	obj := named.Obj()
	if obj == nil || obj.Name() != mcpContentTypeName {
		return
	}
	if obj.Pkg() == nil || !pkgPathHasSuffix(obj.Pkg().Path(), mcpContentPkgSuffixes) {
		return
	}
	// Look for Type: "text" in the literal.
	hasTextType := false
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		k, ok := kv.Key.(*ast.Ident)
		if !ok || k.Name != "Type" {
			continue
		}
		if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING && strings.Trim(lit.Value, "\"`") == "text" {
			hasTextType = true
			break
		}
	}
	if !hasTextType {
		return
	}
	report(cl.Pos(),
		"evidencelint: mcpContent{Type:\"text\"} literal must route Text through evidence.ClampOutput (FEAT-022 §10); add `// evidence:allow-unbounded reason=\"...\"` to suppress")
}

// --- Pattern 3: os.ReadFile result flowing into a *prompt* variable --------

func checkReadFileToPromptVar(pass *analysis.Pass, as *ast.AssignStmt, report func(token.Pos, string)) {
	if len(as.Rhs) != 1 {
		return
	}
	call, ok := as.Rhs[0].(*ast.CallExpr)
	if !ok {
		return
	}
	if !isReadFileCall(call) {
		return
	}
	for _, lhs := range as.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok || ident.Name == "_" {
			continue
		}
		if !nameLooksLikePrompt(ident.Name) {
			continue
		}
		report(call.Pos(),
			"evidencelint: os.ReadFile/ioutil.ReadFile result flows into prompt-named variable without evidence.ReadFileClamped (FEAT-022 §8); add `// evidence:allow-unbounded reason=\"...\"` to suppress")
		return
	}
}

func isReadFileCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if sel.Sel == nil || sel.Sel.Name != "ReadFile" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return pkg.Name == "os" || pkg.Name == "ioutil"
}

func nameLooksLikePrompt(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "prompt")
}

// --- Pattern 4: server-package response writes from source packages --------

func checkServerWrite(pass *analysis.Pass, ce *ast.CallExpr, report func(token.Pos, string)) {
	sel, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return
	}
	if !responseWriterMethods[sel.Sel.Name] {
		return
	}
	// The data argument(s) we examine. For Encode/WriteString/Write,
	// it's typically the first arg; for Fprintf it's args[1:]. Be
	// permissive: scan all args.
	for _, arg := range ce.Args {
		if argReferencesSourcePkg(pass, arg) {
			report(ce.Pos(),
				"evidencelint: server response writer called with data sourced from library/exec/diff/session/persona without evidence.ClampOutput (FEAT-022 §10); add `// evidence:allow-unbounded reason=\"...\"` to suppress")
			return
		}
	}
}

// argReferencesSourcePkg returns true if the expression structurally
// references a function call or selector whose qualified identifier's
// package path matches one of the source-package suffixes.
func argReferencesSourcePkg(pass *analysis.Pass, e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *ast.SelectorExpr:
			id, ok := x.X.(*ast.Ident)
			if !ok {
				return true
			}
			obj := pass.TypesInfo.ObjectOf(id)
			if obj == nil {
				return true
			}
			pkgName, ok := obj.(*types.PkgName)
			if !ok {
				return true
			}
			imp := pkgName.Imported()
			if imp == nil {
				return true
			}
			if pkgPathHasSuffix(imp.Path(), sourcePkgSuffixes) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// --- Allow-annotation handling ---------------------------------------------

// allowSet maps file name → set of line numbers whose adjacent annotation
// comment is "valid" (i.e. has a non-empty reason="...").
type allowSet map[string]map[int]bool

func collectAllowAnnotations(pass *analysis.Pass) allowSet {
	out := allowSet{}
	for _, f := range pass.Files {
		fname := pass.Fset.File(f.Pos()).Name()
		lines := map[int]bool{}
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				text := c.Text
				if !strings.Contains(text, annotation) {
					continue
				}
				if !hasNonEmptyReason(text) {
					continue
				}
				line := pass.Fset.Position(c.Pos()).Line
				lines[line] = true
				// Also mark the line(s) immediately below (covers
				// "comment above the offending statement").
				lines[line+1] = true
			}
		}
		out[fname] = lines
	}
	return out
}

// hasNonEmptyReason returns true if the comment text contains
// reason="..." with at least one non-quote character inside.
func hasNonEmptyReason(text string) bool {
	idx := strings.Index(text, "reason=")
	if idx < 0 {
		return false
	}
	rest := text[idx+len("reason="):]
	rest = strings.TrimLeft(rest, " \t")
	if !strings.HasPrefix(rest, "\"") {
		return false
	}
	rest = rest[1:]
	end := strings.Index(rest, "\"")
	if end <= 0 {
		return false
	}
	return strings.TrimSpace(rest[:end]) != ""
}

func isAllowed(fset *token.FileSet, pos token.Pos, allow allowSet) bool {
	p := fset.Position(pos)
	lines, ok := allow[p.Filename]
	if !ok {
		return false
	}
	return lines[p.Line]
}
