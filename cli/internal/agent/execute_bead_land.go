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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

const defaultLargeDeletionLineThreshold = 200

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

	// EvidenceDir is the relative path to the per-attempt execution evidence
	// directory (e.g. ".ddx/executions/20260416T181205-b5419982"). When
	// non-empty and the main land succeeds, Land() creates a trailing
	// evidence commit that folds these files into the target branch. The
	// agent's commit SHA is NOT amended — the evidence commit is a separate
	// child commit, preserving closing_commit_sha references.
	EvidenceDir string

	// PostLandCommand is an optional project verification command run after
	// Land() advances the local target ref and syncs the worktree, but before
	// evidence commit creation or push. A failure restores the target ref to
	// its pre-land SHA and preserves ResultRev under refs/ddx/iterations.
	PostLandCommand []string

	// LargeDeletionLineThreshold overrides the default per-file deletion gate
	// threshold. Zero or negative uses defaultLargeDeletionLineThreshold.
	LargeDeletionLineThreshold int
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

	// PushConflict is true when the initial push was rejected (e.g. origin
	// advanced) and the loop's automatic pull/merge/retry recovery ran into
	// a merge conflict it cannot resolve without operator input. Distinct
	// from PushFailed so the loop can park the bead for human review under
	// a short cooldown instead of treating it as a generic push failure.
	PushConflict bool

	// PushRecovered is true when the initial push was rejected but the loop
	// successfully fetched, merged, and re-pushed without operator input.
	// The land is fully complete; PushFailed and PushConflict are false.
	// Set so callers can record telemetry on how often the auto-recovery
	// path actually saves an operator round-trip.
	PushRecovered bool

	// MergedCommitCount is the number of commits reachable from ResultRev but
	// not from BaseRev — i.e. the "size" of the worker's contribution being
	// merged in. Zero on the no-changes path. Set on both the fast-forward
	// and merge-commit paths so metrics can compare contribution sizes.
	MergedCommitCount int

	// EvidenceCommitSHA is set when a trailing execution-evidence commit was
	// created after the main land. When set, NewTip points at this commit
	// (not at the original agent commit or merge commit).
	EvidenceCommitSHA string
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

	// StageDir stages all files under relPath in dir for the next commit.
	StageDir(dir, relPath string) error

	// CommitStaged creates a commit from currently staged changes using msg
	// as the commit message. Returns (sha, nil) when a commit was made,
	// ("", nil) when nothing was staged, and ("", err) on failure.
	CommitStaged(dir, msg string) (string, error)

	// DiffNumstat returns the output of `git diff --numstat base tip --` in
	// dir. Used by the large-deletion gate. Returning ("", nil) indicates no
	// diffable output (e.g. in a test stub with no real repo).
	DiffNumstat(dir, base, tip string) (string, error)

	// DiffNameOnly returns the list of changed file paths between base and tip
	// in dir (`git diff --name-only base tip --`). Used by the syntax-sanity
	// gate. Returning (nil, nil) indicates no changes.
	DiffNameOnly(dir, base, tip string) ([]string, error)
}

// RealLandingGitOps implements LandingGitOps via os/exec git commands.
type RealLandingGitOps struct{}

func (RealLandingGitOps) HasRemote(dir, remote string) bool {
	out, err := internalgit.Command(context.Background(), dir, "remote").Output()
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
	out, err := internalgit.Command(context.Background(), dir, "symbolic-ref", "--short", "HEAD").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git symbolic-ref HEAD: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (RealLandingGitOps) FetchBranch(dir, remote, branch string) error {
	out, err := internalgit.Command(context.Background(), dir, "fetch", remote, branch).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git fetch %s %s: %s: %w", remote, branch, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (RealLandingGitOps) ResolveRef(dir, ref string) (string, error) {
	out, err := internalgit.Command(context.Background(), dir, "rev-parse", "--verify", ref).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (RealLandingGitOps) UpdateRefTo(dir, ref, sha, oldSHA string) error {
	args := []string{"update-ref", ref, sha}
	if oldSHA != "" {
		args = append(args, oldSHA)
	}
	out, err := internalgit.Command(context.Background(), dir, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git update-ref %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (RealLandingGitOps) SyncWorkTreeToHead(dir, fromRev string) error {
	// Step 1: sync the index to HEAD. This is required before checkout-index
	// below will do anything useful, and also keeps subsequent CommitTracker
	// calls from building stale trees.
	out, err := internalgit.Command(context.Background(), dir, "read-tree", "HEAD").CombinedOutput()
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
	diffOut, diffErr := internalgit.Command(context.Background(), dir, "diff", "--name-only", fromRev, "HEAD").CombinedOutput()
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
		probe := internalgit.Command(context.Background(), dir, "ls-files", "--error-unmatch", "--", f)
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
		args := []string{"checkout-index", "-f", "--"}
		args = append(args, indexFiles...)
		out2, err2 := internalgit.Command(context.Background(), dir, args...).CombinedOutput()
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
	out, err := internalgit.Command(context.Background(), dir, "worktree", "add", "--force", "--detach", path, rev).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add %s %s: %s: %w", path, rev, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (RealLandingGitOps) RemoveWorktree(dir, path string) error {
	_ = internalgit.Command(context.Background(), dir, "worktree", "remove", "--force", path).Run()
	_ = internalgit.Command(context.Background(), dir, "worktree", "prune").Run()
	return nil
}

func (RealLandingGitOps) MergeInto(wtDir, srcRev, msg string) error {
	// --no-ff forces a merge commit even when the merge could fast-forward
	// (which shouldn't happen given our caller's ancestry check, but is a
	// defensive guarantee that target history always gets a marker commit).
	// We inject user.name/user.email via -c so the merge commit can be
	// created even when the worktree inherited no git config; the
	// coordinator is a machine actor and should not adopt a human's identity.
	out, err := internalgit.Command(
		context.Background(), wtDir,
		"-c", "user.name=ddx-land-coordinator",
		"-c", "user.email=coordinator@ddx.local",
		"merge", "--no-ff", "-m", msg, srcRev,
	).CombinedOutput()
	if err != nil {
		_ = internalgit.Command(context.Background(), wtDir, "merge", "--abort").Run()
		return fmt.Errorf("git merge --no-ff %s: %s: %w", srcRev, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (RealLandingGitOps) HeadRevAt(dir string) (string, error) {
	out, err := internalgit.Command(context.Background(), dir, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (RealLandingGitOps) PushFFOnly(dir, remote, localRef, targetBranch string) error {
	// Refspec "<local>:<remote>" with no '+' prefix → fast-forward only.
	refspec := fmt.Sprintf("%s:refs/heads/%s", localRef, targetBranch)
	out, err := internalgit.Command(context.Background(), dir, "push", remote, refspec).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push %s %s: %s: %w", remote, refspec, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (RealLandingGitOps) CountCommits(dir, base, tip string) int {
	out, err := internalgit.Command(context.Background(), dir, "rev-list", "--count", base+".."+tip).CombinedOutput()
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

func (RealLandingGitOps) StageDir(dir, relPath string) error {
	// Exclude embedded session logs from the evidence commit — tracking
	// multi-thousand-line .jsonl files caused retry review prompts to
	// balloon past 2M tokens and crash every provider with n_keep > n_ctx
	// (ddx-39e27896). manifest.json, result.json, prompt.md, and
	// usage.json remain tracked for audit; the raw session log lives on
	// disk, not in git history.
	args := append([]string{"add", "--", relPath}, EvidenceLandExcludePathspecs()...)
	out, err := internalgit.Command(context.Background(), dir, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git add %s: %s: %w", relPath, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (RealLandingGitOps) CommitStaged(dir, msg string) (string, error) {
	out, _ := internalgit.Command(context.Background(), dir, "diff", "--cached", "--name-only").Output()
	if len(strings.TrimSpace(string(out))) == 0 {
		return "", nil
	}
	commitOut, err := internalgit.Command(context.Background(), dir,
		"-c", "user.name=ddx-land-coordinator",
		"-c", "user.email=coordinator@ddx.local",
		"commit", "-m", msg,
	).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git commit: %s: %w", strings.TrimSpace(string(commitOut)), err)
	}
	shaOut, err := internalgit.Command(context.Background(), dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("rev-parse HEAD after evidence commit: %w", err)
	}
	return strings.TrimSpace(string(shaOut)), nil
}

// DiffNumstat implements LandingGitOps.DiffNumstat.
func (RealLandingGitOps) DiffNumstat(dir, base, tip string) (string, error) {
	out, err := internalgit.Command(context.Background(), dir, "diff", "--numstat", base, tip, "--").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// DiffNameOnly implements LandingGitOps.DiffNameOnly.
func (RealLandingGitOps) DiffNameOnly(dir, base, tip string) ([]string, error) {
	out, err := internalgit.Command(context.Background(), dir, "diff", "--name-only", base, tip, "--").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("listing changed files: %s: %w", strings.TrimSpace(string(out)), err)
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if p := strings.TrimSpace(line); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// FetchOriginAncestryCheck implements LandingGitOps.FetchOriginAncestryCheck.
func (RealLandingGitOps) FetchOriginAncestryCheck(dir, targetBranch string) (PreClaimResult, error) {
	// Step 1: check for origin remote.
	out, err := internalgit.Command(context.Background(), dir, "remote", "get-url", "origin").CombinedOutput()
	if err != nil {
		// No origin remote — single-machine case; skip check.
		return PreClaimResult{Action: "no-origin"}, nil
	}
	_ = out // remote URL not needed

	// Step 2: fetch origin/targetBranch (best-effort; failure is non-fatal but surfaced).
	if fetchOut, fetchErr := internalgit.Command(context.Background(), dir, "fetch", "origin", targetBranch).CombinedOutput(); fetchErr != nil {
		return PreClaimResult{}, fmt.Errorf("git fetch origin %s: %s: %w", targetBranch, strings.TrimSpace(string(fetchOut)), fetchErr)
	}

	// Step 3: resolve local tip.
	localOut, localErr := internalgit.Command(context.Background(), dir, "rev-parse", "--verify", "refs/heads/"+targetBranch).CombinedOutput()
	if localErr != nil {
		return PreClaimResult{}, fmt.Errorf("resolving local %s: %s: %w", targetBranch, strings.TrimSpace(string(localOut)), localErr)
	}
	localSHA := strings.TrimSpace(string(localOut))

	// Step 4: resolve origin tip.
	originOut, originErr := internalgit.Command(context.Background(), dir, "rev-parse", "--verify", "refs/remotes/origin/"+targetBranch).CombinedOutput()
	if originErr != nil {
		return PreClaimResult{}, fmt.Errorf("resolving origin/%s: %s: %w", targetBranch, strings.TrimSpace(string(originOut)), originErr)
	}
	originSHA := strings.TrimSpace(string(originOut))

	// Step 5: compare.
	if localSHA == originSHA {
		return PreClaimResult{Action: "unchanged", LocalSHA: localSHA, OriginSHA: originSHA}, nil
	}

	// Is local an ancestor of origin? (origin is ahead)
	localAncestorErr := internalgit.Command(context.Background(), dir, "merge-base", "--is-ancestor", localSHA, originSHA).Run()
	if localAncestorErr == nil {
		// Origin is ahead: fast-forward local branch via update-ref + sync worktree.
		targetRef := "refs/heads/" + targetBranch
		if upErr := internalgit.Command(context.Background(), dir, "update-ref", targetRef, originSHA, localSHA).Run(); upErr != nil {
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
	originAncestorErr := internalgit.Command(context.Background(), dir, "merge-base", "--is-ancestor", originSHA, localSHA).Run()
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

// landEvidence creates a trailing commit that folds the per-attempt execution
// evidence directory into the target branch. Called after the main land (ff or
// merge) succeeds and before the push. The evidence commit is a normal child of
// result.NewTip — the agent's original commit SHA is not amended.
//
// Best-effort: only works when HEAD is a symbolic ref to targetBranch (the
// normal case for server workers and CLI users). When HEAD is detached or
// points elsewhere, the evidence commit is silently skipped.
func landEvidence(wd, targetBranch string, req LandRequest, gitOps LandingGitOps, result *LandResult) {
	branch, err := gitOps.CurrentBranch(wd)
	if err != nil || branch != targetBranch {
		return
	}
	if err := gitOps.StageDir(wd, req.EvidenceDir); err != nil {
		return
	}
	msg := fmt.Sprintf("chore: add execution evidence [%s]", shortAttempt(req.AttemptID))
	sha, err := gitOps.CommitStaged(wd, msg)
	if err != nil || sha == "" {
		return
	}
	result.EvidenceCommitSHA = sha
	result.NewTip = sha
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

	if preserved, err := preserveIfLargeDeletion(wd, req, gitOps, currentTip, contribCount); err != nil {
		return nil, err
	} else if preserved != nil {
		return preserved, nil
	}
	if preserved, err := preserveIfSyntaxSanityFails(wd, req, gitOps, currentTip, contribCount); err != nil {
		return nil, err
	} else if preserved != nil {
		return preserved, nil
	}

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
		if preserved, err := preserveIfPostLandGateFails(wd, req, gitOps, targetRef, currentTip, result.NewTip, contribCount); err != nil {
			return nil, err
		} else if preserved != nil {
			return preserved, nil
		}
		if req.EvidenceDir != "" {
			landEvidence(wd, targetBranch, req, gitOps, result)
		}
		landPush(wd, targetBranch, result.NewTip, hasOrigin, gitOps, result)
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
	if preserved, err := preserveIfPostLandGateFails(wd, req, gitOps, targetRef, currentTip, result.NewTip, contribCount); err != nil {
		return nil, err
	} else if preserved != nil {
		return preserved, nil
	}
	if req.EvidenceDir != "" {
		landEvidence(wd, targetBranch, req, gitOps, result)
	}
	landPush(wd, targetBranch, result.NewTip, hasOrigin, gitOps, result)
	return result, nil
}

func preserveIfPostLandGateFails(wd string, req LandRequest, gitOps LandingGitOps, targetRef, preLandTip, landedTip string, contribCount int) (*LandResult, error) {
	if len(req.PostLandCommand) == 0 {
		return nil, nil
	}
	output, err := runPostLandCommand(wd, req.PostLandCommand)
	if err == nil {
		return nil, nil
	}

	preserveRef := landIterationRef(req.BeadID, req.AttemptID, preLandTip)
	if upErr := gitOps.UpdateRefTo(wd, preserveRef, req.ResultRev, ""); upErr != nil {
		return nil, fmt.Errorf("preserving %s after post-land gate: %w", preserveRef, upErr)
	}
	if revertErr := gitOps.UpdateRefTo(wd, targetRef, preLandTip, landedTip); revertErr != nil {
		return nil, fmt.Errorf("restoring %s to %s after post-land gate failed: %w", targetRef, preLandTip, revertErr)
	}
	_ = gitOps.SyncWorkTreeToHead(wd, landedTip)

	reason := fmt.Sprintf("post-land gate failed: %s: %v", strings.Join(req.PostLandCommand, " "), err)
	if trimmed := strings.TrimSpace(output); trimmed != "" {
		reason += ": " + truncatePostLandGateOutput(trimmed)
	}
	return &LandResult{
		Status:            "preserved",
		PreserveRef:       preserveRef,
		Reason:            reason,
		MergedCommitCount: contribCount,
	}, nil
}

func runPostLandCommand(wd string, command []string) (string, error) {
	if len(command) == 0 {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = wd
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), ctx.Err()
	}
	return string(out), err
}

func truncatePostLandGateOutput(output string) string {
	const max = 2048
	if len(output) <= max {
		return output
	}
	return output[:max] + "...(truncated)"
}

type largeDeletionFinding struct {
	Path    string
	Deleted int
}

func preserveIfLargeDeletion(wd string, req LandRequest, gitOps LandingGitOps, currentTip string, contribCount int) (*LandResult, error) {
	threshold := largeDeletionLineThreshold(req)
	finding, found, err := largestDeletionFinding(gitOps, wd, req.BaseRev, req.ResultRev, threshold)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	ack, err := landHasLargeDeletionAcknowledgement(wd, req.BaseRev, req.ResultRev)
	if err == nil && ack {
		return nil, nil
	}

	preserveRef := landIterationRef(req.BeadID, req.AttemptID, currentTip)
	if upErr := gitOps.UpdateRefTo(wd, preserveRef, req.ResultRev, ""); upErr != nil {
		return nil, fmt.Errorf("preserving %s after large-deletion gate: %w", preserveRef, upErr)
	}
	return &LandResult{
		Status:            "preserved",
		PreserveRef:       preserveRef,
		Reason:            fmt.Sprintf("large-deletion gate: %s deleted %d lines (threshold %d) without intentional large deletion acknowledgement", finding.Path, finding.Deleted, threshold),
		MergedCommitCount: contribCount,
	}, nil
}

func largeDeletionLineThreshold(req LandRequest) int {
	if req.LargeDeletionLineThreshold > 0 {
		return req.LargeDeletionLineThreshold
	}
	return defaultLargeDeletionLineThreshold
}

func largestDeletionFinding(gitOps LandingGitOps, wd, baseRev, resultRev string, threshold int) (largeDeletionFinding, bool, error) {
	raw, err := gitOps.DiffNumstat(wd, baseRev, resultRev)
	if err != nil {
		return largeDeletionFinding{}, false, fmt.Errorf("checking large deletions: %w", err)
	}
	var largest largeDeletionFinding
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 || parts[1] == "-" {
			continue
		}
		deleted, parseErr := strconv.Atoi(parts[1])
		if parseErr != nil {
			continue
		}
		if deleted > threshold && deleted > largest.Deleted {
			largest = largeDeletionFinding{Path: parts[2], Deleted: deleted}
		}
	}
	return largest, largest.Path != "", nil
}

func landHasLargeDeletionAcknowledgement(wd, baseRev, resultRev string) (bool, error) {
	out, err := internalgit.Command(context.Background(), wd, "log", "--format=%B", baseRev+".."+resultRev).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("checking large-deletion acknowledgement: %s: %w", strings.TrimSpace(string(out)), err)
	}
	msg := strings.ToLower(string(out))
	for _, marker := range []string{
		"intentional large deletion",
		"intentional file removal",
		"intentional file deletion",
	} {
		if strings.Contains(msg, marker) {
			return true, nil
		}
	}
	return false, nil
}

type syntaxSanityFinding struct {
	Path   string
	Reason string
}

func preserveIfSyntaxSanityFails(wd string, req LandRequest, gitOps LandingGitOps, currentTip string, contribCount int) (*LandResult, error) {
	finding, found, err := syntaxSanityFindingForResult(gitOps, wd, req.BaseRev, req.ResultRev)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	preserveRef := landIterationRef(req.BeadID, req.AttemptID, currentTip)
	if upErr := gitOps.UpdateRefTo(wd, preserveRef, req.ResultRev, ""); upErr != nil {
		return nil, fmt.Errorf("preserving %s after syntax sanity gate: %w", preserveRef, upErr)
	}
	return &LandResult{
		Status:            "preserved",
		PreserveRef:       preserveRef,
		Reason:            fmt.Sprintf("syntax sanity gate: %s: %s", finding.Path, finding.Reason),
		MergedCommitCount: contribCount,
	}, nil
}

func syntaxSanityFindingForResult(gitOps LandingGitOps, wd, baseRev, resultRev string) (syntaxSanityFinding, bool, error) {
	paths, err := gitOps.DiffNameOnly(wd, baseRev, resultRev)
	if err != nil {
		return syntaxSanityFinding{}, false, err
	}
	for _, path := range paths {
		result, ok, err := gitFileAt(wd, resultRev, path)
		if err != nil {
			return syntaxSanityFinding{}, false, err
		}
		if !ok {
			continue // deleted files are handled by deletion gates, not syntax checks.
		}
		base, _, err := gitFileAt(wd, baseRev, path)
		if err != nil {
			return syntaxSanityFinding{}, false, err
		}
		if reason, bad := syntaxSanityFailure(path, base, result); bad {
			return syntaxSanityFinding{Path: path, Reason: reason}, true, nil
		}
	}
	return syntaxSanityFinding{}, false, nil
}

func gitFileAt(wd, rev, path string) ([]byte, bool, error) {
	out, err := internalgit.Command(context.Background(), wd, "show", rev+":"+path).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "does not exist") || strings.Contains(string(out), "exists on disk, but not in") {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("reading %s at %s: %s: %w", path, rev, strings.TrimSpace(string(out)), err)
	}
	return out, true, nil
}

func syntaxSanityFailure(path string, base, result []byte) (string, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		var v any
		if err := json.Unmarshal(result, &v); err != nil {
			return "invalid JSON: " + err.Error(), true
		}
	case ".go":
		if _, err := parser.ParseFile(token.NewFileSet(), path, result, parser.AllErrors); err != nil {
			return "invalid Go syntax: " + err.Error(), true
		}
	case ".svelte":
		return svelteSanityFailure(base, result)
	}
	return "", false
}

func svelteSanityFailure(base, result []byte) (string, bool) {
	baseHadScript := bytes.Contains(base, []byte("<script"))
	resultHasScript := bytes.Contains(result, []byte("<script"))
	resultClosesScript := bytes.Contains(result, []byte("</script>"))
	if baseHadScript && (!resultHasScript || !resultClosesScript) {
		return "Svelte file lost script structure", true
	}
	if resultHasScript && !resultClosesScript {
		return "Svelte script tag is not closed", true
	}
	return "", false
}

// landPush pushes the new target tip to origin when a remote exists. Push
// failure is non-fatal for the local land outcome; it is surfaced via
// PushFailed/PushError on the result.
//
// On the first push rejection landPush attempts one automatic recovery —
// fetch origin, merge our local tip into the new origin tip, advance the
// local target ref, and retry the push. The race-with-remote case (origin
// advanced with non-overlapping work between our local merge and our push)
// is the common failure in a multi-worker / multi-operator setup and can be
// fixed without operator intervention. If the auto-merge produces a conflict
// the loop cannot resolve, landPush marks the result PushConflict (a distinct
// signal from generic PushFailed) so the caller can park the bead for human
// review under a short cooldown rather than the long park used for
// non-recoverable push failures.
func landPush(wd, targetBranch, newTip string, hasOrigin bool, gitOps LandingGitOps, result *LandResult) {
	if !hasOrigin {
		return
	}
	firstErr := gitOps.PushFFOnly(wd, "origin", newTip, targetBranch)
	if firstErr == nil {
		return
	}
	// First push rejected. Try one automatic pull/merge/retry-push pass.
	if recovered, mergeSHA, recErr := landPushAutoRecover(wd, targetBranch, newTip, gitOps); recovered {
		result.NewTip = mergeSHA
		result.PushRecovered = true
		return
	} else if recErr != nil && isPushAutoMergeConflict(recErr) {
		// Auto-merge could not be resolved without operator input.
		result.PushConflict = true
		result.PushError = recErr.Error()
		return
	}
	// Either recovery itself failed for non-conflict reasons (network, auth,
	// fetch error, retry push still rejected) or origin was already at our
	// tip. Surface the original push error so operator messages match the
	// historical contract.
	result.PushFailed = true
	result.PushError = firstErr.Error()
}

// landPushAutoMergeConflictError tags a recovery error that originates from
// an unresolvable git merge conflict, so landPush can branch on it without
// pattern-matching free-form error strings.
type landPushAutoMergeConflictError struct{ inner error }

func (e *landPushAutoMergeConflictError) Error() string { return e.inner.Error() }
func (e *landPushAutoMergeConflictError) Unwrap() error { return e.inner }

func isPushAutoMergeConflict(err error) bool {
	for err != nil {
		if _, ok := err.(*landPushAutoMergeConflictError); ok {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// landPushAutoRecover attempts one fetch + merge + retry-push pass after the
// initial push was rejected. Returns recovered=true and the new merge SHA
// when the retry-push succeeds; otherwise returns recovered=false plus an
// error that indicates whether the recovery hit a merge conflict (wrapped in
// landPushAutoMergeConflictError) or some other failure.
//
// Caller responsibilities (already met by landPush):
//   - The local target ref is at newTip and not yet pushed.
//   - hasOrigin is true.
func landPushAutoRecover(wd, targetBranch, newTip string, gitOps LandingGitOps) (recovered bool, mergeSHA string, err error) {
	if fetchErr := gitOps.FetchBranch(wd, "origin", targetBranch); fetchErr != nil {
		return false, "", fmt.Errorf("auto-recovery fetch origin %s: %w", targetBranch, fetchErr)
	}
	originSHA, resolveErr := gitOps.ResolveRef(wd, "refs/remotes/origin/"+targetBranch)
	if resolveErr != nil {
		return false, "", fmt.Errorf("auto-recovery resolve origin/%s: %w", targetBranch, resolveErr)
	}
	if originSHA == newTip {
		// Origin is already at our tip — push must have failed for a reason
		// other than a non-fast-forward (auth, branch protection, large
		// blob, etc.). Recovery cannot help.
		return false, "", fmt.Errorf("origin already at local tip; push failure not recoverable by auto-merge")
	}
	tempWT, mkErr := os.MkdirTemp("", "ddx-push-recover-*")
	if mkErr != nil {
		return false, "", fmt.Errorf("auto-recovery temp worktree: %w", mkErr)
	}
	_ = os.RemoveAll(tempWT)
	if addErr := gitOps.AddWorktree(wd, tempWT, originSHA); addErr != nil {
		return false, "", fmt.Errorf("auto-recovery add worktree at %s: %w", originSHA, addErr)
	}
	defer func() { _ = gitOps.RemoveWorktree(wd, tempWT) }()

	mergeMsg := fmt.Sprintf("Merge origin/%s into %s after push race", targetBranch, targetBranch)
	if mergeErr := gitOps.MergeInto(tempWT, newTip, mergeMsg); mergeErr != nil {
		return false, "", &landPushAutoMergeConflictError{inner: mergeErr}
	}
	mergeSHA, headErr := gitOps.HeadRevAt(tempWT)
	if headErr != nil {
		return false, "", fmt.Errorf("auto-recovery read merge HEAD: %w", headErr)
	}
	targetRef := "refs/heads/" + targetBranch
	if updErr := gitOps.UpdateRefTo(wd, targetRef, mergeSHA, newTip); updErr != nil {
		return false, "", fmt.Errorf("auto-recovery advance %s to %s: %w", targetRef, mergeSHA, updErr)
	}
	_ = gitOps.SyncWorkTreeToHead(wd, newTip)
	if pushErr := gitOps.PushFFOnly(wd, "origin", mergeSHA, targetBranch); pushErr != nil {
		return false, "", fmt.Errorf("auto-recovery retry push: %w", pushErr)
	}
	return true, mergeSHA, nil
}

// LandConflictAutoRecover attempts a 3-way merge of a preserved iteration ref
// onto the current target-branch tip using the ort strategy with -X ours. The
// strategy resolves mechanical conflicts (positional drift from parallel beads)
// by favouring the current tip's version of any conflicting sections while still
// including the preserved iteration's non-conflicting changes in the merge
// commit. If the merge succeeds the local target branch is advanced and the new
// merge commit SHA is returned.
//
// A non-nil error means the ort merge failed (unresolvable content conflict or
// git error) — the caller should escalate to a focused conflict-resolve agent.
// The target branch is never modified on error.
func LandConflictAutoRecover(wd, preserveRef string, gitOps LandingGitOps) (string, error) {
	targetBranch, err := gitOps.CurrentBranch(wd)
	if err != nil {
		return "", fmt.Errorf("resolving target branch: %w", err)
	}
	targetRef := "refs/heads/" + targetBranch

	currentTip, err := gitOps.ResolveRef(wd, targetRef)
	if err != nil {
		return "", fmt.Errorf("resolving target tip %s: %w", targetRef, err)
	}

	iterSHA, err := gitOps.ResolveRef(wd, preserveRef)
	if err != nil {
		return "", fmt.Errorf("resolving preserved ref %s: %w", preserveRef, err)
	}

	tempWT, mkErr := os.MkdirTemp("", "ddx-conflict-recover-*")
	if mkErr != nil {
		return "", fmt.Errorf("creating temp worktree: %w", mkErr)
	}
	_ = os.RemoveAll(tempWT)
	if addErr := gitOps.AddWorktree(wd, tempWT, currentTip); addErr != nil {
		return "", fmt.Errorf("adding temp worktree at %s: %w", currentTip, addErr)
	}
	defer func() { _ = gitOps.RemoveWorktree(wd, tempWT) }()

	mergeMsg := fmt.Sprintf("Merge preserved iteration %s after base drift (ort -X ours)", preserveRef)
	out, mergeErr := internalgit.Command(
		context.Background(), tempWT,
		"-c", "user.name=ddx-land-coordinator",
		"-c", "user.email=coordinator@ddx.local",
		"merge", "--no-ff", "-s", "ort", "-X", "ours", "-m", mergeMsg, iterSHA,
	).CombinedOutput()
	if mergeErr != nil {
		_ = internalgit.Command(context.Background(), tempWT, "merge", "--abort").Run()
		return "", fmt.Errorf("ort merge: %s: %w", strings.TrimSpace(string(out)), mergeErr)
	}

	mergeSHA, headErr := gitOps.HeadRevAt(tempWT)
	if headErr != nil {
		return "", fmt.Errorf("reading merge HEAD: %w", headErr)
	}
	if updErr := gitOps.UpdateRefTo(wd, targetRef, mergeSHA, currentTip); updErr != nil {
		return "", fmt.Errorf("advancing %s to %s: %w", targetRef, mergeSHA, updErr)
	}
	_ = gitOps.SyncWorkTreeToHead(wd, currentTip)
	return mergeSHA, nil
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
		if land.PushConflict {
			res.Reason = PushConflictReasonPrefix + " " + land.PushError
		} else if land.PushFailed {
			res.Reason = PushFailedReasonPrefix + " " + land.PushError
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
		EvidenceDir:  res.ExecutionDir,
	}
}
