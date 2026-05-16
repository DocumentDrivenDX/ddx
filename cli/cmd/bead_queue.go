package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (f *CommandFactory) newBeadQueueCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Manage in-priority queue ordering",
		Long: `Move beads within their existing priority bucket without changing urgency.

Queue ranks are stored as preserved metadata and only affect ordering inside
one priority value. Use queue commands when you want to change sequence, and
use ` + "`ddx bead update --priority`" + ` when you want to change urgency.`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(f.newBeadQueueTopCommand())
	cmd.AddCommand(f.newBeadQueueMoveCommand())
	cmd.AddCommand(f.newBeadQueueClearCommand())

	return cmd
}

func (f *CommandFactory) newBeadQueueTopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "top <id>",
		Short: "Move a bead to the front of its priority bucket",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return f.withBeadTrackerWriteLock(func() error {
				if err := f.beadStore().QueueTop(args[0]); err != nil {
					return err
				}
				_, err := f.beadAutoCommit("queue top " + args[0])
				return err
			})
		},
	}
}

func (f *CommandFactory) newBeadQueueMoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "move <id>",
		Short: "Move a bead before or after another bead in the same priority bucket",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			before, _ := cmd.Flags().GetString("before")
			after, _ := cmd.Flags().GetString("after")
			if before == "" && after == "" {
				return fmt.Errorf("must specify exactly one of --before or --after")
			}
			if before != "" && after != "" {
				return fmt.Errorf("cannot specify both --before and --after")
			}
			return f.withBeadTrackerWriteLock(func() error {
				if before != "" {
					if err := f.beadStore().QueueMove(args[0], before, true); err != nil {
						return err
					}
				} else {
					if err := f.beadStore().QueueMove(args[0], after, false); err != nil {
						return err
					}
				}
				_, err := f.beadAutoCommit("queue move " + args[0])
				return err
			})
		},
	}
	cmd.Flags().String("before", "", "Place the bead before another bead in the same priority bucket")
	cmd.Flags().String("after", "", "Place the bead after another bead in the same priority bucket")
	return cmd
}

func (f *CommandFactory) newBeadQueueClearCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "clear <id>",
		Short: "Remove the explicit queue rank from a bead",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return f.withBeadTrackerWriteLock(func() error {
				if err := f.beadStore().QueueClear(args[0]); err != nil {
					return err
				}
				_, err := f.beadAutoCommit("queue clear " + args[0])
				return err
			})
		},
	}
}
