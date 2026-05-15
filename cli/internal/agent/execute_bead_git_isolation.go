package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

const (
	executeBeadGitWrapperDirName       = "git-bin"
	executeBeadGitWrapperFileName      = "git"
	executeBeadGitPrivateConfigName    = "git-local-private.cfg"
	executeBeadGitGlobalConfigFileName = "git-global.cfg"
)

func prepareExecuteBeadGitIsolation(projectRoot, wtPath, stateDir string) (map[string]string, error) {
	if err := enableExecuteBeadWorktreeConfig(projectRoot); err != nil {
		return nil, err
	}

	worktreeGitDir, err := executeBeadWorktreeGitDir(wtPath)
	if err != nil {
		return nil, err
	}
	if err := seedExecuteBeadWorktreeConfig(projectRoot, worktreeGitDir); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating execute-bead git isolation dir: %w", err)
	}
	wrapperDir := filepath.Join(stateDir, executeBeadGitWrapperDirName)
	if err := os.MkdirAll(wrapperDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating execute-bead git wrapper dir: %w", err)
	}

	privateCfg := filepath.Join(stateDir, executeBeadGitPrivateConfigName)
	if err := ensureExecuteBeadPrivateConfig(privateCfg); err != nil {
		return nil, err
	}
	globalCfg := filepath.Join(stateDir, executeBeadGitGlobalConfigFileName)
	if err := ensureExecuteBeadPrivateConfig(globalCfg); err != nil {
		return nil, err
	}

	realGit, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("resolving git binary for execute-bead wrapper: %w", err)
	}
	wrapperPath := filepath.Join(wrapperDir, executeBeadGitWrapperFileName)
	if err := os.WriteFile(wrapperPath, []byte(renderExecuteBeadGitWrapper(realGit, privateCfg)), 0o755); err != nil {
		return nil, fmt.Errorf("writing execute-bead git wrapper: %w", err)
	}

	env := map[string]string{
		"PATH":              wrapperDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GIT_CONFIG_GLOBAL": globalCfg,
		"GIT_CONFIG_SYSTEM": "/dev/null",
	}
	return env, nil
}

func sanitizeExecuteBeadWorktreeConfig(wtPath string) {
	for _, key := range []string{"core.bare", "core.worktree"} {
		_ = internalgit.Command(context.Background(), wtPath, "config", "--worktree", "--unset-all", key).Run()
	}
}

func enableExecuteBeadWorktreeConfig(projectRoot string) error {
	if out, err := internalgit.Command(context.Background(), projectRoot, "config", "extensions.worktreeConfig", "true").CombinedOutput(); err != nil {
		return fmt.Errorf("enable extensions.worktreeConfig for execute-bead worktree: %s: %w",
			strings.TrimSpace(string(out)), err)
	}
	return nil
}

func executeBeadWorktreeGitDir(wtPath string) (string, error) {
	gitMetaPath := filepath.Join(wtPath, ".git")
	info, err := os.Stat(gitMetaPath)
	if err != nil {
		return "", fmt.Errorf("stat worktree .git: %w", err)
	}
	if info.IsDir() {
		return gitMetaPath, nil
	}

	raw, err := os.ReadFile(gitMetaPath)
	if err != nil {
		return "", fmt.Errorf("read worktree .git file: %w", err)
	}
	line := strings.TrimSpace(string(raw))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("worktree .git file missing gitdir prefix: %q", line)
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Clean(filepath.Join(wtPath, gitDir))
	}
	return gitDir, nil
}

func seedExecuteBeadWorktreeConfig(projectRoot, worktreeGitDir string) error {
	dst := filepath.Join(worktreeGitDir, "config.worktree")
	if info, err := os.Stat(dst); err == nil && info.Size() > 0 {
		return nil
	}

	commonDirOut, err := internalgit.Command(context.Background(), projectRoot, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return fmt.Errorf("locating git common dir for execute-bead: %w", err)
	}
	commonDir := strings.TrimSpace(string(commonDirOut))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Clean(filepath.Join(projectRoot, commonDir))
	}
	src := filepath.Join(commonDir, "config")
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read shared git config for execute-bead seed: %w", err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("write worktree config seed: %w", err)
	}
	return nil
}

func ensureExecuteBeadPrivateConfig(path string) error {
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create private git config dir: %w", err)
	}
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		return fmt.Errorf("create private git config: %w", err)
	}
	return nil
}

func renderExecuteBeadGitWrapper(realGit, privateCfg string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

real_git=%q
private_cfg=%q

args=("$@")
sub_idx=-1
i=0
while (( i < ${#args[@]} )); do
  case "${args[$i]}" in
    -C|--git-dir|--work-tree|--namespace|--exec-path|--super-prefix|--config-env|-c)
      ((i+=2))
      continue
      ;;
    --)
      ((i++))
      break
      ;;
    -*)
      ((i++))
      continue
      ;;
    *)
      sub_idx=$i
      break
      ;;
  esac
done

if (( sub_idx >= 0 )) && [[ "${args[$sub_idx]}" == "config" ]]; then
  prefix=("${args[@]:0:sub_idx}")
  suffix=("${args[@]:sub_idx+1}")

  repo_scope_idx=-1
  explicit_scope=0
  dangerous_key=0
  for i in "${!suffix[@]}"; do
    case "${suffix[$i]}" in
      core.bare|core.worktree)
        dangerous_key=1
        ;;
      --local|--worktree)
        repo_scope_idx=$i
        explicit_scope=1
        ;;
      --global|--system|--file|--blob)
        explicit_scope=1
        ;;
    esac
  done

  if (( dangerous_key == 1 )); then
    if (( repo_scope_idx >= 0 )); then
      suffix[$repo_scope_idx]=--file
      suffix=("${suffix[@]:0:repo_scope_idx+1}" "$private_cfg" "${suffix[@]:repo_scope_idx+1}")
    elif (( explicit_scope == 0 )); then
      suffix=(--file "$private_cfg" "${suffix[@]}")
    fi
  elif (( repo_scope_idx >= 0 )); then
    suffix[$repo_scope_idx]=--worktree
  elif (( explicit_scope == 0 )); then
    suffix=(--worktree "${suffix[@]}")
  fi

  exec "$real_git" "${prefix[@]}" config "${suffix[@]}"
fi

exec "$real_git" "${args[@]}"
`, realGit, privateCfg)
}
