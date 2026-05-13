package agent

import (
	"context"
	"math"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	agentlib "github.com/easel/fizeau"
)

const profileSnapshotCacheWindow = 30 * time.Second

var (
	profileSnapshotCacheMu sync.Mutex
	profileSnapshotCache   = map[agentlib.FizeauService]profileSnapshotCacheEntry{}
	profileSnapshotNow     = time.Now
)

type profileSnapshotCacheEntry struct {
	snap     ProfileSnapshot
	loadedAt time.Time
}

// ProfileSnapshot is the fizeau-owned routing vocabulary DDx uses for hidden
// lifecycle dispatches. DDx selects policy names from this snapshot; it does
// not resolve concrete models.
type ProfileSnapshot struct {
	Profiles []agentlib.PolicyInfo
	Models   []agentlib.ModelInfo
}

type ImplementationProfileSelection struct {
	Name     string
	MinPower int
	Degraded bool
	Note     string
}

// LoadProfileSnapshot fetches fizeau profiles and models with short-lived,
// stale-on-error memoization keyed to the service identity.
func LoadProfileSnapshot(ctx context.Context, svc agentlib.FizeauService) (ProfileSnapshot, error) {
	if svc == nil {
		return ProfileSnapshot{}, nil
	}
	key, cacheable := profileSnapshotCacheKey(svc)
	now := profileSnapshotNow()

	var last profileSnapshotCacheEntry
	var hadLast bool
	if cacheable {
		profileSnapshotCacheMu.Lock()
		if entry, ok := profileSnapshotCache[key]; ok && now.Sub(entry.loadedAt) < profileSnapshotCacheWindow {
			snap := cloneProfileSnapshot(entry.snap)
			profileSnapshotCacheMu.Unlock()
			return snap, nil
		}
		last, hadLast = profileSnapshotCache[key]
		profileSnapshotCacheMu.Unlock()
	}

	profiles, err := svc.ListPolicies(ctx)
	if err != nil {
		if hadLast {
			return cloneProfileSnapshot(last.snap), nil
		}
		return ProfileSnapshot{}, err
	}
	models, err := svc.ListModels(ctx, agentlib.ModelFilter{})
	if err != nil {
		if hadLast {
			return cloneProfileSnapshot(last.snap), nil
		}
		return ProfileSnapshot{}, err
	}

	snap := ProfileSnapshot{
		Profiles: append([]agentlib.PolicyInfo(nil), profiles...),
		Models:   append([]agentlib.ModelInfo(nil), models...),
	}
	if cacheable {
		profileSnapshotCacheMu.Lock()
		profileSnapshotCache[key] = profileSnapshotCacheEntry{snap: cloneProfileSnapshot(snap), loadedAt: now}
		profileSnapshotCacheMu.Unlock()
	}
	return snap, nil
}

func profileSnapshotCacheKey(svc agentlib.FizeauService) (agentlib.FizeauService, bool) {
	t := reflect.TypeOf(svc)
	if t == nil || !t.Comparable() {
		return nil, false
	}
	return svc, true
}

func cloneProfileSnapshot(snap ProfileSnapshot) ProfileSnapshot {
	return ProfileSnapshot{
		Profiles: append([]agentlib.PolicyInfo(nil), snap.Profiles...),
		Models:   append([]agentlib.ModelInfo(nil), snap.Models...),
	}
}

// SelectCheapestProfile returns the lowest-power profile band that has at least
// one available, auto-routable model.
func SelectCheapestProfile(snap ProfileSnapshot) string {
	profiles := satisfiableProfiles(snap, 0)
	sort.SliceStable(profiles, func(i, j int) bool {
		left, right := profiles[i], profiles[j]
		if profileMaxForSort(left) != profileMaxForSort(right) {
			return profileMaxForSort(left) < profileMaxForSort(right)
		}
		if left.MinPower != right.MinPower {
			return left.MinPower < right.MinPower
		}
		return preferProfile(left, right)
	})
	if len(profiles) == 0 {
		return ""
	}
	return profiles[0].Name
}

// SelectStrongestProfile returns the highest-power profile band that has at
// least one available, auto-routable model.
func SelectStrongestProfile(snap ProfileSnapshot) string {
	profiles := satisfiableProfiles(snap, 0)
	sort.SliceStable(profiles, func(i, j int) bool {
		left, right := profiles[i], profiles[j]
		if left.MinPower != right.MinPower {
			return left.MinPower > right.MinPower
		}
		if profileMaxForSort(left) != profileMaxForSort(right) {
			return profileMaxForSort(left) > profileMaxForSort(right)
		}
		return preferProfile(left, right)
	})
	if len(profiles) == 0 {
		return ""
	}
	return profiles[0].Name
}

// SelectStrongestProfileAbove returns the strongest satisfiable profile whose
// lower bound is at least floor.
func SelectStrongestProfileAbove(snap ProfileSnapshot, floor int) string {
	profiles := satisfiableProfiles(snap, floor)
	sort.SliceStable(profiles, func(i, j int) bool {
		left, right := profiles[i], profiles[j]
		if left.MinPower != right.MinPower {
			return left.MinPower > right.MinPower
		}
		if profileMaxForSort(left) != profileMaxForSort(right) {
			return profileMaxForSort(left) > profileMaxForSort(right)
		}
		return preferProfile(left, right)
	})
	if len(profiles) == 0 {
		return ""
	}
	return profiles[0].Name
}

// SelectImplementationProfile chooses an opaque Fizeau profile name for
// implementation work. It ranks by policy metadata, not by hard-coded policy
// names. Ordinary auto-routing avoids profiles with hard requirements (for
// example local-only/no-remote policy requirements) unless DDx has an explicit
// matching intent; explicit --profile remains a raw passthrough handled by
// Fizeau.
func SelectImplementationProfile(snap ProfileSnapshot, tier escalation.ModelTier) ImplementationProfileSelection {
	return selectImplementationProfile(snap, tier, 0)
}

// SelectImplementationProfileForMinPower chooses an implementation profile
// whose advertised power range can satisfy the requested lower bound. It is
// used by retry escalation so DDx does not keep a weak profile pinned after it
// has raised MinPower beyond that profile's range.
func SelectImplementationProfileForMinPower(snap ProfileSnapshot, tier escalation.ModelTier, floor int) ImplementationProfileSelection {
	return selectImplementationProfile(snap, tier, floor)
}

func selectImplementationProfile(snap ProfileSnapshot, tier escalation.ModelTier, floor int) ImplementationProfileSelection {
	profiles := implementationCandidateProfiles(snap, floor)
	if len(profiles) == 0 {
		return ImplementationProfileSelection{}
	}
	selected, note := chooseImplementationCandidate(profiles, tier, floor)
	return ImplementationProfileSelection{
		Name:     selected.Name,
		MinPower: selected.MinPower,
		Degraded: note != "",
		Note:     note,
	}
}

func implementationCandidateProfiles(snap ProfileSnapshot, floor int) []agentlib.PolicyInfo {
	profiles := satisfiableProfiles(snap, 0)
	out := make([]agentlib.PolicyInfo, 0, len(profiles))
	for _, profile := range profiles {
		if hasHardPolicyRequirement(profile) {
			continue
		}
		if floor > 0 && profileMaxForSort(profile) < floor {
			continue
		}
		out = append(out, profile)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return implementationProfileLess(snap, out[i], out[j])
	})
	return out
}

func chooseImplementationCandidate(profiles []agentlib.PolicyInfo, tier escalation.ModelTier, floor int) (agentlib.PolicyInfo, string) {
	if floor > 0 && tier != escalation.TierSmart {
		return profiles[0], ""
	}
	switch tier {
	case escalation.TierSmart:
		return profiles[len(profiles)-1], ""
	case escalation.TierStandard:
		if selected, ok := firstBandAboveLowest(profiles); ok {
			return selected, ""
		}
		return profiles[0], "medium profile unavailable; using only available profile"
	case escalation.TierCheap, "":
		return profiles[0], ""
	default:
		return profiles[0], ""
	}
}

func firstBandAboveLowest(profiles []agentlib.PolicyInfo) (agentlib.PolicyInfo, bool) {
	if len(profiles) <= 1 {
		return agentlib.PolicyInfo{}, false
	}
	lowest := profiles[0]
	for _, profile := range profiles[1:] {
		if profile.MinPower > lowest.MinPower || profileMaxForSort(profile) > profileMaxForSort(lowest) {
			return profile, true
		}
	}
	return agentlib.PolicyInfo{}, false
}

func hasHardPolicyRequirement(profile agentlib.PolicyInfo) bool {
	for _, requirement := range profile.Require {
		if strings.TrimSpace(requirement) != "" {
			return true
		}
	}
	return false
}

func satisfiableProfiles(snap ProfileSnapshot, floor int) []agentlib.PolicyInfo {
	out := make([]agentlib.PolicyInfo, 0, len(snap.Profiles))
	for _, profile := range snap.Profiles {
		if profile.Name == "" {
			continue
		}
		if profile.MinPower == 0 && profile.MaxPower == 0 {
			continue
		}
		if floor > 0 && profile.MinPower < floor {
			continue
		}
		if profileHasAvailableModel(profile, snap.Models) {
			out = append(out, profile)
		}
	}
	return out
}

func profileHasAvailableModel(profile agentlib.PolicyInfo, models []agentlib.ModelInfo) bool {
	for _, model := range models {
		if !model.Available || !model.AutoRoutable || model.ExactPinOnly {
			continue
		}
		if profile.MinPower > 0 && model.Power < profile.MinPower {
			continue
		}
		if profile.MaxPower > 0 && model.Power > profile.MaxPower {
			continue
		}
		return true
	}
	return false
}

func implementationProfileLess(snap ProfileSnapshot, left, right agentlib.PolicyInfo) bool {
	if profileMaxForSort(left) != profileMaxForSort(right) {
		return profileMaxForSort(left) < profileMaxForSort(right)
	}
	if left.MinPower != right.MinPower {
		return left.MinPower < right.MinPower
	}
	leftCost, leftCostOK := profileBestCost(left, snap.Models)
	rightCost, rightCostOK := profileBestCost(right, snap.Models)
	if leftCostOK != rightCostOK {
		return leftCostOK
	}
	if leftCostOK && rightCostOK && leftCost != rightCost {
		return leftCost < rightCost
	}
	leftSpeed, leftSpeedOK := profileBestSpeed(left, snap.Models)
	rightSpeed, rightSpeedOK := profileBestSpeed(right, snap.Models)
	if leftSpeedOK != rightSpeedOK {
		return leftSpeedOK
	}
	if leftSpeedOK && rightSpeedOK && leftSpeed != rightSpeed {
		return leftSpeed > rightSpeed
	}
	return left.Name < right.Name
}

func profileBestCost(profile agentlib.PolicyInfo, models []agentlib.ModelInfo) (float64, bool) {
	best := 0.0
	ok := false
	for _, model := range models {
		if !modelFitsProfile(profile, model) {
			continue
		}
		cost := model.Cost.InputPerMTok + model.Cost.OutputPerMTok
		if !ok || cost < best {
			best = cost
			ok = true
		}
	}
	return best, ok
}

func profileBestSpeed(profile agentlib.PolicyInfo, models []agentlib.ModelInfo) (float64, bool) {
	best := 0.0
	ok := false
	for _, model := range models {
		if !modelFitsProfile(profile, model) {
			continue
		}
		speed := model.PerfSignal.SpeedTokensPerSec
		if speed <= 0 {
			continue
		}
		if !ok || speed > best {
			best = speed
			ok = true
		}
	}
	return best, ok
}

func modelFitsProfile(profile agentlib.PolicyInfo, model agentlib.ModelInfo) bool {
	if !model.Available || !model.AutoRoutable || model.ExactPinOnly {
		return false
	}
	if profile.MinPower > 0 && model.Power < profile.MinPower {
		return false
	}
	if profile.MaxPower > 0 && model.Power > profile.MaxPower {
		return false
	}
	return true
}

func profileMaxForSort(profile agentlib.PolicyInfo) int {
	if profile.MaxPower == 0 {
		return math.MaxInt
	}
	return profile.MaxPower
}

func preferProfile(left, right agentlib.PolicyInfo) bool {
	leftLocal := left.AllowLocal
	rightLocal := right.AllowLocal
	if leftLocal != rightLocal {
		return leftLocal
	}
	return left.Name < right.Name
}

func selectProfileForDispatch(ctx context.Context, projectRoot string, svc agentlib.FizeauService, runner AgentRunner, selector func(ProfileSnapshot) string) string {
	if selector == nil || runner != nil {
		return ""
	}
	selectedSvc := svc
	if selectedSvc == nil {
		factory := serviceRunFactory
		if factory == nil {
			factory = NewServiceFromWorkDir
		}
		built, err := factory(projectRoot)
		if err != nil {
			return ""
		}
		selectedSvc = built
	}
	snap, err := LoadProfileSnapshot(ctx, selectedSvc)
	if err != nil {
		return ""
	}
	return selector(snap)
}
