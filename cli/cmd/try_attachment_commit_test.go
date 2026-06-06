package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteLoopLanding_StagesAttachmentDir(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env, beadID, head := seedTryAttachmentCommitRepo(t)
	factory := NewCommandFactory(env.Dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (agent.ExecuteBeadReport, error) {
		return agent.ExecuteBeadReport{
			BeadID:           id,
			AttemptID:        "20260515T190001-attach-stage",
			Status:           agent.ExecuteBeadStatusSuccess,
			SessionID:        "sess-attach-stage",
			BaseRev:          head,
			ResultRev:        head,
			ProjectRoot:      env.Dir,
			RequestedProfile: "smart",
		}, nil
	})

	_, err := executeCommand(
		factory.NewRootCommand(),
		"try",
		beadID,
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)
	require.NoError(t, err)

	attachmentDir := filepath.ToSlash(ddxroot.JoinRelative("attachments", beadID))
	attachmentPath := filepath.ToSlash(ddxroot.JoinRelative("attachments", beadID, "events.jsonl"))
	assert.Contains(t, runGitCmd(t, env.Dir, "ls-files", "--", attachmentDir), attachmentPath)
	assert.Empty(t, runGitCmd(t, env.Dir, "status", "--short", "--", attachmentDir))
}

func TestExecuteLoopLanding_AttachmentsCommittedUnderIndexLockContention(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env, beadID, head := seedTryAttachmentCommitRepo(t)
	plantStaleLock(t, env.Dir)

	factory := NewCommandFactory(env.Dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (agent.ExecuteBeadReport, error) {
		return agent.ExecuteBeadReport{
			BeadID:           id,
			AttemptID:        "20260515T190002-attach-lock",
			Status:           agent.ExecuteBeadStatusSuccess,
			SessionID:        "sess-attach-lock",
			BaseRev:          head,
			ResultRev:        head,
			ProjectRoot:      env.Dir,
			RequestedProfile: "smart",
		}, nil
	})

	_, err := executeCommand(
		factory.NewRootCommand(),
		"try",
		beadID,
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)
	require.NoError(t, err)

	attachmentDir := filepath.ToSlash(ddxroot.JoinRelative("attachments", beadID))
	attachmentPath := filepath.ToSlash(ddxroot.JoinRelative("attachments", beadID, "events.jsonl"))
	assert.Contains(t, runGitCmd(t, env.Dir, "ls-files", "--", attachmentDir), attachmentPath)
	assert.Empty(t, runGitCmd(t, env.Dir, "status", "--short", "--", attachmentDir))
	_, statErr := os.Stat(filepath.Join(env.Dir, ".git", "index.lock"))
	assert.True(t, os.IsNotExist(statErr), "stale index.lock should be removed during the durable-audit commit")
}

func seedTryAttachmentCommitRepo(t *testing.T) (*TestEnvironment, string, string) {
	t.Helper()

	env := NewTestEnvironment(t)
	require.NoError(t, os.WriteFile(filepath.Join(env.Dir, "README.md"), []byte("# try attachments\n"), 0o644))

	store := bead.NewStore(filepath.Join(env.Dir, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	beadID := "ddx-try-attachments-001"
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    beadID,
		Title: "Try attachment commit bead",
	}))

	gitAddAndCommit(t, env.Dir, "chore: seed try attachment repo", "README.md", ddxroot.JoinRelative("beads.jsonl"))
	head := runGitCmd(t, env.Dir, "rev-parse", "HEAD")
	return env, beadID, head
}
