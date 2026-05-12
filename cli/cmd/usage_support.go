package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// usageRow holds aggregated stats for a single harness.
type usageRow struct {
	Harness                string  `json:"harness" yaml:"harness"`
	Sessions               int     `json:"sessions" yaml:"sessions"`
	InputTokens            int     `json:"input_tokens" yaml:"input_tokens"`
	OutputTokens           int     `json:"output_tokens" yaml:"output_tokens"`
	CostUSD                float64 `json:"cost_usd" yaml:"cost_usd"`
	CostBasis              string  `json:"cost_basis,omitempty" yaml:"cost_basis,omitempty"`
	AvgDurationMS          float64 `json:"avg_duration_ms" yaml:"avg_duration_ms"`
	QuotaState             string  `json:"quota_state,omitempty" yaml:"quota_state,omitempty"`
	SignalProvider         string  `json:"signal_provider,omitempty" yaml:"signal_provider,omitempty"`
	SignalKind             string  `json:"signal_kind,omitempty" yaml:"signal_kind,omitempty"`
	SignalFreshness        string  `json:"signal_freshness,omitempty" yaml:"signal_freshness,omitempty"`
	SignalBasis            string  `json:"signal_basis,omitempty" yaml:"signal_basis,omitempty"`
	NativeInputTokens      int     `json:"native_input_tokens,omitempty" yaml:"native_input_tokens,omitempty"`
	NativeOutputTokens     int     `json:"native_output_tokens,omitempty" yaml:"native_output_tokens,omitempty"`
	NativeTotalTokens      int     `json:"native_total_tokens,omitempty" yaml:"native_total_tokens,omitempty"`
	NativeSessionCount     int     `json:"native_session_count,omitempty" yaml:"native_session_count,omitempty"`
	NativeQuotaUsedPercent int     `json:"native_quota_used_percent,omitempty" yaml:"native_quota_used_percent,omitempty"`
}

const (
	usageCostBasisReported       = "reported"
	usageCostBasisEstimated      = "estimated"
	usageCostBasisEstimatedValue = "estimated_value"
	usageCostBasisMixed          = "mixed"
)

type usageAgg struct {
	sessions     int
	inputTokens  int
	outputTokens int
	costUSD      float64
	totalDurMS   int
	costBasis    string
}

type usageSessionRecord struct {
	entry agent.SessionIndexEntry
}

func (a *usageAgg) addSession(entry agent.SessionIndexEntry) {
	a.sessions++
	a.inputTokens += entry.InputTokens
	a.outputTokens += entry.OutputTokens
	a.totalDurMS += entry.DurationMS
	if entry.CostUSD > 0 {
		a.costUSD += entry.CostUSD
		a.mergeCostBasis(usageCostBasisReported)
		return
	}
	if entry.Model == "" {
		return
	}
	if est := agent.EstimateCost(entry.Model, entry.InputTokens, entry.OutputTokens); est >= 0 {
		a.costUSD += est
		if est > 0 {
			a.mergeCostBasis(usageCostBasisEstimated)
		}
	}
}

func (a *usageAgg) toRow(harness string) usageRow {
	var avgDur float64
	if a.sessions > 0 {
		avgDur = float64(a.totalDurMS) / float64(a.sessions)
	}
	row := usageRow{
		Harness:       harness,
		Sessions:      a.sessions,
		InputTokens:   a.inputTokens,
		OutputTokens:  a.outputTokens,
		CostUSD:       a.costUSD,
		CostBasis:     inferredUsageCostBasis(harness, a.costUSD, a.costBasis),
		AvgDurationMS: avgDur,
	}
	applyUsageCostBasis(&row, isSubscriptionHarnessName(harness))
	return row
}

func (a *usageAgg) mergeCostBasis(basis string) {
	if basis == "" {
		return
	}
	if a.costBasis == "" {
		a.costBasis = basis
		return
	}
	if a.costBasis != basis {
		a.costBasis = usageCostBasisMixed
	}
}

func inferredUsageCostBasis(harness string, costUSD float64, basis string) string {
	if costUSD <= 0 {
		return ""
	}
	if isSubscriptionHarnessName(harness) {
		return usageCostBasisEstimatedValue
	}
	if basis != "" {
		return basis
	}
	return usageCostBasisEstimated
}

func isSubscriptionHarnessName(harness string) bool {
	switch strings.ToLower(harness) {
	case "claude", "codex":
		return true
	default:
		return false
	}
}

func applyUsageCostBasis(row *usageRow, isSubscription bool) {
	if row == nil || row.CostUSD <= 0 {
		return
	}
	if isSubscription {
		row.CostBasis = usageCostBasisEstimatedValue
		return
	}
	if row.CostBasis == "" {
		row.CostBasis = usageCostBasisEstimated
	}
}

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
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("unrecognized format %q, want today, Nd, or YYYY-MM-DD", s)
	}
	return t, nil
}

func aggregateUsageFromSessionIndex(logDir, harnessFilter string, since time.Time) ([]usageRow, error) {
	records, err := readUsageSessionIndexRecords(logDir, harnessFilter, since)
	if err != nil {
		return nil, err
	}
	byHarness := map[string]*usageAgg{}
	order := []string{}
	for _, record := range records {
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

func enrichUsageRowsWithRoutingSignals(workDir string, rows []usageRow) []usageRow {
	return rows
}

func readUsageSessionIndexRecords(logDir, harnessFilter string, since time.Time) ([]usageSessionRecord, error) {
	entries, err := agent.ReadSessionIndex(logDir, agent.SessionIndexQuery{StartedAfter: &since})
	if err != nil {
		return nil, err
	}
	records := make([]usageSessionRecord, 0, len(entries))
	for _, idx := range entries {
		if harnessFilter != "" && idx.Harness != harnessFilter {
			continue
		}
		records = append(records, usageSessionRecord{entry: idx})
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].entry.StartedAt.Before(records[j].entry.StartedAt)
	})
	return records, nil
}

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
