package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
)

func TestResolveLogDir(t *testing.T) {
	abs := func(p string) string {
		out, err := filepath.Abs(p)
		if err != nil {
			t.Fatalf("filepath.Abs(%q): %v", p, err)
		}
		return out
	}

	t.Run("empty configured uses convention DDx root when project has no in-tree state", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", t.TempDir())
		projectRoot := t.TempDir()
		want := ddxroot.JoinProject(projectRoot, "agent-logs")
		got := ResolveLogDir(projectRoot, "")
		if got != want {
			t.Errorf("ResolveLogDir(%q, %q) = %q; want %q", projectRoot, "", got, want)
		}
	})

	t.Run("relative ddx path uses convention DDx root when project has no in-tree state", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", t.TempDir())
		projectRoot := t.TempDir()
		want := ddxroot.JoinProject(projectRoot, "agent-logs")
		got := ResolveLogDir(projectRoot, ".ddx/agent-logs")
		if got != want {
			t.Errorf("ResolveLogDir(%q, %q) = %q; want %q", projectRoot, ".ddx/agent-logs", got, want)
		}
	})

	t.Run("in-tree ddx state stays anchored at project root", func(t *testing.T) {
		projectRoot := t.TempDir()
		testutils.MakeInitializedDDxRoot(t, projectRoot)
		want := filepath.Join(projectRoot, ddxroot.DirName, "agent-logs")
		if got := ResolveLogDir(projectRoot, ""); got != want {
			t.Errorf("ResolveLogDir(%q, %q) = %q; want %q", projectRoot, "", got, want)
		}
		if got := ResolveLogDir(projectRoot, ".ddx/agent-logs"); got != want {
			t.Errorf("ResolveLogDir(%q, %q) = %q; want %q", projectRoot, ".ddx/agent-logs", got, want)
		}
	})

	t.Run("relative configured outside ddx stays anchored at project root", func(t *testing.T) {
		projectRoot := t.TempDir()
		want := filepath.Join(projectRoot, "var/logs")
		got := ResolveLogDir(projectRoot, "var/logs")
		if got != want {
			t.Errorf("ResolveLogDir(%q, %q) = %q; want %q", projectRoot, "var/logs", got, want)
		}
	})

	t.Run("absolute configured is returned unchanged", func(t *testing.T) {
		projectRoot := t.TempDir()
		configured := abs("/var/log/ddx")
		got := ResolveLogDir(projectRoot, configured)
		if got != configured {
			t.Errorf("ResolveLogDir(%q, %q) = %q; want %q", projectRoot, configured, got, configured)
		}
	})

	t.Run("empty projectRoot with relative configured returns configured unchanged", func(t *testing.T) {
		got := ResolveLogDir("", ".ddx/agent-logs")
		if got != ".ddx/agent-logs" {
			t.Errorf("ResolveLogDir(%q, %q) = %q; want %q", "", ".ddx/agent-logs", got, ".ddx/agent-logs")
		}
	})

	t.Run("session log helper follows convention DDx root", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", t.TempDir())
		projectRoot := t.TempDir()
		want := filepath.Join(ddxroot.Path(context.Background(), projectRoot), "agent-logs")
		if got := ResolveLogDir(projectRoot, DefaultLogDir); got != want {
			t.Errorf("ResolveLogDir(%q, %q) = %q; want %q", projectRoot, DefaultLogDir, got, want)
		}
	})
}
