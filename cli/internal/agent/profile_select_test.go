package agent

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectCheapestProfile_LowestBandWithAvailableModel(t *testing.T) {
	snap := ProfileSnapshot{
		Profiles: []agentlib.PolicyInfo{
			{Name: "standard", MinPower: 7, MaxPower: 8},
			{Name: "cheap", MinPower: 5, MaxPower: 5},
			{Name: "too-low", MinPower: 1, MaxPower: 2},
		},
		Models: []agentlib.ModelInfo{
			{ID: "standard-model", Power: 7, Available: true, AutoRoutable: true},
			{ID: "cheap-model", Power: 5, Available: true, AutoRoutable: true},
		},
	}

	assert.Equal(t, "cheap", SelectCheapestProfile(snap))
}

func TestSelectCheapestProfile_UsesPolicyMetadataWhenModelSnapshotEmpty(t *testing.T) {
	snap := ProfileSnapshot{
		Profiles: []agentlib.PolicyInfo{
			{Name: "cheap", MinPower: 5, MaxPower: 5},
			{Name: "default", MinPower: 7, MaxPower: 8},
			{Name: "smart", MinPower: 9, MaxPower: 10},
		},
	}

	assert.Equal(t, "cheap", SelectCheapestProfile(snap))
	assert.Equal(t, "default", SelectImplementationProfile(snap, escalation.TierStandard).Name)
}

func TestSelectCheapestProfile_TieDoesNotPreferLocalPolicy(t *testing.T) {
	snap := ProfileSnapshot{
		Profiles: []agentlib.PolicyInfo{
			{Name: "z-local", MinPower: 5, MaxPower: 5, AllowLocal: true},
			{Name: "a-remote", MinPower: 5, MaxPower: 5},
		},
		Models: []agentlib.ModelInfo{
			{ID: "candidate", Power: 5, Available: true, AutoRoutable: true},
		},
	}

	assert.Equal(t, "a-remote", SelectCheapestProfile(snap))
}

func TestSelectStrongestProfile_HighestBandWithAvailableModel(t *testing.T) {
	snap := ProfileSnapshot{
		Profiles: []agentlib.PolicyInfo{
			{Name: "cheap", MinPower: 5, MaxPower: 5},
			{Name: "standard", MinPower: 7, MaxPower: 8},
			{Name: "smart", MinPower: 9, MaxPower: 10},
		},
		Models: []agentlib.ModelInfo{
			{ID: "cheap-model", Power: 5, Available: true, AutoRoutable: true},
			{ID: "standard-model", Power: 7, Available: true, AutoRoutable: true},
			{ID: "smart-model", Power: 10, Available: true, AutoRoutable: true},
		},
	}

	assert.Equal(t, "smart", SelectStrongestProfile(snap))
}

func TestSelectStrongestProfileAbove_RespectsFloor(t *testing.T) {
	snap := ProfileSnapshot{
		Profiles: []agentlib.PolicyInfo{
			{Name: "standard", MinPower: 7, MaxPower: 8},
			{Name: "smart", MinPower: 9, MaxPower: 10},
		},
		Models: []agentlib.ModelInfo{
			{ID: "standard-model", Power: 8, Available: true, AutoRoutable: true},
			{ID: "smart-model", Power: 9, Available: true, AutoRoutable: true},
		},
	}

	assert.Equal(t, "smart", SelectStrongestProfileAbove(snap, 9))
	assert.Empty(t, SelectStrongestProfileAbove(snap, 11))
}

func TestSelectProfile_ReturnsEmptyWhenNothingSatisfies(t *testing.T) {
	snap := ProfileSnapshot{
		Profiles: []agentlib.PolicyInfo{
			{Name: "cheap", MinPower: 5, MaxPower: 5},
			{Name: "smart", MinPower: 9, MaxPower: 10},
		},
		Models: []agentlib.ModelInfo{
			{ID: "offline", Power: 10, Available: false, AutoRoutable: true},
			{ID: "exact-only", Power: 10, Available: true, AutoRoutable: true, ExactPinOnly: true},
		},
	}

	assert.Empty(t, SelectCheapestProfile(snap))
	assert.Empty(t, SelectStrongestProfile(snap))
	assert.Empty(t, SelectStrongestProfileAbove(snap, 1))
}

func TestSelectImplementationProfile_StandardUsesOpaqueMediumBand(t *testing.T) {
	snap := ProfileSnapshot{
		Profiles: []agentlib.PolicyInfo{
			{Name: "p-max", MinPower: 90, MaxPower: 100},
			{Name: "p-low", MinPower: 10, MaxPower: 20},
			{Name: "p-balanced", MinPower: 50, MaxPower: 70},
		},
		Models: []agentlib.ModelInfo{
			{ID: "low", Power: 10, Available: true, AutoRoutable: true},
			{ID: "balanced", Power: 60, Available: true, AutoRoutable: true},
			{ID: "max", Power: 95, Available: true, AutoRoutable: true},
		},
	}

	got := SelectImplementationProfile(snap, escalation.TierStandard)

	assert.Equal(t, "p-balanced", got.Name)
	assert.Equal(t, 50, got.MinPower)
	assert.False(t, got.Degraded)
}

func TestSelectImplementationProfile_CanonicalFizeauPolicies(t *testing.T) {
	snap := canonicalFizeauPolicySnapshot()

	assert.Equal(t, "cheap", SelectImplementationProfile(snap, escalation.TierCheap).Name)
	assert.Equal(t, "default", SelectImplementationProfile(snap, escalation.TierStandard).Name)
	assert.Equal(t, "smart", SelectImplementationProfile(snap, escalation.TierSmart).Name)
}

func TestSelectImplementationProfile_DoesNotHardcodeTierNames(t *testing.T) {
	snap := ProfileSnapshot{
		Profiles: []agentlib.PolicyInfo{
			{Name: "alpha", MinPower: 1, MaxPower: 2},
			{Name: "bravo", MinPower: 3, MaxPower: 4},
			{Name: "charlie", MinPower: 5, MaxPower: 6},
		},
		Models: []agentlib.ModelInfo{
			{ID: "a", Power: 1, Available: true, AutoRoutable: true},
			{ID: "b", Power: 3, Available: true, AutoRoutable: true},
			{ID: "c", Power: 5, Available: true, AutoRoutable: true},
		},
	}

	assert.Equal(t, "alpha", SelectImplementationProfile(snap, escalation.TierCheap).Name)
	assert.Equal(t, "bravo", SelectImplementationProfile(snap, escalation.TierStandard).Name)
	assert.Equal(t, "charlie", SelectImplementationProfile(snap, escalation.TierSmart).Name)
}

func TestSelectImplementationProfile_DoesNotSelectRequirementProfileForOrdinaryWork(t *testing.T) {
	snap := canonicalFizeauPolicySnapshot()

	got := SelectImplementationProfile(snap, escalation.TierCheap)

	assert.Equal(t, "cheap", got.Name)
}

func TestSelectCheapestProfile_DoesNotSelectRequirementProfile(t *testing.T) {
	snap := canonicalFizeauPolicySnapshot()

	assert.Equal(t, "cheap", SelectCheapestProfile(snap))
}

func TestSelectImplementationProfile_MetadataTieBreaksByCostAndSpeedNotLocalPreference(t *testing.T) {
	snap := ProfileSnapshot{
		Profiles: []agentlib.PolicyInfo{
			{Name: "local", MinPower: 40, MaxPower: 60, AllowLocal: true},
			{Name: "remote", MinPower: 40, MaxPower: 60},
		},
		Models: []agentlib.ModelInfo{
			{ID: "local-candidate", Power: 50, Available: true, AutoRoutable: true, Cost: agentlib.CostInfo{InputPerMTok: 1, OutputPerMTok: 1}, PerfSignal: agentlib.PerfSignal{SpeedTokensPerSec: 50}},
			{ID: "remote-candidate", Power: 50, Available: true, AutoRoutable: true, Cost: agentlib.CostInfo{InputPerMTok: 3, OutputPerMTok: 3}, PerfSignal: agentlib.PerfSignal{SpeedTokensPerSec: 10}},
		},
	}

	got := SelectImplementationProfile(snap, escalation.TierCheap)

	assert.Equal(t, "local", got.Name)
}

func TestSelectImplementationProfile_DegradesToOnlyAvailableProfile(t *testing.T) {
	snap := ProfileSnapshot{
		Profiles: []agentlib.PolicyInfo{
			{Name: "p-low", MinPower: 10, MaxPower: 20},
			{Name: "p-balanced", MinPower: 50, MaxPower: 70},
		},
		Models: []agentlib.ModelInfo{
			{ID: "low", Power: 10, Available: true, AutoRoutable: true},
			{ID: "balanced-offline", Power: 60, Available: false, AutoRoutable: true},
		},
	}

	got := SelectImplementationProfile(snap, escalation.TierStandard)

	assert.Equal(t, "p-low", got.Name)
	assert.True(t, got.Degraded)
	assert.Contains(t, got.Note, "medium profile unavailable")
}

func TestSelectImplementationProfile_StandardFallsBackDownBeforeSmart(t *testing.T) {
	snap := ProfileSnapshot{
		Profiles: []agentlib.PolicyInfo{
			{Name: "p-low", MinPower: 10, MaxPower: 20},
			{Name: "p-balanced", MinPower: 50, MaxPower: 70},
			{Name: "p-max", MinPower: 90, MaxPower: 100},
		},
		Models: []agentlib.ModelInfo{
			{ID: "low", Power: 10, Available: true, AutoRoutable: true},
			{ID: "balanced-offline", Power: 60, Available: false, AutoRoutable: true},
			{ID: "max", Power: 95, Available: true, AutoRoutable: true},
		},
	}

	got := SelectImplementationProfile(snap, escalation.TierStandard)

	assert.Equal(t, "p-low", got.Name)
	assert.True(t, got.Degraded)
	assert.Contains(t, got.Note, "weaker available profile before smart")
}

func TestSelectImplementationProfileForMinPower_MovesOffWeakProfileOnRetry(t *testing.T) {
	snap := ProfileSnapshot{
		Profiles: []agentlib.PolicyInfo{
			{Name: "p-low", MinPower: 10, MaxPower: 20},
			{Name: "p-balanced", MinPower: 50, MaxPower: 70},
			{Name: "p-max", MinPower: 90, MaxPower: 100},
		},
		Models: []agentlib.ModelInfo{
			{ID: "low", Power: 10, Available: true, AutoRoutable: true},
			{ID: "balanced", Power: 60, Available: true, AutoRoutable: true},
			{ID: "max", Power: 95, Available: true, AutoRoutable: true},
		},
	}

	got := SelectImplementationProfileForMinPower(snap, escalation.TierCheap, 50)

	assert.Equal(t, "p-balanced", got.Name)
	assert.Equal(t, 50, got.MinPower)
	assert.False(t, got.Degraded)
	assert.Empty(t, got.Note)
}

func canonicalFizeauPolicySnapshot() ProfileSnapshot {
	return ProfileSnapshot{
		Profiles: []agentlib.PolicyInfo{
			{Name: "cheap", MinPower: 5, MaxPower: 5, AllowLocal: true},
			{Name: "default", MinPower: 7, MaxPower: 8, AllowLocal: true},
			{Name: "smart", MinPower: 9, MaxPower: 10},
			{Name: "air-gapped", MinPower: 5, MaxPower: 5, AllowLocal: true, Require: []string{"no_remote"}},
		},
		Models: []agentlib.ModelInfo{
			{ID: "cheap-model", Power: 5, Available: true, AutoRoutable: true},
			{ID: "default-model", Power: 7, Available: true, AutoRoutable: true},
			{ID: "smart-model", Power: 9, Available: true, AutoRoutable: true},
		},
	}
}

func TestLoadProfileSnapshot_MemoizesAndStaleOnError(t *testing.T) {
	resetProfileSnapshotCacheForTest(t)
	now := time.Unix(1000, 0)
	oldNow := profileSnapshotNow
	profileSnapshotNow = func() time.Time { return now }
	t.Cleanup(func() { profileSnapshotNow = oldNow })

	svc := &profileSnapshotServiceStub{
		profiles: []agentlib.PolicyInfo{{Name: "cheap", MinPower: 5, MaxPower: 5}},
		models:   []agentlib.ModelInfo{{ID: "cheap-model", Power: 5, Available: true, AutoRoutable: true}},
	}

	first, err := LoadProfileSnapshot(context.Background(), svc)
	require.NoError(t, err)
	assert.Equal(t, "cheap", first.Profiles[0].Name)

	second, err := LoadProfileSnapshot(context.Background(), svc)
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.Equal(t, 1, svc.profileCalls)
	assert.Equal(t, 1, svc.modelCalls)

	now = now.Add(profileSnapshotCacheWindow + time.Second)
	svc.err = fmt.Errorf("service unavailable")
	stale, err := LoadProfileSnapshot(context.Background(), svc)
	require.NoError(t, err)
	assert.Equal(t, first, stale)
	assert.Equal(t, 2, svc.profileCalls)
	assert.Equal(t, 1, svc.modelCalls)
}

func TestLoadProfileSnapshot_KeepsPoliciesWhenModelsUnavailable(t *testing.T) {
	resetProfileSnapshotCacheForTest(t)
	svc := &profileSnapshotServiceStub{
		profiles: []agentlib.PolicyInfo{
			{Name: "cheap", MinPower: 5, MaxPower: 5},
			{Name: "smart", MinPower: 9, MaxPower: 10},
		},
		modelErr: fmt.Errorf("model inventory unavailable"),
	}

	snap, err := LoadProfileSnapshot(context.Background(), svc)

	require.NoError(t, err)
	require.Len(t, snap.Profiles, 2)
	assert.Empty(t, snap.Models)
	assert.Equal(t, "smart", SelectStrongestProfile(snap))
	assert.Equal(t, 1, svc.profileCalls)
	assert.Equal(t, 1, svc.modelCalls)
}

func TestSelectProfileForDispatch_ColdProjectCacheLoadsPolicySnapshot(t *testing.T) {
	resetProfileSnapshotCacheForTest(t)
	svc := &profileSnapshotServiceStub{
		profiles: []agentlib.PolicyInfo{
			{Name: "cheap", MinPower: 5, MaxPower: 5},
			{Name: "default", MinPower: 7, MaxPower: 8},
			{Name: "smart", MinPower: 9, MaxPower: 10},
		},
		modelErr: fmt.Errorf("model inventory unavailable"),
	}
	SetServiceRunFactory(func(string) (agentlib.FizeauService, error) {
		return svc, nil
	})
	t.Cleanup(func() { SetServiceRunFactory(nil) })

	got := selectProfileForDispatch(context.Background(), t.TempDir(), nil, nil, SelectStrongestProfile)

	assert.Equal(t, "smart", got)
	assert.Equal(t, 1, svc.profileCalls)
	assert.Equal(t, 0, svc.modelCalls)
}

func TestLoadProfileSnapshot_DegradesToPolicyOnlyWhenModelDiscoveryTimesOut(t *testing.T) {
	resetProfileSnapshotCacheForTest(t)
	oldTimeout := profileSnapshotLoadTimeout
	profileSnapshotLoadTimeout = 25 * time.Millisecond
	t.Cleanup(func() { profileSnapshotLoadTimeout = oldTimeout })

	svc := &profileSnapshotServiceStub{
		profiles:   []agentlib.PolicyInfo{{Name: "cheap", MinPower: 5, MaxPower: 5}},
		modelDelay: time.Second,
	}

	start := time.Now()
	got, err := LoadProfileSnapshot(context.Background(), svc)

	require.NoError(t, err)
	assert.Equal(t, "cheap", got.Profiles[0].Name)
	assert.Empty(t, got.Models)
	assert.Equal(t, 1, svc.profileCalls)
	assert.Equal(t, 1, svc.modelCalls)
	assert.Less(t, time.Since(start), 250*time.Millisecond)
}

func resetProfileSnapshotCacheForTest(t *testing.T) {
	t.Helper()
	profileSnapshotCacheMu.Lock()
	old := profileSnapshotCache
	profileSnapshotCache = map[string]profileSnapshotCacheEntry{}
	profileSnapshotCacheMu.Unlock()
	t.Cleanup(func() {
		profileSnapshotCacheMu.Lock()
		profileSnapshotCache = old
		profileSnapshotCacheMu.Unlock()
	})
}

type profileSnapshotServiceStub struct {
	profiles     []agentlib.PolicyInfo
	models       []agentlib.ModelInfo
	err          error
	profileErr   error
	modelErr     error
	modelDelay   time.Duration
	profileCalls int
	modelCalls   int
}

func (s *profileSnapshotServiceStub) Execute(context.Context, agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	ch := make(chan agentlib.ServiceEvent)
	close(ch)
	return ch, nil
}

func (s *profileSnapshotServiceStub) ResolveRoute(context.Context, agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
	return nil, nil
}

func (s *profileSnapshotServiceStub) TailSessionLog(context.Context, string) (<-chan agentlib.ServiceEvent, error) {
	ch := make(chan agentlib.ServiceEvent)
	close(ch)
	return ch, nil
}

func (s *profileSnapshotServiceStub) ListHarnesses(context.Context) ([]agentlib.HarnessInfo, error) {
	return nil, nil
}

func (s *profileSnapshotServiceStub) ListProviders(context.Context) ([]agentlib.ProviderInfo, error) {
	return nil, nil
}

func (s *profileSnapshotServiceStub) ListModels(ctx context.Context, _ agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	s.modelCalls++
	if s.modelErr != nil {
		return nil, s.modelErr
	}
	if s.err != nil {
		return nil, s.err
	}
	if s.modelDelay > 0 {
		select {
		case <-time.After(s.modelDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return append([]agentlib.ModelInfo(nil), s.models...), nil
}

func (s *profileSnapshotServiceStub) ListPolicies(context.Context) ([]agentlib.PolicyInfo, error) {
	s.profileCalls++
	if s.profileErr != nil {
		return nil, s.profileErr
	}
	if s.err != nil {
		return nil, s.err
	}
	return append([]agentlib.PolicyInfo(nil), s.profiles...), nil
}

func (s *profileSnapshotServiceStub) HealthCheck(context.Context, agentlib.HealthTarget) error {
	return nil
}

func (s *profileSnapshotServiceStub) RecordRouteAttempt(context.Context, agentlib.RouteAttempt) error {
	return nil
}

func (s *profileSnapshotServiceStub) RouteStatus(context.Context) (*agentlib.RouteStatusReport, error) {
	return nil, nil
}

func (s *profileSnapshotServiceStub) ListSessionLogs(context.Context) ([]agentlib.SessionLogEntry, error) {
	return nil, nil
}

func (s *profileSnapshotServiceStub) WriteSessionLog(context.Context, string, io.Writer) error {
	return nil
}

func (s *profileSnapshotServiceStub) ReplaySession(context.Context, string, io.Writer) error {
	return nil
}

func (s *profileSnapshotServiceStub) UsageReport(context.Context, agentlib.UsageReportOptions) (*agentlib.UsageReport, error) {
	return nil, nil
}
