package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// ProviderLaunchSubcommand is the hidden ddx subcommand path that wraps a
// provider binary invocation with prctl(PR_SET_PDEATHSIG, SIGKILL) before
// execve'ing into the real binary. This is the local seam that defends
// against orphan codex/claude subprocesses when the worker dies abnormally
// — Fizeau (the external service library) only sets Setpgid on the
// provider Cmd, never Pdeathsig, so without this wrapper a SIGKILL'd
// worker leaves its codex children running under PID 1 indefinitely.
//
// See bead ddx-01b89378 (28 codex orphans, ppid=1, consuming ~90% CPU)
// for the production incident that motivated the wrapper.
const ProviderLaunchSubcommand = "__provider-launch"

// providerShimNames lists the harness binaries whose direct subprocesses
// must be wrapped with parent-death-signal protection. The list mirrors
// fizeau's builtinHarnesses entries that have a non-empty Binary field
// (codex, claude, gemini, opencode, pi). HTTP-only providers (openrouter,
// lmstudio, omlx, …) never spawn a subprocess so they are not shimmed.
var providerShimNames = []string{"codex", "claude", "gemini", "opencode", "pi"}

// BuildProviderLaunchCmd constructs an *exec.Cmd that invokes a provider
// binary through the local pdeathsig+setpgid seam.
//
// The returned Cmd has SysProcAttr populated via cmdSetProcessGroup so the
// wrapper itself becomes a parent-death-signal target. On Linux this means
// the kernel sends SIGKILL to the wrapper when the worker dies. Because
// Pdeathsig is preserved across execve(2) for ordinary binaries (per
// prctl(2)), the death-signal carries over to the provider binary the
// wrapper execves into.
//
// Unlike OSExecutor.ExecuteInDir which does the full streaming+timeout
// dance, this helper only assembles the Cmd — callers (Fizeau, via the
// PATH shim) own start/wait. The function exists primarily so the
// provider-launch contract is unit-testable (see
// TestExecutor_ProviderSpawnSetsPdeathsigAndSetpgid) and so the shim
// scripts have a single canonical construction site.
func BuildProviderLaunchCmd(ctx context.Context, binary string, args ...string) *exec.Cmd {
	var cmd *exec.Cmd
	if ctx == nil {
		cmd = exec.Command(binary, args...)
	} else {
		cmd = exec.CommandContext(ctx, binary, args...)
	}
	cmdSetProcessGroup(cmd)
	return cmd
}

// providerShimDir is the per-process tempdir holding wrapper shims for each
// provider binary. It is created lazily on the first call to
// EnsureProviderShimOnPATH and removed by a cleanup hook the caller
// installs. Guarded by providerShimMu.
var (
	providerShimMu      sync.Mutex
	providerShimDirPath string
)

// EnsureProviderShimOnPATH installs wrapper scripts for each known provider
// binary in a tempdir, prepends that tempdir to the calling process's PATH,
// and returns the shim directory plus a cleanup function. Subsequent calls
// are no-ops that reuse the existing shim dir.
//
// The shim is a tiny shell script that re-execs ddx __provider-launch
// <realBinary> "$@". When fizeau's harness runner LookPaths "codex" /
// "claude" / etc., it now finds the shim, fork+execs it, the shim
// fork+execs ddx __provider-launch, and ddx __provider-launch calls
// prctl(PR_SET_PDEATHSIG, SIGKILL) before execve'ing into the real
// provider binary. The pdeathsig is preserved across the final execve so
// the kernel reaps the provider child when the worker dies.
//
// ddxBinary should be a resolved absolute path to the running ddx
// executable (typically from os.Executable()).
func EnsureProviderShimOnPATH(ddxBinary string) (dir string, cleanup func(), err error) {
	providerShimMu.Lock()
	defer providerShimMu.Unlock()

	if providerShimDirPath != "" {
		return providerShimDirPath, func() {}, nil
	}

	tmpDir, err := os.MkdirTemp("", "ddx-provider-shim-")
	if err != nil {
		return "", func() {}, fmt.Errorf("provider-shim: mkdir: %w", err)
	}

	// Resolve each real provider binary by consulting the current PATH
	// (before we mutate it). Providers that are not on PATH are simply
	// skipped — if fizeau later tries to use them it will fail the same
	// way it would have without the shim.
	realPATH := os.Getenv("PATH")
	for _, name := range providerShimNames {
		realPath, lookErr := exec.LookPath(name)
		if lookErr != nil {
			continue
		}
		shimPath := filepath.Join(tmpDir, name)
		shim := fmt.Sprintf("#!/bin/sh\nexec %q %s %q \"$@\"\n", ddxBinary, ProviderLaunchSubcommand, realPath)
		if writeErr := os.WriteFile(shimPath, []byte(shim), 0o755); writeErr != nil {
			_ = os.RemoveAll(tmpDir)
			return "", func() {}, fmt.Errorf("provider-shim: write %s: %w", name, writeErr)
		}
	}

	newPATH := tmpDir + string(os.PathListSeparator) + realPATH
	if setErr := os.Setenv("PATH", newPATH); setErr != nil {
		_ = os.RemoveAll(tmpDir)
		return "", func() {}, fmt.Errorf("provider-shim: setenv PATH: %w", setErr)
	}

	providerShimDirPath = tmpDir
	cleanup = func() {
		providerShimMu.Lock()
		defer providerShimMu.Unlock()
		if providerShimDirPath == "" {
			return
		}
		// Restore PATH by stripping our prefix; leave the rest alone in
		// case other code further mutated PATH after us.
		current := os.Getenv("PATH")
		prefix := providerShimDirPath + string(os.PathListSeparator)
		if strings.HasPrefix(current, prefix) {
			_ = os.Setenv("PATH", strings.TrimPrefix(current, prefix))
		}
		_ = os.RemoveAll(providerShimDirPath)
		providerShimDirPath = ""
	}
	return tmpDir, cleanup, nil
}
