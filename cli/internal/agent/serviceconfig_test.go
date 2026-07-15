package agent

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceConstructorsUseFizeauProjectConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	for _, name := range []string{"FIZEAU_PROVIDER", "FIZEAU_BASE_URL", "FIZEAU_API_KEY", "FIZEAU_MODEL"} {
		t.Setenv(name, "")
	}

	globalDir := filepath.Join(home, ".config", "fizeau")
	require.NoError(t, os.MkdirAll(globalDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte(`providers:
  global-provider:
    type: anthropic
    api_key: global-test-key
default: global-provider
`), 0o600))

	type constructor func(string) (agentlib.FizeauService, error)
	constructors := []struct {
		name string
		new  constructor
	}{
		{name: "execution", new: NewServiceFromWorkDir},
		{name: "context-scoped", new: func(workDir string) (agentlib.FizeauService, error) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return NewServiceFromWorkDirCtx(ctx, workDir)
		}},
		{name: "preflight", new: NewPreflightServiceFromWorkDir},
	}

	for i, tc := range constructors {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			projectProvider := fmt.Sprintf("project-provider-%d", i)
			fizeauDir := filepath.Join(workDir, ".fizeau")
			require.NoError(t, os.MkdirAll(fizeauDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(fizeauDir, "config.yaml"), []byte(fmt.Sprintf(`providers:
  %s:
    type: anthropic
    api_key: project-test-key
default: %s
`, projectProvider, projectProvider)), 0o600))

			// Generic DDx execution config must not become a provider registry for
			// any execution-facing service constructor.
			ddxDir := filepath.Join(workDir, ddxroot.DirName)
			require.NoError(t, os.MkdirAll(ddxDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"
agent:
  timeout_ms: 300000
`), 0o600))

			svc, err := tc.new(workDir)
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			providers, err := svc.ListProviders(ctx)
			require.NoError(t, err)
			names := make([]string, 0, len(providers))
			for _, provider := range providers {
				names = append(names, provider.Name)
			}
			assert.Contains(t, names, "global-provider", "Fizeau global config must remain merged")
			assert.Contains(t, names, projectProvider, "constructor must load this workdir's Fizeau project config")
			assert.NotContains(t, names, "openai-127-0-0-1-1", "DDx generic execution config must not synthesize providers")
		})
	}
}

func TestNoExecutionServiceConfigSynthesis(t *testing.T) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, "serviceconfig.go", nil, parser.SkipObjectResolution)
	require.NoError(t, err)

	executionConstructors := map[string]bool{
		"NewServiceFromWorkDir":          false,
		"NewServiceFromWorkDirCtx":       false,
		"NewPreflightServiceFromWorkDir": false,
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if _, ok := executionConstructors[fn.Name.Name]; !ok {
			continue
		}
		executionConstructors[fn.Name.Name] = true
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.Ident:
				assert.NotContains(t, []string{
					"ServiceConfig",
					"serviceConfigFromDDxEndpointsNoFilter",
					"newEndpointServiceConfigWithoutLiveFilter",
					"endpointProviderEntry",
					"ResolveProviderRequestTimeout",
					"DefaultProviderRequestTimeout",
				}, n.Name, "%s must not synthesize execution provider config", fn.Name.Name)
			case *ast.SelectorExpr:
				if fn.Name.Name != "NewPreflightServiceFromWorkDir" {
					assert.NotEqual(t, "ServiceConfig", n.Sel.Name, "%s must leave ServiceConfig nil", fn.Name.Name)
				}
			}
			return true
		})
	}
	for name, found := range executionConstructors {
		assert.True(t, found, "missing execution constructor %s", name)
	}

	serviceRun, err := os.ReadFile("service_run.go")
	require.NoError(t, err)
	for _, forbidden := range []string{"ResolveProviderRequestTimeout", "DefaultProviderRequestTimeout", "endpointRequestTimeout", "appendProviderTimeoutHint"} {
		assert.NotContains(t, string(serviceRun), forbidden, "execution dispatch must not retain DDx provider policy")
	}
	resolved, err := os.ReadFile(filepath.Join("..", "config", "resolved.go"))
	require.NoError(t, err)
	assert.NotContains(t, string(resolved), "r.model = agent.Model", "project model must never become an execution constraint")
}
