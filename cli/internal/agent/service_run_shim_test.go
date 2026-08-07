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

// TestRunWithConfigViaService_DoesNotMutatePATH proves the production service
// dispatch path reaches Fizeau without mutating PATH first.
func TestRunWithConfigViaService_DoesNotMutatePATH(t *testing.T) {
	stub := &passthroughTestService{}
	SetServiceRunFactory(func(string) (agentlib.FizeauService, error) {
		return stub, nil
	})
	t.Cleanup(func() { SetServiceRunFactory(nil) })

	initialPATH := os.Getenv("PATH")
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{Model: "haiku"}).Resolve(config.CLIOverrides{Harness: "agent", Model: "haiku"})
	_, err := RunWithConfigViaService(context.Background(), t.TempDir(), rcfg, AgentRunRuntime{Prompt: "test"})
	require.NoError(t, err)
	require.True(t, stub.executeCalled, "RunWithConfigViaService should still reach the stub service")
	require.Equal(t, initialPATH, os.Getenv("PATH"))
}

// TestAgentPackageSuite_DoesNotExecTestBinaryAsProviderLauncher proves the
// package-level guard never lets the package tests recurse into a real
// provider binary.
func TestAgentPackageSuite_DoesNotExecTestBinaryAsProviderLauncher(t *testing.T) {
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
