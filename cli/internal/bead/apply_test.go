package bead

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingOperationBackend struct {
	applyCalled bool
	applyID     string
	applyOp     Operation
	applyCtx    context.Context
}

func (b *recordingOperationBackend) Init() error { return nil }

func (b *recordingOperationBackend) ReadAll() ([]Bead, error) {
	panic("ReadAll should not be called when Apply delegates to OperationApplier")
}

func (b *recordingOperationBackend) WriteAll([]Bead) error {
	panic("WriteAll should not be called when Apply delegates to OperationApplier")
}

func (b *recordingOperationBackend) WithLock(func() error) error {
	panic("WithLock should not be called when Apply delegates to OperationApplier")
}

func (b *recordingOperationBackend) Apply(ctx context.Context, id string, op Operation) error {
	b.applyCalled = true
	b.applyID = id
	b.applyOp = op
	b.applyCtx = ctx
	return nil
}

func TestStore_Apply_DelegatesToOperationApplier_WhenAvailable(t *testing.T) {
	t.Parallel()

	backend := &recordingOperationBackend{}
	s := &Store{backend: backend}
	op := MutateFunc(func(b *Bead) error {
		b.Notes = "should not run in this test"
		return nil
	})

	require.NoError(t, s.Apply("ddx-apply-01", op))
	require.True(t, backend.applyCalled, "Store.Apply must delegate to the backend fast path")
	require.Equal(t, "ddx-apply-01", backend.applyID)
	require.NotNil(t, backend.applyOp)
	require.Equal(t, fmt.Sprintf("%T", op), fmt.Sprintf("%T", backend.applyOp))
	require.NotNil(t, backend.applyCtx)
}

func TestStore_Apply_GenericFallback_WhenNotAvailable(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	b := &Bead{Title: "apply fallback"}
	require.NoError(t, s.Create(b))

	require.NoError(t, s.Apply(b.ID, MutateFunc(func(b *Bead) error {
		b.Notes = "applied through the generic fallback"
		b.Description = "mutated via Store.Apply"
		return nil
	})))

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, "apply fallback", got.Title)
	assert.Equal(t, "applied through the generic fallback", got.Notes)
	assert.Equal(t, "mutated via Store.Apply", got.Description)
	assert.Equal(t, b.ID, got.ID)
	assert.False(t, got.UpdatedAt.IsZero())
}

func TestJSONLBackend_Apply_FallsThroughForUnknownOps(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "beads.jsonl")
	lockDir := filepath.Join(dir, ".lock")
	backend := NewJSONLBackend(dir, file, lockDir, time.Second)
	require.NoError(t, backend.Init())

	original := Bead{
		ID:        "ddx-jsonl-apply",
		Title:     "jsonl apply",
		Status:    StatusOpen,
		Priority:  2,
		IssueType: DefaultType,
		CreatedAt: time.Unix(10, 0).UTC(),
		UpdatedAt: time.Unix(10, 0).UTC(),
		Labels:    []string{"apply-test"},
	}
	require.NoError(t, backend.WriteAll([]Bead{original}))

	require.NoError(t, backend.Apply(context.Background(), original.ID, MutateFunc(func(b *Bead) error {
		b.Notes = "updated through JSONLBackend.Apply"
		return nil
	})))

	got, err := backend.ReadAll()
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, original.ID, got[0].ID)
	assert.Equal(t, "updated through JSONLBackend.Apply", got[0].Notes)
	assert.True(t, got[0].UpdatedAt.After(original.UpdatedAt) || got[0].UpdatedAt.Equal(original.UpdatedAt))
}

func TestBeadDataModel_InvariantsHold(t *testing.T) {
	forEachBackendConformanceCase(t, func(t *testing.T, tc backendConformanceCase) {
		s := newBackendConformanceStore(t, tc.backend)

		dep := &Bead{Title: fmt.Sprintf("%s-dependency", tc.name), Priority: 2}
		require.NoError(t, s.Create(dep))

		target := &Bead{
			Title:    fmt.Sprintf("%s-target", tc.name),
			Priority: 1,
			Labels:   []string{"area:bead", "kind:feature"},
		}
		target.AddDep(dep.ID, "blocks")
		require.NoError(t, s.Create(target))

		before, err := s.Get(target.ID)
		require.NoError(t, err)
		beforeEvents, err := s.Events(target.ID)
		require.NoError(t, err)
		assertBeadSnapshotInvariants(t, *before)
		assert.Empty(t, beforeEvents)

		require.NoError(t, s.Apply(target.ID, MutateFunc(func(b *Bead) error {
			b.Notes = "mutated through Apply"
			return nil
		})))
		afterApply, err := s.Get(target.ID)
		require.NoError(t, err)
		afterApplyEvents, err := s.Events(target.ID)
		require.NoError(t, err)
		assertBeadMutationInvariants(t, *before, *afterApply, beforeEvents, afterApplyEvents)

		require.NoError(t, s.Claim(target.ID, "worker-1"))
		afterClaim, err := s.Get(target.ID)
		require.NoError(t, err)
		afterClaimEvents, err := s.Events(target.ID)
		require.NoError(t, err)
		assertBeadMutationInvariants(t, *afterApply, *afterClaim, afterApplyEvents, afterClaimEvents)
		assert.Equal(t, StatusInProgress, afterClaim.Status)
		assert.NotEmpty(t, afterClaim.Owner)
		assert.NotEmpty(t, afterClaim.Extra[ClaimHeartbeatExtraKey])
		var cancelRequested bool
		cancelRequested, err = s.IsCancelRequested(target.ID)
		require.NoError(t, err)
		assert.False(t, cancelRequested)

		require.NoError(t, s.AppendEvent(target.ID, BeadEvent{
			Kind:      "test-event",
			Summary:   "first event",
			Body:      "body-1",
			Actor:     "tester",
			CreatedAt: time.Unix(20, 0).UTC(),
		}))
		afterEvent, err := s.Get(target.ID)
		require.NoError(t, err)
		afterEventEvents, err := s.Events(target.ID)
		require.NoError(t, err)
		assertBeadMutationInvariants(t, *afterClaim, *afterEvent, afterClaimEvents, afterEventEvents)
		require.Len(t, afterEventEvents, 1)
		assert.Equal(t, "test-event", afterEventEvents[0].Kind)
		assert.Equal(t, "first event", afterEventEvents[0].Summary)

		var requested bool
		requested, err = s.RequestCancel(target.ID)
		require.NoError(t, err)
		assert.True(t, requested)
		afterRequested, err := s.Get(target.ID)
		require.NoError(t, err)
		afterRequestedEvents, err := s.Events(target.ID)
		require.NoError(t, err)
		assertBeadMutationInvariants(t, *afterEvent, *afterRequested, afterEventEvents, afterRequestedEvents)
		assert.True(t, isExtraTrue(afterRequested.Extra[ExtraCancelRequested]))
		assert.False(t, isExtraTrue(afterRequested.Extra[ExtraCancelHonored]))
		cancelRequested, err = s.IsCancelRequested(target.ID)
		require.NoError(t, err)
		assert.True(t, cancelRequested)

		require.NoError(t, s.MarkCancelHonored(target.ID))
		afterHonored, err := s.Get(target.ID)
		require.NoError(t, err)
		afterHonoredEvents, err := s.Events(target.ID)
		require.NoError(t, err)
		assertBeadMutationInvariants(t, *afterRequested, *afterHonored, afterRequestedEvents, afterHonoredEvents)
		assert.True(t, isExtraTrue(afterHonored.Extra[ExtraCancelRequested]))
		assert.True(t, isExtraTrue(afterHonored.Extra[ExtraCancelHonored]))
		cancelRequested, err = s.IsCancelRequested(target.ID)
		require.NoError(t, err)
		assert.False(t, cancelRequested)

		require.NoError(t, s.Unclaim(target.ID))
		afterUnclaim, err := s.Get(target.ID)
		require.NoError(t, err)
		afterUnclaimEvents, err := s.Events(target.ID)
		require.NoError(t, err)
		assertBeadMutationInvariants(t, *afterHonored, *afterUnclaim, afterHonoredEvents, afterUnclaimEvents)
		assert.Empty(t, afterUnclaim.Owner)
		for _, key := range ClaimMetadataExtraKeys {
			assert.Empty(t, afterUnclaim.Extra[key], "claim key %q must be cleared on Unclaim", key)
		}
		assert.Empty(t, afterUnclaim.Extra[ClaimHeartbeatExtraKey])

		require.NoError(t, s.SetExecutionCooldown(target.ID, time.Now().UTC().Add(2*time.Hour), "no_changes", "retry later", ""))
		afterCooldown, err := s.Get(target.ID)
		require.NoError(t, err)
		afterCooldownEvents, err := s.Events(target.ID)
		require.NoError(t, err)
		assertBeadMutationInvariants(t, *afterUnclaim, *afterCooldown, afterUnclaimEvents, afterCooldownEvents)
		rawRetryAfter, ok := afterCooldown.Extra[ExtraRetryAfter]
		require.True(t, ok, "cooldown retry-after marker must be present")
		retryAfter, ok := rawRetryAfter.(string)
		require.True(t, ok, "cooldown retry-after marker must be a string")
		_, err = time.Parse(time.RFC3339, retryAfter)
		require.NoError(t, err)
	})
}

func assertBeadSnapshotInvariants(t *testing.T, b Bead) {
	t.Helper()

	require.NoError(t, ValidateID(b.ID))
	assert.NotEmpty(t, b.Title, "title must remain non-empty")
	assert.NotEmpty(t, b.IssueType, "issue_type must remain set")
	assert.Contains(t, allLifecycleStatusesForTests(), b.Status, "status must stay canonical")
	assert.GreaterOrEqual(t, b.Priority, MinPriority, "priority must stay within range")
	assert.LessOrEqual(t, b.Priority, MaxPriority, "priority must stay within range")
	assert.False(t, b.CreatedAt.IsZero(), "created_at must be populated")
	assert.False(t, b.UpdatedAt.IsZero(), "updated_at must be populated")
	assert.False(t, b.UpdatedAt.Before(b.CreatedAt), "updated_at must not precede created_at")
	assert.True(t, uniqueStrings(b.Labels), "labels must remain unique")
	assert.True(t, dependenciesAreValid(b), "dependencies must remain valid and unique")
	assertClaimInvariant(t, b)
	assertCancelInvariant(t, b)
	assertEventInvariant(t, b)
	assertCooldownInvariant(t, b)
}

func assertBeadMutationInvariants(t *testing.T, before, after Bead, beforeEvents, afterEvents []BeadEvent) {
	t.Helper()

	assert.Equal(t, before.ID, after.ID, "ID must remain stable")
	assert.True(t, after.CreatedAt.Equal(before.CreatedAt), "created_at must remain stable")
	assert.False(t, after.UpdatedAt.Before(before.UpdatedAt), "updated_at must be monotonic")
	require.GreaterOrEqual(t, len(afterEvents), len(beforeEvents), "events must be append-only")
	assert.Equal(t, beforeEvents, afterEvents[:len(beforeEvents)], "events must be append-only")
	assertBeadSnapshotInvariants(t, after)
}

func uniqueStrings(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func dependenciesAreValid(b Bead) bool {
	seen := make(map[string]struct{}, len(b.Dependencies))
	for _, depID := range b.DepIDs() {
		if err := ValidateID(depID); err != nil {
			return false
		}
		if depID == b.ID {
			return false
		}
		if _, ok := seen[depID]; ok {
			return false
		}
		seen[depID] = struct{}{}
	}
	return true
}

func assertClaimInvariant(t *testing.T, b Bead) {
	t.Helper()

	owner := strings.TrimSpace(b.Owner)
	if owner == "" {
		for _, key := range ClaimMetadataExtraKeys {
			assert.Empty(t, b.Extra[key], "claim key %q must be absent when the bead is unclaimed", key)
		}
		assert.Empty(t, b.Extra[ClaimHeartbeatExtraKey], "heartbeat must be absent when the bead is unclaimed")
		return
	}

	assert.Equal(t, StatusInProgress, b.Status, "claimed beads must be in_progress")
	assert.NotEmpty(t, b.Extra["claimed-at"], "claimed-at must be recorded when a bead is claimed")
	assert.NotEmpty(t, b.Extra["claimed-pid"], "claimed-pid must be recorded when a bead is claimed")
	if _, ok := b.Extra["claimed-machine"]; ok {
		assert.NotEmpty(t, b.Extra["claimed-machine"], "claimed-machine must be recorded when present")
	}
	assert.NotEmpty(t, b.Extra[ClaimHeartbeatExtraKey], "heartbeat must exist while claimed")
}

func assertCancelInvariant(t *testing.T, b Bead) {
	t.Helper()

	requested := isExtraTrue(b.Extra[ExtraCancelRequested])
	honored := isExtraTrue(b.Extra[ExtraCancelHonored])
	if honored {
		assert.True(t, requested, "cancel-honored implies cancel-requested")
	}
	if !requested {
		assert.False(t, honored, "cancel-honored cannot be set without cancel-requested")
	}
}

func assertEventInvariant(t *testing.T, b Bead) {
	t.Helper()

	events := decodeBeadEvents(b.Extra["events"])
	for i := 1; i < len(events); i++ {
		assert.False(t, events[i].CreatedAt.Before(events[i-1].CreatedAt), "events must be chronological")
	}
}

func assertCooldownInvariant(t *testing.T, b Bead) {
	t.Helper()

	raw, ok := b.Extra[ExtraRetryAfter]
	if !ok {
		return
	}
	retryAfter, ok := raw.(string)
	require.True(t, ok, "retry-after must be a string")
	if retryAfter == "" {
		return
	}
	_, err := time.Parse(time.RFC3339, retryAfter)
	require.NoError(t, err)
}
