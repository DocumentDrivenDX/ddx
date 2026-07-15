package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// TestExecuteBeadLandsEvidence is the AC (1) regression test: a simulated
// successful execute-bead attempt against a real temp git repo asserts that
// the working tree is clean while the local evidence remains outside durable
// branch history. AC (2): the worker's closing_commit_sha still resolves after
// the merge.
func TestExecuteBeadLandsEvidence(t *testing.T) {
	r := newLandTestRepo(t)

	// Simulate the orchestrator materializing evidence files in the project root.
	attemptID := "20260416T181205-aabb1122"
	evidenceDir := filepath.Join(ddxroot.DirName, "executions", attemptID)
	fullDir := filepath.Join(r.dir, evidenceDir)
	if err := os.MkdirAll(fullDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeEvidenceFile(t, fullDir, "manifest.json", `{"attempt_id":"`+attemptID+`","bead_id":"ddx-evidence-test"}`)
	writeEvidenceFile(t, fullDir, "result.json", `{"status":"success","bead_id":"ddx-evidence-test"}`)
	writeEvidenceFile(t, fullDir, "prompt.md", "# Prompt\nExecute bead ddx-evidence-test")

	// Create a worker commit (simulates agent output).
	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature content\n", "feat: implement feature [ddx-evidence-test]")

	// Land with evidence — fast-forward path.
	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-evidence-test",
		AttemptID:    attemptID,
		TargetBranch: "main",
		EvidenceDir:  filepath.ToSlash(evidenceDir),
	}
	land, err := Land(r.dir, req, RealLandingGitOps{})
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected status=landed, got %q (reason=%q)", land.Status, land.Reason)
	}

	// Working tree is clean: evidence is gitignored, so it is untracked+ignored.
	statusOut := r.runGit("status", "--porcelain")
	if strings.TrimSpace(statusOut) != "" {
		t.Errorf("working tree not clean after Land:\n%s", statusOut)
	}

	// Execution evidence must NEVER reach the durable branch (ddx-d10073a8);
	// NewTip is the implementation commit, not an evidence commit.
	assertNoExecutionEvidenceOnBranch(t, r, "main")
	if land.EvidenceCommitSHA != "" {
		t.Errorf("evidence must not be committed; got %s", land.EvidenceCommitSHA)
	}
	if land.NewTip != workerSHA {
		t.Errorf("NewTip = %s, want worker commit %s (no evidence commit)", land.NewTip, workerSHA)
	}

	// AC (2): closing_commit_sha (the tip that will be recorded) still resolves.
	checkCmd := exec.Command("git", "-C", r.dir, "cat-file", "-e", land.NewTip)
	if err := checkCmd.Run(); err != nil {
		t.Errorf("AC 2 FAILED: closing_commit_sha %s does not resolve: %v", land.NewTip, err)
	}
	// The worker's original commit must also still resolve.
	checkCmd = exec.Command("git", "-C", r.dir, "cat-file", "-e", workerSHA)
	if err := checkCmd.Run(); err != nil {
		t.Errorf("AC 2 FAILED: worker commit %s no longer resolves: %v", workerSHA, err)
	}
	// Worker commit must be reachable from main.
	if !r.shaReachable("refs/heads/main", workerSHA) {
		t.Errorf("worker commit %s not reachable from main", workerSHA)
	}
}

// TestExecuteBeadLandsEvidence_MergePath exercises the merge path (target
// branch has advanced since the worker started) and verifies evidence is
// committed and the worktree is clean.
func TestExecuteBeadLandsEvidence_MergePath(t *testing.T) {
	r := newLandTestRepo(t)

	attemptID := "20260416T181206-ccdd3344"
	evidenceDir := filepath.Join(ddxroot.DirName, "executions", attemptID)
	fullDir := filepath.Join(r.dir, evidenceDir)
	if err := os.MkdirAll(fullDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeEvidenceFile(t, fullDir, "manifest.json", `{"attempt_id":"`+attemptID+`"}`)
	writeEvidenceFile(t, fullDir, "result.json", `{"status":"merged"}`)

	// Worker branches off baseSHA.
	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "feature\n", "feat: worker")

	// Sibling advances main. Sync the working tree so it matches siblingSHA's
	// tree — update-ref alone only moves the ref, leaving the working tree stale.
	siblingSHA := r.commitOn(r.baseSHA, "sibling.txt", "sibling\n", "feat: sibling")
	r.runGit("update-ref", "refs/heads/main", siblingSHA)
	r.runGit("read-tree", "HEAD")
	r.runGit("checkout-index", "-f", "-a")

	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-evidence-merge",
		AttemptID:    attemptID,
		TargetBranch: "main",
		EvidenceDir:  filepath.ToSlash(evidenceDir),
	}
	land, err := Land(r.dir, req, RealLandingGitOps{})
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected landed, got %q", land.Status)
	}
	if !land.Merged {
		t.Errorf("expected Merged=true on merge path")
	}

	statusOut := r.runGit("status", "--porcelain")
	if strings.TrimSpace(statusOut) != "" {
		t.Errorf("working tree not clean after merge-path land:\n%s", statusOut)
	}

	if land.EvidenceCommitSHA != "" {
		t.Errorf("evidence must not be committed on merge path; got %s", land.EvidenceCommitSHA)
	}
	assertNoExecutionEvidenceOnBranch(t, r, "main")

	// Worker commit still resolves (closing_commit_sha contract).
	checkCmd := exec.Command("git", "-C", r.dir, "cat-file", "-e", workerSHA)
	if err := checkCmd.Run(); err != nil {
		t.Errorf("worker SHA %s doesn't resolve after merge: %v", workerSHA, err)
	}
	if !r.shaReachable("refs/heads/main", workerSHA) {
		t.Errorf("worker commit %s not reachable from main after merge", workerSHA)
	}
}

// TestLandNoEvidenceDir verifies that when EvidenceDir is empty, no evidence
// commit is created and behavior matches the pre-fix code path.
func TestLandNoEvidenceDir(t *testing.T) {
	r := newLandTestRepo(t)

	workerSHA := r.commitOn(r.baseSHA, "feature.txt", "hello\n", "feat: hello")

	req := LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      r.baseSHA,
		ResultRev:    workerSHA,
		BeadID:       "ddx-no-evidence",
		AttemptID:    "20260416T181207-eeff5566",
		TargetBranch: "main",
	}
	land, err := Land(r.dir, req, RealLandingGitOps{})
	if err != nil {
		t.Fatalf("Land: %v", err)
	}
	if land.Status != "landed" {
		t.Fatalf("expected landed, got %q", land.Status)
	}
	if land.EvidenceCommitSHA != "" {
		t.Errorf("expected no evidence commit, got %s", land.EvidenceCommitSHA)
	}
	if land.NewTip != workerSHA {
		t.Errorf("NewTip = %s, want %s (no evidence commit)", land.NewTip, workerSHA)
	}
}

// TestVerifyCleanWorktree verifies the normal safety-net path when the project
// already ignores execution evidence.
func TestVerifyCleanWorktree(t *testing.T) {
	r := newLandTestRepo(t)

	attemptID := "20260416T181208-verify01"
	evidenceDir := filepath.Join(r.dir, ExecuteBeadArtifactDir, attemptID)
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeEvidenceFile(t, evidenceDir, "result.json", `{"status":"test"}`)
	dirRel := filepath.ToSlash(filepath.Join(ExecuteBeadArtifactDir, attemptID))

	// Evidence is gitignored (ddx-d10073a8), so it never dirties the worktree.
	statusBefore := r.runGit("status", "--porcelain", "--", dirRel)
	if strings.TrimSpace(statusBefore) != "" {
		t.Fatalf("gitignored evidence should not appear dirty: %s", statusBefore)
	}

	if err := VerifyCleanWorktree(r.dir, attemptID); err != nil {
		t.Fatalf("VerifyCleanWorktree: %v", err)
	}

	// Evidence must NOT be committed and must remain on disk; worktree stays clean.
	if _, err := os.Stat(filepath.Join(evidenceDir, "result.json")); err != nil {
		t.Errorf("evidence must remain on disk: %v", err)
	}
	assertNoExecutionEvidenceOnBranch(t, r, "main")
	statusAfter := r.runGit("status", "--porcelain", "--", dirRel)
	if strings.TrimSpace(statusAfter) != "" {
		t.Errorf("evidence dir dirty after VerifyCleanWorktree:\n%s", statusAfter)
	}
}

// TestVerifyCleanWorktreeNoOp verifies VerifyCleanWorktree is a no-op when the
// evidence directory does not exist.
func TestVerifyCleanWorktreeNoOp(t *testing.T) {
	r := newLandTestRepo(t)

	attemptID := "20260416T181209-noop0001"
	if err := VerifyCleanWorktree(r.dir, attemptID); err != nil {
		t.Errorf("VerifyCleanWorktree should be no-op when evidence dir doesn't exist: %v", err)
	}
}

func TestVerifyCleanWorktree_MissingGitignoreUsesLocalExcludeAndNeverCommitsEvidence(t *testing.T) {
	r := newLandTestRepo(t)
	r.runGit("rm", ".gitignore")
	r.runGit("commit", "-m", "remove project gitignore")

	attemptID := "20260715T120000-localexc"
	dirRel := filepath.ToSlash(filepath.Join(ExecuteBeadArtifactDir, attemptID))
	evidenceFile := filepath.Join(r.dir, filepath.FromSlash(dirRel), "result.json")
	if err := os.MkdirAll(filepath.Dir(evidenceFile), 0o755); err != nil {
		t.Fatal(err)
	}
	wantBytes := []byte(`{"status":"preserved"}`)
	if err := os.WriteFile(evidenceFile, wantBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	headBefore := r.resolveRef("HEAD")

	if err := VerifyCleanWorktree(r.dir, attemptID); err != nil {
		t.Fatalf("VerifyCleanWorktree: %v", err)
	}
	// A second call must not duplicate the local rule.
	if err := VerifyCleanWorktree(r.dir, attemptID); err != nil {
		t.Fatalf("VerifyCleanWorktree second call: %v", err)
	}

	if got := r.resolveRef("HEAD"); got != headBefore {
		t.Fatalf("HEAD changed: got %s, want %s", got, headBefore)
	}
	if _, err := os.Stat(filepath.Join(r.dir, ".gitignore")); !os.IsNotExist(err) {
		t.Fatalf("tracked .gitignore must remain absent, stat err=%v", err)
	}
	excludeBytes := readGitExclude(t, r)
	if count := countExactLines(string(excludeBytes), executionEvidenceLocalExclude); count != 1 {
		t.Fatalf("local exclude rule count = %d, want 1; exclude contents:\n%s", count, excludeBytes)
	}
	if got := r.runGit("status", "--porcelain", "--untracked-files=all", "--", dirRel); got != "" {
		t.Fatalf("evidence remains visible in status:\n%s", got)
	}
	if got := r.runGit("ls-files", "--", dirRel); got != "" {
		t.Fatalf("evidence was added to the index: %s", got)
	}
	if got := r.runGit("log", "--format=%H", "--all", "--", dirRel); got != "" {
		t.Fatalf("evidence appeared in history: %s", got)
	}
	gotBytes, err := os.ReadFile(evidenceFile)
	if err != nil {
		t.Fatalf("reading retained evidence: %v", err)
	}
	if string(gotBytes) != string(wantBytes) {
		t.Fatalf("evidence bytes changed: got %q, want %q", gotBytes, wantBytes)
	}
}

func TestVerifyCleanWorktree_TrackedOrStagedEvidenceFailsWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(*landTestRepo, string)
		wantErr string
	}{
		{
			name: "tracked",
			prepare: func(r *landTestRepo, dirRel string) {
				r.runGit("add", "-f", "--", dirRel)
				r.runGit("commit", "-m", "seed forbidden tracked evidence")
			},
			wantErr: "tracked",
		},
		{
			name: "staged",
			prepare: func(r *landTestRepo, dirRel string) {
				r.runGit("add", "-f", "--", dirRel)
			},
			wantErr: "staged",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newLandTestRepo(t)
			attemptID := "20260715T120100-" + tc.name
			dirRel := filepath.ToSlash(filepath.Join(ExecuteBeadArtifactDir, attemptID))
			evidenceFile := filepath.Join(r.dir, filepath.FromSlash(dirRel), "result.json")
			if err := os.MkdirAll(filepath.Dir(evidenceFile), 0o755); err != nil {
				t.Fatal(err)
			}
			wantBytes := []byte(`{"state":"` + tc.name + `"}`)
			if err := os.WriteFile(evidenceFile, wantBytes, 0o644); err != nil {
				t.Fatal(err)
			}
			tc.prepare(r, dirRel)

			headBefore := r.resolveRef("HEAD")
			statusBefore := r.runGit("status", "--porcelain=v1", "--untracked-files=all")
			indexBefore := r.runGit("diff", "--cached", "--binary")
			excludeBefore := readGitExclude(t, r)

			err := VerifyCleanWorktree(r.dir, attemptID)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("VerifyCleanWorktree error = %v, want actionable %q error", err, tc.wantErr)
			}
			if got := r.resolveRef("HEAD"); got != headBefore {
				t.Fatalf("HEAD changed: got %s, want %s", got, headBefore)
			}
			if got := r.runGit("status", "--porcelain=v1", "--untracked-files=all"); got != statusBefore {
				t.Fatalf("worktree/index status mutated:\nBEFORE:\n%s\nAFTER:\n%s", statusBefore, got)
			}
			if got := r.runGit("diff", "--cached", "--binary"); got != indexBefore {
				t.Fatalf("staged diff mutated:\nBEFORE:\n%s\nAFTER:\n%s", indexBefore, got)
			}
			if got := readGitExclude(t, r); string(got) != string(excludeBefore) {
				t.Fatalf("info/exclude mutated on invariant failure:\nBEFORE:\n%s\nAFTER:\n%s", excludeBefore, got)
			}
			gotBytes, readErr := os.ReadFile(evidenceFile)
			if readErr != nil {
				t.Fatalf("reading evidence after failure: %v", readErr)
			}
			if string(gotBytes) != string(wantBytes) {
				t.Fatalf("evidence bytes changed: got %q, want %q", gotBytes, wantBytes)
			}
		})
	}
}

func TestLandRejectsCandidateContainingExecutionEvidence(t *testing.T) {
	r := newLandTestRepo(t)
	r.runGit("rm", ".gitignore")
	r.runGit("commit", "-m", "remove execution ignore coverage")
	baseRev := r.resolveRef("HEAD")
	resultRev := r.commitOnFiles(baseRev, "feat: forbidden evidence candidate", map[string]string{
		"feature.txt": "feature\n",
		".ddx/executions/attempt/custom-report.md": "local report\n",
	})

	_, err := Land(r.dir, LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      baseRev,
		ResultRev:    resultRev,
		BeadID:       "ddx-forbidden-evidence",
		AttemptID:    "20260715T131000-candidate",
		TargetBranch: "main",
	}, RealLandingGitOps{})
	if err == nil || !strings.Contains(err.Error(), "local execution evidence path") {
		t.Fatalf("Land error = %v, want local evidence invariant rejection", err)
	}
	if got := r.resolveRef("refs/heads/main"); got != baseRev {
		t.Fatalf("main moved despite rejected candidate: got %s, want %s", got, baseRev)
	}
	assertNoExecutionEvidenceOnBranch(t, r, "main")
}

func TestLandRejectsCandidateWhoseHistoryAddedThenDeletedExecutionEvidence(t *testing.T) {
	r := newLandTestRepo(t)
	r.runGit("rm", ".gitignore")
	r.runGit("commit", "-m", "remove execution ignore coverage")
	baseRev := r.resolveRef("HEAD")
	evidencePath := ".ddx/executions/attempt/transient-report.md"
	addRev := r.commitOnFiles(baseRev, "feat: add transient evidence", map[string]string{
		"feature.txt": "feature\n",
		evidencePath:  "local report\n",
	})
	resultRev := r.commitDeleteOn(addRev, evidencePath, "chore: delete transient evidence")

	if got := r.runGit("diff", "--name-only", baseRev+".."+resultRev, "--", ExecuteBeadArtifactDir); got != "" {
		t.Fatalf("test precondition failed: final candidate tree still changes evidence: %s", got)
	}
	_, err := Land(r.dir, LandRequest{
		WorktreeDir:  r.dir,
		BaseRev:      baseRev,
		ResultRev:    resultRev,
		BeadID:       "ddx-transient-evidence",
		AttemptID:    "20260715T131100-add-delete",
		TargetBranch: "main",
	}, RealLandingGitOps{})
	if err == nil || !strings.Contains(err.Error(), "candidate history commit") {
		t.Fatalf("Land error = %v, want add-then-delete history rejection", err)
	}
	if got := r.resolveRef("refs/heads/main"); got != baseRev {
		t.Fatalf("main moved despite rejected add-then-delete history: got %s, want %s", got, baseRev)
	}
	assertNoExecutionEvidenceOnBranch(t, r, "main")
}

func TestVerifyCleanWorktree_ConcurrentLocalExcludeInstallIsIdempotent(t *testing.T) {
	r := newLandTestRepo(t)
	r.runGit("rm", ".gitignore")
	r.runGit("commit", "-m", "remove project gitignore")

	const workers = 12
	attemptIDs := make([]string, workers)
	for i := range workers {
		attemptIDs[i] = fmt.Sprintf("20260715T1202%02d-concurrent", i)
		dir := filepath.Join(r.dir, ExecuteBeadArtifactDir, attemptIDs[i])
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeEvidenceFile(t, dir, "result.json", `{"status":"preserved"}`)
	}

	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for _, attemptID := range attemptIDs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- VerifyCleanWorktree(r.dir, attemptID)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent VerifyCleanWorktree: %v", err)
		}
	}

	excludeBytes := readGitExclude(t, r)
	if count := countExactLines(string(excludeBytes), executionEvidenceLocalExclude); count != 1 {
		t.Fatalf("concurrent local exclude rule count = %d, want 1; contents:\n%s", count, excludeBytes)
	}
	if got := r.runGit("status", "--porcelain", "--untracked-files=all", "--", ExecuteBeadArtifactDir); got != "" {
		t.Fatalf("concurrent evidence paths remain visible in status:\n%s", got)
	}
}

func TestPreHarnessEvidenceExcludeResolvesRepositoryFromSubdirectory(t *testing.T) {
	r := newLandTestRepo(t)
	subdir := filepath.Join(r.dir, "nested", "workspace")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureExecutionEvidenceLocalExcludeForWorkspace(subdir); err != nil {
		t.Fatalf("installing local exclude from repository subdirectory: %v", err)
	}
	if count := countExactLines(string(readGitExclude(t, r)), executionEvidenceLocalExclude); count != 1 {
		t.Fatalf("repository-local exclude count = %d, want 1", count)
	}
}

// TestEvidenceRetentionErrorsAreNeverDiscarded is a source-level regression
// guard over the three production execution entry points. Runtime injection at
// this layer would duplicate each command/server fixture; the contract being
// guarded is structural: every helper call must branch on and return its error.
func TestEvidenceRetentionErrorsAreNeverDiscarded(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	agentDir := filepath.Dir(thisFile)
	for _, target := range []struct {
		path            string
		landingBoundary string
	}{
		{filepath.Join(agentDir, "..", "..", "cmd", "execute_loop_shared.go"), "if prepareCandidateCycleLanding(res)"},
		{filepath.Join(agentDir, "..", "..", "cmd", "try.go"), "if prepareCandidateCycleLanding(res)"},
		{filepath.Join(agentDir, "..", "server", "workers.go"), "operatorCancel :="},
	} {
		source, err := os.ReadFile(target.path)
		if err != nil {
			t.Fatalf("read %s: %v", target.path, err)
		}
		text := string(source)
		if strings.Contains(text, "_ = agent.VerifyCleanWorktree") {
			t.Errorf("%s discards VerifyCleanWorktree errors", target.path)
		}
		verifyAt := strings.Index(text, "if retentionErr := agent.VerifyCleanWorktree")
		if verifyAt < 0 {
			t.Errorf("%s does not branch on VerifyCleanWorktree errors", target.path)
			continue
		}
		if !strings.Contains(text, `fmt.Errorf("retaining local execution evidence: %w", retentionErr)`) {
			t.Errorf("%s does not wrap and surface VerifyCleanWorktree errors", target.path)
		}
		if !strings.Contains(text[verifyAt:], "errors.Join(") {
			t.Errorf("%s does not preserve an existing execution error with the retention failure", target.path)
		}
		boundaryAt := strings.Index(text[verifyAt:], target.landingBoundary)
		if boundaryAt < 0 {
			t.Errorf("%s does not verify evidence before landing boundary %q", target.path, target.landingBoundary)
		}
	}
}

func readGitExclude(t *testing.T, r *landTestRepo) []byte {
	t.Helper()
	excludePath := r.runGit("rev-parse", "--git-path", "info/exclude")
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(r.dir, filepath.FromSlash(excludePath))
	}
	content, err := os.ReadFile(excludePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read Git info/exclude: %v", err)
	}
	return content
}

func countExactLines(content, want string) int {
	count := 0
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == want {
			count++
		}
	}
	return count
}

func writeEvidenceFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
