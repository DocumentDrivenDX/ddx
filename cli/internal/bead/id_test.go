package bead

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateID_AcceptsValidIDs(t *testing.T) {
	t.Parallel()

	for _, id := range []string{
		"ddx-1234abcd",
		"bx-00000001",
		"Project-42",
		"A1b2C3d4",
	} {
		require.NoError(t, ValidateID(id), "expected %q to be valid", id)
	}
}

func TestValidateID_RejectsInvalidCharset(t *testing.T) {
	t.Parallel()

	for _, id := range []string{
		"ddx_1234",
		"bad id",
		"ddx/1234",
		"ddx.1234",
	} {
		require.ErrorIs(t, ValidateID(id), ErrInvalidID, "expected %q to be rejected", id)
	}
}

func TestValidateID_RejectsLengthBounds(t *testing.T) {
	t.Parallel()

	shortID := "ddx-123"
	longID := strings.Repeat("a", MaxIDLength+1)

	require.ErrorIs(t, ValidateID(shortID), ErrInvalidID)
	require.ErrorIs(t, ValidateID(longID), ErrInvalidID)
}

func TestRandomHexIDGenerator_ProducesValidIDs(t *testing.T) {
	t.Parallel()

	gen := RandomHexIDGenerator{Prefix: DefaultIDPrefix, Bytes: 4}
	seen := make(map[string]struct{}, 100)

	for i := 0; i < 100; i++ {
		id, err := gen.GenID(context.Background())
		require.NoError(t, err)
		require.NoError(t, ValidateID(id))
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate id generated: %s", id)
		}
		seen[id] = struct{}{}
	}
}

func TestSequentialIDGenerator_ProducesUniqueOrdered(t *testing.T) {
	t.Parallel()

	gen := &SequentialIDGenerator{Prefix: DefaultIDPrefix}
	ids := make([]string, 0, 1000)
	seen := make(map[string]struct{}, 1000)

	for i := 0; i < 1000; i++ {
		id, err := gen.GenID(context.Background())
		require.NoError(t, err)
		require.NoError(t, ValidateID(id))
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate id generated: %s", id)
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	require.Equal(t, ids, sorted, "sequential ids must be lexicographically increasing")

	for i, id := range ids {
		require.Equal(t, fmt.Sprintf("%s%08x", DefaultIDPrefix, i+1), id)
	}
}
