package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/require"
)

type fakeExecuteBeadGit struct {
	mu sync.Mutex

	mainHeadRev  string
	headRevSeq   []string
	headRevIdx   int
	wtHeadRev    string
	wtDirty      bool
	synthRev     string
	wtHeadRevErr error
	dirty        bool
	mergeErr     error
	updateRefErr error

	addedWTs   []string
	addedWTRev string
	removedWTs []string
	refs       map[string]string
	worktrees  []string

	mergeCalls int
	mergeRev   string
}

func (f *fakeExecuteBeadGit) HeadRev(dir string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Contains(dir, agent.ExecuteBeadWtPrefix) {
		if f.wtHeadRevErr != nil {
			return "", f.wtHeadRevErr
		}
		return f.wtHeadRev, nil
	}
	if len(f.headRevSeq) > 0 {
		idx := f.headRevIdx
		if idx >= len(f.headRevSeq) {
			idx = len(f.headRevSeq) - 1
		}
		rev := f.headRevSeq[idx]
		f.headRevIdx++
		return rev, nil
	}
	return f.mainHeadRev, nil
}

func (f *fakeExecuteBeadGit) ResolveRev(dir, rev string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mainHeadRev, nil
}

func (f *fakeExecuteBeadGit) IsDirty(dir string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Contains(dir, agent.ExecuteBeadWtPrefix) {
		return f.wtDirty, nil
	}
	return f.dirty, nil
}

func (f *fakeExecuteBeadGit) WorktreeAdd(dir, wtPath, rev string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addedWTs = append(f.addedWTs, wtPath)
	f.addedWTRev = rev
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		return err
	}
	beadFile := filepath.Join(dir, ddxroot.DirName, "beads.jsonl")
	if _, err := os.Stat(beadFile); err == nil {
		if err := copyTestFile(beadFile, filepath.Join(wtPath, ddxroot.DirName, "beads.jsonl")); err != nil {
			return err
		}
	}
	docsDir := filepath.Join(dir, "docs")
	if _, err := os.Stat(docsDir); err == nil {
		if err := copyTree(docsDir, filepath.Join(wtPath, "docs")); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeExecuteBeadGit) WorktreeRemove(dir, wtPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removedWTs = append(f.removedWTs, wtPath)
	return os.RemoveAll(wtPath)
}

func (f *fakeExecuteBeadGit) WorktreeList(dir string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.worktrees, nil
}

func (f *fakeExecuteBeadGit) SynthesizeCommit(dir, msg string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Contains(dir, agent.ExecuteBeadWtPrefix) && f.synthRev != "" {
		f.wtHeadRev = f.synthRev
		return true, nil
	}
	return false, nil
}

func (f *fakeExecuteBeadGit) WorktreePrune(dir string) error { return nil }

func (f *fakeExecuteBeadGit) Merge(dir, rev string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mergeCalls++
	f.mergeRev = rev
	return f.mergeErr
}

func (f *fakeExecuteBeadGit) UpdateRef(dir, ref, sha string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updateRefErr != nil {
		return f.updateRefErr
	}
	if f.refs == nil {
		f.refs = make(map[string]string)
	}
	f.refs[ref] = sha
	return nil
}

func (f *fakeExecuteBeadGit) DeleteRef(dir, ref string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refs != nil {
		delete(f.refs, ref)
	}
	return nil
}

type fakeAgentRunner struct {
	mu         sync.Mutex
	result     *agent.Result
	err        error
	last       agent.RunArgs
	sideEffect func(opts agent.RunArgs) error
}

func (r *fakeAgentRunner) Run(opts agent.RunArgs) (*agent.Result, error) {
	r.mu.Lock()
	r.last = opts
	result := r.result
	runErr := r.err
	sideEffect := r.sideEffect
	r.mu.Unlock()
	if sideEffect != nil {
		if err := sideEffect(opts); err != nil {
			return nil, err
		}
	}
	return result, runErr
}

func newExecuteBeadFactory(t *testing.T, git *fakeExecuteBeadGit, runner *fakeAgentRunner) *CommandFactory {
	t.Helper()
	f := NewCommandFactory(t.TempDir())
	seedDefaultExecuteBeads(t, f.WorkingDir)
	f.AgentRunnerOverride = runner
	f.executeBeadGitOverride = git
	f.executeBeadOrchestratorGitOverride = git
	f.executeBeadLandingAdvancerOverride = fakeLandingAdvancerFromGit(git)
	return f
}

func fakeLandingAdvancerFromGit(git *fakeExecuteBeadGit) func(res *agent.ExecuteBeadResult) (*agent.LandResult, error) {
	return func(res *agent.ExecuteBeadResult) (*agent.LandResult, error) {
		if err := git.Merge("", res.ResultRev); err != nil {
			preserveRef := agent.PreserveRef(res.BeadID, res.BaseRev)
			_ = git.UpdateRef("", preserveRef, res.ResultRev)
			return &agent.LandResult{
				Status:      "preserved",
				PreserveRef: preserveRef,
				Reason:      "merge failed",
			}, nil
		}
		return &agent.LandResult{Status: "landed", NewTip: res.ResultRev}, nil
	}
}

func assertPreserveRef(t *testing.T, ref, beadID, baseRev string) {
	t.Helper()
	shortSHA := baseRev
	if len(shortSHA) > 12 {
		shortSHA = shortSHA[:12]
	}
	pattern := fmt.Sprintf(`^refs/ddx/iterations/%s/\d{8}T\d{6}Z-%s$`,
		regexp.QuoteMeta(beadID), regexp.QuoteMeta(shortSHA))
	require.Regexp(t, pattern, ref)
}

func runExecuteBead(t *testing.T, f *CommandFactory, git *fakeExecuteBeadGit, beadID string, extraArgs ...string) agent.ExecuteBeadResult {
	t.Helper()

	rcfg, err := config.LoadAndResolve(f.WorkingDir, config.CLIOverrides{})
	require.NoError(t, err)

	runner := f.AgentRunnerOverride
	if runner == nil {
		runner = &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
	}

	res, err := agent.ExecuteBeadWithConfig(context.Background(), f.WorkingDir, beadID, rcfg, agent.ExecuteBeadRuntime{
		AgentRunner: runner,
	}, git)
	require.NoError(t, err, "execute-bead engine should not return an error")
	if fr, ok := runner.(*fakeAgentRunner); ok && fr.result != nil {
		appendTestRoutingEvidence(t, f.WorkingDir, beadID, fr.result.Harness, fr.result.Provider, fr.result.Model, fr.result.RouteReason, fr.result.ResolvedBaseURL)
	}
	if res.ResultRev != "" && res.ResultRev != res.BaseRev && res.ExitCode == 0 {
		res.Outcome = "merged"
		res.Status = agent.ExecuteBeadStatusSuccess
	} else if res.ResultRev == res.BaseRev {
		res.Outcome = "no-evidence"
	}
	return *res
}

func parseExecuteBeadJSON(t *testing.T, out string) agent.ExecuteBeadResult {
	t.Helper()
	jsonStart := strings.Index(out, "{")
	require.NotEqual(t, -1, jsonStart, "output should contain JSON: %s", out)
	jsonPart := out[jsonStart:]
	var res agent.ExecuteBeadResult
	dec := json.NewDecoder(bytes.NewBufferString(jsonPart))
	require.NoError(t, dec.Decode(&res), "output should be valid JSON: %s", jsonPart)
	return res
}

func seedExecuteBead(t *testing.T, workDir string, b *bead.Bead) {
	t.Helper()
	store := bead.NewStore(filepath.Join(workDir, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	if _, err := store.Get(b.ID); err == nil {
		return
	}
	require.NoError(t, store.Create(context.Background(), b))
}

func seedDefaultExecuteBeads(t *testing.T, workDir string) {
	t.Helper()
	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "my-bead",
		Title:     "Test execute-bead",
		Status:    bead.StatusOpen,
		Priority:  0,
		IssueType: bead.DefaultType,
	})
	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "shared-bead",
		Title:     "Shared execute-bead",
		Status:    bead.StatusOpen,
		Priority:  0,
		IssueType: bead.DefaultType,
	})
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return os.Chmod(target, info.Mode())
	})
}

func copyTestFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, info.Mode())
}
