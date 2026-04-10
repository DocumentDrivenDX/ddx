package cmd

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/spf13/cobra"
)

// usageRow holds aggregated stats for a single harness.
type usageRow struct {
	Harness       string  `json:"harness"`
	Sessions      int     `json:"sessions"`
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	CostUSD       float64 `json:"cost_usd"`
	AvgDurationMS float64 `json:"avg_duration_ms"`
}

type usageAgg struct {
	sessions     int
	inputTokens  int
	outputTokens int
	costUSD      float64
	totalDurMS   int
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
			rows, err := aggregateUsageFromRoutingMetrics(r.Config.SessionLogDir, harness, sinceTime)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				logFile := filepath.Join(r.Config.SessionLogDir, "sessions.jsonl")
				rows, err = aggregateUsage(logFile, harness, sinceTime, time.Time{}, nil)
			}
			if err != nil {
				return err
			}

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
func aggregateUsage(logFile, harnessFilter string, since time.Time, before time.Time, mirrored map[string]struct{}) ([]usageRow, error) {
	byHarness, order, err := aggregateUsageAggs(logFile, harnessFilter, since, before, mirrored)
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
func aggregateUsageAggs(logFile, harnessFilter string, since time.Time, before time.Time, mirrored map[string]struct{}) (map[string]*usageAgg, []string, error) {
	f, err := os.Open(logFile)
	if os.IsNotExist(err) {
		return map[string]*usageAgg{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	byHarness := map[string]*usageAgg{}
	order := []string{}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry agent.SessionEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Timestamp.Before(since) {
			continue
		}
		if mirrored != nil {
			if key := usageMirrorKey(entry.NativeSessionID, entry.TraceID, entry.SpanID); key != "" {
				if _, ok := mirrored[key]; ok {
					continue
				}
			}
		}
		if !before.IsZero() && !entry.Timestamp.Before(before) {
			continue
		}
		if harnessFilter != "" && entry.Harness != harnessFilter {
			continue
		}

		a, exists := byHarness[entry.Harness]
		if !exists {
			a = &usageAgg{}
			byHarness[entry.Harness] = a
			order = append(order, entry.Harness)
		}
		a.addSession(entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	return byHarness, order, nil
}

// aggregateUsageFromRoutingMetrics prefers the minimal routing-outcome store
// and supplements it with legacy session rows that predate the first mirrored
// routing sample so mixed stores remain backward compatible without double
// counting current runs.
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
	migrationCutoff := time.Time{}
	registry := agent.NewRegistry()

	for _, outcome := range outcomes {
		if key := usageMirrorKey(outcome.NativeSessionID, outcome.TraceID, outcome.SpanID); key != "" {
			mirrored[key] = struct{}{}
		}
		if migrationCutoff.IsZero() || outcome.ObservedAt.Before(migrationCutoff) {
			migrationCutoff = outcome.ObservedAt
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

	sessionAggs, sessionOrder, err := aggregateUsageAggs(filepath.Join(logDir, "sessions.jsonl"), harnessFilter, since, migrationCutoff, mirrored)
	if err != nil {
		return nil, err
	}

	for _, h := range sessionOrder {
		row := sessionAggs[h]
		a, exists := byHarness[h]
		if !exists {
			byHarness[h] = row
			order = append(order, h)
			continue
		}
		a.sessions += row.sessions
		a.inputTokens += row.inputTokens
		a.outputTokens += row.outputTokens
		a.costUSD += row.costUSD
		a.totalDurMS += row.totalDurMS
	}

	rows := make([]usageRow, 0, len(order))
	for _, h := range order {
		rows = append(rows, byHarness[h].toRow(h))
	}
	return rows, nil
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
	fmt.Fprintf(out, "%-12s  %8s  %13s  %14s  %10s  %12s\n",
		"HARNESS", "SESSIONS", "INPUT TOKENS", "OUTPUT TOKENS", "EST. COST", "AVG DURATION")

	var totalSessions int
	var totalInput, totalOutput int
	var totalCost float64
	var totalDurMS float64

	for _, r := range rows {
		fmt.Fprintf(out, "%-12s  %8d  %13s  %14s  %10s  %11.1fs\n",
			r.Harness,
			r.Sessions,
			formatComma(r.InputTokens),
			formatComma(r.OutputTokens),
			fmt.Sprintf("$%.2f", r.CostUSD),
			r.AvgDurationMS/1000.0,
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
	_ = w.Write([]string{"harness", "sessions", "input_tokens", "output_tokens", "cost_usd", "avg_duration_ms"})
	for _, r := range rows {
		_ = w.Write([]string{
			r.Harness,
			strconv.Itoa(r.Sessions),
			strconv.Itoa(r.InputTokens),
			strconv.Itoa(r.OutputTokens),
			fmt.Sprintf("%.4f", r.CostUSD),
			fmt.Sprintf("%.1f", r.AvgDurationMS),
		})
	}
	w.Flush()
	return w.Error()
}
