package artifacttypes

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildIndex_InferredPrefixCollisionIsNonFatal(t *testing.T) {
	types := []Type{
		{
			Plugin: "helix", TypeID: "metric-definition", Name: "Metric Definition",
			Prefix: "MD", PrefixExplicit: false,
			SourceMetaPath: "workflows/.../metric-definition/meta.yml",
		},
		{
			Plugin: "helix", TypeID: "metrics-dashboard", Name: "Metrics Dashboard",
			Prefix: "MD", PrefixExplicit: false,
			SourceMetaPath: "workflows/.../metrics-dashboard/meta.yml",
		},
	}

	idx, err := buildIndex(types)
	require.NoError(t, err, "inferred-prefix collision must not fatal-error the index build")
	require.NotNil(t, idx)
	require.Len(t, idx.Types, 2, "both types should remain indexed in Types")

	_, ok := idx.Lookup("helix", "metric-definition")
	require.True(t, ok, "first type must still be lookable by key")
	_, ok = idx.Lookup("helix", "metrics-dashboard")
	require.True(t, ok, "shadowed type must still be lookable by key")

	got, ok := idx.LookupPrefix("MD")
	require.True(t, ok)
	require.Equal(t, "metric-definition", got.TypeID, "first-seen winner keeps the byPrefix slot")
}

func TestBuildIndex_ExplicitPrefixCollisionIsFatal(t *testing.T) {
	types := []Type{
		{
			Plugin: "helix", TypeID: "alpha", Name: "Alpha",
			Prefix: "X", PrefixExplicit: true,
			SourceMetaPath: "a/meta.yml",
		},
		{
			Plugin: "helix", TypeID: "beta", Name: "Beta",
			Prefix: "X", PrefixExplicit: true,
			SourceMetaPath: "b/meta.yml",
		},
	}

	_, err := buildIndex(types)
	require.Error(t, err, "explicit-on-both prefix collision must remain fatal")
	require.Contains(t, err.Error(), `duplicate artifact type prefix "X"`)
}

func TestBuildIndex_MixedExplicitInferredCollisionIsNonFatal(t *testing.T) {
	types := []Type{
		{
			Plugin: "helix", TypeID: "alpha", Name: "Alpha",
			Prefix: "X", PrefixExplicit: true,
			SourceMetaPath: "a/meta.yml",
		},
		{
			Plugin: "helix", TypeID: "beta", Name: "Beta",
			Prefix: "X", PrefixExplicit: false,
			SourceMetaPath: "b/meta.yml",
		},
	}

	idx, err := buildIndex(types)
	require.NoError(t, err, "collision is non-fatal when at least one side is inferred")
	got, ok := idx.LookupPrefix("X")
	require.True(t, ok)
	require.Equal(t, "alpha", got.TypeID, "first-seen winner keeps byPrefix")
}
