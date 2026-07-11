package bead

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigrator_NewMigrator_UsesFactory(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "migrator.go", nil, 0)
	require.NoError(t, err)

	foundStorePtr := false
	foundNewStoreCall := false
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.StarExpr:
			if ident, ok := node.X.(*ast.Ident); ok && ident.Name == "Store" {
				foundStorePtr = true
			}
		case *ast.CallExpr:
			if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "NewStore" {
				foundNewStoreCall = true
			}
		}
		return true
	})

	require.False(t, foundStorePtr, "migrator.go should not name *Store directly")
	require.True(t, foundNewStoreCall, "migrator.go should construct its store through NewStore")
}
