package agent

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// ddxStateGitScope resolves the git repository and matching pathspecs for
// DDx-managed state. In in-tree mode, callers continue to use the project
// worktree with `.ddx/...` pathspecs. In convention mode, callers switch to
// the XDG state repository and strip the `.ddx/` prefix.
func ddxStateGitScope(projectRoot string, projectPathspecs ...string) (string, []string) {
	inTree := ddxroot.InTree(projectRoot)
	if info, err := os.Stat(inTree); err == nil && info.IsDir() {
		return projectRoot, append([]string(nil), projectPathspecs...)
	}

	stateRoot := ddxroot.JoinProject(projectRoot)

	pathspecs := make([]string, 0, len(projectPathspecs))
	for _, pathspec := range projectPathspecs {
		cleaned := filepath.ToSlash(strings.TrimSpace(pathspec))
		cleaned = strings.TrimPrefix(cleaned, "./")
		cleaned = strings.TrimPrefix(cleaned, ".ddx/")
		pathspecs = append(pathspecs, cleaned)
	}
	return stateRoot, pathspecs
}

// ddxStateCommitArgs injects a stable DDx-local author identity for
// convention-mode commits so the XDG state repository never depends on global
// git config. In-tree commits keep the existing project-local git behavior.
func ddxStateCommitArgs(projectRoot, gitDir string, args ...string) []string {
	if filepath.Clean(gitDir) == filepath.Clean(projectRoot) {
		return append([]string(nil), args...)
	}

	prefix := []string{
		"-c", "user.name=DDx State Root",
		"-c", "user.email=ddx-state-root@localhost",
		"-c", "commit.gpgsign=false",
	}
	return append(prefix, args...)
}
