package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSupervisorRegistry_ReconcileAllVisitsEachRegisteredProject(t *testing.T) {
	workDir := setupTestDir(t)
	srv := New(":0", workDir)
	require.NotNil(t, srv.supervisorRegistry)
	t.Cleanup(func() {
		_ = srv.Shutdown()
	})

	badRoot := t.TempDir()
	goodRootA := t.TempDir()
	goodRootB := t.TempDir()
	for _, root := range []string{badRoot, goodRootA, goodRootB} {
		initSupervisorProject(t, root)
	}

	writeDesiredState(t, goodRootA, 0)
	writeDesiredState(t, goodRootB, 0)
	writeMalformedDesiredState(t, badRoot)

	srv.RegisterProject(badRoot)
	srv.RegisterProject(goodRootA)
	srv.RegisterProject(goodRootB)

	err := srv.supervisorRegistry.ReconcileAll()
	require.Error(t, err, "malformed desired.json should surface an error")
	require.Len(t, srv.supervisorRegistry.supervisors, 4, "registry must create one supervisor per registered project")
	require.NoError(t, srv.Shutdown())
}

func TestServer_MultiProjectSupervisorPicksUpNewProjects(t *testing.T) {
	workDir := setupTestDir(t)
	srv := New(":0", workDir)
	require.NotNil(t, srv.supervisorRegistry)

	projectA := t.TempDir()
	projectB := t.TempDir()
	initSupervisorProject(t, projectA)
	initSupervisorProject(t, projectB)
	writeDesiredState(t, projectA, 1)
	writeDesiredState(t, projectB, 1)

	srv.RegisterProject(projectA)
	srv.RegisterProject(projectB)

	t.Setenv("DDX_SUPERVISOR_TICK", "50ms")
	srv.StartSupervisor()
	t.Cleanup(func() {
		_ = srv.Shutdown()
	})

	require.Eventually(t, func() bool {
		supA := srv.supervisorRegistry.getOrCreate(projectA)
		supB := srv.supervisorRegistry.getOrCreate(projectB)
		if supA == nil || supA.manager == nil || supB == nil || supB.manager == nil {
			return false
		}
		return runningManagedWorkerCount(t, supA.manager, projectA) == 1 &&
			runningManagedWorkerCount(t, supB.manager, projectB) == 1
	}, 2*time.Second, 25*time.Millisecond)
	require.NoError(t, srv.Shutdown())
}

func TestServer_DDXSupervisedProjectsEnvRegistersProjects(t *testing.T) {
	workDir := setupTestDir(t)

	projectA := t.TempDir()
	projectB := t.TempDir()
	initSupervisorProject(t, projectA)
	initSupervisorProject(t, projectB)
	writeDesiredState(t, projectA, 0)
	writeDesiredState(t, projectB, 0)

	t.Setenv("DDX_SUPERVISED_PROJECTS", strings.Join([]string{projectA, projectB}, string(os.PathListSeparator)))

	srv := New(":0", workDir)
	require.NotNil(t, srv.State())

	projects := srv.State().GetProjects()
	paths := make([]string, 0, len(projects))
	for _, proj := range projects {
		paths = append(paths, proj.Path)
	}

	assert.Contains(t, paths, workDir)
	assert.Contains(t, paths, projectA)
	assert.Contains(t, paths, projectB)
	assert.Len(t, paths, 3)
}

func initSupervisorProject(t *testing.T, root string) {
	t.Helper()
	ddxDir := testutils.MakeInitializedDDxRoot(t, root)
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), []byte(""), 0o644))
}

func writeDesiredState(t *testing.T, root string, desiredCount int) {
	t.Helper()
	supervisor := NewWorkerSupervisor(NewWorkerManager(root))
	desired := DefaultWorkerDesiredState(root)
	desired.DesiredCount = desiredCount
	desired.DefaultSpec.OpaquePassthrough = true
	require.NoError(t, supervisor.SaveDesiredState(&desired))
}

func writeMalformedDesiredState(t *testing.T, root string) {
	t.Helper()
	ddxDir := testutils.MakeInitializedDDxRoot(t, root)
	workersDir := filepath.Join(ddxDir, "workers")
	require.NoError(t, os.MkdirAll(workersDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workersDir, "desired.json"), []byte("{"), 0o644))
}
