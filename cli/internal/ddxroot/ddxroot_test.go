package ddxroot

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
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

func expectedLocalIdentity(projectRoot string) string {
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		absRoot = filepath.Clean(projectRoot)
	}
	sum := sha1.Sum([]byte(absRoot))
	return filepath.Join("local", filepath.Base(absRoot)+"-"+hex.EncodeToString(sum[:])[:8])
}
