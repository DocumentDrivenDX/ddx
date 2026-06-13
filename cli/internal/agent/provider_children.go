package agent

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
)

const (
	providerChildActionTerminated = "terminated"
	reasonSupersededProviderChild = "superseded_provider_child"
	reasonAttemptEnded            = "attempt_ended"
	providerChildCleanupArtifact  = "provider-children.json"
)

var providerCLINames = map[string]struct{}{
	"claude":   {},
	"codex":    {},
	"gemini":   {},
	"opencode": {},
	"pi":       {},
}

type providerChildProcess struct {
	PID       int
	Provider  string
	Command   string
	StartedAt time.Time
}

type providerChildReapRecord struct {
	PID        int     `json:"pid"`
	Provider   string  `json:"provider"`
	Command    string  `json:"command,omitempty"`
	AgeSeconds float64 `json:"age_seconds"`
	Action     string  `json:"action"`
	Reason     string  `json:"reason"`
}

type providerChildCleanupReport struct {
	AttemptID   string                       `json:"attempt_id"`
	BeadID      string                       `json:"bead_id"`
	Trigger     string                       `json:"trigger"`
	ActiveRoute string                       `json:"active_route,omitempty"`
	ScannedAt   time.Time                    `json:"scanned_at"`
	Survivors   []workerstatus.ProviderChild `json:"survivors,omitempty"`
	Reaped      []providerChildReapRecord    `json:"reaped,omitempty"`
	ScanErr     string                       `json:"scan_error,omitempty"`
}

var providerChildScanner = func(ctx context.Context, rootPID int, now time.Time) ([]providerChildProcess, error) {
	return scanProviderChildProcessesImpl(ctx, rootPID, now)
}

var terminateProviderChild = terminateProviderChildImpl

func providerForCommand(cmdline string) string {
	parts := strings.Fields(strings.TrimSpace(cmdline))
	if len(parts) == 0 {
		return ""
	}
	base := filepath.Base(parts[0])
	if _, ok := providerCLINames[base]; ok {
		return base
	}
	return ""
}

func providerRouteLabel(provider, model string) string {
	route := strings.TrimSpace(provider)
	if route != "" && strings.TrimSpace(model) != "" {
		route += "/" + strings.TrimSpace(model)
	}
	return route
}

func routeOwnsProvider(provider string, routeTokens ...string) bool {
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" {
		return false
	}
	for _, tok := range routeTokens {
		tok = strings.ToLower(strings.TrimSpace(tok))
		if tok == "" {
			continue
		}
		if tok == p {
			return true
		}
		if seg := strings.SplitN(tok, "/", 2); len(seg) > 0 && seg[0] == p {
			return true
		}
	}
	return false
}

func scanProviderChildrenForStatus(ctx context.Context, rootPID int, routeLabel, harness, phase string, now time.Time) []workerstatus.ProviderChild {
	procs, err := providerChildScanner(ctx, rootPID, now)
	if err != nil || len(procs) == 0 {
		return nil
	}
	out := make([]workerstatus.ProviderChild, 0, len(procs))
	for _, proc := range procs {
		owner := ""
		if routeOwnsProvider(proc.Provider, routeLabel, harness) {
			owner = strings.TrimSpace(routeLabel)
			if owner == "" {
				owner = strings.TrimSpace(harness)
			}
		}
		out = append(out, workerstatus.ProviderChild{
			PID:        proc.PID,
			Provider:   proc.Provider,
			Harness:    proc.Provider,
			RouteOwner: owner,
			Phase:      phase,
			AgeSeconds: childAgeSeconds(proc, now),
		})
	}
	return out
}

func reapProviderChildren(ctx context.Context, rootPID int, now time.Time, reasonFor func(providerChildProcess) string) ([]providerChildReapRecord, []workerstatus.ProviderChild, error) {
	procs, err := providerChildScanner(ctx, rootPID, now)
	if err != nil {
		return nil, nil, err
	}
	var reaped []providerChildReapRecord
	var survivors []workerstatus.ProviderChild
	for _, proc := range procs {
		reason := reasonFor(proc)
		if reason == "" {
			survivors = append(survivors, workerstatus.ProviderChild{
				PID:        proc.PID,
				Provider:   proc.Provider,
				Harness:    proc.Provider,
				AgeSeconds: childAgeSeconds(proc, now),
			})
			continue
		}
		terminateProviderChild(proc.PID)
		reaped = append(reaped, providerChildReapRecord{
			PID:        proc.PID,
			Provider:   proc.Provider,
			Command:    proc.Command,
			AgeSeconds: childAgeSeconds(proc, now),
			Action:     providerChildActionTerminated,
			Reason:     reason,
		})
	}
	return reaped, survivors, nil
}

func reapSupersededProviderChildren(ctx context.Context, rootPID int, routeLabel, harness string, now time.Time) ([]providerChildReapRecord, []workerstatus.ProviderChild) {
	if strings.TrimSpace(routeLabel) == "" && strings.TrimSpace(harness) == "" {
		return nil, nil
	}
	reaped, survivors, err := reapProviderChildren(ctx, rootPID, now, func(proc providerChildProcess) string {
		if routeOwnsProvider(proc.Provider, routeLabel, harness) {
			return ""
		}
		return reasonSupersededProviderChild
	})
	if err != nil {
		return nil, nil
	}
	return reaped, survivors
}

func reapAllProviderChildren(ctx context.Context, rootPID int, now time.Time) []providerChildReapRecord {
	reaped, _, err := reapProviderChildren(ctx, rootPID, now, func(providerChildProcess) string {
		return reasonAttemptEnded
	})
	if err != nil {
		return nil
	}
	return reaped
}

func childAgeSeconds(proc providerChildProcess, now time.Time) float64 {
	if proc.StartedAt.IsZero() {
		return 0
	}
	age := now.Sub(proc.StartedAt)
	if age < 0 {
		return 0
	}
	return age.Seconds()
}

func writeProviderChildCleanupArtifact(projectRoot, attemptID string, report *providerChildCleanupReport) {
	if report == nil || strings.TrimSpace(projectRoot) == "" || strings.TrimSpace(attemptID) == "" {
		return
	}
	path := filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID, providerChildCleanupArtifact)
	_ = writeArtifactJSON(path, report)
}
