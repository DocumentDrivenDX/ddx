package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeadMigrate_AutoCommits(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env := NewTestEnvironment(t)
	env.CreateConfig(autoCommitConfig)
	require.NoError(t, os.WriteFile(filepath.Join(env.Dir, "README.md"), []byte("# migrate\n"), 0o644))

	old := time.Now().UTC().Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	rows := strings.Join([]string{
		`{"id":"ddx-c1","title":"closed with events","status":"closed","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `","closing_commit_sha":"deadbeef","events":[{"kind":"review","summary":"APPROVE","body":"ok","created_at":"` + old + `"}]}`,
		`{"id":"ddx-c2","title":"closed no events","status":"closed","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `","closing_commit_sha":"deadbeef"}`,
		`{"id":"ddx-open","title":"open","status":"open","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `"}`,
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(env.Dir, ddxroot.DirName, "beads.jsonl"), []byte(rows), 0o644))
	configRel := ddxroot.JoinRelative("config.yaml")
	beadsRel := ddxroot.JoinRelative("beads.jsonl")
	archiveRel := ddxroot.JoinRelative("beads-archive.jsonl")
	attachmentsRel := ddxroot.JoinRelative("attachments")

	gitAddAndCommit(t, env.Dir, "chore: seed migrate repo", "README.md", configRel, beadsRel)

	out, err := executeCommand(NewCommandFactory(env.Dir).NewRootCommand(), "bead", "migrate", "--json")
	require.NoError(t, err)

	var stats struct {
		EventsExternalized int `json:"EventsExternalized"`
		Archived           int `json:"Archived"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &stats))
	assert.Equal(t, 1, stats.EventsExternalized)
	assert.Equal(t, 2, stats.Archived)

	subject := runGitCmd(t, env.Dir, "log", "-1", "--pretty=%s")
	assert.Regexp(t, `^chore\(beads\): externalize and archive`, subject)

	paths := runGitCmd(t, env.Dir, "show", "--name-only", "--pretty=format:", "HEAD")
	assert.Contains(t, paths, filepath.ToSlash(beadsRel))
	assert.Contains(t, paths, filepath.ToSlash(archiveRel))
	assert.Contains(t, paths, filepath.ToSlash(ddxroot.JoinRelative("attachments", "ddx-c1", "events.jsonl")))

	status := runGitCmd(t, env.Dir, "status", "--short", "--", filepath.ToSlash(beadsRel), filepath.ToSlash(archiveRel), filepath.ToSlash(attachmentsRel))
	assert.Empty(t, status)
}

func TestBeadMigrate_NoChangesNoCommit(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env := NewTestEnvironment(t)
	env.CreateConfig(autoCommitConfig)
	require.NoError(t, os.WriteFile(filepath.Join(env.Dir, "README.md"), []byte("# migrate\n"), 0o644))

	old := time.Now().UTC().Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	rows := strings.Join([]string{
		`{"id":"ddx-c1","title":"closed with events","status":"closed","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `","closing_commit_sha":"deadbeef","events":[{"kind":"review","summary":"APPROVE","body":"ok","created_at":"` + old + `"}]}`,
		`{"id":"ddx-open","title":"open","status":"open","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `"}`,
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(env.Dir, ddxroot.DirName, "beads.jsonl"), []byte(rows), 0o644))
	configRel := ddxroot.JoinRelative("config.yaml")
	beadsRel := ddxroot.JoinRelative("beads.jsonl")
	gitAddAndCommit(t, env.Dir, "chore: seed migrate repo", "README.md", configRel, beadsRel)

	_, err := executeCommand(NewCommandFactory(env.Dir).NewRootCommand(), "bead", "migrate", "--json")
	require.NoError(t, err)
	headAfterFirstRun := runGitCmd(t, env.Dir, "rev-parse", "HEAD")

	out, err := executeCommand(NewCommandFactory(env.Dir).NewRootCommand(), "bead", "migrate", "--json")
	require.NoError(t, err)

	var stats struct {
		EventsExternalized int `json:"EventsExternalized"`
		Archived           int `json:"Archived"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &stats))
	assert.Equal(t, 0, stats.EventsExternalized)
	assert.Equal(t, 0, stats.Archived)
	assert.Equal(t, headAfterFirstRun, runGitCmd(t, env.Dir, "rev-parse", "HEAD"))
}

func TestBeadReconcileAttachments(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env := NewTestEnvironment(t)
	require.NoError(t, os.WriteFile(filepath.Join(env.Dir, "README.md"), []byte("# attachments\n"), 0o644))

	existingPath := filepath.Join(env.Dir, ddxroot.DirName, "attachments", "ddx-existing", "events.jsonl")
	existingRel := ddxroot.JoinRelative("attachments", "ddx-existing", "events.jsonl")
	newRel := ddxroot.JoinRelative("attachments", "ddx-new", "events.jsonl")
	attachmentsRel := ddxroot.JoinRelative("attachments")
	require.NoError(t, os.MkdirAll(filepath.Dir(existingPath), 0o755))
	require.NoError(t, os.WriteFile(existingPath, []byte("{\"kind\":\"review\"}\n"), 0o644))
	gitAddAndCommit(t, env.Dir, "chore: seed attachments", "README.md", existingRel)

	require.NoError(t, os.WriteFile(existingPath, []byte("{\"kind\":\"review\"}\n{\"kind\":\"summary\"}\n"), 0o644))
	newPath := filepath.Join(env.Dir, ddxroot.DirName, "attachments", "ddx-new", "events.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(newPath), 0o755))
	require.NoError(t, os.WriteFile(newPath, []byte("{\"kind\":\"audit\"}\n"), 0o644))

	out, err := executeCommand(NewCommandFactory(env.Dir).NewRootCommand(), "bead", "reconcile-attachments")
	require.NoError(t, err)

	want := "chore(beads): reconcile missing attachment commits (1 dirs, 1 modified)"
	assert.Contains(t, out, want)
	assert.Equal(t, want, runGitCmd(t, env.Dir, "log", "-1", "--pretty=%s"))

	paths := runGitCmd(t, env.Dir, "show", "--name-only", "--pretty=format:", "HEAD")
	assert.Contains(t, paths, filepath.ToSlash(existingRel))
	assert.Contains(t, paths, filepath.ToSlash(newRel))
	assert.Empty(t, runGitCmd(t, env.Dir, "status", "--short", "--", filepath.ToSlash(attachmentsRel)))
}

func TestBeadReconcileAttachments_NoChanges(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env := NewTestEnvironment(t)
	require.NoError(t, os.WriteFile(filepath.Join(env.Dir, "README.md"), []byte("# clean\n"), 0o644))
	gitAddAndCommit(t, env.Dir, "chore: seed clean repo", "README.md")

	headBefore := runGitCmd(t, env.Dir, "rev-parse", "HEAD")
	out, err := executeCommand(NewCommandFactory(env.Dir).NewRootCommand(), "bead", "reconcile-attachments")
	require.NoError(t, err)

	assert.Contains(t, out, "no attachment changes to reconcile")
	assert.Equal(t, headBefore, runGitCmd(t, env.Dir, "rev-parse", "HEAD"))
}

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := runGitCmdOutput(dir, args...)
	require.NoError(t, err, "git %v failed: %s", args, strings.TrimSpace(out))
	return strings.TrimSpace(out)
}

func runGitCmdOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = scrubbedCmdGitEnv()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func scrubbedCmdGitEnv() []string {
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
