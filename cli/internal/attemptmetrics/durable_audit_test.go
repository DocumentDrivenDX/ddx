package attemptmetrics_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttemptMetricsAppendLeavesNoDirtyTrackedFile(t *testing.T) {
	projectRoot := initDurableAuditRepo(t)
	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-metrics-clean", Title: "Metrics clean"}))
	runGitAttemptMetrics(t, projectRoot, "add", ".")
	runGitAttemptMetrics(t, projectRoot, "commit", "-m", "chore: seed tracker")

	report := agent.ExecuteBeadReport{
		BeadID:           "ddx-metrics-clean",
		AttemptID:        "20260515T101828-metrics-clean",
		Status:           agent.ExecuteBeadStatusSuccess,
		SessionID:        "sess-metrics-clean",
		Model:            "gpt-5.5",
		Harness:          "codex",
		Provider:         "openai",
		RequestedProfile: "smart",
		ProjectRoot:      projectRoot,
	}
	require.NoError(t, agent.FinalizeDurableAttemptAudit(projectRoot, store, report))

	status := runGitAttemptMetrics(t, projectRoot, "status", "--short", "--", ".ddx/metrics/attempts.jsonl")
	assert.Empty(t, status)

	subject := runGitAttemptMetrics(t, projectRoot, "log", "-1", "--pretty=%s")
	assert.Equal(t, "chore: update tracker (execute-bead 20260515T101828-metrics-clean)", subject)

	raw, err := os.ReadFile(ddxroot.JoinProject(projectRoot, "metrics", "attempts.jsonl"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), report.AttemptID)
}

func initDurableAuditRepo(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	runGitAttemptMetrics(t, root, "init", "-b", "main")
	runGitAttemptMetrics(t, root, "config", "user.email", "test@ddx.test")
	runGitAttemptMetrics(t, root, "config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# test\n"), 0o644))
	return root
}

func runGitAttemptMetrics(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = scrubbedAttemptMetricsGitEnv()
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, strings.TrimSpace(string(out)))
	return strings.TrimSpace(string(out))
}

func scrubbedAttemptMetricsGitEnv() []string {
	parent := os.Environ()
	env := make([]string, 0, len(parent))
	for _, kv := range parent {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		env = append(env, kv)
	}
	return env
}
