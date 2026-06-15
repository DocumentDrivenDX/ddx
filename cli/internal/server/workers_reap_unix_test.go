//go:build !windows

package server

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkerDescendantCleanupCandidatesSelectsAttemptHooksAndProviders(t *testing.T) {
	rows := []workerProcessRow{
		{PID: 100, PPID: 1, PGID: 100, Command: "/home/erik/.local/bin/ddx server"},
		{PID: 101, PPID: 100, PGID: 100, Command: "git commit -m chore: execute-bead iteration -m Ddx-Attempt-Id: attempt-1"},
		{PID: 102, PPID: 101, PGID: 100, Command: "/bin/sh .git/hooks/pre-commit"},
		{PID: 103, PPID: 102, PGID: 100, Command: "lefthook run pre-commit"},
		{PID: 104, PPID: 100, PGID: 104, Command: "/home/linuxbrew/.linuxbrew/bin/codex --no-alt-screen"},
		{PID: 105, PPID: 104, PGID: 104, Command: "node /home/linuxbrew/.linuxbrew/bin/gemini"},
		{PID: 200, PPID: 1, PGID: 200, Command: "codex --dangerously-bypass-approvals-and-sandbox resume"},
	}

	got := workerDescendantCleanupCandidates(rows, 100, "attempt-1")
	pids := make([]int, 0, len(got))
	reasons := map[int]string{}
	for _, candidate := range got {
		pids = append(pids, candidate.PID)
		reasons[candidate.PID] = candidate.Reason
	}
	sort.Ints(pids)

	assert.Equal(t, []int{101, 102, 103, 104, 105}, pids)
	assert.Equal(t, "attempt_finalization", reasons[101])
	assert.Equal(t, "attempt_finalization", reasons[102])
	assert.Equal(t, "attempt_finalization", reasons[103])
	assert.Equal(t, "provider_cli", reasons[104])
	assert.Equal(t, "provider_cli", reasons[105])
}

func TestWorkerProviderForCommandDetectsNodeWrappedProvider(t *testing.T) {
	assert.Equal(t, "claude", workerProviderForCommand("/home/erik/.local/bin/claude --model sonnet"))
	assert.Equal(t, "gemini", workerProviderForCommand("node /home/linuxbrew/.linuxbrew/bin/gemini"))
	assert.Equal(t, "codex", workerProviderForCommand("[codex] <defunct>"))
	assert.Empty(t, workerProviderForCommand("git commit -m msg"))
}
