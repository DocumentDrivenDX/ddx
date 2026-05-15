package ddxroot

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
)

func TestDDxRoot_InTreeMode(t *testing.T) {
	projectRoot := t.TempDir()
	inTree := filepath.Join(projectRoot, ".ddx")
	if err := os.MkdirAll(inTree, 0o755); err != nil {
		t.Fatalf("mkdir .ddx: %v", err)
	}

	got := Path(context.Background(), projectRoot)
	if got != inTree {
		t.Fatalf("Path() = %q, want %q", got, inTree)
	}
}

func TestDDxRoot_ConventionMode_XDGPath(t *testing.T) {
	projectRoot := t.TempDir()
	initGitRepo(t, projectRoot)
	runGit(t, projectRoot, "remote", "add", "origin", "git@github.com:easel/ddx.git")

	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)

	got := Path(context.Background(), projectRoot)
	want := filepath.Join(xdg, "ddx", "projects", "github.com", "easel", "ddx")
	if got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestDDxRoot_BootstrapInitsGitRepoInXDG(t *testing.T) {
	projectRoot := filepath.Join(t.TempDir(), "demo-project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("mkdir project root: %v", err)
	}
	initGitRepo(t, projectRoot)

	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)

	root := Path(context.Background(), projectRoot)

	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("Path() root %q not bootstrapped as a directory: stat err=%v", root, err)
	}
	if !headExistsForTest(t, root) {
		t.Fatalf("bootstrap root %q has no HEAD commit", root)
	}
	out := runGitOutput(t, root, "rev-parse", "--path-format=absolute", "--git-dir")
	if filepath.Clean(strings.TrimSpace(string(out))) != filepath.Join(root, ".git") {
		t.Fatalf("git dir = %q, want %q", strings.TrimSpace(string(out)), filepath.Join(root, ".git"))
	}
}

func TestDDxRoot_BootstrapIdempotent(t *testing.T) {
	projectRoot := filepath.Join(t.TempDir(), "demo-project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("mkdir project root: %v", err)
	}
	initGitRepo(t, projectRoot)

	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)

	const callers = 2
	start := make(chan struct{})
	results := make([]string, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			results[idx] = Path(context.Background(), projectRoot)
		}(i)
	}

	close(start)
	wg.Wait()

	for i := 1; i < callers; i++ {
		if results[i] != results[0] {
			t.Fatalf("Path() mismatch under concurrent bootstrap: %q vs %q", results[i], results[0])
		}
	}
	if !headExistsForTest(t, results[0]) {
		t.Fatalf("bootstrap root %q has no HEAD commit after concurrent bootstrap", results[0])
	}

	var registry worktreeRegistry
	readJSONFile(t, filepath.Join(results[0], "worktrees.json"), &registry)
	if len(registry.Paths) != 1 {
		t.Fatalf("registered paths = %d, want 1", len(registry.Paths))
	}
}

func TestWorktreeRegistry_FirstWorktreeIsMaster(t *testing.T) {
	projectRoot := filepath.Join(t.TempDir(), "demo-project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("mkdir project root: %v", err)
	}
	initGitRepo(t, projectRoot)

	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)

	root := Path(context.Background(), projectRoot)

	var registry worktreeRegistry
	readJSONFile(t, filepath.Join(root, "worktrees.json"), &registry)

	absProjectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		t.Fatalf("abs project root: %v", err)
	}
	if registry.Master != absProjectRoot {
		t.Fatalf("registry master = %q, want %q", registry.Master, absProjectRoot)
	}
	if len(registry.Paths) != 1 {
		t.Fatalf("registry paths = %d, want 1", len(registry.Paths))
	}
	entry := registry.Paths[0]
	if entry.Path != absProjectRoot {
		t.Fatalf("registry path = %q, want %q", entry.Path, absProjectRoot)
	}
	if entry.FirstSeenAt == "" {
		t.Fatalf("first_seen_at should be populated")
	}
	if entry.LastSeenAt == "" {
		t.Fatalf("last_seen_at should be populated")
	}
	if entry.Hostname == "" {
		t.Fatalf("hostname should be populated")
	}
}

func TestDDxRoot_ConventionMode_LocalFallback(t *testing.T) {
	projectRoot := filepath.Join(t.TempDir(), "demo-project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("mkdir project root: %v", err)
	}
	initGitRepo(t, projectRoot)

	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)

	got := Path(context.Background(), projectRoot)
	want := filepath.Join(xdg, "ddx", "projects", expectedLocalIdentity(projectRoot))
	if got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestDDxRoot_URLParsing(t *testing.T) {
	projectRoot := filepath.Join(t.TempDir(), "demo-project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("mkdir project root: %v", err)
	}

	tests := []struct {
		name      string
		remoteURL string
		want      string
	}{
		{
			name:      "https github",
			remoteURL: "https://github.com/easel/ddx.git",
			want:      filepath.Join("github.com", "easel", "ddx"),
		},
		{
			name:      "ssh github",
			remoteURL: "git@github.com:easel/ddx.git",
			want:      filepath.Join("github.com", "easel", "ddx"),
		},
		{
			name:      "custom host",
			remoteURL: "https://gitlab.example.com/team/repo",
			want:      filepath.Join("gitlab.example.com", "team", "repo"),
		},
		{
			name:      "missing remote",
			remoteURL: "",
			want:      expectedLocalIdentity(projectRoot),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectIdentityFromRemote(projectRoot, tt.remoteURL)
			if got != tt.want {
				t.Fatalf("projectIdentityFromRemote(%q) = %q, want %q", tt.remoteURL, got, tt.want)
			}
		})
	}
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := gitpkg.Command(context.Background(), dir, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	cmd := gitpkg.Command(context.Background(), dir, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return out
}

func headExistsForTest(t *testing.T, dir string) bool {
	t.Helper()
	return gitpkg.Command(context.Background(), dir, "rev-parse", "--verify", "HEAD^{commit}").Run() == nil
}

func readJSONFile(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
}

func expectedLocalIdentity(projectRoot string) string {
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		absRoot = filepath.Clean(projectRoot)
	}
	sum := sha1.Sum([]byte(absRoot))
	return filepath.Join("local", filepath.Base(absRoot)+"-"+hex.EncodeToString(sum[:])[:8])
}
