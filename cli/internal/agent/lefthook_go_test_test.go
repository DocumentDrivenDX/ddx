package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func repoRootForLefthookGoTest(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
}

func copyLefthookGoTestScript(t *testing.T) string {
	t.Helper()

	source := filepath.Join(repoRootForLefthookGoTest(t), "scripts", "lefthook-go-test.sh")
	data, err := os.ReadFile(source)
	require.NoError(t, err)

	root := t.TempDir()
	scriptPath := filepath.Join(root, "scripts", "lefthook-go-test.sh")
	require.NoError(t, os.MkdirAll(filepath.Dir(scriptPath), 0o755))
	require.NoError(t, os.WriteFile(scriptPath, data, 0o755))

	require.NoError(t, os.MkdirAll(filepath.Join(root, "cli", "internal", "agent"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "notes"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "cli", "internal", "agent", "example.go"), []byte("package agent\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "cli", "internal", "agent", "another.go"), []byte("package agent\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "cli", "internal", "agent", "example_test.go"), []byte("package agent\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes", "example.go"), []byte("package notes\n"), 0o644))

	return root
}

func fakeGoBinary(t *testing.T) (string, string) {
	t.Helper()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "go.log")
	require.NoError(t, os.WriteFile(logPath, nil, 0o644))
	script := "#!/bin/sh\nprintf '%s | go %s\\n' \"$(pwd)\" \"$*\" >> \"$FAKE_GO_LOG\"\n"
	script += "exit 0\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go"), []byte(script), 0o755))
	return dir, logPath
}

func envWithTestOverrides(overrides map[string]string) []string {
	skip := make(map[string]struct{}, len(overrides))
	for key := range overrides {
		skip[key] = struct{}{}
	}

	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if _, ok := skip[key]; ok {
			continue
		}
		env = append(env, kv)
	}
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	return env
}

func runLefthookGoTestScript(t *testing.T, dir, scriptPath, fakeGoDir, logPath string, args ...string) (string, error) {
	t.Helper()

	cmd := exec.Command("sh", append([]string{scriptPath}, args...)...)
	cmd.Dir = dir
	cmd.Env = envWithTestOverrides(map[string]string{
		"FAKE_GO_LOG": logPath,
		"PATH":        fakeGoDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	})
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func readGoLog(t *testing.T, logPath string) []string {
	t.Helper()

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil
	}

	var lines []string
	for _, line := range strings.Split(trimmed, "\n") {
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func TestLefthookGoTest_FromRepoRootRunsInternalAgent(t *testing.T) {
	root := copyLefthookGoTestScript(t)
	fakeGoDir, logPath := fakeGoBinary(t)

	out, err := runLefthookGoTestScript(t, root, filepath.Join(root, "scripts", "lefthook-go-test.sh"), fakeGoDir, logPath, "cli/internal/agent/example.go")
	require.NoError(t, err, out)

	lines := readGoLog(t, logPath)
	require.Len(t, lines, 1)
	assert.Equal(t, filepath.Join(root, "cli")+" | go test -short -race -timeout 30m ./internal/agent", lines[0])
}

func TestLefthookGoTest_FromCliRootRemainsCompatible(t *testing.T) {
	root := copyLefthookGoTestScript(t)
	fakeGoDir, logPath := fakeGoBinary(t)

	out, err := runLefthookGoTestScript(t, filepath.Join(root, "cli"), filepath.Join(root, "scripts", "lefthook-go-test.sh"), fakeGoDir, logPath, "internal/agent/example.go")
	require.NoError(t, err, out)

	lines := readGoLog(t, logPath)
	require.Len(t, lines, 1)
	assert.Equal(t, filepath.Join(root, "cli")+" | go test -short -race -timeout 30m ./internal/agent", lines[0])
}

func TestLefthookGoTest_UnmappedGoPathFailsClosed(t *testing.T) {
	root := copyLefthookGoTestScript(t)
	fakeGoDir, logPath := fakeGoBinary(t)

	out, err := runLefthookGoTestScript(t, root, filepath.Join(root, "scripts", "lefthook-go-test.sh"), fakeGoDir, logPath, "notes/example.go")
	require.Error(t, err)
	assert.Contains(t, out, "cannot map Go path notes/example.go")
	assert.Empty(t, readGoLog(t, logPath))
}

func TestLefthookGoTest_DeduplicatesPackageSelection(t *testing.T) {
	root := copyLefthookGoTestScript(t)
	fakeGoDir, logPath := fakeGoBinary(t)

	out, err := runLefthookGoTestScript(t, root, filepath.Join(root, "scripts", "lefthook-go-test.sh"), fakeGoDir, logPath, "cli/internal/agent/example.go", "cli/internal/agent/another.go")
	require.NoError(t, err, out)

	lines := readGoLog(t, logPath)
	require.Len(t, lines, 1)
	assert.Equal(t, filepath.Join(root, "cli")+" | go test -short -race -timeout 30m ./internal/agent", lines[0])
}

func readFileString(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
