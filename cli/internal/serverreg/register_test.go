package serverreg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsTransientProjectPathMatchesPollution(t *testing.T) {
	cases := []string{
		filepath.Join(os.TempDir(), "ddx-go", "TestExecuteBeadInvalidBeadIDbead^1233542036", "001"),
		filepath.Join(os.TempDir(), "TestFoo123", "001"),
		"/home/erik/tmp/TestExecuteBeadInvalidBeadIDbead~1324027905/001",
		"/home/erik/.cache/fleet-tmp/ddx-cmd-tests-774492673/TestBeadCommandsCRUDLifecycle2946498393/001",
		"/home/erik/.cache/fleet-tmp/TestIntegration_MultiWorkerLockContention_5Workers607960191/001/ddxfixture",
		"/Users/erik/Projects/.ddx-exec-wt/.execute-bead-wt-ddx-12345678-20260505T000000-deadbeef",
		"/Users/erik/Projects/ddx/.claude/worktrees/agent-a7bf6d44",
		"/Users/erik/Projects/helix-yolo/runs/codex-20260525T190541/workspace",
	}
	for _, c := range cases {
		if !isTransientProjectPath(c) {
			t.Errorf("expected transient project path: %s", c)
		}
	}
}

func TestIsTransientProjectPathAllowsRealProjects(t *testing.T) {
	cases := []string{
		"/Users/erik/Projects/ddx",
		"/Users/erik/Projects/tablespec",
		"/home/user/Projects/helix",
		"/srv/ddx",
	}
	for _, c := range cases {
		if isTransientProjectPath(c) {
			t.Errorf("expected real project path: %s", c)
		}
	}
}
