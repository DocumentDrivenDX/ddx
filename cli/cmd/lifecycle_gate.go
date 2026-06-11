package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) checkLifecycleMigrationGate(cmd *cobra.Command) error {
	if shouldBypassLifecycleMigrationGate(cmd) {
		return nil
	}

	s := f.beadStore()
	mig, err := bead.NewMigrator(bead.MigratorOptions{Dir: s.Dir})
	if err != nil {
		return err
	}
	status, err := mig.DetectLifecycleMigrationRequired(context.Background())
	if err != nil {
		return err
	}
	if !status.Required() {
		return nil
	}

	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	if root := cmd.Root(); root != nil {
		root.SilenceUsage = true
		root.SilenceErrors = true
	}

	if commandWantsJSON(cmd) {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(status); err != nil {
			return err
		}
		return NewExitError(ExitCodeGeneralError, "")
	}

	return fmt.Errorf("%s: %w\nrun `ddx bead migrate --lifecycle --dry-run` to inspect; run `ddx bead migrate --lifecycle --apply` to migrate", status.Code, status.Err())
}

func shouldBypassLifecycleMigrationGate(cmd *cobra.Command) bool {
	if cmd == nil {
		return true
	}
	if cmd.Name() == "help" {
		return true
	}

	path := strings.Fields(cmd.CommandPath())
	if len(path) == 0 {
		return true
	}
	if len(path) == 1 {
		return true
	}

	switch path[1] {
	case "version", "doctor", "completion":
		return true
	}

	if len(path) >= 3 && path[1] == "agent" && path[2] == "doctor" {
		return true
	}
	if len(path) >= 3 && path[1] == "bead" && path[2] == "doctor" {
		return true
	}
	if len(path) >= 3 && path[1] == "bead" && path[2] == "needs-human" {
		return true
	}
	if len(path) >= 3 && path[1] == "bead" && path[2] == "status" {
		return true
	}
	if len(path) >= 3 && path[1] == "bead" && path[2] == "reconcile" {
		apply, _ := cmd.Flags().GetBool("apply")
		return !apply
	}
	if len(path) >= 3 && path[1] == "bead" && path[2] == "migrate" {
		lifecycle, _ := cmd.Flags().GetBool("lifecycle")
		return lifecycle
	}

	return false
}

func commandWantsJSON(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.Flags().Lookup("json")
	if flag == nil {
		return false
	}
	value, err := cmd.Flags().GetBool("json")
	return err == nil && value
}
