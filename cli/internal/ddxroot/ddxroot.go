package ddxroot

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
)

const DirName = ".ddx"

type WorktreeRegistry = worktreeRegistry
type WorktreeRegistryEntry = worktreeRegistryEntry

func InTree(projectRoot string, elems ...string) string {
	parts := append([]string{projectRoot, DirName}, elems...)
	return filepath.Join(parts...)
}

func JoinHome(home string, elems ...string) string {
	parts := append([]string{home, DirName}, elems...)
	return filepath.Join(parts...)
}

func JoinRelative(elems ...string) string {
	parts := append([]string{DirName}, elems...)
	return filepath.Join(parts...)
}

// ExistingPath returns the DDx state root for projectRoot only when it already
// exists. Unlike Path, it never bootstraps a convention root.
func ExistingPath(ctx context.Context, projectRoot string) (string, bool) {
	inTree := InTree(projectRoot)
	if info, err := os.Stat(inTree); err == nil && info.IsDir() {
		return inTree, true
	}
	root := cachedConventionRoot(ctx, projectRoot)
	if info, err := os.Stat(root); err == nil && info.IsDir() {
		return root, true
	}
	return "", false
}

func ExistingJoinProject(ctx context.Context, projectRoot string, elems ...string) (string, bool) {
	root, ok := ExistingPath(ctx, projectRoot)
	if !ok {
		return "", false
	}
	parts := append([]string{root}, elems...)
	return filepath.Join(parts...), true
}

func JoinProject(projectRoot string, elems ...string) string {
	return JoinProjectContext(context.Background(), projectRoot, elems...)
}

func JoinProjectContext(ctx context.Context, projectRoot string, elems ...string) string {
	parts := append([]string{Path(ctx, projectRoot)}, elems...)
	return filepath.Join(parts...)
}

// Path returns the DDx state root for projectRoot.
//
// When the project already has an in-tree `.ddx/` directory, that remains
// authoritative. Otherwise DDx falls back to an XDG-scoped convention root
// derived from the project's canonical git remote, or a deterministic local
// identity.
func Path(ctx context.Context, projectRoot string) string {
	inTree := InTree(projectRoot)
	if info, err := os.Stat(inTree); err == nil && info.IsDir() {
		return inTree
	}
	return bootstrappedConventionRoot(ctx, projectRoot)
}

// pathCacheKey identifies one convention root within one process. It is
// scoped by projectsRoot() (derived from XDG_DATA_HOME) so tests that repoint
// XDG_DATA_HOME between calls never share a stale entry.
type pathCacheKey struct {
	projectsRoot string
	worktree     string
}

// pathCacheEntry memoizes convention-root resolution and bootstrap state for
// one key. identity/root are filled once (identity resolution has no
// invalidation trigger within a process); bootstrapped is only set true after
// bootstrapConventionRootFn returns nil, and is cleared again if the root is
// later found missing, so a failed or since-removed bootstrap is retried
// rather than permanently cached as successful.
type pathCacheEntry struct {
	mu           sync.Mutex
	identity     string
	root         string
	bootstrapped bool
}

// pathCache memoizes ddxroot.Path's convention-root bootstrap per process.
// Without this, every call in the convention-root branch (no in-tree .ddx/)
// re-runs bootstrapConventionRoot, which shells out to git 2-5 times
// (rev-parse --git-dir, remote get-url origin, rev-parse --verify HEAD, and
// on first use init/add/commit) plus a lock acquire/release and a
// worktrees.json rewrite. That costs on the order of 100-200ms per call on a
// slow-exec host, and this path is reachable from every bead.NewStore call
// and every worker liveness heartbeat tick, so an unmemoized Path() makes any
// tight-timing loop (or test) unreliable. See ddx-related investigation of
// TestExecuteBeadLoopReleasesServerUnavailableWhenProbeHealthy.
var pathCache sync.Map // map[pathCacheKey]*pathCacheEntry

// bootstrapConventionRootFn is a test seam over bootstrapConventionRoot.
var bootstrapConventionRootFn = bootstrapConventionRoot

// bootstrappedConventionRoot resolves and, at most once per process per key,
// bootstraps the convention root for projectRoot.
func bootstrappedConventionRoot(ctx context.Context, projectRoot string) string {
	root, entry, ok := cachedConventionRootEntry(ctx, projectRoot)
	if !ok {
		// canonicalWorktreePath failed (Getwd failure resolving a relative
		// projectRoot); fall back to the uncached path rather than caching
		// under a key we can't reliably reproduce.
		root = filepath.Join(projectsRoot(), projectIdentity(ctx, projectRoot))
		_ = bootstrapConventionRootFn(ctx, projectRoot, root)
		return root
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.bootstrapped {
		if info, err := os.Stat(entry.root); err == nil && info.IsDir() {
			return entry.root
		}
		// The root vanished out from under a long-running process; re-bootstrap.
		entry.bootstrapped = false
	}
	if err := bootstrapConventionRootFn(ctx, projectRoot, entry.root); err == nil {
		entry.bootstrapped = true
	}
	return root
}

// cachedConventionRoot resolves (without bootstrapping) the convention root
// for projectRoot, reusing a cached project identity when available. Used by
// ExistingPath, which must never trigger or trust a bootstrap.
func cachedConventionRoot(ctx context.Context, projectRoot string) string {
	root, _, ok := cachedConventionRootEntry(ctx, projectRoot)
	if !ok {
		return filepath.Join(projectsRoot(), projectIdentity(ctx, projectRoot))
	}
	return root
}

// cachedConventionRootEntry loads or creates the cache entry for projectRoot,
// resolving its identity/root once. ok is false only when projectRoot can't
// be canonicalized (e.g. os.Getwd failing on a relative path), in which case
// callers must not cache and should fall back to the uncached computation.
func cachedConventionRootEntry(ctx context.Context, projectRoot string) (root string, entry *pathCacheEntry, ok bool) {
	worktree, err := canonicalWorktreePath(projectRoot)
	if err != nil {
		return "", nil, false
	}
	key := pathCacheKey{projectsRoot: projectsRoot(), worktree: worktree}
	loaded, _ := pathCache.LoadOrStore(key, &pathCacheEntry{})
	entry = loaded.(*pathCacheEntry)

	entry.mu.Lock()
	if entry.identity == "" {
		entry.identity = projectIdentity(ctx, projectRoot)
		entry.root = filepath.Join(key.projectsRoot, entry.identity)
	}
	root = entry.root
	entry.mu.Unlock()
	return root, entry, true
}

// resetPathCache clears all memoized convention-root state. Tests that
// exercise convention-root bootstrapping under distinct XDG_DATA_HOME/project
// roots don't need this (each key is naturally distinct), but tests that
// reuse a root or assert on repeated bootstrap calls should call it via
// t.Cleanup to avoid cross-test leakage within the same test binary.
func resetPathCache() {
	pathCache.Range(func(key, _ any) bool {
		pathCache.Delete(key)
		return true
	})
}

func projectsRoot() string {
	return filepath.Join(xdgDataHome(), "ddx", "projects")
}

// GlobalDir returns the DDx global state root: ${XDG_DATA_HOME}/ddx/global.
func GlobalDir() string {
	return filepath.Join(xdgDataHome(), "ddx", "global")
}

func xdgDataHome() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return xdg
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "ddx-xdg")
	}
	return filepath.Join(home, ".local", "share")
}

func LoadWorktreeRegistry(ctx context.Context, projectRoot string) (WorktreeRegistry, error) {
	root := Path(ctx, projectRoot)
	return readWorktreeRegistry(filepath.Join(root, "worktrees.json"))
}

func SetMasterWorktree(ctx context.Context, projectRoot, masterPath string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	root := Path(ctx, projectRoot)
	return withBootstrapLock(root, func() error {
		registryPath := filepath.Join(root, "worktrees.json")
		registry, err := readWorktreeRegistry(registryPath)
		if err != nil {
			return err
		}

		canonicalPath, err := canonicalWorktreePath(masterPath)
		if err != nil {
			return err
		}

		now := time.Now().UTC().Format(time.RFC3339Nano)
		hostname, err := os.Hostname()
		if err != nil || strings.TrimSpace(hostname) == "" {
			hostname = "unknown"
		}

		changed := upsertWorktreeRegistryEntry(&registry, canonicalPath, now, hostname)
		if registry.Master != canonicalPath {
			registry.Master = canonicalPath
			changed = true
		}
		if !changed {
			return nil
		}
		return writeWorktreeRegistry(registryPath, registry)
	})
}

func projectIdentity(ctx context.Context, projectRoot string) string {
	return projectIdentityFromRemote(projectRoot, resolveOriginURL(ctx, projectRoot))
}

func projectIdentityFromRemote(projectRoot, remoteURL string) string {
	if remoteProject, ok := parseRemoteProject(remoteURL); ok {
		return remoteProject
	}
	return localProjectIdentity(projectRoot)
}

func resolveOriginURL(ctx context.Context, projectRoot string) string {
	if ctx == nil {
		ctx = context.Background()
	}
	out, err := gitpkg.Command(ctx, projectRoot, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func parseRemoteProject(remoteURL string) (string, bool) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return "", false
	}
	if host, repoPath, ok := parseSCPLikeRemote(remoteURL); ok {
		return filepath.Join(host, repoPath), true
	}
	u, err := url.Parse(remoteURL)
	if err != nil || u.Host == "" {
		return "", false
	}
	repoPath := normalizeRepoPath(u.Path)
	if repoPath == "" {
		return "", false
	}
	return filepath.Join(u.Host, repoPath), true
}

func parseSCPLikeRemote(remoteURL string) (host, repoPath string, ok bool) {
	if strings.Contains(remoteURL, "://") {
		return "", "", false
	}
	at := strings.IndexByte(remoteURL, '@')
	colon := strings.IndexByte(remoteURL, ':')
	if at <= 0 || colon <= at+1 {
		return "", "", false
	}
	host = strings.TrimSpace(remoteURL[at+1 : colon])
	repoPath = normalizeRepoPath(remoteURL[colon+1:])
	if host == "" || repoPath == "" {
		return "", "", false
	}
	return host, repoPath, true
}

func normalizeRepoPath(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "/")
	raw = strings.TrimSuffix(raw, ".git")
	if raw == "" {
		return ""
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	if len(parts) == 0 {
		return ""
	}
	return filepath.Join(parts...)
}

func localProjectIdentity(projectRoot string) string {
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		absRoot = filepath.Clean(projectRoot)
	}
	base := filepath.Base(absRoot)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "project"
	}
	return filepath.Join("local", base+"-"+shortPathHash(absRoot))
}

func shortPathHash(path string) string {
	sum := sha1.Sum([]byte(path))
	return hex.EncodeToString(sum[:])[:8]
}

const bootstrapCommitMessage = "chore: bootstrap ddx state"

var bootstrapLockStaleAge = 2 * time.Minute

type worktreeRegistry struct {
	Paths  []worktreeRegistryEntry `json:"paths"`
	Master string                  `json:"master"`
}

type worktreeRegistryEntry struct {
	Path        string `json:"path"`
	FirstSeenAt string `json:"first_seen_at"`
	LastSeenAt  string `json:"last_seen_at"`
	Hostname    string `json:"hostname"`
}

func bootstrapConventionRoot(ctx context.Context, projectRoot, root string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return withBootstrapLock(root, func() error {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return err
		}
		if err := ensureGitRepo(ctx, root); err != nil {
			return err
		}
		changed, err := ensureWorktreeRegistry(projectRoot, root)
		if err != nil {
			return err
		}
		if err := ensureHeadCommit(ctx, root, changed); err != nil {
			return err
		}
		return nil
	})
}

func withBootstrapLock(root string, fn func() error) error {
	lockDir := root + ".bootstrap.lock"
	if err := os.MkdirAll(filepath.Dir(lockDir), 0o755); err != nil {
		return err
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		err := os.Mkdir(lockDir, 0o755)
		if err == nil {
			_ = os.WriteFile(filepath.Join(lockDir, "pid"), []byte(strconv.Itoa(os.Getpid())), 0o644)
			_ = os.WriteFile(filepath.Join(lockDir, "acquired_at"), []byte(time.Now().UTC().Format(time.RFC3339Nano)), 0o644)
			defer os.RemoveAll(lockDir)
			return fn()
		}
		if breakStaleBootstrapLock(lockDir) {
			continue
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func breakStaleBootstrapLock(lockDir string) bool {
	acquiredAt, err := os.ReadFile(filepath.Join(lockDir, "acquired_at"))
	if err == nil {
		if ts, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(acquiredAt))); parseErr == nil && time.Since(ts) > bootstrapLockStaleAge {
			_ = os.RemoveAll(lockDir)
			return true
		}
	}
	return false
}

func ensureGitRepo(ctx context.Context, root string) error {
	if out, err := gitpkg.Command(ctx, root, "rev-parse", "--git-dir").CombinedOutput(); err == nil {
		_ = out
		return nil
	}
	out, err := gitpkg.Command(ctx, root, "init").CombinedOutput()
	if err != nil {
		return newGitError("git init", out, err)
	}
	return nil
}

func ensureWorktreeRegistry(projectRoot, root string) (bool, error) {
	worktreePath, err := canonicalWorktreePath(projectRoot)
	if err != nil {
		return false, err
	}

	registryPath := filepath.Join(root, "worktrees.json")
	registry, err := readWorktreeRegistry(registryPath)
	if err != nil {
		return false, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "unknown"
	}

	changed := upsertWorktreeRegistryEntry(&registry, worktreePath, now, hostname)
	if registry.Master == "" {
		registry.Master = worktreePath
		changed = true
	}
	if !changed {
		return false, nil
	}
	return true, writeWorktreeRegistry(registryPath, registry)
}

func upsertWorktreeRegistryEntry(registry *worktreeRegistry, worktreePath, now, hostname string) bool {
	for i := range registry.Paths {
		if registry.Paths[i].Path != worktreePath {
			continue
		}
		changed := false
		if registry.Paths[i].FirstSeenAt == "" {
			registry.Paths[i].FirstSeenAt = now
			changed = true
		}
		if registry.Paths[i].LastSeenAt != now {
			if registry.Paths[i].LastSeenAt == "" {
				registry.Paths[i].LastSeenAt = registry.Paths[i].FirstSeenAt
			}
			if registry.Paths[i].LastSeenAt != now {
				registry.Paths[i].LastSeenAt = now
			}
			changed = true
		}
		if registry.Paths[i].Hostname == "" {
			registry.Paths[i].Hostname = hostname
			changed = true
		}
		return changed
	}

	registry.Paths = append(registry.Paths, worktreeRegistryEntry{
		Path:        worktreePath,
		FirstSeenAt: now,
		LastSeenAt:  now,
		Hostname:    hostname,
	})
	return true
}

func canonicalWorktreePath(projectRoot string) (string, error) {
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func readWorktreeRegistry(path string) (worktreeRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return worktreeRegistry{}, nil
		}
		return worktreeRegistry{}, err
	}
	if len(data) == 0 {
		return worktreeRegistry{}, nil
	}
	var registry worktreeRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return worktreeRegistry{}, err
	}
	return registry, nil
}

func writeWorktreeRegistry(path string, registry worktreeRegistry) error {
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func ensureHeadCommit(ctx context.Context, root string, stageRegistry bool) error {
	if headExists(ctx, root) {
		return nil
	}
	if stageRegistry {
		out, err := gitpkg.Command(ctx, root, "add", "worktrees.json").CombinedOutput()
		if err != nil {
			return newGitError("git add worktrees.json", out, err)
		}
	}
	out, err := gitpkg.Command(
		ctx,
		root,
		"-c", "user.name=DDx Bootstrap",
		"-c", "user.email=ddx-bootstrap@localhost",
		"-c", "commit.gpgsign=false",
		"commit", "--allow-empty", "-m", bootstrapCommitMessage,
	).CombinedOutput()
	if err != nil {
		return newGitError("git commit", out, err)
	}
	return nil
}

func headExists(ctx context.Context, root string) bool {
	return gitpkg.Command(ctx, root, "rev-parse", "--verify", "HEAD^{commit}").Run() == nil
}

func newGitError(op string, out []byte, err error) error {
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		return err
	}
	return &gitOperationError{op: op, msg: msg, err: err}
}

type gitOperationError struct {
	op  string
	msg string
	err error
}

func (e *gitOperationError) Error() string {
	return e.op + ": " + e.msg
}

func (e *gitOperationError) Unwrap() error {
	return e.err
}
