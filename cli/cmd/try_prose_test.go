package cmd

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// proseStubChangedFiles returns a fixed list of paths. Used by tests to drive
// the real AttachProseEvidence machinery without needing a real git diff.
func proseStubChangedFiles(paths []string) func(string, string, string) ([]string, error) {
	return func(_, _, _ string) ([]string, error) {
		return append([]string(nil), paths...), nil
	}
}

// proseStubReadFile returns fixed content for any docs path. The content
// includes phrases the embedded checker treats as findings so the hook
// records a non-empty findings list even without a real worktree.
func proseStubReadFile(content string) func(string, string, string) ([]byte, error) {
	return func(_, _, _ string) ([]byte, error) {
		return []byte(content), nil
	}
}

func newTryProseTestBead(t *testing.T, env *TestEnvironment, id string) *bead.Store {
	t.Helper()
	store := bead.NewStore(env.Dir + "/.ddx")
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    id,
		Title: "docs-changing attempt",
	}))
	return store
}

func newTryProseStubReport(beadID string) agent.ExecuteBeadReport {
	return agent.ExecuteBeadReport{
		BeadID:    beadID,
		AttemptID: "20260514T155540-prose-test",
		Status:    agent.ExecuteBeadStatusSuccess,
		SessionID: "sess-prose",
		BaseRev:   "deadbeef00000001",
		ResultRev: "deadbeef00000002",
	}
}

// TestTryDocsChanged_AttachesProseEvidence: when an attempt changes Markdown
// under docs/, the prose hook runs and a prose-check.findings event is
// appended to the bead history.
func TestTryDocsChanged_AttachesProseEvidence(t *testing.T) {
	env := NewTestEnvironment(t)
	store := newTryProseTestBead(t, env, "docs-bead-001")

	factory := NewCommandFactory(env.Dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		return newTryProseStubReport(beadID), nil
	})
	factory.proseEvidenceHookOverride = agent.NewDefaultProseEvidenceHook(agent.ProseEvidenceConfig{
		ProjectRoot:   env.Dir,
		Events:        store,
		Actor:         "test-worker",
		Source:        "ddx try",
		ChangedFiles:  proseStubChangedFiles([]string{"docs/example.md"}),
		ReadFileAtRev: proseStubReadFile("# Example\n\nThis is content.\n"),
	})

	root := factory.NewRootCommand()
	out, err := executeCommand(root, "try", "docs-bead-001", "--no-review", "--no-review-i-know-what-im-doing")
	require.NoError(t, err, "ddx try must succeed: %s", out)

	events, err := store.Events("docs-bead-001")
	require.NoError(t, err)

	var proseEvent *bead.BeadEvent
	for i := range events {
		if events[i].Kind == agent.ProseEvidenceEventKind {
			proseEvent = &events[i]
			break
		}
	}
	require.NotNil(t, proseEvent, "expected prose-check.findings event after docs-changing attempt")

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(proseEvent.Body), &body))
	assert.Equal(t, true, body["advisory"], "prose findings must be marked advisory")
	docs, _ := body["changed_docs"].([]any)
	require.Len(t, docs, 1)
	assert.Equal(t, "docs/example.md", docs[0])
}

// TestTryNoDocsChanged_SkipsProseCheck: when an attempt's diff contains no
// docs/**/*.md files, the prose hook records nothing on the bead.
func TestTryNoDocsChanged_SkipsProseCheck(t *testing.T) {
	env := NewTestEnvironment(t)
	store := newTryProseTestBead(t, env, "nodocs-bead-001")

	factory := NewCommandFactory(env.Dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		return newTryProseStubReport(beadID), nil
	})
	factory.proseEvidenceHookOverride = agent.NewDefaultProseEvidenceHook(agent.ProseEvidenceConfig{
		ProjectRoot:   env.Dir,
		Events:        store,
		Actor:         "test-worker",
		Source:        "ddx try",
		ChangedFiles:  proseStubChangedFiles([]string{"cli/cmd/main.go", "README.md"}),
		ReadFileAtRev: proseStubReadFile("ignored"),
	})

	root := factory.NewRootCommand()
	out, err := executeCommand(root, "try", "nodocs-bead-001", "--no-review", "--no-review-i-know-what-im-doing")
	require.NoError(t, err, "ddx try must succeed: %s", out)

	events, err := store.Events("nodocs-bead-001")
	require.NoError(t, err)
	for _, ev := range events {
		assert.NotEqual(t, agent.ProseEvidenceEventKind, ev.Kind,
			"prose-check.findings event must NOT be appended when no docs/*.md files changed")
	}
}

// TestTryProseFindings_AdvisoryByDefault: prose findings are recorded but
// must not block closure — the bead still transitions to closed.
func TestTryProseFindings_AdvisoryByDefault(t *testing.T) {
	env := NewTestEnvironment(t)
	store := newTryProseTestBead(t, env, "advisory-bead-001")

	factory := NewCommandFactory(env.Dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		return newTryProseStubReport(beadID), nil
	})
	factory.proseEvidenceHookOverride = agent.NewDefaultProseEvidenceHook(agent.ProseEvidenceConfig{
		ProjectRoot:   env.Dir,
		Events:        store,
		Actor:         "test-worker",
		Source:        "ddx try",
		ChangedFiles:  proseStubChangedFiles([]string{"docs/advisory.md"}),
		ReadFileAtRev: proseStubReadFile("# Advisory\n\nThis is robust comprehensive content.\n"),
	})

	root := factory.NewRootCommand()
	out, err := executeCommand(root, "try", "advisory-bead-001", "--no-review", "--no-review-i-know-what-im-doing")
	require.NoError(t, err, "ddx try with advisory prose findings must still exit zero: %s", out)

	b, err := store.Get("advisory-bead-001")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, b.Status,
		"prose findings must not block closure; bead must be closed after successful attempt")
}

// TestTryProseFindings_FedBackBeforeFinalization: the prose hook runs before
// the bead is closed so findings land in the editable attempt's evidence
// history. We verify this by capturing the bead status when the hook fires —
// it must still be in_progress, not closed.
func TestTryProseFindings_FedBackBeforeFinalization(t *testing.T) {
	env := NewTestEnvironment(t)
	store := newTryProseTestBead(t, env, "timing-bead-001")

	var mu sync.Mutex
	var statusAtHook string
	var hookFired bool

	factory := NewCommandFactory(env.Dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		return newTryProseStubReport(beadID), nil
	})
	factory.proseEvidenceHookOverride = func(ctx context.Context, beadID string, report agent.ExecuteBeadReport) error {
		mu.Lock()
		defer mu.Unlock()
		hookFired = true
		if b, err := store.Get(beadID); err == nil {
			statusAtHook = b.Status
		}
		return nil
	}

	root := factory.NewRootCommand()
	out, err := executeCommand(root, "try", "timing-bead-001", "--no-review", "--no-review-i-know-what-im-doing")
	require.NoError(t, err, "ddx try must succeed: %s", out)

	mu.Lock()
	defer mu.Unlock()
	require.True(t, hookFired, "prose hook must run for a successful docs-changing attempt")
	assert.Equal(t, bead.StatusInProgress, statusAtHook,
		"prose hook must run before CloseWithEvidence; bead status at hook time must still be in_progress, not closed")

	// Sanity: after the loop completes the bead must be closed.
	b, err := store.Get("timing-bead-001")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, b.Status)
}
