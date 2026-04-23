package graphql

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// ProviderStatuses is the resolver for the providerStatuses field.
// It mirrors the output of `ddx agent providers`, annotating each row with
// kind=ENDPOINT, a lastCheckedAt timestamp, and rolling usage derived from
// the sessions index. Quota is populated when the upstream ProviderInfo
// exposes token-level quota headers; null otherwise (FEAT-014 no-fabrication).
func (r *queryResolver) ProviderStatuses(ctx context.Context) ([]*ProviderStatus, error) {
	svc, err := agent.NewServiceFromWorkDir(r.WorkingDir)
	if err != nil {
		return []*ProviderStatus{}, nil //nolint:nilerr
	}
	providers, err := svc.ListProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing providers: %w", err)
	}

	now := time.Now().UTC()
	entries := r.sessionIndexEntries()
	lastChecked := now.Format(time.RFC3339)

	results := make([]*ProviderStatus, 0, len(providers))
	for _, p := range providers {
		url := p.BaseURL
		if url == "" {
			url = "(api)"
		}
		ps := &ProviderStatus{
			Name:              p.Name,
			Kind:              ProviderKindEndpoint,
			ProviderType:      p.Type,
			BaseURL:           url,
			Model:             p.DefaultModel,
			Status:            p.Status,
			ModelCount:        p.ModelCount,
			IsDefault:         p.IsDefault,
			LastCheckedAt:     strPtr(lastChecked),
			DefaultForProfile: defaultProfilesForEndpoint(p),
		}
		if p.CooldownState != nil && !p.CooldownState.Until.IsZero() {
			s := p.CooldownState.Until.UTC().Format(time.RFC3339)
			ps.CooldownUntil = &s
		}
		ps.Usage = buildUsage(entries, p.Name, agent.MatchProvider, now)
		ps.Quota = quotaFromProviderInfo(p)
		results = append(results, ps)
	}

	return results, nil
}

// HarnessStatuses is the resolver for the harnessStatuses field.
// It returns one row per subprocess harness (kind=HARNESS). Reachability is
// taken from HarnessInfo.Available, rolling usage from the sessions index,
// and quota from the harness-reported rate-limit data when available.
func (r *queryResolver) HarnessStatuses(ctx context.Context) ([]*ProviderStatus, error) {
	svc, err := agent.NewServiceFromWorkDir(r.WorkingDir)
	if err != nil {
		return []*ProviderStatus{}, nil //nolint:nilerr
	}
	infos, err := svc.ListHarnesses(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing harnesses: %w", err)
	}

	now := time.Now().UTC()
	entries := r.sessionIndexEntries()
	lastChecked := now.Format(time.RFC3339)

	results := make([]*ProviderStatus, 0, len(infos))
	for _, info := range infos {
		ps := &ProviderStatus{
			Name:              info.Name,
			Kind:              ProviderKindHarness,
			ProviderType:      harnessTypeLabel(info),
			BaseURL:           "(subprocess)",
			Model:             info.DefaultModel,
			Status:            harnessStatusLine(info),
			ModelCount:        harnessModelCount(info),
			IsDefault:         false,
			LastCheckedAt:     strPtr(lastChecked),
			DefaultForProfile: []string{},
		}
		ps.Usage = buildUsage(entries, info.Name, agent.MatchHarness, now)
		ps.Quota = quotaFromHarnessInfo(info)
		results = append(results, ps)
	}

	return results, nil
}

// DefaultRouteStatus is the resolver for the defaultRouteStatus field.
func (r *queryResolver) DefaultRouteStatus(ctx context.Context) (*DefaultRouteStatus, error) {
	svc, err := agent.NewServiceFromWorkDir(r.WorkingDir)
	if err != nil {
		return nil, nil //nolint:nilerr
	}
	dec, err := svc.ResolveRoute(ctx, agentlib.RouteRequest{})
	if err != nil {
		return &DefaultRouteStatus{}, nil //nolint:nilerr
	}
	result := &DefaultRouteStatus{ModelRef: dec.Model}
	if dec.Provider != "" {
		p := dec.Provider
		result.ResolvedProvider = &p
	}
	if dec.Model != "" {
		m := dec.Model
		result.ResolvedModel = &m
	}
	return result, nil
}

// ProviderTrend is the resolver for the providerTrend field.
// It aggregates the sessions index into time buckets for one provider/harness
// and computes a projected-run-out-in-hours callout from the last-24h slope.
func (r *queryResolver) ProviderTrend(ctx context.Context, name string, windowDays int) (*ProviderTrend, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	if windowDays != 7 && windowDays != 30 {
		return nil, fmt.Errorf("windowDays must be 7 or 30")
	}
	now := time.Now().UTC()
	entries := r.sessionIndexEntries()

	kind, detected := detectProviderOrHarness(ctx, r, name)
	bucketKind := agent.MatchProvider
	if kind == ProviderKindHarness {
		bucketKind = agent.MatchHarness
	}

	bucketSize := time.Hour
	if windowDays > 7 {
		bucketSize = 4 * time.Hour
	}
	buckets := agent.BucketUsage(entries, name, bucketKind, now, windowDays, bucketSize)
	series := make([]*ProviderTrendPoint, 0, len(buckets))
	for _, b := range buckets {
		series = append(series, &ProviderTrendPoint{
			BucketStart: b.Start.UTC().Format(time.RFC3339),
			Tokens:      b.Tokens,
			Requests:    b.Requests,
		})
	}

	trend := &ProviderTrend{
		Name:       name,
		Kind:       kind,
		WindowDays: windowDays,
		Series:     series,
	}

	if detected != nil {
		if ceiling := detected.ceilingTokens; ceiling > 0 {
			c := ceiling
			trend.CeilingTokens = &c
			if p := projectRunOutHours(buckets, float64(ceiling)-float64(detected.remaining)); p > 0 {
				trend.ProjectedRunOutHours = &p
			}
		}
	}

	return trend, nil
}

// --------- helpers ---------

type detectedRow struct {
	ceilingTokens int
	remaining     int
}

// detectProviderOrHarness looks up a name against providers first, then
// harnesses, returning the resolved kind and quota signal (if any).
func detectProviderOrHarness(ctx context.Context, r *queryResolver, name string) (ProviderKind, *detectedRow) {
	svc, err := agent.NewServiceFromWorkDir(r.WorkingDir)
	if err != nil {
		return ProviderKindEndpoint, nil
	}
	providers, _ := svc.ListProviders(ctx)
	for _, p := range providers {
		if strings.EqualFold(p.Name, name) {
			q := quotaFromProviderInfo(p)
			return ProviderKindEndpoint, detectedFromQuota(q)
		}
	}
	harnesses, _ := svc.ListHarnesses(ctx)
	for _, h := range harnesses {
		if strings.EqualFold(h.Name, name) {
			q := quotaFromHarnessInfo(h)
			return ProviderKindHarness, detectedFromQuota(q)
		}
	}
	return ProviderKindEndpoint, nil
}

func detectedFromQuota(q *ProviderQuota) *detectedRow {
	if q == nil {
		return nil
	}
	row := &detectedRow{ceilingTokens: -1, remaining: -1}
	if q.CeilingTokens != nil {
		row.ceilingTokens = *q.CeilingTokens
	}
	if q.Remaining != nil {
		row.remaining = *q.Remaining
	}
	return row
}

// projectRunOutHours returns the hours projected until `used` reaches ceiling
// based on the last-24-hour slope of the bucket series. Returns 0 when the
// slope is non-positive, the ceiling is already met, or the series is too
// short to estimate.
func projectRunOutHours(buckets []agent.UsageBucket, used float64) float64 {
	if len(buckets) < 2 {
		return 0
	}
	// Last 24 hours of buckets; if series is bucketed in 4h blocks, that's 6
	// buckets; 1h buckets gives 24.
	n := 24
	if n > len(buckets) {
		n = len(buckets)
	}
	tail := buckets[len(buckets)-n:]
	tokens := make([]float64, len(tail))
	for i, b := range tail {
		tokens[i] = float64(b.Tokens)
	}
	perBucket := agent.LinearSlope(tokens)
	// Convert slope-per-bucket to slope-per-hour.
	var bucketHours float64
	if len(tail) >= 2 {
		bucketHours = tail[1].Start.Sub(tail[0].Start).Hours()
	}
	if bucketHours <= 0 {
		bucketHours = 1
	}
	perHour := perBucket / bucketHours
	if perHour <= 0 {
		return 0
	}
	// Estimate remaining headroom as ceilingTokens - used. When `used` is
	// negative (ceiling unknown) we can't project.
	remaining := -used
	if remaining <= 0 {
		return 0
	}
	hours := remaining / perHour
	if hours <= 0 {
		return 0
	}
	return hours
}

// sessionIndexEntries reads the project's session index for all available
// shards. Errors are swallowed — a missing index is a normal "no data" state.
func (r *queryResolver) sessionIndexEntries() []agent.SessionIndexEntry {
	logDir := agent.SessionLogDirForWorkDir(r.WorkingDir)
	if logDir == "" {
		return nil
	}
	entries, err := agent.ReadSessionIndex(logDir, agent.SessionIndexQuery{})
	if err != nil {
		return nil
	}
	return entries
}

func buildUsage(entries []agent.SessionIndexEntry, name string, kind agent.UsageMatchKind, now time.Time) *ProviderUsage {
	counts := agent.AggregateUsageCounts(entries, name, kind, now)
	u := &ProviderUsage{}
	v := counts.TokensLastHour
	u.TokensUsedLastHour = &v
	v2 := counts.TokensLast24h
	u.TokensUsedLast24h = &v2
	v3 := counts.RequestsLastHour
	u.RequestsLastHour = &v3
	v4 := counts.RequestsLast24h
	u.RequestsLast24h = &v4
	return u
}

// quotaFromProviderInfo derives a ProviderQuota from the upstream ProviderInfo.
// Returns nil when no token-level ceiling is published.
func quotaFromProviderInfo(p agentlib.ProviderInfo) *ProviderQuota {
	if p.Quota == nil {
		return nil
	}
	return quotaFromState(p.Quota)
}

// quotaFromHarnessInfo derives a ProviderQuota from the upstream HarnessInfo.
// Returns nil when no token-level ceiling is published.
func quotaFromHarnessInfo(info agentlib.HarnessInfo) *ProviderQuota {
	if info.Quota == nil {
		return nil
	}
	return quotaFromState(info.Quota)
}

// quotaFromState derives a ProviderQuota from an upstream QuotaState. The
// upstream QuotaWindow doesn't expose absolute token ceilings (only percent
// used), so we surface the window length and reset time only. Ceiling and
// remaining stay unknown unless a harness-specific rate-limit-header path
// populates them via QuotaFromRateLimitSignal.
func quotaFromState(state *agentlib.QuotaState) *ProviderQuota {
	if state == nil || len(state.Windows) == 0 {
		return nil
	}
	var windowSeconds int
	var resetAt string
	for _, w := range state.Windows {
		if strings.EqualFold(w.LimitID, "extra") {
			continue
		}
		if windowSeconds == 0 && w.WindowMinutes > 0 {
			windowSeconds = w.WindowMinutes * 60
		}
		if resetAt == "" {
			if w.ResetsAt != "" {
				resetAt = w.ResetsAt
			} else if w.ResetsAtUnix > 0 {
				resetAt = time.Unix(w.ResetsAtUnix, 0).UTC().Format(time.RFC3339)
			}
		}
		if windowSeconds > 0 && resetAt != "" {
			break
		}
	}
	if windowSeconds == 0 && resetAt == "" {
		return nil
	}
	q := &ProviderQuota{}
	if windowSeconds > 0 {
		v := windowSeconds
		q.CeilingWindowSeconds = &v
	}
	if resetAt != "" {
		v := resetAt
		q.ResetAt = &v
	}
	return q
}

// QuotaFromRateLimitSignal produces a ProviderQuota from a parsed rate-limit
// header signal (see agent.ParseRateLimitHeaders). Exposed for future call
// sites where the server captures harness response headers.
func QuotaFromRateLimitSignal(sig agent.RateLimitSignal) *ProviderQuota {
	if !sig.HasAny() {
		return nil
	}
	q := &ProviderQuota{}
	if sig.CeilingTokens >= 0 {
		v := sig.CeilingTokens
		q.CeilingTokens = &v
	}
	if sig.CeilingWindowSeconds >= 0 {
		v := sig.CeilingWindowSeconds
		q.CeilingWindowSeconds = &v
	}
	if sig.Remaining >= 0 {
		v := sig.Remaining
		q.Remaining = &v
	}
	if !sig.ResetAt.IsZero() {
		v := sig.ResetAt.UTC().Format(time.RFC3339)
		q.ResetAt = &v
	}
	return q
}

// defaultProfilesForEndpoint returns the profile names where this endpoint is
// a default candidate. Current service surface exposes only a single
// IsDefault flag, so we return ["default"] when set, [] otherwise. The return
// shape supports multi-profile expansion without breaking callers.
func defaultProfilesForEndpoint(p agentlib.ProviderInfo) []string {
	if p.IsDefault {
		return []string{"default"}
	}
	return []string{}
}

func harnessTypeLabel(info agentlib.HarnessInfo) string {
	if info.Type != "" {
		return info.Type
	}
	return "subprocess"
}

func harnessStatusLine(info agentlib.HarnessInfo) string {
	if info.Available {
		return "available"
	}
	if info.Error != "" {
		return "unavailable: " + info.Error
	}
	return "unavailable"
}

func harnessModelCount(_ agentlib.HarnessInfo) int {
	// Harness-reported model counts flow through a separate model-discovery
	// path; leave 0 until that surface is exposed to avoid fabricating a
	// number from capability flags.
	return 0
}
