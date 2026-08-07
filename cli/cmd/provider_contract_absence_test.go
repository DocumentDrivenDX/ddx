package cmd

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/easel/fizeau"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestDDXNeverInstallsProviderShimOrLaunchWrapper(t *testing.T) {
	stub := &noShimTestService{}
	agent.SetServiceRunFactory(func(string) (agentlib.FizeauService, error) {
		return stub, nil
	})
	t.Cleanup(func() { agent.SetServiceRunFactory(nil) })

	initialPATH := os.Getenv("PATH")
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{Model: "haiku"}).Resolve(config.CLIOverrides{
		Harness: "agent",
		Model:   "haiku",
	})

	_, err := agent.RunWithConfigViaService(context.Background(), t.TempDir(), rcfg, agent.AgentRunRuntime{
		Prompt: "test",
	})
	require.NoError(t, err)
	require.True(t, stub.executeCalled)
	require.Equal(t, initialPATH, os.Getenv("PATH"))

	root := NewCommandFactory(t.TempDir()).NewRootCommand()
	found := false
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Name() == "__provider-launch" {
			found = true
			return
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
	require.False(t, found, "root command tree must not expose a provider-launch entrypoint")
}

type noShimTestService struct {
	executeCalled bool
}

func (s *noShimTestService) Execute(ctx context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	s.executeCalled = true
	ch := make(chan agentlib.ServiceEvent, 1)
	ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok"}`)}
	close(ch)
	return ch, nil
}

func (*noShimTestService) Continue(context.Context, agentlib.ServiceContinuationRequest) (<-chan agentlib.ServiceEvent, error) {
	return nil, agentlib.ErrContinuationUnsupported
}

func (*noShimTestService) PreparePortableRuntime(context.Context, agentlib.PortableRuntimeRequest) (*agentlib.PortableRuntimeBundle, error) {
	return nil, agentlib.ErrPortableRuntimeClosureIncomplete
}

func (*noShimTestService) TailSessionLog(context.Context, string) (<-chan agentlib.ServiceEvent, error) {
	ch := make(chan agentlib.ServiceEvent)
	close(ch)
	return ch, nil
}

func (*noShimTestService) ListHarnesses(context.Context) ([]agentlib.HarnessInfo, error) {
	return []agentlib.HarnessInfo{{Name: "agent", Available: true}}, nil
}

func (*noShimTestService) ListProviders(context.Context) ([]agentlib.ProviderInfo, error) {
	return nil, nil
}

func (*noShimTestService) ListModels(context.Context, agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	return nil, nil
}

func (*noShimTestService) ListPolicies(context.Context) ([]agentlib.PolicyInfo, error) {
	return nil, nil
}

func (*noShimTestService) HealthCheck(context.Context, agentlib.HealthTarget) error {
	return nil
}

func (*noShimTestService) ResolveRoute(context.Context, agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
	return nil, nil
}

func (*noShimTestService) RecordRouteAttempt(context.Context, agentlib.RouteAttempt) error {
	return nil
}

func (*noShimTestService) RouteStatus(context.Context) (*agentlib.RouteStatusReport, error) {
	return nil, nil
}

func (*noShimTestService) UsageReport(context.Context, agentlib.UsageReportOptions) (*agentlib.UsageReport, error) {
	return nil, nil
}

func (*noShimTestService) ListSessionLogs(context.Context) ([]agentlib.SessionLogEntry, error) {
	return nil, nil
}

func (*noShimTestService) WriteSessionLog(context.Context, string, io.Writer) error {
	return nil
}

func (*noShimTestService) ReplaySession(context.Context, string, io.Writer) error {
	return nil
}
