package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/spf13/cobra"
)

// agentWorkerCurrentAttempt mirrors server.CurrentAttemptInfo for JSON decoding.
type agentWorkerCurrentAttempt struct {
	AttemptID string    `json:"attempt_id"`
	BeadID    string    `json:"bead_id"`
	BeadTitle string    `json:"bead_title,omitempty"`
	Harness   string    `json:"harness,omitempty"`
	Model     string    `json:"model,omitempty"`
	Phase     string    `json:"phase"`
	StartedAt time.Time `json:"started_at"`
	ElapsedMS int64     `json:"elapsed_ms"`
}

// agentWorkerServerRecord mirrors server.WorkerRecord for JSON decoding.
type agentWorkerServerRecord struct {
	ID             string                     `json:"id"`
	Kind           string                     `json:"kind"`
	State          string                     `json:"state"`
	ProjectRoot    string                     `json:"project_root"`
	Harness        string                     `json:"harness,omitempty"`
	Model          string                     `json:"model,omitempty"`
	StartedAt      time.Time                  `json:"started_at,omitempty"`
	Attempts       int                        `json:"attempts,omitempty"`
	Successes      int                        `json:"successes,omitempty"`
	Failures       int                        `json:"failures,omitempty"`
	CurrentBead    string                     `json:"current_bead,omitempty"`
	CurrentAttempt *agentWorkerCurrentAttempt `json:"current_attempt,omitempty"`
	LastResult     *struct {
		BeadID string `json:"bead_id,omitempty"`
	} `json:"last_result,omitempty"`
	PID      int   `json:"pid,omitempty"`
	PIDAlive *bool `json:"pid_alive,omitempty"`
}

// agentWorkerDisplay is the unified display record for table output and JSON.
type agentWorkerDisplay struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	State     string    `json:"state"`
	BeadID    string    `json:"bead_id,omitempty"`
	BeadTitle string    `json:"bead_title,omitempty"`
	Harness   string    `json:"harness,omitempty"`
	Model     string    `json:"model,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	ElapsedMS int64     `json:"elapsed_ms,omitempty"`
	Attempts  int       `json:"attempts,omitempty"`
	Successes int       `json:"successes,omitempty"`
	Failures  int       `json:"failures,omitempty"`
	// PID is the operating-system process id of the worker, when a
	// subprocess is registered. Surfaced so external tooling can target
	// the process directly (e.g. `kill -TERM <pid>`) when the CLI stop
	// path is unavailable.
	PID int `json:"pid,omitempty"`
	// PIDAlive is true when PID > 0 and the process is alive, false when PID
	// is known dead. Nil when PID == 0 (goroutine-only worker).
	PIDAlive *bool `json:"pid_alive,omitempty"`
}

func (f *CommandFactory) newAgentWorkersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workers",
		Short: "Show running agent workers and their current bead",
		Long: `Show all running agent workers and the bead each is currently executing.

Server-spawned workers are fetched from the running ddx server. Inline
workers (started by running 'ddx work' / 'ddx agent execute-loop' directly)
are detected by scanning active execute-bead worktrees in the project.

Examples:
  ddx agent workers
  ddx agent workers --json
  ddx agent workers --watch
  ddx agent workers --project /path/to/project`,
		Args: cobra.NoArgs,
		RunE: f.runAgentWorkers,
	}
	cmd.Flags().Bool("json", false, "Emit raw JSON array")
	cmd.Flags().Bool("watch", false, "Re-render every 2s until Ctrl-C")
	cmd.Flags().String("project", "", "Project root to query (default: detected from CWD)")

	cmd.AddCommand(f.newAgentWorkersStopCommand())
	cmd.AddCommand(f.newAgentWorkersPruneCommand())
	return cmd
}

// newAgentWorkersStopCommand wires the `ddx agent workers stop` subcommand.
// Targeting modes are mutually exclusive:
//
//	ddx agent workers stop <worker-id>     — one worker by id
//	ddx agent workers stop --all-over <d>  — every running worker older than <d>
//	ddx agent workers stop --state <state> — every worker in <state>
//	ddx agent workers stop --bead <id>     — the worker assigned to <bead-id>
//
// Each match POSTs /api/agent/workers/{id}/stop on the running ddx server,
// which triggers the graceful SIGTERM → grace → SIGKILL path in
// WorkerManager.Stop. Returns a non-zero exit code if any target fails.
func (f *CommandFactory) newAgentWorkersStopCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop [worker-id]",
		Short: "Gracefully stop running agent workers",
		Long: `Gracefully terminate one or more running agent workers.

The server sends SIGTERM to the worker's process group, waits for the configured
grace window, and escalates to SIGKILL if the leader is still alive. The worker's
bead claim is released and a bead.stopped event is appended to the tracker before
the kill, so claims are not leaked even when the full grace elapses.

Examples:
  ddx agent workers stop worker-20260418T100000-abcd
  ddx agent workers stop --all-over 1h
  ddx agent workers stop --state running
  ddx agent workers stop --bead ddx-abc12345`,
		RunE: f.runAgentWorkersStop,
	}
	cmd.Flags().Duration("all-over", 0, "Stop every running worker older than this duration")
	cmd.Flags().String("state", "", "Stop every worker in the given state (e.g. running)")
	cmd.Flags().String("bead", "", "Stop the worker assigned to the given bead id")
	cmd.Flags().String("project", "", "Project root to query (default: detected from CWD)")
	cmd.Flags().Bool("json", false, "Emit one JSON object per worker acted on")
	return cmd
}

func (f *CommandFactory) runAgentWorkersStop(cmd *cobra.Command, args []string) error {
	allOver, _ := cmd.Flags().GetDuration("all-over")
	stateFilter, _ := cmd.Flags().GetString("state")
	beadFilter, _ := cmd.Flags().GetString("bead")
	projectFlag, _ := cmd.Flags().GetString("project")
	asJSON, _ := cmd.Flags().GetBool("json")

	// Enforce that the operator picks exactly one targeting mode.
	modes := 0
	if len(args) > 0 {
		modes++
	}
	if allOver > 0 {
		modes++
	}
	if stateFilter != "" {
		modes++
	}
	if beadFilter != "" {
		modes++
	}
	if modes == 0 {
		return fmt.Errorf("specify a worker id or one of --all-over, --state, --bead")
	}
	if modes > 1 {
		return fmt.Errorf("specify exactly one of: <worker-id>, --all-over, --state, --bead")
	}

	projectRoot := projectFlag
	if projectRoot == "" {
		projectRoot = gitpkg.FindProjectRoot(f.WorkingDir)
	}

	var targets []string
	if len(args) > 0 {
		targets = []string{args[0]}
	} else {
		workers := collectAgentWorkers(projectRoot)
		now := time.Now()
		for _, wk := range workers {
			if wk.Kind == "local" {
				// Local (execute-bead) workers are not reachable through the
				// server stop endpoint. Skip them here — operators must stop
				// them via the worktree's own lifecycle.
				continue
			}
			if allOver > 0 {
				if wk.State != "running" || wk.StartedAt.IsZero() {
					continue
				}
				if now.Sub(wk.StartedAt) <= allOver {
					continue
				}
			}
			if stateFilter != "" && wk.State != stateFilter {
				continue
			}
			if beadFilter != "" && wk.BeadID != beadFilter {
				continue
			}
			targets = append(targets, wk.ID)
		}
	}

	if len(targets) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no matching workers")
		return nil
	}

	base := resolveServerURL(projectRoot)
	client := newLocalServerClient()

	var firstErr error
	for _, id := range targets {
		reqURL := base + "/api/agent/workers/" + id + "/stop"
		req, err := http.NewRequestWithContext(cmd.Context(), http.MethodPost, reqURL, nil)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "stop %s: %v\n", id, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			msg := strings.TrimSpace(string(body))
			fmt.Fprintf(cmd.ErrOrStderr(), "stop %s: server error (%d): %s\n", id, resp.StatusCode, msg)
			if firstErr == nil {
				firstErr = fmt.Errorf("server error (%d) for %s", resp.StatusCode, id)
			}
			continue
		}
		if asJSON {
			fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(string(body)))
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "stopping %s\n", id)
		}
	}
	return firstErr
}

func (f *CommandFactory) runAgentWorkers(cmd *cobra.Command, _ []string) error {
	asJSON, _ := cmd.Flags().GetBool("json")
	watch, _ := cmd.Flags().GetBool("watch")
	projectFlag, _ := cmd.Flags().GetString("project")

	projectRoot := projectFlag
	if projectRoot == "" {
		projectRoot = gitpkg.FindProjectRoot(f.WorkingDir)
	}

	render := func(out io.Writer) error {
		workers := collectAgentWorkers(projectRoot)
		if asJSON {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(workers)
		}
		return printAgentWorkersTable(out, workers)
	}

	if !watch {
		return render(cmd.OutOrStdout())
	}

	ctx := cmd.Context()
	for {
		// Clear screen and move cursor to top-left
		fmt.Fprint(cmd.OutOrStdout(), "\033[2J\033[H")
		fmt.Fprintf(cmd.OutOrStdout(), "Workers — %s  (Ctrl-C to stop)\n\n",
			time.Now().Format("15:04:05"))
		if err := render(cmd.OutOrStdout()); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(2 * time.Second):
		}
	}
}

// collectAgentWorkers fetches workers from the server and local manifest scan,
// merges and deduplicates them, and returns the unified list.
func collectAgentWorkers(projectRoot string) []agentWorkerDisplay {
	serverWorkers, serverAttemptIDs := fetchServerAgentWorkers(projectRoot)
	localWorkers := scanLocalBeadWorkers(projectRoot, serverAttemptIDs)

	result := make([]agentWorkerDisplay, 0, len(serverWorkers)+len(localWorkers))
	result = append(result, serverWorkers...)
	result = append(result, localWorkers...)
	return result
}

// fetchServerAgentWorkers calls GET /api/agent/workers and converts the
// response to display records. Returns a set of attempt IDs already covered
// by the server so local scanning can skip them.
func fetchServerAgentWorkers(projectRoot string) ([]agentWorkerDisplay, map[string]bool) {
	base := resolveServerURL(projectRoot)
	client := newLocalServerClient()
	resp, err := client.Get(base + "/api/agent/workers")
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close() //nolint:errcheck

	var records []agentWorkerServerRecord
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
		return nil, nil
	}

	out := make([]agentWorkerDisplay, 0, len(records))
	attemptIDs := make(map[string]bool)

	for _, r := range records {
		d := agentWorkerDisplay{
			ID:        r.ID,
			Kind:      r.Kind,
			State:     r.State,
			Harness:   r.Harness,
			Model:     r.Model,
			StartedAt: r.StartedAt,
			Attempts:  r.Attempts,
			Successes: r.Successes,
			Failures:  r.Failures,
			PID:       r.PID,
			PIDAlive:  r.PIDAlive,
		}
		if d.Kind == "" {
			d.Kind = "server"
		}
		if r.CurrentAttempt != nil {
			d.BeadID = r.CurrentAttempt.BeadID
			d.BeadTitle = r.CurrentAttempt.BeadTitle
			if r.CurrentAttempt.Harness != "" {
				d.Harness = r.CurrentAttempt.Harness
			}
			if r.CurrentAttempt.Model != "" {
				d.Model = r.CurrentAttempt.Model
			}
			d.ElapsedMS = r.CurrentAttempt.ElapsedMS
			attemptIDs[r.CurrentAttempt.AttemptID] = true
		} else if r.LastResult != nil && r.LastResult.BeadID != "" {
			d.BeadID = r.LastResult.BeadID
		} else if r.CurrentBead != "" {
			d.BeadID = r.CurrentBead
		}
		out = append(out, d)
	}
	return out, attemptIDs
}

// agentLocalManifest is a minimal representation of .ddx/executions/{id}/manifest.json.
type agentLocalManifest struct {
	AttemptID string    `json:"attempt_id"`
	WorkerID  string    `json:"worker_id,omitempty"`
	BeadID    string    `json:"bead_id"`
	CreatedAt time.Time `json:"created_at"`
	Requested struct {
		Harness string `json:"harness,omitempty"`
		Model   string `json:"model,omitempty"`
	} `json:"requested"`
	Bead struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"bead"`
}

// scanLocalBeadWorkers scans .ddx/executions/*/manifest.json and checks
// active git worktrees to find locally-running execute-bead executions
// not already covered by the server.
func scanLocalBeadWorkers(projectRoot string, serverAttemptIDs map[string]bool) []agentWorkerDisplay {
	execDir := filepath.Join(projectRoot, ".ddx", "executions")
	entries, err := os.ReadDir(execDir)
	if err != nil {
		return nil
	}

	active := agentActiveWorktrees(projectRoot)

	var out []agentWorkerDisplay
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(execDir, entry.Name(), "manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var m agentLocalManifest
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}

		// Skip if covered by server (attempt already tracked)
		if serverAttemptIDs[m.AttemptID] {
			continue
		}
		// Skip if worker_id is set — server-submitted executions are in the server list
		if m.WorkerID != "" {
			continue
		}

		// Check if the worktree for this attempt is still active
		wtPath := agentLocalWorktreePath(m.BeadID, m.AttemptID)
		if !active[wtPath] {
			continue
		}

		elapsed := time.Since(m.CreatedAt)
		d := agentWorkerDisplay{
			ID:        "local-" + m.AttemptID,
			Kind:      "local",
			State:     "running",
			BeadID:    m.BeadID,
			BeadTitle: m.Bead.Title,
			Harness:   m.Requested.Harness,
			Model:     m.Requested.Model,
			StartedAt: m.CreatedAt,
			ElapsedMS: elapsed.Milliseconds(),
		}
		out = append(out, d)
	}
	return out
}

// agentLocalWorktreePath reconstructs the absolute path of an execute-bead
// worktree for the given bead and attempt IDs. Mirrors executeBeadWorktreePath
// in internal/agent/execute_bead.go.
func agentLocalWorktreePath(beadID, attemptID string) string {
	base := os.Getenv("DDX_EXEC_WT_DIR")
	if base == "" {
		base = filepath.Join(os.TempDir(), agent.ExecuteBeadTmpSubdir)
	}
	return filepath.Join(base, agent.ExecuteBeadWtPrefix+beadID+"-"+attemptID)
}

// agentActiveWorktrees returns a set of absolute worktree paths currently
// registered in the git repository at projectRoot.
func agentActiveWorktrees(projectRoot string) map[string]bool {
	out, err := gitpkg.Command(context.Background(), projectRoot, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil
	}
	paths := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			paths[strings.TrimPrefix(line, "worktree ")] = true
		}
	}
	return paths
}

// printAgentWorkersTable renders the workers list as a tab-aligned table.
func printAgentWorkersTable(w io.Writer, workers []agentWorkerDisplay) error {
	if len(workers) == 0 {
		_, err := fmt.Fprintln(w, "no active workers")
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STATE\tBEAD\tTITLE\tHARNESS\tMODEL\tELAPSED\tATTEMPTS")
	for _, wk := range workers {
		beadID := wk.BeadID
		if beadID == "" {
			beadID = "-"
		}
		title := agentTruncateTitle(wk.BeadTitle, 40)
		if title == "" {
			title = "-"
		}
		harness := wk.Harness
		if harness == "" {
			harness = "-"
		}
		model := wk.Model
		if model == "" {
			model = "-"
		}
		elapsed := agentFormatElapsed(wk.ElapsedMS, wk.StartedAt)
		attempts := fmt.Sprintf("%d", wk.Attempts)

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			wk.State, beadID, title, harness, model, elapsed, attempts)
	}
	return tw.Flush()
}

// agentTruncateTitle truncates s to at most max runes, appending "…" if truncated.
func agentTruncateTitle(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

// agentWorkerPruneResult mirrors server.PruneResult for JSON decoding.
type agentWorkerPruneResult struct {
	ID      string `json:"id"`
	BeadID  string `json:"bead_id,omitempty"`
	Harness string `json:"harness,omitempty"`
	Age     string `json:"age"`
	Reason  string `json:"reason"`
}

func (f *CommandFactory) newAgentWorkersPruneCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Reap stale worker registry entries whose process is no longer alive",
		Long: `Scan the worker registry and reap entries that are still marked running
but whose recorded process has already exited. This repairs registry noise left
by crashes, OOM kills, or server restarts without affecting live workers.

For each reaped entry the bead claim is released (status returns to open) and a
bead.reaped event is appended to the tracker. The on-disk state is updated to
state=reaped.

Examples:
  ddx agent workers prune
  ddx agent workers prune --max-age 24h
  ddx agent workers prune --json`,
		Args: cobra.NoArgs,
		RunE: f.runAgentWorkersPrune,
	}
	cmd.Flags().Duration("max-age", 0, "Also prune workers older than this duration regardless of PID liveness")
	cmd.Flags().Bool("json", false, "Emit JSON array of pruned workers")
	cmd.Flags().String("project", "", "Project root to query (default: detected from CWD)")
	return cmd
}

func (f *CommandFactory) runAgentWorkersPrune(cmd *cobra.Command, _ []string) error {
	maxAge, _ := cmd.Flags().GetDuration("max-age")
	asJSON, _ := cmd.Flags().GetBool("json")
	projectFlag, _ := cmd.Flags().GetString("project")

	projectRoot := projectFlag
	if projectRoot == "" {
		projectRoot = gitpkg.FindProjectRoot(f.WorkingDir)
	}

	base := resolveServerURL(projectRoot)
	client := newLocalServerClient()

	url := base + "/api/agent/workers/prune"
	if maxAge > 0 {
		url += "?max_age=" + maxAge.String()
	}
	req, err := http.NewRequestWithContext(cmd.Context(), http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("prune: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("prune: server error (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var results []agentWorkerPruneResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return fmt.Errorf("prune: decode response: %w", err)
	}

	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	if len(results) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no stale workers found")
		return nil
	}

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tBEAD\tHARNESS\tAGE\tREASON")
	for _, r := range results {
		beadID := r.BeadID
		if beadID == "" {
			beadID = "-"
		}
		harness := r.Harness
		if harness == "" {
			harness = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.ID, beadID, harness, r.Age, r.Reason)
	}
	return tw.Flush()
}

// agentFormatElapsed returns a human-readable elapsed duration like "4m32s".
// Uses elapsedMS if positive; otherwise derives from startedAt.
func agentFormatElapsed(elapsedMS int64, startedAt time.Time) string {
	var d time.Duration
	if elapsedMS > 0 {
		d = time.Duration(elapsedMS) * time.Millisecond
	} else if !startedAt.IsZero() {
		d = time.Since(startedAt)
		if d < 0 {
			d = 0
		}
	} else {
		return "-"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
