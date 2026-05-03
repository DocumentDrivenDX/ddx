package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPersistReviewerStream_WritesToBundle covers ddx-f8a11202 AC #2: a large
// reviewer stream persists to the attempt's execution bundle; the returned
// relative path is suitable for an event-body reference.
func TestPersistReviewerStream_WritesToBundle(t *testing.T) {
	projectRoot := t.TempDir()
	attemptID := "20260420T150000-abc123"
	beadID := "ddx-test-f8a11202"

	stream := strings.Repeat("S", 2*1024*1024) // 2MB reviewer stream

	rel, err := PersistReviewerStream(projectRoot, beadID, attemptID, stream)
	require.NoError(t, err)
	require.NotEmpty(t, rel)

	abs := filepath.Join(projectRoot, rel)
	data, err := os.ReadFile(abs)
	require.NoError(t, err)
	assert.Equal(t, len(stream), len(data), "full stream must land on disk verbatim; truncation here loses audit evidence")
	assert.Equal(t, filepath.Join(".ddx", "executions", attemptID, "reviewer-stream.log"), rel,
		"artifact location must follow the .ddx/executions/<attempt>/ convention so existing tooling discovers it")
}

// TestPersistReviewerStream_FallbackWhenAttemptMissing covers the path where
// the review happens outside an execute-bead attempt (manual review, test
// invocations). The stream still persists — in a grouped reviewer-streams
// directory — so evidence is never dropped.
func TestPersistReviewerStream_FallbackWhenAttemptMissing(t *testing.T) {
	projectRoot := t.TempDir()

	rel, err := PersistReviewerStream(projectRoot, "ddx-test", "", "verdict text")
	require.NoError(t, err)
	require.NotEmpty(t, rel)
	assert.Contains(t, rel, filepath.Join(".ddx", "executions", "reviewer-streams"),
		"fallback location must be discoverable via the same .ddx/executions/ root")
	abs := filepath.Join(projectRoot, rel)
	_, err = os.Stat(abs)
	require.NoError(t, err, "fallback must actually write the file, not just return a path")
}

// TestReviewEventBody_CapsAtMax covers AC #2: the event body for a reviewer
// verdict event must be at most 512 bytes regardless of reviewer rationale
// size. A 10KB rationale shall be summarized + artifact-referenced, not
// truncated in a way that loses the verdict or the path.
func TestReviewEventBody_CapsAtMax(t *testing.T) {
	rationale := strings.Repeat("A very long first line of rationale that must be truncated. ", 200)
	artifactPath := ".ddx/executions/20260420T150000-abc123/reviewer-stream.log"

	body := ReviewEventBody("APPROVE", rationale, artifactPath)

	assert.LessOrEqual(t, len(body), maxReviewerEventBody,
		"event body must never exceed the cap — downstream scanners and bd imports rely on per-field limits")
	assert.True(t, strings.HasPrefix(body, "APPROVE"),
		"verdict must be the first line so human readers see it without scrolling")
	assert.Contains(t, body, artifactPath,
		"artifact path must survive truncation — without it the reviewer stream is orphaned from its bead")
}

// TestReviewEventBody_EmptyRationaleStillCarriesVerdictAndArtifact covers the
// BLOCK-without-rationale malfunction path: even when the reviewer produced
// no usable rationale, the event must still link to the raw stream on disk so
// the operator can debug what happened.
func TestReviewEventBody_EmptyRationaleStillCarriesVerdictAndArtifact(t *testing.T) {
	artifactPath := ".ddx/executions/20260420T150000-abc123/reviewer-stream.log"

	body := ReviewEventBody("BLOCK without rationale", "", artifactPath)

	assert.Contains(t, body, "BLOCK without rationale")
	assert.Contains(t, body, artifactPath,
		"malfunction events must always reference the raw stream — this is the one case where forensics matter most")
}

// TestReviewEventBody_NoArtifactPath_StillFits covers the artifact-write-error
// fallback: if the sidecar write fails we still emit a body with verdict +
// short rationale, we just skip the artifact line. Body must still be bounded.
func TestReviewEventBody_NoArtifactPath_StillFits(t *testing.T) {
	rationale := strings.Repeat("x", 5000)
	body := ReviewEventBody("APPROVE", rationale, "")

	assert.LessOrEqual(t, len(body), maxReviewerEventBody)
	assert.True(t, strings.HasPrefix(body, "APPROVE"))
	assert.NotContains(t, body, "artifact:",
		"when no artifact was written, the body must not dangle a broken reference")
}
