package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	agentlib "github.com/DocumentDrivenDX/fizeau"
	"github.com/spf13/cobra"
)

// routeStatusJSON is the top-level JSON output shape.
type routeStatusJSON struct {
	Selection       *routeStatusSelectionJSON `json:"selection,omitempty"`
	Providers       []routeStatusProviderJSON `json:"providers,omitempty"`
	Models          []routeStatusModelJSON    `json:"models,omitempty"`
	RecentDecisions []routeStatusDecisionJSON `json:"recent_decisions,omitempty"`
	ActiveCooldowns []routeStatusCooldownJSON `json:"active_cooldowns,omitempty"`
}

type routeStatusSelectionJSON struct {
	Harness  string `json:"harness,omitempty"`
	Provider string `json:"provider,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Model    string `json:"model,omitempty"`
	Power    int    `json:"power,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type routeStatusProviderJSON struct {
	Name          string `json:"name"`
	Type          string `json:"type,omitempty"`
	Status        string `json:"status,omitempty"`
	DefaultModel  string `json:"default_model,omitempty"`
	CostClass     string `json:"cost_class,omitempty"`
	Quota         string `json:"quota,omitempty"`
	Usage         string `json:"usage,omitempty"`
	CooldownUntil string `json:"cooldown_until,omitempty"`
	Error         string `json:"error,omitempty"`
}

type routeStatusModelJSON struct {
	Provider             string  `json:"provider,omitempty"`
	Harness              string  `json:"harness,omitempty"`
	Model                string  `json:"model"`
	Available            bool    `json:"available"`
	AutoRoutable         bool    `json:"auto_routable"`
	Power                int     `json:"power,omitempty"`
	InputCostPerMTokUSD  float64 `json:"input_cost_per_mtok_usd,omitempty"`
	OutputCostPerMTokUSD float64 `json:"output_cost_per_mtok_usd,omitempty"`
	SpeedTokensPerSec    float64 `json:"speed_tokens_per_sec,omitempty"`
	CatalogRef           string  `json:"catalog_ref,omitempty"`
	RankPosition         int     `json:"rank_position,omitempty"`
}

// recentRoutingDecision is a merged view of a single routing decision sourced from
// either the live Fizeau RouteStatus report or a kind:routing bead evidence event.
//
// The live report carries current route decisions and cooldowns. kind:routing bead
// events remain for audit-only provenance.
type recentRoutingDecision struct {
	ObservedAt   time.Time
	Source       string // "bead-evidence" or "route-status"
	BeadID       string // populated for bead-evidence entries
	Harness      string // populated for route-status entries
	Provider     string // route-status decision provider or bead-evidence resolved_provider
	Model        string
	RouteReason  string // populated for bead-evidence entries
	Success      bool   // populated for route-status entries
	SuccessKnown bool   // false for bead-evidence entries (success not recorded)
	LatencyMS    int    // populated for route-status entries
}

// beadRoutingDecisionsFromStore reads kind:routing evidence events from all beads
// in the store at workDir/.ddx and returns them as recentRoutingDecision entries.
func beadRoutingDecisionsFromStore(workDir string) []recentRoutingDecision {
	store := bead.NewStore(filepath.Join(workDir, ".ddx"))
	beads, err := store.ReadAll()
	if err != nil {
		return nil
	}
	var out []recentRoutingDecision
	for _, b := range beads {
		events := routingEventsFromBeadExtra(b.Extra)
		for _, e := range events {
			d := recentRoutingDecision{
				ObservedAt: e.CreatedAt,
				Source:     "bead-evidence",
				BeadID:     b.ID,
			}
			if e.Body != "" {
				var body struct {
					ResolvedProvider string `json:"resolved_provider"`
					ResolvedModel    string `json:"resolved_model"`
					RouteReason      string `json:"route_reason"`
				}
				if jsonErr := json.Unmarshal([]byte(e.Body), &body); jsonErr == nil {
					d.Provider = body.ResolvedProvider
					d.Model = body.ResolvedModel
					d.RouteReason = body.RouteReason
				}
			}
			out = append(out, d)
		}
	}
	return out
}

// recentRoutingDecisionsFromReport converts the live RouteStatus report into
// recentRoutingDecision rows.
func recentRoutingDecisionsFromReport(report *agentlib.RouteStatusReport) []recentRoutingDecision {
	if report == nil {
		return nil
	}
	var out []recentRoutingDecision
	for _, route := range report.Routes {
		if route.LastDecision == nil || route.LastDecisionAt.IsZero() {
			continue
		}
		out = append(out, recentRoutingDecision{
			ObservedAt:   route.LastDecisionAt,
			Source:       "route-status",
			Harness:      route.LastDecision.Harness,
			Provider:     route.LastDecision.Provider,
			Model:        route.LastDecision.Model,
			RouteReason:  route.LastDecision.Reason,
			SuccessKnown: false,
		})
	}
	return out
}

// routingEventsFromBeadExtra extracts kind:routing BeadEvents from a bead's
// Extra map without an additional store read. Extra["events"] is stored as
// []any of map[string]any when loaded from JSONL.
func routingEventsFromBeadExtra(extra map[string]any) []bead.BeadEvent {
	if extra == nil {
		return nil
	}
	raw, ok := extra["events"]
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	var out []bead.BeadEvent
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		k, _ := m["kind"].(string)
		if k != "routing" {
			continue
		}
		e := bead.BeadEvent{Kind: k}
		if v, ok := m["summary"].(string); ok {
			e.Summary = v
		}
		if v, ok := m["body"].(string); ok {
			e.Body = v
		}
		if v, ok := m["actor"].(string); ok {
			e.Actor = v
		}
		if v, ok := m["source"].(string); ok {
			e.Source = v
		}
		if v, ok := m["created_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				e.CreatedAt = t
			}
		}
		out = append(out, e)
	}
	return out
}

// routeStatusDecisionJSON is the JSON-serialisable form of one routing decision
// from either the live RouteStatus report ("route-status") or kind:routing bead
// evidence ("bead-evidence").
type routeStatusDecisionJSON struct {
	ObservedAt      string `json:"observed_at"`
	Source          string `json:"source"`                     // "bead-evidence" or "route-status"
	BeadID          string `json:"bead_id,omitempty"`          // bead-evidence only
	Harness         string `json:"harness,omitempty"`          // route-status only
	CanonicalTarget string `json:"canonical_target,omitempty"` // route-status only
	Provider        string `json:"provider,omitempty"`         // bead-evidence only
	Model           string `json:"model,omitempty"`
	RouteReason     string `json:"route_reason,omitempty"` // bead-evidence only
	Success         bool   `json:"success,omitempty"`
	LatencyMs       int    `json:"latency_ms,omitempty"`
}

type routeStatusCooldownJSON struct {
	Provider      string `json:"provider"`
	CooldownUntil string `json:"cooldown_until"`
}

type routeStatusCooldown struct {
	provider      string
	cooldownUntil time.Time
}

func routeStatusProvidersJSON(providers []agentlib.ProviderInfo, harnesses []agentlib.HarnessInfo) []routeStatusProviderJSON {
	harnessByName := make(map[string]agentlib.HarnessInfo, len(harnesses))
	for _, h := range harnesses {
		harnessByName[h.Name] = h
	}

	rows := make([]routeStatusProviderJSON, 0, len(providers)+len(harnesses))
	seen := make(map[string]struct{}, len(providers))
	for _, p := range providers {
		seen[p.Name] = struct{}{}
		row := routeStatusProviderJSON{
			Name:         p.Name,
			Type:         p.Type,
			Status:       p.Status,
			DefaultModel: p.DefaultModel,
			Quota:        formatQuotaState(p.Quota),
			Usage:        formatUsageWindows(p.UsageWindows),
		}
		if p.CooldownState != nil && !p.CooldownState.Until.IsZero() {
			row.CooldownUntil = p.CooldownState.Until.UTC().Format(time.RFC3339)
		}
		if p.LastError != nil {
			row.Error = p.LastError.Detail
		}
		if h, ok := harnessByName[p.Name]; ok {
			row.CostClass = h.CostClass
			if row.Quota == "" {
				row.Quota = formatQuotaState(h.Quota)
			}
			if row.Usage == "" {
				row.Usage = formatUsageWindows(h.UsageWindows)
			}
		}
		rows = append(rows, row)
	}
	for _, h := range harnesses {
		if _, ok := seen[h.Name]; ok || !h.AutoRoutingEligible {
			continue
		}
		status := "unavailable"
		if h.Available {
			status = "available"
		}
		row := routeStatusProviderJSON{
			Name:         h.Name,
			Type:         h.Type,
			Status:       status,
			DefaultModel: h.DefaultModel,
			CostClass:    h.CostClass,
			Quota:        formatQuotaState(h.Quota),
			Usage:        formatUsageWindows(h.UsageWindows),
		}
		if h.LastError != nil {
			row.Error = h.LastError.Detail
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

func routeStatusModelsJSON(models []agentlib.ModelInfo) []routeStatusModelJSON {
	rows := make([]routeStatusModelJSON, 0, len(models))
	for _, m := range models {
		rows = append(rows, routeStatusModelJSON{
			Provider:             m.Provider,
			Harness:              m.Harness,
			Model:                m.ID,
			Available:            m.Available,
			AutoRoutable:         m.AutoRoutable,
			Power:                m.Power,
			InputCostPerMTokUSD:  m.Cost.InputPerMTok,
			OutputCostPerMTokUSD: m.Cost.OutputPerMTok,
			SpeedTokensPerSec:    m.PerfSignal.SpeedTokensPerSec,
			CatalogRef:           m.CatalogRef,
			RankPosition:         m.RankPosition,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Provider != rows[j].Provider {
			return rows[i].Provider < rows[j].Provider
		}
		return rows[i].Model < rows[j].Model
	})
	return rows
}

func providerCooldowns(providers []agentlib.ProviderInfo) []routeStatusCooldown {
	var out []routeStatusCooldown
	for _, p := range providers {
		if p.CooldownState == nil || p.CooldownState.Until.IsZero() || time.Now().After(p.CooldownState.Until) {
			continue
		}
		out = append(out, routeStatusCooldown{provider: p.Name, cooldownUntil: p.CooldownState.Until})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].provider < out[j].provider })
	return out
}

func formatQuotaState(q *agentlib.QuotaState) string {
	if q == nil {
		return "-"
	}
	status := q.Status
	if status == "" {
		status = "unknown"
	}
	if len(q.Windows) == 0 {
		return status
	}
	best := q.Windows[0]
	for _, w := range q.Windows[1:] {
		if w.UsedPercent > best.UsedPercent {
			best = w
		}
	}
	if best.UsedPercent > 0 {
		return fmt.Sprintf("%s %.0f%% %s", status, best.UsedPercent, best.Name)
	}
	return status
}

func formatUsageWindows(windows []agentlib.UsageWindow) string {
	if len(windows) == 0 {
		return "-"
	}
	w := windows[0]
	for _, candidate := range windows {
		if candidate.Name == "7d" {
			w = candidate
			break
		}
	}
	parts := []string{}
	if w.TotalTokens > 0 {
		parts = append(parts, fmt.Sprintf("%dtok", w.TotalTokens))
	}
	if w.CostUSD > 0 {
		parts = append(parts, fmt.Sprintf("$%.4f", w.CostUSD))
	}
	if len(parts) == 0 {
		return "-"
	}
	if w.Name != "" {
		parts = append(parts, w.Name)
	}
	return strings.Join(parts, " ")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func formatCostPerMTok(v float64) string {
	if v <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f", v)
}

func truncateRouteStr(value string, n int) string {
	if n <= 0 || len(value) <= n {
		return value
	}
	if n <= 2 {
		return value[:n]
	}
	return value[:n-2] + ".."
}

func (f *CommandFactory) newAgentRouteStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "route-status",
		Short: "Show live provider/model routing status",
		Long: `Shows the live provider and model inventory used by Fizeau routing:
provider status, presented models, power, cost, usage, quota, recent decisions,
and active cooldowns.

	Examples:
	  ddx agent route-status
	  ddx agent route-status --model qwen3.5-27b
	  ddx agent route-status --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			model, _ := cmd.Flags().GetString("model")
			asJSON, _ := cmd.Flags().GetBool("json")

			svc, err := agent.NewServiceFromWorkDir(f.WorkingDir)
			if err != nil {
				return fmt.Errorf("loading agent config: %w", err)
			}

			ctx := context.Background()
			providers, err := svc.ListProviders(ctx)
			if err != nil {
				return fmt.Errorf("listing providers: %w", err)
			}
			harnesses, err := svc.ListHarnesses(ctx)
			if err != nil {
				return fmt.Errorf("listing harnesses: %w", err)
			}
			models, err := svc.ListModels(ctx, agentlib.ModelFilter{})
			if err != nil {
				return fmt.Errorf("listing models: %w", err)
			}
			if model != "" {
				filtered := models[:0]
				for _, m := range models {
					if m.ID == model {
						filtered = append(filtered, m)
					}
				}
				models = filtered
			}

			var selection *routeStatusSelectionJSON
			if decision, err := svc.ResolveRoute(ctx, agentlib.RouteRequest{}); err == nil && decision != nil {
				selection = &routeStatusSelectionJSON{
					Harness:  decision.Harness,
					Provider: decision.Provider,
					Endpoint: decision.Endpoint,
					Model:    decision.Model,
					Power:    decision.Power,
					Reason:   decision.Reason,
				}
			}

			report, err := svc.RouteStatus(ctx)
			if err != nil {
				return fmt.Errorf("route status: %w", err)
			}

			// Load recent routing decisions from two complementary sources:
			//
			//  1. Live RouteStatus report — current route decisions and cooldowns.
			//  2. kind:routing bead events — execution provenance per bead (which
			//     provider/model was selected and why), tied to a specific bead ID.
			var recentDecisions []recentRoutingDecision

			recentDecisions = append(recentDecisions, recentRoutingDecisionsFromReport(report)...)

			// Merge kind:routing bead evidence entries.
			recentDecisions = append(recentDecisions, beadRoutingDecisionsFromStore(f.WorkingDir)...)

			// Sort by time and take the last N.
			sort.Slice(recentDecisions, func(i, j int) bool {
				return recentDecisions[i].ObservedAt.Before(recentDecisions[j].ObservedAt)
			})
			const maxRecent = 10
			if len(recentDecisions) > maxRecent {
				recentDecisions = recentDecisions[len(recentDecisions)-maxRecent:]
			}

			activeCooldowns := providerCooldowns(providers)

			if asJSON {
				payload := routeStatusJSON{Selection: selection}
				payload.Providers = routeStatusProvidersJSON(providers, harnesses)
				payload.Models = routeStatusModelsJSON(models)
				for _, d := range recentDecisions {
					jd := routeStatusDecisionJSON{
						ObservedAt:  d.ObservedAt.UTC().Format(time.RFC3339),
						Source:      d.Source,
						BeadID:      d.BeadID,
						Harness:     d.Harness,
						Provider:    d.Provider,
						Model:       d.Model,
						RouteReason: d.RouteReason,
					}
					if d.Source == "route-status" {
						jd.CanonicalTarget = d.Provider
						jd.Success = d.Success
						jd.LatencyMs = d.LatencyMS
					}
					payload.RecentDecisions = append(payload.RecentDecisions, jd)
				}
				for _, c := range activeCooldowns {
					cd := routeStatusCooldownJSON{
						Provider:      c.provider,
						CooldownUntil: c.cooldownUntil.UTC().Format(time.RFC3339),
					}
					payload.ActiveCooldowns = append(payload.ActiveCooldowns, cd)
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}

			out := cmd.OutOrStdout()
			if selection != nil {
				fmt.Fprintf(out, "Selected: %s/%s", selection.Provider, selection.Model)
				if selection.Harness != "" {
					fmt.Fprintf(out, " via %s", selection.Harness)
				}
				if selection.Power > 0 {
					fmt.Fprintf(out, " power=%d", selection.Power)
				}
				if selection.Reason != "" {
					fmt.Fprintf(out, " (%s)", selection.Reason)
				}
				fmt.Fprintln(out)
			} else {
				fmt.Fprintln(out, "Selected: (none)")
			}
			fmt.Fprintln(out)

			fmt.Fprintln(out, "Providers")
			fmt.Fprintf(out, "%-24s %-12s %-14s %-11s %-18s %s\n",
				"PROVIDER", "TYPE", "STATUS", "COST", "QUOTA", "USAGE")
			for _, p := range routeStatusProvidersJSON(providers, harnesses) {
				fmt.Fprintf(out, "%-24s %-12s %-14s %-11s %-18s %s\n",
					truncateRouteStr(p.Name, 24),
					truncateRouteStr(p.Type, 12),
					truncateRouteStr(p.Status, 14),
					truncateRouteStr(p.CostClass, 11),
					truncateRouteStr(p.Quota, 18),
					truncateRouteStr(p.Usage, 32),
				)
			}

			fmt.Fprintln(out)
			fmt.Fprintln(out, "Models")
			fmt.Fprintf(out, "%-24s %-36s %-5s %-5s %-11s %-11s %s\n",
				"PROVIDER", "MODEL", "POWER", "AUTO", "IN$/MTOK", "OUT$/MTOK", "STATUS")
			for _, m := range routeStatusModelsJSON(models) {
				status := "down"
				if m.Available {
					status = "up"
				}
				power := "-"
				if m.Power > 0 {
					power = fmt.Sprintf("%d", m.Power)
				}
				fmt.Fprintf(out, "%-24s %-36s %-5s %-5t %-11s %-11s %s\n",
					truncateRouteStr(firstNonEmpty(m.Provider, m.Harness), 24),
					truncateRouteStr(m.Model, 36),
					power,
					m.AutoRoutable,
					formatCostPerMTok(m.InputCostPerMTokUSD),
					formatCostPerMTok(m.OutputCostPerMTokUSD),
					status,
				)
			}

			// --- Section 2: Recent Routing Decisions ---
			fmt.Fprintln(out)
			fmt.Fprintf(out, "Recent Routing Decisions (last %d)\n", len(recentDecisions))
			fmt.Fprintf(out, "%s\n", strings.Repeat("-", 90))
			if len(recentDecisions) == 0 {
				fmt.Fprintln(out, "  (no recorded decisions)")
			} else {
				fmt.Fprintf(out, "%-22s %-14s %-24s %-20s %-6s %s\n",
					"OBSERVED_AT", "SOURCE", "PROVIDER", "MODEL", "OK", "BEAD")
				for _, d := range recentDecisions {
					ok := "-"
					if d.SuccessKnown {
						ok = "no"
						if d.Success {
							ok = "yes"
						}
					}
					fmt.Fprintf(out, "%-22s %-14s %-24s %-20s %-6s %s\n",
						d.ObservedAt.UTC().Format("2006-01-02T15:04:05Z"),
						d.Source,
						truncateRouteStr(d.Provider, 24),
						truncateRouteStr(d.Model, 20),
						ok,
						d.BeadID,
					)
				}
			}

			// --- Section 3: Active Health Cooldowns ---
			fmt.Fprintln(out)
			fmt.Fprintf(out, "Active Health Cooldowns\n")
			fmt.Fprintf(out, "%s\n", strings.Repeat("-", 70))
			if len(activeCooldowns) == 0 {
				fmt.Fprintln(out, "  (none)")
			} else {
				fmt.Fprintf(out, "%-24s %s\n", "PROVIDER", "COOLDOWN_UNTIL")
				for _, c := range activeCooldowns {
					fmt.Fprintf(out, "%-24s %s\n",
						c.provider,
						c.cooldownUntil.UTC().Format("2006-01-02T15:04:05Z"),
					)
				}
			}

			return nil
		},
	}
	cmd.Flags().String("model", "", "Filter to a concrete model id")
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}
