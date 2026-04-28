package agent

// prompt_ingress_oversize_test.go covers FEAT-022 §8 / Stage D1: every
// agent-side --prompt file reader must hard-fail on oversize with an
// actionable error naming the source path, observed size, configured
// cap, and the config key that adjusts the cap.

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// promptIngressTestCap is the cap installed during oversize-fixture tests
// so each subtest can write a small file (2 × cap) instead of multi-MB
// fixtures. The test installs this via t.Cleanup on promptFileCapBytes.
const promptIngressTestCap = 1024

// installSmallPromptCap lowers promptFileCapBytes for the duration of the
// test and restores the original cap afterwards.
func installSmallPromptCap(t *testing.T) {
	t.Helper()
	prev := promptFileCapBytes
	promptFileCapBytes = promptIngressTestCap
	t.Cleanup(func() { promptFileCapBytes = prev })
}

// writeOversizeFixture creates a 2 × promptIngressTestCap file in a
// temp directory and returns its absolute path.
func writeOversizeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "huge-prompt.bin")
	body := make([]byte, promptIngressTestCap*2)
	for i := range body {
		body[i] = 'x'
	}
	if err := os.WriteFile(p, body, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return p
}

// assertOversizeErrorMessage verifies the four AC substrings: fixture
// path, observed size, cap, and the config-key hint.
func assertOversizeErrorMessage(t *testing.T, err error, fixturePath string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected non-nil oversize error")
	}
	msg := err.Error()
	observed := fmt.Sprintf("%d", promptIngressTestCap*2)
	cap := fmt.Sprintf("%d", promptIngressTestCap)
	for _, want := range []string{fixturePath, observed, cap, "evidence_caps.max_prompt_bytes"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q missing required substring %q", msg, want)
		}
	}
}

// TestPromptIngressOversize is the named acceptance gate for Stage D1.
// One subtest per in-scope site.
func TestPromptIngressOversize(t *testing.T) {
	t.Run("runner_resolvePrompt", func(t *testing.T) {
		installSmallPromptCap(t)
		fixture := writeOversizeFixture(t)
		r := NewRunner(Config{})
		var runnerOpts RunOptions
		runnerOpts.PromptFile = fixture
		_, err := r.resolvePrompt(runnerOpts)
		assertOversizeErrorMessage(t, err, fixture)
	})

	t.Run("compare_defaultResolvePromptForCompare", func(t *testing.T) {
		installSmallPromptCap(t)
		fixture := writeOversizeFixture(t)
		_, err := defaultResolvePromptForCompare(AgentRunRuntime{PromptFile: fixture})
		assertOversizeErrorMessage(t, err, fixture)
	})

	t.Run("compare_benchmarkSuiteReader", func(t *testing.T) {
		installSmallPromptCap(t)
		fixture := writeOversizeFixture(t)
		suite := &BenchmarkSuite{
			Name:    "test",
			Version: "v1",
			Prompts: []BenchmarkPrompt{{ID: "p1", Name: "p1", PromptFile: fixture}},
		}
		runCompareCalls := 0
		_, err := RunBenchmarkWith(func(CompareRuntime) (*ComparisonRecord, error) {
			runCompareCalls++
			return nil, fmt.Errorf("runCompare must not be called when prompt-file read fails")
		}, suite)
		if runCompareCalls != 0 {
			t.Errorf("runCompare invoked %d times; oversize prompt file must short-circuit", runCompareCalls)
		}
		assertOversizeErrorMessage(t, err, fixture)
	})

	t.Run("service_run_RunViaServiceWith", func(t *testing.T) {
		installSmallPromptCap(t)
		fixture := writeOversizeFixture(t)
		svc := &promptIngressStubAgent{}
		var svcOpts RunOptions
		svcOpts.Harness = "agent"
		svcOpts.PromptFile = fixture
		_, err := RunViaServiceWith(context.Background(), svc, t.TempDir(), svcOpts)
		if svc.calls != 0 {
			t.Errorf("svc.Execute invoked %d times; oversize prompt file must short-circuit before dispatch", svc.calls)
		}
		assertOversizeErrorMessage(t, err, fixture)
	})

	t.Run("execute_bead_buildPrompt", func(t *testing.T) {
		installSmallPromptCap(t)
		fixture := writeOversizeFixture(t)
		_, _, err := buildPrompt(t.TempDir(), &bead.Bead{ID: "ddx-test"}, nil, nil, "", fixture, "claude", "")
		assertOversizeErrorMessage(t, err, fixture)
	})
}

// promptIngressStubAgent is a minimal DdxAgent that records Execute
// invocations. Used to assert oversize prompt-file reads short-circuit
// before any provider dispatch.
type promptIngressStubAgent struct {
	calls int
}

func (s *promptIngressStubAgent) Execute(ctx context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	s.calls++
	ch := make(chan agentlib.ServiceEvent)
	close(ch)
	return ch, nil
}

func (s *promptIngressStubAgent) TailSessionLog(ctx context.Context, sessionID string) (<-chan agentlib.ServiceEvent, error) {
	ch := make(chan agentlib.ServiceEvent)
	close(ch)
	return ch, nil
}
func (s *promptIngressStubAgent) ListHarnesses(ctx context.Context) ([]agentlib.HarnessInfo, error) {
	return nil, nil
}
func (s *promptIngressStubAgent) ListProviders(ctx context.Context) ([]agentlib.ProviderInfo, error) {
	return nil, nil
}
func (s *promptIngressStubAgent) ListModels(ctx context.Context, filter agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	return nil, nil
}
func (s *promptIngressStubAgent) ListProfiles(ctx context.Context) ([]agentlib.ProfileInfo, error) {
	return nil, nil
}
func (s *promptIngressStubAgent) ResolveProfile(ctx context.Context, name string) (*agentlib.ResolvedProfile, error) {
	return nil, nil
}
func (s *promptIngressStubAgent) ProfileAliases(ctx context.Context) (map[string]string, error) {
	return nil, nil
}
func (s *promptIngressStubAgent) HealthCheck(ctx context.Context, target agentlib.HealthTarget) error {
	return nil
}
func (s *promptIngressStubAgent) ResolveRoute(ctx context.Context, req agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
	return nil, nil
}
func (s *promptIngressStubAgent) RecordRouteAttempt(ctx context.Context, attempt agentlib.RouteAttempt) error {
	return nil
}
func (s *promptIngressStubAgent) RouteStatus(ctx context.Context) (*agentlib.RouteStatusReport, error) {
	return nil, nil
}
func (s *promptIngressStubAgent) ListSessionLogs(ctx context.Context) ([]agentlib.SessionLogEntry, error) {
	return nil, nil
}
func (s *promptIngressStubAgent) WriteSessionLog(ctx context.Context, sessionID string, w io.Writer) error {
	return nil
}
func (s *promptIngressStubAgent) ReplaySession(ctx context.Context, sessionID string, w io.Writer) error {
	return nil
}
func (s *promptIngressStubAgent) UsageReport(ctx context.Context, opts agentlib.UsageReportOptions) (*agentlib.UsageReport, error) {
	return nil, nil
}

// TestPromptIngressEvidencePrimitiveUsage is the AST gate that prevents
// future drift: every os.ReadFile / ioutil.ReadFile call inside the
// in-scope agent files whose argument names a "*PromptFile" symbol
// must either route through evidence (readPromptFileBounded /
// evidence.ReadFileClamped) or carry an //evidence:allow-unbounded
// annotation.
func TestPromptIngressEvidencePrimitiveUsage(t *testing.T) {
	files := []string{
		"runner.go",
		"compare_adapter.go",
		"service_run.go",
		"execute_bead.go",
	}

	for _, src := range files {
		t.Run(src, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, src, nil, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse %s: %v", src, err)
			}

			hasAllow := func(node ast.Node) bool {
				nodePos := fset.Position(node.Pos())
				for _, cg := range file.Comments {
					cgEnd := fset.Position(cg.End())
					if cgEnd.Filename != nodePos.Filename {
						continue
					}
					if cgEnd.Line >= nodePos.Line-3 && cgEnd.Line < nodePos.Line {
						for _, c := range cg.List {
							if strings.Contains(c.Text, "evidence:allow-unbounded") {
								return true
							}
						}
					}
				}
				return false
			}

			var violations []string
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				ident, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				readFile := (ident.Name == "os" && sel.Sel.Name == "ReadFile") ||
					(ident.Name == "ioutil" && sel.Sel.Name == "ReadFile")
				if !readFile || len(call.Args) == 0 {
					return true
				}
				// Identify --prompt-file-style arguments. We match selector
				// expressions whose right-hand side is "PromptFile" (e.g.
				// opts.PromptFile, prompt.PromptFile). These are the symbols
				// Stage D1 routes through readPromptFileBounded.
				argSel, ok := call.Args[0].(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if argSel.Sel == nil || argSel.Sel.Name != "PromptFile" {
					return true
				}
				if !hasAllow(call) {
					p := fset.Position(call.Pos())
					violations = append(violations,
						fmt.Sprintf("%s: os.ReadFile(<*.PromptFile>) at line %d not routed through evidence", src, p.Line))
				}
				return true
			})

			if len(violations) > 0 {
				t.Errorf("FEAT-022 §8 violation(s):\n  %s",
					strings.Join(violations, "\n  "))
			}
		})
	}
}
