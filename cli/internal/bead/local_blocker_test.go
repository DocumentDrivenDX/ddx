package bead

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalBlockerRef_RoundTrip(t *testing.T) {
	ref, err := NewLocalBlockerRef(
		LocalBlockerKindLocalResourceExhaustion,
		[]string{" /var/tmp ", "/var/lib/ddx"},
		" fingerprint-123 ",
	)
	require.NoError(t, err)

	data, err := json.Marshal(ref)
	require.NoError(t, err)

	var got LocalBlockerRef
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, ref, got)

	parsed, ok := ParseLocalBlockerRef(map[string]any{
		"kind":           "local_resource_exhaustion",
		"resource_roots": []any{"/var/tmp", "/var/lib/ddx"},
		"fingerprint":    "fingerprint-123",
	})
	require.True(t, ok)
	assert.Equal(t, ref, parsed)

	_, ok = ParseLocalBlockerRef(map[string]any{
		"kind":           "unknown",
		"resource_roots": []any{"/var/tmp"},
	})
	assert.False(t, ok)

	parsed, ok = ParseLocalBlockerRef(map[string]any{
		"kind": "local_resource_exhaustion",
	})
	require.True(t, ok)
	assert.Equal(t, LocalBlockerRef{Kind: LocalBlockerKindLocalResourceExhaustion, ResourceRoots: []string{}}, parsed)

	_, err = NewLocalBlockerRef("", []string{"/var/tmp"}, "")
	require.Error(t, err)
}

func TestLifecycleTransition_ClearsLocalBlockerOnOpen(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{
		Title:  "blocked by local resources",
		Status: StatusBlocked,
		Extra: map[string]any{
			ExtraLifecycleExternalBlockerReason: "host resource pressure",
			ExtraLifecycleLocalBlockerRef: LocalBlockerRef{
				Kind:          LocalBlockerKindLocalResourceExhaustion,
				ResourceRoots: []string{"/var/tmp", "/var/lib/ddx"},
				Fingerprint:   "fingerprint-123",
			},
		},
	}
	require.NoError(t, s.Create(testCtx(), b))

	require.NoError(t, s.SetLifecycleStatus(b.ID, StatusOpen, LifecycleTransitionOptions{ManualReopen: true}))

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, got.Status)
	assert.NotContains(t, got.Extra, ExtraLifecycleExternalBlockerReason)
	assert.NotContains(t, got.Extra, ExtraLifecycleLocalBlockerRef)
}
