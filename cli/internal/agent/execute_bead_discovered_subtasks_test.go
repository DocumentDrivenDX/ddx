package agent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExecuteBeadResultParsesDiscoveredSubtasks proves that discovered_subtasks
// round-trips from result.json into ExecuteBeadResult and that the field is
// absent (nil/omitted) when not present. FEAT-006 §406-426 / ddx-b523b55b AC2.
func TestExecuteBeadResultParsesDiscoveredSubtasks(t *testing.T) {
	t.Run("field populated when present", func(t *testing.T) {
		raw := []byte(`{
			"bead_id": "ddx-aabbccdd",
			"base_rev": "abc123",
			"outcome": "task_succeeded",
			"discovered_subtasks": [
				{
					"title": "Add read-coverage for X",
					"description": "Optional detail about what is needed",
					"labels": ["area:tests"],
					"priority": "P1"
				},
				{
					"title": "Refactor Y"
				}
			]
		}`)

		var res ExecuteBeadResult
		require.NoError(t, json.Unmarshal(raw, &res))

		require.Len(t, res.DiscoveredSubtasks, 2)
		require.Equal(t, "Add read-coverage for X", res.DiscoveredSubtasks[0].Title)
		require.Equal(t, "Optional detail about what is needed", res.DiscoveredSubtasks[0].Description)
		require.Equal(t, []string{"area:tests"}, res.DiscoveredSubtasks[0].Labels)
		require.Equal(t, "P1", res.DiscoveredSubtasks[0].Priority)
		require.Equal(t, "Refactor Y", res.DiscoveredSubtasks[1].Title)
		require.Empty(t, res.DiscoveredSubtasks[1].Description)
		require.Nil(t, res.DiscoveredSubtasks[1].Labels)

		// Re-marshal and verify the field is present in the output.
		out, err := json.Marshal(&res)
		require.NoError(t, err)
		var roundTripped map[string]any
		require.NoError(t, json.Unmarshal(out, &roundTripped))
		_, ok := roundTripped["discovered_subtasks"]
		require.True(t, ok, "discovered_subtasks must be present in marshaled output when populated")
	})

	t.Run("field absent when not present", func(t *testing.T) {
		raw := []byte(`{"bead_id": "ddx-aabbccdd", "base_rev": "abc123", "outcome": "task_succeeded"}`)

		var res ExecuteBeadResult
		require.NoError(t, json.Unmarshal(raw, &res))

		require.Nil(t, res.DiscoveredSubtasks, "DiscoveredSubtasks must be nil when absent in JSON")

		// Re-marshal and verify the field is omitted.
		out, err := json.Marshal(&res)
		require.NoError(t, err)
		var roundTripped map[string]any
		require.NoError(t, json.Unmarshal(out, &roundTripped))
		_, ok := roundTripped["discovered_subtasks"]
		require.False(t, ok, "discovered_subtasks must be omitted from marshaled output when nil")
	})

	t.Run("field absent when empty array in sidecar", func(t *testing.T) {
		// An empty array must not populate the field (matches sidecar-read logic:
		// len(subtasks) > 0 guard). Verify zero-value struct has nil field.
		var res ExecuteBeadResult
		require.Nil(t, res.DiscoveredSubtasks)

		out, err := json.Marshal(&res)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(out, &m))
		_, ok := m["discovered_subtasks"]
		require.False(t, ok, "discovered_subtasks must be omitted when nil (omitempty)")
	})
}
