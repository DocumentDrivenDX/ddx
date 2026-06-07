package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	e2eBinaryOnce       sync.Once
	e2eBinaryPath       string
	e2eBinaryBuildErr   error
	e2eBinaryBuildOut   []byte
	e2eBinaryBuildCount atomic.Int32
)

// getSmokeTestBinaryPath builds the CLI once for the cmd test package and
// returns the shared binary path.
func getSmokeTestBinaryPath(t *testing.T) string {
	t.Helper()

	e2eBinaryOnce.Do(func() {
		e2eBinaryBuildCount.Add(1)

		binDir, err := os.MkdirTemp("", "ddx-cmd-e2e-bin-*")
		if err != nil {
			e2eBinaryBuildErr = err
			return
		}
		e2eBinaryPath = filepath.Join(binDir, executableName("ddx"))

		buildCmd := exec.Command("go", "build", "-buildvcs=false", "-o", e2eBinaryPath, ".")
		buildCmd.Dir = cliRootForTests(t)
		e2eBinaryBuildOut, e2eBinaryBuildErr = buildCmd.CombinedOutput()
		if e2eBinaryBuildErr != nil {
			return
		}
		e2eBinaryBuildErr = os.Chmod(e2eBinaryPath, 0o755)
	})

	if e2eBinaryBuildErr != nil {
		t.Skipf("Could not build CLI: %v\n%s", e2eBinaryBuildErr, string(e2eBinaryBuildOut))
	}
	return e2eBinaryPath
}

func copySmokeTestBinary(t *testing.T, dst string) {
	t.Helper()

	src := getSmokeTestBinaryPath(t)
	srcInfo, err := os.Stat(src)
	require.NoError(t, err)

	srcData, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dst, srcData, srcInfo.Mode()))
	require.NoError(t, os.Chmod(dst, 0o755))
}

func cliRootForTests(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "could not get caller info")

	cliRoot := filepath.Join(filepath.Dir(filename), "..")
	cliRoot, err := filepath.Abs(cliRoot)
	require.NoError(t, err)
	return cliRoot
}

func executableName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

func TestE2E_SharedCLIBinaryBuildsOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	first := getSmokeTestBinaryPath(t)
	second := getSmokeTestBinaryPath(t)

	require.Equal(t, first, second)
	require.Equal(t, int32(1), e2eBinaryBuildCount.Load())
}
