package docprose

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

const SupportedValeVersion = "vale version 3.13.0"

// ValeAlert is the raw alert shape returned by Vale JSON output before DDx
// normalizes it into user-facing prose findings.
type ValeAlert struct {
	File     string
	Check    string
	Line     int
	Span     [2]int
	Severity string
	Message  string
	Match    string
}

// ValeDiagnosticKind classifies runner failures that DDx wants to surface as
// prose-checker diagnostics instead of generic execution errors.
type ValeDiagnosticKind string

const (
	ValeDiagnosticMissing     ValeDiagnosticKind = "missing"
	ValeDiagnosticUnsupported ValeDiagnosticKind = "unsupported"
)

// ValeDiagnosticError reports missing or unsupported Vale installations.
type ValeDiagnosticError struct {
	Kind    ValeDiagnosticKind
	Path    string
	Version string
	Message string
}

func (e *ValeDiagnosticError) Error() string {
	return e.Message
}

// ValeRunner invokes the pinned Vale binary using a temporary DDx-generated
// configuration and parses Vale's JSON output into raw alerts.
type ValeRunner struct{}

func NewValeRunner() *ValeRunner {
	return &ValeRunner{}
}

// Findings runs Vale against the provided paths and returns raw alerts.
func (r *ValeRunner) Findings(ctx context.Context, settings Settings, paths ...string) ([]ValeAlert, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("vale runner requires at least one path")
	}

	binary, err := exec.LookPath("vale")
	if err != nil {
		return nil, &ValeDiagnosticError{
			Kind:    ValeDiagnosticMissing,
			Message: "Vale prose checker is not installed or is not on PATH",
		}
	}

	expectedVersion := expectedValeVersion(settings)

	versionCmd := exec.CommandContext(ctx, binary, "--version")
	versionOutput, err := versionCmd.CombinedOutput()
	version := strings.TrimSpace(string(versionOutput))
	if err != nil || version != expectedVersion {
		msg := fmt.Sprintf(
			"Vale prose checker did not report a supported version; expected %q",
			expectedVersion,
		)
		if version != "" {
			msg += fmt.Sprintf("; got %q", version)
		}
		if err != nil {
			msg += fmt.Sprintf("; error: %v", err)
		}
		return nil, &ValeDiagnosticError{
			Kind:    ValeDiagnosticUnsupported,
			Path:    binary,
			Version: version,
			Message: msg,
		}
	}

	tempCfg, err := NewTempValeConfig(settings)
	if err != nil {
		return nil, err
	}
	defer tempCfg.Cleanup()

	args := []string{"--config", tempCfg.INIPath(), "--output=JSON", "--no-global"}
	args = append(args, paths...)

	cmd := exec.CommandContext(ctx, binary, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	alerts, parseErr := parseValeAlertsJSON(stdout.Bytes())
	if parseErr != nil {
		return nil, parseErr
	}
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return alerts, nil
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("vale run failed: %w: %s", runErr, msg)
		}
		return nil, fmt.Errorf("vale run failed: %w", runErr)
	}

	return alerts, nil
}

type valeJSONAlert struct {
	Path     string `json:"Path"`
	Check    string `json:"Check"`
	Line     int    `json:"Line"`
	Span     []int  `json:"Span"`
	Severity string `json:"Severity"`
	Message  string `json:"Message"`
	Match    string `json:"Match"`
}

type valeJSONFile struct {
	Path   string          `json:"Path"`
	Alerts []valeJSONAlert `json:"Alerts"`
}

type valeJSONFiles struct {
	Files []valeJSONFile `json:"Files"`
}

func parseValeAlertsJSON(data []byte) ([]ValeAlert, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}

	if alerts, ok, err := parseValeAlertMap(data); err != nil {
		return nil, err
	} else if ok {
		return alerts, nil
	}

	if alerts, ok, err := parseValeFilesShape(data); err != nil {
		return nil, err
	} else if ok {
		return alerts, nil
	}

	return nil, fmt.Errorf("parse vale JSON output: unsupported shape")
}

func parseValeAlertMap(data []byte) ([]ValeAlert, bool, error) {
	var raw map[string][]valeJSONAlert
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, false, nil
	}
	alerts := make([]ValeAlert, 0, len(raw))
	for file, items := range raw {
		for _, item := range items {
			alert, err := normalizeValeAlert(file, item)
			if err != nil {
				return nil, false, err
			}
			alerts = append(alerts, alert)
		}
	}
	sortValeAlerts(alerts)
	return alerts, true, nil
}

func parseValeFilesShape(data []byte) ([]ValeAlert, bool, error) {
	var raw valeJSONFiles
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, false, nil
	}
	if len(raw.Files) == 0 {
		return nil, true, nil
	}

	alerts := make([]ValeAlert, 0)
	for _, file := range raw.Files {
		for _, item := range file.Alerts {
			alert, err := normalizeValeAlert(file.Path, item)
			if err != nil {
				return nil, false, err
			}
			alerts = append(alerts, alert)
		}
	}
	sortValeAlerts(alerts)
	return alerts, true, nil
}

func normalizeValeAlert(file string, item valeJSONAlert) (ValeAlert, error) {
	span := [2]int{}
	switch len(item.Span) {
	case 0:
		span = [2]int{}
	case 1:
		span[0] = item.Span[0]
	case 2:
		span[0] = item.Span[0]
		span[1] = item.Span[1]
	default:
		span[0] = item.Span[0]
		span[1] = item.Span[1]
	}

	if file == "" {
		file = strings.TrimSpace(item.Path)
	}
	if file == "" {
		return ValeAlert{}, fmt.Errorf("parse vale JSON output: missing file path")
	}
	if item.Check == "" {
		return ValeAlert{}, fmt.Errorf("parse vale JSON output: missing check for %s:%d", file, item.Line)
	}
	if item.Severity == "" {
		return ValeAlert{}, fmt.Errorf("parse vale JSON output: missing severity for %s:%d", file, item.Line)
	}
	if item.Message == "" {
		return ValeAlert{}, fmt.Errorf("parse vale JSON output: missing message for %s:%d", file, item.Line)
	}
	if item.Match == "" {
		return ValeAlert{}, fmt.Errorf("parse vale JSON output: missing match for %s:%d", file, item.Line)
	}

	return ValeAlert{
		File:     file,
		Check:    item.Check,
		Line:     item.Line,
		Span:     span,
		Severity: item.Severity,
		Message:  item.Message,
		Match:    item.Match,
	}, nil
}

func sortValeAlerts(alerts []ValeAlert) {
	sort.SliceStable(alerts, func(i, j int) bool {
		if alerts[i].File != alerts[j].File {
			return alerts[i].File < alerts[j].File
		}
		if alerts[i].Line != alerts[j].Line {
			return alerts[i].Line < alerts[j].Line
		}
		if alerts[i].Span[0] != alerts[j].Span[0] {
			return alerts[i].Span[0] < alerts[j].Span[0]
		}
		if alerts[i].Span[1] != alerts[j].Span[1] {
			return alerts[i].Span[1] < alerts[j].Span[1]
		}
		if alerts[i].Check != alerts[j].Check {
			return alerts[i].Check < alerts[j].Check
		}
		if alerts[i].Severity != alerts[j].Severity {
			return alerts[i].Severity < alerts[j].Severity
		}
		if alerts[i].Message != alerts[j].Message {
			return alerts[i].Message < alerts[j].Message
		}
		return alerts[i].Match < alerts[j].Match
	})
}

func expectedValeVersion(settings Settings) string {
	version := strings.TrimSpace(settings.Vale.Version)
	if version == "" {
		return SupportedValeVersion
	}
	if strings.HasPrefix(version, "vale ") {
		return version
	}
	return "vale version " + version
}
