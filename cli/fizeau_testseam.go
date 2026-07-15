//go:build testseam

package main

// This file is compiled only into subprocess integration-test binaries. It
// bridges the exec boundary by installing Fizeau's public, build-tagged
// FakeProvider at CLI startup. Production DDx binaries cannot compile this
// file or reference FakeProvider.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	agentlib "github.com/easel/fizeau"
)

const fizeauTestPlanEnv = "DDX_FIZEAU_TEST_PLAN"

type fizeauTestPlan struct {
	SleepMS          int      `json:"sleep_ms,omitempty"`
	WritePath        string   `json:"write_path"`
	WriteContent     string   `json:"write_content"`
	CommitMessage    string   `json:"commit_message"`
	SeamLog          string   `json:"seam_log,omitempty"`
	TripwireBinDir   string   `json:"tripwire_bin_dir,omitempty"`
	TripwireNames    []string `json:"tripwire_names,omitempty"`
	TripwireSentinel string   `json:"tripwire_sentinel,omitempty"`
}

type fizeauTestSeamService struct {
	agentlib.FizeauService
}

var fizeauTestSeamInitErr error
var fizeauTestSeamToolsDir string

func init() {
	if strings.TrimSpace(os.Getenv(fizeauTestPlanEnv)) != "" {
		fizeauTestSeamInitErr = installFizeauTestSeamPATH()
	}
	agent.SetServiceRunFactory(newFizeauTestSeamService)
}

func newFizeauTestSeamService(string) (agentlib.FizeauService, error) {
	if fizeauTestSeamInitErr != nil {
		return nil, fizeauTestSeamInitErr
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	base, err := agentlib.New(agentlib.ServiceOptions{QuotaRefreshContext: ctx})
	if err != nil {
		return nil, err
	}
	return &fizeauTestSeamService{FizeauService: base}, nil
}

// installFizeauTestSeamPATH leaves the tagged subprocess only the executables
// its deterministic tool calls need. Fizeau remains free to inventory its
// environment, but no developer-installed provider CLI is present to probe or
// launch. The parent integration test removes the directory with its plan.
func installFizeauTestSeamPATH() error {
	allowed := make(map[string]string, 2)
	for _, name := range []string{"git", "sh"} {
		path, err := exec.LookPath(name)
		if err != nil {
			return fmt.Errorf("locate %s for Fizeau test-seam PATH: %w", name, err)
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("resolve %s for Fizeau test-seam PATH: %w", name, err)
		}
		allowed[name] = resolved
	}
	planPath := strings.TrimSpace(os.Getenv(fizeauTestPlanEnv))
	dir := filepath.Join(filepath.Dir(planPath), fmt.Sprintf(".fizeau-tools-%d", os.Getpid()))
	if err := os.Mkdir(dir, 0o700); err != nil {
		return fmt.Errorf("create Fizeau test-seam PATH: %w", err)
	}
	for name, target := range allowed {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("link %s into Fizeau test-seam PATH: %w", name, err)
		}
	}
	if err := os.Setenv("PATH", dir); err != nil {
		return fmt.Errorf("activate Fizeau test-seam PATH: %w", err)
	}
	fizeauTestSeamToolsDir = dir
	return nil
}

func (s *fizeauTestSeamService) Execute(ctx context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	plan, err := loadFizeauTestPlan()
	if err != nil {
		return nil, err
	}
	if err := appendFizeauTestSeamLog(plan.SeamLog, req); err != nil {
		return nil, err
	}

	var callIndex atomic.Int64
	tripwiresReady := make(chan struct{})
	fake := &agentlib.FakeProvider{
		Dynamic: func(agentlib.FakeRequest) (agentlib.FakeResponse, error) {
			<-tripwiresReady
			call := callIndex.Add(1)
			if req.Metadata[agent.DDXModeEnvKey] != agent.DDXModeBeadExecution {
				return agentlib.FakeResponse{
					Text:  `{"score":100,"rationale":"test seam","suggested_fixes":[],"waivers_applied":[]}`,
					Usage: agentlib.TokenUsage{Input: 1, Output: 1, Total: 2},
				}, nil
			}
			if call > 1 {
				return agentlib.FakeResponse{
					Text:  "completed by Fizeau test seam",
					Usage: agentlib.TokenUsage{Input: 1, Output: 1, Total: 2},
				}, nil
			}
			if plan.SleepMS > 0 {
				time.Sleep(time.Duration(plan.SleepMS) * time.Millisecond)
			}
			beadID := req.Metadata[agent.DDXBeadIDEnvKey]
			writePath := strings.ReplaceAll(plan.WritePath, "${DDX_BEAD_ID}", beadID)
			writeContent := strings.ReplaceAll(plan.WriteContent, "${DDX_BEAD_ID}", beadID)
			commitMessage := strings.ReplaceAll(plan.CommitMessage, "${DDX_BEAD_ID}", beadID)
			writeArgs, err := json.Marshal(map[string]string{
				"path":    writePath,
				"content": writeContent,
			})
			if err != nil {
				return agentlib.FakeResponse{}, err
			}
			bashArgs, err := json.Marshal(map[string]string{
				"command": "git add -- " + shellQuote(writePath) + " && git commit -m " + shellQuote(commitMessage),
			})
			if err != nil {
				return agentlib.FakeResponse{}, err
			}
			return agentlib.FakeResponse{
				ToolCalls: []agentlib.ToolCall{
					{ID: "write-fixture", Name: "write", Arguments: writeArgs},
					{ID: "commit-fixture", Name: "bash", Arguments: bashArgs},
				},
				Usage: agentlib.TokenUsage{Input: 1, Output: 1, Total: 2},
			}, nil
		},
	}
	quotaCtx, cancel := context.WithCancel(ctx)
	cancel()
	opts := agentlib.ServiceOptions{QuotaRefreshContext: quotaCtx}
	opts.FakeProvider = fake
	inner, err := agentlib.New(opts)
	if err != nil {
		close(tripwiresReady)
		return nil, err
	}
	events, err := inner.Execute(ctx, req)
	if err != nil {
		close(tripwiresReady)
		return nil, err
	}
	// Fizeau starts constructor-time inventory refreshes asynchronously even
	// with an already-cancelled quota context. Let those cancelled goroutines
	// observe the git/sh-only PATH and exit before arming the execution-stage
	// provider tripwires. The FakeProvider is blocked on tripwiresReady, so no
	// agent turn or tool call can begin during this drain window.
	if strings.TrimSpace(plan.TripwireBinDir) != "" {
		time.Sleep(250 * time.Millisecond)
	}
	if err := installFizeauProviderTripwires(plan); err != nil {
		close(tripwiresReady)
		return nil, err
	}
	close(tripwiresReady)
	return events, nil
}

// installFizeauProviderTripwires links the test's poison binaries into the
// already-restricted PATH after Fizeau has synchronously resolved the route.
// A sentinel is then executed through the same PATH and observer, proving the
// provider launch log is live rather than relying on an empty-file assertion.
func installFizeauProviderTripwires(plan fizeauTestPlan) error {
	if strings.TrimSpace(plan.TripwireBinDir) == "" {
		return nil
	}
	destDir := fizeauTestSeamToolsDir
	if strings.TrimSpace(destDir) == "" {
		return fmt.Errorf("restricted Fizeau test-seam PATH is unavailable")
	}
	for _, name := range append(append([]string(nil), plan.TripwireNames...), plan.TripwireSentinel) {
		if strings.TrimSpace(name) == "" {
			continue
		}
		source := filepath.Join(plan.TripwireBinDir, name)
		dest := filepath.Join(destDir, name)
		if err := os.Symlink(source, dest); err != nil {
			return fmt.Errorf("install Fizeau provider tripwire %s: %w", name, err)
		}
	}
	if strings.TrimSpace(plan.TripwireSentinel) == "" {
		return nil
	}
	cmd := exec.Command(filepath.Join(destDir, plan.TripwireSentinel))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("execute Fizeau provider tripwire sentinel: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func loadFizeauTestPlan() (fizeauTestPlan, error) {
	path := strings.TrimSpace(os.Getenv(fizeauTestPlanEnv))
	if path == "" {
		return fizeauTestPlan{}, fmt.Errorf("%s is required by the tagged Fizeau test seam", fizeauTestPlanEnv)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fizeauTestPlan{}, fmt.Errorf("read Fizeau test plan: %w", err)
	}
	var plan fizeauTestPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return fizeauTestPlan{}, fmt.Errorf("decode Fizeau test plan: %w", err)
	}
	if strings.TrimSpace(plan.WritePath) == "" || strings.TrimSpace(plan.CommitMessage) == "" {
		return fizeauTestPlan{}, fmt.Errorf("Fizeau test plan requires write_path and commit_message")
	}
	return plan, nil
}

func appendFizeauTestSeamLog(path string, req agentlib.ServiceExecuteRequest) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	line := strings.Join([]string{
		"pid=" + strconv.Itoa(os.Getpid()),
		"mode=" + req.Metadata[agent.DDXModeEnvKey],
		"bead=" + req.Metadata[agent.DDXBeadIDEnvKey],
		"harness=" + req.Harness,
		"provider=" + req.Provider,
		"model=" + req.Model,
	}, " ") + "\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open Fizeau test seam log: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("append Fizeau test seam log: %w", err)
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
