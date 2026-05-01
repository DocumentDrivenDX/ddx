package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newAgentExecutionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "executions",
		Short: "Manage execute-bead execution bundles (.ddx/executions/)",
		Long: `Inspect and retrieve execute-bead execution bundles.

Bundles are written by execute-bead under .ddx/executions/<attempt-id>/. When
an out-of-band mirror is configured under executions.mirror in .ddx/config.yaml,
each finalized bundle is uploaded to the mirror and indexed in
.ddx/executions/mirror-index.jsonl.

Use 'ddx agent executions fetch <attempt-id>' to pull a previously mirrored
bundle back to local disk.`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.AddCommand(f.newAgentExecutionsFetchCommand())
	return cmd
}

func (f *CommandFactory) newAgentExecutionsFetchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch <attempt-id>",
		Short: "Pull a mirrored execution bundle back to .ddx/executions/<attempt-id>/",
		Long: `fetch retrieves a previously mirrored bundle from the configured execution
mirror and writes it back to .ddx/executions/<attempt-id>/ for local inspection.

The mirror entry is resolved from .ddx/executions/mirror-index.jsonl. The
bundle is rehydrated under the project's .ddx/executions/<attempt-id>/ path
unless --dest is given.`,
		Args: cobra.ExactArgs(1),
		RunE: f.runAgentExecutionsFetch,
	}
	cmd.Flags().String("project", "", "Project root (default: CWD git root). Env: DDX_PROJECT_ROOT")
	cmd.Flags().String("dest", "", "Destination directory (default: .ddx/executions/<attempt-id>/)")
	cmd.Flags().Bool("json", false, "Output result as JSON")
	return cmd
}

func (f *CommandFactory) runAgentExecutionsFetch(cmd *cobra.Command, args []string) error {
	attemptID := args[0]
	if !validBeadID.MatchString(attemptID) {
		return fmt.Errorf("invalid attempt id %q", attemptID)
	}
	projectFlag, _ := cmd.Flags().GetString("project")
	dest, _ := cmd.Flags().GetString("dest")
	asJSON, _ := cmd.Flags().GetBool("json")

	projectRoot := resolveProjectRoot(projectFlag, f.WorkingDir)

	entry, err := agent.LookupMirrorEntry(projectRoot, attemptID)
	if err != nil {
		return fmt.Errorf("reading mirror index: %w", err)
	}
	if entry == nil {
		return fmt.Errorf("no mirror entry for attempt %s in %s", attemptID, agent.ExecutionsMirrorIndexFile)
	}

	backend, err := agent.NewMirrorBackend(entry.Kind)
	if err != nil {
		return err
	}

	if dest == "" {
		dest = filepath.Join(projectRoot, agent.ExecuteBeadArtifactDir, attemptID)
	}

	if err := backend.Fetch(entry.MirrorURI, dest); err != nil {
		return fmt.Errorf("fetching bundle: %w", err)
	}

	if asJSON {
		out := struct {
			AttemptID string `json:"attempt_id"`
			BeadID    string `json:"bead_id,omitempty"`
			MirrorURI string `json:"mirror_uri"`
			Dest      string `json:"dest"`
			ByteSize  int64  `json:"byte_size"`
		}{
			AttemptID: entry.AttemptID,
			BeadID:    entry.BeadID,
			MirrorURI: entry.MirrorURI,
			Dest:      dest,
			ByteSize:  entry.ByteSize,
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "attempt: %s\n", entry.AttemptID)
	if entry.BeadID != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "bead:    %s\n", entry.BeadID)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "mirror:  %s\n", entry.MirrorURI)
	fmt.Fprintf(cmd.OutOrStdout(), "dest:    %s\n", dest)
	fmt.Fprintf(cmd.OutOrStdout(), "bytes:   %d\n", entry.ByteSize)
	return nil
}
