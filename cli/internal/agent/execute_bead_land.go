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
//      target branch directly to the worker's ResultRev via update-ref. The
//      worker's commit keeps its original parent; no new commit is created.
//   3. Otherwise — the target has advanced since the worker started. Create
//      a temporary detached worktree at the current target tip, run
//      `git merge --no-ff` to introduce the worker's ResultRev, and
//      fast-forward the target branch to the resulting merge commit. The
//      worker's commit is NEVER rewritten: its parent is still BaseRev, so
//      a later replay observes the same inputs the worker saw.
//   4. If the merge conflicts — abort cleanly, preserve the original
//      ResultRev under refs/ddx/iterations/<bead-id>/<attempt-id>-<short-tip>,
//      and return preserved status. Target branch is never modified.
//   5. If an origin remote exists — push the new target tip. The push is
//      strictly fast-forward; push failures are reported via PushFailed but
//      do not roll back the local target ref.
//
// Replay fidelity is the reason for merge-over-rebase: a rebased commit has
// a rewritten parent that lies about what the worker saw at execution time.
// A merge commit preserves both histories — the worker's original commit
// (parent = BaseRev) and the target's prior tip — losslessly.
//
// The coordinator owning the goroutine provides the serialization guarantee,
// so Land() itself does not take any locks.

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
)

// LandRequest is one submission to the land coordinator: "here is the worker's
// result from base B to result R for bead X; land it on the project's target
// branch."
type LandRequest struct {
	// WorktreeDir is the path to the project's repository directory (the
	// directory the coordinator operates on). The original worker worktree
	// has typically already been removed by the time Land() runs — Land()
	// creates its own temporary worktrees when a merge is needed.
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
	//                  (either ResultRev itself on the fast-forward path,
	//                  or a merge commit on the merge path).
	//   - "preserved": the merge conflicted; ResultRev is saved under
	//                  PreserveRef and the target branch is unchanged.
	//   - "no-changes": ResultRev == BaseRev; nothing to land.
	Status string

	// NewTip is the new value of the target branch when Status == "landed".
	// On the fast-forward path NewTip == ResultRev; on the merge path NewTip
	// is the SHA of the merge commit (whose parents are the prior target
	// tip and ResultRev). Empty when preserved or no-changes.
	NewTip string

	// Merged is true when the land took the merge-commit path (current tip
	// had advanced beyond BaseRev, so Land() created a merge commit to
	// combine the worker's result with the new target tip). False on the
	// fast-forward path where the worker's commit became the new tip
	// unchanged.
	Merged bool

	// PreserveRef is set when Status == "preserved". It names the ref under
	// refs/ddx/iterations/ where ResultRev was saved for later recovery.
	PreserveRef string

	// Reason is a human-readable explanation, especially useful when
	// Status == "preserved" (e.g. "merge conflict") or when PushFailed.
	Reason string

	// PushFailed is true when the local target ref was advanced successfully
	// but the subsequent push to origin was rejected (e.g. non-fast-forward).
	// The local state is authoritative; the remote will need to be
	// reconciled by a later land or an operator.
	PushFailed bool

	// PushError captures the underlying push error when PushFailed is true.
	PushError string

	// MergedCommitCount is the number of commits reachable from ResultRev but
	// not from BaseRev — i.e. the "size" of the worker's contribution being
	// merged in. Zero on the no-changes path. Set on both the fast-forward
	// and merge-commit paths so metrics can compare contribution sizes.
	MergedCommitCount int
}

// PreClaimResult is the outcome of a FetchOriginAncestryCheck call.
type PreClaimResult struct {
	// Action is one of:
	//   "unchanged"     — local tip == origin tip, no action taken
	//   "fast-forwarded"— local was behind origin; local branch was advanced
	//   "no-origin"     — no origin remote; check skipped
	//   "local-ahead"   — local is ahead of origin; no action needed
	//   "diverged"      — neither is ancestor of the other; claim should be aborted
	Action    string
	LocalSHA  string
	OriginSHA string
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

	// SyncWorkTreeToHead syncs the index AND the working-tree files in dir
	// to HEAD after a non-checkout ref update (e.g. update-ref). fromRev is
	// the commit HEAD pointed at BEFORE the ref update; it is used to
	// compute the minimal set of tracked files changed by the update so
	// that unrelated local modifications (agent-logs, beads.jsonl heartbeat
	// writes, operator scratch) are NOT clobbered.
	//
	// Needed because Land() advances the target ref via update-ref, which
	// touches neither the index nor the worktree. Before this fix, Land()
	// only ran `git read-tree HEAD` to sync the index — leaving the main
	// worktree showing phantom deleted/modified entries for every file the
	// landed commit touched, and subsequent agents reading files from disk
	// would see stale content.
	//
	// Implementation: `git read-tree HEAD` + `git diff --name-only fromRev
	// HEAD` to enumerate changed files + `git checkout-index -f --` to
	// materialize them from the freshly-synced index.
	SyncWorkTreeToHead(dir, fromRev string) error

	// AddWorktree creates a new worktree at path checked out at rev in dir.
	AddWorktree(dir, path, rev string) error

	// RemoveWorktree removes the worktree at path in dir (force).
	RemoveWorktree(dir, path string) error

	// MergeInto runs `git merge --no-ff -m msg srcRev` inside wtDir, which
	// must already be checked out at the current target tip. A clean merge
	// produces a merge commit whose parents are [currentTip, srcRev]; the
	// commit SHA can be read with HeadRevAt. Returns nil on clean merge,
	// or an error on conflict. On error, the implementation is responsible
	// for running `git merge --abort` so the worktree is left clean.
	MergeInto(wtDir, srcRev, msg string) error

	// HeadRevAt returns HEAD of the git workdir at dir.
	HeadRevAt(dir string) (string, error)

	// PushFFOnly pushes localRef to remote as targetBranch with strict
	// fast-forward semantics. Returns an error when the push would not be
	// fast-forward or when the network call fails.
	PushFFOnly(dir, remote, localRef, targetBranch string) error

	// FetchOriginAncestryCheck fetches origin/targetBranch and compares it
	// to the local branch tip. When origin is ahead the local branch is
	// fast-forwarded via update-ref + read-tree. When the two tips have
	// diverged (neither is ancestor of the other) the returned result has
	// Action == "diverged". When no origin remote exists the result has
	// Action == "no-origin". The caller decides whether to abort a claim.
	FetchOriginAncestryCheck(dir, targetBranch string) (PreClaimResult, error)

	// CountCommits returns the number of commits reachable from tip but not
	// from base (i.e. git rev-list --count base..tip). Used to record the
	// contribution size in land metrics. Returns 0 on error.
	CountCommits(dir, base, tip string) int
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

func (RealLandingGitOps) SyncWorkTreeToHead(dir, fromRev string) error {
	// Step 1: sync the index to HEAD. This is required before checkout-index
	// below will do anything useful, and also keeps subsequent CommitTracker
	// calls from building stale trees.
	out, err := osexec.Command("git", "-C", dir, "read-tree", "HEAD").CombinedOutput()
	if err != nil {
		return fmt.Errorf("git read-tree HEAD: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Step 2: compute the list of tracked files changed between the previous
	// HEAD and the current HEAD. These are the files that the landing commit
	// added, modified, or deleted. We only restore THESE files to avoid
	// clobbering unrelated local modifications (agent-logs being written by
	// the running server, beads.jsonl heartbeat updates, operator scratch).
	if fromRev == "" {
		// No previous HEAD known — fall back to the unsafe behaviour of
		// read-tree only. Acceptable when the caller is a best-effort path
		// that cannot reconstruct fromRev.
		return nil
	}
	diffOut, diffErr := osexec.Command("git", "-C", dir, "diff", "--name-only", fromRev, "HEAD").CombinedOutput()
	if diffErr != nil {
		// Diff failed (bad fromRev, shallow history, etc.) — leave the
		// worktree stale rather than risk a broken checkout. The CommitTracker
		// stale-tree bug is the prior status quo and no worse than before.
		return nil
	}
	changed := strings.Fields(strings.TrimSpace(string(diffOut)))
	if len(changed) == 0 {
		return nil
	}

	// Step 3: split into existing-in-HEAD (checkout-index) and
	// deleted-in-HEAD (os.Remove) buckets. checkout-index only writes files
	// that are in the index; it cannot represent a deletion, so we handle
	// those ourselves.
	var indexFiles []string
	var removedFiles []string
	for _, f := range changed {
		probe := osexec.Command("git", "-C", dir, "ls-files", "--error-unmatch", "--", f)
		if probe.Run() == nil {
			indexFiles = append(indexFiles, f)
		} else {
			removedFiles = append(removedFiles, f)
		}
	}

	// Step 4: materialize the index-present files into the working tree.
	// -f overwrites any stale content at these exact paths. Unrelated files
	// are untouched because we pass the specific path list.
	if len(indexFiles) > 0 {
		args := []string{"-C", dir, "checkout-index", "-f", "--"}
		args = append(args, indexFiles...)
		out2, err2 := osexec.Command("git", args...).CombinedOutput()
		if err2 != nil {
			return fmt.Errorf("git checkout-index -f: %s: %w", strings.TrimSpace(string(out2)), err2)
		}
	}

	// Step 5: remove files that the landing commit deleted and whose removal
	// did not propagate to the worktree because update-ref bypassed the
	// working-tree update.
	for _, f := range removedFiles {
		full := filepath.Join(dir, f)
		_ = os.Remove(full) // best-effort; leave the file if removal fails
	}

	return nil
}

func (RealLandingGitOps) AddWorktree(dir, path, rev string) error {
	// --detach so the worktree does not create a persistent branch.
	out, err := osexec.Command("git", "-C", dir, "worktree", "add", "--force", "--detach", path, rev).CombinedOutput()
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

func (RealLandingGitOps) MergeInto(wtDir, srcRev, msg string) error {
	// --no-ff forces a merge commit even when the merge could fast-forward
	// (which shouldn't happen given our caller's ancestry check, but is a
	// defensive guarantee that target history always gets a marker commit).
	// We inject user.name/user.email via -c so the merge commit can be
	// created even when the worktree inherited no git config; the
	// coordinator is a machine actor and should not adopt a human's identity.
	out, err := osexec.Command(
		"git", "-C", wtDir,
		"-c", "user.name=ddx-land-coordinator",
		"-c", "user.email=coordinator@ddx.local",
		"merge", "--no-ff", "-m", msg, srcRev,
	).CombinedOutput()
	if err != nil {
		_ = osexec.Command("git", "-C", wtDir, "merge", "--abort").Run()
		return fmt.Errorf("git merge --no-ff %s: %s: %w", srcRev, strings.TrimSpace(string(out)), err)
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

func (RealLandingGitOps) CountCommits(dir, base, tip string) int {
	out, err := osexec.Command("git", "-C", dir, "rev-list", "--count", base+".."+tip).CombinedOutput()
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(out))
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

// FetchOriginAncestryCheck implements LandingGitOps.FetchOriginAncestryCheck.
func (RealLandingGitOps) FetchOriginAncestryCheck(dir, targetBranch string) (PreClaimResult, error) {
	// Step 1: check for origin remote.
	out, err := osexec.Command("git", "-C", dir, "remote", "get-url", "origin").CombinedOutput()
	if err != nil {
		// No origin remote — single-machine case; skip check.
		return PreClaimResult{Action: "no-origin"}, nil
	}
	_ = out // remote URL not needed

	// Step 2: fetch origin/targetBranch (best-effort; failure is non-fatal but surfaced).
	if fetchOut, fetchErr := osexec.Command("git", "-C", dir, "fetch", "origin", targetBranch).CombinedOutput(); fetchErr != nil {
		return PreClaimResult{}, fmt.Errorf("git fetch origin %s: %s: %w", targetBranch, strings.TrimSpace(string(fetchOut)), fetchErr)
	}

	// Step 3: resolve local tip.
	localOut, localErr := osexec.Command("git", "-C", dir, "rev-parse", "--verify", "refs/heads/"+targetBranch).CombinedOutput()
	if localErr != nil {
		return PreClaimResult{}, fmt.Errorf("resolving local %s: %s: %w", targetBranch, strings.TrimSpace(string(localOut)), localErr)
	}
	localSHA := strings.TrimSpace(string(localOut))

	// Step 4: resolve origin tip.
	originOut, originErr := osexec.Command("git", "-C", dir, "rev-parse", "--verify", "refs/remotes/origin/"+targetBranch).CombinedOutput()
	if originErr != nil {
		return PreClaimResult{}, fmt.Errorf("resolving origin/%s: %s: %w", targetBranch, strings.TrimSpace(string(originOut)), originErr)
	}
	originSHA := strings.TrimSpace(string(originOut))

	// Step 5: compare.
	if localSHA == originSHA {
		return PreClaimResult{Action: "unchanged", LocalSHA: localSHA, OriginSHA: originSHA}, nil
	}

	// Is local an ancestor of origin? (origin is ahead)
	localAncestorErr := osexec.Command("git", "-C", dir, "merge-base", "--is-ancestor", localSHA, originSHA).Run()
	if localAncestorErr == nil {
		// Origin is ahead: fast-forward local branch via update-ref + sync worktree.
		targetRef := "refs/heads/" + targetBranch
		if upErr := osexec.Command("git", "-C", dir, "update-ref", targetRef, originSHA, localSHA).Run(); upErr != nil {
			return PreClaimResult{}, fmt.Errorf("fast-forwarding %s to %s: %w", targetRef, originSHA, upErr)
		}
		// Sync index + working tree to new HEAD so the main worktree files
		// reflect the pulled-down origin commits. Pass localSHA as fromRev
		// to restrict the restore to files actually changed by origin's
		// advance, preserving unrelated local modifications.
		_ = (RealLandingGitOps{}).SyncWorkTreeToHead(dir, localSHA)
		return PreClaimResult{Action: "fast-forwarded", LocalSHA: localSHA, OriginSHA: originSHA}, nil
	}

	// Is origin an ancestor of local? (local is ahead)
	originAncestorErr := osexec.Command("git", "-C", dir, "merge-base", "--is-ancestor", originSHA, localSHA).Run()
	if originAncestorErr == nil {
		return PreClaimResult{Action: "local-ahead", LocalSHA: localSHA, OriginSHA: originSHA}, nil
	}

	// Neither is ancestor of the other: diverged.
	return PreClaimResult{Action: "diverged", LocalSHA: localSHA, OriginSHA: originSHA}, nil
}

// landIterationRef returns the documented hidden ref for a land-time preserve.
// Format: refs/ddx/iterations/<bead-id>/<attempt-id>-<short-tip>. The short-tip
// captures the current target tip at the time of the conflict so subsequent
// recovery tools can reconstruct which merge target was in play.
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

// Land performs fetch → (ff or merge) → push for a single submission.
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

	contribCount := gitOps.CountCommits(wd, req.BaseRev, req.ResultRev)

	// Fast path: no sibling advanced the target branch → straight ff via
	// update-ref. The worker's commit becomes the new tip unchanged; its
	// parent is still BaseRev, so replay sees the same inputs the worker
	// saw. No merge commit is created.
	if currentTip == req.BaseRev {
		if err := gitOps.UpdateRefTo(wd, targetRef, req.ResultRev, currentTip); err != nil {
			return nil, fmt.Errorf("fast-forwarding %s to %s: %w", targetRef, req.ResultRev, err)
		}
		// Refresh the main worktree's index + working-tree files so the
		// operator and subsequent agents see the newly-landed commit's
		// changes. Pass currentTip as fromRev so only files actually
		// changed by this commit are restored — unrelated local
		// modifications (agent-logs, beads.jsonl heartbeat) are preserved.
		// Non-fatal on error.
		_ = gitOps.SyncWorkTreeToHead(wd, currentTip)
		result := &LandResult{
			Status:            "landed",
			NewTip:            req.ResultRev,
			Merged:            false,
			MergedCommitCount: contribCount,
		}
		landPush(wd, targetBranch, req.ResultRev, hasOrigin, gitOps, result)
		return result, nil
	}

	// Merge path: the target has advanced since the worker started. Create
	// a temp detached worktree at currentTip and run `git merge --no-ff
	// ResultRev` inside it. The result is a merge commit whose parents are
	// [currentTip, ResultRev]. Crucially, ResultRev itself is NOT rewritten:
	// its parent is still BaseRev, so replay observes the original inputs.
	tempWT, tempWtErr := os.MkdirTemp("", "ddx-land-wt-*")
	if tempWtErr != nil {
		return nil, fmt.Errorf("creating temp worktree dir: %w", tempWtErr)
	}
	// os.MkdirTemp creates the dir, but git worktree add refuses to run if
	// the target already exists. Remove it first so git can recreate it.
	_ = os.RemoveAll(tempWT)
	if err := gitOps.AddWorktree(wd, tempWT, currentTip); err != nil {
		return nil, fmt.Errorf("adding temp worktree at %s: %w", currentTip, err)
	}
	defer func() { _ = gitOps.RemoveWorktree(wd, tempWT) }()

	mergeMsg := fmt.Sprintf("Merge bead %s attempt %s into %s", req.BeadID, shortAttempt(req.AttemptID), targetBranch)
	if err := gitOps.MergeInto(tempWT, req.ResultRev, mergeMsg); err != nil {
		// Merge conflict: preserve the original ResultRev and return.
		preserveRef := landIterationRef(req.BeadID, req.AttemptID, currentTip)
		if upErr := gitOps.UpdateRefTo(wd, preserveRef, req.ResultRev, ""); upErr != nil {
			return nil, fmt.Errorf("preserving %s after merge conflict: %w", preserveRef, upErr)
		}
		return &LandResult{
			Status:            "preserved",
			PreserveRef:       preserveRef,
			Reason:            "merge conflict",
			MergedCommitCount: contribCount,
		}, nil
	}

	// Merge clean: read the merge commit SHA from the temp worktree's HEAD
	// and fast-forward the target branch to it.
	mergeSHA, err := gitOps.HeadRevAt(tempWT)
	if err != nil {
		return nil, fmt.Errorf("reading merge HEAD: %w", err)
	}
	if err := gitOps.UpdateRefTo(wd, targetRef, mergeSHA, currentTip); err != nil {
		return nil, fmt.Errorf("fast-forwarding %s to merge commit %s: %w", targetRef, mergeSHA, err)
	}
	// Refresh the main worktree's index + working-tree files. Pass
	// currentTip as fromRev; the merge commit's tree contains both the
	// current-tip files and the incoming ResultRev files, so the diff
	// against currentTip yields exactly the files affected by the merge.
	// Unrelated local modifications are preserved.
	_ = gitOps.SyncWorkTreeToHead(wd, currentTip)

	result := &LandResult{
		Status:            "landed",
		NewTip:            mergeSHA,
		Merged:            true,
		MergedCommitCount: contribCount,
	}
	landPush(wd, targetBranch, mergeSHA, hasOrigin, gitOps, result)
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
		// Fast-forward or merge commit — either way the target branch now
		// contains the worker's result. ResultRev's own parent is still
		// BaseRev so replay fidelity is preserved.
		res.Outcome = "merged"
		res.Reason = ""
		res.PreserveRef = ""
		if land.Merged {
			res.Reason = "merged onto current tip"
		}
		if land.PushFailed {
			res.Reason = "landed locally; push failed: " + land.PushError
		}
		// NewTip reflects the ref actually on the target branch (either
		// ResultRev on the ff path or the merge commit SHA on the merge path).
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
		reasonForStatus = "merge conflict"
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
