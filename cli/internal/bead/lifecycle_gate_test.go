package bead

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLifecycleMigrationGateDetectsLegacyQueue(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, ".ddx"))
	require.NoError(t, os.MkdirAll(store.Dir, 0o755))

	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	rows := strings.Join([]string{
		`{"id":"ddx-human","title":"human","status":"open","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `","labels":["needs_human"]}`,
		`{"id":"ddx-investigate","title":"investigate","status":"open","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `","labels":["triage:needs-investigation"],"work-last-detail":"rerun with smart agent"}`,
		`{"id":"ddx-pseudo","title":"pseudo","status":"needs_investigation","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `"}`,
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(store.File, []byte(rows), 0o644))

	status, err := store.DetectLifecycleMigrationRequired()
	require.NoError(t, err)
	assert.True(t, status.QueuePresent)
	assert.True(t, status.SchemaMarkerMissing)
	assert.True(t, status.Required())
	assert.Equal(t, LifecycleMigrationGateCodeRequired, status.Code)
	assert.Equal(t, 1, status.LegacyNeedsHumanLabels)
	assert.Equal(t, 1, status.LegacyNeedsInvestigationLabels)
	assert.Equal(t, 1, status.LegacyNeedsInvestigationStatuses)
	assert.Equal(t, 1, status.LegacyNoChangesMetadataRows)
	assert.ElementsMatch(t, []string{"ddx-human", "ddx-investigate", "ddx-pseudo"}, status.SampleBeadIDs)
	assert.Contains(t, status.Error(), "ddx-human")
}

func TestLifecycleMigrationGateReportsMarkerlessCleanQueueWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, ".ddx"))

	missing, err := store.DetectLifecycleMigrationRequired()
	require.NoError(t, err)
	assert.False(t, missing.QueuePresent)
	assert.False(t, missing.Required())
	assert.Equal(t, LifecycleMigrationGateCodeNotRequired, missing.Code)

	require.NoError(t, os.MkdirAll(store.Dir, 0o755))
	now := time.Now().UTC().Format(time.RFC3339)
	row := `{"id":"ddx-clean","title":"clean","status":"open","priority":2,"issue_type":"task","created_at":"` + now + `","updated_at":"` + now + `"}` + "\n"
	require.NoError(t, os.WriteFile(store.File, []byte(row), 0o644))

	markerless, err := store.DetectLifecycleMigrationRequired()
	require.NoError(t, err)
	assert.True(t, markerless.QueuePresent)
	assert.True(t, markerless.SchemaMarkerMissing)
	assert.False(t, markerless.Required())
	assert.Equal(t, LifecycleMigrationGateCodeNotRequired, markerless.Code)
}

func TestLifecycleMigrationGateDoesNotRequireCleanMarkedQueue(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, ".ddx"))
	require.NoError(t, store.Init(testCtx()))
	require.NoError(t, store.WriteLifecycleSchemaMarker(time.Now().UTC()))
	now := time.Now().UTC().Format(time.RFC3339)
	row := `{"id":"ddx-clean","title":"clean","status":"open","priority":2,"issue_type":"task","created_at":"` + now + `","updated_at":"` + now + `"}` + "\n"
	require.NoError(t, os.WriteFile(store.File, []byte(row), 0o644))

	clean, err := store.DetectLifecycleMigrationRequired()
	require.NoError(t, err)
	assert.True(t, clean.QueuePresent)
	assert.False(t, clean.SchemaMarkerMissing)
	assert.False(t, clean.Required())
	assert.Equal(t, LifecycleMigrationGateCodeNotRequired, clean.Code)
}

func TestLifecycleMigrationGateRequiresOldMarker(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, ".ddx"))
	require.NoError(t, store.Init(testCtx()))
	require.NoError(t, os.WriteFile(store.LifecycleSchemaMarkerPath(), []byte(`{"version":0}`+"\n"), 0o644))

	status, err := store.DetectLifecycleMigrationRequired()
	require.NoError(t, err)
	assert.True(t, status.SchemaMarkerOld)
	assert.True(t, status.Required())
	assert.Equal(t, LifecycleMigrationGateCodeRequired, status.Code)
}
