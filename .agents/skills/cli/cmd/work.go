package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// newWorkCommand creates the top-level "ddx work" command as a UX alias for
// "ddx agent execute-loop". All flags and behavior are identical.
func (f *CommandFactory) newWorkCommand() *cobra.Command {
	inner := f.newAgentExecuteLoopCommand()

	cmd := &cobra.Command{
		Use:   "work",
		Short: "Work the bead execution queue",
		Long: `work is the primary operator-facing surface for draining the bead
execution queue. It is an alias for "ddx agent execute-loop" — all flags
and behavior are identical.

` + inner.Long,
		Example: `  # Drain the current execution-ready queue once and exit
  ddx work

  # Pick one ready bead, execute it, and stop
  ddx work --profile default --once

  # Run continuously as a bounded queue worker
  ddx work --poll-interval 30s

  # Force a specific harness/model for a debugging pass
  ddx work --once --harness agent --model minimax/minimax-m2.7

  # Run inline in the current process
  ddx work --local --once`,
		Args: inner.Args,
		RunE: inner.RunE,
	}

	// Clone all flags from the execute-loop command
	inner.Flags().VisitAll(func(flag *pflag.Flag) {
		cmd.Flags().AddFlag(flag)
	})

	return cmd
}
