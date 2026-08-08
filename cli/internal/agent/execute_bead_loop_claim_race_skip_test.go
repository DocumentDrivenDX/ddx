package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type claimRaceFirstStore struct {
	*bead.Store
	firstFailBead string
	claimCalls    atomic.Int32
	claimedIDs    []string
}

func (s *claimRaceFirstStore) ClaimWithOptions(id, assignee, sessionID, attemptID string) error {
	n := s.claimCalls.Add(1)
	if n == 1 && id == s.firstFailBead {
		return bead.ErrAlreadyClaimed
	}
	s.claimedIDs = append(s.claimedIDs, id)
	return s.Store.ClaimWithOptions(id, assignee, sessionID, attemptID)
}

func TestWorkLoop_ClaimRaceSkipsLoserAndClaimsNextReady(t *testing.T) {
	inner := bead.NewStore(t.TempDir())
	require.NoError(t, inner.Init(context.Background()))
	first := &bead.Bead{ID: "ddx-claim-race-first", Title: "first", Priority: 0}
	second := &bead.Bead{ID: "ddx-claim-race-second", Title: "second", Priority: 1}
	require.NoError(t, inner.Create(context.Background(), first))
	require.NoError(t, inner.Create(context.Background(), second))

	store := &claimRaceFirstStore{Store: inner, firstFailBead: first.ID}
	var executed []string
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, SessionID: "s", ResultRev: "rev"}, nil
		}),
	}

	var events bytes.Buffer
	opts := config.TestLoopConfigOpts{Assignee: "worker-claim-race"}
	rcfg := config.NewTestConfigForLoop(opts).Resolve(config.TestLoopOverrides(opts))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _ = worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Once:      false,
		EventSink: &events,
		SessionID: "claim-race-skip",
	})

	byType := map[string]int{}
	for _, line := range bytes.Split(events.Bytes(), []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var ev struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		byType[ev.Type]++
	}
	assert.GreaterOrEqual(t, byType["picker.claim_race"], 1)
	require.NotEmpty(t, store.claimedIDs)
	assert.Equal(t, second.ID, store.claimedIDs[0])
	assert.Contains(t, executed, second.ID)
}
