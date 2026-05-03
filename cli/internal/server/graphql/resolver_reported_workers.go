package graphql

import "context"

// ReportedWorkers is the resolver for the reportedWorkers field. Returns an
// empty list when no provider is wired (e.g. the perf or test harness builds a
// resolver without the ingest registry).
func (r *queryResolver) ReportedWorkers(ctx context.Context) ([]*ReportedWorker, error) {
	provider := r.Resolver.ReportedWorkers
	if provider == nil {
		return []*ReportedWorker{}, nil
	}
	out := provider.GetReportedWorkers()
	if out == nil {
		return []*ReportedWorker{}, nil
	}
	return out, nil
}
