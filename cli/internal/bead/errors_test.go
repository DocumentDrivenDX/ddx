package bead

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSentinelErrors_AreErrorValues(t *testing.T) {
	t.Parallel()

	errs := []error{
		ErrNotFound,
		ErrConflict,
		ErrInvalidID,
		ErrAlreadyClaimed,
		ErrNotClaimedByOwner,
		ErrUnsupported,
	}

	seen := make(map[error]struct{}, len(errs))
	for _, err := range errs {
		require.Error(t, err)
		require.NotEmpty(t, err.Error())
		seen[err] = struct{}{}
	}

	require.Len(t, seen, len(errs), "sentinel errors must be distinct values")
}
