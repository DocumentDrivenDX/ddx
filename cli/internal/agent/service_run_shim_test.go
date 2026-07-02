package agent

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunWithConfigViaService_InstallsProviderShim asserts that a production
// call to RunWithConfigViaService (the entry point exercised by ddx run, ddx
// try, ddx work, and the server) installs the provider PATH shim before
// constructing the Fizeau service. Without this, fizeau's LookPath("codex")
// finds the raw provider binary and spawns it with only Setpgid — no
// Pdeathsig — so a SIGKILL of the worker leaks the provider child as a
// ppid=1 orphan (bead ddx-f2b413ea).
func TestRunWithConfigViaService_InstallsProviderShim(t *testing.T) {
	stub := &passthroughTestService{}
	SetServiceRunFactory(func(string) (agentlib.FizeauService, error) {
		return stub, nil
	})
	t.Cleanup(func() { SetServiceRunFactory(nil) })

	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{
		Model: "haiku",
	}).Resolve(config.CLIOverrides{Harness: "agent"})

	_, err := RunWithConfigViaService(context.Background(), t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "test",
	})
	require.NoError(t, err)
	require.True(t, stub.executeCalled)

	path := os.Getenv("PATH")
	found := false
	for _, entry := range strings.Split(path, string(os.PathListSeparator)) {
		if strings.Contains(entry, "ddx-provider-shim-") {
			found = true
			break
		}
	}
	assert.True(t, found,
		"RunWithConfigViaService must install ddx-provider-shim on PATH so fizeau's LookPath(codex/claude/…) finds the Pdeathsig wrapper; got PATH=%s", path)
}
