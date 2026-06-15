package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadLefthookPreCommitSection(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "lefthook.yml"))
	require.NoError(t, err)
	content := string(data)

	idx := strings.Index(content, "pre-commit:")
	require.True(t, idx >= 0, "lefthook.yml: pre-commit block not found")
	end := strings.Index(content[idx:], "\nci:")
	require.True(t, end >= 0, "lefthook.yml: ci block not found after pre-commit")
	return content[idx : idx+end]
}

func TestLefthookPreCommitIncludesDDxRootAudit(t *testing.T) {
	section := loadLefthookPreCommitSection(t)

	assert.Contains(t, section, "ddxroot-path-audit:")
	assert.Contains(t, section, `filepath\.Join\([^)]*"\.ddx"`)
	assert.Contains(t, section, `filepath\.Join\(\s*(projectRoot|[A-Za-z0-9_\.]*WorkingDir)\s*,\s*[A-Za-z_][A-Za-z0-9_]*RelPath\s*\)`)
	assert.Contains(t, section, `(projectRoot|[A-Za-z0-9_\.]*WorkingDir)\s*\+\s*"/\.ddx(?:/|")`)
	assert.Contains(t, section, `grep -Ev '(^cli/internal/ddxroot/)|(_test\.go$)'`)
	assert.Contains(t, section, "Hardcoded DDx state-root callsites found outside cli/internal/ddxroot/ and approved test fixtures")
}

func TestPreCommitGoTestFailsWhenAnyPackageFails(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "cmd"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "internal", "agent"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "cmd", "x_test.go"), []byte("package cmd\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "internal", "agent", "x_test.go"), []byte("package agent\n"), 0o644))

	bin := filepath.Join(tmp, "bin")
	require.NoError(t, os.MkdirAll(bin, 0o755))
	fakeGo := `#!/bin/sh
case "$*" in
  *"./cmd"*) echo "cmd failed"; exit 7 ;;
  *"./internal/agent"*) echo "agent passed"; exit 0 ;;
  *) echo "unexpected args: $*" >&2; exit 2 ;;
esac
`
	require.NoError(t, os.WriteFile(filepath.Join(bin, "go"), []byte(fakeGo), 0o755))

	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "lefthook-go-test.sh"))
	require.NoError(t, err)
	cmd := exec.Command("sh", script, "./cmd/bead.go", "./internal/agent/types.go")
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "a failing first package must make the hook script fail even if a later package passes")
	assert.Contains(t, string(out), "cmd failed")
	assert.Contains(t, string(out), "agent passed")
}
