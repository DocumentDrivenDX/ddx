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

// loadLefthookPrePushSection reads lefthook.yml and returns the content
// of the pre-push block.
func loadLefthookPrePushSection(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "lefthook.yml"))
	require.NoError(t, err)
	content := string(data)

	idx := strings.Index(content, "pre-push:")
	require.True(t, idx >= 0, "lefthook.yml: pre-push block not found")
	return content[idx:]
}

// loadMakefileTestFullSection reads cli/Makefile and returns the content
// of the test-full target.
func loadMakefileTestFullSection(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "Makefile"))
	require.NoError(t, err)
	content := string(data)

	idx := strings.Index(content, "test-full:")
	require.True(t, idx >= 0, "Makefile: test-full target not found")
	end := idx + 512
	if end > len(content) {
		end = len(content)
	}
	return content[idx:end]
}

// TestLefthookPrePushNoFullTests verifies the pre-push hook no longer runs
// the full test suite — CI on origin provides that gate instead.
func TestLefthookPrePushNoFullTests(t *testing.T) {
	section := loadLefthookPrePushSection(t)
	assert.NotContains(t, section, "full-tests:",
		"pre-push hook must not run the full test suite; CI on origin handles that gate")
	assert.NotContains(t, section, "go test -race -cover",
		"pre-push hook must not run go test -race -cover; that belongs in CI or make test-full")
}

// TestMakefileTestFullSetsCGOForRace verifies the Makefile test-full target
// sets CGO_ENABLED=1 so the race detector works in environments that inherit
// CGO_ENABLED=0.
func TestMakefileTestFullSetsCGOForRace(t *testing.T) {
	section := loadMakefileTestFullSection(t)
	assert.Contains(t, section, "CGO_ENABLED=1",
		"test-full must set CGO_ENABLED=1 so -race works when env has CGO_ENABLED=0")
	assert.Contains(t, section, "go test -race",
		"test-full must invoke go test -race")
}

// TestMakefileTestFullScrubsGitLocalEnv verifies the Makefile test-full target
// removes hook-exported Git-local environment variables before tests create
// fixture repositories.
func TestMakefileTestFullScrubsGitLocalEnv(t *testing.T) {
	section := loadMakefileTestFullSection(t)
	scrubIdx := strings.Index(section, "git rev-parse --local-env-vars")
	testIdx := strings.Index(section, "go test -race")

	require.NotEqual(t, -1, scrubIdx, "test-full must enumerate Git-local environment variables")
	require.NotEqual(t, -1, testIdx, "test-full must invoke go test -race")
	assert.Less(t, scrubIdx, testIdx, "Git-local environment must be scrubbed before go test runs")
}

func runGitConfigHealthScript(t *testing.T, config string) (string, error) {
	t.Helper()
	repoRoot := t.TempDir()
	gitDir := filepath.Join(repoRoot, ".git")
	require.NoError(t, os.Mkdir(gitDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte(config), 0o644))

	cmd := exec.Command("sh", filepath.Join("..", "..", "scripts", "git-config-health.sh"))
	cmd.Env = append(os.Environ(), "DDX_GIT_CONFIG_HEALTH_ROOT="+repoRoot)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func TestGitConfigHealthScriptRejectsCoreBare(t *testing.T) {
	output, err := runGitConfigHealthScript(t, `[core]
	repositoryformatversion = 0
	bare = true
`)

	require.Error(t, err)
	assert.Contains(t, output, "core.bare=true")
	assert.Contains(t, output, "git config --unset core.bare")
}

func TestGitConfigHealthScriptRejectsFixtureIdentity(t *testing.T) {
	output, err := runGitConfigHealthScript(t, `[core]
	repositoryformatversion = 0
	bare = false
[user]
	name = DDx Fixture
	email = fixture@ddx.test
`)

	require.Error(t, err)
	assert.Contains(t, output, "fixture identity")
	assert.Contains(t, output, "git config --unset user.name")
	assert.Contains(t, output, "git config --unset user.email")
}
