package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeOrphanHarnessScanner struct {
	processes []orphanHarnessProcess
}

func (f fakeOrphanHarnessScanner) Scan(context.Context) ([]orphanHarnessProcess, error) {
	return append([]orphanHarnessProcess(nil), f.processes...), nil
}

type fakeOrphanHarnessLeaseStore struct {
	leases     map[string]bead.ClaimLeaseRecord
	released   []string
	events     map[string][]bead.BeadEvent
	releaseErr error
	appendErr  error
	claimErr   error
}

func (s *fakeOrphanHarnessLeaseStore) ClaimLease(id string) (bead.ClaimLeaseRecord, bool, error) {
	if s.claimErr != nil {
		return bead.ClaimLeaseRecord{}, false, s.claimErr
	}
	rec, ok := s.leases[id]
	return rec, ok, nil
}

func (s *fakeOrphanHarnessLeaseStore) Release(id, _, _ string) error {
	if s.releaseErr != nil {
		return s.releaseErr
	}
	s.released = append(s.released, id)
	return nil
}

func (s *fakeOrphanHarnessLeaseStore) AppendEvent(id string, event bead.BeadEvent) error {
	if s.appendErr != nil {
		return s.appendErr
	}
	if s.events == nil {
		s.events = make(map[string][]bead.BeadEvent)
	}
	s.events[id] = append(s.events[id], event)
	return nil
}

func TestWorkStartup_ReapsOrphanedHarnessChildren(t *testing.T) {
	projectRoot := t.TempDir()
	tempRoot := filepath.Join(t.TempDir(), "exec-wt")
	require.NoError(t, os.MkdirAll(ddxroot.JoinProject(projectRoot), 0o755))
	t.Setenv(config.ExecutionWorktreeRootEnv, tempRoot)

	orphanBeadID := "ddx-deadbeef"
	orphanAttemptID := "20260527T090438-deadbeef"
	orphanWorktree := filepath.Join(tempRoot, ExecuteBeadWtPrefix+orphanBeadID+"-"+orphanAttemptID)
	require.NoError(t, os.MkdirAll(orphanWorktree, 0o755))

	deadOwnerPID := deadPID(t)
	store := &fakeOrphanHarnessLeaseStore{
		leases: map[string]bead.ClaimLeaseRecord{
			orphanBeadID: {
				BeadID: orphanBeadID,
				PID:    deadOwnerPID,
			},
		},
	}
	var killed []int
	scanner := fakeOrphanHarnessScanner{
		processes: []orphanHarnessProcess{
			{
				PID:     4242,
				PPID:    1,
				Command: "claude --print -p --verbose --output-format stream-json " + filepath.Base(orphanWorktree),
				Cwd:     orphanWorktree,
			},
			{
				PID:     4243,
				PPID:    1,
				Command: "bash -lc echo unrelated",
				Cwd:     tempRoot,
			},
		},
	}

	reaped, err := reapOrphanedHarnessChildren(
		context.Background(),
		projectRoot,
		scanner,
		store,
		store,
		store,
		"worker-a",
		&bytes.Buffer{},
		nil,
		func(pid int) error {
			killed = append(killed, pid)
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, 1, reaped)
	require.Equal(t, []int{4242}, killed)
	require.Equal(t, []string{orphanBeadID}, store.released)

	events := store.events[orphanBeadID]
	require.Len(t, events, 1)
	assert.Equal(t, "operator_attention", events[0].Kind)
	assert.Equal(t, "orphaned_harness_child", events[0].Summary)

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(events[0].Body), &body))
	assert.Equal(t, "orphaned_harness_child", body["reason"])
	assert.Equal(t, orphanBeadID, body["bead_id"])
	assert.Equal(t, float64(4242), body["process_pid"])
	assert.Equal(t, float64(1), body["process_parent_pid"])
}

func TestWorkStartup_DoesNotReapLiveOwnedChildren(t *testing.T) {
	projectRoot := t.TempDir()
	tempRoot := filepath.Join(t.TempDir(), "exec-wt")
	require.NoError(t, os.MkdirAll(ddxroot.JoinProject(projectRoot), 0o755))
	t.Setenv(config.ExecutionWorktreeRootEnv, tempRoot)

	orphanBeadID := "ddx-deadbeef"
	liveBeadID := "ddx-feedface"
	orphanWorktree := filepath.Join(tempRoot, ExecuteBeadWtPrefix+orphanBeadID+"-20260527T090438-deadbeef")
	liveWorktree := filepath.Join(tempRoot, ExecuteBeadWtPrefix+liveBeadID+"-20260527T090438-feedface")
	require.NoError(t, os.MkdirAll(orphanWorktree, 0o755))
	require.NoError(t, os.MkdirAll(liveWorktree, 0o755))

	deadOwnerPID := deadPID(t)
	store := &fakeOrphanHarnessLeaseStore{
		leases: map[string]bead.ClaimLeaseRecord{
			orphanBeadID: {
				BeadID: orphanBeadID,
				PID:    deadOwnerPID,
			},
			liveBeadID: {
				BeadID: liveBeadID,
				PID:    os.Getpid(),
			},
		},
	}
	var killed []int
	scanner := fakeOrphanHarnessScanner{
		processes: []orphanHarnessProcess{
			{
				PID:     5151,
				PPID:    1,
				Command: "codex exec --json -C " + orphanWorktree,
				Cwd:     tempRoot,
			},
			{
				PID:     5252,
				PPID:    4321,
				Command: "claude --print -p --verbose --output-format stream-json",
				Cwd:     liveWorktree,
			},
			{
				PID:     5353,
				PPID:    1,
				Command: "bash -lc echo unrelated",
				Cwd:     tempRoot,
			},
		},
	}

	reaped, err := reapOrphanedHarnessChildren(
		context.Background(),
		projectRoot,
		scanner,
		store,
		store,
		store,
		"worker-a",
		&bytes.Buffer{},
		nil,
		func(pid int) error {
			killed = append(killed, pid)
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, 1, reaped)
	require.Equal(t, []int{5151}, killed)
	require.Equal(t, []string{orphanBeadID}, store.released)
	assert.Empty(t, store.events[liveBeadID], "live-owned child must not be reaped")
}

func TestWorkStartup_ReaperDoesNotKillOtherWorkspaceHarness(t *testing.T) {
	projectRoot := t.TempDir()
	otherProjectRoot := t.TempDir()
	tempRoot := filepath.Join(t.TempDir(), "exec-wt")
	otherTempRoot := filepath.Join(t.TempDir(), "other-exec-wt")
	require.NoError(t, os.MkdirAll(ddxroot.JoinProject(projectRoot), 0o755))
	require.NoError(t, os.MkdirAll(ddxroot.JoinProject(otherProjectRoot), 0o755))
	require.NoError(t, os.MkdirAll(tempRoot, 0o755))
	require.NoError(t, os.MkdirAll(otherTempRoot, 0o755))

	projectBeadID := "ddx-11111111"
	otherProjectBeadID := "ddx-22222222"
	projectWorktree := filepath.Join(tempRoot, ExecuteBeadWtPrefix+projectBeadID+"-20260527T090438-aaaa")
	otherProjectWorktree := filepath.Join(otherTempRoot, ExecuteBeadWtPrefix+otherProjectBeadID+"-20260527T090438-bbbb")
	require.NoError(t, os.MkdirAll(projectWorktree, 0o755))
	require.NoError(t, os.MkdirAll(otherProjectWorktree, 0o755))

	// Set ExecutionTempRoot to our test's tempRoot for this project
	t.Setenv(config.ExecutionWorktreeRootEnv, tempRoot)

	deadOwnerPID := deadPID(t)
	store := &fakeOrphanHarnessLeaseStore{
		leases: map[string]bead.ClaimLeaseRecord{
			projectBeadID: {
				BeadID: projectBeadID,
				PID:    deadOwnerPID,
			},
			otherProjectBeadID: {
				BeadID: otherProjectBeadID,
				PID:    deadOwnerPID,
			},
		},
	}
	var killed []int
	// Both harnesses appear orphaned (PPID=1), but only the one in the current
	// project's execRoot should be reaped.
	scanner := fakeOrphanHarnessScanner{
		processes: []orphanHarnessProcess{
			{
				PID:     6161,
				PPID:    1,
				Command: "claude exec --json -C " + projectWorktree,
				Cwd:     projectWorktree,
			},
			{
				PID:     6262,
				PPID:    1,
				Command: "codex exec --json -C " + otherProjectWorktree,
				Cwd:     otherProjectWorktree,
			},
		},
	}

	// Only pass the projectRoot, not otherProjectRoot
	reaped, err := reapOrphanedHarnessChildren(
		context.Background(),
		projectRoot,
		scanner,
		store,
		store,
		store,
		"worker-a",
		&bytes.Buffer{},
		nil,
		func(pid int) error {
			killed = append(killed, pid)
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, 1, reaped, "only harness within project's execRoot should be reaped")
	require.Equal(t, []int{6161}, killed)
	require.Equal(t, []string{projectBeadID}, store.released)
	assert.Empty(t, store.events[otherProjectBeadID], "other-workspace harness must not be reaped")
}

func deadPID(t *testing.T) int {
	t.Helper()

	cmd := exec.Command("sh", "-c", "exit 0")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	require.NoError(t, cmd.Wait())
	return pid
}
