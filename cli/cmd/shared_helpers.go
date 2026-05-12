package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
)

// resolveProjectRoot resolves the target project root from CLI flags,
// environment, or the current working directory.
func resolveProjectRoot(projectFlag, workingDir string) string {
	if projectFlag != "" {
		return projectFlag
	}
	if env := os.Getenv("DDX_PROJECT_ROOT"); env != "" {
		return env
	}
	return gitpkg.FindProjectRoot(workingDir)
}

type preClaimGitOps interface {
	CurrentBranch(dir string) (string, error)
	FetchOriginAncestryCheck(dir, targetBranch string) (agent.PreClaimResult, error)
}

// buildCLIPreClaimHook returns a PreClaimHook for inline queue work
// that fetches origin and verifies ancestry before each bead claim.
func buildCLIPreClaimHook(projectRoot string, gitOps preClaimGitOps) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		branch, err := gitOps.CurrentBranch(projectRoot)
		if err != nil {
			return nil // can't determine branch — skip
		}
		res, err := gitOps.FetchOriginAncestryCheck(projectRoot, branch)
		if err != nil {
			if !agent.IsIgnorableFetchOriginError(err) {
				return err
			}
			return nil
		}
		if res.Action == "diverged" {
			return fmt.Errorf("local branch %s has diverged from origin (local=%s origin=%s); reconcile manually before claiming",
				branch, res.LocalSHA, res.OriginSHA)
		}
		return nil
	}
}

func buildCLIResourceChecker(projectRoot string, override agent.ExecutionResourceChecker) agent.ExecutionResourceChecker {
	if override != nil {
		return override
	}
	return agent.NewExecutionResourceChecker(projectRoot, &agent.RealGitOps{})
}

// resolveServerURL determines the base URL for the running DDx server.
func resolveServerURL(projectRoot string) string {
	if u := os.Getenv("DDX_SERVER_URL"); u != "" {
		return u
	}
	if u := serverpkg.ReadServerAddr(); u != "" {
		return rewriteBindAddrForClient(u)
	}
	return "https://127.0.0.1:7743"
}

func rewriteBindAddrForClient(u string) string {
	for _, bind := range []string{"//0.0.0.0:", "//[::]:", "//[::0]:"} {
		if idx := strings.Index(u, bind); idx >= 0 {
			return u[:idx] + "//127.0.0.1:" + u[idx+len(bind):]
		}
	}
	return u
}

// newLocalServerClient returns an http.Client configured for the local DDx server.
func newLocalServerClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // local self-signed cert
		},
	}
}

// resolveWorktree creates or reuses a named worktree under .worktrees/.
func resolveWorktree(repoRoot, name string) (string, error) {
	if repoRoot == "" {
		out, err := gitpkg.Command(context.Background(), "", "rev-parse", "--show-toplevel").Output()
		if err != nil {
			return "", fmt.Errorf("cannot detect repo root: %w", err)
		}
		repoRoot = strings.TrimSpace(string(out))
	}

	wtDir := filepath.Join(repoRoot, ".worktrees", name)
	if _, err := os.Stat(wtDir); err == nil {
		return wtDir, nil
	}

	cmd := gitpkg.Command(context.Background(), repoRoot, "worktree", "add", wtDir, "-b", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		cmd2 := gitpkg.Command(context.Background(), repoRoot, "worktree", "add", wtDir, name)
		if out2, err2 := cmd2.CombinedOutput(); err2 != nil {
			return "", fmt.Errorf("git worktree add failed: %s\n%s", string(out), string(out2))
		}
	}

	return wtDir, nil
}
