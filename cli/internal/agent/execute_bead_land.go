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
//   1. Read the current target tip from the local branch.
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
//   5. Finalize the local-only execution evidence without staging or committing
//      it, then sync the operator checkout to the new head when safe to do so.
//
// Network sync is intentionally out of scope for Land(): no fetch and no
// push happen here. Operators sync origin with raw git commands after the
// drain session completes.
//
// Replay fidelity is the reason for merge-over-rebase: a rebased commit has
// a rewritten parent that lies about what the worker saw at execution time.
// A merge commit preserves both histories — the worker's original commit
// (parent = BaseRev) and the target's prior tip — losslessly.
//
// The in-process coordinator serializes submissions inside one process. Land()
// also takes the process-shared main-git lock so separate `ddx work` processes
// Land narrows the process-shared main-git lock to the shared-ref update
// boundary so separate `ddx work` processes cannot interleave target-ref
// writes while read-only prep stays outside the lock and post-land checkout
// sync runs under the same lock acquisition.

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

	"github.com/DocumentDrivenDX/ddx/internal/config"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/trackerpaths"
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
	// non-empty and the main land succeeds, Land() uses a temporary copy while
	// finalizing the result. The canonical project-root bundle remains local
	// and uncommitted; the temporary finalization worktree is removed afterward.
	EvidenceDir string

	// PostLandCommand is an optional project verification command run after
	// Land() advances the local target ref and syncs the worktree, but before
	// local evidence finalization or checkout sync. A failure restores the target
	// ref to its pre-land SHA and preserves ResultRev under refs/ddx/iterations.
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

	// LandedTip is the target branch tip immediately after the implementation
	// lands. It matches NewTip for current results.
	LandedTip string

	// TargetBranch is the resolved branch name that Land() advanced or attempted
	// to advance. It is set on landed and preserved results so callers can make
	// branch-local recovery explicit in terminal output and evidence.
	TargetBranch string

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
	// Status == "preserved" (e.g. "merge conflict").
	Reason string

	// MergedCommitCount is the number of commits reachable from ResultRev but
	// not from BaseRev — i.e. the "size" of the worker's contribution being
	// merged in. Zero on the no-changes path. Set on both the fast-forward
	// and merge-commit paths so metrics can compare contribution sizes.
	MergedCommitCount int

	// EvidenceCommitSHA is retained only for decoding legacy result shapes.
	// Current landing code never assigns it because execution evidence is local.
	EvidenceCommitSHA string

	// CheckoutSyncDeferred is true when the target ref advanced but DDx left
	// the operator checkout files untouched because dirty local paths overlap
	// files changed by the new target tip.
	CheckoutSyncDeferred bool

	// CheckoutSyncDeferredPaths lists the dirty paths that caused checkout sync
	// to be skipped.
	CheckoutSyncDeferredPaths []string
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

// IsIgnorableFetchOriginError reports whether a pre-claim ancestry-check
// failure came from the best-effort network fetch. Local worktree safety
// failures must propagate so workers do not claim work from an unsafe trunk.
func IsIgnorableFetchOriginError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.HasPrefix(msg, "git fetch origin ") || strings.HasPrefix(msg, "git fetch origin:")
}

// LandingGitOps abstracts the git operations Land() needs. RealLandingGitOps
// shells out to git; tests inject fakes or run against real temp repos.
type LandingGitOps interface {
	// CurrentBranch returns the branch HEAD currently points at in dir, or
	// an error if HEAD is detached or unresolvable.
	CurrentBranch(dir string) (string, error)

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

	// AddBranchWorktree creates a new worktree at path checked out on branch
	// in dir. It is used for clean landing finalization when the operator
	// checkout already has the target branch checked out.
	AddBranchWorktree(dir, path, branch string) error

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

	// CountCommits returns the number of commits reachable from tip but not
	// from base (i.e. git rev-list --count base..tip). Used to record the
	// contribution size in land metrics. Returns 0 on error.
	CountCommits(dir, base, tip string) int

	// VerifyCandidateHistory rejects local-only execution evidence in any
	// candidate commit. RealLandingGitOps performs the production history scan;
	// synthetic Git implementations provide their own revision semantics.
	VerifyCandidateHistory(dir, base, tip string) error

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

func (RealLandingGitOps) CurrentBranch(dir string) (string, error) {
	out, err := internalgit.Command(context.Background(), dir, "symbolic-ref", "--short", "HEAD").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git symbolic-ref HEAD: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (RealLandingGitOps) ResolveRef(dir, ref string) (string, error) {
	out, err := internalgit.Command(context.Background(), dir, "rev-parse", "--verify", ref).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (RealLandingGitOps) UpdateRefTo(dir, ref, sha, oldSHA string) error {
	if ref == "HEAD" {
		return fmt.Errorf("refusing to update HEAD directly; landing must target a concrete ref")
	}
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

// alwaysSkipSyncPaths lists live-state files governed by the tracker backend.
// The committed worktree version is a stale snapshot of tracker state at claim
// time; the main-worktree copy is always authoritative and must never be
// overwritten by SyncWorkTreeToHead regardless of dirty-before-land state.
var alwaysSkipSyncPaths = []string{
	".ddx/beads.jsonl",
	".ddx/beads-archive.jsonl",
}

func (RealLandingGitOps) SyncWorkTreeToHead(dir, fromRev string) error {
	return syncWorkTreeToHeadExcludingPaths(dir, fromRev, alwaysSkipSyncPaths)
}

// readTreeHeadWithRetry runs `git read-tree HEAD` in dir, retrying with
// exponential backoff when the git index lock is held by a concurrent process.
//
// Background (ddx-7e659c95): git read-tree needs to acquire .git/index.lock.
// When an operator git command (git status, git commit, etc.) holds the lock
// concurrently, git exits 128 with "Unable to create index.lock: File exists".
// Without retry the main worktree index stays dirty after a merge landing,
// causing a subsequent `git commit beads.jsonl` to sweep in phantom reverts of
// the bead's changes.
//
// The retry budget (30 s, 100 ms → 1 s backoff) matches the
// DefaultLockRetryPolicy used by withTrackerLock so contention handling is
// consistent across all ddx git-index operations.
func readTreeHeadWithRetry(dir string) ([]byte, error) {
	const (
		initialBackoff = 100 * time.Millisecond
		maxBackoff     = 1 * time.Second
		maxElapsed     = 30 * time.Second
	)
	start := time.Now()
	backoff := initialBackoff
	var lastDiag string
	for {
		out, err := internalgit.Command(context.Background(), dir, "read-tree", "HEAD").CombinedOutput()
		if err == nil {
			return out, nil
		}
		// Only retry on index lock contention; all other errors are fatal.
		if !strings.Contains(string(out), "index.lock") {
			return out, err
		}
		// Identify the lock owner and break the lock if it is stale or
		// owned by a dead process. This converts a hard 30s wait into an
		// instant recovery for the common crashed-git case.
		if result, recErr := recoverGitIndexLock(dir); recErr == nil {
			lastDiag = result.Reason
			if result.Removed {
				// Lock cleared — retry immediately.
				continue
			}
		}
		if time.Since(start) >= maxElapsed {
			if lastDiag != "" {
				return out, fmt.Errorf("%s; index-lock owner: %s: %w",
					strings.TrimSpace(string(out)), lastDiag, err)
			}
			return out, err
		}
		time.Sleep(backoff)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func syncWorkTreeToHeadExcludingPaths(dir, fromRev string, skipPaths []string) error {
	skipWorktreePaths, err := checkpointSkipWorktreePaths(dir)
	if err != nil {
		return err
	}

	// Step 1: sync the index to HEAD. This is required before checkout-index
	// below will do anything useful, and also keeps subsequent CommitTracker
	// calls from building stale trees.
	//
	// ddx-7e659c95: git read-tree requires .git/index.lock. If an operator
	// git command (git status, git commit, etc.) holds the lock concurrently,
	// git read-tree exits 128 with "Unable to create index.lock: File exists".
	// Retry with exponential backoff for up to 30 s so transient operator
	// contention does not leave the main worktree index dirty.
	out, err := readTreeHeadWithRetry(dir)
	if err != nil {
		return fmt.Errorf("git read-tree HEAD: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if err := restoreCheckpointSkipWorktreePaths(dir, skipWorktreePaths); err != nil {
		return err
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
	skip := map[string]bool{}
	for _, path := range skipPaths {
		skip[filepath.ToSlash(path)] = true
	}
	for _, path := range skipWorktreePaths {
		skip[filepath.ToSlash(path)] = true
	}
	// blobAt returns the blob hash of path at rev, or "" when absent.
	blobAt := func(rev, path string) string {
		out, err := internalgit.Command(context.Background(), dir, "rev-parse", rev+":"+path).CombinedOutput()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	// worktreeBlob returns the blob hash of the file on disk, or "" when absent.
	worktreeBlob := func(path string) string {
		if _, statErr := os.Stat(filepath.Join(dir, path)); statErr != nil {
			return ""
		}
		out, err := internalgit.Command(context.Background(), dir, "hash-object", "--", path).CombinedOutput()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	var indexFiles []string
	var removedFiles []string
	for _, f := range changed {
		if skip[filepath.ToSlash(f)] {
			continue
		}
		// Local work is never overwritten: only restore a path whose
		// on-disk content is provably STALE (matches fromRev) or already
		// current (matches HEAD). Anything else is a local modification
		// that raced in during the land — leave it; the next land's
		// pre-land checkpoint commits it. Recomputed per file here, not
		// from a claim-time dirty list, because that list misses edits
		// made while the attempt ran (observed live: operator edit
		// clobbered by checkout-index -f, 2026-07-06 on eitri).
		wtHash := worktreeBlob(f)
		fromHash := blobAt(fromRev, f)
		probe := internalgit.Command(context.Background(), dir, "ls-files", "--error-unmatch", "--", f)
		if probe.Run() == nil {
			headHash := blobAt("HEAD", f)
			switch wtHash {
			case headHash:
				// Already current — nothing to do.
			case fromHash:
				// Stale (including absent-in-both) — safe to materialize.
				indexFiles = append(indexFiles, f)
			default:
				// Locally modified (or locally created/deleted vs
				// fromRev) — preserve.
			}
		} else {
			if wtHash != "" && wtHash == fromHash {
				removedFiles = append(removedFiles, f)
			}
			// Locally modified copies of a HEAD-deleted file are preserved.
		}
	}

	// Step 4: materialize the index-present files into the working tree.
	// -f overwrites any stale content at these exact paths. Unrelated files
	// are untouched because we pass the specific path list.
	if len(indexFiles) > 0 {
		for _, f := range indexFiles {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755); err != nil {
				return fmt.Errorf("creating checkout parent for %s: %w", f, err)
			}
		}
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

func syncWorkTreeToHeadGuarded(gitOps LandingGitOps, dir, fromRev string, dirtyBefore []string, result *LandResult) {
	overlap, err := checkoutSyncDirtyOverlapPaths(dir, fromRev, dirtyBefore)
	if err == nil && len(overlap) > 0 {
		if result != nil {
			result.CheckoutSyncDeferred = true
			result.CheckoutSyncDeferredPaths = appendUniqueStrings(result.CheckoutSyncDeferredPaths, overlap...)
		}
		_ = syncWorkTreeToHeadExcludingPaths(dir, fromRev, overlap)
		return
	}
	_ = gitOps.SyncWorkTreeToHead(dir, fromRev)
}

// checkpointLandingWorktreeLocalChanges commits any tracked local
// modifications (staged or unstaged) in dir as a checkpoint commit. Local
// work is never reset, refused, or deferred at the land boundary — it is
// committed onto the current tip BEFORE the land advances the ref, while
// HEAD, index, and working tree still agree (committing after update-ref
// would snapshot a stale index and produce phantom reverts). Managed tracker
// paths stay uncommitted: they are WithLock-owned live state with their own
// commit machinery. Returns whether a checkpoint commit was created.
func checkpointLandingWorktreeLocalChanges(dir, reason string) (bool, error) {
	out, err := internalgit.Command(context.Background(), dir, "status", "--porcelain", "--untracked-files=no").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("checkpoint status: %s: %w", strings.TrimSpace(string(out)), err)
	}
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 4 {
			continue
		}
		p := strings.TrimSpace(line[3:])
		// Rename entries are "R  old -> new"; commit both sides.
		if idx := strings.Index(p, " -> "); idx >= 0 {
			for _, side := range []string{p[:idx], p[idx+4:]} {
				if side = strings.TrimSpace(side); side != "" && !skipLandingCheckpointPath(side) {
					paths = append(paths, side)
				}
			}
			continue
		}
		if p == "" || skipLandingCheckpointPath(p) {
			continue
		}
		paths = append(paths, p)
	}
	if len(paths) == 0 {
		return false, nil
	}
	// Discriminate genuine local work from STALE content. A sibling land
	// can move the ref before this land's post-lock sync runs, making every
	// file it touched look dirty here; committing that state would create
	// phantom reverts. A file whose on-disk blob matches the blob at any
	// recent first-parent ancestor is stale (the sync will repair it), not
	// local work — skip it. Content matching no recent ancestor is real
	// local modification and is committed.
	revsOut, revsErr := internalgit.Command(context.Background(), dir, "rev-list", "--first-parent", "-20", "HEAD").CombinedOutput()
	if revsErr == nil {
		revs := strings.Fields(strings.TrimSpace(string(revsOut)))
		var genuine []string
		for _, path := range paths {
			wtOut, hashErr := internalgit.Command(context.Background(), dir, "hash-object", "--", path).CombinedOutput()
			if hashErr != nil {
				// Deleted or unreadable on disk — deletion of a tracked
				// file is local work; keep it.
				genuine = append(genuine, path)
				continue
			}
			wtHash := strings.TrimSpace(string(wtOut))
			stale := false
			for _, rev := range revs {
				blobOut, blobErr := internalgit.Command(context.Background(), dir, "rev-parse", rev+":"+path).CombinedOutput()
				if blobErr == nil && strings.TrimSpace(string(blobOut)) == wtHash {
					stale = true
					break
				}
			}
			if !stale {
				genuine = append(genuine, path)
			}
		}
		paths = genuine
	}
	if len(paths) == 0 {
		return false, nil
	}
	// Concurrent lands in the same checkout race this section: another
	// land's sync can consume the dirt between our status snapshot and the
	// commit. Tolerate paths vanishing (--ignore-errors) and an
	// emptied-out commit — "nothing left to checkpoint" is success.
	addArgs := append([]string{"add", "--ignore-errors", "--"}, paths...)
	if out, err := internalgit.Command(context.Background(), dir, addArgs...).CombinedOutput(); err != nil {
		lower := strings.ToLower(string(out))
		if !strings.Contains(lower, "did not match any files") && !strings.Contains(lower, "pathspec") {
			return false, fmt.Errorf("checkpoint add: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}
	msg := fmt.Sprintf("chore: checkpoint local tree before land (%s)", reason)
	// Commit ONLY the checkpointed paths: other index entries (notably
	// pre-staged tracker files awaiting their own durable-audit commit)
	// must stay staged, not be swept into the checkpoint.
	commitArgs := []string{"-c", "user.name=ddx-land-checkpoint", "-c", "user.email=land-checkpoint@ddx.local", "commit", "--no-verify", "-m", msg, "--"}
	commitArgs = append(commitArgs, paths...)
	if out, err := internalgit.Command(context.Background(), dir, commitArgs...).CombinedOutput(); err != nil {
		lower := strings.ToLower(string(out))
		if strings.Contains(lower, "nothing to commit") || strings.Contains(lower, "nothing added to commit") || strings.Contains(lower, "no changes added to commit") {
			return false, nil
		}
		return false, fmt.Errorf("checkpoint commit: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return true, nil
}

func skipLandingCheckpointPath(path string) bool {
	clean := strings.TrimSpace(filepath.ToSlash(path))
	if clean == "" {
		return false
	}
	if trackerpaths.IsManagedTrackerPath(clean) {
		return true
	}
	return isExecutionEvidencePath(clean) || isLockMetricsPath(clean)
}

func ensureLandingWorktreeReady(dir, targetBranch string) error {
	return ensureLandingWorktreeReadyWithLockState(dir, targetBranch, false)
}

// ensureLandingWorktreeReadyWithLockState validates the landing checkout and
// recovers a staged index. mainGitLockHeld must be true when the caller already
// owns withMainGitLock; recovery then runs directly instead of recursively
// acquiring the non-reentrant process-shared lock.
func ensureLandingWorktreeReadyWithLockState(dir, targetBranch string, mainGitLockHeld bool) error {
	if !isInsideGitWorktree(dir) {
		return nil
	}

	branchOut, err := internalgit.Command(context.Background(), dir, "branch", "--show-current").CombinedOutput()
	if err != nil {
		return fmt.Errorf("checking landing branch: %s: %w", strings.TrimSpace(string(branchOut)), err)
	}
	branch := strings.TrimSpace(string(branchOut))
	if branch == "" {
		return fmt.Errorf("landing worktree is detached; expected branch %q", targetBranch)
	}
	if targetBranch != "" && branch != targetBranch {
		return fmt.Errorf("landing branch mismatch: on %q, want %q", branch, targetBranch)
	}
	return waitForEmptyGitIndexWithLockState(dir, 2*time.Second, mainGitLockHeld)
}

func isInsideGitWorktree(dir string) bool {
	out, err := internalgit.Command(context.Background(), dir, "rev-parse", "--is-inside-work-tree").CombinedOutput()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func waitForEmptyGitIndex(dir string, timeout time.Duration) error {
	return waitForEmptyGitIndexWithLockState(dir, timeout, false)
}

func waitForEmptyGitIndexWithLockState(dir string, timeout time.Duration, mainGitLockHeld bool) error {
	deadline := time.Now().Add(timeout)
	repairedCorruptIndex := false
	for {
		out, err := internalgit.Command(context.Background(), dir, "diff", "--cached", "--quiet").CombinedOutput()
		if err == nil {
			return nil
		}
		// There are staged changes (or a corrupt index). DDx-managed tracker
		// files (.ddx/beads.jsonl, .ddx/metrics/attempts.jsonl, …) are
		// append-mostly metadata that concurrent workers rewrite continuously;
		// a short wait for them to settle reliably fails on a busy multi-worker
		// host and wedges the queue (ddx-df77e668). They are not code, and the
		// next claim rewrites them anyway, so they must never block a claim.
		// When the only staged paths are tracker files, the landing worktree is
		// clean for claim purposes.
		if blocking, ok := blockingStagedPaths(dir); ok && len(blocking) == 0 {
			return nil
		}
		if !repairedCorruptIndex && isRecoverableLandingIndexCorruption(string(out)) {
			recovery, err := recoverLandingIndex(dir, mainGitLockHeld, &repairedCorruptIndex)
			if err != nil {
				return err
			}
			if recovery.clean {
				return nil
			}
			if recovery.progressed {
				continue
			}
		}
		if time.Now().After(deadline) {
			recovery, err := recoverLandingIndex(dir, mainGitLockHeld, &repairedCorruptIndex)
			if err != nil {
				return err
			}
			if recovery.clean {
				return nil
			}
			if recovery.progressed {
				continue
			}
			stagedOut, _ := internalgit.Command(context.Background(), dir, "diff", "--cached", "--name-status").CombinedOutput()
			staged := strings.TrimSpace(string(stagedOut))
			if staged == "" {
				staged = "unknown staged changes"
			}
			return fmt.Errorf("landing worktree has staged changes after waiting %s:\n%s", timeout, staged)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

type landingIndexRecovery struct {
	clean      bool
	progressed bool
}

func recoverLandingIndex(dir string, mainGitLockHeld bool, repairedCorruptIndex *bool) (landingIndexRecovery, error) {
	if mainGitLockHeld {
		return recoverLandingIndexLocked(dir, repairedCorruptIndex)
	}

	var recovery landingIndexRecovery
	err := withMainGitLock(dir, "landing_index_recovery", func() error {
		var recoveryErr error
		recovery, recoveryErr = recoverLandingIndexLocked(dir, repairedCorruptIndex)
		return recoveryErr
	})
	return recovery, err
}

// recoverLandingIndexLocked performs one short mutation pass after the
// read-only wait budget expires. The caller holds the process-shared main-Git
// lock, so read-tree, reset, and checkpoint commits cannot race another land's
// ref/index synchronization.
func recoverLandingIndexLocked(dir string, repairedCorruptIndex *bool) (landingIndexRecovery, error) {
	if err := internalgit.Command(context.Background(), dir, "diff", "--cached", "--quiet").Run(); err == nil {
		return landingIndexRecovery{clean: true}, nil
	}
	if repairedCorruptIndex != nil && !*repairedCorruptIndex {
		out, _ := internalgit.Command(context.Background(), dir, "diff", "--cached", "--quiet").CombinedOutput()
		if isRecoverableLandingIndexCorruption(string(out)) {
			*repairedCorruptIndex = true
			if _, err := readTreeHeadWithRetry(dir); err != nil {
				return landingIndexRecovery{}, fmt.Errorf("repairing landing worktree index: %w", err)
			}
			if err := internalgit.Command(context.Background(), dir, "diff", "--cached", "--quiet").Run(); err == nil {
				return landingIndexRecovery{clean: true, progressed: true}, nil
			}
		}
	}

	// A prior land can leave the index at a recent ancestor. Re-reading HEAD is
	// safe only for that exact signature.
	if matches, _ := indexMatchesRecentAncestorTree(dir, 20); matches {
		if _, err := readTreeHeadWithRetry(dir); err == nil {
			if err := internalgit.Command(context.Background(), dir, "diff", "--cached", "--quiet").Run(); err == nil {
				return landingIndexRecovery{clean: true, progressed: true}, nil
			}
		}
	}

	// An all-evidence staged set is local state: remove it only from the index
	// and preserve the bytes. Mixed/code sets fall through to the real-work
	// checkpoint, which explicitly excludes execution evidence.
	if unstageOrphanedExecutionEvidence(dir) {
		clean := internalgit.Command(context.Background(), dir, "diff", "--cached", "--quiet").Run() == nil
		return landingIndexRecovery{clean: clean, progressed: true}, nil
	}
	committed, err := checkpointLandingWorktreeLocalChanges(dir, "staged at land boundary")
	if err != nil {
		return landingIndexRecovery{}, err
	}
	return landingIndexRecovery{progressed: committed}, nil
}

func isRecoverableLandingIndexCorruption(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "index file smaller than expected") ||
		strings.Contains(lower, "bad index file") ||
		strings.Contains(lower, "unexpected end of file while reading index")
}

// isExecutionEvidencePath reports whether a repo-relative (forward-slash) path
// is DDx per-machine execution evidence under .ddx/executions/.
func isExecutionEvidencePath(path string) bool {
	p := strings.TrimSpace(path)
	return p == ".ddx/executions" || strings.HasPrefix(p, ".ddx/executions/")
}

// unstageOrphanedExecutionEvidence unstages staged .ddx/executions/* evidence
// when that is the ONLY thing staged in dir, so orphaned per-machine evidence
// left by a dead/interrupted attempt cannot jam the landing-index guard. It
// returns true when it unstaged something (the caller re-checks the index).
// A staged set containing any non-execution-evidence path is left untouched so
// real staged work is never silently discarded. Unstaging (rather than
// committing) keeps execution dirs out of git history; the durable audit trail
// lives in .ddx/attachments. See ddx-2ab14458.
func unstageOrphanedExecutionEvidence(dir string) bool {
	out, err := internalgit.Command(context.Background(), dir, "diff", "--cached", "--name-only").CombinedOutput()
	if err != nil {
		return false
	}
	var staged []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if p := strings.TrimSpace(line); p != "" {
			staged = append(staged, p)
		}
	}
	if len(staged) == 0 {
		return false
	}
	for _, p := range staged {
		if !isExecutionEvidencePath(p) {
			return false
		}
	}
	if err := internalgit.Command(context.Background(), dir, "reset", "-q", "HEAD", "--", ".ddx/executions").Run(); err != nil {
		return false
	}
	return true
}

// blockingStagedPaths returns the staged paths that would genuinely block a
// claim — i.e. every staged path that is NOT a DDx-managed tracker/metadata
// file. ok is false when the staged list cannot be read (e.g. a corrupt
// index), so callers can fall through to their corruption-recovery path
// instead of mistaking the read failure for "no blocking changes".
func blockingStagedPaths(dir string) (blocking []string, ok bool) {
	out, err := internalgit.Command(context.Background(), dir, "diff", "--cached", "--name-only").CombinedOutput()
	if err != nil {
		return nil, false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		p := strings.TrimSpace(line)
		if p == "" || trackerpaths.IsManagedTrackerPath(p) {
			continue
		}
		blocking = append(blocking, p)
	}
	return blocking, true
}

// indexMatchesRecentAncestorTree reports whether the current index's tree
// matches the tree of any of the last `maxDepth` ancestors of HEAD. A
// match means the index reflects a state we have already advanced past —
// the signature of a swallowed SyncWorkTreeToHead failure — and is
// therefore safe to overwrite by re-reading HEAD into the index.
//
// Operator-staged work introduces novel tree contents that will not
// match any historical tree, so the heuristic preserves their work.
func indexMatchesRecentAncestorTree(dir string, maxDepth int) (bool, error) {
	out, err := internalgit.Command(context.Background(), dir, "write-tree").Output()
	if err != nil {
		return false, err
	}
	indexTree := strings.TrimSpace(string(out))
	if indexTree == "" {
		return false, nil
	}
	for depth := 1; depth <= maxDepth; depth++ {
		ref := fmt.Sprintf("HEAD~%d^{tree}", depth)
		treeOut, treeErr := internalgit.Command(context.Background(), dir, "rev-parse", "--verify", ref).Output()
		if treeErr != nil {
			return false, nil
		}
		if strings.TrimSpace(string(treeOut)) == indexTree {
			return true, nil
		}
	}
	return false, nil
}

func checkoutSyncDirtyOverlapPaths(dir, fromRev string, dirtyPaths []string) ([]string, error) {
	if fromRev == "" || len(dirtyPaths) == 0 {
		return nil, nil
	}
	diffOut, diffErr := internalgit.Command(context.Background(), dir, "diff", "--name-only", fromRev, "HEAD").CombinedOutput()
	if diffErr != nil {
		return nil, diffErr
	}
	changed := map[string]bool{}
	for _, path := range strings.Fields(strings.TrimSpace(string(diffOut))) {
		changed[filepath.ToSlash(path)] = true
	}
	if len(changed) == 0 {
		return nil, nil
	}
	var overlap []string
	for _, path := range dirtyPaths {
		slashPath := filepath.ToSlash(path)
		if checkoutSyncDeferralIgnoredPath(slashPath) {
			continue
		}
		if changed[slashPath] {
			overlap = append(overlap, slashPath)
		}
	}
	return overlap, nil
}

func checkoutSyncDeferralIgnoredPath(path string) bool {
	if strings.HasPrefix(path, ".ddx/executions/") ||
		strings.HasPrefix(path, ".ddx/runs/") ||
		strings.HasPrefix(path, ".ddx/backups/") ||
		strings.HasPrefix(path, ".ddx/run-state/") ||
		strings.HasPrefix(path, ".ddx/.git-tracker.lock/") ||
		(strings.HasPrefix(path, ".ddx/.git-tracker.lock.") && strings.HasSuffix(path, ".lock")) {
		return true
	}
	return path == ".ddx/run-state.json" || path == ExecutionCleanupMetadataFileName
}

func appendUniqueStrings(existing []string, additions ...string) []string {
	seen := make(map[string]bool, len(existing)+len(additions))
	for _, value := range existing {
		seen[value] = true
	}
	for _, value := range additions {
		if seen[value] {
			continue
		}
		seen[value] = true
		existing = append(existing, value)
	}
	return existing
}

func (RealLandingGitOps) AddWorktree(dir, path, rev string) error {
	// --detach so the worktree does not create a persistent branch.
	out, err := internalgit.Command(context.Background(), dir, "worktree", "add", "--force", "--detach", path, rev).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add %s %s: %s: %w", path, rev, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (RealLandingGitOps) AddBranchWorktree(dir, path, branch string) error {
	out, err := internalgit.Command(context.Background(), dir, "worktree", "add", "--force", path, branch).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add %s %s: %s: %w", path, branch, strings.TrimSpace(string(out)), err)
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

func (RealLandingGitOps) VerifyCandidateHistory(dir, base, tip string) error {
	return VerifyCandidateHasNoExecutionEvidence(dir, base, tip)
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

// FetchOriginAncestryCheck performs an origin ancestry check that first fetches
// origin/<targetBranch>. Because it performs network I/O it MUST NOT be called
// from the network-free `ddx work` drain loop (reliability principle P9,
// docs/helix/06-iterate/reliability-principles.md). The drain-loop and `ddx try`
// pre-claim hooks call LocalAncestryCheck instead; this fetch variant is
// reserved for the operator-driven refresh path (FEAT-023 `ddx sync`).
func (RealLandingGitOps) FetchOriginAncestryCheck(dir, targetBranch string) (PreClaimResult, error) {
	var result PreClaimResult
	err := withMainGitLock(dir, "ancestry_fetch", func() error {
		var checkErr error
		result, checkErr = fetchOriginAncestryCheckLocked(dir, targetBranch)
		return checkErr
	})
	return result, err
}

// LocalAncestryCheck compares the local target branch against the last-observed
// origin remote-tracking ref (refs/remotes/origin/<targetBranch>) WITHOUT any
// network I/O. It is the network-free pre-claim check used by the `ddx work`
// drain loop and `ddx try` (reliability principle P9: `git fetch` is never
// called from the queue loop). Refreshing the remote-tracking ref is the
// operator's responsibility via FEAT-023 `ddx sync`. When no origin remote is
// configured, or no remote-tracking ref has been observed yet, the check
// fails open (Action=="no-origin") rather than wedging the queue.
func (RealLandingGitOps) LocalAncestryCheck(dir, targetBranch string) (PreClaimResult, error) {
	return localAncestryCheckLocked(dir, targetBranch)
}

func fetchOriginAncestryCheckLocked(dir, targetBranch string) (PreClaimResult, error) {
	if err := ensureLandingWorktreeReadyWithLockState(dir, targetBranch, true); err != nil {
		return PreClaimResult{}, err
	}

	// Check for origin remote.
	if _, err := internalgit.Command(context.Background(), dir, "remote", "get-url", "origin").CombinedOutput(); err != nil {
		// No origin remote — single-machine case; skip check.
		return PreClaimResult{Action: "no-origin"}, nil
	}

	// Fetch origin/targetBranch (best-effort; failure is non-fatal but surfaced).
	if fetchOut, fetchErr := internalgit.Command(context.Background(), dir, "fetch", "origin", targetBranch).CombinedOutput(); fetchErr != nil {
		if trimmed := strings.TrimSpace(string(fetchOut)); trimmed != "" {
			return PreClaimResult{}, fmt.Errorf("git fetch origin %s: %s: %w", targetBranch, trimmed, fetchErr)
		}
		return PreClaimResult{}, fmt.Errorf("git fetch origin %s: %w", targetBranch, fetchErr)
	}

	return compareLocalAgainstOriginTracking(dir, targetBranch)
}

func localAncestryCheckLocked(dir, targetBranch string) (PreClaimResult, error) {
	if err := ensureLandingWorktreeReady(dir, targetBranch); err != nil {
		return PreClaimResult{}, err
	}

	// Check for origin remote.
	if _, err := internalgit.Command(context.Background(), dir, "remote", "get-url", "origin").CombinedOutput(); err != nil {
		// No origin remote — single-machine case; skip check.
		return PreClaimResult{Action: "no-origin"}, nil
	}

	// No last-observed origin tip yet (never synced) — skip rather than wedge
	// the queue. FEAT-023 `ddx sync` populates the remote-tracking ref.
	if err := internalgit.Command(context.Background(), dir, "rev-parse", "--verify", "refs/remotes/origin/"+targetBranch).Run(); err != nil {
		return PreClaimResult{Action: "no-origin"}, nil
	}

	return compareLocalAgainstOriginTracking(dir, targetBranch)
}

// compareLocalAgainstOriginTracking resolves refs/heads/<targetBranch> and the
// last-observed refs/remotes/origin/<targetBranch> and reports their ancestry
// relationship. It performs no network I/O. The caller must hold the main git
// lock and must have already verified that an origin remote exists.
func compareLocalAgainstOriginTracking(dir, targetBranch string) (PreClaimResult, error) {
	// Resolve local tip.
	localOut, localErr := internalgit.Command(context.Background(), dir, "rev-parse", "--verify", "refs/heads/"+targetBranch).CombinedOutput()
	if localErr != nil {
		return PreClaimResult{}, fmt.Errorf("resolving local %s: %s: %w", targetBranch, strings.TrimSpace(string(localOut)), localErr)
	}
	localSHA := strings.TrimSpace(string(localOut))

	// Resolve last-observed origin tip from the remote-tracking ref.
	originOut, originErr := internalgit.Command(context.Background(), dir, "rev-parse", "--verify", "refs/remotes/origin/"+targetBranch).CombinedOutput()
	if originErr != nil {
		return PreClaimResult{}, fmt.Errorf("resolving origin/%s: %s: %w", targetBranch, strings.TrimSpace(string(originOut)), originErr)
	}
	originSHA := strings.TrimSpace(string(originOut))

	// Compare.
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

func pinPreserveRef(gitOps LandingGitOps, wd string, req LandRequest, tip string) (string, error) {
	preserveRef := landIterationRef(req.BeadID, req.AttemptID, tip)
	if err := gitOps.UpdateRefTo(wd, preserveRef, req.ResultRev, ""); err != nil {
		return preserveRef, err
	}
	return preserveRef, nil
}

func buildPreservedResult(req LandRequest, preserveRef, reason string, contribCount int) *LandResult {
	return &LandResult{
		Status:            "preserved",
		PreserveRef:       preserveRef,
		Reason:            reason,
		TargetBranch:      req.TargetBranch,
		MergedCommitCount: contribCount,
	}
}

// prepareLandEvidence copies the execution evidence into the temporary landing
// worktree and rewrites the result artifact there. It runs outside the main-git
// lock so large evidence directories do not hold the shared lock while the
// filesystem work is in progress. The canonical project-root bundle remains
// untouched and the temporary copy is removed with the finalization worktree.
func prepareLandEvidence(projectRoot, wd string, req LandRequest, gitOps LandingGitOps, result *LandResult) error {
	if req.EvidenceDir == "" {
		return nil
	}
	if err := copyEvidenceDirForLanding(projectRoot, wd, req.EvidenceDir); err != nil {
		return fmt.Errorf("copying evidence into landing worktree: %w", err)
	}
	if err := rewriteFinalResultArtifactForLand(wd, req, result); err != nil {
		return fmt.Errorf("rewriting final result artifact: %w", err)
	}
	// Execution evidence is per-machine and must never be committed (ddx-d10073a8):
	// it is copied onto disk for landing/review to read, but NOT staged, so it
	// never rides a commit into the durable branch.
	return nil
}

func rewriteFinalResultArtifactForLand(wd string, req LandRequest, land *LandResult) error {
	if req.EvidenceDir == "" || land == nil {
		return nil
	}
	resultPath := filepath.Join(wd, filepath.FromSlash(req.EvidenceDir), "result.json")
	raw, err := os.ReadFile(resultPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read result.json: %w", err)
	}

	var res ExecuteBeadResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return fmt.Errorf("parse result.json: %w", err)
	}
	if res.BeadID == "" {
		res.BeadID = req.BeadID
	}
	if res.AttemptID == "" {
		res.AttemptID = req.AttemptID
	}
	if res.BaseRev == "" {
		res.BaseRev = req.BaseRev
	}
	if res.ExecutionDir == "" {
		res.ExecutionDir = filepath.ToSlash(req.EvidenceDir)
	}
	if res.ResultFile == "" {
		res.ResultFile = filepath.ToSlash(filepath.Join(req.EvidenceDir, "result.json"))
	}
	if res.ResultRev == "" {
		if res.ImplementationRev != "" {
			res.ResultRev = res.ImplementationRev
		} else {
			res.ResultRev = req.ResultRev
		}
	}
	if res.ImplementationRev == "" && res.ResultRev != "" {
		res.ImplementationRev = res.ResultRev
	}
	// Legacy result files could record the trailing evidence revision in the
	// request revision. Preserve that shape when rewriting old on-disk bundles;
	// current landing requests never create or assign an evidence revision.
	if res.EvidenceRev == "" &&
		req.ResultRev != "" &&
		res.ImplementationRev != "" &&
		req.ResultRev != res.ImplementationRev {
		res.EvidenceRev = req.ResultRev
	}

	ApplyLandResultToExecuteBeadResult(&res, land)
	return writeArtifactJSON(resultPath, &res)
}

// evidenceDirHasTrackedFiles reports whether legacy history already tracks
// files under dirRel. Local publication must not overwrite such a checkout.
func evidenceDirHasTrackedFiles(wd, dirRel string) bool {
	out, err := internalgit.Command(context.Background(), wd, "ls-files", "--", filepath.FromSlash(dirRel)).Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

func landingFinalizationWorktree(projectRoot, wd, targetBranch string, gitOps LandingGitOps) (string, func(), error) {
	if !sameFilesystemPath(projectRoot, wd) {
		return wd, func() {}, nil
	}
	tempWT, err := config.MkdirExecutionScratch(projectRoot, "ddx-land-finalize-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating landing finalization worktree: %w", err)
	}
	_ = os.RemoveAll(tempWT)
	if err := gitOps.AddBranchWorktree(wd, tempWT, targetBranch); err != nil {
		_ = os.RemoveAll(tempWT)
		return "", nil, fmt.Errorf("adding landing finalization worktree for %s: %w", targetBranch, err)
	}
	cleanup := func() {
		_ = gitOps.RemoveWorktree(wd, tempWT)
		_ = os.RemoveAll(tempWT)
	}
	return tempWT, cleanup, nil
}

func copyEvidenceDirForLanding(projectRoot, landingWD, relPath string) error {
	if relPath == "" || sameFilesystemPath(projectRoot, landingWD) {
		return nil
	}
	cleanRel, ok := cleanRelativePath(relPath)
	if !ok {
		return fmt.Errorf("invalid evidence path %q", relPath)
	}
	if evidenceDirHasTrackedFiles(landingWD, cleanRel) {
		return nil
	}
	src := filepath.Join(projectRoot, cleanRel)
	dst := filepath.Join(landingWD, cleanRel)
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

func cleanRelativePath(path string) (string, bool) {
	if path == "" || filepath.IsAbs(path) {
		return "", false
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || clean == ".." {
		return "", false
	}
	return clean, true
}

func sameFilesystemPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	absA, err := filepath.Abs(a)
	if err != nil {
		absA = filepath.Clean(a)
	}
	absB, err := filepath.Abs(b)
	if err != nil {
		absB = filepath.Clean(b)
	}
	if resolvedA, err := filepath.EvalSymlinks(absA); err == nil {
		absA = resolvedA
	}
	if resolvedB, err := filepath.EvalSymlinks(absB); err == nil {
		absB = resolvedB
	}
	return absA == absB
}

// Land performs the local land flow for a single submission.
// It serializes only the shared-ref update boundary with the process-shared
// main-git lock so separate ddx work processes cannot interleave landing with
// other target-ref writes.
//
// projectRoot is the directory containing the project's .git. req.WorktreeDir,
// when non-empty, is used as the git working directory for all commands; when
// empty, projectRoot is used. Both usually refer to the same dir — the field
// is kept for forward compatibility with multi-clone topologies.
func Land(projectRoot string, req LandRequest, gitOps LandingGitOps) (*LandResult, error) {
	return landLocked(projectRoot, req, gitOps)
}

type landPreparedState struct {
	targetBranch string
	targetRef    string
	currentTip   string
	contribCount int
}

func prepareLandTarget(projectRoot, wd string, req LandRequest, gitOps LandingGitOps) (*landPreparedState, *LandResult, error) {
	targetBranch := req.TargetBranch
	if targetBranch == "" {
		br, err := gitOps.CurrentBranch(wd)
		if err != nil {
			return nil, nil, fmt.Errorf("resolving target branch: %w", err)
		}
		targetBranch = br
	}
	req.TargetBranch = targetBranch
	if err := gitOps.VerifyCandidateHistory(wd, req.BaseRev, req.ResultRev); err != nil {
		return nil, nil, err
	}
	if err := ensureLandingWorktreeReady(wd, targetBranch); err != nil {
		return nil, nil, err
	}
	targetRef := "refs/heads/" + targetBranch

	currentTip, err := gitOps.ResolveRef(wd, targetRef)
	if err != nil {
		return nil, nil, fmt.Errorf("resolving target tip %s: %w", targetRef, err)
	}
	contribCount := gitOps.CountCommits(wd, req.BaseRev, req.ResultRev)

	if preserved, err := preserveIfLargeDeletion(wd, req, gitOps, currentTip, contribCount); err != nil {
		return nil, nil, err
	} else if preserved != nil {
		return nil, preserved, nil
	}
	if preserved, err := preserveIfSyntaxSanityFails(wd, req, gitOps, currentTip, contribCount); err != nil {
		return nil, nil, err
	} else if preserved != nil {
		return nil, preserved, nil
	}

	return &landPreparedState{
		targetBranch: targetBranch,
		targetRef:    targetRef,
		currentTip:   currentTip,
		contribCount: contribCount,
	}, nil, nil
}

func landLocked(projectRoot string, req LandRequest, gitOps LandingGitOps) (*LandResult, error) {
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
	syncOperatorAfterLand := sameFilesystemPath(projectRoot, wd)
	var operatorDirtyBeforeLand []string
	if syncOperatorAfterLand {
		operatorDirtyBeforeLand = dirtyWorktreePaths(wd)
	}

	for {
		prep, preserved, err := prepareLandTarget(projectRoot, wd, req, gitOps)
		if err != nil {
			return nil, err
		}
		if preserved != nil {
			return preserved, nil
		}
		req.TargetBranch = prep.targetBranch

		var result *LandResult
		var syncFromRev string
		var finalWD string
		var cleanup func()
		var retry bool

		err = withMainGitLock(projectRoot, "land_ref_update", func() error {
			// Commit — never reset or refuse — local tracked modifications
			// before the ref moves. Inside the critical section so a racing
			// land's in-flight sync is never snapshotted as local work.
			if committed, cpErr := checkpointLandingWorktreeLocalChanges(wd, "pre-land "+prep.targetRef); cpErr != nil {
				return cpErr
			} else if committed {
				retry = true
				return nil
			}
			lockedTip, err := gitOps.ResolveRef(wd, prep.targetRef)
			if err != nil {
				return fmt.Errorf("re-resolving target tip %s: %w", prep.targetRef, err)
			}
			if lockedTip != prep.currentTip {
				retry = true
				return nil
			}

			// Fast path: no sibling advanced the target branch → straight ff via
			// update-ref. The worker's commit becomes the new tip unchanged; its
			// parent is still BaseRev, so replay sees the same inputs the worker
			// saw. No merge commit is created.
			if prep.currentTip == req.BaseRev {
				if err := gitOps.UpdateRefTo(wd, prep.targetRef, req.ResultRev, prep.currentTip); err != nil {
					return fmt.Errorf("fast-forwarding %s to %s: %w", prep.targetRef, req.ResultRev, err)
				}
				result = &LandResult{
					Status:            "landed",
					NewTip:            req.ResultRev,
					LandedTip:         req.ResultRev,
					TargetBranch:      prep.targetBranch,
					Merged:            false,
					MergedCommitCount: prep.contribCount,
				}
				finalWD, cleanup, err = landingFinalizationWorktree(projectRoot, wd, prep.targetBranch, gitOps)
				if err != nil {
					return err
				}
				return nil
			}

			// Merge path: the target has advanced since the worker started. Create
			// a temp detached worktree at currentTip and run `git merge --no-ff
			// ResultRev` inside it. The result is a merge commit whose parents are
			// [currentTip, ResultRev]. Crucially, ResultRev itself is NOT rewritten:
			// its parent is still BaseRev, so replay observes the original inputs.
			tempWT, tempWtErr := config.MkdirExecutionScratch(projectRoot, "ddx-land-wt-*")
			if tempWtErr != nil {
				return fmt.Errorf("creating temp worktree dir: %w", tempWtErr)
			}
			// os.MkdirTemp creates the dir, but git worktree add refuses to run if
			// the target already exists. Remove it first so git can recreate it.
			_ = os.RemoveAll(tempWT)
			if err := gitOps.AddWorktree(wd, tempWT, lockedTip); err != nil {
				return fmt.Errorf("adding temp worktree at %s: %w", lockedTip, err)
			}
			defer func() { _ = gitOps.RemoveWorktree(wd, tempWT) }()

			mergeMsg := fmt.Sprintf("Merge bead %s attempt %s into %s", req.BeadID, shortAttempt(req.AttemptID), prep.targetBranch)
			if err := gitOps.MergeInto(tempWT, req.ResultRev, mergeMsg); err != nil {
				// Merge conflict: preserve the original ResultRev and return.
				preserveRef := landIterationRef(req.BeadID, req.AttemptID, lockedTip)
				if upErr := gitOps.UpdateRefTo(wd, preserveRef, req.ResultRev, ""); upErr != nil {
					return fmt.Errorf("preserving %s after merge conflict: %w", preserveRef, upErr)
				}
				result = &LandResult{
					Status:            "preserved",
					PreserveRef:       preserveRef,
					Reason:            "merge conflict",
					TargetBranch:      prep.targetBranch,
					MergedCommitCount: prep.contribCount,
				}
				return nil
			}

			// Merge clean: read the merge commit SHA from the temp worktree's HEAD
			// and fast-forward the target branch to it.
			mergeSHA, err := gitOps.HeadRevAt(tempWT)
			if err != nil {
				return fmt.Errorf("reading merge HEAD: %w", err)
			}
			if err := gitOps.UpdateRefTo(wd, prep.targetRef, mergeSHA, lockedTip); err != nil {
				return fmt.Errorf("fast-forwarding %s to merge commit %s: %w", prep.targetRef, mergeSHA, err)
			}
			result = &LandResult{
				Status:            "landed",
				NewTip:            mergeSHA,
				LandedTip:         mergeSHA,
				TargetBranch:      prep.targetBranch,
				Merged:            true,
				MergedCommitCount: prep.contribCount,
			}
			finalWD, cleanup, err = landingFinalizationWorktree(projectRoot, wd, prep.targetBranch, gitOps)
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if retry {
			continue
		}
		if result != nil && result.Status == "preserved" {
			return result, nil
		}
		if result == nil {
			return nil, fmt.Errorf("landing completed without a result")
		}
		if cleanup != nil {
			defer func() {
				if cleanup != nil {
					cleanup()
				}
			}()
		}

		postLandOutput, postLandErr := runPostLandCommand(finalWD, req.PostLandCommand)
		if postLandErr != nil {
			err = withMainGitLock(projectRoot, "land_post_land_failure", func() error {
				var preserved *LandResult
				preserved, err = preserveIfPostLandGateFailsLocked(finalWD, req, gitOps, prep.targetRef, prep.currentTip, result.NewTip, prep.contribCount, postLandOutput, postLandErr)
				if err != nil {
					return err
				}
				result = preserved
				syncFromRev = result.NewTip
				if syncFromRev == "" {
					syncFromRev = req.ResultRev
				}
				if result != nil && syncOperatorAfterLand && syncFromRev != "" {
					syncWorkTreeToHeadGuarded(gitOps, wd, syncFromRev, operatorDirtyBeforeLand, result)
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			return result, nil
		}

		if req.EvidenceDir != "" {
			if prepareErr := prepareLandEvidence(projectRoot, finalWD, req, gitOps, result); prepareErr != nil {
				err = withMainGitLock(projectRoot, "land_post_land_finalize", func() error {
					preserved, preserveErr := preserveAfterLocalEvidencePreparationFailure(finalWD, req, gitOps, prep.targetRef, prep.currentTip, result.NewTip, prep.contribCount, prepareErr)
					if preserveErr != nil {
						return preserveErr
					}
					result = preserved
					syncFromRev = result.NewTip
					return nil
				})
				if err != nil {
					return nil, err
				}
				return result, nil
			}
		}
		err = withMainGitLock(projectRoot, "land_post_land_finalize", func() error {
			syncFromRev = prep.currentTip
			if result != nil && syncOperatorAfterLand && syncFromRev != "" {
				syncWorkTreeToHeadGuarded(gitOps, wd, syncFromRev, operatorDirtyBeforeLand, result)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return result, nil
	}
}

func preserveIfPostLandGateFailsLocked(wd string, req LandRequest, gitOps LandingGitOps, targetRef, preLandTip, landedTip string, contribCount int, output string, gateErr error) (*LandResult, error) {
	if len(req.PostLandCommand) == 0 {
		return nil, nil
	}
	preserveRef, err := pinPreserveRef(gitOps, wd, req, preLandTip)
	if err != nil {
		return nil, fmt.Errorf("preserving %s after post-land gate: %w", preserveRef, err)
	}
	if revertErr := gitOps.UpdateRefTo(wd, targetRef, preLandTip, landedTip); revertErr != nil {
		return nil, fmt.Errorf("restoring %s to %s after post-land gate failed: %w", targetRef, preLandTip, revertErr)
	}

	reason := fmt.Sprintf("post-land gate failed: %s: %v", strings.Join(req.PostLandCommand, " "), gateErr)
	if trimmed := strings.TrimSpace(output); trimmed != "" {
		reason += ": " + truncatePostLandGateOutput(trimmed)
	}
	return buildPreservedResult(req, preserveRef, reason, contribCount), nil
}

func preserveAfterLocalEvidencePreparationFailure(wd string, req LandRequest, gitOps LandingGitOps, targetRef, preLandTip, landedTip string, contribCount int, evidenceErr error) (*LandResult, error) {
	preserveRef, err := pinPreserveRef(gitOps, wd, req, preLandTip)
	if err != nil {
		return nil, fmt.Errorf("preserving %s after local evidence preparation failure: %w", preserveRef, err)
	}
	if landedTip != "" {
		if revertErr := gitOps.UpdateRefTo(wd, targetRef, preLandTip, landedTip); revertErr != nil {
			return nil, fmt.Errorf("restoring %s to %s after local evidence preparation failed: %w", targetRef, preLandTip, revertErr)
		}
	}
	return buildPreservedResult(req, preserveRef, "local evidence preparation failed: "+evidenceErr.Error(), contribCount), nil
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

	preserveRef, err := pinPreserveRef(gitOps, wd, req, currentTip)
	if err != nil {
		return nil, fmt.Errorf("preserving %s after large-deletion gate: %w", preserveRef, err)
	}
	return buildPreservedResult(req, preserveRef, fmt.Sprintf(LargeDeletionGateReasonPrefix+" %s deleted %d lines (threshold %d) without intentional large deletion acknowledgement", finding.Path, finding.Deleted, threshold), contribCount), nil
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

	preserveRef, err := pinPreserveRef(gitOps, wd, req, currentTip)
	if err != nil {
		return nil, fmt.Errorf("preserving %s after syntax sanity gate: %w", preserveRef, err)
	}
	return buildPreservedResult(req, preserveRef, fmt.Sprintf("syntax sanity gate: %s: %s", finding.Path, finding.Reason), contribCount), nil
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
	dirtyBefore := dirtyWorktreePaths(wd)
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

	tempWT, mkErr := config.MkdirExecutionScratch(wd, "ddx-conflict-recover-*")
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
	syncWorkTreeToHeadGuarded(gitOps, wd, currentTip, dirtyBefore, nil)
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
	if land.TargetBranch != "" {
		res.TargetBranch = land.TargetBranch
	}
	switch land.Status {
	case "landed":
		// Fast-forward or merge commit — either way the target branch now
		// contains the worker's result. ResultRev's own parent is still
		// BaseRev so replay fidelity is preserved.
		reason := ""
		res.PreserveRef = ""
		if land.Merged {
			reason = "merged onto current tip"
		}
		if land.CheckoutSyncDeferred {
			detail := "checkout_sync_deferred"
			if len(land.CheckoutSyncDeferredPaths) > 0 {
				detail += ": " + strings.Join(land.CheckoutSyncDeferredPaths, ", ")
			}
			if reason == "" {
				reason = detail
			} else {
				reason += "; " + detail
			}
		}
		ApplyLandingToResult(res, &BeadLandingResult{Outcome: "merged", Reason: reason})
		landedTip := land.NewTip
		if land.LandedTip != "" {
			landedTip = land.LandedTip
		}
		// LandedTip reflects the ref that actually landed the implementation:
		// either ResultRev on the ff path or the merge commit SHA on the merge
		// path. The compatibility aliases point at that implementation state.
		if landedTip != "" {
			// Preserve the implementation rev before rewriting the compat alias.
			if res.ImplementationRev == "" {
				res.ImplementationRev = res.ResultRev
			}
			res.LandedRev = landedTip
			res.ResultRev = landedTip // backwards-compat alias mirrors LandedRev
		}
	case "preserved":
		res.Outcome = "preserved"
		res.Reason = land.Reason
		res.PreserveRef = land.PreserveRef
	case "no-changes":
		// Only overwrite when the worker itself did not already report
		// a richer no-changes rationale.
		if res.Outcome == "" || res.Outcome == ExecuteBeadOutcomeTaskSucceeded {
			ApplyLandingToResult(res, &BeadLandingResult{Outcome: "no-changes", Reason: land.Reason})
		} else if res.Reason == "" {
			res.Reason = land.Reason
		}
	}
	// Re-classify loop-visible status from the landing outcome.
	reasonForStatus := res.Reason
	if land.Status == "preserved" {
		// Only legacy/generic preserve reasons should fall back to
		// land_conflict. Safety gates preserve good output for operator
		// review and must not be reported as merge failures.
		if strings.HasPrefix(res.Reason, PreMergeChecksReason) ||
			isPreservedNeedsReviewReason(res.Reason) {
			reasonForStatus = res.Reason
		} else {
			reasonForStatus = "merge conflict"
		}
	}
	res.Status = ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, reasonForStatus)
	res.OrchestratorStatus = res.Status
	res.Detail = ExecuteBeadStatusDetail(res.Status, res.Reason, res.Error)
}

// BuildLandRequestFromResult constructs a LandRequest for the coordinator from
// an ExecuteBeadResult. The coordinator always passes projectRoot as the
// workdir — the worker's original worktree has already been cleaned up by the
// time Land() runs.
func BuildLandRequestFromResult(projectRoot string, res *ExecuteBeadResult) LandRequest {
	// Before the first land, ResultRev points at the implementation candidate.
	// After a result has already landed once, ResultRev is rewritten to the
	// landed branch tip; in that case prefer the preserved ImplementationRev so
	// re-submission does not try to land the already-landed branch tip. Legacy
	// results may still describe a trailing evidence commit, but current
	// execution evidence is local-only and never enters candidate history.
	candidateRev := res.ResultRev
	if res.LandedRev != "" && res.ImplementationRev != "" {
		candidateRev = res.ImplementationRev
	}
	if candidateRev == "" {
		candidateRev = res.ImplementationRev
	}
	return LandRequest{
		WorktreeDir:  projectRoot,
		BaseRev:      res.BaseRev,
		ResultRev:    candidateRev,
		BeadID:       res.BeadID,
		AttemptID:    res.AttemptID,
		TargetBranch: "",
		EvidenceDir:  res.ExecutionDir,
	}
}
