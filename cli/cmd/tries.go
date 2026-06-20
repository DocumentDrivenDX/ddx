package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/spf13/cobra"
)

// worktreeRecord is a layer-2 worktree attempt record derived from result.json.
type worktreeRecord struct {
	AttemptID    string    `json:"attempt_id,omitempty"`
	BeadID       string    `json:"bead_id,omitempty"`
	WorktreePath string    `json:"worktree_path,omitempty"`
	StartedAt    time.Time `json:"started_at,omitempty"`
	FinishedAt   time.Time `json:"finished_at,omitempty"`
	Outcome      string    `json:"outcome,omitempty"`
	Status       string    `json:"status,omitempty"`
	PreserveRef  string    `json:"preserve_ref,omitempty"`
	BaseRev      string    `json:"base_rev,omitempty"`
	ResultRev    string    `json:"result_rev,omitempty"`
}

func (f *CommandFactory) newTriesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tries",
		Short: "Layer-2 worktree records (start/end, merge/preserve outcomes)",
		Long: `List layer-2 worktree records from executions written by 'ddx try'.

Reads .ddx/executions/*/result.json and surfaces worktree start/end events
and merge-or-preserve outcomes. No new persistence is added.

See FEAT-001 §21b.`,
		Args: cobra.NoArgs,
		RunE: f.runTriesList,
	}

	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

// scanWorktreeRecords reads .ddx/executions/*/result.json under workingDir and
// returns records where worktree_path is non-empty (layer-2 worktree attempts).
func scanWorktreeRecords(workingDir string) ([]worktreeRecord, error) {
	execRoot := filepath.Join(resolveBeadStoreRoot(workingDir), "executions")
	entries, err := os.ReadDir(execRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read executions dir: %w", err)
	}

	var records []worktreeRecord
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		resultPath := filepath.Join(execRoot, entry.Name(), "result.json")
		raw, err := os.ReadFile(resultPath)
		if err != nil {
			continue
		}
		var res agent.ExecuteBeadResult
		if err := json.Unmarshal(raw, &res); err != nil {
			continue
		}
		if res.WorktreePath == "" {
			continue
		}
		records = append(records, worktreeRecord{
			AttemptID:    res.AttemptID,
			BeadID:       res.BeadID,
			WorktreePath: res.WorktreePath,
			StartedAt:    res.StartedAt,
			FinishedAt:   res.FinishedAt,
			Outcome:      res.Outcome,
			Status:       res.Status,
			PreserveRef:  res.PreserveRef,
			BaseRev:      res.BaseRev,
			ResultRev:    res.ResultRev,
		})
	}

	sort.Slice(records, func(i, j int) bool {
		if records[i].StartedAt.IsZero() && records[j].StartedAt.IsZero() {
			return records[i].AttemptID < records[j].AttemptID
		}
		if records[i].StartedAt.IsZero() {
			return false
		}
		if records[j].StartedAt.IsZero() {
			return true
		}
		return records[i].StartedAt.Before(records[j].StartedAt)
	})
	return records, nil
}

func (f *CommandFactory) runTriesList(cmd *cobra.Command, _ []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")

	workspaceRoot := f.beadWorkspaceRoot()
	if workspaceRoot == "" {
		workspaceRoot = f.WorkingDir
	}

	records, err := scanWorktreeRecords(workspaceRoot)
	if err != nil {
		return err
	}

	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if records == nil {
			records = []worktreeRecord{}
		}
		return enc.Encode(records)
	}

	if len(records) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no layer-2 worktree records found")
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%-24s  %-20s  %-24s  %-24s  %-12s  %s\n",
		"ATTEMPT_ID", "BEAD_ID", "STARTED_AT", "FINISHED_AT", "OUTCOME", "STATUS")
	for _, r := range records {
		startedAt := ""
		if !r.StartedAt.IsZero() {
			startedAt = r.StartedAt.UTC().Format(time.RFC3339)
		}
		finishedAt := ""
		if !r.FinishedAt.IsZero() {
			finishedAt = r.FinishedAt.UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%-24s  %-20s  %-24s  %-24s  %-12s  %s\n",
			r.AttemptID, r.BeadID, startedAt, finishedAt, r.Outcome, r.Status)
	}
	return nil
}
