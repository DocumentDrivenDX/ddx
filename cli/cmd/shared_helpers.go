package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/spf13/cobra"
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

func resolveDDxProjectRoot(workingDir string) string {
	if workingDir == "" {
		return ""
	}
	if workspaceRoot := gitpkg.FindNearestDDxWorkspace(workingDir); workspaceRoot != "" {
		return workspaceRoot
	}
	return workingDir
}

// resolveBeadStoreRoot prefers an existing in-tree .ddx directory when one is
// already present, which keeps command fixtures that seed a bare tracker store
// in-tree from drifting to the XDG convention root.
func resolveBeadStoreRoot(projectRoot string) string {
	if projectRoot == "" {
		return ddxroot.JoinProject(projectRoot)
	}
	inTree := filepath.Join(projectRoot, ddxroot.DirName)
	if info, err := os.Stat(inTree); err == nil && info.IsDir() {
		return inTree
	}
	return ddxroot.JoinProject(projectRoot)
}

func (f *CommandFactory) commandBeadStoreRoot(projectFlag, projectRoot string) string {
	if projectFlag != "" || os.Getenv("DDX_PROJECT_ROOT") != "" {
		return resolveBeadStoreRoot(projectRoot)
	}
	if root := f.beadStoreRoot(); root != "" {
		return root
	}
	return resolveBeadStoreRoot(projectRoot)
}

func commandStatePath(workingDir string, elems ...string) string {
	return ddxroot.JoinProject(resolveDDxProjectRoot(workingDir), elems...)
}

type preClaimGitOps interface {
	CurrentBranch(dir string) (string, error)
	LocalAncestryCheck(dir, targetBranch string) (agent.PreClaimResult, error)
}

// buildCLIPreClaimHook returns a PreClaimHook for inline queue work that
// verifies the local target branch against the last-observed origin
// remote-tracking ref before each bead claim. It performs no network I/O
// (reliability principle P9): the queue's forward progress is never coupled to
// origin reachability. Origin refresh is operator-driven via `ddx sync`.
func buildCLIPreClaimHook(projectRoot string, gitOps preClaimGitOps) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		branch, err := gitOps.CurrentBranch(projectRoot)
		if err != nil {
			return nil // can't determine branch — skip
		}
		res, err := gitOps.LocalAncestryCheck(projectRoot, branch)
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

// isGitWorkTree reports whether dir is inside a git working tree. Used to skip
// default-branch preflight on non-git fixtures (e.g. bare temp dirs in tests).
func isGitWorkTree(dir string) bool {
	if dir == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := gitpkg.Command(ctx, dir, "rev-parse", "--is-inside-work-tree").CombinedOutput()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// enforceDefaultBranchPreflight runs the network-free default-branch diagnostic
// once at ddx work / ddx try drain start, before any store claim.
//
// Hard-fail kinds (mismatched upstream, detached HEAD, missing upstream) exit
// non-zero unless allowNonDefault is true. When allowed, a warning names the
// current branch, its configured upstream, and origin/HEAD. Advisory results
// (local behind / diverged / missing origin/HEAD) print a warning and do not
// block. Non-git directories are skipped.
func enforceDefaultBranchPreflight(projectRoot string, allowNonDefault bool, warn io.Writer) error {
	if !isGitWorkTree(projectRoot) {
		return nil
	}
	res := gitpkg.CheckDefaultBranchPreflight(projectRoot)
	if res.HardFail() {
		if allowNonDefault {
			if warn != nil {
				fmt.Fprintln(warn, formatAllowNonDefaultBranchWarning(res))
			}
			return nil
		}
		return fmt.Errorf("%s", res.Message)
	}
	if res.Warn() && warn != nil {
		fmt.Fprintf(warn, "WARNING: %s\n", res.Message)
	}
	return nil
}

// formatAllowNonDefaultBranchWarning names the current branch, upstream, and
// origin/HEAD so operators can confirm a deliberate non-default drain.
func formatAllowNonDefaultBranchWarning(res gitpkg.DefaultBranchPreflight) string {
	current := res.CurrentBranch
	if current == "" {
		current = "(detached HEAD)"
	}
	upstream := res.UpstreamRef
	if upstream == "" {
		if res.UpstreamBranch != "" {
			upstream = "refs/heads/" + res.UpstreamBranch
		} else {
			upstream = "(none)"
		}
	}
	originHEAD := res.OriginHEADRef
	if originHEAD == "" {
		if res.DefaultBranch != "" {
			originHEAD = "refs/remotes/origin/" + res.DefaultBranch
		} else {
			originHEAD = "(unknown)"
		}
	}
	return fmt.Sprintf(
		"WARNING: --allow-non-default-branch: continuing on branch %q (upstream %s) while origin/HEAD is %s",
		current, upstream, originHEAD,
	)
}

func workTrackerSyncEnabled(cmd *cobra.Command) bool {
	watch, _ := cmd.Flags().GetBool("watch")
	if !watch {
		if cmd.Flags().Changed("tracker-sync") {
			enabled, _ := cmd.Flags().GetBool("tracker-sync")
			return enabled
		}
		return false
	}
	if noSync, _ := cmd.Flags().GetBool("no-tracker-sync"); noSync {
		return false
	}
	if cmd.Flags().Changed("tracker-sync") {
		enabled, _ := cmd.Flags().GetBool("tracker-sync")
		return enabled
	}
	return true
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
	return newLocalServerClientTimeout(30 * time.Second)
}

// newLocalServerClientTimeout is like newLocalServerClient but with a caller-chosen
// timeout. The DDx server presents a self-signed cert (CN=ddx-server), so local
// clients must skip verification; a bare http.Client verifies and fails the
// handshake with "remote error: tls: bad certificate". See ddx bad-cert fix.
func newLocalServerClientTimeout(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // local self-signed cert
		},
	}
}
