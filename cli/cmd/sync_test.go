package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// fakeSyncGit records every git invocation and returns queued responses in order.
// Tests build the response queue to match the exact sequence of git calls for a
// given scenario.
type fakeSyncGit struct {
	responses []fakeGitResp
	idx       int
	allCalls  [][]string
}

type fakeGitResp struct {
	out []byte
	err error
}

func (f *fakeSyncGit) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	f.allCalls = append(f.allCalls, append([]string{}, args...))
	if f.idx >= len(f.responses) {
		return nil, nil
	}
	r := f.responses[f.idx]
	f.idx++
	return r.out, r.err
}

func ok() fakeGitResp { return fakeGitResp{} }
func outErr(out, e string) fakeGitResp {
	return fakeGitResp{out: []byte(out), err: fmt.Errorf("%s", e)}
}
func outOK(out string) fakeGitResp { return fakeGitResp{out: []byte(out)} }

// cleanSyncSequence returns the ordered git responses for a sync run where:
//   - DDx paths are clean (no stash needed)
//   - merge succeeds
//   - all commit checks return empty (nothing to commit)
//   - push succeeds
func cleanSyncSequence() []fakeGitResp {
	return []fakeGitResp{
		ok(),      // fetch origin
		outOK(""), // status --porcelain -- .ddx paths (clean)
		ok(),      // merge origin/main
		outOK(""), // status --porcelain -- .ddx/beads.jsonl (clean)
		outOK(""), // status --porcelain -- .ddx/executions .ddx/plugins (clean)
		ok(),      // push origin HEAD:main
	}
}

// dirtySyncSequence returns ordered git responses for a sync run where
// .ddx/beads.jsonl is tracked-dirty and gets stashed/committed.
func dirtySyncSequence() []fakeGitResp {
	return []fakeGitResp{
		ok(),                           // fetch origin
		outOK(" M .ddx/beads.jsonl\n"), // status --porcelain (tracked dirty)
		ok(),                           // stash push
		ok(),                           // merge origin/main
		ok(),                           // stash pop
		outOK(" M .ddx/beads.jsonl\n"), // status before commit beads.jsonl
		ok(),                           // git add .ddx/beads.jsonl
		ok(),                           // git commit "chore: tracker"
		outOK(""),                      // status before commit executions/plugins
		ok(),                           // push origin HEAD:main
	}
}

// makeTempDDxDir creates a temp dir with a .ddx subdirectory and returns both paths.
func makeTempDDxDir(t *testing.T) (repoRoot, ddxDir string) {
	t.Helper()
	repoRoot = t.TempDir()
	ddxDir = filepath.Join(repoRoot, ddxroot.DirName)
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatalf("mkdir .ddx: %v", err)
	}
	return repoRoot, ddxDir
}

func newSyncerFor(t *testing.T, fake *fakeSyncGit) (s *syncer, ddxDir string) {
	t.Helper()
	_, ddxDir = makeTempDDxDir(t)
	s = &syncer{
		repoRoot: filepath.Dir(ddxDir),
		ddxDir:   ddxDir,
		runner:   fake.run,
		out:      &bytes.Buffer{},
	}
	return s, ddxDir
}

// ---- AC3: Allowlist enforcement ----

// TestSync_UnrelatedDirtyFilesUntouched verifies that a file outside the
// DDx-managed allowlist is never passed to 'git add' during a sync run.
// A non-DDx dirty file (e.g. src/main.go) must survive the sync unchanged.
func TestSync_UnrelatedDirtyFilesUntouched(t *testing.T) {
	fake := &fakeSyncGit{responses: cleanSyncSequence()}
	s, _ := newSyncerFor(t, fake)

	if err := s.run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, call := range fake.allCalls {
		if len(call) == 0 || call[0] != "add" {
			continue
		}
		for _, arg := range call[1:] {
			if arg == "--" {
				continue
			}
			allowed := false
			for _, p := range ddxManagedPaths {
				if strings.HasPrefix(arg, p) || arg == p {
					allowed = true
					break
				}
			}
			if !allowed {
				t.Errorf("git add called with non-DDx path %q; unrelated files must not be staged", arg)
			}
		}
	}
}

// ---- AC4: No destructive flags ----

// TestSync_NoForceOrNoVerifyFlags verifies that no git invocation during a
// sync run ever includes --force, --no-verify, or other destructive flags.
func TestSync_NoForceOrNoVerifyFlags(t *testing.T) {
	fake := &fakeSyncGit{responses: dirtySyncSequence()}
	s, _ := newSyncerFor(t, fake)
	_ = s.run(context.Background()) // outcome doesn't matter

	forbidden := []string{"--force", "-f", "--no-verify", "--force-with-lease", "--hard", "--delete"}
	for _, call := range fake.allCalls {
		for _, arg := range call {
			for _, bad := range forbidden {
				if arg == bad {
					t.Errorf("git call contained forbidden flag %q: %v", bad, call)
				}
			}
		}
	}
}

// ---- AC5: Stash-pop conflict aborts with structured exit ----

// TestSync_StashPopConflictAborts verifies that when 'git stash pop' fails
// (e.g. merge conflict), sync returns a non-nil error and writes
// sync-failure.json to .ddx/.
func TestSync_StashPopConflictAborts(t *testing.T) {
	responses := []fakeGitResp{
		ok(),                                // fetch origin
		outOK(" M .ddx/beads.jsonl\n"),      // status --porcelain (tracked dirty)
		ok(),                                // stash push
		ok(),                                // merge origin/main
		outErr("CONFLICT", "exit status 1"), // stash pop → conflict
	}
	fake := &fakeSyncGit{responses: responses}
	s, ddxDir := newSyncerFor(t, fake)

	err := s.run(context.Background())
	if err == nil {
		t.Fatal("expected error on stash-pop conflict, got nil")
	}
	if !strings.Contains(err.Error(), "stash-pop conflict") {
		t.Errorf("error message should mention 'stash-pop conflict', got: %v", err)
	}

	assertSyncFailureWritten(t, ddxDir, "stash-pop conflict")
}

// ---- AC5: Double push failure aborts with structured exit ----

// TestSync_DoublePushFailAborts verifies that when push fails twice (first
// rejected non-fast-forward, then rejected again on retry), and HEAD differs
// from origin/main each time, sync returns a non-nil error and writes
// sync-failure.json.
func TestSync_DoublePushFailAborts(t *testing.T) {
	headSHA := "aaa111bbb222ccc333ddd444eee555fff666aaa1"
	originSHA := "bbb222ccc333ddd444eee555fff666aaa111bbb2"
	// First attempt: all clean, push rejected (non-fast-forward), HEAD != origin/main.
	// Second attempt (retry): all clean again, push rejected again, HEAD still != origin/main.
	responses := []fakeGitResp{
		// First attempt
		ok(),      // fetch origin
		outOK(""), // status --porcelain -- .ddx paths (clean)
		ok(),      // merge origin/main
		outOK(""), // status beads.jsonl
		outOK(""), // status executions/plugins
		outErr("error: failed to push some refs\nhint: Updates were rejected because the remote contains work that you\nhint: do not have locally. Integrate the remote changes (e.g.\nhint: 'git pull ...') before pushing again.\nhint: See the 'Note about fast-forwards' in 'git push --help' for details.\nnon-fast-forward", "exit status 1"),
		outOK(headSHA + "\n"),   // rev-parse HEAD
		outOK(originSHA + "\n"), // rev-parse origin/main (diverged → retry)
		// Retry (second attempt)
		ok(),      // fetch origin
		outOK(""), // status --porcelain -- .ddx paths
		ok(),      // merge origin/main
		outOK(""), // status beads.jsonl
		outOK(""), // status executions/plugins
		outErr("non-fast-forward", "exit status 1"), // push fails again
		outOK(headSHA + "\n"),                       // rev-parse HEAD
		outOK(originSHA + "\n"),                     // rev-parse origin/main (still diverged → abort)
	}
	fake := &fakeSyncGit{responses: responses}
	s, ddxDir := newSyncerFor(t, fake)

	err := s.run(context.Background())
	if err == nil {
		t.Fatal("expected error on double push failure, got nil")
	}
	if !strings.Contains(err.Error(), "push failed twice") {
		t.Errorf("error message should mention 'push failed twice', got: %v", err)
	}

	assertSyncFailureWritten(t, ddxDir, "push failed twice")
}

// ---- AC6: Doctor surfaces sync failure ----

// TestDoctor_SyncFailureSurfaced verifies that checkSyncFailure returns a
// DiagnosticIssue describing the recorded failure when sync-failure.json exists.
func TestDoctor_SyncFailureSurfaced(t *testing.T) {
	dir := t.TempDir()
	failurePath := filepath.Join(dir, "sync-failure.json")

	ts := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	failure := SyncFailure{Timestamp: ts, Reason: "stash-pop conflict: merge conflict in .ddx/beads.jsonl"}
	data, _ := json.Marshal(failure)
	if err := os.WriteFile(failurePath, data, 0o644); err != nil {
		t.Fatalf("write failure file: %v", err)
	}

	issue := checkSyncFailure(failurePath)
	if issue == nil {
		t.Fatal("expected DiagnosticIssue, got nil")
	}
	if issue.Type != "sync_aborted" {
		t.Errorf("expected type sync_aborted, got %q", issue.Type)
	}
	if !strings.Contains(issue.Description, "stash-pop conflict") {
		t.Errorf("description should contain the stored reason, got: %q", issue.Description)
	}
	if !strings.Contains(issue.Description, "2026-04-29") {
		t.Errorf("description should contain the timestamp, got: %q", issue.Description)
	}
	if len(issue.Remediation) == 0 {
		t.Error("expected at least one remediation step")
	}
}

// TestDoctor_NoSyncFailureFile verifies that checkSyncFailure returns nil when
// there is no sync-failure.json (clean state).
func TestDoctor_NoSyncFailureFile(t *testing.T) {
	dir := t.TempDir()
	issue := checkSyncFailure(filepath.Join(dir, "sync-failure.json"))
	if issue != nil {
		t.Errorf("expected nil for missing failure file, got %+v", issue)
	}
}

// ---- Allowlist coverage: only DDx paths committed ----

// TestSync_CommitsOnlyDDxPaths verifies that when DDx files are dirty, only
// DDx-managed paths are staged — the git add args never include any other path.
func TestSync_CommitsOnlyDDxPaths(t *testing.T) {
	fake := &fakeSyncGit{responses: dirtySyncSequence()}
	s, _ := newSyncerFor(t, fake)

	if err := s.run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, call := range fake.allCalls {
		if len(call) == 0 || call[0] != "add" {
			continue
		}
		// First arg after "add" should be "--"
		foundSep := false
		for i, arg := range call[1:] {
			if arg == "--" {
				foundSep = true
				continue
			}
			if !foundSep {
				continue
			}
			// Every path after "--" must be in the DDx allowlist.
			allowed := false
			for _, p := range ddxManagedPaths {
				if arg == p || strings.HasPrefix(arg, p+"/") {
					allowed = true
					break
				}
			}
			if !allowed {
				t.Errorf("git add[%d] staged non-DDx path %q", i, arg)
			}
		}
	}
}

// ---- Watch mode: exits cleanly on context cancel ----

// TestSync_WatchExitsOnContextCancel verifies that --watch mode exits promptly
// when its context is cancelled (simulating SIGINT / Ctrl-C).
func TestSync_WatchExitsOnContextCancel(t *testing.T) {
	// Provide enough clean responses for one sync cycle.
	fake := &fakeSyncGit{responses: cleanSyncSequence()}
	repoRoot, ddxDir := makeTempDDxDir(t)
	_ = ddxDir

	factory := NewCommandFactory(repoRoot)
	factory.syncGitRunnerOverride = fake.run
	rootCmd := factory.NewRootCommand()
	rootCmd.SetArgs([]string{"sync", "--watch", "--interval=100ms"})

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Run in a goroutine; it should return once ctx expires.
	done := make(chan error, 1)
	go func() {
		done <- rootCmd.ExecuteContext(ctx)
	}()

	select {
	case <-done:
		// Exited as expected (either clean or context error).
	case <-time.After(2 * time.Second):
		t.Fatal("--watch mode did not exit within 2s after context cancel")
	}
}

// ---- AC1: Push uses HEAD:main refspec ----

// TestSync_PushesCurrentHEADToOriginMainWhenOnNonMainBranch verifies that sync
// pushes the current HEAD (not the local branch named "main") to origin/main,
// so it works correctly when the operator is on a non-main branch.
func TestSync_PushesCurrentHEADToOriginMainWhenOnNonMainBranch(t *testing.T) {
	fake := &fakeSyncGit{responses: cleanSyncSequence()}
	s, _ := newSyncerFor(t, fake)

	if err := s.run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var foundPush bool
	for _, call := range fake.allCalls {
		if len(call) == 0 || call[0] != "push" {
			continue
		}
		foundPush = true
		usesHEADRefspec := false
		for _, arg := range call {
			if arg == "HEAD:main" {
				usesHEADRefspec = true
				break
			}
		}
		if !usesHEADRefspec {
			t.Errorf("git push does not use HEAD:main refspec (pushes stale local main); got: %v", call)
		}
	}
	if !foundPush {
		t.Error("no git push call was made")
	}
}

// ---- AC2: HEAD already equals origin/main → success ----

// TestSync_NonFastForwardButHeadAlreadyEqualsOriginMainTreatsAsSuccess simulates
// a push rejection (non-fast-forward) that occurs when the operator is on a
// non-main branch whose HEAD is already aligned with origin/main. After fetch,
// rev-parse HEAD and rev-parse origin/main return the same SHA, so sync should
// exit 0 and remove any existing .ddx/sync-failure.json.
func TestSync_NonFastForwardButHeadAlreadyEqualsOriginMainTreatsAsSuccess(t *testing.T) {
	sha := "abc1234def5678abc1234def5678abc1234def56"
	responses := []fakeGitResp{
		ok(),      // fetch origin
		outOK(""), // status --porcelain -- .ddx paths (clean)
		ok(),      // merge origin/main
		outOK(""), // status beads.jsonl (clean)
		outOK(""), // status executions/plugins (clean)
		outErr("error: failed to push\nnon-fast-forward", "exit status 1"), // push rejected
		outOK(sha + "\n"), // rev-parse HEAD
		outOK(sha + "\n"), // rev-parse origin/main (same SHA → already aligned)
	}
	fake := &fakeSyncGit{responses: responses}
	s, ddxDir := newSyncerFor(t, fake)

	// Pre-create a stale failure file; it must be removed on success.
	failurePath := filepath.Join(ddxDir, "sync-failure.json")
	if err := os.WriteFile(failurePath, []byte(`{"timestamp":"2026-01-01T00:00:00Z","reason":"old"}`), 0o644); err != nil {
		t.Fatalf("setup: write failure file: %v", err)
	}

	err := s.run(context.Background())
	if err != nil {
		t.Fatalf("expected success when HEAD already equals origin/main, got: %v", err)
	}

	if _, statErr := os.Stat(failurePath); !os.IsNotExist(statErr) {
		t.Error("sync-failure.json should be removed when HEAD already equals origin/main")
	}
}

// ---- Helpers ----

func assertSyncFailureWritten(t *testing.T, ddxDir, reasonSubstr string) {
	t.Helper()
	failurePath := filepath.Join(ddxDir, "sync-failure.json")
	data, err := os.ReadFile(failurePath)
	if err != nil {
		t.Fatalf("sync-failure.json not written: %v", err)
	}
	var failure SyncFailure
	if err := json.Unmarshal(data, &failure); err != nil {
		t.Fatalf("sync-failure.json not valid JSON: %v", err)
	}
	if failure.Reason == "" {
		t.Error("sync-failure.json has empty reason")
	}
	if !strings.Contains(failure.Reason, reasonSubstr) {
		t.Errorf("sync-failure.json reason should contain %q, got %q", reasonSubstr, failure.Reason)
	}
	if failure.Timestamp.IsZero() {
		t.Error("sync-failure.json has zero timestamp")
	}
}
