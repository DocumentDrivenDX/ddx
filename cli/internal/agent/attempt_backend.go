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
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	agentlib "github.com/easel/fizeau"
)

const (
	AttemptBackendWorktree    = "worktree"
	AttemptBackendLocalClone  = "local-clone"
	AttemptBackendDockerClone = "docker-clone"
	AttemptBackendInTree      = "in-tree"

	ExecuteBeadClonePrefix      = ".execute-bead-clone-"
	ExecuteBeadDockerHomePrefix = ".execute-bead-home-"
	ExecuteBeadDockerRunPrefix  = ".execute-bead-runtime-"
	ExecuteBeadInTreeLockPrefix = ".execute-bead-in-tree-lock-"
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
	Backend             string
	ProjectRoot         string
	WorkDir             string
	BeadID              string
	AttemptID           string
	BaseRev             string
	KeepOnError         bool
	DockerHome          string
	DockerRun           string
	DockerSharedGoCache string
	gitOps              GitOps
	inTreeLockFile      *os.File
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
	case AttemptBackendInTree:
		return InTreeAttemptBackend{}, nil
	default:
		return nil, fmt.Errorf("unknown attempt backend %q (valid: %s, %s, %s, %s)",
			name, AttemptBackendWorktree, AttemptBackendLocalClone, AttemptBackendDockerClone, AttemptBackendInTree)
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
	configureAttemptCloneTransientExcludes(clonePath)

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
	ws.DockerRun = executeBeadDockerRunPath(req.ProjectRoot, req.BeadID, req.AttemptID)
	ws.DockerHome = filepath.Join(ws.DockerRun, "home")
	if err := prepareDockerAttemptRuntime(ws.DockerRun); err != nil {
		_ = os.RemoveAll(ws.WorkDir)
		return nil, err
	}
	if err := prepareDockerAttemptHome(ws.DockerHome); err != nil {
		_ = os.RemoveAll(ws.WorkDir)
		_ = os.RemoveAll(ws.DockerRun)
		return nil, err
	}
	if !dockerSharedGoCacheDisabled(b.Docker) {
		shared := dockerSharedGoCachePath(req.ProjectRoot)
		if err := os.MkdirAll(shared, 0o700); err == nil {
			ws.DockerSharedGoCache = shared
		}
	}
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
	image, err := resolveDockerAttemptImage(ctx, cfg, req.ProjectRoot, image)
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolving ddx executable for docker backend: %w", err)
	}
	promptPath, err := containerPath(req.Workspace.WorkDir, req.Runtime.PromptFile)
	if err != nil {
		return nil, err
	}

	containerName := dockerContainerName(req.Workspace)
	args := dockerRunArgs(cfg, req.Workspace, exe, image, dockerToolMounts())
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

	cmd := dockerAttemptCommand(ctx, args...)
	cmd.Dir = req.Workspace.ProjectRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	defer func() {
		_ = dockerRemoveContainer(context.Background(), containerName)
	}()
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
	if ws != nil {
		_ = dockerRemoveContainer(context.Background(), dockerContainerName(ws))
		if ws.DockerHome != "" {
			_ = os.RemoveAll(ws.DockerHome)
		}
		if ws.DockerRun != "" {
			_ = os.RemoveAll(ws.DockerRun)
		}
	}
	return (LocalCloneAttemptBackend{}).Cleanup(ctx, ws)
}

type InTreeAttemptBackend struct{}

func (InTreeAttemptBackend) Name() string { return AttemptBackendInTree }

func (InTreeAttemptBackend) Prepare(ctx context.Context, req AttemptBackendPrepareRequest) (*AttemptWorkspace, error) {
	if err := checkTreeClean(ctx, req.ProjectRoot, req.BeadID); err != nil {
		return nil, fmt.Errorf("in-tree backend requires clean working tree before claim: %w", err)
	}

	lockPath := executeBeadInTreeLockPath(req.ProjectRoot, req.BeadID)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating in-tree lock parent dir: %w", err)
	}

	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening in-tree lock file: %w", err)
	}

	if err := acquireExclusiveLock(lockFile); err != nil {
		lockFile.Close()
		return nil, fmt.Errorf("in-tree attempt already running for this bead (worker lock held by another process): %w", err)
	}

	_, _ = fmt.Fprintf(lockFile, "%d\n", os.Getpid())
	_ = lockFile.Sync()

	return &AttemptWorkspace{
		Backend:        AttemptBackendInTree,
		ProjectRoot:    req.ProjectRoot,
		WorkDir:        req.ProjectRoot,
		BeadID:         req.BeadID,
		AttemptID:      req.AttemptID,
		BaseRev:        req.BaseRev,
		gitOps:         req.GitOps,
		inTreeLockFile: lockFile,
	}, nil
}

func (InTreeAttemptBackend) Run(ctx context.Context, req AttemptBackendRunRequest) (*Result, error) {
	return dispatchAgentRun(ctx, req.Workspace.ProjectRoot, req.Service, req.AgentRunner, req.Config, req.Runtime)
}

func (InTreeAttemptBackend) PublishResult(context.Context, *AttemptWorkspace, *ExecuteBeadResult) error {
	return nil
}

func (InTreeAttemptBackend) Cleanup(_ context.Context, ws *AttemptWorkspace) error {
	if ws == nil {
		return nil
	}
	if ws.inTreeLockFile != nil {
		_ = releaseExclusiveLock(ws.inTreeLockFile)
		_ = ws.inTreeLockFile.Close()
	}
	if ws.BeadID != "" {
		lockPath := executeBeadInTreeLockPath(ws.ProjectRoot, ws.BeadID)
		_ = os.Remove(lockPath)
	}
	return nil
}

func checkTreeClean(ctx context.Context, projectRoot, beadID string) error {
	out, err := internalgit.Command(ctx, projectRoot, "status", "--porcelain").Output()
	if err != nil {
		return fmt.Errorf("checking git status: %w", err)
	}

	var dirtyFiles []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if path == "" {
			continue
		}
		if strings.HasPrefix(path, ".ddx") || strings.HasPrefix(path, ExecuteBeadArtifactDir) {
			continue
		}
		dirtyFiles = append(dirtyFiles, path)
	}

	if len(dirtyFiles) > 0 {
		return fmt.Errorf("working tree is not clean; modified files:\n%s", strings.Join(dirtyFiles, "\n"))
	}
	return nil
}

func executeBeadInTreeLockPath(projectRoot, beadID string) string {
	return filepath.Join(config.ExecutionTempRoot(projectRoot), ExecuteBeadInTreeLockPrefix+beadID)
}

func acquireExclusiveLock(lockFile *os.File) error {
	if lockFile == nil {
		return fmt.Errorf("lock file is nil")
	}
	return inTreeLockAcquire(lockFile)
}

func releaseExclusiveLock(lockFile *os.File) error {
	if lockFile == nil {
		return nil
	}
	return inTreeLockRelease(lockFile)
}

func executeBeadClonePath(projectRoot, beadID, attemptID string) string {
	return filepath.Join(config.ExecutionTempRoot(projectRoot), ExecuteBeadClonePrefix+beadID+"-"+attemptID)
}

func executeBeadDockerHomePath(projectRoot, beadID, attemptID string) string {
	return filepath.Join(config.ExecutionTempRoot(projectRoot), ExecuteBeadDockerHomePrefix+beadID+"-"+attemptID)
}

func executeBeadDockerRunPath(projectRoot, beadID, attemptID string) string {
	return filepath.Join(config.ExecutionTempRoot(projectRoot), ExecuteBeadDockerRunPrefix+beadID+"-"+attemptID)
}

// dockerSharedCacheRoot is a per-project directory used to persist caches
// (notably Go's build cache) across attempts. Living outside the per-attempt
// runtime dir means it survives Cleanup() and is reused on the next attempt,
// turning cold Go builds into warm ones.
func dockerSharedCacheRoot(projectRoot string) string {
	return filepath.Join(config.ExecutionTempRoot(projectRoot), ".ddx-shared-cache-"+shortPathHash(projectRoot))
}

func dockerSharedGoCachePath(projectRoot string) string {
	return filepath.Join(dockerSharedCacheRoot(projectRoot), "gocache")
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
	return []string{ExecuteBeadWtPrefix, ExecuteBeadClonePrefix, ExecuteBeadDockerHomePrefix, ExecuteBeadDockerRunPrefix}
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

func configureAttemptCloneTransientExcludes(clonePath string) {
	out, err := internalgit.Command(context.Background(), clonePath, "rev-parse", "--git-path", "info/exclude").Output()
	if err != nil {
		return
	}
	excludePath := strings.TrimSpace(string(out))
	if excludePath == "" {
		return
	}
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(clonePath, excludePath)
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return
	}
	raw, _ := os.ReadFile(excludePath)
	text := string(raw)
	var additions []string
	for _, pattern := range []string{"/.gocache/", "/.tmp/"} {
		if !excludeTextHasPattern(text, pattern) {
			additions = append(additions, pattern)
		}
	}
	if len(additions) == 0 {
		return
	}
	if len(text) > 0 && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += strings.Join(additions, "\n") + "\n"
	_ = os.WriteFile(excludePath, []byte(text), 0o644)
}

func excludeTextHasPattern(text, pattern string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == pattern {
			return true
		}
	}
	return false
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

func resolveDockerAttemptImage(ctx context.Context, cfg *config.ExecutionsDockerConfig, projectRoot, baseImage string) (string, error) {
	if cfg != nil && strings.TrimSpace(cfg.ProjectImage) != "" {
		return strings.TrimSpace(cfg.ProjectImage), nil
	}
	dockerfile, ok, err := dockerProjectDockerfile(projectRoot, cfg)
	if err != nil || !ok {
		return baseImage, err
	}
	contextDir, err := dockerProjectContext(projectRoot, cfg)
	if err != nil {
		return "", err
	}
	tag := "ddx-project-attempt-" + shortPathHash(projectRoot) + ":latest"
	if skipRebuildIfImagePresent(cfg) && dockerImageExists(ctx, tag) {
		return tag, nil
	}
	args := []string{
		"build", "-q",
		"--build-arg", "DDX_BASE_IMAGE=" + baseImage,
		"-f", dockerfile,
		"-t", tag,
		contextDir,
	}
	if out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("building docker project attempt image: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return tag, nil
}

// skipRebuildIfImagePresent reports whether the project-image fast-path should
// short-circuit `docker build` when the tagged image already exists.
// Default true.
func skipRebuildIfImagePresent(cfg *config.ExecutionsDockerConfig) bool {
	if cfg == nil || cfg.SkipImageRebuildIfPresent == nil {
		return true
	}
	return *cfg.SkipImageRebuildIfPresent
}

func dockerImageExists(ctx context.Context, tag string) bool {
	if strings.TrimSpace(tag) == "" {
		return false
	}
	out, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format", "{{.Id}}", tag).CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

func dockerSharedGoCacheDisabled(cfg *config.ExecutionsDockerConfig) bool {
	if cfg == nil {
		return false
	}
	return cfg.DisableSharedGoCache
}

func dockerProjectDockerfile(projectRoot string, cfg *config.ExecutionsDockerConfig) (string, bool, error) {
	configured := ""
	if cfg != nil {
		configured = strings.TrimSpace(cfg.ProjectDockerfile)
	}
	if configured == "" {
		defaultPath := ddxroot.InTree(projectRoot, "attempt-runner.Dockerfile")
		info, err := os.Stat(defaultPath)
		if err != nil {
			if os.IsNotExist(err) {
				return "", false, nil
			}
			return "", false, err
		}
		if info.IsDir() {
			return "", false, fmt.Errorf("docker project dockerfile is a directory: %s", defaultPath)
		}
		return defaultPath, true, nil
	}
	path, err := projectBoundPath(projectRoot, configured)
	if err != nil {
		return "", false, fmt.Errorf("docker project dockerfile: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", false, fmt.Errorf("docker project dockerfile %s: %w", path, err)
	}
	if info.IsDir() {
		return "", false, fmt.Errorf("docker project dockerfile is a directory: %s", path)
	}
	return path, true, nil
}

func dockerProjectContext(projectRoot string, cfg *config.ExecutionsDockerConfig) (string, error) {
	configured := ""
	if cfg != nil {
		configured = strings.TrimSpace(cfg.ProjectContext)
	}
	if configured == "" {
		return projectRoot, nil
	}
	path, err := projectBoundPath(projectRoot, configured)
	if err != nil {
		return "", fmt.Errorf("docker project context: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("docker project context %s: %w", path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("docker project context is not a directory: %s", path)
	}
	return path, nil
}

func projectBoundPath(projectRoot, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is empty")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(projectRoot, filepath.FromSlash(path))
	}
	cleanRoot := filepath.Clean(projectRoot)
	cleanPath := filepath.Clean(path)
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes project root: %s", path)
	}
	return cleanPath, nil
}

type dockerToolMount struct {
	Name string
	Path string
}

func dockerRunArgs(cfg *config.ExecutionsDockerConfig, ws *AttemptWorkspace, exe, image string, tools []dockerToolMount) []string {
	name := dockerContainerName(ws)
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
		"-e", "PATH=/usr/local/go/bin:/opt/go/bin:/usr/local/bin:/usr/bin:/bin",
		"-e", "HOME=/ddx-runtime/home",
		"-e", "XDG_CACHE_HOME=/ddx-runtime/cache",
		"-e", "TMPDIR=/ddx-runtime/tmp",
		"-e", "GOCACHE=/work/.gocache",
		"-e", "GOTMPDIR=/ddx-runtime/go-tmp",
		"-e", "DDX_PROJECT_ROOT=/work",
		"-e", DDXModeEnvKey + "=" + DDXModeBeadExecution,
		"-e", "DDX_BEAD_ID=" + ws.BeadID,
		"-e", "DDX_ATTEMPT_ID=" + ws.AttemptID,
	}
	if ws.DockerRun != "" {
		args = append(args, "--mount", "type=bind,src="+ws.DockerRun+",dst=/ddx-runtime")
		gocacheSrc := filepath.Join(ws.DockerRun, "work-gocache")
		if ws.DockerSharedGoCache != "" {
			gocacheSrc = ws.DockerSharedGoCache
		}
		args = append(args, "--mount", "type=bind,src="+gocacheSrc+",dst=/work/.gocache")
		args = append(args, "--mount", "type=bind,src="+filepath.Join(ws.DockerRun, "work-tmp")+",dst=/work/.tmp")
	}
	for _, tool := range tools {
		if tool.Name == "" || tool.Path == "" {
			continue
		}
		args = append(args, "--mount", "type=bind,src="+tool.Path+",dst=/usr/local/bin/"+tool.Name+",readonly")
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

func dockerContainerName(ws *AttemptWorkspace) string {
	if ws == nil {
		return "ddx-attempt-unknown"
	}
	return "ddx-attempt-" + sanitizeDockerName(ws.AttemptID)
}

func dockerAttemptCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmdSetProcessGroup(cmd)
	return cmd
}

func dockerRemoveContainer(ctx context.Context, name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	out, err := exec.CommandContext(ctx, "docker", "rm", "-f", name).CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.ToLower(string(out))
	if strings.Contains(msg, "no such container") {
		return nil
	}
	return fmt.Errorf("removing docker attempt container %s: %s: %w", name, strings.TrimSpace(string(out)), err)
}

func dockerToolMounts() []dockerToolMount {
	var mounts []dockerToolMount
	for _, name := range []string{"claude", "codex", "gemini"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			path = resolved
		}
		mounts = append(mounts, dockerToolMount{Name: name, Path: path})
	}
	return mounts
}

func prepareDockerAttemptRuntime(runDir string) error {
	if strings.TrimSpace(runDir) == "" {
		return nil
	}
	if err := os.RemoveAll(runDir); err != nil {
		return fmt.Errorf("resetting docker attempt runtime: %w", err)
	}
	for _, dir := range []string{
		"",
		"cache",
		filepath.Join("go", "pkg", "mod"),
		"go-build-cache",
		"go-tmp",
		"tmp",
		"work-gocache",
		"work-tmp",
	} {
		if err := os.MkdirAll(filepath.Join(runDir, dir), 0o700); err != nil {
			return fmt.Errorf("creating docker attempt runtime: %w", err)
		}
	}
	return nil
}

func prepareDockerAttemptHome(homeDir string) error {
	if strings.TrimSpace(homeDir) == "" {
		return nil
	}
	if err := os.RemoveAll(homeDir); err != nil {
		return fmt.Errorf("resetting docker attempt home: %w", err)
	}
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		return fmt.Errorf("creating docker attempt home: %w", err)
	}
	hostHome, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	for _, file := range []struct {
		src string
		dst string
	}{
		{filepath.Join(hostHome, ".codex", "auth.json"), filepath.Join(homeDir, ".codex", "auth.json")},
		{filepath.Join(hostHome, ".codex", "config.toml"), filepath.Join(homeDir, ".codex", "config.toml")},
		{filepath.Join(hostHome, ".claude", ".credentials.json"), filepath.Join(homeDir, ".claude", ".credentials.json")},
		{filepath.Join(hostHome, ".claude", "settings.json"), filepath.Join(homeDir, ".claude", "settings.json")},
		{filepath.Join(hostHome, ".claude.json"), filepath.Join(homeDir, ".claude.json")},
		// Fizeau provider liveness reads its cached quota snapshots; without
		// them a fresh per-attempt home has no live cloud provider and routing
		// fails with no_viable_provider. Seed the snapshots from the host.
		{filepath.Join(hostHome, ".local", "state", "fizeau", "claude-quota.json"), filepath.Join(homeDir, ".local", "state", "fizeau", "claude-quota.json")},
		{filepath.Join(hostHome, ".local", "state", "fizeau", "codex-quota.json"), filepath.Join(homeDir, ".local", "state", "fizeau", "codex-quota.json")},
		{filepath.Join(hostHome, ".local", "state", "fizeau", "gemini-quota.json"), filepath.Join(homeDir, ".local", "state", "fizeau", "gemini-quota.json")},
	} {
		if err := copyDockerAuthFileIfExists(file.src, file.dst); err != nil {
			return err
		}
	}
	return nil
}

func copyDockerAuthFileIfExists(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat docker auth file %s: %w", src, err)
	}
	if info.IsDir() {
		return nil
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read docker auth file %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("create docker auth dir %s: %w", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, raw, 0o600); err != nil {
		return fmt.Errorf("write docker auth file %s: %w", dst, err)
	}
	return nil
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
