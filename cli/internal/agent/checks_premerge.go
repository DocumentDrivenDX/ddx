package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/checks"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

// PreMergeChecksDir is the project-relative directory scanned for check
// definitions before each land-back attempt.
const PreMergeChecksDir = ".ddx/checks"

// PreMergeChecksEvidenceSubdir is the per-attempt subdirectory under the
// execution evidence dir into which check result files (one JSON per check)
// are written.
const PreMergeChecksEvidenceSubdir = "checks"

// PreMergeChecksOutcome captures everything callers need to (a) decide
// whether to land or preserve and (b) record events on the bead.
type PreMergeChecksOutcome struct {
	// Results is the per-check outcome for every check that actually ran
	// (post-AppliesTo filter, post-bypass filter). Empty when no checks
	// applied.
	Results []checks.Result
	// Bypassed lists checks_bypass entries that took effect (i.e. matched a
	// loaded check by name and were honoured for this run). Each entry is
	// recorded as a `checks-bypass` event by AppendPreMergeChecksEvents.
	Bypassed []bead.ChecksBypassEntry
	// Blocked is true when at least one applicable, non-bypassed check
	// returned status=block or status=error. The caller MUST NOT land the
	// worker's result when Blocked is true; PreserveAfterPreMergeChecks
	// performs the canonical preservation + event-record dance.
	Blocked bool
	// BlockingNames lists the names of the checks responsible for Blocked=true,
	// sorted alphabetically for stable ordering in evidence.
	BlockingNames []string
	// Reason is a short, human-readable summary suitable for the
	// ExecuteBeadResult.Reason / land Reason field on a preserved outcome.
	Reason string
	// EvidenceDir is the absolute directory where per-check JSON result files
	// were written (the directory the checks runner used as EVIDENCE_DIR).
	EvidenceDir string
}

// RunPreMergeChecks loads all .ddx/checks/*.yaml definitions for the project,
// filters by AppliesTo against the bead's labels and the changed-paths set
// computed from base..result, removes any checks named in the bead's
// checks_bypass annotation, and runs the rest in parallel via checks.Run.
//
// The bead's checks_bypass annotation is validated up front via
// bead.ChecksBypasses; any malformed entry (missing reason, etc.) returns a
// non-nil error so the caller can preserve loudly instead of silently
// honouring an invalid bypass.
//
// evidenceDir is the absolute path to the per-attempt execution evidence
// directory (typically <projectRoot>/.ddx/executions/<run-id>/). Check result
// files are written under evidenceDir/checks/. The directory is created if it
// does not exist.
//
// When the project has no .ddx/checks/ directory or no applicable checks,
// the returned outcome has Blocked=false and empty Results — the caller
// should proceed with the land as if no gate were configured.
func RunPreMergeChecks(ctx context.Context, projectRoot string, b *bead.Bead, baseRev, resultRev, evidenceDir string) (*PreMergeChecksOutcome, error) {
	if projectRoot == "" {
		return nil, fmt.Errorf("pre-merge checks: project_root required")
	}
	if b == nil {
		return nil, fmt.Errorf("pre-merge checks: bead required")
	}

	bypasses, err := bead.ChecksBypasses(b)
	if err != nil {
		return nil, fmt.Errorf("pre-merge checks: invalid checks_bypass annotation: %w", err)
	}

	all, err := checks.LoadDir(filepath.Join(projectRoot, PreMergeChecksDir))
	if err != nil {
		return nil, fmt.Errorf("pre-merge checks: load: %w", err)
	}
	if len(all) == 0 {
		return &PreMergeChecksOutcome{}, nil
	}

	bypassByName := make(map[string]bead.ChecksBypassEntry, len(bypasses))
	for _, e := range bypasses {
		bypassByName[e.Name] = e
	}

	var toRun []checks.Check
	var honouredBypass []bead.ChecksBypassEntry
	for _, c := range all {
		if e, ok := bypassByName[c.Name]; ok {
			honouredBypass = append(honouredBypass, e)
			continue
		}
		toRun = append(toRun, c)
	}

	changed, err := preMergeChangedPaths(projectRoot, baseRev, resultRev)
	if err != nil {
		return nil, fmt.Errorf("pre-merge checks: diff base..result: %w", err)
	}

	checksEvidence := filepath.Join(evidenceDir, PreMergeChecksEvidenceSubdir)
	ictx := checks.InvocationContext{
		BeadID:       b.ID,
		DiffBase:     baseRev,
		DiffHead:     resultRev,
		ProjectRoot:  projectRoot,
		EvidenceDir:  checksEvidence,
		RunID:        newPreMergeRunID(),
		BeadLabels:   b.Labels,
		ChangedPaths: changed,
	}

	results, err := checks.Run(ctx, toRun, ictx)
	if err != nil {
		return nil, fmt.Errorf("pre-merge checks: run: %w", err)
	}

	outcome := &PreMergeChecksOutcome{
		Results:     results,
		Bypassed:    honouredBypass,
		EvidenceDir: checksEvidence,
	}
	for _, r := range results {
		if r.Status == checks.StatusBlock || r.Status == checks.StatusError {
			outcome.Blocked = true
			outcome.BlockingNames = append(outcome.BlockingNames, r.Name)
		}
	}
	if outcome.Blocked {
		sort.Strings(outcome.BlockingNames)
		outcome.Reason = fmt.Sprintf("pre-merge checks blocked: %s", strings.Join(outcome.BlockingNames, ", "))
	}
	return outcome, nil
}

// PreMergeChecksReason is the canonical Reason prefix written onto preserved
// landings whose preservation was triggered by the pre-merge checks gate.
// Callers (status classifiers, dashboards) match on this prefix to bucket
// pre-merge-checks misses apart from generic merge conflicts or gate failures.
const PreMergeChecksReason = "pre-merge checks blocked"

// AppendPreMergeChecksEvents records one bead event per honoured bypass
// (kind=checks-bypass) and, when blocked, one event per blocking check
// (kind=checks-blocked). Best-effort: append failures are returned as the
// first error encountered so callers can decide whether to surface or log.
//
// actor and source are forwarded onto every appended event so the audit
// trail attributes the gate decision to the right worker / loop session.
func AppendPreMergeChecksEvents(store BeadEventAppender, beadID string, outcome *PreMergeChecksOutcome, actor, source string, now time.Time) error {
	if store == nil || outcome == nil {
		return nil
	}
	var firstErr error
	for _, e := range outcome.Bypassed {
		body := fmt.Sprintf("name=%s\nreason=%s", e.Name, e.Reason)
		if e.Bead != "" {
			body += "\nbead=" + e.Bead
		}
		ev := bead.BeadEvent{
			Kind:      "checks-bypass",
			Summary:   fmt.Sprintf("pre-merge check %q bypassed", e.Name),
			Body:      body,
			Actor:     actor,
			Source:    source,
			CreatedAt: now,
		}
		if err := store.AppendEvent(beadID, ev); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if outcome.Blocked {
		for _, r := range outcome.Results {
			if r.Status != checks.StatusBlock && r.Status != checks.StatusError {
				continue
			}
			body := fmt.Sprintf("name=%s\nstatus=%s", r.Name, r.Status)
			if r.Message != "" {
				body += "\nmessage=" + r.Message
			}
			ev := bead.BeadEvent{
				Kind:      "checks-blocked",
				Summary:   fmt.Sprintf("pre-merge check %q returned %s", r.Name, r.Status),
				Body:      body,
				Actor:     actor,
				Source:    source,
				CreatedAt: now,
			}
			if err := store.AppendEvent(beadID, ev); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// PreserveAfterPreMergeChecks is the canonical preservation step invoked when
// RunPreMergeChecks returned Blocked=true. It writes refs/ddx/iterations/...
// pointing at the worker's resultRev (so the work survives for later
// inspection) and returns the synthetic LandResult{Status:"preserved"} that
// callers can hand to ApplyLandResultToExecuteBeadResult.
//
// The preserved iteration ref naming follows the existing
// landIterationRef convention so operators see a single ref namespace
// regardless of which gate triggered the preservation.
func PreserveAfterPreMergeChecks(projectRoot string, res *ExecuteBeadResult, outcome *PreMergeChecksOutcome, gitOps OrchestratorGitOps) (*LandResult, error) {
	if outcome == nil || !outcome.Blocked {
		return nil, fmt.Errorf("pre-merge checks: preserve called without blocked outcome")
	}
	if gitOps == nil {
		gitOps = &RealOrchestratorGitOps{}
	}
	ref := PreserveRef(res.BeadID, res.BaseRev)
	if err := gitOps.UpdateRef(projectRoot, ref, res.ResultRev); err != nil {
		return nil, fmt.Errorf("pre-merge checks: preserving %s: %w", ref, err)
	}
	return &LandResult{
		Status:      "preserved",
		PreserveRef: ref,
		Reason:      outcome.Reason,
	}, nil
}

// preMergeChangedPaths returns the list of paths changed between baseRev and
// resultRev as reported by `git diff --name-only`. Returns (nil, nil) when
// either rev is empty (e.g. a no-changes attempt that should not exercise the
// path-glob filters at all).
func preMergeChangedPaths(projectRoot, baseRev, resultRev string) ([]string, error) {
	if baseRev == "" || resultRev == "" || baseRev == resultRev {
		return nil, nil
	}
	out, err := internalgit.Command(context.Background(), projectRoot, "diff", "--name-only", baseRev, resultRev).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s %s: %s: %w", baseRev, resultRev, strings.TrimSpace(string(out)), err)
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if p := strings.TrimSpace(line); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

func newPreMergeRunID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b[:])
}
