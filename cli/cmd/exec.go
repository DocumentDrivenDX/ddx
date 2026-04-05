package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newExecCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec",
		Short: "Validate and run DDx execution definitions",
		Long:  "Manage DDx execution definitions and immutable run history.",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(f.newExecDefineCommand())
	cmd.AddCommand(f.newExecListCommand())
	cmd.AddCommand(f.newExecShowCommand())
	cmd.AddCommand(f.newExecValidateCommand())
	cmd.AddCommand(f.newExecRunCommand())
	cmd.AddCommand(f.newExecLogCommand())
	cmd.AddCommand(f.newExecResultCommand())
	cmd.AddCommand(f.newExecHistoryCommand())
	return cmd
}

func (f *CommandFactory) newExecDefineCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "define <name>",
		Short: "Create an execution definition",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			artifactID, _ := cmd.Flags().GetString("artifact")
			command, _ := cmd.Flags().GetString("command")
			execType, _ := cmd.Flags().GetString("type")

			if artifactID == "" {
				return fmt.Errorf("--artifact is required")
			}
			if command == "" {
				return fmt.Errorf("--command is required")
			}
			if execType == "" {
				execType = "check"
			}

			def := ddxexec.Definition{
				ID:          args[0],
				ArtifactIDs: []string{artifactID},
				Executor: ddxexec.ExecutorSpec{
					Kind:    ddxexec.ExecutorKindCommand,
					Command: strings.Fields(command),
				},
				Active:    true,
				CreatedAt: time.Now().UTC(),
			}
			_ = execType

			store := f.execStore()
			if err := store.SaveDefinition(def); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), def.ID)
			return nil
		},
	}
	cmd.Flags().String("artifact", "", "Artifact ID to associate with this definition")
	cmd.Flags().String("command", "", "Command to execute")
	cmd.Flags().String("type", "", "Execution type: check, metric, or build (default: check)")
	return cmd
}

func (f *CommandFactory) execStore() *ddxexec.Store {
	store := ddxexec.NewStore(f.WorkingDir)
	if f.AgentRunnerOverride != nil {
		store.AgentRunner = f.AgentRunnerOverride
	} else {
		store.AgentRunner = f.agentRunner()
	}
	return store
}

func (f *CommandFactory) newExecListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List execution definitions",
		RunE: func(cmd *cobra.Command, args []string) error {
			artifactID, _ := cmd.Flags().GetString("artifact")
			defs, err := f.execStore().ListDefinitions(artifactID)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("json") {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(defs)
			}
			for _, def := range defs {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s\n", def.ID, strings.Join(def.ArtifactIDs, ","))
			}
			return nil
		},
	}
	cmd.Flags().String("artifact", "", "Filter by artifact ID")
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) newExecShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <definition-id>",
		Short: "Show one execution definition",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			def, err := f.execStore().ShowDefinition(args[0])
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("json") {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(def)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "ID:       %s\n", def.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "Artifacts:%s\n", strings.Join(def.ArtifactIDs, ", "))
			fmt.Fprintf(cmd.OutOrStdout(), "Kind:     %s\n", def.Executor.Kind)
			fmt.Fprintf(cmd.OutOrStdout(), "Created:  %s\n", def.CreatedAt.Format(time.RFC3339))
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) newExecValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <definition-id>",
		Short: "Validate a definition and linked artifacts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			def, doc, err := f.execStore().Validate(args[0])
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("json") {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"definition_id": def.ID,
					"artifact_id":   doc.ID,
					"status":        "ok",
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s validated with %s\n", def.ID, doc.ID)
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) newExecRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <definition-id>",
		Short: "Execute a definition and persist one run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rec, err := f.execStore().Run(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("json") {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rec)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %d\n", rec.RunID, rec.Status, rec.ExitCode)
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) newExecLogCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log <run-id>",
		Short: "Show raw logs for one run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			stdout, stderr, err := f.execStore().Log(args[0])
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("json") {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]string{"stdout": stdout, "stderr": stderr})
			}
			if stdout != "" {
				fmt.Fprintln(cmd.OutOrStdout(), stdout)
			}
			if stderr != "" {
				fmt.Fprintln(cmd.OutOrStdout(), stderr)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) newExecResultCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "result <run-id>",
		Short: "Show structured result data for one run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := f.execStore().Result(args[0])
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		},
	}
	return cmd
}

func (f *CommandFactory) newExecHistoryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Inspect historical runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			artifactID, _ := cmd.Flags().GetString("artifact")
			definitionID, _ := cmd.Flags().GetString("definition")
			records, err := f.execStore().History(artifactID, definitionID)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("json") {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(records)
			}
			for _, rec := range records {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s  %d\n", rec.RunID, rec.DefinitionID, rec.Status, rec.ExitCode)
			}
			return nil
		},
	}
	cmd.Flags().String("artifact", "", "Filter by artifact ID")
	cmd.Flags().String("definition", "", "Filter by definition ID")
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}
