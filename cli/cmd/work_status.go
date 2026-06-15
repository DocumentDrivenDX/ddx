package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/activework"
	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/spf13/cobra"
)

// newWorkStatusCommand creates the "ddx work status" subcommand: a
// project-scoped live-worker report.
//
// The default report lists only live `ddx work` / `ddx try` processes whose
// resolved project root matches the requested project. The earlier failure
// mode this command exists to prevent is a global `ps | grep ddx work` scan
// surfacing a worker from another repository while operators are asking
// about a specific project's queue.
func (f *CommandFactory) newWorkStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show live ddx workers scoped to this project",
		Long: `status lists the live ddx worker processes (ddx work / ddx try)
whose project root matches the current project. The default scope is the
git root of the working directory; pass --project to override or
--all-projects to inspect every worker on the host.

Workers from other repositories are intentionally excluded from the
default view. Reporting them as evidence that "the worker is still
running" hides whether the requested project's queue has stalled.

Output columns (terminal):
  PID         — process id of the worker
  AGE         — wall-clock age since the worker process started
  COMMAND     — the worker's command line (truncated for terminals)
  BEAD        — active bead id when it can be inferred from argv or the
                isolated execute-bead worktree
  ATTEMPT     — active attempt id when a fresh worker liveness sidecar
                records one for the live worker
  WORKTREE    — execute-bead worktree path when the worker is inside one

JSON output preserves the full set of fields including project_root,
attempt_id, phase, child_pid, and last_activity_at when available.`,
		Example: `  # Show live workers for the current project
  ddx work status

  # JSON for piping to jq
  ddx work status --json

  # Explicit cross-project view
  ddx work status --all-projects --json`,
		Args: cobra.NoArgs,
		RunE: f.runWorkStatus,
	}
	cmd.Flags().String("project", "", "Target project root path (default: CWD git root). Env: DDX_PROJECT_ROOT")
	cmd.Flags().Bool("all-projects", false, "Report workers from every project; opt-in escape hatch")
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

// workerStatusScanner is set by tests via the CommandFactory to inject a
// deterministic worker set without spawning real processes.
type workerStatusScanner = workerstatus.Scanner

// WorkStatusReport is the JSON payload emitted by `ddx work status --json`.
type WorkStatusReport struct {
	ProjectRoot string                    `json:"project_root,omitempty"`
	Scope       string                    `json:"scope"`
	Workers     []workerstatus.LiveWorker `json:"workers"`
	ActiveWork  activework.Snapshot       `json:"active_work"`
}

func (f *CommandFactory) runWorkStatus(cmd *cobra.Command, _ []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	allProjects, _ := cmd.Flags().GetBool("all-projects")
	asJSON, _ := cmd.Flags().GetBool("json")

	projectRoot := resolveProjectRoot(projectFlag, f.WorkingDir)

	scanner := f.workerScanner()
	all, err := scanner.Scan(cmd.Context())
	if err != nil {
		return fmt.Errorf("scan live workers: %w", err)
	}

	now := time.Now().UTC()
	liveWorkers := enrichWorkersWithRunState(
		workerstatus.EnrichWithFreshLiveness(filterAndSortWorkers(all, projectRoot, allProjects), now),
		now,
	)
	liveWorkers = mergeServerManagedWorkers(
		liveWorkers,
		f.serverManagedWorkerRecords(projectRoot),
		projectRoot,
		allProjects,
		now,
	)
	report := WorkStatusReport{
		ProjectRoot: projectRoot,
		Scope:       "project",
		Workers:     liveWorkers,
	}
	if active, err := collectActiveWorkSnapshot(projectRoot, bead.NewStore(resolveBeadStoreRoot(projectRoot)), now); err == nil {
		report.ActiveWork = active
	} else {
		return fmt.Errorf("work status: active work query: %w", err)
	}
	if allProjects {
		report.Scope = "all-projects"
	}

	out := cmd.OutOrStdout()
	if asJSON {
		return writeWorkStatusJSON(out, report)
	}
	writeWorkStatusText(out, report)
	return nil
}

func (f *CommandFactory) workerScanner() workerStatusScanner {
	if f.workerScannerOverride != nil {
		return f.workerScannerOverride
	}
	return workerstatus.New()
}

// enrichWorkersWithRunState reconciles each live worker with the freshest
// per-attempt run-state record (.ddx/run-state/<attempt>.json) whose PID
// matches it. Run-state is read from each worker's own project root, so
// all-projects scope does not leak attempts across projects.
//
// Two cases are handled so that workers[], active_work.records[], and text
// status all report one stable canonical attempt id for a single live worker
// (ddx-f93e6ef9):
//
//   - Fill: liveness enrichment did not supply bead/attempt/worktree (sidecar
//     absent, start-time-skewed, or its attempt id missing) — run-state fills
//     the empty fields (ddx-f9b41107).
//   - Canonical override: the liveness sidecar supplied a *stale* attempt id
//     while run-state holds a fresher record for the same PID. When run-state
//     reports a different attempt and was refreshed more recently than the
//     liveness record's last activity, run-state is the canonical execution
//     attempt and its bead/attempt/worktree win over the stale liveness fields.
func enrichWorkersWithRunState(workers []workerstatus.LiveWorker, now time.Time) []workerstatus.LiveWorker {
	byProject := make(map[string][]agent.RunState)
	for i := range workers {
		w := &workers[i]
		if w.PID <= 0 {
			continue
		}
		states, ok := byProject[w.ProjectRoot]
		if !ok {
			states, _ = agent.ReadRunStates(w.ProjectRoot)
			byProject[w.ProjectRoot] = states
		}
		rec, ok := freshestRunStateForPID(states, w.PID, now)
		if !ok {
			continue
		}
		if rec.AttemptID != "" && rec.AttemptID != w.AttemptID && rec.RefreshedAt.After(w.LastActivityAt) {
			// Stale liveness attempt; run-state is canonical for this PID.
			if rec.BeadID != "" {
				w.BeadID = rec.BeadID
			}
			w.AttemptID = rec.AttemptID
			if rec.WorktreePath != "" {
				w.ExecutionWorktree = rec.WorktreePath
			}
			if !rec.RefreshedAt.IsZero() {
				w.LastActivityAt = rec.RefreshedAt.UTC()
			}
			continue
		}
		if w.BeadID == "" {
			w.BeadID = rec.BeadID
		}
		if w.AttemptID == "" {
			w.AttemptID = rec.AttemptID
		}
		if w.ExecutionWorktree == "" {
			w.ExecutionWorktree = rec.WorktreePath
		}
	}
	return workers
}

// freshestRunStateForPID returns the most recently refreshed non-expired
// run-state record for pid, if any.
func freshestRunStateForPID(states []agent.RunState, pid int, now time.Time) (agent.RunState, bool) {
	var best agent.RunState
	found := false
	for _, s := range states {
		if s.PID != pid {
			continue
		}
		if !s.ExpiresAt.IsZero() && now.After(s.ExpiresAt) {
			continue
		}
		if !found || s.RefreshedAt.After(best.RefreshedAt) {
			best = s
			found = true
		}
	}
	return best, found
}

func filterAndSortWorkers(all []workerstatus.LiveWorker, projectRoot string, allProjects bool) []workerstatus.LiveWorker {
	var filtered []workerstatus.LiveWorker
	if allProjects {
		filtered = append(filtered, all...)
	} else {
		filtered = workerstatus.FilterByProject(all, projectRoot)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].StartedAt.Equal(filtered[j].StartedAt) {
			return filtered[i].PID < filtered[j].PID
		}
		return filtered[i].StartedAt.Before(filtered[j].StartedAt)
	})
	return filtered
}

func (f *CommandFactory) serverManagedWorkerRecords(projectRoot string) []serverpkg.WorkerRecord {
	if f.workerScannerOverride == nil || os.Getenv("DDX_SERVER_URL") != "" {
		if records, ok := fetchServerManagedWorkerRecords(projectRoot); ok {
			return records
		}
	}
	records, err := readServerManagedWorkerRecords(projectRoot)
	if err != nil {
		return nil
	}
	return records
}

func fetchServerManagedWorkerRecords(projectRoot string) ([]serverpkg.WorkerRecord, bool) {
	base := strings.TrimRight(resolveServerURL(projectRoot), "/")
	req, err := http.NewRequest(http.MethodGet, base+"/api/agent/workers", nil)
	if err != nil {
		return nil, false
	}
	resp, err := newLocalServerClient().Do(req)
	if err != nil {
		return nil, false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false
	}
	var records []serverpkg.WorkerRecord
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
		return nil, false
	}
	return records, true
}

func mergeServerManagedWorkers(workers []workerstatus.LiveWorker, records []serverpkg.WorkerRecord, projectRoot string, allProjects bool, now time.Time) []workerstatus.LiveWorker {
	if len(records) == 0 {
		return workers
	}
	seen := make(map[string]bool, len(workers))
	for _, w := range workers {
		if w.AttemptID != "" {
			seen["attempt:"+w.AttemptID] = true
		}
		if w.PID > 0 {
			seen[fmt.Sprintf("pid:%d", w.PID)] = true
		}
	}
	for _, rec := range records {
		if !isLiveServerManagedWorker(rec, now) {
			continue
		}
		if !allProjects && !workerstatus.SamePath(rec.ProjectRoot, projectRoot) {
			continue
		}
		w := liveWorkerFromServerRecord(rec, now)
		if w.AttemptID != "" && seen["attempt:"+w.AttemptID] {
			continue
		}
		if w.PID > 0 && seen[fmt.Sprintf("pid:%d", w.PID)] {
			continue
		}
		if w.AttemptID != "" {
			seen["attempt:"+w.AttemptID] = true
		}
		if w.PID > 0 {
			seen[fmt.Sprintf("pid:%d", w.PID)] = true
		}
		workers = append(workers, w)
	}
	sort.SliceStable(workers, func(i, j int) bool {
		if workers[i].StartedAt.Equal(workers[j].StartedAt) {
			return workers[i].PID < workers[j].PID
		}
		return workers[i].StartedAt.Before(workers[j].StartedAt)
	})
	return workers
}

func readServerManagedWorkerRecords(projectRoot string) ([]serverpkg.WorkerRecord, error) {
	dir := workerstatus.LivenessDir(projectRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]serverpkg.WorkerRecord, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name(), "status.json"))
		if err != nil {
			continue
		}
		var rec serverpkg.WorkerRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}
		if rec.Status == "" {
			rec.Status = rec.State
		}
		if rec.ID == "" || rec.Kind == "" || rec.State == "" {
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}

func isLiveServerManagedWorker(rec serverpkg.WorkerRecord, now time.Time) bool {
	if rec.State != "running" && rec.State != "stopping" {
		return false
	}
	if !rec.FinishedAt.IsZero() {
		return false
	}
	if rec.State == "stopping" && now.Sub(serverWorkerLastActivity(rec, rec.StartedAt)) > 2*time.Minute {
		return false
	}
	return true
}

func liveWorkerFromServerRecord(rec serverpkg.WorkerRecord, now time.Time) workerstatus.LiveWorker {
	rec = enrichServerRecordWithRunState(rec, now)
	startedAt := rec.StartedAt.UTC()
	if startedAt.IsZero() && rec.CurrentAttempt != nil {
		startedAt = rec.CurrentAttempt.StartedAt.UTC()
	}
	age := now.Sub(startedAt)
	if startedAt.IsZero() || age < 0 {
		age = 0
	}
	beadID := rec.CurrentBead
	attemptID := ""
	phase := rec.State
	if rec.CurrentAttempt != nil {
		if rec.CurrentAttempt.BeadID != "" {
			beadID = rec.CurrentAttempt.BeadID
		}
		attemptID = rec.CurrentAttempt.AttemptID
		if rec.CurrentAttempt.Phase != "" {
			phase = rec.CurrentAttempt.Phase
		}
	}
	message := ""
	if rec.LastError != "" && rec.LastError != "success" {
		message = rec.LastError
	}
	return workerstatus.LiveWorker{
		PID:            rec.PID,
		Command:        "server-managed ddx work " + rec.ID,
		StartedAt:      startedAt,
		AgeSeconds:     age.Seconds(),
		Age:            workerstatus.FormatAge(age),
		ProjectRoot:    rec.ProjectRoot,
		BeadID:         beadID,
		AttemptID:      attemptID,
		Phase:          phase,
		Message:        message,
		LastActivityAt: serverWorkerLastActivity(rec, now),
	}
}

func enrichServerRecordWithRunState(rec serverpkg.WorkerRecord, now time.Time) serverpkg.WorkerRecord {
	if rec.State != "running" || rec.ProjectRoot == "" {
		return rec
	}
	if rec.CurrentAttempt != nil && rec.CurrentAttempt.AttemptID != "" {
		return rec
	}
	state, err := agent.ReadRunState(rec.ProjectRoot)
	if err != nil || state == nil || state.AttemptID == "" {
		return rec
	}
	if rec.CurrentBead != "" && state.BeadID != "" && rec.CurrentBead != state.BeadID {
		return rec
	}
	if !state.ExpiresAt.IsZero() && now.After(state.ExpiresAt) {
		return rec
	}
	if rec.CurrentBead == "" {
		rec.CurrentBead = state.BeadID
	}
	rec.CurrentAttempt = &serverpkg.CurrentAttemptInfo{
		AttemptID: state.AttemptID,
		BeadID:    state.BeadID,
		Harness:   state.Harness,
		Model:     state.Model,
		Profile:   rec.Profile,
		Phase:     "running",
		StartedAt: state.StartedAt,
	}
	if rec.Harness == "" {
		rec.Harness = state.Harness
	}
	if rec.Model == "" {
		rec.Model = state.Model
	}
	return rec
}

func serverWorkerLastActivity(rec serverpkg.WorkerRecord, fallback time.Time) time.Time {
	best := fallback.UTC()
	if best.IsZero() && !rec.StartedAt.IsZero() {
		best = rec.StartedAt.UTC()
	}
	consider := func(ts time.Time) {
		if ts.IsZero() {
			return
		}
		ts = ts.UTC()
		if best.IsZero() || ts.After(best) {
			best = ts
		}
	}
	consider(rec.StartedAt)
	for _, evt := range rec.Lifecycle {
		consider(evt.Timestamp)
	}
	for _, phase := range rec.RecentPhases {
		consider(phase.TS)
	}
	if rec.CurrentAttempt != nil {
		consider(rec.CurrentAttempt.StartedAt)
	}
	return best
}

func writeWorkStatusJSON(out io.Writer, report WorkStatusReport) error {
	if report.Workers == nil {
		report.Workers = []workerstatus.LiveWorker{}
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func writeWorkStatusText(out io.Writer, report WorkStatusReport) {
	if report.ActiveWork.Count > 0 {
		fmt.Fprintf(out, "active work for %s: %d bead(s)\n", report.ProjectRoot, report.ActiveWork.Count)
		if len(report.ActiveWork.BeadIDs) > 0 {
			fmt.Fprintf(out, "  beads: %s\n", strings.Join(report.ActiveWork.BeadIDs, ", "))
		}
	} else {
		fmt.Fprintf(out, "active work for %s: 0 bead(s)\n", report.ProjectRoot)
	}
	if len(report.Workers) == 0 {
		if report.Scope == "all-projects" {
			fmt.Fprintln(out, "No live ddx workers found on this host.")
			return
		}
		fmt.Fprintf(out, "No live ddx workers for project %s.\n", report.ProjectRoot)
		return
	}
	if report.Scope == "all-projects" {
		fmt.Fprintf(out, "live ddx workers (all projects): %d\n", len(report.Workers))
	} else {
		fmt.Fprintf(out, "live ddx workers for %s: %d\n", report.ProjectRoot, len(report.Workers))
	}
	for _, w := range report.Workers {
		bead := w.BeadID
		if bead == "" {
			bead = "-"
		}
		attempt := w.AttemptID
		if attempt == "" {
			attempt = "-"
		}
		worktree := w.ExecutionWorktree
		if worktree == "" {
			worktree = "-"
		}
		line := fmt.Sprintf("  pid=%d age=%s project=%s bead=%s attempt=%s worktree=%s",
			w.PID, w.Age, w.ProjectRoot, bead, attempt, worktree)
		if w.Phase != "" {
			line += fmt.Sprintf(" phase=%s", w.Phase)
		}
		if w.Message != "" {
			line += fmt.Sprintf(" message=%q", w.Message)
		}
		fmt.Fprintf(out, "%s\n    %s\n", line, w.Command)
	}
}
