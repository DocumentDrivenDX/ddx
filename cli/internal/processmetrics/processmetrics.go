package processmetrics

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

const (
	stateKnown     = "known"
	stateEstimated = "estimated"
	stateUnknown   = "unknown"
)

// Service derives process metrics from bead and session records on demand.
type Service struct {
	WorkingDir string

	store *bead.Store
}

// New creates a process-metrics service rooted at workingDir.
func New(workingDir string) *Service {
	return &Service{WorkingDir: workingDir}
}

// ParseSince converts a CLI/query window string into a cutoff time.
// Empty input returns the zero time and no error.
func ParseSince(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}

	now := time.Now()
	switch raw {
	case "today":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), nil
	case "now":
		return now, nil
	}

	if strings.HasSuffix(raw, "d") {
		n, err := parsePositiveInt(strings.TrimSuffix(raw, "d"))
		if err != nil {
			return time.Time{}, fmt.Errorf("expected Nd window, got %q", raw)
		}
		return now.AddDate(0, 0, -n), nil
	}

	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("unrecognized window %q, want today, Nd, RFC3339, or YYYY-MM-DD", raw)
}

// Query configures a cost or lifecycle window.
type Query struct {
	Since     time.Time
	HasSince  bool
	BeadID    string
	FeatureID string
}

// State tracks whether a derived value is known, estimated, or unknown.
type State string

// AggregateSummary is the dashboard payload returned by summary.
type AggregateSummary struct {
	Beads struct {
		Total            int `json:"total"`
		Open             int `json:"open"`
		InProgress       int `json:"in_progress"`
		Closed           int `json:"closed"`
		Reopened         int `json:"reopened"`
		KnownCycleTime   int `json:"known_cycle_time"`
		UnknownCycleTime int `json:"unknown_cycle_time"`
		KnownCost        int `json:"known_cost"`
		EstimatedCost    int `json:"estimated_cost"`
		UnknownCost      int `json:"unknown_cost"`
	} `json:"beads"`

	Sessions struct {
		Total         int     `json:"total"`
		Correlated    int     `json:"correlated"`
		Uncorrelated  int     `json:"uncorrelated"`
		InputTokens   int     `json:"input_tokens"`
		OutputTokens  int     `json:"output_tokens"`
		TotalTokens   int     `json:"total_tokens"`
		KnownCost     int     `json:"known_cost"`
		EstimatedCost int     `json:"estimated_cost"`
		UnknownCost   int     `json:"unknown_cost"`
		CostUSD       float64 `json:"cost_usd"`
	} `json:"sessions"`

	Cost struct {
		Beads            int     `json:"beads"`
		Features         int     `json:"features"`
		KnownCostUSD     float64 `json:"known_cost_usd"`
		EstimatedCostUSD float64 `json:"estimated_cost_usd"`
		UnknownBeads     int     `json:"unknown_beads"`
	} `json:"cost"`

	CycleTime struct {
		KnownCount   int    `json:"known_count"`
		UnknownCount int    `json:"unknown_count"`
		AverageMS    *int64 `json:"average_ms,omitempty"`
		MinMS        *int64 `json:"min_ms,omitempty"`
		MaxMS        *int64 `json:"max_ms,omitempty"`
	} `json:"cycle_time"`

	Rework struct {
		KnownClosed   int     `json:"known_closed"`
		KnownReopened int     `json:"known_reopened"`
		UnknownCount  int     `json:"unknown_count"`
		ReopenRate    float64 `json:"reopen_rate"`
		RevisionCount int     `json:"revision_count"`
	} `json:"rework"`
}

// CostReport describes bead and feature cost attribution.
type CostReport struct {
	Scope     string           `json:"scope"`
	Window    Window           `json:"window,omitempty"`
	BeadID    string           `json:"bead_id,omitempty"`
	FeatureID string           `json:"feature_id,omitempty"`
	Beads     []BeadCostRow    `json:"beads,omitempty"`
	Features  []FeatureCostRow `json:"features,omitempty"`
	Summary   CostSummary      `json:"summary"`
}

// CostSummary aggregates cost rows.
type CostSummary struct {
	Beads            int     `json:"beads"`
	Features         int     `json:"features"`
	KnownCostUSD     float64 `json:"known_cost_usd"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	UnknownBeads     int     `json:"unknown_beads"`
}

// BeadCostRow is one bead's cost attribution.
type BeadCostRow struct {
	BeadID          string      `json:"bead_id"`
	Title           string      `json:"title"`
	SpecID          string      `json:"spec_id,omitempty"`
	Status          string      `json:"status"`
	SessionIDs      []string    `json:"session_ids,omitempty"`
	InputTokens     int         `json:"input_tokens"`
	OutputTokens    int         `json:"output_tokens"`
	TotalTokens     int         `json:"total_tokens"`
	CostState       State       `json:"cost_state"`
	CostUSD         *float64    `json:"cost_usd,omitempty"`
	UnknownSessions int         `json:"unknown_sessions,omitempty"`
	Provenance      []SourceRef `json:"provenance,omitempty"`
}

// FeatureCostRow aggregates bead rows by spec-id.
type FeatureCostRow struct {
	SpecID       string   `json:"spec_id"`
	BeadIDs      []string `json:"bead_ids,omitempty"`
	InputTokens  int      `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens"`
	TotalTokens  int      `json:"total_tokens"`
	CostState    State    `json:"cost_state"`
	CostUSD      *float64 `json:"cost_usd,omitempty"`
	UnknownBeads int      `json:"unknown_beads,omitempty"`
}

// CycleTimeReport describes lifecycle timing facts.
type CycleTimeReport struct {
	Scope   string           `json:"scope"`
	Window  Window           `json:"window,omitempty"`
	Beads   []CycleTimeRow   `json:"beads,omitempty"`
	Summary CycleTimeSummary `json:"summary"`
}

// CycleTimeSummary aggregates lifecycle facts.
type CycleTimeSummary struct {
	KnownCount   int    `json:"known_count"`
	UnknownCount int    `json:"unknown_count"`
	AverageMS    *int64 `json:"average_ms,omitempty"`
	MinMS        *int64 `json:"min_ms,omitempty"`
	MaxMS        *int64 `json:"max_ms,omitempty"`
}

// CycleTimeRow describes one bead's lifecycle timing.
type CycleTimeRow struct {
	BeadID           string      `json:"bead_id"`
	Title            string      `json:"title"`
	SpecID           string      `json:"spec_id,omitempty"`
	Status           string      `json:"status"`
	CreatedAt        time.Time   `json:"created_at"`
	FirstClosedAt    *time.Time  `json:"first_closed_at,omitempty"`
	LastClosedAt     *time.Time  `json:"last_closed_at,omitempty"`
	CycleTimeMS      *int64      `json:"cycle_time_ms,omitempty"`
	ReopenCount      *int        `json:"reopen_count,omitempty"`
	RevisionCount    *int        `json:"revision_count,omitempty"`
	TimeInOpenMS     *int64      `json:"time_in_open_ms,omitempty"`
	TimeInProgressMS *int64      `json:"time_in_in_progress_ms,omitempty"`
	TimeInClosedMS   *int64      `json:"time_in_closed_ms,omitempty"`
	CycleState       State       `json:"cycle_state"`
	Provenance       []SourceRef `json:"provenance,omitempty"`
}

// ReworkReport describes reopen and post-close churn.
type ReworkReport struct {
	Scope   string        `json:"scope"`
	Window  Window        `json:"window,omitempty"`
	Beads   []ReworkRow   `json:"beads,omitempty"`
	Summary ReworkSummary `json:"summary"`
}

// ReworkSummary aggregates rework facts.
type ReworkSummary struct {
	KnownClosed   int     `json:"known_closed"`
	KnownReopened int     `json:"known_reopened"`
	UnknownCount  int     `json:"unknown_count"`
	ReopenRate    float64 `json:"reopen_rate"`
	RevisionCount int     `json:"revision_count"`
}

// ReworkRow describes reopen and revision facts for one bead.
type ReworkRow struct {
	BeadID        string      `json:"bead_id"`
	Title         string      `json:"title"`
	SpecID        string      `json:"spec_id,omitempty"`
	Status        string      `json:"status"`
	FirstClosedAt *time.Time  `json:"first_closed_at,omitempty"`
	Reopened      bool        `json:"reopened"`
	ReopenCount   *int        `json:"reopen_count,omitempty"`
	RevisionCount *int        `json:"revision_count,omitempty"`
	ReworkState   State       `json:"rework_state"`
	Provenance    []SourceRef `json:"provenance,omitempty"`
}

// Window captures the query cutoff used to derive a report.
type Window struct {
	Since *time.Time `json:"since,omitempty"`
	Label string     `json:"label,omitempty"`
}

// SourceRef explains where a derived fact came from.
type SourceRef struct {
	BeadID           string     `json:"bead_id,omitempty"`
	SessionID        string     `json:"session_id,omitempty"`
	NativeSessionID  string     `json:"native_session_id,omitempty"`
	NativeLogRef     string     `json:"native_log_ref,omitempty"`
	ClosingCommitSHA string     `json:"closing_commit_sha,omitempty"`
	Timestamp        *time.Time `json:"timestamp,omitempty"`
	Source           string     `json:"source,omitempty"`
}

func (s *Service) Summary(query Query) (AggregateSummary, error) {
	beads, sessions, _, err := s.loadInputs()
	if err != nil {
		return AggregateSummary{}, err
	}

	costReport, err := s.Cost(query)
	if err != nil {
		return AggregateSummary{}, err
	}
	cycleReport, err := s.CycleTime(query)
	if err != nil {
		return AggregateSummary{}, err
	}
	reworkReport, err := s.Rework(query)
	if err != nil {
		return AggregateSummary{}, err
	}

	var summary AggregateSummary
	visibleBeads, visibleBeadIDs := beadsVisibleInSummaryWindow(beads, query)
	summary.Beads.Total = len(visibleBeads)

	for _, b := range visibleBeads {
		switch b.Status {
		case bead.StatusOpen:
			summary.Beads.Open++
		case bead.StatusInProgress:
			summary.Beads.InProgress++
		case bead.StatusClosed:
			summary.Beads.Closed++
		}
	}
	var cycleKnownTotalMS int64
	for _, row := range cycleReport.Beads {
		if query.HasSince {
			if _, ok := visibleBeadIDs[row.BeadID]; !ok {
				continue
			}
		}
		if row.CycleState == stateKnown {
			summary.Beads.KnownCycleTime++
			summary.CycleTime.KnownCount++
			if row.CycleTimeMS != nil {
				cycleKnownTotalMS += *row.CycleTimeMS
				if summary.CycleTime.MinMS == nil || *row.CycleTimeMS < *summary.CycleTime.MinMS {
					summary.CycleTime.MinMS = int64Ptr(*row.CycleTimeMS)
				}
				if summary.CycleTime.MaxMS == nil || *row.CycleTimeMS > *summary.CycleTime.MaxMS {
					summary.CycleTime.MaxMS = int64Ptr(*row.CycleTimeMS)
				}
			}
		} else {
			summary.Beads.UnknownCycleTime++
			summary.CycleTime.UnknownCount++
		}
	}
	if summary.CycleTime.KnownCount > 0 {
		avg := cycleKnownTotalMS / int64(summary.CycleTime.KnownCount)
		summary.CycleTime.AverageMS = &avg
	}

	costBeadCount := 0
	featureIDs := make(map[string]struct{})
	for _, row := range costReport.Beads {
		if query.HasSince {
			if _, ok := visibleBeadIDs[row.BeadID]; !ok && len(row.SessionIDs) == 0 {
				continue
			}
		}
		costBeadCount++
		if row.SpecID != "" {
			featureIDs[row.SpecID] = struct{}{}
		}
		switch row.CostState {
		case stateKnown:
			summary.Beads.KnownCost++
		case stateEstimated:
			summary.Beads.EstimatedCost++
		default:
			summary.Beads.UnknownCost++
		}
	}

	summary.Cost.Beads = costBeadCount
	summary.Cost.Features = len(featureIDs)
	for _, row := range costReport.Beads {
		if query.HasSince {
			if _, ok := visibleBeadIDs[row.BeadID]; !ok && len(row.SessionIDs) == 0 {
				continue
			}
		}
		switch row.CostState {
		case stateKnown:
			if row.CostUSD != nil {
				summary.Cost.KnownCostUSD += *row.CostUSD
			}
		case stateEstimated:
			if row.CostUSD != nil {
				summary.Cost.EstimatedCostUSD += *row.CostUSD
			}
		default:
			summary.Cost.UnknownBeads++
		}
	}

	for _, sess := range sessions {
		if query.HasSince && sess.Timestamp.Before(query.Since) {
			continue
		}
		summary.Sessions.Total++
		summary.Sessions.InputTokens += sessionInputTokens(sess)
		summary.Sessions.OutputTokens += sessionOutputTokens(sess)
		summary.Sessions.TotalTokens += sessionTotalTokens(sess)
		if sessionBeadID(sess) != "" {
			summary.Sessions.Correlated++
		} else {
			summary.Sessions.Uncorrelated++
		}
		switch sessionCostState(sess) {
		case stateKnown:
			summary.Sessions.KnownCost++
		case stateEstimated:
			summary.Sessions.EstimatedCost++
		default:
			summary.Sessions.UnknownCost++
		}
		if cost := sessionDerivedCost(sess); cost != nil {
			summary.Sessions.CostUSD += *cost
		}
	}

	for _, row := range reworkReport.Beads {
		if query.HasSince {
			if _, ok := visibleBeadIDs[row.BeadID]; !ok {
				continue
			}
		}
		if row.ReworkState != stateKnown {
			summary.Rework.UnknownCount++
			continue
		}
		summary.Rework.KnownClosed++
		if row.Reopened {
			summary.Rework.KnownReopened++
			summary.Beads.Reopened++
		}
		if row.RevisionCount != nil {
			summary.Rework.RevisionCount += *row.RevisionCount
		}
	}
	if summary.Rework.KnownClosed > 0 {
		summary.Rework.ReopenRate = float64(summary.Rework.KnownReopened) / float64(summary.Rework.KnownClosed)
	}
	return summary, nil
}

// Cost computes bead and feature cost attribution.
func (s *Service) Cost(query Query) (CostReport, error) {
	beads, sessions, sessionByID, err := s.loadInputs()
	if err != nil {
		return CostReport{}, err
	}
	_ = sessions

	selected := selectBeads(beads, query.BeadID, query.FeatureID)
	rows := make([]BeadCostRow, 0, len(selected))
	for _, b := range selected {
		row := buildBeadCostRow(b, sessions, sessionByID, query)
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].BeadID < rows[j].BeadID
	})

	featureRows := buildFeatureCostRows(rows)

	report := CostReport{
		Scope:     costScope(query),
		Window:    windowForQuery(query),
		BeadID:    query.BeadID,
		FeatureID: query.FeatureID,
		Beads:     rows,
		Features:  featureRows,
	}

	for _, row := range rows {
		report.Summary.Beads++
		switch row.CostState {
		case stateKnown:
			if row.CostUSD != nil {
				report.Summary.KnownCostUSD += *row.CostUSD
			}
		case stateEstimated:
			if row.CostUSD != nil {
				report.Summary.EstimatedCostUSD += *row.CostUSD
			}
		default:
			report.Summary.UnknownBeads++
		}
	}
	report.Summary.Features = len(featureRows)
	return report, nil
}

// CycleTime computes lifecycle timing facts.
func (s *Service) CycleTime(query Query) (CycleTimeReport, error) {
	beads, sessions, sessionByID, err := s.loadInputs()
	if err != nil {
		return CycleTimeReport{}, err
	}
	_ = sessions

	rows := make([]CycleTimeRow, 0, len(beads))
	for _, b := range beads {
		row := buildCycleTimeRow(s.WorkingDir, b, sessionByID)
		if query.HasSince && row.FirstClosedAt != nil && row.FirstClosedAt.Before(query.Since) {
			continue
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].FirstClosedAt == nil && rows[j].FirstClosedAt == nil {
			return rows[i].BeadID < rows[j].BeadID
		}
		if rows[i].FirstClosedAt == nil {
			return false
		}
		if rows[j].FirstClosedAt == nil {
			return true
		}
		if !rows[i].FirstClosedAt.Equal(*rows[j].FirstClosedAt) {
			return rows[i].FirstClosedAt.Before(*rows[j].FirstClosedAt)
		}
		return rows[i].BeadID < rows[j].BeadID
	})

	report := CycleTimeReport{
		Scope:  "all",
		Window: windowForQuery(query),
		Beads:  rows,
	}
	for _, row := range rows {
		if row.CycleState == stateKnown && row.CycleTimeMS != nil {
			report.Summary.KnownCount++
			if report.Summary.AverageMS == nil {
				report.Summary.AverageMS = int64Ptr(0)
				report.Summary.MinMS = int64Ptr(*row.CycleTimeMS)
				report.Summary.MaxMS = int64Ptr(*row.CycleTimeMS)
			}
			*report.Summary.AverageMS += *row.CycleTimeMS
			if row.CycleTimeMS != nil {
				if *row.CycleTimeMS < *report.Summary.MinMS {
					*report.Summary.MinMS = *row.CycleTimeMS
				}
				if *row.CycleTimeMS > *report.Summary.MaxMS {
					*report.Summary.MaxMS = *row.CycleTimeMS
				}
			}
		} else {
			report.Summary.UnknownCount++
		}
	}
	if report.Summary.KnownCount > 0 && report.Summary.AverageMS != nil {
		avg := *report.Summary.AverageMS / int64(report.Summary.KnownCount)
		report.Summary.AverageMS = &avg
	}
	return report, nil
}

// Rework computes reopen and revision facts.
func (s *Service) Rework(query Query) (ReworkReport, error) {
	beads, sessions, sessionByID, err := s.loadInputs()
	if err != nil {
		return ReworkReport{}, err
	}
	_ = sessions

	rows := make([]ReworkRow, 0, len(beads))
	for _, b := range beads {
		row := buildReworkRow(s.WorkingDir, b, sessionByID)
		if query.HasSince && row.FirstClosedAt != nil && row.FirstClosedAt.Before(query.Since) {
			continue
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].FirstClosedAt == nil && rows[j].FirstClosedAt == nil {
			return rows[i].BeadID < rows[j].BeadID
		}
		if rows[i].FirstClosedAt == nil {
			return false
		}
		if rows[j].FirstClosedAt == nil {
			return true
		}
		if !rows[i].FirstClosedAt.Equal(*rows[j].FirstClosedAt) {
			return rows[i].FirstClosedAt.Before(*rows[j].FirstClosedAt)
		}
		return rows[i].BeadID < rows[j].BeadID
	})

	report := ReworkReport{
		Scope:  "all",
		Window: windowForQuery(query),
		Beads:  rows,
	}

	for _, row := range rows {
		if row.ReworkState != stateKnown {
			report.Summary.UnknownCount++
			continue
		}
		report.Summary.KnownClosed++
		if row.Reopened {
			report.Summary.KnownReopened++
		}
		if row.RevisionCount != nil {
			report.Summary.RevisionCount += *row.RevisionCount
		}
	}
	if report.Summary.KnownClosed > 0 {
		report.Summary.ReopenRate = float64(report.Summary.KnownReopened) / float64(report.Summary.KnownClosed)
	}
	return report, nil
}

func costScope(query Query) string {
	switch {
	case query.BeadID != "":
		return "bead"
	case query.FeatureID != "":
		return "feature"
	default:
		return "all"
	}
}

func windowForQuery(query Query) Window {
	if !query.HasSince || query.Since.IsZero() {
		return Window{}
	}
	since := query.Since.UTC()
	return Window{
		Since: &since,
		Label: since.Format(time.RFC3339),
	}
}

func beadsVisibleInSummaryWindow(beads []bead.Bead, query Query) ([]bead.Bead, map[string]struct{}) {
	if !query.HasSince || query.Since.IsZero() {
		visibleIDs := make(map[string]struct{}, len(beads))
		for _, b := range beads {
			visibleIDs[b.ID] = struct{}{}
		}
		return beads, visibleIDs
	}

	visible := make([]bead.Bead, 0, len(beads))
	visibleIDs := make(map[string]struct{}, len(beads))
	for _, b := range beads {
		if b.UpdatedAt.Before(query.Since) {
			continue
		}
		visible = append(visible, b)
		visibleIDs[b.ID] = struct{}{}
	}
	return visible, visibleIDs
}

func (s *Service) beadStore() *bead.Store {
	if s.store != nil {
		return s.store
	}
	dir := filepath.Join(s.WorkingDir, ".ddx")
	s.store = bead.NewStore(dir)
	return s.store
}

func (s *Service) loadInputs() ([]bead.Bead, []agent.SessionEntry, map[string]agent.SessionEntry, error) {
	beads, err := s.beadStore().ReadAll()
	if err != nil {
		return nil, nil, nil, err
	}
	sessions, err := readSessions(filepath.Join(s.WorkingDir, agent.DefaultLogDir, "sessions.jsonl"))
	if err != nil {
		return nil, nil, nil, err
	}
	sessionByID := make(map[string]agent.SessionEntry, len(sessions))
	for _, sess := range sessions {
		sessionByID[sess.ID] = sess
	}
	return beads, sessions, sessionByID, nil
}

func readSessions(path string) ([]agent.SessionEntry, error) {
	return readJSONLRecords[agent.SessionEntry](path)
}

func readJSONLRecords[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var out []T
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec T
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		out = append(out, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func selectBeads(beads []bead.Bead, beadID, featureID string) []bead.Bead {
	if beadID != "" {
		for _, b := range beads {
			if b.ID == beadID {
				return []bead.Bead{b}
			}
		}
		return []bead.Bead{}
	}
	if featureID == "" {
		return beads
	}
	out := make([]bead.Bead, 0, len(beads))
	for _, b := range beads {
		if extraString(b, "spec-id", "spec_id") == featureID {
			out = append(out, b)
		}
	}
	return out
}

func buildBeadCostRow(b bead.Bead, sessions []agent.SessionEntry, sessionByID map[string]agent.SessionEntry, query Query) BeadCostRow {
	matches := matchedSessionsForBead(b, sessions, sessionByID)
	if query.HasSince {
		filtered := matches[:0]
		for _, sess := range matches {
			if sess.Timestamp.Before(query.Since) {
				continue
			}
			filtered = append(filtered, sess)
		}
		matches = filtered
	}

	row := BeadCostRow{
		BeadID:     b.ID,
		Title:      b.Title,
		SpecID:     extraString(b, "spec-id", "spec_id"),
		Status:     b.Status,
		SessionIDs: make([]string, 0, len(matches)),
		Provenance: make([]SourceRef, 0, len(matches)),
	}

	var costTotal float64
	var knownCount, estimatedCount, unknownCount int
	for _, sess := range matches {
		row.SessionIDs = append(row.SessionIDs, sess.ID)
		row.InputTokens += sessionInputTokens(sess)
		row.OutputTokens += sessionOutputTokens(sess)
		row.TotalTokens += sessionTotalTokens(sess)

		if cost := sessionDerivedCost(sess); cost != nil {
			costTotal += *cost
			if sessionCostState(sess) == stateKnown {
				knownCount++
			} else {
				estimatedCount++
			}
		} else {
			unknownCount++
		}

		row.Provenance = append(row.Provenance, sourceRefForSession(b.ID, sess))
	}

	switch {
	case len(matches) == 0:
		row.CostState = stateUnknown
	case unknownCount > 0 && knownCount == 0 && estimatedCount == 0:
		row.CostState = stateUnknown
	case estimatedCount > 0:
		row.CostState = stateEstimated
	default:
		row.CostState = stateKnown
	}
	if row.CostState != stateUnknown {
		row.CostUSD = float64Ptr(costTotal)
	}
	row.UnknownSessions = unknownCount
	return row
}

func buildFeatureCostRows(beadRows []BeadCostRow) []FeatureCostRow {
	grouped := make(map[string]*FeatureCostRow)
	for _, row := range beadRows {
		if row.SpecID == "" {
			continue
		}
		entry, ok := grouped[row.SpecID]
		if !ok {
			entry = &FeatureCostRow{SpecID: row.SpecID}
			grouped[row.SpecID] = entry
		}
		entry.BeadIDs = append(entry.BeadIDs, row.BeadID)
		entry.InputTokens += row.InputTokens
		entry.OutputTokens += row.OutputTokens
		entry.TotalTokens += row.TotalTokens
		if row.CostState == stateUnknown || row.CostUSD == nil {
			entry.UnknownBeads++
			continue
		}
		switch row.CostState {
		case stateKnown:
			if entry.CostUSD == nil {
				entry.CostUSD = float64Ptr(0)
			}
		case stateEstimated:
			if entry.CostState == stateKnown {
				entry.CostState = stateEstimated
			}
		}
		if entry.CostUSD == nil {
			entry.CostUSD = float64Ptr(0)
		}
		*entry.CostUSD += *row.CostUSD
		if row.CostState == stateEstimated {
			entry.CostState = stateEstimated
		} else if entry.CostState == "" {
			entry.CostState = stateKnown
		}
	}

	out := make([]FeatureCostRow, 0, len(grouped))
	for _, row := range grouped {
		sort.Strings(row.BeadIDs)
		if row.CostUSD != nil {
			if row.CostState == "" {
				row.CostState = stateKnown
			}
		} else {
			row.CostState = stateUnknown
		}
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SpecID < out[j].SpecID
	})
	return out
}

func buildCycleTimeRow(workingDir string, b bead.Bead, sessionByID map[string]agent.SessionEntry) CycleTimeRow {
	row := CycleTimeRow{
		BeadID:     b.ID,
		Title:      b.Title,
		SpecID:     extraString(b, "spec-id", "spec_id"),
		Status:     b.Status,
		CreatedAt:  b.CreatedAt.UTC(),
		CycleState: stateUnknown,
	}

	events := beadEvents(b)
	transitions := recognizedTransitions(events)
	sort.Slice(transitions, func(i, j int) bool {
		return transitions[i].At.Before(transitions[j].At)
	})

	closeAt, provenance := firstCloseEvidence(workingDir, b, transitions, sessionByID)
	if closeAt != nil {
		row.FirstClosedAt = closeAt
		row.CycleTimeMS = int64Ptr(closeAt.Sub(b.CreatedAt).Milliseconds())
		row.CycleState = stateKnown
		row.Provenance = append(row.Provenance, provenance...)
	}

	if len(transitions) == 0 {
		return row
	}

	row.Provenance = append(row.Provenance, provenanceForEvents(b.ID, events)...)

	var lastStatus string
	var lastAt time.Time
	var firstClosedSeen bool
	var reopenCount int
	var revisionCount int
	var openMS, inProgressMS, closedMS int64
	for i, tr := range transitions {
		if i == 0 {
			lastStatus = bead.StatusOpen
			lastAt = b.CreatedAt.UTC()
		}
		if tr.At.Before(lastAt) {
			continue
		}
		delta := tr.At.Sub(lastAt).Milliseconds()
		switch lastStatus {
		case bead.StatusOpen:
			openMS += delta
		case bead.StatusInProgress:
			inProgressMS += delta
		case bead.StatusClosed:
			closedMS += delta
		}
		if tr.Status == bead.StatusClosed && !firstClosedSeen {
			firstClosedSeen = true
			if row.FirstClosedAt == nil {
				ts := tr.At
				row.FirstClosedAt = &ts
				row.CycleTimeMS = int64Ptr(ts.Sub(b.CreatedAt).Milliseconds())
				row.CycleState = stateKnown
			}
		}
		if firstClosedSeen {
			if tr.Status == bead.StatusClosed {
				// Status closed is part of the close window; do not count it as rework.
			} else {
				revisionCount++
				if lastStatus == bead.StatusClosed && (tr.Status == bead.StatusOpen || tr.Status == bead.StatusInProgress) {
					reopenCount++
				}
			}
		}
		lastStatus = tr.Status
		lastAt = tr.At
	}

	end := b.UpdatedAt.UTC()
	if end.Before(lastAt) {
		end = lastAt
	}
	if !end.IsZero() {
		delta := end.Sub(lastAt).Milliseconds()
		switch lastStatus {
		case bead.StatusOpen:
			openMS += delta
		case bead.StatusInProgress:
			inProgressMS += delta
		case bead.StatusClosed:
			closedMS += delta
		}
	}

	if openMS > 0 {
		row.TimeInOpenMS = int64Ptr(openMS)
	}
	if inProgressMS > 0 {
		row.TimeInProgressMS = int64Ptr(inProgressMS)
	}
	if closedMS > 0 {
		row.TimeInClosedMS = int64Ptr(closedMS)
	}
	if firstClosedSeen {
		row.ReopenCount = intPtr(reopenCount)
		row.RevisionCount = intPtr(revisionCount)
	}

	if firstClosedSeen {
		lastClose := row.FirstClosedAt
		for _, tr := range transitions {
			if tr.Status == bead.StatusClosed {
				ts := tr.At
				lastClose = &ts
			}
		}
		row.LastClosedAt = lastClose
	}
	return row
}

func buildReworkRow(workingDir string, b bead.Bead, sessionByID map[string]agent.SessionEntry) ReworkRow {
	row := ReworkRow{
		BeadID: b.ID,
		Title:  b.Title,
		SpecID: extraString(b, "spec-id", "spec_id"),
		Status: b.Status,
	}

	events := beadEvents(b)
	transitions := recognizedTransitions(events)
	sort.Slice(transitions, func(i, j int) bool {
		return transitions[i].At.Before(transitions[j].At)
	})

	closeAt, provenance := firstCloseEvidence(workingDir, b, transitions, sessionByID)
	if closeAt == nil {
		return row
	}
	row.FirstClosedAt = closeAt
	row.Provenance = append(row.Provenance, provenance...)
	if len(transitions) == 0 {
		return row
	}
	row.ReworkState = stateKnown

	var reopenCount int
	var revisionCount int
	var sawClose bool
	for _, tr := range transitions {
		if tr.At.Before(*closeAt) {
			continue
		}
		if tr.Status == bead.StatusClosed {
			sawClose = true
			continue
		}
		if sawClose && (tr.Status == bead.StatusOpen || tr.Status == bead.StatusInProgress) {
			reopenCount++
		}
		if tr.Status != bead.StatusClosed {
			revisionCount++
		}
	}
	row.Reopened = reopenCount > 0
	row.ReopenCount = intPtr(reopenCount)
	row.RevisionCount = intPtr(revisionCount)
	return row
}

func matchedSessionsForBead(b bead.Bead, sessions []agent.SessionEntry, sessionByID map[string]agent.SessionEntry) []agent.SessionEntry {
	beadID := b.ID
	seen := make(map[string]struct{})
	out := make([]agent.SessionEntry, 0)

	for _, sess := range sessions {
		if sessionBeadID(sess) == beadID {
			if _, ok := seen[sess.ID]; !ok {
				out = append(out, sess)
				seen[sess.ID] = struct{}{}
			}
		}
	}

	if sessionID := extraString(b, "session_id", "session-id"); sessionID != "" {
		if sess, ok := sessionByID[sessionID]; ok {
			if _, exists := seen[sess.ID]; !exists {
				out = append(out, sess)
				seen[sess.ID] = struct{}{}
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Timestamp.Equal(out[j].Timestamp) {
			return out[i].ID < out[j].ID
		}
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out
}

type statusTransition struct {
	At     time.Time
	Status string
}

func recognizedTransitions(events []bead.BeadEvent) []statusTransition {
	out := make([]statusTransition, 0, len(events))
	for _, event := range events {
		status, ok := statusFromEvent(event)
		if !ok {
			continue
		}
		out = append(out, statusTransition{At: event.CreatedAt.UTC(), Status: status})
	}
	return out
}

func firstCloseEvidence(workingDir string, b bead.Bead, transitions []statusTransition, sessionByID map[string]agent.SessionEntry) (*time.Time, []SourceRef) {
	var eventCloseTimes []time.Time
	var eventProvenance []SourceRef
	for _, tr := range transitions {
		if tr.Status != bead.StatusClosed {
			continue
		}
		ts := tr.At
		eventCloseTimes = append(eventCloseTimes, ts)
		eventProvenance = append(eventProvenance, SourceRef{
			BeadID:    b.ID,
			Timestamp: &ts,
			Source:    "event",
		})
	}
	if len(eventCloseTimes) > 0 {
		sort.Slice(eventCloseTimes, func(i, j int) bool { return eventCloseTimes[i].Before(eventCloseTimes[j]) })
		first := eventCloseTimes[0]
		return &first, eventProvenance
	}

	if sha := extraString(b, "closing_commit_sha"); sha != "" {
		if ts, ok := commitTimeWithWorkingDir(workingDir, sha); ok {
			return &ts, []SourceRef{{
				BeadID:           b.ID,
				ClosingCommitSHA: sha,
				Timestamp:        &ts,
				Source:           "commit",
			}}
		}
	}

	if sessionID := extraString(b, "session_id", "session-id"); sessionID != "" {
		if sess, ok := sessionByID[sessionID]; ok {
			ts := sess.Timestamp.UTC()
			return &ts, []SourceRef{{
				BeadID:          b.ID,
				SessionID:       sess.ID,
				NativeSessionID: sess.NativeSessionID,
				NativeLogRef:    sess.NativeLogRef,
				Timestamp:       &ts,
				Source:          "session",
			}}
		}
	}

	return nil, nil
}

func commitTimeWithWorkingDir(workingDir, sha string) (time.Time, bool) {
	if sha == "" {
		return time.Time{}, false
	}
	cmd := exec.Command("git", "-C", workingDir, "show", "-s", "--format=%cI", sha)
	out, err := cmd.Output()
	if err != nil {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(string(out)))
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

func provenanceForEvents(beadID string, events []bead.BeadEvent) []SourceRef {
	out := make([]SourceRef, 0, len(events))
	for _, event := range events {
		if event.CreatedAt.IsZero() {
			continue
		}
		ts := event.CreatedAt.UTC()
		out = append(out, SourceRef{
			BeadID:    beadID,
			Timestamp: &ts,
			Source:    event.Source,
		})
	}
	return out
}

func sourceRefForSession(beadID string, sess agent.SessionEntry) SourceRef {
	ts := sess.Timestamp.UTC()
	return SourceRef{
		BeadID:          beadID,
		SessionID:       sess.ID,
		NativeSessionID: sess.NativeSessionID,
		NativeLogRef:    sess.NativeLogRef,
		Timestamp:       &ts,
		Source:          "session",
	}
}

func sessionBeadID(sess agent.SessionEntry) string {
	if sess.Correlation == nil {
		return ""
	}
	return strings.TrimSpace(sess.Correlation["bead_id"])
}

func sessionInputTokens(sess agent.SessionEntry) int {
	return sess.InputTokens
}

func sessionOutputTokens(sess agent.SessionEntry) int {
	return sess.OutputTokens
}

func sessionTotalTokens(sess agent.SessionEntry) int {
	if sess.TotalTokens > 0 {
		return sess.TotalTokens
	}
	return sess.InputTokens + sess.OutputTokens
}

func sessionCostState(sess agent.SessionEntry) State {
	if sess.CostUSD > 0 {
		return stateKnown
	}
	if sess.CostUSD == -1 {
		return stateUnknown
	}
	if est, ok := sessionEstimatedCost(sess); ok {
		if est == 0 {
			return stateKnown
		}
		return stateEstimated
	}
	return stateUnknown
}

func sessionDerivedCost(sess agent.SessionEntry) *float64 {
	if sess.CostUSD > 0 {
		return float64Ptr(sess.CostUSD)
	}
	if sess.CostUSD == -1 {
		return nil
	}
	if est, ok := sessionEstimatedCost(sess); ok {
		return float64Ptr(est)
	}
	return nil
}

func sessionEstimatedCost(sess agent.SessionEntry) (float64, bool) {
	if sess.Model == "" {
		return 0, false
	}
	est := agent.EstimateCost(sess.Model, sess.InputTokens, sess.OutputTokens)
	if est < 0 {
		return 0, false
	}
	return est, true
}

func extraString(b bead.Bead, keys ...string) string {
	if b.Extra == nil {
		return ""
	}
	for _, key := range keys {
		if v, ok := b.Extra[key]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func beadEvents(b bead.Bead) []bead.BeadEvent {
	if b.Extra == nil {
		return nil
	}
	raw, ok := b.Extra["events"]
	if !ok {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var events []bead.BeadEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return nil
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].CreatedAt.Before(events[j].CreatedAt)
	})
	return events
}

func statusFromEvent(event bead.BeadEvent) (string, bool) {
	kind := normalizeToken(event.Kind)
	switch kind {
	case "open", "opened":
		return bead.StatusOpen, true
	case "in_progress", "inprogress", "started", "start", "working":
		return bead.StatusInProgress, true
	case "closed", "close", "finished", "done":
		return bead.StatusClosed, true
	case "reopened", "reopen":
		return bead.StatusOpen, true
	}

	candidate := normalizeToken(firstNonEmpty(event.Summary, event.Body))
	switch candidate {
	case "open", "opened":
		return bead.StatusOpen, true
	case "in_progress", "inprogress", "started", "start", "working":
		return bead.StatusInProgress, true
	case "closed", "close", "finished", "done":
		return bead.StatusClosed, true
	case "reopened", "reopen":
		return bead.StatusOpen, true
	}

	if strings.HasPrefix(kind, "status:") {
		return statusFromToken(strings.TrimPrefix(kind, "status:"))
	}
	if strings.HasPrefix(kind, "state:") {
		return statusFromToken(strings.TrimPrefix(kind, "state:"))
	}
	if strings.HasPrefix(kind, "transition:") {
		return statusFromToken(strings.TrimPrefix(kind, "transition:"))
	}
	return "", false
}

func statusFromToken(raw string) (string, bool) {
	switch normalizeToken(raw) {
	case "open", "opened":
		return bead.StatusOpen, true
	case "in_progress", "inprogress", "started", "start", "working":
		return bead.StatusInProgress, true
	case "closed", "close", "finished", "done":
		return bead.StatusClosed, true
	case "reopened", "reopen":
		return bead.StatusOpen, true
	default:
		return "", false
	}
}

func normalizeToken(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	replacer := strings.NewReplacer("-", "_", " ", "_")
	return replacer.Replace(s)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func parsePositiveInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty")
	}
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid")
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

func intPtr(v int) *int {
	return &v
}

func int64Ptr(v int64) *int64 {
	return &v
}

func float64Ptr(v float64) *float64 {
	return &v
}
