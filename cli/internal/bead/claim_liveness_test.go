package bead

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeClaimLeaseForTest(t *testing.T, s *Store, rec ClaimLeaseRecord) {
	t.Helper()
	require.NoError(t, s.writeClaimHeartbeat(rec))
}

func deadPIDForTest(t *testing.T) int {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("processAlive is conservative on windows")
	}

	start := os.Getpid() + 1
	for pid := start; pid < start+10000; pid++ {
		if !processAlive(pid) {
			return pid
		}
	}

	t.Fatal("unable to find a dead PID for the test")
	return 0
}

func TestClaimLeaseIsStale_AliveOwnerSameMachineNotStaleDespiteAge(t *testing.T) {
	s := newTestStore(t)
	beadID := "ddx-claim-alive-same-machine"
	now := time.Now().UTC()
	staleAt := now.Add(-1 * time.Hour)
	machine := currentMachineID()

	require.NoError(t, s.Create(testCtx(), &Bead{ID: beadID, Title: "Alive owner same machine"}))
	require.NoError(t, s.Claim(beadID, "worker-a"))
	writeClaimLeaseForTest(t, s, ClaimLeaseRecord{
		BeadID:    beadID,
		Owner:     "worker-a",
		Machine:   machine,
		StartedAt: staleAt,
		UpdatedAt: staleAt,
		PID:       os.Getpid(),
	})

	assert.False(t, claimLeaseIsStale(s, nil, beadID))

	err := s.Claim(beadID, "worker-b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot claim")

	err = s.ClaimWithOptions(beadID, "worker-b", "sess-alive", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot claim")
}

func TestClaimLeaseIsStale_DeadOwnerPIDReclaimable(t *testing.T) {
	s := newTestStore(t)
	deadPID := deadPIDForTest(t)
	staleAt := time.Now().UTC().Add(-1 * time.Hour)
	machine := currentMachineID()

	beadID := "ddx-claim-dead-same-machine"
	require.NoError(t, s.Create(testCtx(), &Bead{ID: beadID, Title: "Dead owner same machine"}))
	require.NoError(t, s.Claim(beadID, "worker-a"))
	writeClaimLeaseForTest(t, s, ClaimLeaseRecord{
		BeadID:    beadID,
		Owner:     "worker-a",
		Machine:   machine,
		StartedAt: staleAt,
		UpdatedAt: staleAt,
		PID:       deadPID,
	})

	assert.True(t, claimLeaseIsStale(s, nil, beadID))

	require.NoError(t, s.Claim(beadID, "worker-b"))
	got, err := s.Get(testCtx(), beadID)
	require.NoError(t, err)
	assert.Equal(t, "worker-b", got.Owner)

	beadID = "ddx-claim-dead-same-machine-opts"
	require.NoError(t, s.Create(testCtx(), &Bead{ID: beadID, Title: "Dead owner same machine options"}))
	require.NoError(t, s.Claim(beadID, "worker-a"))
	writeClaimLeaseForTest(t, s, ClaimLeaseRecord{
		BeadID:    beadID,
		Owner:     "worker-a",
		Machine:   machine,
		StartedAt: staleAt,
		UpdatedAt: staleAt,
		PID:       deadPID,
	})

	require.NoError(t, s.ClaimWithOptions(beadID, "worker-b", "sess-dead", ""))
	lease, found, err := s.ClaimLease(beadID)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "worker-b", lease.Owner)
}

func TestClaimLeaseIsStale_ForeignMachineFallsBackToAge(t *testing.T) {
	s := newTestStore(t)
	staleAt := time.Now().UTC().Add(-1 * time.Hour)
	foreignMachine := currentMachineID() + "-foreign"

	beadID := "ddx-claim-foreign-machine"
	require.NoError(t, s.Create(testCtx(), &Bead{ID: beadID, Title: "Foreign machine claim"}))
	require.NoError(t, s.Claim(beadID, "worker-a"))
	writeClaimLeaseForTest(t, s, ClaimLeaseRecord{
		BeadID:    beadID,
		Owner:     "worker-a",
		Machine:   foreignMachine,
		StartedAt: staleAt,
		UpdatedAt: staleAt,
		PID:       os.Getpid(),
	})

	assert.True(t, claimLeaseIsStale(s, nil, beadID))

	require.NoError(t, s.Claim(beadID, "worker-b"))
	got, err := s.Get(testCtx(), beadID)
	require.NoError(t, err)
	assert.Equal(t, "worker-b", got.Owner)

	beadID = "ddx-claim-foreign-machine-opts"
	require.NoError(t, s.Create(testCtx(), &Bead{ID: beadID, Title: "Foreign machine claim options"}))
	require.NoError(t, s.Claim(beadID, "worker-a"))
	writeClaimLeaseForTest(t, s, ClaimLeaseRecord{
		BeadID:    beadID,
		Owner:     "worker-a",
		Machine:   foreignMachine,
		StartedAt: staleAt,
		UpdatedAt: staleAt,
		PID:       os.Getpid(),
	})

	require.NoError(t, s.ClaimWithOptions(beadID, "worker-b", "sess-foreign", ""))
	lease, found, err := s.ClaimLease(beadID)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "worker-b", lease.Owner)
}
