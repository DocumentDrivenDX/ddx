package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/docprose"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

// ProseEvidenceEventKind names the bead-event kind appended when prose
// findings (or a no-op skip) are recorded for a docs-changing attempt.
const ProseEvidenceEventKind = "prose-check.findings"

// ProseEvidenceHook runs after a successful execute-bead attempt and
// attaches advisory prose-check evidence when the attempt changed Markdown
// files under docs/. It is wired by the CLI for both `ddx try` and `ddx work`.
type ProseEvidenceHook func(ctx context.Context, beadID string, report ExecuteBeadReport) error

// ProseEvidenceConfig configures AttachProseEvidence. Each field is optional
// and falls back to a real-git default when zero.
type ProseEvidenceConfig struct {
	// ProjectRoot is the project's git working copy used for diff/show.
	ProjectRoot string
	// Events appends bead-event records. Required.
	Events BeadEventAppender
	// Actor identifies the worker recording the event.
	Actor string
	// Source identifies the originating command (e.g. "ddx try", "ddx work").
	Source string
	// Now returns the event timestamp. Defaults to time.Now.
	Now func() time.Time
	// ChangedFiles returns paths changed between baseRev and resultRev.
	// Defaults to `git diff --name-only`.
	ChangedFiles func(projectRoot, baseRev, resultRev string) ([]string, error)
	// ReadFileAtRev reads a file's content at a given revision.
	// Defaults to `git show <rev>:<path>`.
	ReadFileAtRev func(projectRoot, rev, path string) ([]byte, error)
}

// ProseEvidenceResult records what the hook did for an attempt.
type ProseEvidenceResult struct {
	Ran          bool               `json:"ran"`
	ChangedDocs  []string           `json:"changed_docs,omitempty"`
	Findings     []docprose.Finding `json:"findings,omitempty"`
	EvidencePath string             `json:"evidence_path,omitempty"`
}

// AttachProseEvidence is the post-attempt prose-quality hook for
// docs-changing attempts. It is advisory: a non-nil error is returned only
// for caller observability; the loop must continue regardless.
//
// Behavior:
//   - If report.BaseRev == report.ResultRev (no changes), returns Ran=false.
//   - If no docs/**/*.md paths changed, returns Ran=false.
//   - Otherwise, runs the embedded docprose checker on the changed files at
//     ResultRev, writes findings JSON to the per-attempt evidence dir, and
//     appends a `prose-check.findings` bead event.
func AttachProseEvidence(ctx context.Context, beadID string, report ExecuteBeadReport, cfg ProseEvidenceConfig) (ProseEvidenceResult, error) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.ChangedFiles == nil {
		cfg.ChangedFiles = defaultProseChangedFiles
	}
	if cfg.ReadFileAtRev == nil {
		cfg.ReadFileAtRev = defaultProseReadFileAtRev
	}

	baseRev := strings.TrimSpace(report.BaseRev)
	resultRev := strings.TrimSpace(report.ResultRev)
	if baseRev == "" || resultRev == "" || baseRev == resultRev {
		return ProseEvidenceResult{}, nil
	}

	changed, err := cfg.ChangedFiles(cfg.ProjectRoot, baseRev, resultRev)
	if err != nil {
		return ProseEvidenceResult{}, fmt.Errorf("prose evidence: changed files: %w", err)
	}

	docPaths := filterDocsMarkdown(changed)
	if len(docPaths) == 0 {
		return ProseEvidenceResult{}, nil
	}

	projectCfg, _ := config.LoadWithWorkingDir(cfg.ProjectRoot)
	settings, err := docprose.ResolveSettings(projectCfg)
	if err != nil {
		return ProseEvidenceResult{}, fmt.Errorf("prose evidence: resolve settings: %w", err)
	}
	checker, err := docprose.NewChecker(settings.Mode, settings.Vocabulary)
	if err != nil {
		return ProseEvidenceResult{}, fmt.Errorf("prose evidence: build checker: %w", err)
	}

	var findings []docprose.Finding
	for _, rel := range docPaths {
		content, readErr := cfg.ReadFileAtRev(cfg.ProjectRoot, resultRev, rel)
		if readErr != nil {
			continue
		}
		for _, f := range checker.Findings(rel, string(content)) {
			if settings.Severity != "" {
				f.Severity = settings.Severity
			}
			findings = append(findings, f)
		}
	}

	result := ProseEvidenceResult{
		Ran:         true,
		ChangedDocs: docPaths,
		Findings:    findings,
	}

	if cfg.ProjectRoot != "" && strings.TrimSpace(report.AttemptID) != "" {
		evidenceDir := ddxroot.JoinProject(cfg.ProjectRoot, "executions", report.AttemptID)
		if mkErr := os.MkdirAll(evidenceDir, 0o755); mkErr == nil {
			payload, _ := json.MarshalIndent(result, "", "  ")
			evidencePath := filepath.Join(evidenceDir, "prose-findings.json")
			if writeErr := os.WriteFile(evidencePath, payload, 0o644); writeErr == nil {
				result.EvidencePath = ddxroot.JoinRelative("executions", report.AttemptID, "prose-findings.json")
			}
		}
	}

	if cfg.Events != nil {
		body, _ := json.Marshal(map[string]any{
			"changed_docs": docPaths,
			"findings":     findings,
			"advisory":     true,
			"base_rev":     baseRev,
			"result_rev":   resultRev,
			"evidence":     result.EvidencePath,
		})
		summary := fmt.Sprintf("prose check: %d finding(s) across %d doc(s)", len(findings), len(docPaths))
		_ = cfg.Events.AppendEvent(beadID, bead.BeadEvent{
			Kind:      ProseEvidenceEventKind,
			Summary:   summary,
			Body:      string(body),
			Actor:     cfg.Actor,
			Source:    cfg.Source,
			CreatedAt: cfg.Now().UTC(),
		})
	}

	return result, nil
}

// NewDefaultProseEvidenceHook returns a ProseEvidenceHook bound to the given
// configuration. Errors from AttachProseEvidence are returned to the loop
// only for observability; the loop must not park or block the attempt on a
// returned error.
func NewDefaultProseEvidenceHook(cfg ProseEvidenceConfig) ProseEvidenceHook {
	return func(ctx context.Context, beadID string, report ExecuteBeadReport) error {
		_, err := AttachProseEvidence(ctx, beadID, report, cfg)
		return err
	}
}

func filterDocsMarkdown(paths []string) []string {
	var out []string
	for _, p := range paths {
		p = filepath.ToSlash(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if !strings.EqualFold(filepath.Ext(p), ".md") {
			continue
		}
		if !strings.HasPrefix(p, "docs/") {
			continue
		}
		out = append(out, p)
	}
	return out
}

func defaultProseChangedFiles(projectRoot, baseRev, resultRev string) ([]string, error) {
	if projectRoot == "" || baseRev == "" || resultRev == "" || baseRev == resultRev {
		return nil, nil
	}
	out, err := internalgit.Command(context.Background(), projectRoot, "diff", "--name-only", baseRev, resultRev).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s %s: %s: %w", baseRev, resultRev, strings.TrimSpace(string(out)), err)
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if p := strings.TrimSpace(line); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

func defaultProseReadFileAtRev(projectRoot, rev, path string) ([]byte, error) {
	if projectRoot == "" || rev == "" || path == "" {
		return nil, fmt.Errorf("prose evidence: empty git show arguments")
	}
	out, err := internalgit.Command(context.Background(), projectRoot, "show", rev+":"+path).Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s:%s: %w", rev, path, err)
	}
	return out, nil
}
