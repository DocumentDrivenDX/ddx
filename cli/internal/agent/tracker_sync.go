package agent

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

const trackerSyncBranch = "main"

var trackerSyncGitRunner = func(ctx context.Context, gitDir string, args ...string) ([]byte, error) {
	return internalgit.Command(ctx, gitDir, args...).CombinedOutput()
}

func syncTrackerBeforeClaim(ctx context.Context, projectRoot string, log io.Writer, emit func(string, map[string]any)) {
	if strings.TrimSpace(projectRoot) == "" {
		return
	}
	if err := commitTrackerLocked(projectRoot); err != nil {
		emitTrackerSyncAttention(log, emit, "pre_claim", "", "tracker_sync_commit_failed", err.Error())
		return
	}
	if err := trackerSyncFetchAndMerge(ctx, projectRoot, log, emit, "pre_claim"); err != nil {
		emitTrackerSyncAttention(log, emit, "pre_claim", "", "tracker_sync_merge_failed", err.Error())
	}
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
		emitTrackerSyncWarning(log, emit, stage, "fetch_failed", strings.TrimSpace(string(out)))
		return nil
	}
	if out, err := trackerSyncGitRunner(ctx, projectRoot, "merge", "--no-ff", "-m", "chore: sync origin/main", "origin/"+trackerSyncBranch); err != nil {
		_, _ = trackerSyncGitRunner(ctx, projectRoot, "merge", "--abort")
		emitTrackerSyncWarning(log, emit, stage, "merge_failed", strings.TrimSpace(string(out)))
		return err
	}
	return nil
}

func trackerSyncPushWithRetry(ctx context.Context, projectRoot, stage, beadID string, log io.Writer, emit func(string, map[string]any)) error {
	const attempts = 3
	backoff := 100 * time.Millisecond
	var lastErr error
	for i := 0; i < attempts; i++ {
		out, err := trackerSyncGitRunner(ctx, projectRoot, "push", "origin", "HEAD:main")
		if err == nil {
			return nil
		}
		lastErr = fmt.Errorf("git push origin HEAD:main: %s: %w", strings.TrimSpace(string(out)), err)
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
