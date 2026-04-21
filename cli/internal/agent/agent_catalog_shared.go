package agent

import (
	"context"

	agentlib "github.com/DocumentDrivenDX/agent"
)

// profileSurfaceToDDx maps agentlib ProfileSurface names to DDx catalog surface
// identifiers. native-anthropic is absent: the embedded agent harness in DDx
// uses the "embedded-openai" surface; native-openai wins for that slot.
var profileSurfaceToDDx = map[string]string{
	"native-openai": "embedded-openai",
	"claude":        "claude",
	"codex":         "codex",
}

// ApplyCatalogFromService overlays agentlib profile data onto a DDx Catalog.
// It enumerates profiles via svc.ListProfiles and resolves surface-specific
// models via svc.ResolveProfile. Individual profile errors are silently skipped;
// the builtin catalog remains the fallback for any unresolvable entry.
func ApplyCatalogFromService(ctx context.Context, cat *Catalog, svc agentlib.DdxAgent) {
	if cat == nil || svc == nil {
		return
	}
	profiles, err := svc.ListProfiles(ctx)
	if err != nil || len(profiles) == 0 {
		return
	}
	for _, p := range profiles {
		resolved, err := svc.ResolveProfile(ctx, p.Name)
		if err != nil || resolved == nil {
			if p.Deprecated {
				cat.AddOrReplace(CatalogEntry{
					Ref:        p.Name,
					Deprecated: true,
					ReplacedBy: p.Replacement,
				})
			}
			continue
		}
		surfaces := make(map[string]string, len(resolved.Surfaces))
		for _, s := range resolved.Surfaces {
			dst, ok := profileSurfaceToDDx[s.Name]
			if !ok || s.Model == "" {
				continue
			}
			surfaces[dst] = s.Model
		}
		if len(surfaces) == 0 && !resolved.Deprecated {
			continue
		}
		cat.AddOrReplace(CatalogEntry{
			Ref:        p.Name,
			Surfaces:   surfaces,
			Deprecated: resolved.Deprecated,
			ReplacedBy: resolved.Replacement,
		})
	}
}
