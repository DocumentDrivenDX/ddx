package graphql_test

import (
	"errors"
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2"
	gqlast "github.com/vektah/gqlparser/v2/ast"
)

// promptMutationRules lists the schema mutation fields that must keep the
// operator-prompt trust checks in place. The map is explicit so new prompt-like
// mutations fail closed until they are reviewed and added here.
var promptMutationRules = map[string]promptMutationRule{
	"operatorPromptSubmit": {
		resolverName:      "OperatorPromptSubmit",
		requiredCallNames: []string{"validateSameOrigin"},
		requiredIdents:    []string{"CSRFHeaderName", "httpRequestFromContext"},
	},
	"operatorPromptApprove": {
		resolverName:      "OperatorPromptApprove",
		requiredCallNames: []string{"requireOperatorPromptCSRF"},
	},
	"operatorPromptCancel": {
		resolverName:      "OperatorPromptCancel",
		requiredCallNames: []string{"requireOperatorPromptCSRF"},
	},
}

type promptMutationRule struct {
	resolverName      string
	requiredCallNames []string
	requiredIdents    []string
}

type resolverDecl struct {
	file string
	fn   *goast.FuncDecl
}

func TestADRMutationConformance(t *testing.T) {
	dir := packageDir(t)

	schemaSrc, err := os.ReadFile(filepath.Join(dir, "schema.graphql"))
	if err != nil {
		t.Fatal(err)
	}
	serverSrc, err := os.ReadFile(filepath.Join(dir, "..", "server.go"))
	if err != nil {
		t.Fatal(err)
	}
	resolvers, err := loadMutationResolvers(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := lintMutationConformance(string(schemaSrc), string(serverSrc), resolvers); err != nil {
		t.Fatalf("ADR mutation conformance failed: %v", err)
	}

	t.Run("synthetic-nonconformant-resolver", func(t *testing.T) {
		badSchema := `schema { query: Query mutation: Mutation }

type Query {
  _empty: Boolean
}

type Mutation {
  operatorPromptSubmit(input: OperatorPromptSubmitInput!): OperatorPromptSubmitResult!
}

input OperatorPromptSubmitInput {
  prompt: String!
  idempotencyKey: String!
}

type OperatorPromptSubmitResult {
  ok: Boolean!
}
`
		badResolver := map[string]string{
			"resolver_operator_prompt.go": `package graphql

import "context"

func (r *mutationResolver) OperatorPromptSubmit(ctx context.Context, input OperatorPromptSubmitInput) (*OperatorPromptSubmitResult, error) {
	if r.workingDir(ctx) == "" {
		return nil, nil
	}
	return nil, nil
}
`,
		}
		if err := lintMutationConformance(badSchema, string(serverSrc), badResolver); err == nil {
			t.Fatal("expected synthetic nonconformant resolver to be rejected")
		}
	})
}

func lintMutationConformance(schemaSrc, serverSrc string, resolverSources map[string]string) error {
	schema := gqlparser.MustLoadSchema(&gqlast.Source{Name: "schema.graphql", Input: schemaSrc})
	var problems []string

	if err := checkGraphQLRouteGate(serverSrc); err != nil {
		problems = append(problems, err.Error())
	}

	resolvers, err := parseMutationResolvers(resolverSources)
	if err != nil {
		problems = append(problems, err.Error())
	}

	if schema.Mutation == nil {
		problems = append(problems, "schema.graphql does not define a Mutation type")
	} else {
		for _, field := range schema.Mutation.Fields {
			name := field.Name
			if !strings.HasPrefix(name, "operatorPrompt") {
				continue
			}
			rule, ok := promptMutationRules[name]
			if !ok {
				problems = append(problems, fmt.Sprintf("mutation %s is prompt-like but not allowlisted; add an explicit rule before shipping", name))
				continue
			}
			fn, ok := resolvers[rule.resolverName]
			if !ok {
				problems = append(problems, fmt.Sprintf("mutation %s missing resolver %s in parsed source", name, rule.resolverName))
				continue
			}
			if err := checkResolverGate(name, fn, rule); err != nil {
				problems = append(problems, err.Error())
			}
		}
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	return nil
}

func checkGraphQLRouteGate(serverSrc string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "server.go", serverSrc, parser.SkipObjectResolution)
	if err != nil {
		return fmt.Errorf("parse server.go: %w", err)
	}

	var hasPost, hasGet, hasRequireTrusted bool
	goast.Inspect(file, func(n goast.Node) bool {
		call, ok := n.(*goast.CallExpr)
		if !ok {
			return true
		}
		if callName(call.Fun) == "requireTrusted" {
			hasRequireTrusted = true
		}
		if callName(call.Fun) != "trusted" || len(call.Args) == 0 {
			return true
		}
		lit, ok := call.Args[0].(*goast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		switch strings.Trim(lit.Value, "`\"") {
		case "POST /graphql":
			hasPost = true
		case "GET /graphql":
			hasGet = true
		}
		return true
	})
	if !hasRequireTrusted {
		return fmt.Errorf("server.go must keep trusted() delegating to requireTrusted(handler)")
	}
	if !hasPost || !hasGet {
		return fmt.Errorf("server.go must register /graphql through trusted() for both GET and POST (have GET=%t POST=%t)", hasGet, hasPost)
	}
	return nil
}

func parseMutationResolvers(sources map[string]string) (map[string]resolverDecl, error) {
	out := make(map[string]resolverDecl)
	for path, src := range sources {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*goast.FuncDecl)
			if !ok || fn.Body == nil || fn.Recv == nil || len(fn.Recv.List) == 0 {
				continue
			}
			if !isMutationResolver(fn.Recv.List[0].Type) {
				continue
			}
			out[fn.Name.Name] = resolverDecl{file: path, fn: fn}
		}
	}
	return out, nil
}

func isMutationResolver(expr goast.Expr) bool {
	switch v := expr.(type) {
	case *goast.StarExpr:
		if ident, ok := v.X.(*goast.Ident); ok {
			return ident.Name == "mutationResolver"
		}
	case *goast.Ident:
		return v.Name == "mutationResolver"
	}
	return false
}

func checkResolverGate(mutation string, decl resolverDecl, rule promptMutationRule) error {
	calls := map[string]bool{}
	idents := map[string]bool{}
	goast.Inspect(decl.fn.Body, func(n goast.Node) bool {
		switch v := n.(type) {
		case *goast.CallExpr:
			calls[callName(v.Fun)] = true
		case *goast.Ident:
			idents[v.Name] = true
		}
		return true
	})

	var missing []string
	for _, name := range rule.requiredCallNames {
		if !calls[name] {
			missing = append(missing, "call "+name)
		}
	}
	for _, name := range rule.requiredIdents {
		if !idents[name] {
			missing = append(missing, "identifier "+name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s in %s missing %s", mutation, decl.file, strings.Join(missing, ", "))
	}
	return nil
}

func loadMutationResolvers(dir string) (map[string]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "resolver_*.go"))
	if err != nil {
		return nil, err
	}
	sources := make(map[string]string, len(files))
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		if filepath.Base(path) == "generated.go" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		sources[path] = string(data)
	}
	return sources, nil
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func callName(expr goast.Expr) string {
	switch v := expr.(type) {
	case *goast.Ident:
		return v.Name
	case *goast.SelectorExpr:
		if pkg, ok := v.X.(*goast.Ident); ok && looksLikePackageName(pkg.Name) {
			return pkg.Name + "." + v.Sel.Name
		}
		return v.Sel.Name
	}
	return ""
}

func looksLikePackageName(s string) bool {
	if len(s) < 3 {
		return false
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	switch s {
	case "srv", "tb", "ctx", "err", "req", "res":
		return false
	}
	return true
}
