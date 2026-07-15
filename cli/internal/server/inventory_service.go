package server

import (
	"context"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	agentlib "github.com/easel/fizeau"
)

// inventoryService is the complete Fizeau surface server inventory handlers
// may use. In particular it intentionally omits ResolveRoute: inventory can
// report listing facts and factual route status, but must never predict a
// future execution route.
type inventoryService interface {
	ListHarnesses(context.Context) ([]agentlib.HarnessInfo, error)
	ListProviders(context.Context) ([]agentlib.ProviderInfo, error)
	ListModels(context.Context, agentlib.ModelFilter) ([]agentlib.ModelInfo, error)
	RouteStatus(context.Context) (*agentlib.RouteStatusReport, error)
}

type inventoryServiceFactory func(context.Context, string) (inventoryService, error)
type inventoryServiceFactoryKey struct{}

// withInventoryServiceFactory installs a request-scoped test seam. Keeping the
// factory on the request context prevents one test or project request from
// replacing inventory behavior process-wide.
func withInventoryServiceFactory(ctx context.Context, factory inventoryServiceFactory) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if factory == nil {
		return ctx
	}
	return context.WithValue(ctx, inventoryServiceFactoryKey{}, factory)
}

func inventoryServiceForRequest(ctx context.Context, workDir string) (inventoryService, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if factory, ok := ctx.Value(inventoryServiceFactoryKey{}).(inventoryServiceFactory); ok && factory != nil {
		return factory(ctx, workDir)
	}
	return agent.NewServiceFromWorkDirCtx(ctx, workDir)
}
