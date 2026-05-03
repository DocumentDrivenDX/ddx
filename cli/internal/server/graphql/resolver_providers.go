package graphql

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	agentlib "github.com/DocumentDrivenDX/fizeau"
)

var providerStatusCache = struct {
	sync.Mutex
	rows     map[string][]*ProviderStatus
	inFlight map[string]bool
}{
	rows:     make(map[string][]*ProviderStatus),
	inFlight: make(map[string]bool),
}

// harnessRateLimitCache holds the most recently observed rate-limit signal per
// harness name. The server invocation path (or tests) populates it via
// RecordHarnessRateLimit; quotaFromHarnessInfo reads it when the upstream
// HarnessInfo.Quota does not carry token-level ceilings.
var harnessRateLimitCache = struct {
	sync.RWMutex
	byName map[string]agent.RateLimitSignal
}{byName: make(map[string]agent.RateLimitSignal)}

// RecordHarnessRateLimit stores the latest parsed rate-limit signal for a
// harness invocation. Intended for the server's harness-dispatch path to call
// after each response; tests use it directly.
func RecordHarnessRateLimit(name string, sig agent.RateLimitSignal) {
	name = strings.TrimSpace(name)
	if name == "" || !sig.HasAny() {
		return
	}
	harnessRateLimitCache.Lock()
	harnessRateLimitCache.byName[name] = sig
	harnessRateLimitCache.Unlock()
}

// LookupHarnessRateLimit returns the last-observed signal for a harness, or
// ok=false if none has been recorded.
func LookupHarnessRateLimit(name string) (agent.RateLimitSignal, bool) {
	harnessRateLimitCache.RLock()
	defer harnessRateLimitCache.RUnlock()
	sig, ok := harnessRateLimitCache.byName[name]
	return sig, ok
}

// resetHarnessRateLimitCache is a test seam.
func resetHarnessRateLimitCache() {
	harnessRateLimitCache.Lock()
	harnessRateLimitCache.byName = make(map[string]agent.RateLimitSignal)
	harnessRateLimitCache.Unlock()
}

// ProviderStatuses is the resolver for the providerStatuses field.
// It mirrors the output of `ddx agent providers`, annotating each row with
// kind=ENDPOINT, a lastCheckedAt timestamp, and rolling usage derived from
// the sessions index. Quota is populated when the upstream ProviderInfo
// exposes token-level quota headers; null otherwise (FEAT-014 no-fabrication).
func (r *queryResolver) ProviderStatuses(ctx context.Context) ([]*ProviderStatus, error) {
	now := time.Now().UTC()
	entries := r.sessionIndexEntries(ctx)

	if rows := cachedProviderRows(r.workingDir(ctx)); len(rows) > 0 {
		refreshProviderStatuses(r.workingDir(ctx))
		return providerRowsWithUsage(rows, entries, now), nil
	}

	if snapshots, ok, err := agent.ConfiguredProviderSnapshots(r.workingDir(ctx)); err == nil && ok {
		rows := providerStatusesFromInfos(snapshots, entries, now)
		storeProviderRows(r.workingDir(ctx), rows)
		refreshProviderStatuses(r.workingDir(ctx))
		return rows, nil
	} else if err != nil {
		return nil, fmt.Errorf("loading provider snapshots: %w", err)
	}

	// Legacy/global provider config fallback. Bound the synchronous path so the
	// UI can still first-paint harness rows even if provider probing is slow.
	probeCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	providers, err := liveProviderInfos(probeCtx, r.workingDir(ctx))
	if err != nil {
		refreshProviderStatuses(r.workingDir(ctx))
		return []*ProviderStatus{}, nil
	}
	rows := providerStatusesFromInfos(providers, entries, now)
	storeProviderRows(r.workingDir(ctx), rows)
	return rows, nil
}

// HarnessStatuses is the resolver for the harnessStatuses field.
// It returns one row per subprocess harness (kind=HARNESS). Reachability is
// taken from HarnessInfo.Available, rolling usage from the sessions index,
// and quota from the harness-reported rate-limit data when available.
func (r *queryResolver) HarnessStatuses(ctx context.Context) ([]*ProviderStatus, error) {
	svc, err := agentlib.New(agentlib.ServiceOptions{})
	if err != nil {
		return []*ProviderStatus{}, nil //nolint:nilerr
	}
	infos, err := svc.ListHarnesses(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing harnesses: %w", err)
	}

	now := time.Now().UTC()
	entries := r.sessionIndexEntries(ctx)
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
			Reachable:         info.Available,
			Detail:            harnessDetail(info),
			ModelCount:        harnessModelCount(info),
			IsDefault:         false,
			LastCheckedAt:     strPtr(lastChecked),
			DefaultForProfile: []string{},
		}
		ps.Usage = buildUsage(entries, info.Name, agent.MatchHarness, now)
		ps.Quota = quotaFromHarnessInfo(info)
		ps.Sparkline = buildSparkline(entries, info.Name, agent.MatchHarness, now)
		results = append(results, ps)
	}

	return results, nil
}

// DefaultRouteStatus is the resolver for the defaultRouteStatus field.
func (r *queryResolver) DefaultRouteStatus(ctx context.Context) (*DefaultRouteStatus, error) {
	svc, err := agent.NewServiceFromWorkDir(r.workingDir(ctx))
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
	entries := r.sessionIndexEntries(ctx)

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
			remaining := detected.remaining
			if remaining < 0 {
				remaining = ceiling - sumTailTokens(buckets, 24)
			}
			if p := projectRunOutHours(buckets, float64(remaining)); p > 0 {
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
	if svc, err := agentlib.New(agentlib.ServiceOptions{}); err == nil {
		harnesses, _ := svc.ListHarnesses(ctx)
		for _, h := range harnesses {
			if strings.EqualFold(h.Name, name) {
				q := quotaFromHarnessInfo(h)
				return ProviderKindHarness, detectedFromQuota(q)
			}
		}
	}
	if snapshots, ok, _ := agent.ConfiguredProviderSnapshots(r.workingDir(ctx)); ok {
		for _, p := range snapshots {
			if strings.EqualFold(p.Name, name) {
				q := quotaFromProviderInfo(p)
				return ProviderKindEndpoint, detectedFromQuota(q)
			}
		}
	}
	if providers, err := liveProviderInfos(ctx, r.workingDir(ctx)); err == nil {
		for _, p := range providers {
			if strings.EqualFold(p.Name, name) {
				q := quotaFromProviderInfo(p)
				return ProviderKindEndpoint, detectedFromQuota(q)
			}
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

// projectRunOutHours returns the hours projected until current headroom is used
// based on the last-24-hour slope of the bucket series. Returns 0 when the
// slope is non-positive, headroom is gone/unknown, or the series is too
// short to estimate.
func projectRunOutHours(buckets []agent.UsageBucket, remaining float64) float64 {
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
	if remaining <= 0 {
		return 0
	}
	hours := remaining / perHour
	if hours <= 0 {
		return 0
	}
	return hours
}

func sumTailTokens(buckets []agent.UsageBucket, maxBuckets int) int {
	if maxBuckets <= 0 || len(buckets) == 0 {
		return 0
	}
	if maxBuckets > len(buckets) {
		maxBuckets = len(buckets)
	}
	total := 0
	for _, b := range buckets[len(buckets)-maxBuckets:] {
		total += b.Tokens
	}
	return total
}

func liveProviderInfos(ctx context.Context, workDir string) ([]agentlib.ProviderInfo, error) {
	svc, err := agent.NewStatusProbeServiceFromWorkDir(workDir)
	if err != nil {
		return nil, err
	}
	return svc.ListProviders(ctx)
}

func refreshProviderStatuses(workDir string) {
	providerStatusCache.Lock()
	if providerStatusCache.inFlight[workDir] {
		providerStatusCache.Unlock()
		return
	}
	providerStatusCache.inFlight[workDir] = true
	providerStatusCache.Unlock()

	go func() {
		defer func() {
			providerStatusCache.Lock()
			delete(providerStatusCache.inFlight, workDir)
			providerStatusCache.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		providers, err := liveProviderInfos(ctx, workDir)
		if err != nil {
			return
		}
		rows := providerStatusesFromInfos(providers, nil, time.Now().UTC())
		storeProviderRows(workDir, rows)
	}()
}

func cachedProviderRows(workDir string) []*ProviderStatus {
	providerStatusCache.Lock()
	defer providerStatusCache.Unlock()
	return cloneProviderRows(providerStatusCache.rows[workDir])
}

func storeProviderRows(workDir string, rows []*ProviderStatus) {
	providerStatusCache.Lock()
	defer providerStatusCache.Unlock()
	providerStatusCache.rows[workDir] = cloneProviderRows(rows)
}

func providerStatusesFromInfos(providers []agentlib.ProviderInfo, entries []agent.SessionIndexEntry, now time.Time) []*ProviderStatus {
	lastChecked := now.UTC().Format(time.RFC3339)
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
			Reachable:         providerReachable(p),
			Detail:            providerDetail(p),
			ModelCount:        p.ModelCount,
			IsDefault:         p.IsDefault,
			LastCheckedAt:     strPtr(lastChecked),
			DefaultForProfile: defaultProfilesForEndpoint(p),
		}
		if ps.ProviderType == "" {
			ps.ProviderType = "endpoint"
		}
		if p.CooldownState != nil && !p.CooldownState.Until.IsZero() {
			s := p.CooldownState.Until.UTC().Format(time.RFC3339)
			ps.CooldownUntil = &s
		}
		ps.Usage = buildUsage(entries, p.Name, agent.MatchProvider, now)
		ps.Quota = quotaFromProviderInfo(p)
		ps.Sparkline = buildSparkline(entries, p.Name, agent.MatchProvider, now)
		results = append(results, ps)
	}
	return results
}

func providerRowsWithUsage(rows []*ProviderStatus, entries []agent.SessionIndexEntry, now time.Time) []*ProviderStatus {
	out := cloneProviderRows(rows)
	for _, row := range out {
		row.Usage = buildUsage(entries, row.Name, agent.MatchProvider, now)
		row.Sparkline = buildSparkline(entries, row.Name, agent.MatchProvider, now)
	}
	return out
}

// buildSparkline returns a 24-element slice of hourly token totals for the
// last 24 hours (oldest-first). Returns nil when fewer than 6 of the 24
// hourly buckets have non-zero token counts — the UI uses that floor to
// suppress noisy single-spike sparklines (FEAT-014 AC 2: "Sparkline renders
// when ≥6 hourly buckets of usage are available").
func buildSparkline(entries []agent.SessionIndexEntry, name string, kind agent.UsageMatchKind, now time.Time) []int {
	if len(entries) == 0 {
		return nil
	}
	buckets := agent.BucketUsage(entries, name, kind, now, 1, time.Hour)
	totals := make([]int, len(buckets))
	nonEmpty := 0
	for i, b := range buckets {
		totals[i] = b.Tokens
		if b.Tokens > 0 {
			nonEmpty++
		}
	}
	if nonEmpty < 6 {
		return nil
	}
	return totals
}

func cloneProviderRows(rows []*ProviderStatus) []*ProviderStatus {
	if len(rows) == 0 {
		return nil
	}
	out := make([]*ProviderStatus, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		clone := *row
		if row.DefaultForProfile != nil {
			clone.DefaultForProfile = append([]string(nil), row.DefaultForProfile...)
		}
		if row.Sparkline != nil {
			clone.Sparkline = append([]int(nil), row.Sparkline...)
		}
		out = append(out, &clone)
	}
	return out
}

// sessionIndexEntries reads the project's session index for all available
// shards. Errors are swallowed — a missing index is a normal "no data" state.
func (r *queryResolver) sessionIndexEntries(ctx context.Context) []agent.SessionIndexEntry {
	logDir := agent.SessionLogDirForWorkDir(r.workingDir(ctx))
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
	// FEAT-014 no-fabrication: when the session index has no rows attributable
	// to this name, return nil so the UI renders "not reported" instead of
	// fabricated "0 / 0" counts.
	if counts.TokensLastHour == 0 && counts.TokensLast24h == 0 &&
		counts.RequestsLastHour == 0 && counts.RequestsLast24h == 0 {
		return nil
	}
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
// When a rate-limit signal has been captured for this harness (via
// RecordHarnessRateLimit — typically on the server's harness-dispatch path
// after parsing response headers with agent.ParseRateLimitHeaders), that
// signal takes precedence because it carries absolute token ceilings. The
// upstream HarnessInfo.Quota only exposes percent-used windows without an
// absolute ceiling. Returns nil when nothing usable is published.
func quotaFromHarnessInfo(info agentlib.HarnessInfo) *ProviderQuota {
	if sig, ok := LookupHarnessRateLimit(info.Name); ok {
		if q := QuotaFromRateLimitSignal(sig); q != nil {
			return q
		}
	}
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

func providerReachable(p agentlib.ProviderInfo) bool {
	return strings.EqualFold(strings.TrimSpace(p.Status), "connected")
}

func providerDetail(p agentlib.ProviderInfo) string {
	if p.LastError != nil && strings.TrimSpace(p.LastError.Detail) != "" {
		return p.LastError.Detail
	}
	for _, ep := range p.EndpointStatus {
		if ep.LastError != nil && strings.TrimSpace(ep.LastError.Detail) != "" {
			return ep.LastError.Detail
		}
		if strings.TrimSpace(ep.Status) != "" && !strings.EqualFold(ep.Status, p.Status) {
			return ep.Status
		}
	}
	if strings.TrimSpace(p.Status) != "" {
		if strings.EqualFold(p.Status, "unknown") {
			return "not checked yet"
		}
		return p.Status
	}
	return "not reported"
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

func harnessDetail(info agentlib.HarnessInfo) string {
	if info.LastError != nil && strings.TrimSpace(info.LastError.Detail) != "" {
		return info.LastError.Detail
	}
	if strings.TrimSpace(info.Error) != "" {
		return info.Error
	}
	if info.Available && strings.TrimSpace(info.Path) != "" {
		return info.Path
	}
	if info.Available {
		return "available"
	}
	return "binary not found"
}

func harnessModelCount(_ agentlib.HarnessInfo) int {
	// Harness-reported model counts flow through a separate model-discovery
	// path; leave 0 until that surface is exposed to avoid fabricating a
	// number from capability flags.
	return 0
}
