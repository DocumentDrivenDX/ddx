# Git Fixture Env Audit

**Bead**: ddx-08e90869  
**Run**: 20260508T034146-7869f507  
**Date**: 2026-05-08  
**Scope**: Raw Git fixture invocations across `cli/cmd`, `cli/internal/agent`, `cli/internal/server`, and fixture scripts.

## Background

During the ddx-b8bbd0a6 incident, `scripts/build-fixture-repo.sh` was called in a lefthook hook context where `GIT_DIR`, `GIT_WORK_TREE`, and `GIT_INDEX_FILE` were set to the outer (primary checkout) repository. The `git config user.email "fixture@ddx.test"` and `git config user.name "DDx Fixture"` calls in `create_project` wrote to the outer repo's config because `GIT_DIR` redirected all git operations. This produced `core.bare=true` and leaked fixture identity into the primary checkout.

The isolation primitive is `gitpkg.Command()` / `gitpkg.CleanEnv()` (defined in `cli/internal/git/exec.go`), which strips the git-local env var set before spawning any git subprocess.

---

## Package-level Guards Already in Place

| Package | Guard | Notes |
|---|---|---|
| `cli/cmd` | `TestMain` in `cmd/testutils_test.go` scrubs all `GIT_*` vars | Also uses `gitpkg.CleanEnv()` in `initGit()` helper |
| `cli/internal/git` | `TestMain` in `git/git_basic_test.go` scrubs all `GIT_*` vars | `scrubbedGitEnv()` / `runGitInDir()` helpers use scrubbed env |
| `cli/internal/server` | `TestMain` in `server/server_test.go` scrubs all `GIT_*` vars | Added before this bead |
| `cli/internal/agent` | No `TestMain` | See per-file dispositions below |
| `lefthook.yml:full-tests` | Pre-push hook scrubs `$(git rev-parse --local-env-vars)` before `go test` | Boundary guard; does not cover non-lefthook invocations |

---

## Hits: `cli/cmd/`

### `cli/cmd/testutils_test.go:255-280` — `initGit()` method on `TestEnvironment`
- **Operations**: `git init`, `git config user.email`, `git config user.name`
- **Disposition**: **already-safe** — explicitly sets `cmd.Env = gitpkg.CleanEnv()` on each invocation; `TestMain` also scrubs `GIT_*` for the package.

### `cli/cmd/acceptance_test.go:99,103,107` and `:469,473,477` — inline git fixture setup
- **Operations**: `exec.Command("git", "init")`, `git config user.email`, `git config user.name`
- **Disposition**: **already-safe** — package `TestMain` scrubs `GIT_*` before tests run; these inline calls inherit a clean env.

### `cli/cmd/agent_execute_loop_test.go:43-60` — inline `git init` and `git commit`
- **Operations**: `git init <workDir>`, `git -C <workDir> -c user.name=... -c user.email=... commit`
- **Disposition**: **already-safe** — uses `-c` inline config flags (no config file writes); package `TestMain` scrubs env. The `-c` flag approach bypasses `git config` file writes entirely, so no corruption is possible even if `GIT_DIR` were set.

### `cli/cmd/agent_run_passthrough_test.go:128-132` — inline fixture setup
- **Operations**: `git init <dir>`, `git -C <dir> config user.email`, `git -C <dir> config user.name`, `git -C <dir> commit`
- **Disposition**: **already-safe** — package `TestMain` scrubs env.

### `cli/cmd/bead_acceptance_test.go:918` — `git commit` inline
- **Operations**: `exec.Command("git", "commit", "-m", message)` with `.Dir` set
- **Disposition**: **already-safe** — write to an already-init'd temp dir; package `TestMain` scrubs env.

### `cli/cmd/bead_workspace_test.go:32,62,112` — `git worktree remove`
- **Operations**: `exec.Command("git", "-C", projectRoot, "worktree", "remove", "--force", worktreeRoot)`
- **Disposition**: **already-safe** — destructive cleanup, no config writes; package `TestMain` scrubs env.

### `cli/cmd/contract_test.go:42,46,50` and similar groups — fixture setup
- **Operations**: `git init`, `git config user.email`, `git config user.name`
- **Disposition**: **already-safe** — package `TestMain` scrubs env.

### `cli/cmd/doctor_repo_health_test.go:51-271` — multiple git config reads/writes
- **Operations**: `git config core.bare true`, `git config core.worktree`, `git worktree add`, various reads
- **Disposition**: **already-safe** — these are intentional writes to temp-dir repos for health-check testing; package `TestMain` scrubs env.

### `cli/cmd/e2e_test.go:61,66,70` — fixture setup
- **Operations**: `git init`, `git config user.email`, `git config user.name`
- **Disposition**: **already-safe** — package `TestMain` scrubs env.

### `cli/cmd/init_test.go:470,507,536,633-640,677,704,723,759,778` — fixture setup
- **Operations**: `git init`, `git config`, `git commit`
- **Disposition**: **already-safe** — package `TestMain` scrubs env.

### `cli/cmd/work_resource_exhausted_test.go:94-98` — inline fixture setup
- **Operations**: `git init <projectRoot>`, `git -C <projectRoot> -c user.name=... -c user.email=... commit`
- **Disposition**: **already-safe** — uses `-c` inline flags (no config file writes); package `TestMain` scrubs env.

---

## Hits: `cli/internal/agent/`

**Note**: The `cli/internal/agent` package has no `TestMain`. However, all fixtures reviewed use the `-c user.name=... -c user.email=...` inline config flag pattern rather than separate `git config` calls. The `-c` flag provides identity inline and does not write to the git config file, so it cannot corrupt `GIT_DIR`-redirected config files even if the env is contaminated.

### `cli/internal/agent/evidence_telemetry_test.go:27-55` and `:215,229` — fixture setup
- **Operations**: `git init <projectRoot>`, `git -C <projectRoot> -c user.name=... -c user.email=... commit`
- **Disposition**: **already-safe** — uses `-c` inline config; `git init <dir>` with an explicit directory argument creates the repo at that path regardless of `GIT_DIR` (git treats the positional argument as the init path, not `GIT_DIR`).

### `cli/internal/agent/execute_bead_land_test.go:56,123,161` — fixture setup
- **Operations**: `git init --bare <bareDir>`, `git -C <wt> -c user.name=... commit`
- **Disposition**: **already-safe** — bare init with explicit path; `-c` flag for identity.

### `cli/internal/agent/execute_bead_review_test.go:560,575` — fixture setup
- **Operations**: `git init <projectRoot>`, `git -C ... -c user.name=... commit`
- **Disposition**: **already-safe** — explicit path for init; `-c` flag for identity.

### `cli/internal/agent/execute_bead_review_evidence_test.go:235,250` and `execute_bead_review_pairing_test.go:67` — fixture setup
- **Operations**: `git init <projectRoot>`, `git -C ... -c user.name=... commit`
- **Disposition**: **already-safe** — explicit path for init; `-c` flag for identity.

### `cli/internal/agent/review_verdict_test.go:238,253` — fixture setup
- **Operations**: `git init <projectRoot>`, `git -C ... -c user.name=... commit`
- **Disposition**: **already-safe** — explicit path for init; `-c` flag for identity.

---

## Hits: `cli/internal/server/`

### `cli/internal/server/workers_test.go:292-298` — `runCmd()` helper
- **Operations**: Any git command passed by caller (init, config, commit, worktree add/remove)
- **Prior disposition**: raw `exec.Command` without CleanEnv — **risk** for tests that call `t.Setenv("GIT_DIR", ...)` in the same package
- **Action taken**: **converted** — added `if name == "git" { cmd.Env = gitpkg.CleanEnv() }` in `runCmd`. Package `TestMain` scrubs env for normal runs; explicit CleanEnv in `runCmd` provides defense in depth when individual tests re-contaminate env via `t.Setenv`.

### `cli/internal/server/workers_test.go:284-290` — `initGitRepo()` helper
- **Operations**: `git init`, `git add -A`, `git -c user.name=... commit`
- **Disposition**: **converted** transitively — delegates to `runCmd`, which now sets CleanEnv for git.

### `cli/internal/server/workers_test.go:776` — inline `exec.Command("git", "-C", root, "rev-parse", ...)`
- **Operations**: READ — `git rev-parse refs/heads/main`
- **Disposition**: **already-safe** — read-only; cannot corrupt config. Package `TestMain` scrubs env. A wrong result here would cause an immediate test failure, not silent corruption.

### `cli/internal/server/workers_gates_test.go:48-52` — `runCmd` fixture setup
- **Disposition**: **converted** transitively via `runCmd` fix.

### `cli/internal/server/workers_gates_test.go:54` — inline `exec.Command("git", "-C", root, "rev-parse", ...)`
- **Operations**: READ
- **Disposition**: **already-safe** — read-only.

### `cli/internal/server/workers_intake_test.go:151-155` — `runCmd` fixture setup
- **Disposition**: **converted** transitively via `runCmd` fix.

### `cli/internal/server/workers_resolve_route_test.go:135-139` — `runCmd` fixture setup
- **Disposition**: **converted** transitively via `runCmd` fix.

### `cli/internal/server/land_coordinator_test.go:264-323` — inline env scrubbing + git calls
- **Operations**: Builds `cleanEnv` by filtering `GIT_*` from `os.Environ()`, then passes to git subprocesses
- **Disposition**: **already-safe** — has its own inline CleanEnv-equivalent construction.

### `cli/internal/server/graphql/integration_test.go:56` — `exec.Command("git", "init", workDir)`
- **Operations**: `git init <workDir>` with explicit path
- **Disposition**: **already-safe** — `git init <dir>` with explicit path argument; package TestMain scrubs env.

---

## Hits: `scripts/`

### `scripts/build-fixture-repo.sh:69-100` — `create_project()` function
- **Operations**: `git init -q -b main`, `git config user.email "fixture@ddx.test"`, `git config user.name "DDx Fixture"`, `git add`, `git commit`
- **Prior disposition**: no env scrubbing — **risk** when script is invoked from any context where `GIT_DIR` is set (hook invocation, test from within a lefthook pre-push, etc.). This was the root-cause callsite of the ddx-b8bbd0a6 incident.
- **Action taken**: **converted** — added explicit `unset` loop for all git-local env vars at the start of the `create_project` subshell, before any git commands. The var list matches `gitpkg.localEnvVars()` (the canonical set from `git rev-parse --local-env-vars` + fallback hardcoded set in `cli/internal/git/exec.go`).

---

## Summary

| Disposition | Count |
|---|---|
| already-safe | 25+ callsites |
| converted | 4 (`runCmd` + 3 transitively via runCmd; `build-fixture-repo.sh create_project`) |
| follow-up | 0 |

All reusable test helpers identified under `cli/cmd`, `cli/internal/agent`, `cli/internal/server`, and fixture scripts have been audited. Inline one-off git calls are protected by package-level `TestMain` guards and/or use the `-c` flag approach that avoids config file writes. The `runCmd` helper and `build-fixture-repo.sh create_project` were the only callsites requiring active conversion.

---

## Regression Coverage Added

- `cli/internal/git/exec_test.go:TestGitFixtureHelpersStripHookEnv` — contaminates `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`; runs `git init` + `git config user.email/user.name` via `gitpkg.Command()`; asserts fixture dir received the writes and outer repo config is unchanged.
