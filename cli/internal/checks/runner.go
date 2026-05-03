package checks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
)

// Run evaluates every check in `all` whose AppliesTo filter matches `ctx`,
// in parallel, and returns the collected results sorted by check name.
//
// Each applicable check is run as `sh -c <command>` with the documented
// environment variables injected. After execution, the runner reads the
// expected ${EvidenceDir}/${name}.json file and parses it into a Result.
//
// Failure modes that produce status=error:
//   - Non-zero process exit.
//   - Result file missing.
//   - Result file unparseable / unknown status value.
//
// Run returns an error only for setup failures (e.g. cannot create
// EvidenceDir). Per-check failures are reported as Result{Status:error}
// in the returned slice.
func Run(ctx context.Context, all []Check, ictx InvocationContext) ([]Result, error) {
	if err := os.MkdirAll(ictx.EvidenceDir, 0o755); err != nil {
		return nil, fmt.Errorf("checks: create evidence dir: %w", err)
	}

	var applicable []Check
	for _, c := range all {
		if c.Applies(ictx) {
			applicable = append(applicable, c)
		}
	}

	results := make([]Result, len(applicable))
	var wg sync.WaitGroup
	for i := range applicable {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = runOne(ctx, applicable[i], ictx)
		}(i)
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	return results, nil
}

func runOne(ctx context.Context, c Check, ictx InvocationContext) Result {
	resultPath := filepath.Join(ictx.EvidenceDir, c.Name+".json")
	// Best-effort cleanup of any stale result file from a prior run so a
	// check that crashes without writing one is correctly classified as
	// status=error rather than silently picking up the previous output.
	_ = os.Remove(resultPath)

	cmd := exec.CommandContext(ctx, "sh", "-c", c.Command)
	cmd.Dir = ictx.ProjectRoot
	cmd.Env = append(os.Environ(),
		"BEAD_ID="+ictx.BeadID,
		"DIFF_BASE="+ictx.DiffBase,
		"DIFF_HEAD="+ictx.DiffHead,
		"PROJECT_ROOT="+ictx.ProjectRoot,
		"EVIDENCE_DIR="+ictx.EvidenceDir,
		"RUN_ID="+ictx.RunID,
		"CHECK_NAME="+c.Name,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			return Result{
				Name:     c.Name,
				Status:   StatusError,
				Message:  fmt.Sprintf("check %q failed to start: %v", c.Name, runErr),
				ExitCode: -1,
				Stderr:   stderr.String(),
			}
		}
	}

	if runErr != nil {
		return Result{
			Name:     c.Name,
			Status:   StatusError,
			Message:  fmt.Sprintf("check %q exited %d: %s", c.Name, exitCode, trim(stderr.String())),
			ExitCode: exitCode,
			Stderr:   stderr.String(),
		}
	}

	data, readErr := os.ReadFile(resultPath)
	if readErr != nil {
		return Result{
			Name:     c.Name,
			Status:   StatusError,
			Message:  fmt.Sprintf("check %q produced no result file at %s", c.Name, resultPath),
			ExitCode: exitCode,
			Stderr:   stderr.String(),
		}
	}

	var parsed Result
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Result{
			Name:     c.Name,
			Status:   StatusError,
			Message:  fmt.Sprintf("check %q wrote invalid JSON to %s: %v", c.Name, resultPath, err),
			ExitCode: exitCode,
			Stderr:   stderr.String(),
		}
	}
	parsed.Name = c.Name
	parsed.ExitCode = exitCode
	parsed.Stderr = stderr.String()
	switch parsed.Status {
	case StatusPass, StatusBlock, StatusError:
		// ok
	default:
		return Result{
			Name:     c.Name,
			Status:   StatusError,
			Message:  fmt.Sprintf("check %q returned unknown status %q", c.Name, parsed.Status),
			ExitCode: exitCode,
			Stderr:   stderr.String(),
		}
	}
	return parsed
}

func trim(s string) string {
	const max = 500
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
