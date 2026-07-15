package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFizeauUnavailableReturnsTypedExecutionFailure(t *testing.T) {
	stub := &passthroughTestService{
		executeErr: fmt.Errorf("service unavailable: connection refused"),
	}
	SetServiceRunFactory(func(string) (agentlib.FizeauService, error) {
		return stub, nil
	})
	t.Cleanup(func() { SetServiceRunFactory(nil) })

	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{})
	_, err := RunWithConfigViaService(context.Background(), t.TempDir(), rcfg, AgentRunRuntime{Prompt: "test"})
	require.Error(t, err)

	var typed *ProviderFailureError
	require.True(t, errors.As(err, &typed), "immediate Fizeau Execute errors must cross the boundary as ProviderFailureError: %v", err)
	assert.Equal(t, FailureModeProviderConnectivity, typed.Failure.Reason)
	assert.True(t, typed.Failure.Retryable)
	assert.True(t, stub.executeCalled)
	assert.Empty(t, stub.lastReq.Harness, "DDx must not select a fallback harness")
	assert.Empty(t, stub.lastReq.Provider, "DDx must not select a fallback provider")
	assert.Empty(t, stub.lastReq.Model, "DDx must not select a fallback model")
}

func TestCapabilitiesViaServiceUsesOnlyFizeauInventory(t *testing.T) {
	stub := &passthroughTestService{harnessInfos: []agentlib.HarnessInfo{{
		Name:                 "opaque-harness",
		Available:            true,
		Path:                 "/fizeau/inventory/opaque-harness",
		DefaultModel:         "opaque-default",
		SupportedPermissions: []string{"safe", "unrestricted"},
		SupportedReasoning:   []string{"odd", "strong"},
		CostClass:            "local",
		ExactPinSupport:      true,
	}}}
	SetServiceRunFactory(func(string) (agentlib.FizeauService, error) {
		return stub, nil
	})
	t.Cleanup(func() { SetServiceRunFactory(nil) })

	caps, err := CapabilitiesViaService(context.Background(), t.TempDir(), "opaque-harness")
	require.NoError(t, err)
	require.True(t, stub.listHarnessesCalled)
	assert.Equal(t, "opaque-harness", caps.Harness)
	assert.True(t, caps.Available)
	assert.Equal(t, "/fizeau/inventory/opaque-harness", caps.Path)
	assert.Equal(t, "opaque-default", caps.Model)
	assert.Equal(t, []string{"opaque-default"}, caps.Models)
	assert.Equal(t, []string{"odd", "strong"}, caps.ReasoningLevels)
	assert.Equal(t, "local", caps.CostClass)
	assert.True(t, caps.IsLocal)
	assert.True(t, caps.ExactPinSupport)
	assert.True(t, caps.SupportsEffort)
	assert.True(t, caps.SupportsPermissions)
	assert.Empty(t, caps.Binary, "DDx must not reconstruct a concrete binary from local knowledge")
	assert.Empty(t, caps.Surface, "DDx must not reconstruct a routing surface from local knowledge")
}

func TestScriptAndVirtualDispatchThroughFizeauOnly(t *testing.T) {
	for _, harness := range []string{"script", "virtual"} {
		t.Run(harness, func(t *testing.T) {
			stub := &passthroughTestService{}
			SetServiceRunFactory(func(string) (agentlib.FizeauService, error) {
				return stub, nil
			})
			t.Cleanup(func() { SetServiceRunFactory(nil) })

			rcfg := resolvedWithPassthrough(harness, "opaque-provider-value", "opaque-model-value", 0, 0)
			_, err := RunWithConfigViaService(context.Background(), t.TempDir(), rcfg, AgentRunRuntime{
				Prompt: "opaque prompt",
			})
			require.NoError(t, err)
			require.True(t, stub.executeCalled)
			assert.Equal(t, harness, stub.lastReq.Harness)
			assert.Equal(t, "opaque-model-value", stub.lastReq.Model)
			assert.Equal(t, "opaque-provider-value", stub.lastReq.Provider)
		})
	}
}

func TestFizeauExecuteStartCallbackImmediatelyPrecedesExecute(t *testing.T) {
	stub := &passthroughTestService{}
	SetServiceRunFactory(func(string) (agentlib.FizeauService, error) {
		return stub, nil
	})
	t.Cleanup(func() { SetServiceRunFactory(nil) })

	callbackCount := 0
	callbackObservedBeforeExecute := false
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{})
	_, err := RunWithConfigViaService(context.Background(), t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "test exact Execute boundary",
		OnExecuteStart: func() {
			callbackCount++
			callbackObservedBeforeExecute = !stub.executeCalled
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, callbackCount)
	assert.True(t, callbackObservedBeforeExecute, "route-stage callback must fire before Fizeau Execute")
	assert.True(t, stub.executeCalled)
}

// TestRunWithConfigViaService_InstallsProviderShim asserts that a production
// call to RunWithConfigViaService (the entry point exercised by ddx run, ddx
// try, ddx work, and the server) installs the provider PATH shim before
// constructing the Fizeau service. Without this, fizeau's LookPath("codex")
// finds the raw provider binary and spawns it with only Setpgid — no
// Pdeathsig — so a SIGKILL of the worker leaks the provider child as a
// ppid=1 orphan (bead ddx-f2b413ea).
func TestRunWithConfigViaService_InstallsProviderShim(t *testing.T) {
	resetProviderShimStateForTest()
	t.Cleanup(resetProviderShimStateForTest)

	stub := &passthroughTestService{}
	SetServiceRunFactory(func(string) (agentlib.FizeauService, error) {
		return stub, nil
	})
	t.Cleanup(func() { SetServiceRunFactory(nil) })

	fakeDDX := filepath.Join(t.TempDir(), "ddx")
	writeExecutable(t, fakeDDX, "#!/bin/sh\nexit 0\n")
	oldLookup := providerShimExecutableLookup
	providerShimExecutableLookup = func() (string, error) { return fakeDDX, nil }
	t.Cleanup(func() { providerShimExecutableLookup = oldLookup })

	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{
		Model: "haiku",
	}).Resolve(config.CLIOverrides{Harness: "agent", Model: "haiku"})

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

// TestRunWithConfigViaService_UsesProviderShimExecutableResolver proves the
// sole production service entrypoint resolves the ddx executable through the
// shared validator before mutating PATH.
func TestRunWithConfigViaService_UsesProviderShimExecutableResolver(t *testing.T) {
	resetProviderShimStateForTest()
	t.Cleanup(resetProviderShimStateForTest)

	stub := &passthroughTestService{}
	SetServiceRunFactory(func(string) (agentlib.FizeauService, error) {
		return stub, nil
	})
	t.Cleanup(func() { SetServiceRunFactory(nil) })

	fakeDDX := filepath.Join(t.TempDir(), "ddx")
	writeExecutable(t, fakeDDX, "#!/bin/sh\nexit 0\n")
	oldLookup := providerShimExecutableLookup
	providerShimExecutableLookup = func() (string, error) { return fakeDDX, nil }
	t.Cleanup(func() { providerShimExecutableLookup = oldLookup })

	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{})
	_, err := RunWithConfigViaService(context.Background(), t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "test",
	})
	require.NoError(t, err)
	require.True(t, stub.executeCalled, "RunWithConfigViaService must resolve the service through the shared seam")
	require.Contains(t, os.Getenv("PATH"), "ddx-provider-shim-", "RunWithConfigViaService must install a provider shim")
}

// TestAgentPackageSuite_DoesNotExecTestBinaryAsProviderLauncher proves the
// package-level guard never lets the package tests recurse into
// `agent.test __provider-launch` or a real provider binary.
func TestAgentPackageSuite_DoesNotExecTestBinaryAsProviderLauncher(t *testing.T) {
	resetProviderShimStateForTest()
	t.Cleanup(resetProviderShimStateForTest)

	stub := &passthroughTestService{}
	SetServiceRunFactory(func(string) (agentlib.FizeauService, error) {
		return stub, nil
	})
	t.Cleanup(func() { SetServiceRunFactory(nil) })

	fakeProviderDir := t.TempDir()
	sentinel := filepath.Join(t.TempDir(), "provider-leak.marker")
	for _, name := range []string{"codex", "claude", "gemini", "opencode", "pi"} {
		writeExecutable(t, filepath.Join(fakeProviderDir, name), "#!/bin/sh\nprintf %s "+strconv.Quote("unexpected-"+name)+" > "+strconv.Quote(sentinel)+"\nexit 99\n")
	}
	t.Setenv("PATH", fakeProviderDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{
		Model: "haiku",
	}).Resolve(config.CLIOverrides{Harness: "agent", Model: "haiku"})

	_, err := RunWithConfigViaService(context.Background(), t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "test",
	})
	require.NoError(t, err)
	require.True(t, stub.executeCalled, "RunWithConfigViaService should still reach the stub service")

	psOut, err := exec.Command("ps", "-o", "ppid=,pid=,args=", "-ax").CombinedOutput()
	require.NoError(t, err, "ps should be available for the process-tree guard")
	selfPID := strconv.Itoa(os.Getpid())
	for _, line := range strings.Split(string(psOut), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != selfPID {
			continue
		}
		cmdline := strings.Join(fields[2:], " ")
		assert.NotContains(t, cmdline, filepath.Base(os.Args[0])+" __provider-launch")
	}
	_, err = os.Stat(sentinel)
	assert.ErrorIs(t, err, os.ErrNotExist, "no provider binary should have executed")
}

func resetProviderShimStateForTest() {
	providerShimMu.Lock()
	defer providerShimMu.Unlock()
	current := os.Getenv("PATH")
	if providerShimDirPath != "" {
		prefix := providerShimDirPath + string(os.PathListSeparator)
		current = strings.TrimPrefix(current, prefix)
		_ = os.RemoveAll(providerShimDirPath)
		providerShimDirPath = ""
	}
	for {
		entries := strings.Split(current, string(os.PathListSeparator))
		if len(entries) == 0 {
			break
		}
		first := strings.TrimSpace(entries[0])
		if first == "" || !strings.Contains(filepath.Base(first), "ddx-provider-shim-") {
			break
		}
		_ = os.RemoveAll(first)
		current = strings.Join(entries[1:], string(os.PathListSeparator))
	}
	_ = os.Setenv("PATH", current)
}
