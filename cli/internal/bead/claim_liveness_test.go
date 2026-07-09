package bead

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
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

func TestClaimLivenessRootUsesConfiguredRuntimeRoot(t *testing.T) {
	runtimeRoot := t.TempDir()
	t.Setenv(config.ExecutionWorktreeRootEnv, runtimeRoot)

	ddxDir := filepath.Join(t.TempDir(), ddxroot.DirName)
	got := ClaimLivenessRoot(ddxDir)

	rawTempFallbackPrefix := filepath.Clean(os.TempDir()) + string(filepath.Separator) + claimLivenessNamespace + string(filepath.Separator)
	assert.True(t, strings.HasPrefix(got, filepath.Clean(runtimeRoot)+string(filepath.Separator)),
		"expected claim liveness root under configured runtime root %q, got %q", runtimeRoot, got)
	assert.False(t, strings.HasPrefix(got, rawTempFallbackPrefix),
		"claim liveness root should avoid raw os.TempDir() when a runtime root is configured, got %q", got)
}

func TestClaimLivenessRootPreservesStableProjectScoping(t *testing.T) {
	runtimeRoot := t.TempDir()
	t.Setenv(config.ExecutionWorktreeRootEnv, runtimeRoot)

	ddxDirA := filepath.Join(t.TempDir(), ddxroot.DirName)
	ddxDirB := filepath.Join(t.TempDir(), ddxroot.DirName)

	gotA1 := ClaimLivenessRoot(ddxDirA)
	gotA2 := ClaimLivenessRoot(ddxDirA)
	gotB := ClaimLivenessRoot(ddxDirB)

	assert.Equal(t, gotA1, gotA2, "two workers for the same project must resolve the same claim-liveness root")
	assert.NotEqual(t, gotA1, gotB, "distinct project roots must resolve distinct scoped claim-liveness roots")

	sharedPrefix := filepath.Clean(runtimeRoot) + string(filepath.Separator)
	assert.True(t, strings.HasPrefix(gotA1, sharedPrefix))
	assert.True(t, strings.HasPrefix(gotB, sharedPrefix))
}

func TestClaimLivenessRootFallsBackToOSTempDirWithoutRuntimeRoot(t *testing.T) {
	t.Setenv(config.ExecutionWorktreeRootEnv, "")
	t.Setenv("HOME", t.TempDir())

	ddxDir := filepath.Join(t.TempDir(), ddxroot.DirName)
	got := ClaimLivenessRoot(ddxDir)

	root := canonicalClaimRoot(ddxDir)
	sum := sha1.Sum([]byte(root))
	want := filepath.Join(os.TempDir(), claimLivenessNamespace, hex.EncodeToString(sum[:]))

	assert.Equal(t, want, got, "without a configured runtime root, claim liveness must keep using os.TempDir()")
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
