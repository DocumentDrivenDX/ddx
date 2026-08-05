package try

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead/accheck"
)

var (
	mechanicalAlreadySatisfiedFilePathRE = regexp.MustCompile(`\b[\w./-]+\.(?:go|md|yaml|yml|json|sh|ts|tsx|svelte|txt|toml|adoc|rst)(?::\d+)?\b`)
	mechanicalAlreadySatisfiedSymbolRE   = regexp.MustCompile(`\bddx-[0-9a-f]{8}\b`)
	mechanicalAlreadySatisfiedBacktickRE = regexp.MustCompile("`([A-Za-z_][A-Za-z0-9_.-]*)`")
	errMechanicalTokenFound              = errors.New("mechanical token found")
)

var packageGatePollutionSignals = []string{
	"unrelated package gate",
	"package gate red",
	"package gate failed",
	"package gate is not green",
	"full package gate is not green",
	"full package gate",
	"lefthook run pre-commit",
	"lefthook pre-commit",
	"pre-commit is blocked",
	"go test ./",
	"go test ./internal/",
	"go test ./cmd/",
}

// MechanicalAlreadySatisfiedChecker returns a SatisfactionChecker that closes
// only fully mechanical acceptance criteria already present in the project
// tree. It intentionally refuses to auto-close beads with test-name,
// build-gate, command, or prose AC so implementation work still requires the
// normal verification path.
func MechanicalAlreadySatisfiedChecker(acceptance, projectRoot string) SatisfactionChecker {
	acceptance = strings.TrimSpace(acceptance)
	if acceptance == "" || strings.TrimSpace(projectRoot) == "" {
		return nil
	}

	items := accheck.ParseAcceptance(acceptance)
	if len(items) == 0 {
		return nil
	}

	checker := mechanicalSatisfactionCheckerFunc(func(ctx context.Context, beadID string, noChangesCount int) (bool, string, error) {
		_ = ctx
		_ = beadID
		_ = noChangesCount

		var evidence []string
		for _, item := range items {
			switch item.Kind {
			case accheck.KindTestName, accheck.KindBuildGate, accheck.KindCommand, accheck.KindProse:
				return false, "", nil
			case accheck.KindSymbol:
				if item.Name == "" {
					return false, "", nil
				}
				found, sample, err := projectContainsToken(projectRoot, item.Name)
				if err != nil {
					return false, "", err
				}
				if !found {
					return false, "", nil
				}
				evidence = append(evidence, fmt.Sprintf("%s found in %s", item.Name, sample))
			case accheck.KindNegative:
				if item.Name == "" {
					return false, "", nil
				}
				found, sample, err := projectContainsToken(projectRoot, item.Name)
				if err != nil {
					return false, "", err
				}
				if found {
					return false, fmt.Sprintf("symbol %q still present in %s", item.Name, sample), nil
				}
				evidence = append(evidence, fmt.Sprintf("%s absent as required", item.Name))
			case accheck.KindFilePath:
				path := extractAcceptanceFilePath(item.Text)
				if path == "" {
					return false, "", nil
				}
				if _, err := os.Stat(filepath.Join(projectRoot, filepath.FromSlash(path))); err != nil {
					return false, "", nil
				}
				evidence = append(evidence, fmt.Sprintf("%s exists", path))
			case accheck.KindMechanical:
				anchors := extractMechanicalAnchors(item.Text)
				if len(anchors) == 0 {
					return false, "", nil
				}
				for _, anchor := range anchors {
					if strings.Contains(anchor, "/") || strings.Contains(anchor, ".") {
						path := anchor
						if idx := strings.IndexByte(path, ':'); idx >= 0 {
							path = path[:idx]
						}
						if _, err := os.Stat(filepath.Join(projectRoot, filepath.FromSlash(path))); err != nil {
							return false, "", nil
						}
						evidence = append(evidence, fmt.Sprintf("%s exists", path))
						continue
					}
					found, sample, err := projectContainsToken(projectRoot, anchor)
					if err != nil {
						return false, "", err
					}
					if !found {
						return false, "", nil
					}
					evidence = append(evidence, fmt.Sprintf("%s found in %s", anchor, sample))
				}
			default:
				return false, "", nil
			}
		}

		if len(evidence) == 0 {
			return false, "", nil
		}
		return true, "mechanical AC satisfied: " + strings.Join(evidence, "; "), nil
	})
	return checker
}

func reportHasPackageGatePollution(report Report) bool {
	if report.Status != StatusNoChanges {
		return false
	}

	parsed := ParseNoChangesRationale(report.NoChangesRationale)
	if parsed.Kind == NoChangesKindVerified {
		return false
	}

	if parsed.LifecycleStatus == "blocked" && noChangesBlockedReasonLooksExternalRecheckable(parsed.Reason, parsed.SuggestedAction, report.NoChangesRationale) {
		return false
	}

	combined := strings.ToLower(strings.Join([]string{
		strings.TrimSpace(report.NoChangesRationale),
		strings.TrimSpace(report.Detail),
		strings.TrimSpace(report.Error),
		strings.TrimSpace(report.Stderr),
		parsed.Reason,
		parsed.SuggestedAction,
	}, "\n"))
	if combined == "" {
		return false
	}

	if !containsAnyText(combined, packageGatePollutionSignals...) {
		return false
	}

	return containsAnyText(combined,
		"unrelated",
		"pre-existing",
		"not green",
		"red",
		"failed",
		"failure",
		"race",
	)
}

type mechanicalSatisfactionCheckerFunc func(ctx context.Context, beadID string, noChangesCount int) (bool, string, error)

func (f mechanicalSatisfactionCheckerFunc) CheckSatisfied(ctx context.Context, beadID string, noChangesCount int) (bool, string, error) {
	return f(ctx, beadID, noChangesCount)
}

func extractAcceptanceFilePath(text string) string {
	m := mechanicalAlreadySatisfiedFilePathRE.FindString(text)
	if m == "" {
		return ""
	}
	if idx := strings.IndexByte(m, ':'); idx >= 0 {
		m = m[:idx]
	}
	return strings.TrimSpace(m)
}

func extractMechanicalAnchors(text string) []string {
	var anchors []string
	if path := extractAcceptanceFilePath(text); path != "" {
		anchors = append(anchors, path)
	}
	for _, match := range mechanicalAlreadySatisfiedBacktickRE.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
			anchors = append(anchors, strings.TrimSpace(match[1]))
		}
	}
	anchors = append(anchors, mechanicalAlreadySatisfiedSymbolRE.FindAllString(text, -1)...)
	return uniqueStrings(anchors)
}

func projectContainsToken(projectRoot, needle string) (bool, string, error) {
	needle = strings.TrimSpace(needle)
	if needle == "" || strings.TrimSpace(projectRoot) == "" {
		return false, "", nil
	}

	var sample string
	walkErr := filepath.WalkDir(projectRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".ddx", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !isTextCandidate(path) {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(data), needle) {
			sample = strings.TrimPrefix(path, projectRoot+string(filepath.Separator))
			return errMechanicalTokenFound
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, errMechanicalTokenFound) {
		return false, "", walkErr
	}
	return sample != "", sample, nil
}

func isTextCandidate(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".md", ".yaml", ".yml", ".json", ".sh", ".ts", ".tsx", ".svelte", ".txt", ".toml", ".adoc", ".rst", ".jsonl":
		return true
	default:
		base := filepath.Base(path)
		return base == "Makefile" || base == "Dockerfile"
	}
}

func uniqueStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func containsAnyText(text string, needles ...string) bool {
	for _, needle := range needles {
		if needle == "" {
			continue
		}
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
