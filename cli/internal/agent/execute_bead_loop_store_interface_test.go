package agent

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/require"
)

// TestExecuteBeadLoopStoreDoesNotExposeConcreteStore proves the execute-loop
// store dependency is an interface that can be satisfied without naming
// *bead.Store outside test construction helpers (TD-027 §21 / ddx-a9652d0d).
func TestExecuteBeadLoopStoreDoesNotExposeConcreteStore(t *testing.T) {
	t.Parallel()

	iface := reflect.TypeOf((*ExecuteBeadLoopStore)(nil)).Elem()
	require.Equal(t, reflect.Interface, iface.Kind(),
		"ExecuteBeadLoopStore must be an interface")

	concrete := reflect.TypeOf((*bead.Store)(nil))
	for i := 0; i < iface.NumMethod(); i++ {
		m := iface.Method(i)
		for j := 0; j < m.Type.NumIn(); j++ {
			require.NotEqual(t, concrete, m.Type.In(j),
				"ExecuteBeadLoopStore.%s input must not be *bead.Store", m.Name)
		}
		for j := 0; j < m.Type.NumOut(); j++ {
			require.NotEqual(t, concrete, m.Type.Out(j),
				"ExecuteBeadLoopStore.%s output must not be *bead.Store", m.Name)
		}
	}

	// Production ExecuteBeadWorker.Store field must be the interface, not
	// the concrete store type.
	workerField, ok := reflect.TypeOf(ExecuteBeadWorker{}).FieldByName("Store")
	require.True(t, ok, "ExecuteBeadWorker.Store field must exist")
	require.Equal(t, reflect.Interface, workerField.Type.Kind(),
		"ExecuteBeadWorker.Store must be an interface")
	require.Equal(t, iface, workerField.Type,
		"ExecuteBeadWorker.Store must be ExecuteBeadLoopStore")

	// AST: production execute_bead_loop.go must not declare *bead.Store types
	// (construction helpers live in tests / NewStore call sites typed as the
	// interface).
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	loopFile := filepath.Join(filepath.Dir(thisFile), "execute_bead_loop.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, loopFile, nil, 0)
	require.NoError(t, err)

	beadAlias := ""
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) == "github.com/DocumentDrivenDX/ddx/internal/bead" {
			beadAlias = "bead"
			if imp.Name != nil {
				beadAlias = imp.Name.Name
			}
			break
		}
	}
	require.NotEmpty(t, beadAlias)

	var hits []string
	ast.Inspect(file, func(n ast.Node) bool {
		star, ok := n.(*ast.StarExpr)
		if !ok {
			return true
		}
		sel, ok := star.X.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != beadAlias || sel.Sel.Name != "Store" {
			return true
		}
		pos := fset.Position(star.Pos())
		hits = append(hits, pos.String())
		return true
	})
	require.Empty(t, hits,
		"execute_bead_loop.go must not name *bead.Store; hits: %s", strings.Join(hits, ", "))

	// Compile-time: a local fake with no *bead.Store embedding satisfies the
	// interface. Methods panic if invoked — this is a type-shape proof only.
	var _ ExecuteBeadLoopStore = (*loopStoreInterfaceStub)(nil)
	_ = context.Background()
	_ = time.Time{}
}

// loopStoreInterfaceStub proves ExecuteBeadLoopStore does not require
// *bead.Store. Methods panic if called.
type loopStoreInterfaceStub struct{}

func (*loopStoreInterfaceStub) ReadyExecution() ([]bead.Bead, error) { panic("stub") }
func (*loopStoreInterfaceStub) Get(context.Context, string) (*bead.Bead, error) {
	panic("stub")
}
func (*loopStoreInterfaceStub) Create(context.Context, *bead.Bead) error { panic("stub") }
func (*loopStoreInterfaceStub) Claim(string, string) error               { panic("stub") }
func (*loopStoreInterfaceStub) Unclaim(string) error                     { panic("stub") }
func (*loopStoreInterfaceStub) TouchClaimHeartbeat(string) error         { panic("stub") }
func (*loopStoreInterfaceStub) CloseWithEvidence(string, string, string) error {
	panic("stub")
}
func (*loopStoreInterfaceStub) AppendEvent(string, bead.BeadEvent) error { panic("stub") }
func (*loopStoreInterfaceStub) Events(string) ([]bead.BeadEvent, error)  { panic("stub") }
func (*loopStoreInterfaceStub) SetExecutionCooldown(string, time.Time, string, string, string) error {
	panic("stub")
}
func (*loopStoreInterfaceStub) AppendNotes(string, string) error { panic("stub") }
func (*loopStoreInterfaceStub) IncrNoChangesCount(string) (int, error) {
	panic("stub")
}
func (*loopStoreInterfaceStub) Reopen(string, string, string) error { panic("stub") }
func (*loopStoreInterfaceStub) Update(context.Context, string, func(*bead.Bead)) error {
	panic("stub")
}
func (*loopStoreInterfaceStub) UpdateWithLifecycleStatus(string, string, bead.LifecycleTransitionOptions, func(*bead.Bead) error) error {
	panic("stub")
}
func (*loopStoreInterfaceStub) ParkToProposed(string, bead.ParkReason, func(*bead.Bead)) error {
	panic("stub")
}
func (*loopStoreInterfaceStub) ParkToProposedWithIntakeEvent(string, string, string, string, string, map[string]any, time.Time, func(*bead.Bead)) error {
	panic("stub")
}
