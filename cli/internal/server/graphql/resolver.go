package graphql

// THIS CODE WILL BE UPDATED WITH SCHEMA CHANGES. PREVIOUS IMPLEMENTATION FOR SCHEMA CHANGES WILL BE KEPT IN THE COMMENT SECTION. IMPLEMENTATION FOR UNCHANGED SCHEMA WILL BE KEPT.

import (
	"context"
	"fmt"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// NewResolver constructs a Resolver with mandatory fields validated.
// Returns an error if state is nil or workingDir is empty, ensuring callers
// cannot construct a resolver that would panic on first use.
func NewResolver(state StateProvider, workingDir string) (*Resolver, error) {
	if state == nil {
		return nil, fmt.Errorf("resolver: state provider is required")
	}
	if workingDir == "" {
		return nil, fmt.Errorf("resolver: working directory is required")
	}
	return &Resolver{
		State:      state,
		WorkingDir: workingDir,
	}, nil
}

// BeadLifecycleSubscriber can subscribe to live lifecycle events from a bead store.
// bead.WatcherHub satisfies this interface.
type BeadLifecycleSubscriber interface {
	SubscribeLifecycle(projectID string) (<-chan bead.LifecycleEvent, func())
}

type Resolver struct {
	State      StateProvider
	WorkingDir string
	Workers    ProgressSubscriber
	BeadBus    BeadLifecycleSubscriber
	// ExecLogs provides execution run log retrieval for the executionEvidence
	// subscription. If nil, that subscription returns an error.
	ExecLogs ExecLogProvider
	// CoordMetrics provides coordinator metrics snapshots for the
	// coordinatorMetrics subscription. If nil, that subscription returns an error.
	CoordMetrics CoordinatorMetricsProvider
	// MetricsPollInterval controls how often CoordinatorMetrics polls for
	// changes. Defaults to 1 second when zero.
	MetricsPollInterval time.Duration
}

// BeadsReady is the resolver for the beadsReady field.
func (r *queryResolver) BeadsReady(ctx context.Context, first *int, after *string, last *int, before *string) (*BeadConnection, error) {
	panic("not implemented")
}

// BeadsBlocked is the resolver for the beadsBlocked field.
func (r *queryResolver) BeadsBlocked(ctx context.Context, first *int, after *string, last *int, before *string) (*BeadConnection, error) {
	panic("not implemented")
}

// BeadsStatus is the resolver for the beadsStatus field.
func (r *queryResolver) BeadsStatus(ctx context.Context) (*BeadStatusCounts, error) {
	panic("not implemented")
}

// BeadDepTree is the resolver for the beadDepTree field.
func (r *queryResolver) BeadDepTree(ctx context.Context, beadID string) (string, error) {
	panic("not implemented")
}

// Bead is the resolver for the bead field.
func (r *queryResolver) Bead(ctx context.Context, id string) (*Bead, error) {
	panic("not implemented")
}

// DocumentByPath is the resolver for the documentByPath field.
func (r *queryResolver) DocumentByPath(ctx context.Context, path string) (*Document, error) {
	panic("not implemented")
}

// DocStale is the resolver for the docStale field.
func (r *queryResolver) DocStale(ctx context.Context) ([]*StaleReason, error) {
	panic("not implemented")
}

// DocDeps is the resolver for the docDeps field.
func (r *queryResolver) DocDeps(ctx context.Context, documentID string) ([]string, error) {
	panic("not implemented")
}

// DocDependents is the resolver for the docDependents field.
func (r *queryResolver) DocDependents(ctx context.Context, documentID string) ([]string, error) {
	panic("not implemented")
}

// DocHistory is the resolver for the docHistory field.
func (r *queryResolver) DocHistory(ctx context.Context, documentID string, first *int, after *string, last *int, before *string) (*CommitConnection, error) {
	panic("not implemented")
}

// DocDiff is the resolver for the docDiff field.
func (r *queryResolver) DocDiff(ctx context.Context, documentID string, ref *string) (string, error) {
	panic("not implemented")
}

// Doc is the resolver for the doc field.
func (r *queryResolver) Doc(ctx context.Context, id string) (*Document, error) {
	panic("not implemented")
}

// Search is the resolver for the search field.
func (r *queryResolver) Search(ctx context.Context, query string, first *int, after *string, last *int, before *string) (*SearchResultConnection, error) {
	panic("not implemented")
}

// Health is the resolver for the health field.
func (r *queryResolver) Health(ctx context.Context) (*HealthStatus, error) {
	panic("not implemented")
}

// Ready is the resolver for the ready field.
func (r *queryResolver) Ready(ctx context.Context) (*ReadyStatus, error) {
	panic("not implemented")
}

// MetricsSummary is the resolver for the metricsSummary field.
func (r *queryResolver) MetricsSummary(ctx context.Context, since *string) (*AggregateSummary, error) {
	panic("not implemented")
}

// MetricsCost is the resolver for the metricsCost field.
func (r *queryResolver) MetricsCost(ctx context.Context, since *string, bead *string, feature *string) (*CostReport, error) {
	panic("not implemented")
}

// MetricsCycleTime is the resolver for the metricsCycleTime field.
func (r *queryResolver) MetricsCycleTime(ctx context.Context, since *string) (*CycleTimeReport, error) {
	panic("not implemented")
}

// MetricsRework is the resolver for the metricsRework field.
func (r *queryResolver) MetricsRework(ctx context.Context, since *string) (*ReworkReport, error) {
	panic("not implemented")
}

// Providers is the resolver for the providers field.
func (r *queryResolver) Providers(ctx context.Context) ([]*Provider, error) {
	panic("not implemented")
}

// Provider is the resolver for the provider field.
func (r *queryResolver) Provider(ctx context.Context, name string) (*Provider, error) {
	panic("not implemented")
}

// BeadLifecycle is the resolver for the beadLifecycle subscription.
// Implemented in resolver_sub_bead.go.

// ExecutionEvidence is the resolver for the executionEvidence subscription.
// Implemented in resolver_sub_exec.go.

// CoordinatorMetrics is the resolver for the coordinatorMetrics subscription.
// Implemented in resolver_sub_exec.go.

// Mutation returns MutationResolver implementation.
func (r *Resolver) Mutation() MutationResolver { return &mutationResolver{r} }

// Query returns QueryResolver implementation.
func (r *Resolver) Query() QueryResolver { return &queryResolver{r} }

// Subscription returns SubscriptionResolver implementation.
func (r *Resolver) Subscription() SubscriptionResolver { return &subscriptionResolver{r} }

type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
type subscriptionResolver struct{ *Resolver }
