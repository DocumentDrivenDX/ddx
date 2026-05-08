package agent

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"
	"sync"
	"time"

	agentlib "github.com/DocumentDrivenDX/fizeau"
)

const profileSnapshotCacheWindow = 30 * time.Second

var (
	profileSnapshotCacheMu sync.Mutex
	profileSnapshotCache   = map[string]profileSnapshotCacheEntry{}
	profileSnapshotNow     = time.Now
)

type profileSnapshotCacheEntry struct {
	snap     ProfileSnapshot
	loadedAt time.Time
}

// ProfileSnapshot is the fizeau-owned routing vocabulary DDx uses for hidden
// lifecycle dispatches. DDx selects profile names from this snapshot; it does
// not resolve concrete models.
type ProfileSnapshot struct {
	Profiles []agentlib.ProfileInfo
	Models   []agentlib.ModelInfo
}

// LoadProfileSnapshot fetches fizeau profiles and models with short-lived,
// stale-on-error memoization keyed to the service identity.
func LoadProfileSnapshot(ctx context.Context, svc agentlib.FizeauService) (ProfileSnapshot, error) {
	if svc == nil {
		return ProfileSnapshot{}, nil
	}
	key := profileSnapshotCacheKey(svc)
	now := profileSnapshotNow()

	profileSnapshotCacheMu.Lock()
	if entry, ok := profileSnapshotCache[key]; ok && now.Sub(entry.loadedAt) < profileSnapshotCacheWindow {
		snap := cloneProfileSnapshot(entry.snap)
		profileSnapshotCacheMu.Unlock()
		return snap, nil
	}
	last, hadLast := profileSnapshotCache[key]
	profileSnapshotCacheMu.Unlock()

	profiles, err := svc.ListProfiles(ctx)
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
		Profiles: append([]agentlib.ProfileInfo(nil), profiles...),
		Models:   append([]agentlib.ModelInfo(nil), models...),
	}
	profileSnapshotCacheMu.Lock()
	profileSnapshotCache[key] = profileSnapshotCacheEntry{snap: cloneProfileSnapshot(snap), loadedAt: now}
	profileSnapshotCacheMu.Unlock()
	return snap, nil
}

func profileSnapshotCacheKey(svc agentlib.FizeauService) string {
	v := reflect.ValueOf(svc)
	if !v.IsValid() {
		return "<nil>"
	}
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return fmt.Sprintf("%s:%x", v.Type(), v.Pointer())
	default:
		return fmt.Sprintf("%T:%v", svc, svc)
	}
}

func cloneProfileSnapshot(snap ProfileSnapshot) ProfileSnapshot {
	return ProfileSnapshot{
		Profiles: append([]agentlib.ProfileInfo(nil), snap.Profiles...),
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

func satisfiableProfiles(snap ProfileSnapshot, floor int) []agentlib.ProfileInfo {
	out := make([]agentlib.ProfileInfo, 0, len(snap.Profiles))
	for _, profile := range snap.Profiles {
		if profile.Name == "" || profile.Deprecated {
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

func profileHasAvailableModel(profile agentlib.ProfileInfo, models []agentlib.ModelInfo) bool {
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

func profileMaxForSort(profile agentlib.ProfileInfo) int {
	if profile.MaxPower == 0 {
		return math.MaxInt
	}
	return profile.MaxPower
}

func preferProfile(left, right agentlib.ProfileInfo) bool {
	leftLocal := left.ProviderPreference == "local-first"
	rightLocal := right.ProviderPreference == "local-first"
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
		built, err := NewServiceFromWorkDir(projectRoot)
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
