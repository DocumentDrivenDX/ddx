package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
	"github.com/DocumentDrivenDX/ddx/internal/serverreg"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newArtifactCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "artifact",
		Aliases: []string{"artifacts"},
		Short:   "Inspect and regenerate DDx artifacts",
		Long: `Inspect and regenerate graph artifacts.

Regeneration is an explicit wrapper around the FEAT-010 execution substrate:
DDx selects a generator definition for the artifact, records the run, and marks
the record with produces_artifact. DDx does not synthesize manual fallback
prompts when an artifact has no generator provenance.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			serverreg.TryRegisterAsync(f.WorkingDir)
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(f.newArtifactRegenerateCommand())
	return cmd
}

func (f *CommandFactory) newArtifactRegenerateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "regenerate <artifact-id>",
		Short: "Regenerate one artifact through its execution definition",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			artifactID := strings.TrimSpace(args[0])
			definitionID, _ := cmd.Flags().GetString("definition")

			graph, err := f.buildDocGraph()
			if err != nil {
				return err
			}
			if _, ok := graph.Show(artifactID); !ok {
				return fmt.Errorf("artifact %q not found in document graph", artifactID)
			}

			store := f.execStore()
			defID, err := selectArtifactGenerator(store, artifactID, definitionID)
			if err != nil {
				return err
			}

			rec, err := store.RunWithOptions(cmd.Context(), defID, ddxexec.RunOptions{
				ProducesArtifact: artifactID,
			})
			if err != nil {
				return err
			}

			if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rec)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %d\n", rec.RunID, rec.Status, rec.ExitCode)
			return nil
		},
	}
	cmd.Flags().String("definition", "", "Execution definition ID to use when multiple generators exist")
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func selectArtifactGenerator(store *ddxexec.Store, artifactID, explicitDefinitionID string) (string, error) {
	if explicitDefinitionID != "" {
		def, err := store.ShowDefinition(explicitDefinitionID)
		if err != nil {
			return "", err
		}
		if !stringSliceContains(def.ArtifactIDs, artifactID) {
			return "", fmt.Errorf("exec definition %q does not produce artifact %q", explicitDefinitionID, artifactID)
		}
		return def.ID, nil
	}

	defs, err := store.ListDefinitions(artifactID)
	if err != nil {
		return "", err
	}
	switch len(defs) {
	case 0:
		return "", fmt.Errorf("artifact %q has no generator definition or generated_by provenance; cannot regenerate", artifactID)
	case 1:
		return defs[0].ID, nil
	default:
		ids := make([]string, 0, len(defs))
		for _, def := range defs {
			ids = append(ids, def.ID)
		}
		return "", fmt.Errorf("artifact %q has multiple generator definitions (%s); pass --definition", artifactID, strings.Join(ids, ", "))
	}
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
