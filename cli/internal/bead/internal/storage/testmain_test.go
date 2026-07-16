package storage

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	storageTestHelperModeEnv    = "DDX_STORAGE_TEST_HELPER_MODE"
	storageTestProjectDirEnv    = "DDX_STORAGE_TEST_PROJECT_DIR"
	storageTestExpectedHomeEnv  = "DDX_STORAGE_TEST_EXPECTED_HOME"
	storageTestExpectedXDGEnv   = "DDX_STORAGE_TEST_EXPECTED_XDG"
	storageTestExpectedGitDir   = "DDX_STORAGE_TEST_EXPECTED_GIT_DIR"
	storageTestExpectedGitWork  = "DDX_STORAGE_TEST_EXPECTED_GIT_WORK_TREE"
	storageTestHelperOperator   = "operator-home"
	storageTestHelperProcessEnv = "process-state"
)

var (
	storageTestHome             string
	storageTestXDGDataHome      string
	inheritedStorageTestHome    string
	inheritedStorageTestXDG     string
	inheritedStorageTestGitVars map[string]string
)

// TestMain prevents the internal storage package from reading operator-level
// DDx or git state. Store selection tests must depend only on their fixtures.
func TestMain(m *testing.M) {
	inheritedStorageTestHome = os.Getenv("HOME")
	inheritedStorageTestXDG = os.Getenv("XDG_DATA_HOME")
	inheritedStorageTestGitVars = make(map[string]string)
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok && strings.HasPrefix(name, "GIT_") {
			inheritedStorageTestGitVars[name] = value
			if err := os.Unsetenv(name); err != nil {
				fmt.Fprintf(os.Stderr, "storage tests: unset %s: %v\n", name, err)
				os.Exit(1)
			}
		}
	}

	stateRoot, err := os.MkdirTemp("", "ddx-storage-test-state-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "storage tests: create isolated state root: %v\n", err)
		os.Exit(1)
	}
	storageTestHome = filepath.Join(stateRoot, "home")
	storageTestXDGDataHome = filepath.Join(stateRoot, "xdg-data")
	for _, dir := range []string{storageTestHome, storageTestXDGDataHome} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "storage tests: create isolated directory: %v\n", err)
			_ = os.RemoveAll(stateRoot)
			os.Exit(1)
		}
	}
	if err := os.Setenv("HOME", storageTestHome); err != nil {
		fmt.Fprintf(os.Stderr, "storage tests: set isolated HOME: %v\n", err)
		_ = os.RemoveAll(stateRoot)
		os.Exit(1)
	}
	if err := os.Setenv("XDG_DATA_HOME", storageTestXDGDataHome); err != nil {
		fmt.Fprintf(os.Stderr, "storage tests: set isolated XDG_DATA_HOME: %v\n", err)
		_ = os.RemoveAll(stateRoot)
		os.Exit(1)
	}

	code := m.Run()
	if err := os.RemoveAll(stateRoot); err != nil && code == 0 {
		fmt.Fprintf(os.Stderr, "storage tests: remove isolated state root: %v\n", err)
		code = 1
	}
	os.Exit(code)
}

func TestInternalStoragePackageIgnoresOperatorHomeConfig(t *testing.T) {
	if os.Getenv(storageTestHelperModeEnv) == storageTestHelperOperator {
		hostileHome := os.Getenv(storageTestExpectedHomeEnv)
		require.Equal(t, hostileHome, inheritedStorageTestHome)
		require.NotEqual(t, hostileHome, os.Getenv("HOME"))

		projectDir := os.Getenv(storageTestProjectDirEnv)
		require.NotEmpty(t, projectDir)
		store := NewStore(filepath.Join(projectDir, ddxroot.DirName))
		require.NoError(t, store.Init(context.Background()))
		require.DirExists(t, filepath.Join(projectDir, ddxroot.DirName, "axon"))
		return
	}

	hostileHome := t.TempDir()
	hostileConfigDir := filepath.Join(hostileHome, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(hostileConfigDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(hostileConfigDir, "config.yaml"),
		[]byte("version: [\n"),
		0o644,
	))

	projectDir := t.TempDir()
	writeStoreConfig(t, projectDir, BackendAxon)
	runStorageTestSubprocess(t, "TestInternalStoragePackageIgnoresOperatorHomeConfig",
		storageTestHelperModeEnv+"="+storageTestHelperOperator,
		storageTestExpectedHomeEnv+"="+hostileHome,
		storageTestProjectDirEnv+"="+projectDir,
		"HOME="+hostileHome,
	)
}

func TestInternalStoragePackageScrubsInheritedProcessState(t *testing.T) {
	if os.Getenv(storageTestHelperModeEnv) == storageTestHelperProcessEnv {
		hostileHome := os.Getenv(storageTestExpectedHomeEnv)
		hostileXDG := os.Getenv(storageTestExpectedXDGEnv)
		require.Equal(t, hostileHome, inheritedStorageTestHome)
		require.Equal(t, hostileXDG, inheritedStorageTestXDG)
		require.Equal(t, os.Getenv(storageTestExpectedGitDir), inheritedStorageTestGitVars["GIT_DIR"])
		require.Equal(t, os.Getenv(storageTestExpectedGitWork), inheritedStorageTestGitVars["GIT_WORK_TREE"])

		assert.Equal(t, storageTestHome, os.Getenv("HOME"))
		assert.Equal(t, storageTestXDGDataHome, os.Getenv("XDG_DATA_HOME"))
		assert.NotEqual(t, hostileHome, os.Getenv("HOME"))
		assert.NotEqual(t, hostileXDG, os.Getenv("XDG_DATA_HOME"))
		assert.DirExists(t, storageTestHome)
		assert.DirExists(t, storageTestXDGDataHome)
		for _, entry := range os.Environ() {
			assert.Falsef(t, strings.HasPrefix(entry, "GIT_"), "inherited git state remained: %s", entry)
		}
		return
	}

	hostileHome := filepath.Join(t.TempDir(), "operator-home")
	hostileXDG := filepath.Join(t.TempDir(), "operator-xdg")
	hostileGitDir := filepath.Join(t.TempDir(), "operator-git-dir")
	hostileGitWorkTree := filepath.Join(t.TempDir(), "operator-work-tree")
	runStorageTestSubprocess(t, "TestInternalStoragePackageScrubsInheritedProcessState",
		storageTestHelperModeEnv+"="+storageTestHelperProcessEnv,
		storageTestExpectedHomeEnv+"="+hostileHome,
		storageTestExpectedXDGEnv+"="+hostileXDG,
		storageTestExpectedGitDir+"="+hostileGitDir,
		storageTestExpectedGitWork+"="+hostileGitWorkTree,
		"HOME="+hostileHome,
		"XDG_DATA_HOME="+hostileXDG,
		"GIT_DIR="+hostileGitDir,
		"GIT_WORK_TREE="+hostileGitWorkTree,
	)
}

func runStorageTestSubprocess(t *testing.T, testName string, overrides ...string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+testName+"$", "-test.count=1")
	cmd.Env = storageTestSubprocessEnv(overrides...)
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "subprocess output:\n%s", output)
}

func storageTestSubprocessEnv(overrides ...string) []string {
	replaced := make(map[string]struct{}, len(overrides))
	for _, entry := range overrides {
		name, _, _ := strings.Cut(entry, "=")
		replaced[name] = struct{}{}
	}

	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || strings.HasPrefix(name, "GIT_") {
			continue
		}
		if _, ok := replaced[name]; !ok {
			env = append(env, entry)
		}
	}
	return append(env, overrides...)
}
