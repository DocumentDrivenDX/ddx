package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/spf13/cobra"
)

// usageRow holds aggregated stats for a single harness.
type usageRow struct {
	Harness                string  `json:"harness"`
	Sessions               int     `json:"sessions"`
	InputTokens            int     `json:"input_tokens"`
	OutputTokens           int     `json:"output_tokens"`
	CostUSD                float64 `json:"cost_usd"`
	AvgDurationMS          float64 `json:"avg_duration_ms"`
	QuotaState             string  `json:"quota_state,omitempty"`
	SignalProvider         string  `json:"signal_provider,omitempty"`
	SignalKind             string  `json:"signal_kind,omitempty"`
	SignalFreshness        string  `json:"signal_freshness,omitempty"`
	SignalBasis            string  `json:"signal_basis,omitempty"`
	NativeInputTokens      int     `json:"native_input_tokens,omitempty"`
	NativeOutputTokens     int     `json:"native_output_tokens,omitempty"`
	NativeTotalTokens      int     `json:"native_total_tokens,omitempty"`
	NativeSessionCount     int     `json:"native_session_count,omitempty"`
	NativeQuotaUsedPercent int     `json:"native_quota_used_percent,omitempty"`
}

type usageAgg struct {
	sessions     int
	inputTokens  int
	outputTokens int
	costUSD      float64
	totalDurMS   int
}

type usageSessionRecord struct {
	entry agent.SessionEntry
	key   string
}

func (a *usageAgg) addSession(entry agent.SessionEntry) {
	a.sessions++
	a.inputTokens += entry.InputTokens
	a.outputTokens += entry.OutputTokens
	a.totalDurMS += entry.Duration

	// Use recorded cost if available, else estimate from pricing table.
	if entry.CostUSD > 0 {
		a.costUSD += entry.CostUSD
		return
	}
	if entry.Model == "" {
		return
	}
	if est := agent.EstimateCost(entry.Model, entry.InputTokens, entry.OutputTokens); est >= 0 {
		a.costUSD += est
	}
}

func (a *usageAgg) addOutcome(outcome agent.RoutingOutcome, registry *agent.Registry) {
	a.sessions++
	a.inputTokens += outcome.InputTokens
	a.outputTokens += outcome.OutputTokens
	a.totalDurMS += outcome.LatencyMS
	if outcome.CostUSD > 0 {
		a.costUSD += outcome.CostUSD
		return
	}
	if registry == nil {
		return
	}
	if harness, ok := registry.Get(outcome.Harness); ok && !harness.IsLocal && outcome.Model != "" {
		if est := agent.EstimateCost(outcome.Model, outcome.InputTokens, outcome.OutputTokens); est >= 0 {
			a.costUSD += est
		}
	}
}

func (a *usageAgg) toRow(harness string) usageRow {
	var avgDur float64
	if a.sessions > 0 {
		avgDur = float64(a.totalDurMS) / float64(a.sessions)
	}
	return usageRow{
		Harness:       harness,
		Sessions:      a.sessions,
		InputTokens:   a.inputTokens,
		OutputTokens:  a.outputTokens,
		CostUSD:       a.costUSD,
		AvgDurationMS: avgDur,
	}
}

func (f *CommandFactory) newAgentUsageCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Show per-harness token and cost summary",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			harness, _ := cmd.Flags().GetString("harness")
			since, _ := cmd.Flags().GetString("since")
			format, _ := cmd.Flags().GetString("format")

			sinceTime, err := parseSince(since)
			if err != nil {
				return fmt.Errorf("invalid --since value: %w", err)
			}

			r := f.agentRunner()
			logDir := r.Config.SessionLogDir
			if logDir != "" && !filepath.IsAbs(logDir) {
				logDir = filepath.Join(f.WorkingDir, logDir)
			}
			rows, err := aggregateUsageFromRoutingMetrics(logDir, harness, sinceTime)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				logFile := filepath.Join(logDir, "sessions.jsonl")
				rows, err = aggregateUsage(logFile, harness, sinceTime, nil)
			}
			if err != nil {
				return err
			}
			rows = enrichUsageRowsWithRoutingSignals(r, rows)

			switch format {
			case "json":
				return renderUsageJSON(cmd, rows)
			case "csv":
				return renderUsageCSV(cmd, rows)
			default:
				return renderUsageTable(cmd, rows)
			}
		},
	}

	cmd.Flags().String("harness", "", "Filter by harness name")
	cmd.Flags().String("since", "30d", "Time window: today, 7d, 30d, or ISO date (e.g. 2026-04-01)")
	cmd.Flags().String("format", "table", "Output format: table, json, csv")
	return cmd
}

// parseSince converts a --since string to a time.Time cutoff.
func parseSince(s string) (time.Time, error) {
	switch s {
	case "today":
		now := time.Now()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), nil
	case "":
		return time.Now().AddDate(0, 0, -30), nil
	}
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return time.Time{}, fmt.Errorf("expected Nd format, got %q", s)
		}
		return time.Now().AddDate(0, 0, -n), nil
	}
	// Try ISO date
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("unrecognized format %q, want today, Nd, or YYYY-MM-DD", s)
	}
	return t, nil
}

// aggregateUsage reads sessions.jsonl and returns per-harness aggregates.
func aggregateUsage(logFile, harnessFilter string, since time.Time, mirrored map[string]struct{}) ([]usageRow, error) {
	byHarness, order, err := aggregateUsageAggs(logFile, harnessFilter, since, nil, mirrored)
	if err != nil {
		return nil, err
	}

	rows := make([]usageRow, 0, len(order))
	for _, h := range order {
		rows = append(rows, byHarness[h].toRow(h))
	}
	return rows, nil
}

// aggregateUsageAggs is the compatibility-aware session aggregator used by
// both the primary routing-store path and the legacy fallback path.
func aggregateUsageAggs(logFile, harnessFilter string, since time.Time, cutoffByHarness map[string]time.Time, mirrored map[string]struct{}) (map[string]*usageAgg, []string, error) {
	byHarness := map[string]*usageAgg{}
	order := []string{}

	err := agent.ForEachJSONL[agent.SessionEntry](logFile, func(entry agent.SessionEntry) error {
		if entry.Timestamp.Before(since) {
			return nil
		}
		if mirrored != nil {
			if key := usageMirrorKey(entry.NativeSessionID, entry.TraceID, entry.SpanID); key != "" {
				if _, ok := mirrored[key]; ok {
					return nil
				}
			}
		}
		if cutoffByHarness != nil {
			if cutoff, ok := cutoffByHarness[entry.Harness]; ok && !cutoff.IsZero() && !entry.Timestamp.Before(cutoff) {
				return nil
			}
		}
		if harnessFilter != "" && entry.Harness != harnessFilter {
			return nil
		}

		a, exists := byHarness[entry.Harness]
		if !exists {
			a = &usageAgg{}
			byHarness[entry.Harness] = a
			order = append(order, entry.Harness)
		}
		a.addSession(entry)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return byHarness, order, nil
}

// aggregateUsageFromRoutingMetrics prefers the minimal routing-outcome store
// and supplements it with legacy session rows that predate the first mirrored
// routing sample for each harness so mixed stores remain backward compatible
// without double counting current runs.
func aggregateUsageFromRoutingMetrics(logDir, harnessFilter string, since time.Time) ([]usageRow, error) {
	store := agent.NewRoutingMetricsStore(logDir)
	outcomes, err := store.ReadOutcomes()
	if err != nil {
		return nil, err
	}
	if len(outcomes) == 0 {
		return nil, nil
	}

	byHarness := map[string]*usageAgg{}
	order := []string{}
	mirrored := map[string]struct{}{}
	cutoffByHarness := map[string]time.Time{}
	registry := agent.NewRegistry()

	for _, outcome := range outcomes {
		if key := usageMirrorKey(outcome.NativeSessionID, outcome.TraceID, outcome.SpanID); key != "" {
			mirrored[key] = struct{}{}
		}
		if cutoff, ok := cutoffByHarness[outcome.Harness]; !ok || outcome.ObservedAt.Before(cutoff) {
			cutoffByHarness[outcome.Harness] = outcome.ObservedAt
		}
		if outcome.ObservedAt.Before(since) {
			continue
		}
		if harnessFilter != "" && outcome.Harness != harnessFilter {
			continue
		}

		a, exists := byHarness[outcome.Harness]
		if !exists {
			a = &usageAgg{}
			byHarness[outcome.Harness] = a
			order = append(order, outcome.Harness)
		}
		a.addOutcome(outcome, registry)
	}

	if len(order) == 0 {
		return nil, nil
	}

	sessionRecords, err := readUsageSessionRecords(filepath.Join(logDir, "sessions.jsonl"), harnessFilter, since)
	if err != nil {
		return nil, err
	}

	lastIndexByHarness := map[string]int{}
	for i, record := range sessionRecords {
		lastIndexByHarness[record.entry.Harness] = i
	}

	skipCurrentRun := map[int]struct{}{}
	for harness, idx := range lastIndexByHarness {
		if _, ok := byHarness[harness]; !ok {
			continue
		}
		if sessionRecords[idx].key == "" {
			skipCurrentRun[idx] = struct{}{}
		}
	}

	for i, record := range sessionRecords {
		if _, ok := skipCurrentRun[i]; ok {
			continue
		}
		if record.key != "" {
			if _, ok := mirrored[record.key]; ok {
				continue
			}
		}
		if cutoff, ok := cutoffByHarness[record.entry.Harness]; ok && !cutoff.IsZero() && !record.entry.Timestamp.Before(cutoff) {
			continue
		}

		a, exists := byHarness[record.entry.Harness]
		if !exists {
			a = &usageAgg{}
			byHarness[record.entry.Harness] = a
			order = append(order, record.entry.Harness)
		}
		a.addSession(record.entry)
	}

	rows := make([]usageRow, 0, len(order))
	for _, h := range order {
		rows = append(rows, byHarness[h].toRow(h))
	}
	return rows, nil
}

func enrichUsageRowsWithRoutingSignals(r *agent.Runner, rows []usageRow) []usageRow {
	if r == nil {
		return rows
	}
	for i := range rows {
		state := r.ProbeHarnessState(rows[i].Harness, 2*time.Second)
		rows[i].QuotaState = state.QuotaState
		if state.RoutingSignal == nil {
			continue
		}
		signal := state.RoutingSignal
		rows[i].SignalProvider = signal.Provider
		rows[i].SignalKind = signal.Source.Kind
		rows[i].SignalFreshness = signal.Source.Freshness
		rows[i].SignalBasis = signal.Source.Basis
		rows[i].NativeInputTokens = signal.HistoricalUsage.InputTokens
		rows[i].NativeOutputTokens = signal.HistoricalUsage.OutputTokens
		rows[i].NativeTotalTokens = signal.HistoricalUsage.TotalTokens
		rows[i].NativeSessionCount = signal.HistoricalUsage.SessionCount
		rows[i].NativeQuotaUsedPercent = signal.CurrentQuota.UsedPercent
	}
	return rows
}

func usageMirrorKey(nativeSessionID, traceID, spanID string) string {
	switch {
	case nativeSessionID != "":
		return "native:" + nativeSessionID
	case traceID != "":
		return "trace:" + traceID
	case spanID != "":
		return "span:" + spanID
	default:
		return ""
	}
}

func readUsageSessionRecords(logFile, harnessFilter string, since time.Time) ([]usageSessionRecord, error) {
	var records []usageSessionRecord
	err := agent.ForEachJSONL[agent.SessionEntry](logFile, func(entry agent.SessionEntry) error {
		if entry.Timestamp.Before(since) {
			return nil
		}
		if harnessFilter != "" && entry.Harness != harnessFilter {
			return nil
		}
		records = append(records, usageSessionRecord{
			entry: entry,
			key:   usageMirrorKey(entry.NativeSessionID, entry.TraceID, entry.SpanID),
		})
		return nil
	})
	return records, err
}

// formatComma formats an integer with comma separators.
func formatComma(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	start := len(s) % 3
	b.WriteString(s[:start])
	for i := start; i < len(s); i += 3 {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func renderUsageTable(cmd *cobra.Command, rows []usageRow) error {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%-12s  %8s  %13s  %14s  %10s  %12s  %-8s  %-18s  %-10s\n",
		"HARNESS", "SESSIONS", "INPUT TOKENS", "OUTPUT TOKENS", "EST. COST", "AVG DURATION", "QUOTA", "SOURCE", "FRESHNESS")

	var totalSessions int
	var totalInput, totalOutput int
	var totalCost float64
	var totalDurMS float64

	for _, r := range rows {
		source := r.SignalProvider
		if r.SignalKind != "" {
			if source != "" {
				source += "/"
			}
			source += r.SignalKind
		}
		fmt.Fprintf(out, "%-12s  %8d  %13s  %14s  %10s  %11.1fs  %-8s  %-18s  %-10s\n",
			r.Harness,
			r.Sessions,
			formatComma(r.InputTokens),
			formatComma(r.OutputTokens),
			fmt.Sprintf("$%.2f", r.CostUSD),
			r.AvgDurationMS/1000.0,
			r.QuotaState,
			source,
			r.SignalFreshness,
		)
		totalSessions += r.Sessions
		totalInput += r.InputTokens
		totalOutput += r.OutputTokens
		totalCost += r.CostUSD
		totalDurMS += r.AvgDurationMS * float64(r.Sessions)
	}

	var avgTotal float64
	if totalSessions > 0 {
		avgTotal = totalDurMS / float64(totalSessions)
	}
	fmt.Fprintf(out, "%-12s  %8d  %13s  %14s  %10s  %11.1fs\n",
		"TOTAL",
		totalSessions,
		formatComma(totalInput),
		formatComma(totalOutput),
		fmt.Sprintf("$%.2f", totalCost),
		avgTotal/1000.0,
	)
	return nil
}

func renderUsageJSON(cmd *cobra.Command, rows []usageRow) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

func renderUsageCSV(cmd *cobra.Command, rows []usageRow) error {
	w := csv.NewWriter(cmd.OutOrStdout())
	_ = w.Write([]string{"harness", "sessions", "input_tokens", "output_tokens", "cost_usd", "avg_duration_ms", "quota_state", "signal_provider", "signal_kind", "signal_freshness", "native_input_tokens", "native_output_tokens", "native_total_tokens", "native_session_count", "native_quota_used_percent"})
	for _, r := range rows {
		_ = w.Write([]string{
			r.Harness,
			strconv.Itoa(r.Sessions),
			strconv.Itoa(r.InputTokens),
			strconv.Itoa(r.OutputTokens),
			fmt.Sprintf("%.4f", r.CostUSD),
			fmt.Sprintf("%.1f", r.AvgDurationMS),
			r.QuotaState,
			r.SignalProvider,
			r.SignalKind,
			r.SignalFreshness,
			strconv.Itoa(r.NativeInputTokens),
			strconv.Itoa(r.NativeOutputTokens),
			strconv.Itoa(r.NativeTotalTokens),
			strconv.Itoa(r.NativeSessionCount),
			strconv.Itoa(r.NativeQuotaUsedPercent),
		})
	}
	w.Flush()
	return w.Error()
}
