package bead

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// MutateFunc adapts an ad-hoc bead mutation into an Operation. Defined in
// the test build so the implementation does not show up in production
// reachability analysis (no production caller constructs MutateFunc).
type MutateFunc func(*Bead) error

// Apply executes the wrapped mutation.
func (m MutateFunc) Apply(b *Bead) error {
	return m(b)
}

func TestOperation_MutateFuncAdapter_RoundTrip(t *testing.T) {
	t.Parallel()

	var op Operation = MutateFunc(func(b *Bead) error {
		b.Title = "updated title"
		b.Notes = "mutated through Operation"
		return nil
	})

	b := &Bead{Title: "original title"}

	require.NoError(t, op.Apply(b))
	require.Equal(t, "updated title", b.Title)
	require.Equal(t, "mutated through Operation", b.Notes)
}
