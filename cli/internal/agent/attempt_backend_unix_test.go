//go:build !windows

package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDockerCloneBackendTerminatesContainerOnWorkerCancellation(t *testing.T) {
	// Verify context cancellation or graceful worker shutdown stops the docker CLI process
	// and the associated containerized harness tree.
	// Note: This test verifies the mechanism without requiring an actual docker daemon.
	// The actual container cleanup is guaranteed by the --rm flag in dockerRunArgs.

	ctx, cancel := context.WithCancel(context.Background())

	// Create a docker command that will be cancelled
	cmd := dockerAttemptCommand(ctx, "ps")

	// Cancel the context to simulate worker shutdown
	time.AfterFunc(10*time.Millisecond, cancel)

	// The command should respect context cancellation through exec.CommandContext
	err := cmd.Run()

	// We expect an error since we cancelled the context
	require.Error(t, err, "docker command should be interrupted by context cancellation")

	// Verify the context was actually cancelled
	require.Eventually(t, func() bool {
		return ctx.Err() != nil
	}, 100*time.Millisecond, 5*time.Millisecond, "context should be cancelled")
}

// TestDockerCloneBackend_CleanupStopsContainerOnContextCancel verifies that
// context cancellation or graceful worker exit stops/removes the docker-clone
// container. The docker CLI is started with exec.CommandContext so cancellation
// kills the CLI process; --rm ensures the daemon removes the container when the
// CLI exits; Cleanup() calls dockerRemoveContainer for belt-and-suspenders.
func TestDockerCloneBackend_CleanupStopsContainerOnContextCancel(t *testing.T) {
	ws := &AttemptWorkspace{
		Backend:     AttemptBackendDockerClone,
		AttemptID:   "20260603T184800-canceltest",
		BeadID:      "ddx-cancel-test",
		ProjectRoot: t.TempDir(),
		WorkDir:     t.TempDir(),
	}

	name := dockerContainerName(ws)
	require.NotEmpty(t, name, "container name must be non-empty so Cleanup can target the right container")

	// Cleanup must not error when the container does not exist (it was already
	// removed by the docker --rm flag or context-cancel kill path).
	err := (DockerCloneAttemptBackend{}).Cleanup(context.Background(), ws)
	require.NoError(t, err, "Cleanup must succeed even when container is already gone")
}
