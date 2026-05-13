package graphql

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
)

// workingDirKey is the context key used to thread a per-request WorkingDir
// through GraphQL resolvers. LAYER 2 of the GraphQL multi-project fix
// (ddx-055e8d32) lets the scoped /api/projects/{project}/graphql route inject
// the resolved project's WorkingDir into the request context so the
// singleton resolver constructed at server start can serve any project
// without per-request reconstruction.
type workingDirKey struct{}

// WithWorkingDir returns ctx with workingDir attached for downstream resolver
// access. The server's GraphQL HTTP handler MUST call this before delegating
// to the gqlgen handler so resolvers read the request-scoped project root
// rather than the resolver struct's fallback default.
func WithWorkingDir(ctx context.Context, workingDir string) context.Context {
	if workingDir == "" {
		return ctx
	}
	return context.WithValue(ctx, workingDirKey{}, workingDir)
}

// WorkingDirFromContext returns the WorkingDir previously attached via
// WithWorkingDir, or the empty string if none was set. Callers should prefer
// (*Resolver).workingDir(ctx) which falls back to the resolver's default.
func WorkingDirFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(workingDirKey{}).(string); ok {
		return v
	}
	return ""
}

// workingDir returns the request-scoped WorkingDir (via WithWorkingDir) when
// present, falling back to r.WorkingDir. All resolver methods with access to
// ctx should call this rather than reading r.WorkingDir directly so the same
// resolver instance can serve multiple projects safely.
func (r *Resolver) workingDir(ctx context.Context) string {
	if dir := WorkingDirFromContext(ctx); dir != "" {
		return dir
	}
	return r.WorkingDir
}

// BeadLifecycleSubscriber can subscribe to live lifecycle events from a bead store.
// bead.WatcherHub satisfies this interface.
type BeadLifecycleSubscriber interface {
	SubscribeLifecycle(projectID string) (<-chan bead.LifecycleEvent, func())
}

// ExecuteLoopWaker signals running execute-loop workers bound to a project
// to skip their idle-poll sleep and re-scan the ready queue. The
// operatorPromptApprove and operatorPromptSubmit (auto-approve) resolvers
// call WakeProject after a successful proposed → open transition so the
// bead is claimed without waiting for the next poll tick. The server's
// WorkerManager satisfies this interface; tests inject a stub to assert
// the wake call without spinning a real worker.
type ExecuteLoopWaker interface {
	WakeProject(projectRoot string) int
}

// ActionDispatcher starts backend workers for GraphQL action mutations.
// The server package supplies the production implementation so this package
// does not import the outer server package.
type ActionDispatcher interface {
	DispatchWorker(ctx context.Context, kind string, projectRoot string, args *string) (*WorkerDispatchResult, error)
	DispatchPlugin(ctx context.Context, projectRoot string, name string, action string, scope string) (*PluginDispatchResult, error)
	StopWorker(ctx context.Context, id string) (*WorkerLifecycleResult, error)
}

type Resolver struct {
	State      StateProvider
	WorkingDir string
	Workers    ProgressSubscriber
	BeadBus    BeadLifecycleSubscriber
	Actions    ActionDispatcher
	// ExecLogs provides execution run log retrieval for the executionEvidence
	// subscription. If nil, that subscription returns an error.
	ExecLogs ExecLogProvider
	// CoordMetrics provides coordinator metrics snapshots for the
	// coordinatorMetrics subscription. If nil, that subscription returns an error.
	CoordMetrics CoordinatorMetricsProvider
	// MetricsPollInterval controls how often CoordinatorMetrics polls for
	// changes. Defaults to 1 second when zero.
	MetricsPollInterval time.Duration
	// CSRFTokens validates the X-CSRF-Token header on operatorPromptSubmit
	// (and other future write mutations that require CSRF protection).
	// Nil here causes operatorPromptSubmit to reject every call with the
	// CSRF error rather than failing open.
	CSRFTokens CSRFTokenStore
	// OperatorPromptIdempotency deduplicates operatorPromptSubmit calls by
	// idempotency key within a 24-hour window. Server callers must wire a
	// process-wide instance (NewMemoryIdempotencyCache).
	OperatorPromptIdempotency IdempotencyCache
	// OperatorPromptAutoApproveAllowlist is the per-project list of localhost
	// identity actors that may auto-approve their own operator-prompt
	// submissions (operatorPromptSubmit input.autoApprove=true) and that may
	// invoke operatorPromptApprove. The locked Story 15 decision restricts
	// approval to configured-localhost identities — ts-net identities are
	// NEVER eligible regardless of the allowlist contents. The literal
	// sentinel "localhost" in the list matches any localhost actor; otherwise
	// the entry must equal the actor string produced by operatorPromptIdentity
	// (e.g. "localhost:127.0.0.1:55812"). An empty allowlist disables both
	// auto-approve and the manual approve mutation for this project.
	OperatorPromptAutoApproveAllowlist []string
	// PromptCapBytes caps the prompt body size accepted by
	// operatorPromptSubmit. When zero the resolver falls back to
	// DefaultOperatorPromptCapBytes (= evidence.DefaultMaxPromptBytes), which
	// matches the inline-prompt cap on /api/agent/run.
	PromptCapBytes int
	// BuildSHA is the server build commit recorded on the operator-prompt
	// audit event so the immutable first event captures the binary version
	// that accepted the submission. Empty -> "unknown".
	BuildSHA string
	// NodeID is the receiving server's stable node ID, captured on the
	// operator-prompt audit event. Used together with the originating
	// X-Tailscale-Node header (which identifies the *peer*'s node) so the
	// audit trail records both ends of the trust attestation.
	NodeID string
	// ExecuteLoopWaker, when non-nil, is signalled by the operator-prompt
	// approve / auto-approve resolvers immediately after a successful
	// proposed → open transition so a running execute-loop worker bound to
	// the project drops its idle-poll sleep and claims the bead in the
	// current tick. Nil → resolver simply skips the wake (the next tick
	// will pick the bead up after PollInterval).
	ExecuteLoopWaker ExecuteLoopWaker
	// Federation, when non-nil, supplies the spoke registry and fan-out
	// client used by the federationNodes / federated{Beads,Runs,Projects}
	// query resolvers. Nil → those queries return empty lists (the default
	// for non-hub servers).
	Federation FederationProvider
	// RunRequeueIdempotency deduplicates runRequeue mutation calls by
	// idempotency key. Concurrent and repeat calls with the same key return
	// the same originating bead and never enqueue a second time. Server
	// callers wire a process-wide instance (NewMemoryIdempotencyCache); when
	// nil, the resolver returns an error rather than silently failing open.
	RunRequeueIdempotency IdempotencyCache
	// runRequeueMu serializes the lookup+reserve+store steps of the
	// runRequeue mutation so that concurrent calls with the same
	// idempotencyKey collapse to a single re-queue (atomic claims).
	runRequeueMu sync.Mutex
	// ReportedWorkers, when non-nil, supplies the worker_ingest derived view
	// for the reportedWorkers query. Nil → resolver returns an empty list.
	ReportedWorkers ReportedWorkersProvider
}

// NewResolver returns a zero-initialized resolver root.
// Server construction wires the concrete collaborators after creating it.
func NewResolver() *Resolver { return &Resolver{} }

// ReportedWorkersProvider returns a snapshot of the worker_ingest derived
// view (ADR-022 rev 5 §Worker-server interface), already classified by
// freshness state. The server package supplies the production implementation.
type ReportedWorkersProvider interface {
	GetReportedWorkers() []*ReportedWorker
}

// Mutation returns MutationResolver implementation.
func (r *Resolver) Mutation() MutationResolver { return &mutationResolver{r} }

// Query returns QueryResolver implementation.
func (r *Resolver) Query() QueryResolver { return &queryResolver{r} }

// Subscription returns SubscriptionResolver implementation.
func (r *Resolver) Subscription() SubscriptionResolver { return &subscriptionResolver{r} }

type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
type artifactResolver struct{ *Resolver }
type subscriptionResolver struct{ *Resolver }

// ─── Unimplemented query stubs ─────────────────────────────────────────────

// BeadsStatus is the resolver for the beadsStatus field.
// Implemented in resolver_beads.go.

// BeadDepTree is the resolver for the beadDepTree field.
func (r *queryResolver) BeadDepTree(ctx context.Context, beadID string) (string, error) {
	panic("not implemented")
}

// Bead is the resolver for the bead field.
func (r *queryResolver) Bead(ctx context.Context, id string) (*Bead, error) {
	snap, ok := r.State.GetBeadSnapshot(id)
	if !ok {
		return nil, nil
	}
	return beadFromSnapshot(*snap), nil
}

// DocumentByPath is the resolver for the documentByPath field.
// Implemented in resolver_documents.go.

// DocStale is the resolver for the docStale field.
func (r *queryResolver) DocStale(ctx context.Context) ([]*StaleReason, error) {
	graph, err := docgraph.BuildGraphWithConfig(r.workingDir(ctx))
	if err != nil {
		return nil, fmt.Errorf("building document graph: %w", err)
	}
	stale := graph.StaleDocs()
	out := make([]*StaleReason, 0, len(stale))
	for _, reason := range stale {
		out = append(out, &StaleReason{
			ID:      reason.ID,
			Path:    reason.Path,
			Reasons: append([]string(nil), reason.Reasons...),
		})
	}
	return out, nil
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

// ProviderStatuses is the resolver for the providerStatuses field.
// Implemented in resolver_providers.go.

// DefaultRouteStatus is the resolver for the defaultRouteStatus field.
// Implemented in resolver_providers.go.

// BeadLifecycle is the resolver for the beadLifecycle subscription.
// Implemented in resolver_sub_bead.go.

// ExecutionEvidence is the resolver for the executionEvidence subscription.
// Implemented in resolver_sub_exec.go.

// CoordinatorMetrics is the resolver for the coordinatorMetrics subscription.
// Implemented in resolver_sub_exec.go.

// Run and Runs are implemented in resolver_runs.go.
