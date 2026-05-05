package axon

import (
	"context"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/require"
)

type noopTransport struct{}

func (noopTransport) Query(context.Context, string, map[string]any, any) error {
	return nil
}

func TestAxonClient_SchemaBindingsCompile(t *testing.T) {
	t.Parallel()

	client := NewClient(noopTransport{})
	_, err := client.GetBead(context.Background(), "ddx-00000001")
	require.NoError(t, err)

	local := bead.Bead{
		ID:          "ddx-00000001",
		Title:       "scaffold",
		Status:      bead.StatusOpen,
		Priority:    bead.DefaultPriority,
		IssueType:   bead.DefaultType,
		Owner:       "owner",
		CreatedAt:   time.Unix(10, 0).UTC(),
		CreatedBy:   "creator",
		UpdatedAt:   time.Unix(20, 0).UTC(),
		Labels:      []string{"kind:feature"},
		Parent:      "ddx-00000002",
		Description: "local model",
		Acceptance:  "AC",
		Notes:       "notes",
		Dependencies: []bead.Dependency{{
			IssueID:     "ddx-00000001",
			DependsOnID: "ddx-00000002",
			Type:        "blocks",
			CreatedAt:   "2026-05-04T00:00:00Z",
			CreatedBy:   "creator",
			Metadata:    "meta",
		}},
		Extra: map[string]any{"source": "test"},
	}

	remote := BeadFromLocal(local)
	require.Equal(t, local.ID, remote.ID)
	require.Equal(t, local.Title, remote.Title)
	require.Equal(t, local.Status, remote.Status)
	require.Equal(t, local.Priority, remote.Priority)
	require.Equal(t, local.IssueType, remote.IssueType)
	require.Equal(t, local.Owner, remote.Owner)
	require.Equal(t, local.CreatedAt, remote.CreatedAt)
	require.Equal(t, local.CreatedBy, remote.CreatedBy)
	require.Equal(t, local.UpdatedAt, remote.UpdatedAt)
	require.Equal(t, local.Labels, remote.Labels)
	require.Equal(t, local.Parent, remote.Parent)
	require.Equal(t, local.Description, remote.Description)
	require.Equal(t, local.Acceptance, remote.Acceptance)
	require.Equal(t, local.Notes, remote.Notes)
	require.Equal(t, local.Extra, remote.Extra)
	require.Len(t, remote.Dependencies, 1)
	require.Equal(t, local.Dependencies[0].IssueID, remote.Dependencies[0].IssueID)
	require.Equal(t, local.Dependencies[0].DependsOnID, remote.Dependencies[0].DependsOnID)
	require.Equal(t, local.Dependencies[0].Type, remote.Dependencies[0].Type)
	require.Equal(t, local.Dependencies[0].CreatedAt, remote.Dependencies[0].CreatedAt)
	require.Equal(t, local.Dependencies[0].CreatedBy, remote.Dependencies[0].CreatedBy)
	require.Equal(t, local.Dependencies[0].Metadata, remote.Dependencies[0].Metadata)

	back := remote.ToLocal()
	require.Equal(t, local, back)

	input := BeadInputFromLocal(local)
	require.Equal(t, local.ID, input.ID)
	require.Equal(t, local.Title, input.Title)
	require.Equal(t, local.Extra, input.Extra)
	require.Len(t, input.Dependencies, 1)

	lifecycle := bead.LifecycleEvent{
		EventID:   "evt-1",
		BeadID:    local.ID,
		Kind:      "created",
		Summary:   "created",
		Body:      "body",
		Actor:     "actor",
		Timestamp: time.Unix(30, 0).UTC(),
	}
	change := ChangeEventFromLifecycle(lifecycle)
	require.Equal(t, lifecycle.EventID, change.EventID)
	require.Equal(t, lifecycle.BeadID, change.BeadID)
	require.Equal(t, lifecycle.Kind, change.Kind)
	require.Equal(t, lifecycle.Summary, change.Summary)
	require.Equal(t, lifecycle.Body, change.Body)
	require.Equal(t, lifecycle.Actor, change.Actor)
	require.Equal(t, lifecycle.Timestamp, change.Timestamp)
	require.Equal(t, lifecycle, change.ToLifecycleEvent())

	_, err = client.ListBeads(context.Background())
	require.NoError(t, err)

	_, err = client.CreateBead(context.Background(), input)
	require.NoError(t, err)

	_, err = client.UpdateBead(context.Background(), local.ID, 7, input)
	require.NoError(t, err)
}
