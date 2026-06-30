package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/spf13/cobra"
)

// newProviderLaunchCommand returns the hidden `ddx __provider-launch` command.
//
// The provider-launch subcommand wraps a real provider binary invocation
// (codex/claude/gemini/opencode/pi) so the wrapper process becomes a
// kernel parent-death-signal target before handing control off via
// syscall.Exec. Pdeathsig is preserved across execve(2) for ordinary
// (non-setuid/non-capability) binaries (per prctl(2)), so the death
// signal carries over into the provider binary itself — when the worker
// dies abnormally (SIGKILL/OOM/crash) the kernel reaps the provider
// child instead of letting it orphan to PID 1.
//
// The command is hidden because it is an implementation detail of the
// provider shim — operators should never invoke it directly. See
// internal/agent/provider_spawn.go (EnsureProviderShimOnPATH) for the
// shim that wires it into Fizeau's PATH-based binary lookup, and bead
// ddx-01b89378 for the production incident that motivated the seam.
func (f *CommandFactory) newProviderLaunchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                   agent.ProviderLaunchSubcommand + " [binary] [args...]",
		Short:                 "Internal: provider-launch wrapper (sets PR_SET_PDEATHSIG before execve)",
		Long:                  "Internal subcommand used by ddx to wrap codex/claude/gemini provider binaries with kernel parent-death protection. Not for direct operator use.",
		Hidden:                true,
		DisableFlagParsing:    true,
		DisableSuggestions:    true,
		SilenceUsage:          true,
		SilenceErrors:         true,
		DisableFlagsInUseLine: true,
		// Bypass the root command's PersistentPreRunE/PostRunE chain
		// (config init, version gate, update-check, staleness hints) —
		// this is a hot-path exec wrapper called on every provider
		// invocation. Adding any of that overhead would punish every
		// codex/claude spawn for no operator-facing benefit.
		PersistentPreRunE:  func(cmd *cobra.Command, args []string) error { return nil },
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New(agent.ProviderLaunchSubcommand + ": missing binary argument")
			}
			return runProviderLaunch(args[0], args[1:])
		},
	}
	return cmd
}

// runProviderLaunch is the platform-specific entrypoint. On Linux it
// invokes prctl(PR_SET_PDEATHSIG, SIGKILL) and then syscall.Exec's into
// the real binary. On non-Linux platforms it simply execs (or
// falls back to running the binary) — the orphan reaper plus
// cmdKillProcessGroup serve as the fallback there.
//
// On error, it returns from the cobra RunE. On success it does NOT
// return — execve replaces the process image.
func runProviderLaunch(binary string, args []string) error {
	if err := providerLaunchPrepare(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s: prctl: %v\n", agent.ProviderLaunchSubcommand, err)
		// Continue anyway — failing to set pdeathsig is not worse than
		// the pre-shim state, and refusing to exec would break work.
	}
	return providerLaunchExec(binary, args)
}
