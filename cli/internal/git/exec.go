package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Command returns a prepared *exec.Cmd for a git invocation that honors
// cmd.Dir / -C <path> rather than whatever GIT_DIR / GIT_WORK_TREE / etc.
// were set in the parent environment.
//
// Why this wrapper exists (ddx-aa8a5fb3):
// Git's repository-discovery rules mean that if GIT_DIR is set in the
// process environment, git uses THAT repo and ignores both -C <path> and
// the process's working directory for repository selection. ddx commands
// invoked from a git hook (lefthook pre-commit, pre-push, etc.) inherit
// the hook's GIT_DIR / GIT_WORK_TREE / GIT_INDEX_FILE. Without stripping
// those, any exec.Command("git", ...) inside ddx mutates the hook's
// repository (the outer one), producing core.bare=true flips and stray
// core.worktree entries in .git/config.
//
// The wrapper strips exactly git's "local env vars" set (the vars git
// itself clears when spawning worktree subprocesses). This is the
// canonical set documented by `git rev-parse --local-env-vars`. It
// preserves everything else, including GIT_SSH_COMMAND, GIT_TRACE*,
// GIT_AUTHOR/COMMITTER_NAME/EMAIL, and custom debugging overrides.
//
// Dir must be an absolute or working-directory-relative path; it is set
// on the returned cmd so that when no GIT_* local vars are in the
// environment, git discovers the repo from Dir.
func Command(ctx context.Context, dir string, args ...string) *exec.Cmd {
	warnOnce.Do(maybeWarnGitDirLeaked)

	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = CleanEnv()
	return cmd
}

// CleanEnv returns os.Environ() with every git local-env-var removed.
// Exported so sibling packages (e.g. internal/agent) that spawn NON-git
// subprocesses inside a worktree can use the same scrub — those child
// processes often invoke git themselves and need the same protection.
func CleanEnv() []string {
	vars := localEnvVars()
	src := os.Environ()
	out := make([]string, 0, len(src))
	for _, kv := range src {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			out = append(out, kv)
			continue
		}
		if vars[kv[:eq]] {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// localEnvVars returns the set of environment variable names that git
// treats as repository-local (i.e. scoped to the current git process's
// repository selection and cleared when git itself spawns subprocesses
// for other worktrees). Result is cached for process lifetime.
//
// We query `git rev-parse --local-env-vars` once at first use so we pick
// up any vars a newer git version adds. If that query fails (git not on
// PATH, or restricted environment), fall back to the documented set.
func localEnvVars() map[string]bool {
	localEnvVarsOnce.Do(func() {
		set := map[string]bool{}
		// Use exec.Command directly here to avoid recursion through
		// Command(). This runs with inherited env on purpose: even if
		// GIT_DIR is set, `git rev-parse --local-env-vars` is a static
		// list of variable NAMES (not their values), so inherited env
		// doesn't affect the output.
		out, err := exec.Command("git", "rev-parse", "--local-env-vars").Output()
		if err == nil {
			for _, name := range strings.Fields(string(out)) {
				if name != "" {
					set[name] = true
				}
			}
		}
		// Fallback / supplement: the canonical set from git's source
		// (environment.c local_repo_env[]). Kept inline so ddx remains
		// functional even if `git rev-parse` is unavailable.
		for _, name := range []string{
			"GIT_DIR",
			"GIT_WORK_TREE",
			"GIT_INDEX_FILE",
			"GIT_COMMON_DIR",
			"GIT_OBJECT_DIRECTORY",
			"GIT_ALTERNATE_OBJECT_DIRECTORIES",
			"GIT_PREFIX",
			"GIT_INTERNAL_GETTEXT_SH_SCHEME",
			"GIT_SHALLOW_FILE",
			"GIT_NO_REPLACE_OBJECTS",
			"GIT_REPLACE_REF_BASE",
			"GIT_CONFIG",
			"GIT_CONFIG_GLOBAL",
			"GIT_CONFIG_SYSTEM",
			"GIT_CONFIG_PARAMETERS",
			"GIT_CONFIG_COUNT",
			"GIT_INDEX_VERSION",
			"GIT_LITERAL_PATHSPECS",
			"GIT_GLOB_PATHSPECS",
			"GIT_NOGLOB_PATHSPECS",
			"GIT_ICASE_PATHSPECS",
			"GIT_QUARANTINE_PATH",
			"GIT_INTERNAL_NEW_OBJECT_HOOK",
			"GIT_IMPLICIT_WORK_TREE",
		} {
			set[name] = true
		}
		localEnvVarsCache = set
	})
	return localEnvVarsCache
}

var (
	localEnvVarsOnce  sync.Once
	localEnvVarsCache map[string]bool

	warnOnce sync.Once
)

// maybeWarnGitDirLeaked emits a one-time stderr diagnostic when the
// parent environment has GIT_DIR set. This is the canonical smoke signal
// for hook-inherited env contamination; surfacing it during normal runs
// helps catch upstream leaks we haven't yet wrapped.
func maybeWarnGitDirLeaked() {
	if v := os.Getenv("GIT_DIR"); v != "" {
		fmt.Fprintf(os.Stderr,
			"ddx git wrapper: stripped inherited GIT_DIR=%q (likely hook context). "+
				"This is diagnostic; operation proceeds against cmd.Dir instead.\n", v)
	}
}
