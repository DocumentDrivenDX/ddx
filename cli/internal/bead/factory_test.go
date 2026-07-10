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
