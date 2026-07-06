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
