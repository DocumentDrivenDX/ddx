package graphql

import (
	"context"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	agentlib "github.com/easel/fizeau"
)

// inventoryService intentionally exposes only Fizeau listing operations.
// GraphQL inventory must not resolve or predict a future execution route.
type inventoryService interface {
	ListHarnesses(context.Context) ([]agentlib.HarnessInfo, error)
	ListProviders(context.Context) ([]agentlib.ProviderInfo, error)
	ListModels(context.Context, agentlib.ModelFilter) ([]agentlib.ModelInfo, error)
}

type inventoryServiceFactory func(context.Context, string) (inventoryService, error)
type inventoryServiceFactoryKey struct{}

// withInventoryServiceFactory installs a request-scoped package-local test
// seam, avoiding mutable process-wide provider registries in concurrent tests
// and multi-project servers.
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
