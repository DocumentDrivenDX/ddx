package agent

import (
	"context"
	"sort"

	agentlib "github.com/DocumentDrivenDX/fizeau"
)

type ProfileSnapshot struct {
	Profiles []agentlib.ProfileInfo
	Models   []agentlib.ModelInfo
}

func LoadProfileSnapshot(ctx context.Context, svc agentlib.FizeauService) (ProfileSnapshot, error) {
	if svc == nil {
		return ProfileSnapshot{}, nil
	}
	profiles, err := svc.ListProfiles(ctx)
	if err != nil {
		return ProfileSnapshot{}, err
	}
	models, err := svc.ListModels(ctx, agentlib.ModelFilter{})
	if err != nil {
		return ProfileSnapshot{}, err
	}
	return ProfileSnapshot{
		Profiles: append([]agentlib.ProfileInfo(nil), profiles...),
		Models:   append([]agentlib.ModelInfo(nil), models...),
	}, nil
}

func SelectCheapestProfile(snap ProfileSnapshot) string {
	candidates := satisfiableProfiles(snap)
	if len(candidates) == 0 {
		return ""
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if profileUpperPower(left) != profileUpperPower(right) {
			return profileUpperPower(left) < profileUpperPower(right)
		}
		if profilePreferenceRank(left) != profilePreferenceRank(right) {
			return profilePreferenceRank(left) < profilePreferenceRank(right)
		}
		return left.Name < right.Name
	})
	return candidates[0].Name
}

func SelectStrongestProfile(snap ProfileSnapshot) string {
	return selectStrongestProfile(snap, 0, false)
}

func SelectStrongestProfileAbove(snap ProfileSnapshot, floor int) string {
	return selectStrongestProfile(snap, floor, true)
}

func selectStrongestProfile(snap ProfileSnapshot, floor int, enforceFloor bool) string {
	candidates := satisfiableProfiles(snap)
	if len(candidates) == 0 {
		return ""
	}
	filtered := candidates[:0]
	for _, p := range candidates {
		if enforceFloor && p.MinPower < floor {
			continue
		}
		filtered = append(filtered, p)
	}
	if len(filtered) == 0 {
		return ""
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		left, right := filtered[i], filtered[j]
		if left.MinPower != right.MinPower {
			return left.MinPower > right.MinPower
		}
		if profileUpperPower(left) != profileUpperPower(right) {
			return profileUpperPower(left) > profileUpperPower(right)
		}
		if profilePreferenceRank(left) != profilePreferenceRank(right) {
			return profilePreferenceRank(left) < profilePreferenceRank(right)
		}
		return left.Name < right.Name
	})
	return filtered[0].Name
}

func satisfiableProfiles(snap ProfileSnapshot) []agentlib.ProfileInfo {
	out := make([]agentlib.ProfileInfo, 0, len(snap.Profiles))
	for _, p := range snap.Profiles {
		if p.Name == "" || p.Deprecated {
			continue
		}
		if profileHasAvailableModel(p, snap.Models) {
			out = append(out, p)
		}
	}
	return out
}

func profileHasAvailableModel(profile agentlib.ProfileInfo, models []agentlib.ModelInfo) bool {
	for _, m := range models {
		if !m.Available || !m.AutoRoutable {
			continue
		}
		if m.Power < profile.MinPower {
			continue
		}
		if profile.MaxPower > 0 && m.Power > profile.MaxPower {
			continue
		}
		return true
	}
	return false
}

func profileUpperPower(profile agentlib.ProfileInfo) int {
	if profile.MaxPower <= 0 {
		return int(^uint(0) >> 1)
	}
	return profile.MaxPower
}

func profilePreferenceRank(profile agentlib.ProfileInfo) int {
	switch profile.ProviderPreference {
	case "local-first", "local-only":
		return 0
	case "":
		return 1
	default:
		return 2
	}
}

func selectProfileForAuxiliaryDispatch(ctx context.Context, projectRoot string, svc agentlib.FizeauService, runner AgentRunner, selector func(ProfileSnapshot) string) (string, agentlib.FizeauService) {
	selectedSvc := svc
	if selectedSvc == nil && runner == nil {
		factory := serviceRunFactory
		if factory == nil {
			factory = NewServiceFromWorkDir
		}
		built, err := factory(projectRoot)
		if err == nil {
			selectedSvc = built
		}
	}
	snap, err := LoadProfileSnapshot(ctx, selectedSvc)
	if err != nil {
		return "", selectedSvc
	}
	return selector(snap), selectedSvc
}
