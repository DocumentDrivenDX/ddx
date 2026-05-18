package agent

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	agentlib "github.com/easel/fizeau"
)

const (
	AttemptBackendWorktree    = "worktree"
	AttemptBackendLocalClone  = "local-clone"
	AttemptBackendDockerClone = "docker-clone"

	ExecuteBeadClonePrefix = ".execute-bead-clone-"
)

// AttemptBackend owns the workspace and transport mechanics for one
// execute-bead attempt. The caller keeps prompt construction, result
// classification, evidence, gate evaluation, and landing in one shared path.
type AttemptBackend interface {
	Name() string
	Prepare(ctx context.Context, req AttemptBackendPrepareRequest) (*AttemptWorkspace, error)
	Run(ctx context.Context, req AttemptBackendRunRequest) (*Result, error)
	PublishResult(ctx context.Context, ws *AttemptWorkspace, res *ExecuteBeadResult) error
	Cleanup(ctx context.Context, ws *AttemptWorkspace) error
}

type AttemptBackendPrepareRequest struct {
	ProjectRoot string
	BeadID      string
	AttemptID   string
	BaseRev     string
	GitOps      GitOps
}

type AttemptBackendRunRequest struct {
	ProjectRoot string
	Workspace   *AttemptWorkspace
	Service     agentlib.FizeauService
	AgentRunner AgentRunner
	Config      config.ResolvedConfig
	Runtime     AgentRunRuntime
}

type AttemptWorkspace struct {
	Backend     string
	ProjectRoot string
	WorkDir     string
	BeadID      string
	AttemptID   string
	BaseRev     string
	KeepOnError bool
	gitOps      GitOps
}

func ResolveAttemptBackend(rcfg config.ResolvedConfig) (AttemptBackend, error) {
	name := strings.ToLower(strings.TrimSpace(rcfg.AttemptBackend()))
	if name == "" {
		name = AttemptBackendWorktree
	}
	switch name {
	case AttemptBackendWorktree, "linked-worktree":
		return WorktreeAttemptBackend{}, nil
	case AttemptBackendLocalClone, "clone":
		return LocalCloneAttemptBackend{CloneMode: dockerCloneMode(rcfg.ExecutionsDockerConfig())}, nil
	case AttemptBackendDockerClone, "docker":
		return DockerCloneAttemptBackend{Docker: rcfg.ExecutionsDockerConfig()}, nil
	default:
		return nil, fmt.Errorf("unknown attempt backend %q (valid: %s, %s, %s)",
			name, AttemptBackendWorktree, AttemptBackendLocalClone, AttemptBackendDockerClone)
	}
}

type WorktreeAttemptBackend struct{}

func (WorktreeAttemptBackend) Name() string { return AttemptBackendWorktree }

func (WorktreeAttemptBackend) Prepare(ctx context.Context, req AttemptBackendPrepareRequest) (*AttemptWorkspace, error) {
	gitOps := req.GitOps
	if gitOps == nil {
		gitOps = &RealGitOps{}
	}
	wtPath := executeBeadWorktreePath(req.ProjectRoot, req.BeadID, req.AttemptID)
	if err := os.MkdirAll(filepath.Dir(wtPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating execute-bead worktree parent dir: %w", err)
	}
	if err := gitOps.WorktreeAdd(req.ProjectRoot, wtPath, req.BaseRev); err != nil {
		_ = os.RemoveAll(wtPath)
		return nil, fmt.Errorf("creating isolated worktree: %w", err)
	}
	return &AttemptWorkspace{
		Backend:     AttemptBackendWorktree,
		ProjectRoot: req.ProjectRoot,
		WorkDir:     wtPath,
		BeadID:      req.BeadID,
		AttemptID:   req.AttemptID,
		BaseRev:     req.BaseRev,
		gitOps:      gitOps,
	}, nil
}

func (WorktreeAttemptBackend) Run(ctx context.Context, req AttemptBackendRunRequest) (*Result, error) {
	return dispatchAgentRun(ctx, req.ProjectRoot, req.Service, req.AgentRunner, req.Config, req.Runtime)
}

func (WorktreeAttemptBackend) PublishResult(context.Context, *AttemptWorkspace, *ExecuteBeadResult) error {
	return nil
}

func (WorktreeAttemptBackend) Cleanup(_ context.Context, ws *AttemptWorkspace) error {
	if ws == nil || ws.WorkDir == "" {
		return nil
	}
	gitOps := ws.gitOps
	if gitOps == nil {
		gitOps = &RealGitOps{}
	}
	return gitOps.WorktreeRemove(ws.ProjectRoot, ws.WorkDir)
}

type LocalCloneAttemptBackend struct {
	CloneMode string
}

func (LocalCloneAttemptBackend) Name() string { return AttemptBackendLocalClone }

func (b LocalCloneAttemptBackend) Prepare(ctx context.Context, req AttemptBackendPrepareRequest) (*AttemptWorkspace, error) {
	clonePath := executeBeadClonePath(req.ProjectRoot, req.BeadID, req.AttemptID)
	if err := os.MkdirAll(filepath.Dir(clonePath), 0o755); err != nil {
		return nil, fmt.Errorf("creating execute-bead clone parent dir: %w", err)
	}

	cloneArgs := localCloneArgs(b.CloneMode, req.ProjectRoot, clonePath)
	if out, err := internalgit.Command(ctx, req.ProjectRoot, cloneArgs...).CombinedOutput(); err != nil {
		if shouldRetryCloneWithoutHardlinks(b.CloneMode, out) {
			_ = os.RemoveAll(clonePath)
			cloneArgs = localCloneArgs("no-hardlinks", req.ProjectRoot, clonePath)
			out, err = internalgit.Command(ctx, req.ProjectRoot, cloneArgs...).CombinedOutput()
		}
		if err == nil {
			goto checkout
		}
		_ = os.RemoveAll(clonePath)
		return nil, fmt.Errorf("creating isolated clone: %s: %w", strings.TrimSpace(string(out)), err)
	}
checkout:
	if out, err := internalgit.Command(ctx, clonePath, "checkout", "--detach", req.BaseRev).CombinedOutput(); err != nil {
		_ = os.RemoveAll(clonePath)
		return nil, fmt.Errorf("checking out isolated clone base: %s: %w", strings.TrimSpace(string(out)), err)
	}
	seedAttemptCloneUserConfig(ctx, req.ProjectRoot, clonePath)

	return &AttemptWorkspace{
		Backend:     AttemptBackendLocalClone,
		ProjectRoot: req.ProjectRoot,
		WorkDir:     clonePath,
		BeadID:      req.BeadID,
		AttemptID:   req.AttemptID,
		BaseRev:     req.BaseRev,
	}, nil
}

func (LocalCloneAttemptBackend) Run(ctx context.Context, req AttemptBackendRunRequest) (*Result, error) {
	return dispatchAgentRun(ctx, req.ProjectRoot, req.Service, req.AgentRunner, req.Config, req.Runtime)
}

func (LocalCloneAttemptBackend) PublishResult(ctx context.Context, ws *AttemptWorkspace, res *ExecuteBeadResult) error {
	return publishCloneResult(ctx, ws, res)
}

func (LocalCloneAttemptBackend) Cleanup(_ context.Context, ws *AttemptWorkspace) error {
	if ws == nil || ws.WorkDir == "" {
		return nil
	}
	if ws.KeepOnError {
		return nil
	}
	return os.RemoveAll(ws.WorkDir)
}

type DockerCloneAttemptBackend struct {
	Docker *config.ExecutionsDockerConfig
}

func (DockerCloneAttemptBackend) Name() string { return AttemptBackendDockerClone }

func (b DockerCloneAttemptBackend) Prepare(ctx context.Context, req AttemptBackendPrepareRequest) (*AttemptWorkspace, error) {
	ws, err := (LocalCloneAttemptBackend{CloneMode: dockerCloneMode(b.Docker)}).Prepare(ctx, req)
	if err != nil {
		return nil, err
	}
	ws.Backend = AttemptBackendDockerClone
	ws.KeepOnError = b.Docker != nil && b.Docker.KeepOnError
	return ws, nil
}

func (b DockerCloneAttemptBackend) Run(ctx context.Context, req AttemptBackendRunRequest) (*Result, error) {
	cfg := b.Docker
	image := ""
	if cfg != nil {
		image = strings.TrimSpace(cfg.Image)
	}
	if image == "" {
		return nil, fmt.Errorf("docker-clone attempt backend requires executions.docker.image")
	}
	if req.Workspace == nil || req.Workspace.WorkDir == "" {
		return nil, fmt.Errorf("docker-clone attempt backend missing workspace")
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolving ddx executable for docker backend: %w", err)
	}
	promptPath, err := containerPath(req.Workspace.WorkDir, req.Runtime.PromptFile)
	if err != nil {
		return nil, err
	}

	args := dockerRunArgs(cfg, req.Workspace, exe, image)
	runArgs := []string{
		"ddx", "run",
		"--project", "/work",
		"--prompt", promptPath,
		"--output", "json-result",
		"--permissions", firstNonEmpty(req.Runtime.PermissionsOverride, req.Config.Permissions()),
	}
	if h := strings.TrimSpace(req.Config.Harness()); h != "" {
		runArgs = append(runArgs, "--harness", h)
	}
	if p := strings.TrimSpace(req.Config.Provider()); p != "" {
		runArgs = append(runArgs, "--provider", p)
	}
	if m := strings.TrimSpace(req.Config.Model()); m != "" {
		runArgs = append(runArgs, "--model", m)
	}
	if p := strings.TrimSpace(req.Config.Profile()); p != "" {
		runArgs = append(runArgs, "--profile", p)
	}
	if e := strings.TrimSpace(req.Config.Effort()); e != "" {
		runArgs = append(runArgs, "--effort", e)
	}
	if min := req.Config.MinPower(); min > 0 {
		runArgs = append(runArgs, "--min-power", strconv.Itoa(min))
	}
	if max := req.Config.MaxPower(); max > 0 {
		runArgs = append(runArgs, "--max-power", strconv.Itoa(max))
	}
	if timeout := req.Config.Timeout(); timeout > 0 {
		runArgs = append(runArgs, "--timeout", timeout.String())
	}
	args = append(args, runArgs...)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = req.Workspace.ProjectRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	result, parseErr := parseDockerRunResult(stdout.Bytes())
	if parseErr == nil {
		if strings.TrimSpace(stderr.String()) != "" && result.Stderr == "" {
			result.Stderr = strings.TrimSpace(stderr.String())
		}
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("docker-clone attempt failed: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return nil, parseErr
}

func (DockerCloneAttemptBackend) PublishResult(ctx context.Context, ws *AttemptWorkspace, res *ExecuteBeadResult) error {
	return publishCloneResult(ctx, ws, res)
}

func (DockerCloneAttemptBackend) Cleanup(ctx context.Context, ws *AttemptWorkspace) error {
	return (LocalCloneAttemptBackend{}).Cleanup(ctx, ws)
}

func executeBeadClonePath(projectRoot, beadID, attemptID string) string {
	return filepath.Join(config.ExecutionTempRoot(projectRoot), ExecuteBeadClonePrefix+beadID+"-"+attemptID)
}

func localCloneArgs(mode, projectRoot, clonePath string) []string {
	cloneArgs := []string{"clone", "--no-checkout"}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "copy", "no-hardlinks", "no-hardlink":
		cloneArgs = append(cloneArgs, "--no-hardlinks")
	default:
		cloneArgs = append(cloneArgs, "--local")
	}
	return append(cloneArgs, projectRoot, clonePath)
}

func shouldRetryCloneWithoutHardlinks(mode string, out []byte) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "copy", "no-hardlinks", "no-hardlink":
		return false
	}
	msg := strings.ToLower(string(out))
	return strings.Contains(msg, "invalid cross-device link") ||
		strings.Contains(msg, "operation not permitted")
}

func executionAttemptDirPrefixes() []string {
	return []string{ExecuteBeadWtPrefix, ExecuteBeadClonePrefix}
}

func seedAttemptCloneUserConfig(ctx context.Context, projectRoot, clonePath string) {
	userName := strings.TrimSpace(gitConfigValue(ctx, projectRoot, "user.name"))
	userEmail := strings.TrimSpace(gitConfigValue(ctx, projectRoot, "user.email"))
	if userName == "" {
		userName = "ddx"
	}
	if userEmail == "" {
		userEmail = "ddx@local"
	}
	_ = internalgit.Command(ctx, clonePath, "config", "user.name", userName).Run()
	_ = internalgit.Command(ctx, clonePath, "config", "user.email", userEmail).Run()
}

func gitConfigValue(ctx context.Context, dir, key string) string {
	out, err := internalgit.Command(ctx, dir, "config", "--get", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func publishCloneResult(ctx context.Context, ws *AttemptWorkspace, res *ExecuteBeadResult) error {
	if ws == nil || res == nil || ws.WorkDir == "" || ws.ProjectRoot == "" {
		return nil
	}
	resultRev := strings.TrimSpace(res.ResultRev)
	if resultRev == "" || resultRev == res.BaseRev {
		return nil
	}
	srcRef := attemptBackendResultRef("source", ws.BeadID, ws.AttemptID)
	dstRef := attemptBackendResultRef("result", ws.BeadID, ws.AttemptID)
	if out, err := internalgit.Command(ctx, ws.WorkDir, "update-ref", srcRef, resultRev).CombinedOutput(); err != nil {
		return fmt.Errorf("pinning clone result ref: %s: %w", strings.TrimSpace(string(out)), err)
	}
	refspec := "+" + srcRef + ":" + dstRef
	if out, err := internalgit.Command(ctx, ws.ProjectRoot, "fetch", "--no-tags", ws.WorkDir, refspec).CombinedOutput(); err != nil {
		return fmt.Errorf("importing clone result: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func attemptBackendResultRef(kind, beadID, attemptID string) string {
	return "refs/ddx/attempt-backend/" + sanitizeDockerName(kind+"-"+beadID+"-"+attemptID)
}

func dockerCloneMode(cfg *config.ExecutionsDockerConfig) string {
	if cfg == nil {
		return ""
	}
	return cfg.CloneMode
}

func dockerRunArgs(cfg *config.ExecutionsDockerConfig, ws *AttemptWorkspace, exe, image string) []string {
	name := "ddx-attempt-" + sanitizeDockerName(ws.AttemptID)
	args := []string{
		"run", "--rm", "--init",
		"--name", name,
		"--label", "ddx.backend=docker-clone",
		"--label", "ddx.attempt=" + ws.AttemptID,
		"--label", "ddx.project=" + shortPathHash(ws.ProjectRoot),
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"--workdir", "/work",
		"--mount", "type=bind,src=" + ws.WorkDir + ",dst=/work",
		"--mount", "type=bind,src=" + exe + ",dst=/usr/local/bin/ddx,readonly",
		"-e", "HOME=/tmp/ddx-home",
		"-e", "XDG_CACHE_HOME=/tmp/ddx-cache",
		"-e", "DDX_PROJECT_ROOT=/work",
		"-e", DDXModeEnvKey + "=" + DDXModeBeadExecution,
		"-e", "DDX_BEAD_ID=" + ws.BeadID,
		"-e", "DDX_ATTEMPT_ID=" + ws.AttemptID,
	}
	for _, env := range dockerCredentialEnv() {
		args = append(args, "-e", env)
	}
	tmpfsSize := "2g"
	if cfg != nil && strings.TrimSpace(cfg.TmpfsSize) != "" {
		tmpfsSize = strings.TrimSpace(cfg.TmpfsSize)
	}
	args = append(args, "--tmpfs", "/tmp:rw,nosuid,nodev,size="+tmpfsSize+",mode=1777")
	if cfg != nil {
		if cfg.Memory != "" {
			args = append(args, "--memory", cfg.Memory)
		}
		if cfg.MemorySwap != "" {
			args = append(args, "--memory-swap", cfg.MemorySwap)
		}
		if cfg.CPUs != "" {
			args = append(args, "--cpus", cfg.CPUs)
		}
		if cfg.PidsLimit > 0 {
			args = append(args, "--pids-limit", strconv.Itoa(cfg.PidsLimit))
		}
		if cfg.Network != "" {
			args = append(args, "--network", cfg.Network)
		}
	}
	return append(args, image)
}

func dockerCredentialEnv() []string {
	prefixes := []string{
		"ANTHROPIC_", "CODEX_", "FIZEAU_", "GEMINI_", "GOOGLE_",
		"OPENAI_", "OPENROUTER_",
	}
	var env []string
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		for _, prefix := range prefixes {
			if strings.HasPrefix(name, prefix) {
				env = append(env, name)
				break
			}
		}
	}
	return env
}

func containerPath(root, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("docker-clone attempt requires a prompt file inside the workspace")
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("docker-clone prompt file must be inside the attempt workspace: %s", path)
	}
	return filepath.ToSlash(filepath.Join("/work", rel)), nil
}

func parseDockerRunResult(raw []byte) (*Result, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("docker-clone produced no JSON result")
	}
	start := bytes.IndexByte(trimmed, '{')
	if start < 0 {
		return nil, fmt.Errorf("docker-clone output did not contain a JSON result")
	}
	var result Result
	if err := json.Unmarshal(trimmed[start:], &result); err != nil {
		return nil, fmt.Errorf("decoding docker-clone JSON result: %w", err)
	}
	return &result, nil
}

func sanitizeDockerName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-_.")
	if out == "" {
		return "attempt"
	}
	if len(out) > 80 {
		return out[:80]
	}
	return out
}

func shortPathHash(path string) string {
	sum := sha1.Sum([]byte(filepath.Clean(path)))
	return hex.EncodeToString(sum[:])[:12]
}
