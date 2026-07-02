package agent

import (
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent/work"
	"github.com/DocumentDrivenDX/ddx/internal/trackerpaths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedTrackerPathListsStayInSync(t *testing.T) {
	require.Equal(t, []string{
		".ddx/beads.jsonl",
		".ddx/beads-archive.jsonl",
		".ddx/metrics/attempts.jsonl",
		".ddx/attachments",
	}, trackerpaths.ManagedPathspecs())

	cases := []string{
		".ddx/beads.jsonl",
		".ddx/beads-archive.jsonl",
		".ddx/metrics/attempts.jsonl",
		".ddx/attachments/example",
	}

	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			require.True(t, trackerpaths.IsManagedTrackerPath(path))
			require.True(t, work.IsManagedTrackerPath(path))

			assert.Equal(t, []string{path}, dirtyDurableAuditPaths(" M "+path+"\n"))

			repo := newLandTestRepo(t)
			repo.writeFile(path, "managed\n")
			repo.runGit("add", path)

			blocking, ok := blockingStagedPaths(repo.dir)
			require.True(t, ok)
			assert.Empty(t, blocking)
		})
	}
}
