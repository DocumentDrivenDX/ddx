package bead

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
