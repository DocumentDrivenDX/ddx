package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedLegacyLifecycleQueue(t *testing.T, workingDir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(workingDir, ddxroot.DirName), 0o755))
	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	rows := strings.Join([]string{
		`{"id":"ddx-human","title":"human","status":"open","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `","labels":["needs_human"]}`,
		`{"id":"ddx-pseudo","title":"pseudo","status":"needs_investigation","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `","work-last-detail":"rerun with smart agent"}`,
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, ddxroot.DirName, "beads.jsonl"), []byte(rows), 0o644))
}

func TestLifecycleQueueGate_BlocksWorkAndServerOnLegacyQueue(t *testing.T) {
	workingDir := t.TempDir()
	seedLegacyLifecycleQueue(t, workingDir)

	workFactory := newBeadTestRoot(t, workingDir)
	workOut, workErr := executeCommand(workFactory.NewRootCommand(), "work", "--once", "--json")
	require.Error(t, workErr)
	assert.Contains(t, workOut, bead.LifecycleMigrationGateCodeRequired)
	var workStatus bead.LifecycleMigrationGateStatus
	require.NoError(t, json.Unmarshal([]byte(workOut), &workStatus))
	assert.True(t, workStatus.Required())
	assert.Equal(t, 1, workStatus.LegacyNeedsHumanLabels)
	assert.Equal(t, 1, workStatus.LegacyNeedsInvestigationStatuses)

	tryFactory := newBeadTestRoot(t, workingDir)
	tryOut, tryErr := executeCommand(tryFactory.NewRootCommand(), "try", "ddx-human")
	require.Error(t, tryErr)
	assert.Contains(t, tryErr.Error(), bead.LifecycleMigrationGateCodeRequired)
	assert.Empty(t, tryOut)

	serverFactory := newBeadTestRoot(t, workingDir)
	serverStarted := false
	serverFactory.serverListenAndServeOverride = func(cert, key string) error {
		serverStarted = true
		return nil
	}
	serverOut, serverErr := executeCommand(serverFactory.NewRootCommand(), "server", "--tsnet=false")
	require.Error(t, serverErr)
	assert.False(t, serverStarted, "server RunE must not start on an unmigrated lifecycle queue")
	assert.Contains(t, serverErr.Error(), bead.LifecycleMigrationGateCodeRequired)
	assert.Contains(t, serverErr.Error(), "ddx bead migrate --lifecycle --apply")
	assert.Empty(t, serverOut)
}

func TestLifecycleQueueGate_AllowsMigrationDoctorHelpVersion(t *testing.T) {
	workingDir := t.TempDir()
	seedLegacyLifecycleQueue(t, workingDir)

	helpOut, helpErr := executeCommand(newBeadTestRoot(t, workingDir).NewRootCommand(), "--help")
	require.NoError(t, helpErr)
	assert.Contains(t, helpOut, "Document-Driven Development")

	versionOut, versionErr := executeCommand(newBeadTestRoot(t, workingDir).NewRootCommand(), "version")
	require.NoError(t, versionErr)
	assert.Contains(t, versionOut, "DDx")

	doctorOut, doctorErr := executeCommand(newBeadTestRoot(t, workingDir).NewRootCommand(), "bead", "doctor", "--json")
	require.NoError(t, doctorErr)
	assert.NotContains(t, doctorOut, bead.LifecycleMigrationGateCodeRequired)

	migrateOut, migrateErr := executeCommand(newBeadTestRoot(t, workingDir).NewRootCommand(), "bead", "migrate", "--lifecycle", "--dry-run", "--json")
	require.NoError(t, migrateErr)
	assert.Contains(t, migrateOut, `"legacy_needs_human_labels": 1`)
}

func TestLifecycleQueueGate_MigrateApplyClearsGate(t *testing.T) {
	workingDir := t.TempDir()
	seedLegacyLifecycleQueue(t, workingDir)

	beforeOut, beforeErr := executeCommand(newBeadTestRoot(t, workingDir).NewRootCommand(), "bead", "ready", "--json")
	require.Error(t, beforeErr)
	assert.Contains(t, beforeOut, bead.LifecycleMigrationGateCodeRequired)

	applyOut, applyErr := executeCommand(newBeadTestRoot(t, workingDir).NewRootCommand(), "bead", "migrate", "--lifecycle", "--apply", "--json")
	require.NoError(t, applyErr)
	assert.Contains(t, applyOut, `"marker_written": true`)

	afterOut, afterErr := executeCommand(newBeadTestRoot(t, workingDir).NewRootCommand(), "bead", "ready", "--json")
	require.NoError(t, afterErr)
	assert.NotContains(t, afterOut, bead.LifecycleMigrationGateCodeRequired)
	assert.FileExists(t, filepath.Join(workingDir, ddxroot.DirName, bead.LifecycleSchemaMarkerFile))
}
