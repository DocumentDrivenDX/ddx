package agent

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

// trackerSyncFallbackBranch is used only when neither the current branch's
// upstream nor the remote's HEAD symref can be resolved (e.g. old repos with
// no tracking configured and no origin/HEAD symref).
const trackerSyncFallbackBranch = "main"

var trackerSyncGitRunner = func(ctx context.Context, gitDir string, args ...string) ([]byte, error) {
	return internalgit.Command(ctx, gitDir, args...).CombinedOutput()
}

// resolveTrackerSyncBranch determines which branch the tracker should sync
// against. It prefers the current branch's configured upstream, then falls
// back to the remote's default branch (refs/remotes/origin/HEAD), then to
// trackerSyncFallbackBranch for repositories with neither.
func resolveTrackerSyncBranch(ctx context.Context, projectRoot string) string {
	if branch := trackerSyncUpstreamBranch(ctx, projectRoot); branch != "" {
		return branch
	}
	if branch := trackerSyncOriginHeadBranch(ctx, projectRoot); branch != "" {
		return branch
	}
	return trackerSyncFallbackBranch
}

// trackerSyncUpstreamBranch returns the branch name of the current branch's
// upstream (e.g. "master" for an upstream of "origin/master"), or "" if the
// current branch has no upstream configured.
func trackerSyncUpstreamBranch(ctx context.Context, projectRoot string) string {
	out, err := trackerSyncGitRunner(ctx, projectRoot, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err != nil {
		return ""
	}
	ref := strings.TrimSpace(string(out))
	if ref == "" {
		return ""
	}
	if idx := strings.Index(ref, "/"); idx >= 0 {
		return ref[idx+1:]
	}
	return ref
}

// trackerSyncOriginHeadBranch returns the branch name that refs/remotes/origin/HEAD
// points at (the remote's reported default branch), or "" if that symref is
// not set locally.
func trackerSyncOriginHeadBranch(ctx context.Context, projectRoot string) string {
	out, err := trackerSyncGitRunner(ctx, projectRoot, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err != nil {
		return ""
	}
	const prefix = "refs/remotes/origin/"
	ref := strings.TrimSpace(string(out))
	if !strings.HasPrefix(ref, prefix) {
		return ""
	}
	return strings.TrimPrefix(ref, prefix)
}

func syncTrackerBeforeClaim(ctx context.Context, projectRoot string, log io.Writer, emit func(string, map[string]any)) error {
	if strings.TrimSpace(projectRoot) == "" {
		return nil
	}
	if err := commitTrackerLocked(projectRoot); err != nil {
		emitTrackerSyncAttention(log, emit, "pre_claim", "", "tracker_sync_commit_failed", err.Error())
		return err
	}
	if err := trackerSyncFetchAndMerge(ctx, projectRoot, log, emit, "pre_claim"); err != nil {
		if isCorruptTrackerSyncError(err.Error()) {
			emitTrackerSyncAttention(log, emit, "pre_claim", "", "project_git_corrupt", err.Error())
			return err
		}
		emitTrackerSyncAttention(log, emit, "pre_claim", "", "tracker_sync_merge_failed", err.Error())
	}
	return nil
}

func syncTrackerAfterClaim(ctx context.Context, projectRoot, beadID string, log io.Writer, emit func(string, map[string]any)) {
	syncTrackerPublish(ctx, projectRoot, beadID, "claim", log, emit)
}

func syncTrackerAfterClose(ctx context.Context, projectRoot, beadID string, log io.Writer, emit func(string, map[string]any)) {
	syncTrackerPublish(ctx, projectRoot, beadID, "close", log, emit)
}

func syncTrackerPublish(ctx context.Context, projectRoot, beadID, stage string, log io.Writer, emit func(string, map[string]any)) {
	if strings.TrimSpace(projectRoot) == "" {
		return
	}
	if err := commitTrackerLocked(projectRoot); err != nil {
		emitTrackerSyncAttention(log, emit, stage, beadID, "tracker_sync_commit_failed", err.Error())
		return
	}
	if err := trackerSyncPushWithRetry(ctx, projectRoot, stage, beadID, log, emit); err != nil {
		emitTrackerSyncAttention(log, emit, stage, beadID, "tracker_sync_push_failed", err.Error())
	}
}

func trackerSyncFetchAndMerge(ctx context.Context, projectRoot string, log io.Writer, emit func(string, map[string]any), stage string) error {
	if strings.TrimSpace(projectRoot) == "" {
		return nil
	}
	if out, err := trackerSyncGitRunner(ctx, projectRoot, "fetch", "origin"); err != nil {
		outText := strings.TrimSpace(string(out))
		if isCorruptTrackerSyncError(outText) {
			return fmt.Errorf("git fetch origin: %s: %w", outText, err)
		}
		emitTrackerSyncWarning(log, emit, stage, "fetch_failed", outText)
		return nil
	}
	branch := resolveTrackerSyncBranch(ctx, projectRoot)
	if out, err := trackerSyncGitRunner(ctx, projectRoot, "merge", "--no-ff", "-m", "chore: sync origin/"+branch, "origin/"+branch); err != nil {
		_, _ = trackerSyncGitRunner(ctx, projectRoot, "merge", "--abort")
		outText := strings.TrimSpace(string(out))
		if isCorruptTrackerSyncError(outText) {
			return fmt.Errorf("git merge --no-ff origin/%s: %s: %w", branch, outText, err)
		}
		emitTrackerSyncWarning(log, emit, stage, "merge_failed", outText)
		return err
	}
	return nil
}

func trackerSyncPushWithRetry(ctx context.Context, projectRoot, stage, beadID string, log io.Writer, emit func(string, map[string]any)) error {
	refspec := "HEAD:" + resolveTrackerSyncBranch(ctx, projectRoot)
	const attempts = 3
	backoff := 100 * time.Millisecond
	var lastErr error
	for i := 0; i < attempts; i++ {
		out, err := trackerSyncGitRunner(ctx, projectRoot, "push", "origin", refspec)
		if err == nil {
			return nil
		}
		lastErr = fmt.Errorf("git push origin %s: %s: %w", refspec, strings.TrimSpace(string(out)), err)
		if i == attempts-1 {
			break
		}
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		} else {
			time.Sleep(backoff)
		}
		backoff *= 2
	}
	if lastErr != nil {
		emitTrackerSyncWarning(log, emit, stage, "push_failed", lastErr.Error())
	}
	return lastErr
}

func emitTrackerSyncWarning(log io.Writer, emit func(string, map[string]any), stage, reason, detail string) {
	detail = strings.TrimSpace(detail)
	if log != nil {
		if detail != "" {
			_, _ = fmt.Fprintf(log, "tracker sync (%s): %s: %s; continuing with local state\n", stage, reason, detail)
		} else {
			_, _ = fmt.Fprintf(log, "tracker sync (%s): %s; continuing with local state\n", stage, reason)
		}
	}
	if emit != nil {
		payload := map[string]any{
			"stage":  stage,
			"reason": reason,
		}
		if detail != "" {
			payload["detail"] = detail
		}
		emit("loop.tracker_sync_warning", payload)
	}
}

func emitTrackerSyncAttention(log io.Writer, emit func(string, map[string]any), stage, beadID, reason, detail string) {
	detail = strings.TrimSpace(detail)
	if log != nil {
		if detail != "" {
			_, _ = fmt.Fprintf(log, "tracker sync (%s): %s: %s; continuing with local state\n", stage, reason, detail)
		} else {
			_, _ = fmt.Fprintf(log, "tracker sync (%s): %s; continuing with local state\n", stage, reason)
		}
	}
	if emit != nil {
		payload := map[string]any{
			"reason": reason,
			"stage":  stage,
		}
		if beadID != "" {
			payload["bead_id"] = beadID
		}
		if detail != "" {
			payload["detail"] = detail
		}
		emit("loop.operator_attention", payload)
	}
}

func isCorruptTrackerSyncError(detail string) bool {
	if detail == "" {
		return false
	}
	lower := strings.ToLower(detail)
	switch {
	case strings.Contains(lower, "fatal: bad object"),
		strings.Contains(lower, "invalid sha1 pointer"),
		strings.Contains(lower, "did not send all necessary objects"):
		return true
	default:
		return false
	}
}
