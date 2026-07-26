package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/require"
)

// TestServerBeadStoreAccessorsUseBackendInterfaces guards the TD-027
// migration on the server package: the shared store helpers must expose an
// interface, not the concrete *bead.Store.
func TestServerBeadStoreAccessorsUseBackendInterfaces(t *testing.T) {
	backendType := reflect.TypeOf((*bead.Backend)(nil)).Elem()
	concreteType := reflect.TypeOf((*bead.Store)(nil))

	requestMethod := reflect.TypeOf((&Server{}).beadStoreForRequest)
	require.Equal(t, 1, requestMethod.NumIn())
	require.Equal(t, 1, requestMethod.NumOut())
	require.Equal(t, reflect.Interface, requestMethod.Out(0).Kind())
	require.True(t, requestMethod.Out(0).Implements(backendType))
	require.NotEqual(t, concreteType, requestMethod.Out(0))

	storeMethod := reflect.TypeOf((&Server{}).beadStore)
	require.Equal(t, 0, storeMethod.NumIn())
	require.Equal(t, 1, storeMethod.NumOut())
	require.Equal(t, reflect.Interface, storeMethod.Out(0).Kind())
	require.True(t, storeMethod.Out(0).Implements(backendType))
	require.NotEqual(t, concreteType, storeMethod.Out(0))

	var backend bead.Backend = bead.NewStore(t.TempDir())
	require.NotNil(t, backend)
}

// TestServerStoresUseBackendInterfaces verifies server and GraphQL resolver
// seams expose bead.Backend, bead.ReadOnlyBackend, or narrower interfaces
// rather than *bead.Store (TD-027 §21 / ddx-a9652d0d).
func TestServerStoresUseBackendInterfaces(t *testing.T) {
	t.Parallel()

	backendType := reflect.TypeOf((*bead.Backend)(nil)).Elem()
	concreteType := reflect.TypeOf((*bead.Store)(nil))

	// Server accessors already covered by TestServerBeadStoreAccessorsUseBackendInterfaces;
	// re-check here so this single named AC gate stands alone.
	for _, method := range []reflect.Type{
		reflect.TypeOf((&Server{}).beadStoreForRequest),
		reflect.TypeOf((&Server{}).beadStore),
	} {
		require.Equal(t, reflect.Interface, method.Out(0).Kind())
		require.True(t, method.Out(0).Implements(backendType))
		require.NotEqual(t, concreteType, method.Out(0))
	}

	// workerClaimStore and staleDiskEntryCanReleaseClaim must not take *bead.Store.
	claimMethod := reflect.TypeOf(staleDiskEntryCanReleaseClaim)
	require.Equal(t, 2, claimMethod.NumIn())
	require.Equal(t, reflect.Interface, claimMethod.In(0).Kind(),
		"staleDiskEntryCanReleaseClaim must accept an interface, not *bead.Store")
	require.NotEqual(t, concreteType, claimMethod.In(0))

	// GraphQL projectBeadStore return type: parse the production source so
	// this package does not import graphql internals that may pull generated
	// cycles; require the function result is an interface identifier, not
	// *bead.Store.
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	gqlPaths := filepath.Join(filepath.Dir(thisFile), "graphql", "ddxroot_paths.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, gqlPaths, nil, 0)
	require.NoError(t, err)

	var foundProjectBeadStore bool
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "projectBeadStore" {
			continue
		}
		foundProjectBeadStore = true
		require.NotNil(t, fn.Type.Results)
		require.Equal(t, 1, fn.Type.Results.NumFields())
		result := fn.Type.Results.List[0].Type
		// Must be a named interface type (Ident), not *bead.Store (StarExpr).
		_, isStar := result.(*ast.StarExpr)
		require.False(t, isStar, "projectBeadStore must not return *bead.Store")
		ident, isIdent := result.(*ast.Ident)
		require.True(t, isIdent, "projectBeadStore must return a named interface type")
		require.Equal(t, "projectStore", ident.Name)
	}
	require.True(t, foundProjectBeadStore, "projectBeadStore must exist in graphql/ddxroot_paths.go")

	// GraphQL mutationResolver.beadStore likewise.
	mutPath := filepath.Join(filepath.Dir(thisFile), "graphql", "resolver_mutation_beads.go")
	mutFile, err := parser.ParseFile(fset, mutPath, nil, 0)
	require.NoError(t, err)
	var foundMutBeadStore bool
	for _, decl := range mutFile.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "beadStore" {
			continue
		}
		// Only the mutationResolver method (has a receiver).
		if fn.Recv == nil || fn.Recv.NumFields() == 0 {
			continue
		}
		foundMutBeadStore = true
		require.NotNil(t, fn.Type.Results)
		require.Equal(t, 1, fn.Type.Results.NumFields())
		result := fn.Type.Results.List[0].Type
		_, isStar := result.(*ast.StarExpr)
		require.False(t, isStar, "mutationResolver.beadStore must not return *bead.Store")
		ident, isIdent := result.(*ast.Ident)
		require.True(t, isIdent, "mutationResolver.beadStore must return a named interface type")
		require.Equal(t, "projectStore", ident.Name)
	}
	require.True(t, foundMutBeadStore, "mutationResolver.beadStore must exist")

	// projectCoordination.store field must be bead.Backend (interface), not *bead.Store.
	coordPath := filepath.Join(filepath.Dir(thisFile), "coordination.go")
	coordFile, err := parser.ParseFile(fset, coordPath, nil, 0)
	require.NoError(t, err)
	var foundCoordStore bool
	ast.Inspect(coordFile, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name == nil || ts.Name.Name != "projectCoordination" {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, field := range st.Fields.List {
			for _, name := range field.Names {
				if name.Name != "store" {
					continue
				}
				foundCoordStore = true
				sel, ok := field.Type.(*ast.SelectorExpr)
				require.True(t, ok, "projectCoordination.store must be bead.Backend")
				pkg, ok := sel.X.(*ast.Ident)
				require.True(t, ok)
				require.Equal(t, "bead", pkg.Name)
				require.Equal(t, "Backend", sel.Sel.Name)
			}
		}
		return true
	})
	require.True(t, foundCoordStore, "projectCoordination.store field must exist")
}
