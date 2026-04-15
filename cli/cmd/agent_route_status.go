package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	agentconfig "github.com/DocumentDrivenDX/agent/config"
	"github.com/DocumentDrivenDX/agent/observations"
	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/providerstatus"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/spf13/cobra"
)

const (
	routeStatusProbeTimeout    = 5 * time.Second
	defaultRouteHealthCooldown = 30 * time.Minute
)

// agentRouteHealthState mirrors the structure stored by ddx-agent in
// .agent/route-health-{key}.json. Each failure records the time at which
// a provider last failed for a given route key; candidates within the
// cooldown window are deprioritised.
type agentRouteHealthState struct {
	Failures map[string]time.Time `json:"failures,omitempty"`
}

func routeStatusStateKey(routeName string) string {
	r := strings.NewReplacer("/", "_", ":", "_", " ", "_")
	return r.Replace(routeName)
}

func routeStatusHealthFile(workDir, routeKey string) string {
	return filepath.Join(workDir, ".agent", "route-health-"+routeStatusStateKey(routeKey)+".json")
}

func loadAgentRouteHealthState(workDir, routeKey string) agentRouteHealthState {
	data, err := os.ReadFile(routeStatusHealthFile(workDir, routeKey))
	if err != nil {
		return agentRouteHealthState{Failures: make(map[string]time.Time)}
	}
	var state agentRouteHealthState
	if err := json.Unmarshal(data, &state); err != nil {
		return agentRouteHealthState{Failures: make(map[string]time.Time)}
	}
	if state.Failures == nil {
		state.Failures = make(map[string]time.Time)
	}
	return state
}

func parseRouteCooldownDuration(cfg *agentconfig.Config) time.Duration {
	if cfg == nil || strings.TrimSpace(cfg.Routing.HealthCooldown) == "" {
		return defaultRouteHealthCooldown
	}
	d, err := time.ParseDuration(cfg.Routing.HealthCooldown)
	if err != nil || d <= 0 {
		return defaultRouteHealthCooldown
	}
	return d
}

// routeCandidateStatus holds the evaluated routing state for one provider candidate.
type routeCandidateStatus struct {
	Provider      string
	Model         string
	Healthy       bool
	InCooldown    bool
	CooldownUntil time.Time
	AvgLatencyMs  float64
	Reliability   float64
	Score         float64
	Reason        string
	ProbeMsg      string
}

func evalAgentRouteCandidate(
	cfg *agentconfig.Config,
	candidate agentconfig.ModelRouteCandidateConfig,
	obs *observations.Store,
	healthState agentRouteHealthState,
	cooldown time.Duration,
) routeCandidateStatus {
	rc := routeCandidateStatus{
		Provider: candidate.Provider,
		Model:    candidate.Model,
	}

	pc, ok := cfg.GetProvider(candidate.Provider)
	if !ok {
		rc.Reason = "unknown provider"
		rc.ProbeMsg = "unknown provider"
		return rc
	}

	if rc.Model == "" {
		rc.Model = pc.Model
	}

	// Check health cooldown.
	if failedAt, inCooldown := healthState.Failures[candidate.Provider]; inCooldown {
		if time.Since(failedAt) < cooldown {
			rc.InCooldown = true
			rc.CooldownUntil = failedAt.Add(cooldown)
			rc.Reason = fmt.Sprintf("cooldown until %s", rc.CooldownUntil.Format(time.RFC3339))
		}
	}

	// Probe provider health.
	ctx, cancel := context.WithTimeout(context.Background(), routeStatusProbeTimeout)
	pr := providerstatus.Probe(ctx, pc)
	cancel()
	rc.ProbeMsg = pr.Message
	if pr.Reachable {
		if !rc.InCooldown {
			rc.Healthy = true
		}
	} else {
		if !rc.InCooldown {
			rc.Reason = pr.Message
		}
	}

	// Observations for latency estimate.
	if obs != nil && rc.Model != "" {
		key := observations.Key{ProviderSystem: candidate.Provider, Model: rc.Model}
		if mean, ok := obs.MeanSpeed(key); ok && mean > 0 {
			// Convert output tokens/sec to approximate ms for 100 output tokens.
			rc.AvgLatencyMs = 100.0 / mean * 1000.0
		}
	}

	return rc
}

// routeStatusCandidateJSON is the JSON-serialisable form of routeCandidateStatus.
type routeStatusCandidateJSON struct {
	Provider      string  `json:"provider"`
	Model         string  `json:"model,omitempty"`
	Healthy       bool    `json:"healthy"`
	InCooldown    bool    `json:"in_cooldown,omitempty"`
	CooldownUntil string  `json:"cooldown_until,omitempty"`
	AvgDurationMs float64 `json:"avg_duration_ms,omitempty"`
	Reliability   float64 `json:"reliability,omitempty"`
	Score         float64 `json:"score,omitempty"`
	Reason        string  `json:"reason,omitempty"`
}

// routeStatusRouteJSON is the JSON-serialisable form of one model route.
type routeStatusRouteJSON struct {
	RouteKey         string                     `json:"route_key"`
	Strategy         string                     `json:"strategy"`
	SelectedProvider string                     `json:"selected_provider,omitempty"`
	SelectedModel    string                     `json:"selected_model,omitempty"`
	Candidates       []routeStatusCandidateJSON `json:"candidates"`
}

// routeStatusJSON is the top-level JSON output shape.
type routeStatusJSON struct {
	Routes          []routeStatusRouteJSON    `json:"routes,omitempty"`
	RecentDecisions []routeStatusDecisionJSON `json:"recent_decisions,omitempty"`
	ActiveCooldowns []routeStatusCooldownJSON `json:"active_cooldowns,omitempty"`
}

// recentRoutingDecision is a merged view of a single routing decision sourced from
// either the RoutingMetricsStore or a kind:routing bead evidence event.
//
// Both RoutingMetricsStore and kind:routing bead events are intentionally kept:
//   - RoutingMetricsStore (.ddx/agent-logs/routing-outcomes.jsonl) records
//     harness-level analytics (latency, success rate) for every agent run.
//   - kind:routing bead evidence records execution provenance per bead: which
//     provider/model was selected and why, tied to a specific bead ID.
type recentRoutingDecision struct {
	ObservedAt   time.Time
	Source       string // "bead-evidence" or "metrics-store"
	BeadID       string // populated for bead-evidence entries
	Harness      string // populated for metrics-store entries
	Provider     string // resolved_provider (bead-evidence) or CanonicalTarget (metrics-store)
	Model        string
	RouteReason  string // populated for bead-evidence entries
	Success      bool   // populated for metrics-store entries
	SuccessKnown bool   // false for bead-evidence entries (success not recorded)
	LatencyMS    int    // populated for metrics-store entries
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
// from either the RoutingMetricsStore ("metrics-store") or kind:routing bead
// evidence ("bead-evidence").
type routeStatusDecisionJSON struct {
	ObservedAt      string `json:"observed_at"`
	Source          string `json:"source"`                     // "bead-evidence" or "metrics-store"
	BeadID          string `json:"bead_id,omitempty"`          // bead-evidence only
	Harness         string `json:"harness,omitempty"`          // metrics-store only
	CanonicalTarget string `json:"canonical_target,omitempty"` // metrics-store only
	Provider        string `json:"provider,omitempty"`         // bead-evidence only
	Model           string `json:"model,omitempty"`
	RouteReason     string `json:"route_reason,omitempty"` // bead-evidence only
	Success         bool   `json:"success,omitempty"`
	LatencyMs       int    `json:"latency_ms,omitempty"`
}

type routeStatusCooldownJSON struct {
	Route         string `json:"route"`
	Provider      string `json:"provider"`
	FailedAt      string `json:"failed_at"`
	CooldownUntil string `json:"cooldown_until"`
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
		Short: "Show routing table, recent decisions, and active health cooldowns",
		Long: `Shows the current provider routing state, recent routing decisions, and
any health cooldowns currently in effect.

Mirrors ddx-agent route-status output using the Go package API. Requires model
routes to be configured in .agent/config.yaml.

Examples:
  ddx agent route-status
  ddx agent route-status --model qwen3.5-27b
  ddx agent route-status --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			model, _ := cmd.Flags().GetString("model")
			asJSON, _ := cmd.Flags().GetBool("json")

			cfg, err := agentconfig.Load(f.WorkingDir)
			if err != nil {
				return fmt.Errorf("loading agent config: %w", err)
			}

			cooldown := parseRouteCooldownDuration(cfg)

			// Load speed observations from the shared ddx-agent observations store.
			obs, _ := observations.LoadStore(observations.DefaultStorePath())

			// Determine which routes to show.
			var routeKeys []string
			if model != "" {
				if _, ok := cfg.GetModelRoute(model); ok {
					routeKeys = []string{model}
				} else {
					return fmt.Errorf("no route configured for model key %q — check .agent/config.yaml", model)
				}
			} else {
				routeKeys = cfg.ModelRouteNames()
				if len(routeKeys) == 0 {
					if !asJSON {
						fmt.Fprintln(cmd.OutOrStdout(), "No model routes configured in .agent/config.yaml.")
						fmt.Fprintln(cmd.OutOrStdout(), "Use --model <route-key> or configure model_routes in .agent/config.yaml.")
					} else {
						enc := json.NewEncoder(cmd.OutOrStdout())
						enc.SetIndent("", "  ")
						return enc.Encode(routeStatusJSON{})
					}
					return nil
				}
			}

			// Evaluate all route candidates.
			type evaluatedRoute struct {
				key        string
				strategy   string
				candidates []routeCandidateStatus
			}
			routes := make([]evaluatedRoute, 0, len(routeKeys))
			for _, key := range routeKeys {
				route, _ := cfg.GetModelRoute(key)
				healthState := loadAgentRouteHealthState(f.WorkingDir, key)
				strategy := route.Strategy
				if strategy == "" {
					strategy = "first-available"
				}
				candidates := make([]routeCandidateStatus, 0, len(route.Candidates))
				for _, candidate := range route.Candidates {
					rc := evalAgentRouteCandidate(cfg, candidate, obs, healthState, cooldown)
					candidates = append(candidates, rc)
				}
				routes = append(routes, evaluatedRoute{
					key:        key,
					strategy:   strategy,
					candidates: candidates,
				})
			}

			// Load recent routing decisions from two complementary sources:
			//
			//  1. RoutingMetricsStore — harness-level analytics (latency, success
			//     rate) for every agent run, stored in routing-outcomes.jsonl.
			//  2. kind:routing bead events — execution provenance per bead (which
			//     provider/model was selected and why), tied to a specific bead ID.
			//
			// Both are kept because they serve different purposes: RoutingMetricsStore
			// is for performance analytics across all runs; bead evidence links
			// routing decisions to the specific work item being processed.
			var recentDecisions []recentRoutingDecision

			r := f.agentRunner()
			if r.Config.SessionLogDir != "" {
				store := agent.NewRoutingMetricsStore(r.Config.SessionLogDir)
				outcomes, _ := store.ReadOutcomes()
				for _, o := range outcomes {
					recentDecisions = append(recentDecisions, recentRoutingDecision{
						ObservedAt:   o.ObservedAt,
						Source:       "metrics-store",
						Harness:      o.Harness,
						Provider:     o.CanonicalTarget,
						Model:        o.Model,
						Success:      o.Success,
						SuccessKnown: true,
						LatencyMS:    o.LatencyMS,
					})
				}
			}

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

			// Collect active cooldowns across all known routes.
			type cooldownEntry struct {
				route         string
				provider      string
				failedAt      time.Time
				cooldownUntil time.Time
			}
			var activeCooldowns []cooldownEntry
			seenCooldownKeys := make(map[string]struct{})
			for _, ev := range routes {
				healthState := loadAgentRouteHealthState(f.WorkingDir, ev.key)
				for provider, failedAt := range healthState.Failures {
					until := failedAt.Add(cooldown)
					if time.Now().Before(until) {
						ck := ev.key + "|" + provider
						if _, seen := seenCooldownKeys[ck]; !seen {
							seenCooldownKeys[ck] = struct{}{}
							activeCooldowns = append(activeCooldowns, cooldownEntry{
								route:         ev.key,
								provider:      provider,
								failedAt:      failedAt,
								cooldownUntil: until,
							})
						}
					}
				}
			}
			sort.Slice(activeCooldowns, func(i, j int) bool {
				return activeCooldowns[i].route < activeCooldowns[j].route
			})

			if asJSON {
				payload := routeStatusJSON{}
				for _, ev := range routes {
					entry := routeStatusRouteJSON{
						RouteKey: ev.key,
						Strategy: ev.strategy,
					}
					for _, rc := range ev.candidates {
						cj := routeStatusCandidateJSON{
							Provider:      rc.Provider,
							Model:         rc.Model,
							Healthy:       rc.Healthy,
							InCooldown:    rc.InCooldown,
							AvgDurationMs: rc.AvgLatencyMs,
							Reliability:   rc.Reliability,
							Score:         rc.Score,
							Reason:        rc.Reason,
						}
						if !rc.CooldownUntil.IsZero() {
							cj.CooldownUntil = rc.CooldownUntil.Format(time.RFC3339)
						}
						entry.Candidates = append(entry.Candidates, cj)
						if rc.Healthy && entry.SelectedProvider == "" {
							entry.SelectedProvider = rc.Provider
							entry.SelectedModel = rc.Model
						}
					}
					payload.Routes = append(payload.Routes, entry)
				}
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
					if d.Source == "metrics-store" {
						jd.CanonicalTarget = d.Provider
						jd.Success = d.Success
						jd.LatencyMs = d.LatencyMS
					}
					payload.RecentDecisions = append(payload.RecentDecisions, jd)
				}
				for _, c := range activeCooldowns {
					payload.ActiveCooldowns = append(payload.ActiveCooldowns, routeStatusCooldownJSON{
						Route:         c.route,
						Provider:      c.provider,
						FailedAt:      c.failedAt.UTC().Format(time.RFC3339),
						CooldownUntil: c.cooldownUntil.UTC().Format(time.RFC3339),
					})
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}

			out := cmd.OutOrStdout()

			// --- Section 1: Route Table ---
			for i, ev := range routes {
				if i > 0 {
					fmt.Fprintln(out)
				}

				selectedProvider := ""
				selectedModel := ""
				for _, rc := range ev.candidates {
					if rc.Healthy {
						selectedProvider = rc.Provider
						selectedModel = rc.Model
						break
					}
				}

				fmt.Fprintf(out, "Route: %s\n", ev.key)
				fmt.Fprintf(out, "Strategy: %s\n", ev.strategy)
				if selectedProvider != "" {
					fmt.Fprintf(out, "Selected: %s (%s)\n", selectedProvider, selectedModel)
				} else {
					fmt.Fprintf(out, "Selected: (none — all candidates down or in cooldown)\n")
				}

				fmt.Fprintf(out, "%-12s %-32s %-10s %-10s %-10s %-12s %s\n",
					"PROVIDER", "MODEL", "HEALTH", "SCORE", "RELIABILITY", "LATENCY_MS", "REASON")
				for _, rc := range ev.candidates {
					health := "down"
					if rc.Healthy {
						health = "healthy"
					} else if rc.InCooldown {
						health = "cooldown"
					}
					reason := rc.Reason
					if reason == "" {
						reason = rc.ProbeMsg
					}
					fmt.Fprintf(out, "%-12s %-32s %-10s %-10.3f %-10.2f %-12.0f %s\n",
						rc.Provider,
						truncateRouteStr(rc.Model, 32),
						health,
						rc.Score,
						rc.Reliability,
						rc.AvgLatencyMs,
						reason,
					)
				}
			}

			// --- Section 2: Recent Routing Decisions ---
			// Combines metrics-store outcomes (latency/success) and bead-evidence
			// entries (provider/model/reason tied to a specific bead), sorted by time.
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
				fmt.Fprintf(out, "%-20s %-12s %-24s %s\n",
					"ROUTE", "PROVIDER", "FAILED_AT", "COOLDOWN_UNTIL")
				for _, c := range activeCooldowns {
					fmt.Fprintf(out, "%-20s %-12s %-24s %s\n",
						truncateRouteStr(c.route, 20),
						c.provider,
						c.failedAt.UTC().Format("2006-01-02T15:04:05Z"),
						c.cooldownUntil.UTC().Format("2006-01-02T15:04:05Z"),
					)
				}
			}

			return nil
		},
	}
	cmd.Flags().String("model", "", "Requested model route key (e.g. qwen3.5-27b)")
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}
