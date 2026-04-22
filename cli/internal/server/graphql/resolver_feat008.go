package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/persona"
)

var pluginFixtures = []*PluginInfo{
	{
		Name:             "helix",
		Version:          "1.4.2",
		InstalledVersion: strPtr("1.4.2"),
		Type:             "workflow",
		Description:      "HELIX methodology: phases, gates, supervisory dispatch",
		Keywords:         []string{"workflow", "methodology"},
		Status:           "installed",
		RegistrySource:   "builtin",
		DiskBytes:        4200000,
		Manifest:         strPtr(`{"name":"helix","version":"1.4.2"}`),
		Skills:           []string{"helix-align", "helix-plan"},
		Prompts:          []string{"drain-queue", "run-checks"},
		Templates:        []string{"FEAT-spec"},
	},
	{
		Name:           "frontend-design",
		Version:        "0.3.1",
		Type:           "persona-pack",
		Description:    "Palette-disciplined UI/UX review skill",
		Keywords:       []string{"design", "ui", "a11y"},
		Status:         "available",
		RegistrySource: "builtin",
		DiskBytes:      800000,
		Skills:         []string{},
		Prompts:        []string{},
		Templates:      []string{},
	},
	{
		Name:             "ddx-cost-tier",
		Version:          "0.5.0",
		InstalledVersion: strPtr("0.4.2"),
		Type:             "plugin",
		Description:      "Cost-tiered routing policies for ddx agent",
		Keywords:         []string{"routing", "cost"},
		Status:           "update-available",
		RegistrySource:   "https://github.com/example/ddx-plugins",
		DiskBytes:        1200000,
		Skills:           []string{},
		Prompts:          []string{},
		Templates:        []string{},
	},
}

// BeadClose is the resolver for the beadClose field.
func (r *mutationResolver) BeadClose(ctx context.Context, id string, reason *string) (*Bead, error) {
	if r.WorkingDir == "" {
		return nil, fmt.Errorf("working directory not configured")
	}
	store := r.beadStore()
	if err := store.Close(id); err != nil {
		return nil, err
	}
	if reason != nil && strings.TrimSpace(*reason) != "" {
		_ = store.AppendEvent(id, bead.BeadEvent{
			Kind:    "summary",
			Summary: "closed: " + strings.TrimSpace(*reason),
		})
	}
	b, err := store.Get(id)
	if err != nil {
		return nil, err
	}
	return beadModelFromBead(b), nil
}

// WorkerDispatch is the resolver for the workerDispatch field.
func (r *mutationResolver) WorkerDispatch(ctx context.Context, kind string, projectID string, args *string) (*WorkerDispatchResult, error) {
	switch kind {
	case "execute-loop", "realign-specs", "run-checks":
	default:
		return nil, fmt.Errorf("unsupported worker kind %q", kind)
	}
	return &WorkerDispatchResult{
		ID:    "worker-" + strings.ReplaceAll(kind, "-", "-stub-"),
		State: "queued",
		Kind:  kind,
	}, nil
}

// PluginDispatch is the resolver for the pluginDispatch field.
func (r *mutationResolver) PluginDispatch(ctx context.Context, name string, action string, scope string) (*PluginDispatchResult, error) {
	return &PluginDispatchResult{
		ID:     "worker-" + action + "-" + slug(name),
		State:  "queued",
		Action: action,
	}, nil
}

// ComparisonDispatch is the resolver for the comparisonDispatch field.
func (r *mutationResolver) ComparisonDispatch(ctx context.Context, arms []*ComparisonArmInput) (*ComparisonDispatchResult, error) {
	return &ComparisonDispatchResult{
		ID:       "cmp-stub-001",
		State:    "queued",
		ArmCount: len(arms),
	}, nil
}

// PersonaBind is the resolver for the personaBind field.
func (r *mutationResolver) PersonaBind(ctx context.Context, role string, personaName string, projectID string) (*PersonaBindResult, error) {
	root := r.projectRoot(projectID)
	configPath := filepath.Join(root, ".ddx", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return nil, err
	}
	manager := persona.NewBindingManagerWithPath(configPath)
	if err := manager.SetBinding(role, personaName); err != nil {
		return nil, err
	}
	return &PersonaBindResult{Ok: true, Role: role, Persona: personaName}, nil
}

// QueueSummary is the resolver for the queueSummary field.
func (r *queryResolver) QueueSummary(ctx context.Context, projectID string) (*QueueSummary, error) {
	var out QueueSummary
	for _, snap := range r.State.GetBeadSnapshots("", "", projectID, "") {
		switch snap.Status {
		case "blocked":
			out.Blocked++
		case bead.StatusInProgress, "in-progress":
			out.InProgress++
		case bead.StatusOpen:
			out.Ready++
		}
	}
	return &out, nil
}

// EfficacyRows is the resolver for the efficacyRows field.
func (r *queryResolver) EfficacyRows(ctx context.Context) ([]*EfficacyRow, error) {
	return []*EfficacyRow{
		{
			RowKey:             "codex|openai|gpt-5",
			Harness:            "codex",
			Provider:           "openai",
			Model:              "gpt-5",
			Attempts:           42,
			Successes:          40,
			SuccessRate:        0.9524,
			MedianInputTokens:  3200,
			MedianOutputTokens: 1100,
			MedianDurationMs:   28000,
			MedianCostUsd:      floatPtr(0.032),
			Warning:            nil,
		},
		{
			RowKey:             "claude|anthropic|claude-sonnet-4-6",
			Harness:            "claude",
			Provider:           "anthropic",
			Model:              "claude-sonnet-4-6",
			Attempts:           60,
			Successes:          57,
			SuccessRate:        0.95,
			MedianInputTokens:  4100,
			MedianOutputTokens: 1500,
			MedianDurationMs:   45000,
			MedianCostUsd:      floatPtr(0.047),
			Warning:            nil,
		},
		{
			RowKey:             "codex|vidar-omlx|qwen3.6-35b",
			Harness:            "codex",
			Provider:           "vidar-omlx",
			Model:              "qwen3.6-35b",
			Attempts:           80,
			Successes:          48,
			SuccessRate:        0.6,
			MedianInputTokens:  2800,
			MedianOutputTokens: 900,
			MedianDurationMs:   62000,
			MedianCostUsd:      nil,
			Warning:            &EfficacyWarning{Kind: "below-adaptive-floor", Threshold: floatPtr(0.7)},
		},
	}, nil
}

// EfficacyAttempts is the resolver for the efficacyAttempts field.
func (r *queryResolver) EfficacyAttempts(ctx context.Context, rowKey string) (*EfficacyAttempts, error) {
	out := &EfficacyAttempts{RowKey: rowKey}
	for i := 0; i < 10; i++ {
		out.Attempts = append(out.Attempts, &EfficacyAttempt{
			BeadID:            fmt.Sprintf("ddx-attempt-%d", i),
			Outcome:           map[bool]string{true: "failed", false: "succeeded"}[i%4 == 3],
			DurationMs:        20000 + i*1500,
			CostUsd:           floatPtr(0.02 + float64(i)*0.002),
			EvidenceBundleURL: fmt.Sprintf("/executions/exec-%d/result.json", i),
		})
	}
	return out, nil
}

// Comparisons is the resolver for the comparisons field.
func (r *queryResolver) Comparisons(ctx context.Context) ([]*ComparisonRecord, error) {
	return []*ComparisonRecord{}, nil
}

// PluginsList is the resolver for the pluginsList field.
func (r *queryResolver) PluginsList(ctx context.Context) ([]*PluginInfo, error) {
	return pluginFixtures, nil
}

// PluginDetail is the resolver for the pluginDetail field.
func (r *queryResolver) PluginDetail(ctx context.Context, name string) (*PluginInfo, error) {
	for _, plugin := range pluginFixtures {
		if plugin.Name == name {
			return plugin, nil
		}
	}
	return nil, nil
}

// ProjectBindings is the resolver for the projectBindings field.
func (r *queryResolver) ProjectBindings(ctx context.Context, projectID string) (string, error) {
	cfg, err := config.LoadWithWorkingDir(r.projectRoot(projectID))
	if err != nil {
		return "{}", nil
	}
	raw, err := json.Marshal(cfg.PersonaBindings)
	if err != nil {
		return "{}", nil
	}
	return string(raw), nil
}

// PaletteSearch is the resolver for the paletteSearch field.
func (r *queryResolver) PaletteSearch(ctx context.Context, query string) (*PaletteSearchResults, error) {
	out := &PaletteSearchResults{
		Documents:  []*PaletteDocumentResult{},
		Beads:      []*PaletteBeadResult{},
		Actions:    []*PaletteActionResult{},
		Navigation: []*PaletteNavigationResult{},
	}
	if strings.TrimSpace(query) == "" {
		return out, nil
	}
	out.Documents = append(out.Documents, &PaletteDocumentResult{
		Kind:  "document",
		Path:  "docs/helix/01-frame/features/FEAT-008-web-ui.md",
		Title: "FEAT-008 Web UI",
	})
	out.Beads = append(out.Beads, &PaletteBeadResult{
		Kind:  "bead",
		ID:    "ddx-feat008-1",
		Title: "Implement Actions panel",
	})
	out.Actions = append(out.Actions, &PaletteActionResult{
		Kind:  "action",
		ID:    "drain-queue",
		Label: "Drain queue",
	})
	out.Navigation = append(out.Navigation, &PaletteNavigationResult{
		Kind:  "nav",
		Route: "/efficacy",
		Title: "Efficacy",
	})
	return out, nil
}

func (r *Resolver) projectRoot(projectID string) string {
	if r.State != nil {
		if proj, ok := r.State.GetProjectSnapshotByID(projectID); ok && proj.Path != "" {
			return proj.Path
		}
	}
	return r.WorkingDir
}

func strPtr(v string) *string {
	return &v
}

func floatPtr(v float64) *float64 {
	return &v
}

func slug(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, "_", "-")
	v = strings.ReplaceAll(v, " ", "-")
	return v
}
