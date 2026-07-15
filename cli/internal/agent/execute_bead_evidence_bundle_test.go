package agent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/require"
)

type evidenceAgentRunnerFunc func(opts RunArgs) (*Result, error)

func (f evidenceAgentRunnerFunc) Run(opts RunArgs) (*Result, error) { return f(opts) }

type retainingAttemptBackend struct {
	inner     AttemptBackend
	workspace *AttemptWorkspace
}

func (b *retainingAttemptBackend) Name() string { return b.inner.Name() }
func (b *retainingAttemptBackend) Prepare(ctx context.Context, req AttemptBackendPrepareRequest) (*AttemptWorkspace, error) {
	workspace, err := b.inner.Prepare(ctx, req)
	b.workspace = workspace
	return workspace, err
}
func (b *retainingAttemptBackend) Run(ctx context.Context, req AttemptBackendRunRequest) (*Result, error) {
	return b.inner.Run(ctx, req)
}
func (b *retainingAttemptBackend) PublishResult(ctx context.Context, workspace *AttemptWorkspace, res *ExecuteBeadResult) error {
	return b.inner.PublishResult(ctx, workspace, res)
}
func (b *retainingAttemptBackend) ImportCandidate(ctx context.Context, workspace *AttemptWorkspace, res *ExecuteBeadResult) error {
	return b.inner.ImportCandidate(ctx, workspace, res)
}
func (b *retainingAttemptBackend) ReleaseCandidateImport(ctx context.Context, workspace *AttemptWorkspace) error {
	return b.inner.ReleaseCandidateImport(ctx, workspace)
}
func (*retainingAttemptBackend) Cleanup(context.Context, *AttemptWorkspace) error { return nil }

type failingPublishAttemptBackend struct {
	inner AttemptBackend
	err   error
}

func (b failingPublishAttemptBackend) Name() string { return b.inner.Name() }
func (b failingPublishAttemptBackend) Prepare(ctx context.Context, req AttemptBackendPrepareRequest) (*AttemptWorkspace, error) {
	return b.inner.Prepare(ctx, req)
}
func (b failingPublishAttemptBackend) Run(ctx context.Context, req AttemptBackendRunRequest) (*Result, error) {
	return b.inner.Run(ctx, req)
}
func (b failingPublishAttemptBackend) PublishResult(context.Context, *AttemptWorkspace, *ExecuteBeadResult) error {
	return b.err
}
func (b failingPublishAttemptBackend) ImportCandidate(ctx context.Context, workspace *AttemptWorkspace, res *ExecuteBeadResult) error {
	return b.inner.ImportCandidate(ctx, workspace, res)
}
func (b failingPublishAttemptBackend) ReleaseCandidateImport(ctx context.Context, workspace *AttemptWorkspace) error {
	return b.inner.ReleaseCandidateImport(ctx, workspace)
}
func (b failingPublishAttemptBackend) Cleanup(ctx context.Context, workspace *AttemptWorkspace) error {
	return b.inner.Cleanup(ctx, workspace)
}

type failAttemptHeadGitOps struct {
	*RealGitOps
	projectRoot string
}

func (g *failAttemptHeadGitOps) HeadRev(dir string) (string, error) {
	if sameFilesystemPath(dir, g.projectRoot) {
		return g.RealGitOps.HeadRev(dir)
	}
	return "", errors.New("injected attempt HEAD failure")
}

// TestExecuteBeadArtifacts_BundleCreatedInAttemptWorktree proves that
// createArtifactBundle places DirAbs (and all artifact absolute paths) inside
// the isolated attempt worktree, not under the main project root.
func TestExecuteBeadArtifacts_BundleCreatedInAttemptWorktree(t *testing.T) {
	root := t.TempDir()
	wt := t.TempDir()
	const attemptID = "20260508T000000-aabbccdd"

	arts, err := createArtifactBundle(root, wt, attemptID)
	if err != nil {
		t.Fatalf("createArtifactBundle: %v", err)
	}

	// DirAbs must be inside the worktree, not the project root.
	wtAbs, _ := filepath.Abs(wt)
	dirAbs, _ := filepath.Abs(arts.DirAbs)
	if !strings.HasPrefix(dirAbs, wtAbs+string(filepath.Separator)) && dirAbs != wtAbs {
		t.Errorf("DirAbs %q must be under worktree %q", arts.DirAbs, wt)
	}
	rootAbs, _ := filepath.Abs(root)
	if strings.HasPrefix(dirAbs, rootAbs+string(filepath.Separator)) {
		t.Errorf("DirAbs %q must NOT be under project root %q", arts.DirAbs, root)
	}

	// DirRel is unchanged: relative path from repo root.
	wantDirRel := ".ddx/executions/" + attemptID
	if arts.DirRel != wantDirRel {
		t.Errorf("DirRel = %q, want %q", arts.DirRel, wantDirRel)
	}

	// Bundle directory created at worktree path.
	if fi, err := os.Stat(filepath.Join(wt, ExecuteBeadArtifactDir, attemptID)); err != nil || !fi.IsDir() {
		t.Errorf("bundle dir not created at worktree path: %v", err)
	}
	// No bundle directory created at project root.
	if _, err := os.Stat(filepath.Join(root, ExecuteBeadArtifactDir, attemptID)); err == nil {
		t.Errorf("bundle dir must NOT be created at project root %s", root)
	}
}

// TestExecuteBead_NoLandPreservesEvidenceWithoutMainNoise proves that
// no-change/no-evidence/no-land outcomes publish the evidence bundle through an
// explicit controlled-copy path (publishEvidenceBundleToProjectRoot) and, after
// VerifyCleanWorktree confirms its local exclusion, leave the main worktree
// clean.
func TestExecuteBead_NoLandPreservesEvidenceWithoutMainNoise(t *testing.T) {
	r := newLandTestRepo(t)
	const attemptID = "20260508T000000-nolnd123"
	dirRel := filepath.ToSlash(filepath.Join(ExecuteBeadArtifactDir, attemptID))

	// Simulate the attempt worktree having evidence (but no agent commits).
	wt := t.TempDir()
	bundleDir := filepath.Join(wt, ExecuteBeadArtifactDir, attemptID)
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeEvidenceFile(t, bundleDir, "manifest.json", `{"attempt_id":"`+attemptID+`"}`)
	writeEvidenceFile(t, bundleDir, "result.json", `{"status":"no_changes"}`)
	writeEvidenceFile(t, bundleDir, "no_changes_rationale.txt", "verification_command: true\n")

	// Step 1: controlled publish (simulates what the deferred publish does for
	// no-land paths before the worktree is removed).
	if err := publishEvidenceBundleToProjectRoot(r.dir, wt, dirRel); err != nil {
		t.Fatalf("publishEvidenceBundleToProjectRoot: %v", err)
	}

	// After publish, evidence must be in project root (untracked at this point).
	if _, err := os.Stat(filepath.Join(r.dir, ExecuteBeadArtifactDir, attemptID)); err != nil {
		t.Fatalf("evidence not published to project root: %v", err)
	}

	// Step 2: VerifyCleanWorktree leaves gitignored evidence untracked (it must
	// never be committed — ddx-d10073a8).
	if err := VerifyCleanWorktree(r.dir, attemptID); err != nil {
		t.Fatalf("VerifyCleanWorktree: %v", err)
	}

	// The main working tree is clean: evidence is gitignored, so neither dirty
	// nor committed.
	statusOut, _, _ := runGitStatus(r.dir, dirRel)
	if strings.TrimSpace(statusOut) != "" {
		t.Errorf("main worktree not clean after VerifyCleanWorktree:\n%s", statusOut)
	}

	// Evidence is preserved on disk but must NEVER be committed.
	if _, err := os.Stat(filepath.Join(r.dir, ExecuteBeadArtifactDir, attemptID, "manifest.json")); err != nil {
		t.Errorf("evidence must remain on disk: %v", err)
	}
	logOut := r.runGit("log", "--oneline", "--name-only", "HEAD")
	evidenceManifestPath := filepath.ToSlash(filepath.Join(dirRel, "manifest.json"))
	if strings.Contains(logOut, evidenceManifestPath) {
		t.Errorf("evidence manifest must NOT be committed:\n%s", logOut)
	}
}

func TestPublishEvidenceFailurePreservesAttemptWorktreeAndSurfacesError(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	directivePath := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, directivePath, []string{"no-op"})
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: directivePath}).Resolve(config.CLIOverrides{Harness: "script"})

	copyCalls := 0
	injectedErr := errors.New("injected mid-copy execution evidence failure")
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
		AgentRunner: scriptHarnessAgentRunner{},
		NoReview:    true,
		EvidenceFileCopier: func(source, target string, mode os.FileMode) error {
			copyCalls++
			if copyCalls == 2 {
				return injectedErr
			}
			return copyLocalExecutionEvidenceFile(source, target, mode)
		},
	}, &RealGitOps{})
	if err == nil || !strings.Contains(err.Error(), injectedErr.Error()) {
		t.Fatalf("ExecuteBeadWithConfig error = %v, want injected publication failure", err)
	}
	if copyCalls < 2 {
		t.Fatalf("copy calls = %d, want a failure after at least one successful file copy", copyCalls)
	}
	if res == nil {
		t.Fatal("publication failure must return the attempt result")
	}
	if !strings.Contains(res.Error, injectedErr.Error()) {
		t.Fatalf("result error = %q, want publication failure", res.Error)
	}
	if res.Outcome != ExecuteBeadOutcomeTaskFailed || res.Status != ExecuteBeadStatusExecutionFailed {
		t.Fatalf("publication failure result = outcome %q status %q", res.Outcome, res.Status)
	}
	if res.WorktreePath == "" {
		t.Fatal("publication failure result must identify the preserved source worktree")
	}
	if info, statErr := os.Stat(res.WorktreePath); statErr != nil || !info.IsDir() {
		t.Fatalf("source worktree was not preserved: %v", statErr)
	}

	sourceBundle := filepath.Join(res.WorktreePath, filepath.FromSlash(res.ExecutionDir))
	for _, name := range []string{"manifest.json", "prompt.md", "result.json"} {
		path := filepath.Join(sourceBundle, name)
		if _, statErr := os.Stat(path); statErr != nil {
			t.Errorf("complete source bundle missing %s: %v", name, statErr)
		}
	}
	resultBytes, readErr := os.ReadFile(filepath.Join(sourceBundle, "result.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(resultBytes), injectedErr.Error()) {
		t.Fatalf("source result.json does not record publication failure: %s", resultBytes)
	}

	destination := filepath.Join(projectRoot, filepath.FromSlash(res.ExecutionDir))
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("partial destination was accepted at %s: %v", destination, statErr)
	}
	entries, readDirErr := os.ReadDir(filepath.Dir(destination))
	if readDirErr != nil && !os.IsNotExist(readDirErr) {
		t.Fatal(readDirErr)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), filepath.Base(destination)+".publish-") {
			t.Errorf("partial staging directory leaked after failed publication: %s", entry.Name())
		}
	}
}

func TestPublishResultFailureReturnsErrorWithoutMovingTarget(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	mainBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	directivePath := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, directivePath, []string{
		"create-file publish-result.txt valid-candidate",
		"commit feat: create publish-result candidate",
	})
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: directivePath}).Resolve(config.CLIOverrides{Harness: "script"})
	publishErr := errors.New("injected post-bundle PublishResult failure")

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
		AgentRunner: scriptHarnessAgentRunner{},
		NoReview:    true,
		AttemptBackend: failingPublishAttemptBackend{
			inner: WorktreeAttemptBackend{},
			err:   publishErr,
		},
	}, &RealGitOps{})
	require.ErrorContains(t, err, publishErr.Error())
	require.NotNil(t, res)
	require.NotEqual(t, res.BaseRev, res.ResultRev, "test requires a mergeable changed candidate")
	require.Equal(t, ExecuteBeadOutcomeTaskSucceeded, res.Outcome, "server must honor the paired error even if result still looks successful")
	require.Equal(t, mainBefore, runGitInteg(t, projectRoot, "rev-parse", "HEAD"), "failed publication must not move the target")
}

func TestEarlyResultPublicationFailureDemotesResultAndPreservesSource(t *testing.T) {
	tests := []struct {
		name   string
		beadID string
		gitOps func(projectRoot string) GitOps
		failAt int
	}{
		{
			name:   "context load failure",
			beadID: "ddx-missing-context",
			gitOps: func(string) GitOps { return &RealGitOps{} },
			failAt: 1,
		},
		{
			name:   "attempt HEAD failure",
			beadID: "ddx-int-0001",
			gitOps: func(projectRoot string) GitOps {
				return &failAttemptHeadGitOps{RealGitOps: &RealGitOps{}, projectRoot: projectRoot}
			},
			failAt: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectRoot, _ := newScriptHarnessRepo(t, 1)
			directivePath := filepath.Join(t.TempDir(), "directive.txt")
			writeDirectiveFile(t, directivePath, []string{"no-op"})
			rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: directivePath}).Resolve(config.CLIOverrides{Harness: "script"})
			copyCalls := 0
			injectedErr := errors.New("injected early-result evidence publication failure")

			res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, tt.beadID, rcfg, ExecuteBeadRuntime{
				AgentRunner: scriptHarnessAgentRunner{},
				NoReview:    true,
				EvidenceFileCopier: func(source, target string, mode os.FileMode) error {
					copyCalls++
					if copyCalls == tt.failAt {
						return injectedErr
					}
					return copyLocalExecutionEvidenceFile(source, target, mode)
				},
			}, tt.gitOps(projectRoot))
			require.ErrorContains(t, err, injectedErr.Error())
			require.NotNil(t, res)
			require.Equal(t, ExecuteBeadOutcomeTaskFailed, res.Outcome)
			require.Equal(t, ExecuteBeadStatusExecutionFailed, res.Status)
			require.Contains(t, res.Error, injectedErr.Error())
			require.NotEmpty(t, res.WorktreePath)
			info, statErr := os.Stat(res.WorktreePath)
			require.NoError(t, statErr)
			require.True(t, info.IsDir(), "recovery source must be preserved")

			resultPath := filepath.Join(res.WorktreePath, filepath.FromSlash(res.ExecutionDir), "result.json")
			resultBytes, readErr := os.ReadFile(resultPath)
			require.NoError(t, readErr)
			require.Contains(t, string(resultBytes), injectedErr.Error())
		})
	}
}

func TestExecuteBeadScriptHarnessForceAddedEvidenceNeverPublishesCandidate(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	headBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	directivePath := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, directivePath, []string{
		"create-file feature.txt implementation",
		"run mkdir -p .ddx/executions/$DDX_ATTEMPT_ID && printf local-report > .ddx/executions/$DDX_ATTEMPT_ID/custom-report.md",
		"run git add -f .ddx/executions/$DDX_ATTEMPT_ID/custom-report.md",
		"commit feat: force-add escape attempt",
	})
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: directivePath}).Resolve(config.CLIOverrides{Harness: "script"})

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
		AgentRunner: scriptHarnessAgentRunner{},
		NoReview:    true,
	}, &RealGitOps{})
	if err == nil || !strings.Contains(err.Error(), "candidate history commit") {
		t.Fatalf("ExecuteBeadWithConfig error = %v, want candidate-history rejection", err)
	}
	if res == nil {
		t.Fatal("force-added candidate rejection must return an attempt result")
	}
	if res.FailureMode != FailureModeAttemptIntegrity {
		t.Fatalf("failure mode = %q, want %q", res.FailureMode, FailureModeAttemptIntegrity)
	}
	if res.ResultRev == "" || res.ResultRev == res.BaseRev {
		t.Fatalf("test precondition failed: script harness did not create a candidate commit: %+v", res)
	}
	if got := runGitInteg(t, projectRoot, "rev-parse", "HEAD"); got != headBefore {
		t.Fatalf("main moved after rejected force-added candidate: got %s want %s", got, headBefore)
	}
	if out := runGitInteg(t, projectRoot, "log", "HEAD", "--format=%H", "--", ".ddx/executions"); out != "" {
		t.Fatalf("execution evidence reached main history: %s", out)
	}
	evidencePath := filepath.Join(projectRoot, filepath.FromSlash(res.ExecutionDir), "custom-report.md")
	data, readErr := os.ReadFile(evidencePath)
	if readErr != nil || string(data) != "local-report" {
		t.Fatalf("local report was not retained on disk: data=%q err=%v", data, readErr)
	}
}

func TestExecuteBeadNilResultPublicationFailurePreservesSource(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	directivePath := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, directivePath, []string{"no-op"})
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: directivePath}).Resolve(config.CLIOverrides{Harness: "script"})

	var sourceWorktree string
	runner := evidenceAgentRunnerFunc(func(opts RunArgs) (*Result, error) {
		result, runErr := runScriptFn(opts)
		sourceWorktree = opts.WorkDir
		resultPath := filepath.Join(opts.WorkDir, filepath.FromSlash(opts.Correlation["bundle_path"]), "result.json")
		_ = os.RemoveAll(resultPath)
		if err := os.MkdirAll(resultPath, 0o755); err != nil {
			return nil, err
		}
		return result, runErr
	})
	copyCalls := 0
	injectedErr := errors.New("injected nil-result publication failure")
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
		AgentRunner: runner,
		NoReview:    true,
		EvidenceFileCopier: func(source, target string, mode os.FileMode) error {
			copyCalls++
			if copyCalls == 2 {
				return injectedErr
			}
			return copyLocalExecutionEvidenceFile(source, target, mode)
		},
	}, &RealGitOps{})
	if res != nil {
		t.Fatalf("test precondition failed: expected nil result path, got %+v", res)
	}
	if err == nil || !strings.Contains(err.Error(), "writing execute-bead result artifact") || !strings.Contains(err.Error(), injectedErr.Error()) {
		t.Fatalf("joined nil-result error = %v", err)
	}
	if sourceWorktree == "" {
		t.Fatal("runner did not capture the attempt worktree")
	}
	if info, statErr := os.Stat(sourceWorktree); statErr != nil || !info.IsDir() {
		t.Fatalf("nil-result publication failure removed its recovery source: %v", statErr)
	}
	if copyCalls < 2 {
		t.Fatalf("copy calls = %d, want mid-copy failure", copyCalls)
	}
}

func TestExecuteBeadLocalCloneInstallsRootAndAttemptEvidenceExcludesBeforePublication(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	directivePath := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, directivePath, []string{"no-op"})
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: directivePath}).Resolve(config.CLIOverrides{
		Harness:        "script",
		AttemptBackend: AttemptBackendLocalClone,
	})
	backend := &retainingAttemptBackend{inner: LocalCloneAttemptBackend{}}
	statusDuringPublication := "not observed"

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
		AgentRunner:    scriptHarnessAgentRunner{},
		NoReview:       true,
		AttemptBackend: backend,
		EvidenceFileCopier: func(source, target string, mode os.FileMode) error {
			if statusDuringPublication == "not observed" {
				out, statusErr := exec.Command("git", "-C", projectRoot, "status", "--porcelain", "--untracked-files=all", "--", ExecuteBeadArtifactDir).CombinedOutput()
				if statusErr != nil {
					statusDuringPublication = "git status failed: " + statusErr.Error()
				} else {
					statusDuringPublication = strings.TrimSpace(string(out))
				}
			}
			return copyLocalExecutionEvidenceFile(source, target, mode)
		},
	}, &RealGitOps{})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || backend.workspace == nil {
		t.Fatalf("local-clone attempt did not produce a result/workspace: res=%+v workspace=%+v", res, backend.workspace)
	}
	if statusDuringPublication != "" {
		t.Fatalf("project-root Git observed in-flight local evidence publication: %q", statusDuringPublication)
	}
	for name, repo := range map[string]string{
		"project root":  projectRoot,
		"attempt clone": backend.workspace.WorkDir,
	} {
		pathOut, pathErr := exec.Command("git", "-C", repo, "rev-parse", "--git-path", "info/exclude").Output()
		if pathErr != nil {
			t.Fatalf("%s exclude path: %v", name, pathErr)
		}
		excludePath := strings.TrimSpace(string(pathOut))
		if !filepath.IsAbs(excludePath) {
			excludePath = filepath.Join(repo, excludePath)
		}
		content, readErr := os.ReadFile(excludePath)
		if readErr != nil || countExactLines(string(content), executionEvidenceLocalExclude) != 1 {
			t.Fatalf("%s local exclude = %q err=%v", name, content, readErr)
		}
	}
	if out := runGitInteg(t, projectRoot, "status", "--porcelain", "--untracked-files=all", "--", ExecuteBeadArtifactDir); out != "" {
		t.Fatalf("published evidence remains visible at project root: %s", out)
	}
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// runGitStatus runs git status --porcelain for dirRel and returns (output, stdout, error).
func runGitStatus(dir, dirRel string) (string, string, error) {
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain", "--", dirRel)
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	return s, s, err
}
