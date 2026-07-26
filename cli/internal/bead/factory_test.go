package bead

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scriptedBeadReader struct {
	batches [][]Bead
	calls   int
}

func (r *scriptedBeadReader) ReadAll(ctx context.Context) ([]Bead, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(r.batches) == 0 {
		return nil, nil
	}
	idx := r.calls
	r.calls++
	if idx >= len(r.batches) {
		idx = len(r.batches) - 1
	}
	return append([]Bead(nil), r.batches[idx]...), nil
}

func (r *scriptedBeadReader) ReadAllFiltered(ctx context.Context, pred func(Bead) bool) ([]Bead, error) {
	beads, err := r.ReadAll(ctx)
	if err != nil || pred == nil {
		return beads, err
	}
	out := make([]Bead, 0, len(beads))
	for _, b := range beads {
		if pred(b) {
			out = append(out, b)
		}
	}
	return out, nil
}

func (r *scriptedBeadReader) Get(ctx context.Context, id string) (*Bead, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(r.batches) == 0 {
		return nil, nil
	}
	last := r.batches[len(r.batches)-1]
	for i := len(last) - 1; i >= 0; i-- {
		if last[i].ID == id {
			b := last[i]
			return &b, nil
		}
	}
	return nil, nil
}

func waitForLifecycleEvent(t *testing.T, ch <-chan LifecycleEvent) LifecycleEvent {
	t.Helper()
	select {
	case evt, ok := <-ch:
		if !ok {
			t.Fatal("lifecycle event channel closed unexpectedly")
		}
		return evt
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for lifecycle event")
		return LifecycleEvent{}
	}
}

// TestNewLifecycleSubscriberReturnsInterface proves the public lifecycle
// entrypoint (TD-027 §21) returns bead.LifecycleSubscriber and that no
// public non-test API in package bead exposes *lifecycle.WatcherHub.
func TestNewLifecycleSubscriberReturnsInterface(t *testing.T) {
	// Compile-time + runtime: factory returns the public interface only.
	var sub LifecycleSubscriber = NewLifecycleSubscriber(func(string) (BeadReader, error) {
		return &scriptedBeadReader{}, nil
	}, time.Hour)
	require.NotNil(t, sub)
	closer, ok := sub.(interface{ Close() })
	require.True(t, ok, "NewLifecycleSubscriber must return a closable LifecycleSubscriber")
	t.Cleanup(closer.Close)

	// Public package surface must not export WatcherHub / NewWatcherHub, and
	// no exported function may return *lifecycle.WatcherHub.
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, err)
		ast.Inspect(file, func(n ast.Node) bool {
			switch decl := n.(type) {
			case *ast.FuncDecl:
				if !decl.Name.IsExported() {
					return true
				}
				assert.NotEqual(t, "NewWatcherHub", decl.Name.Name, "%s: NewWatcherHub must not be exported from package bead", name)
				if decl.Type.Results != nil {
					for _, field := range decl.Type.Results.List {
						if exprNamesWatcherHub(field.Type) {
							t.Errorf("%s: exported func %s must not return *lifecycle.WatcherHub", name, decl.Name.Name)
						}
					}
				}
			case *ast.TypeSpec:
				if decl.Name.IsExported() {
					assert.NotEqual(t, "WatcherHub", decl.Name.Name, "%s: WatcherHub must not be exported from package bead", name)
				}
			case *ast.Field:
				// Exported struct fields must not expose the concrete hub.
				if exprNamesWatcherHub(decl.Type) {
					for _, id := range decl.Names {
						if id.IsExported() {
							t.Errorf("%s: exported field %s must not have type *lifecycle.WatcherHub", name, id.Name)
						}
					}
				}
			}
			return true
		})
	}
}

func exprNamesWatcherHub(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return exprNamesWatcherHub(e.X)
	case *ast.SelectorExpr:
		if e.Sel != nil && e.Sel.Name == "WatcherHub" {
			if id, ok := e.X.(*ast.Ident); ok && id.Name == "lifecycle" {
				return true
			}
		}
	case *ast.Ident:
		return e.Name == "WatcherHub"
	}
	return false
}

// TestNewLifecycleSubscriber_ReturnsLifecycleSubscriber proves the factory
// returns the public LifecycleSubscriber interface (TD-027 §21) and that the
// returned value works end-to-end: subscribing yields lifecycle events
// derived from bead changes reported by the caller-supplied StoreFactory.
func TestNewLifecycleSubscriber_ReturnsLifecycleSubscriber(t *testing.T) {
	reader := &scriptedBeadReader{
		batches: [][]Bead{
			{{ID: "bx-1", Title: "First bead", Status: StatusOpen}},
		},
	}

	sub := NewLifecycleSubscriber(func(string) (BeadReader, error) {
		return reader, nil
	}, 5*time.Millisecond)
	require.NotNil(t, sub)

	closer, ok := sub.(interface{ Close() })
	require.True(t, ok, "lifecycle subscriber returned by NewLifecycleSubscriber must be closable by its owner")
	t.Cleanup(closer.Close)

	events, unsub, err := sub.SubscribeLifecycle(context.Background(), "project-123")
	require.NoError(t, err)
	t.Cleanup(unsub)

	evt := waitForLifecycleEvent(t, events)
	assert.Equal(t, "bx-1", evt.BeadID)
	assert.Equal(t, "created", evt.Kind)
	assert.Equal(t, "bead bx-1 created: First bead", evt.Summary)
}

// TestPublicBeadPackageDoesNotExportWatcherHub proves TD-027 §21: the
// concrete lifecycle hub (formerly WatcherHub / NewWatcherHub) is no longer
// part of the public bead package surface. Callers must construct lifecycle
// watching through NewLifecycleSubscriber and cannot name the concrete
// implementation from package bead.
func TestPublicBeadPackageDoesNotExportWatcherHub(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, err)
		ast.Inspect(file, func(n ast.Node) bool {
			switch decl := n.(type) {
			case *ast.FuncDecl:
				assert.NotEqual(t, "NewWatcherHub", decl.Name.Name, "%s: NewWatcherHub must not be exported from package bead", name)
			case *ast.TypeSpec:
				assert.NotEqual(t, "WatcherHub", decl.Name.Name, "%s: WatcherHub must not be exported from package bead", name)
			}
			return true
		})
	}
}
