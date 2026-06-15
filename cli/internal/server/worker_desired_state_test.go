package server

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkerDesiredStateRoundTrip covers load/save/validate of
// .ddx/workers/desired.json including desired_count, default spec, restart
// policy, and updated_at (AC1).
func TestWorkerDesiredStateRoundTrip(t *testing.T) {
	root := t.TempDir()

	// Missing file is reported as os.ErrNotExist so callers can branch on it.
	_, err := LoadWorkerDesiredState(root)
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)

	want := &WorkerDesiredState{
		DesiredCount: 2,
		DefaultSpec: WorkerDefaultSpec{
			Mode:         "watch",
			IdleInterval: "30s",
			Profile:      "default",
			Harness:      "claude",
			Provider:     "anthropic",
			Model:        "sonnet",
			LabelFilter:  "area:server",
		},
		Restart: WorkerRestartPolicy{
			Enabled:            true,
			MaxRestartsPerHour: 6,
			Backoff:            "30s",
			BackoffMax:         "10m",
		},
	}

	before := time.Now().UTC()
	require.NoError(t, SaveWorkerDesiredState(root, want))
	// Save stamps Version, ProjectRoot, and UpdatedAt.
	assert.Equal(t, WorkerDesiredStateVersion, want.Version)
	assert.Equal(t, root, want.ProjectRoot)
	assert.False(t, want.UpdatedAt.Before(before), "UpdatedAt must be stamped on save")

	got, err := LoadWorkerDesiredState(root)
	require.NoError(t, err)
	assert.Equal(t, WorkerDesiredStateVersion, got.Version)
	assert.Equal(t, root, got.ProjectRoot)
	assert.Equal(t, 2, got.DesiredCount)
	assert.Equal(t, want.DefaultSpec, got.DefaultSpec)
	assert.Equal(t, want.Restart, got.Restart)
	assert.WithinDuration(t, want.UpdatedAt, got.UpdatedAt, time.Second)
}

func TestWorkerDesiredStateValidate(t *testing.T) {
	cases := []struct {
		name    string
		state   WorkerDesiredState
		wantErr bool
	}{
		{
			name:    "valid",
			state:   WorkerDesiredState{Version: WorkerDesiredStateVersion, DesiredCount: 1},
			wantErr: false,
		},
		{
			name:    "wrong version",
			state:   WorkerDesiredState{Version: 99, DesiredCount: 1},
			wantErr: true,
		},
		{
			name:    "negative desired count",
			state:   WorkerDesiredState{Version: WorkerDesiredStateVersion, DesiredCount: -1},
			wantErr: true,
		},
		{
			name:    "negative max restarts",
			state:   WorkerDesiredState{Version: WorkerDesiredStateVersion, Restart: WorkerRestartPolicy{MaxRestartsPerHour: -1}},
			wantErr: true,
		},
		{
			name:    "bad idle interval",
			state:   WorkerDesiredState{Version: WorkerDesiredStateVersion, DefaultSpec: WorkerDefaultSpec{IdleInterval: "nope"}},
			wantErr: true,
		},
		{
			name:    "bad backoff",
			state:   WorkerDesiredState{Version: WorkerDesiredStateVersion, Restart: WorkerRestartPolicy{Backoff: "nope"}},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.state.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
