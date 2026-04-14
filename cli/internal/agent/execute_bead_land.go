package agent

// execute_bead_land.go — the land coordinator pattern.
//
// This file implements the human-PR landing model for execute-bead results.
// The old Merge() path in execute_bead_orchestrator.go has been deleted; all
// target-ref writes now flow through Land(), which is called exactly once per
// submission by a per-project coordinator goroutine (see
// internal/server/workers.go:LandCoordinator).
//
// The flow mirrors how a human merges PRs:
//
//   1. Fetch the current target tip (from origin when a remote exists, or
//      from the local branch otherwise).
//   2. If the current tip equals the worker's BaseRev — fast-forward the
//      target branch directly to the worker's ResultRev. One bead → one
//      commit, linear history.
//   3. Otherwise — the target has advanced since the worker started. Create
//      a temporary worktree at ResultRev, rebase onto the current tip, and
//      fast-forward the target branch to the rebased commit. Still one
//      bead → one commit, still linear.
//   4. If the rebase conflicts — abort cleanly, preserve the original
//      ResultRev under refs/ddx/iterations/<bead-id>/<attempt-id>-<short-tip>,
//      and return preserved status. Target branch is never modified.
//   5. If an origin remote exists — push the new target tip. The push is
//      strictly fast-forward; push failures are reported via PushFailed but
//      do not roll back the local target ref.
//
// NO git merge --no-edit. NO "chore: checkpoint before merge" commits. The
// coordinator owning the goroutine provides the serialization guarantee, so
// Land() itself does not take any locks.

import (
	"fmt"
	"os"
	osexec "os/exec"
	"strings"
)

// LandRequest is one submission to the land coordinator: "here is the worker's
// result from base B to result R for bead X; land it on the project's target
// branch."
type LandRequest struct {
	// WorktreeDir is the path to the project's repository directory (the
	// directory the coordinator operates on). The original worker worktree
	// has typically already been removed by the time Land() runs — Land()
	// creates its own temporary worktrees when a rebase is needed.
	WorktreeDir string

	// BaseRev is the revision the worker branched off when it started.
	// When the current target tip equals BaseRev, Land() takes the fast path.
	BaseRev string

	// ResultRev is the worker's final commit SHA. Must be reachable in the
	// project's git object store at the time Land() is called.
	ResultRev string

	// BeadID identifies the bead this submission is for. Used to build the
	// preserve-ref path on conflict.
	BeadID string

	// AttemptID uniquely identifies this land attempt. Used to build the
	// preserve-ref path on conflict so concurrent attempts for the same
	// bead do not collide.
	AttemptID string

	// TargetBranch is the branch to advance. When empty, Land() resolves
	// the project's current HEAD branch and uses that.
	TargetBranch string
}

// LandResult describes the outcome of a single Land() call.
type LandResult struct {
	// Status is one of:
	//   - "landed":    the target branch now points at a new commit
	//                  (either ResultRev itself on the fast path, or the
	//                  rebased tip on the rebase path).
	//   - "preserved": the rebase conflicted; ResultRev is saved under
	//                  PreserveRef and the target branch is unchanged.
	//   - "no-changes": ResultRev == BaseRev; nothing to land.
	Status string

	// NewTip is the new value of the target branch when Status == "landed".
	// Empty when preserved or no-changes.
	NewTip string

	// Rebased is true when the land took the rebase path (current tip had
	// advanced beyond BaseRev). False on the fast-forward path.
	Rebased bool

	// PreserveRef is set when Status == "preserved". It names the ref under
	// refs/ddx/iterations/ where ResultRev was saved for later recovery.
	PreserveRef string

	// Reason is a human-readable explanation, especially useful when
	// Status == "preserved" (e.g. "rebase conflict") or when PushFailed.
	Reason string

	// PushFailed is true when the local target ref was advanced successfully
	// but the subsequent push to origin was rejected (e.g. non-fast-forward).
	// The local state is authoritative; the remote will need to be
	// reconciled by a later land or an operator.
	PushFailed bool

	// PushError captures the underlying push error when PushFailed is true.
	PushError string
}

// LandingGitOps abstracts the git operations Land() needs. RealLandingGitOps
// shells out to git; tests inject fakes or run against real temp repos.
type LandingGitOps interface {
	// HasRemote reports whether the given remote name exists in dir.
	HasRemote(dir, remote string) bool

	// CurrentBranch returns the branch HEAD currently points at in dir, or
	// an error if HEAD is detached or unresolvable.
	CurrentBranch(dir string) (string, error)

	// FetchBranch fetches remote/branch into dir (no merge, no checkout).
	// Returns nil when no remote exists.
	FetchBranch(dir, remote, branch string) error

	// ResolveRef resolves ref (e.g. "refs/heads/main" or "origin/main") to a
	// commit SHA. When ref is unresolvable returns ("", error).
	ResolveRef(dir, ref string) (string, error)

	// UpdateRefTo updates ref in dir to sha. When oldSHA is non-empty, the
	// update is conditional on the current ref value equalling oldSHA.
	UpdateRefTo(dir, ref, sha, oldSHA string) error

	// SyncIndexToHead syncs the index of the worktree at dir to HEAD so
	// subsequent `git add; git commit` calls do not create a stale tree.
	// Needed because Land() advances the target ref via update-ref (which
	// does NOT touch the index) and a later CommitTracker call on the same
	// worktree would otherwise commit a tree missing the files from the
	// advanced HEAD. Safe when the worktree is clean; a no-op otherwise.
	SyncIndexToHead(dir string) error

	// CreateBranch creates branch at sha in dir. It is an error if branch
	// already exists.
	CreateBranch(dir, branch, sha string) error

	// DeleteBranch deletes branch in dir (force delete).
	DeleteBranch(dir, branch string) error

	// AddWorktree creates a new worktree at path checked out at rev in dir.
	AddWorktree(dir, path, rev string) error

	// RemoveWorktree removes the worktree at path in dir (force).
	RemoveWorktree(dir, path string) error

	// RebaseOnto runs `git rebase --onto onto upstream HEAD` inside wtDir.
	// Returns nil on clean rebase, or an error on conflict. On error, the
	// implementation is responsible for running `git rebase --abort`.
	RebaseOnto(wtDir, onto, upstream string) error

	// HeadRevAt returns HEAD of the git workdir at dir.
	HeadRevAt(dir string) (string, error)

	// PushFFOnly pushes localRef to remote as targetBranch with strict
	// fast-forward semantics. Returns an error when the push would not be
	// fast-forward or when the network call fails.
	PushFFOnly(dir, remote, localRef, targetBranch string) error
}

// RealLandingGitOps implements LandingGitOps via os/exec git commands.
type RealLandingGitOps struct{}

func (RealLandingGitOps) HasRemote(dir, remote string) bool {
	out, err := osexec.Command("git", "-C", dir, "remote").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == remote {
			return true
		}
	}
	return false
}

func (RealLandingGitOps) CurrentBranch(dir string) (string, error) {
	out, err := osexec.Command("git", "-C", dir, "symbolic-ref", "--short", "HEAD").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git symbolic-ref HEAD: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (RealLandingGitOps) FetchBranch(dir, remote, branch string) error {
	out, err := osexec.Command("git", "-C", dir, "fetch", remote, branch).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git fetch %s %s: %s: %w", remote, branch, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (RealLandingGitOps) ResolveRef(dir, ref string) (string, error) {
	out, err := osexec.Command("git", "-C", dir, "rev-parse", "--verify", ref).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (RealLandingGitOps) UpdateRefTo(dir, ref, sha, oldSHA string) error {
	args := []string{"-C", dir, "update-ref", ref, sha}
	if oldSHA != "" {
		args = append(args, oldSHA)
	}
	out, err := osexec.Command("git", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git update-ref %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (RealLandingGitOps) SyncIndexToHead(dir string) error {
	// `git read-tree HEAD` rewrites the index to match HEAD without
	// touching the working tree files. It is a no-op when the index
	// already matches. Errors are non-fatal — the worst case is that the
	// caller's next CommitTracker creates a stale tree, which is a
	// separate pre-existing bug not introduced by the coordinator pattern.
	out, err := osexec.Command("git", "-C", dir, "read-tree", "HEAD").CombinedOutput()
	if err != nil {
		return fmt.Errorf("git read-tree HEAD: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (RealLandingGitOps) CreateBranch(dir, branch, sha string) error {
	out, err := osexec.Command("git", "-C", dir, "branch", branch, sha).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git branch %s %s: %s: %w", branch, sha, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (RealLandingGitOps) DeleteBranch(dir, branch string) error {
	out, err := osexec.Command("git", "-C", dir, "branch", "-D", branch).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git branch -D %s: %s: %w", branch, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (RealLandingGitOps) AddWorktree(dir, path, rev string) error {
	out, err := osexec.Command("git", "-C", dir, "worktree", "add", "--force", path, rev).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add %s %s: %s: %w", path, rev, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (RealLandingGitOps) RemoveWorktree(dir, path string) error {
	_ = osexec.Command("git", "-C", dir, "worktree", "remove", "--force", path).Run()
	_ = osexec.Command("git", "-C", dir, "worktree", "prune").Run()
	return nil
}

func (RealLandingGitOps) RebaseOnto(wtDir, onto, upstream string) error {
	out, err := osexec.Command("git", "-C", wtDir, "rebase", "--onto", onto, upstream).CombinedOutput()
	if err != nil {
		_ = osexec.Command("git", "-C", wtDir, "rebase", "--abort").Run()
		return fmt.Errorf("git rebase --onto %s %s: %s: %w", onto, upstream, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (RealLandingGitOps) HeadRevAt(dir string) (string, error) {
	out, err := osexec.Command("git", "-C", dir, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (RealLandingGitOps) PushFFOnly(dir, remote, localRef, targetBranch string) error {
	// Refspec "<local>:<remote>" with no '+' prefix → fast-forward only.
	refspec := fmt.Sprintf("%s:refs/heads/%s", localRef, targetBranch)
	out, err := osexec.Command("git", "-C", dir, "push", remote, refspec).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push %s %s: %s: %w", remote, refspec, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// landIterationRef returns the documented hidden ref for a land-time preserve.
// Format: refs/ddx/iterations/<bead-id>/<attempt-id>-<short-tip>. The short-tip
// captures the current target tip at the time of the conflict so subsequent
// recovery tools can reconstruct which rebase target was in play.
func landIterationRef(beadID, attemptID, tip string) string {
	short := tip
	if len(short) > 12 {
		short = short[:12]
	}
	attempt := attemptID
	if attempt == "" {
		attempt = NowFunc().UTC().Format("20060102T150405Z")
	}
	return fmt.Sprintf("refs/ddx/iterations/%s/%s-%s", beadID, attempt, short)
}

// Land performs fetch → rebase-if-needed → ff → push for a single submission.
// Callers MUST serialize calls per projectRoot (the server coordinator
// goroutine provides this). Land() itself takes no internal locks.
//
// projectRoot is the directory containing the project's .git. req.WorktreeDir,
// when non-empty, is used as the git working directory for all commands; when
// empty, projectRoot is used. Both usually refer to the same dir — the field
// is kept for forward compatibility with multi-clone topologies.
func Land(projectRoot string, req LandRequest, gitOps LandingGitOps) (*LandResult, error) {
	if gitOps == nil {
		gitOps = RealLandingGitOps{}
	}
	wd := req.WorktreeDir
	if wd == "" {
		wd = projectRoot
	}

	// Trivial guard — no commits to land.
	if req.ResultRev == "" || req.ResultRev == req.BaseRev {
		return &LandResult{Status: "no-changes", Reason: "result_rev == base_rev"}, nil
	}

	// Resolve target branch (default to project HEAD).
	targetBranch := req.TargetBranch
	if targetBranch == "" {
		br, err := gitOps.CurrentBranch(wd)
		if err != nil {
			return nil, fmt.Errorf("resolving target branch: %w", err)
		}
		targetBranch = br
	}
	targetRef := "refs/heads/" + targetBranch

	// Fetch the current target tip from origin when available; otherwise
	// read the local branch directly.
	hasOrigin := gitOps.HasRemote(wd, "origin")
	if hasOrigin {
		// Fetch is best-effort: a disconnected remote must not block
		// a local land. Log-style errors are swallowed here; the
		// subsequent ResolveRef will surface any fatal issue.
		_ = gitOps.FetchBranch(wd, "origin", targetBranch)
	}
	currentTip, err := gitOps.ResolveRef(wd, targetRef)
	if err != nil {
		return nil, fmt.Errorf("resolving target tip %s: %w", targetRef, err)
	}

	// Fast path: no sibling advanced the target branch → straight ff.
	if currentTip == req.BaseRev {
		if err := gitOps.UpdateRefTo(wd, targetRef, req.ResultRev, currentTip); err != nil {
			return nil, fmt.Errorf("fast-forwarding %s to %s: %w", targetRef, req.ResultRev, err)
		}
		// Refresh the main worktree's index so a subsequent CommitTracker
		// doesn't build a stale tree. Non-fatal on error.
		_ = gitOps.SyncIndexToHead(wd)
		result := &LandResult{
			Status:  "landed",
			NewTip:  req.ResultRev,
			Rebased: false,
		}
		landPush(wd, targetBranch, req.ResultRev, hasOrigin, gitOps, result)
		return result, nil
	}

	// Rebase path: the target has advanced since the worker started. Replay
	// the worker's commits (BaseRev..ResultRev) onto currentTip in a
	// throwaway temp worktree.
	tempBranch := fmt.Sprintf("ddx-land-%s-%s", req.BeadID, shortAttempt(req.AttemptID))
	if err := gitOps.CreateBranch(wd, tempBranch, req.ResultRev); err != nil {
		return nil, fmt.Errorf("creating temp land branch %s: %w", tempBranch, err)
	}
	// Ensure the temp branch is always cleaned up (even on rebase conflict).
	defer func() { _ = gitOps.DeleteBranch(wd, tempBranch) }()

	tempWT, tempWtErr := os.MkdirTemp("", "ddx-land-wt-*")
	if tempWtErr != nil {
		return nil, fmt.Errorf("creating temp worktree dir: %w", tempWtErr)
	}
	// os.MkdirTemp creates the dir, but git worktree add refuses to run if
	// the target already exists. Remove it first so git can recreate it.
	_ = os.RemoveAll(tempWT)
	if err := gitOps.AddWorktree(wd, tempWT, tempBranch); err != nil {
		return nil, fmt.Errorf("adding temp worktree: %w", err)
	}
	defer func() { _ = gitOps.RemoveWorktree(wd, tempWT) }()

	if err := gitOps.RebaseOnto(tempWT, currentTip, req.BaseRev); err != nil {
		// Rebase conflict: preserve the original ResultRev and return.
		preserveRef := landIterationRef(req.BeadID, req.AttemptID, currentTip)
		if upErr := gitOps.UpdateRefTo(wd, preserveRef, req.ResultRev, ""); upErr != nil {
			return nil, fmt.Errorf("preserving %s after rebase conflict: %w", preserveRef, upErr)
		}
		return &LandResult{
			Status:      "preserved",
			PreserveRef: preserveRef,
			Reason:      "rebase conflict",
		}, nil
	}

	// Rebase clean: read the new tip from the temp worktree and ff the target.
	newTip, err := gitOps.HeadRevAt(tempWT)
	if err != nil {
		return nil, fmt.Errorf("reading rebased HEAD: %w", err)
	}
	if err := gitOps.UpdateRefTo(wd, targetRef, newTip, currentTip); err != nil {
		return nil, fmt.Errorf("fast-forwarding %s to rebased tip %s: %w", targetRef, newTip, err)
	}
	// Refresh the main worktree's index so a subsequent CommitTracker
	// doesn't build a stale tree. Non-fatal on error.
	_ = gitOps.SyncIndexToHead(wd)

	result := &LandResult{
		Status:  "landed",
		NewTip:  newTip,
		Rebased: true,
	}
	landPush(wd, targetBranch, newTip, hasOrigin, gitOps, result)
	return result, nil
}

// landPush pushes the new target tip to origin when a remote exists. Push
// failure is non-fatal for the local land outcome; it is surfaced via
// PushFailed/PushError on the result.
func landPush(wd, targetBranch, newTip string, hasOrigin bool, gitOps LandingGitOps, result *LandResult) {
	if !hasOrigin {
		return
	}
	if err := gitOps.PushFFOnly(wd, "origin", newTip, targetBranch); err != nil {
		result.PushFailed = true
		result.PushError = err.Error()
	}
}

// shortAttempt returns a 10-char slug derived from attemptID for use in temp
// branch names. When attemptID is empty, it returns the current timestamp.
func shortAttempt(attemptID string) string {
	if attemptID != "" {
		if len(attemptID) > 16 {
			return attemptID[:16]
		}
		return attemptID
	}
	return NowFunc().UTC().Format("20060102T150405")
}

// ApplyLandResultToExecuteBeadResult maps a LandResult onto an
// ExecuteBeadResult so the execute-bead loop sees the correct final status.
// It is the coordinator-pattern replacement for ApplyLandingToResult.
func ApplyLandResultToExecuteBeadResult(res *ExecuteBeadResult, land *LandResult) {
	if land == nil || res == nil {
		return
	}
	switch land.Status {
	case "landed":
		// One bead → one (rebased) commit → linear history.
		res.Outcome = "merged" // kept as "merged" for compatibility with existing supervisors
		res.Reason = ""
		res.PreserveRef = ""
		if land.Rebased {
			res.Reason = "rebased onto current tip"
		}
		if land.PushFailed {
			res.Reason = "landed locally; push failed: " + land.PushError
		}
		// ResultRev now reflects the ref actually on the target branch.
		if land.NewTip != "" {
			res.ResultRev = land.NewTip
		}
	case "preserved":
		res.Outcome = "preserved"
		res.Reason = land.Reason
		res.PreserveRef = land.PreserveRef
	case "no-changes":
		// Only overwrite when the worker itself did not already report
		// a richer no-changes rationale.
		if res.Outcome == "" || res.Outcome == ExecuteBeadOutcomeTaskSucceeded {
			res.Outcome = "no-changes"
		}
		if res.Reason == "" {
			res.Reason = land.Reason
		}
	}
	// Re-classify loop-visible status from the landing outcome.
	reasonForStatus := res.Reason
	if land.Status == "preserved" {
		// Route preserve reasons through the land-conflict classifier so the
		// loop sees land_conflict (not generic success).
		reasonForStatus = "rebase failed"
	}
	res.Status = ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, reasonForStatus)
	res.Detail = ExecuteBeadStatusDetail(res.Status, res.Reason, res.Error)
}

// BuildLandRequestFromResult constructs a LandRequest for the coordinator from
// an ExecuteBeadResult. The coordinator always passes projectRoot as the
// workdir — the worker's original worktree has already been cleaned up by the
// time Land() runs.
func BuildLandRequestFromResult(projectRoot string, res *ExecuteBeadResult) LandRequest {
	return LandRequest{
		WorktreeDir:  projectRoot,
		BaseRev:      res.BaseRev,
		ResultRev:    res.ResultRev,
		BeadID:       res.BeadID,
		AttemptID:    res.AttemptID,
		TargetBranch: "",
	}
}
