package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/docprose"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newDocProseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prose [paths...]",
		Short: "Check prose quality in Markdown documents",
		Long: `Check prose quality in changed Markdown files or explicit paths.

The default surface is advisory. Findings are deterministic and
machine-readable, with file, line, rule id, severity, rationale, and
suggested edit fields.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return f.runDocProse(cmd, args)
		},
	}
	cmd.Flags().Bool("changed", false, "Check changed Markdown files in the current git repo")
	return cmd
}

func (f *CommandFactory) runDocProse(cmd *cobra.Command, args []string) error {
	changed, _ := cmd.Flags().GetBool("changed")

	cfg, err := config.LoadWithWorkingDir(f.docRoot())
	if err != nil {
		return err
	}

	settings, err := docprose.ResolveSettings(cfg)
	if err != nil {
		return err
	}

	rootDir := f.docRoot()
	var relPaths []string
	if changed {
		if len(args) > 0 {
			return fmt.Errorf("use either --changed or explicit paths, not both")
		}
		relPaths, err = changedMarkdownPaths(rootDir)
		if err != nil {
			return err
		}
	} else {
		if len(args) == 0 {
			return fmt.Errorf("provide one or more paths or pass --changed")
		}
		relPaths = append([]string(nil), args...)
	}

	type docEntry struct {
		rel string
		abs string
	}
	var entries []docEntry
	for _, relPath := range relPaths {
		cleanRel, absPath, ok := normalizeDocPath(rootDir, relPath)
		if !ok {
			continue
		}
		if !strings.EqualFold(filepath.Ext(cleanRel), ".md") {
			continue
		}
		if !pathAllowed(cleanRel, settings.Includes, settings.Excludes) {
			continue
		}
		if _, statErr := os.Stat(absPath); statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return fmt.Errorf("stat %s: %w", relPath, statErr)
		}
		entries = append(entries, docEntry{rel: cleanRel, abs: absPath})
	}

	runnerKind := strings.ToLower(strings.TrimSpace(settings.Runner))
	useEmbedded := runnerKind == "embedded"

	var findings []docprose.Finding
	if !useEmbedded && len(entries) > 0 {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		absPaths := make([]string, 0, len(entries))
		absToRel := make(map[string]string, len(entries))
		for _, entry := range entries {
			absPaths = append(absPaths, entry.abs)
			absToRel[entry.abs] = entry.rel
		}
		alerts, valeErr := docprose.NewValeRunner().Findings(ctx, settings, absPaths...)
		if valeErr != nil {
			var diag *docprose.ValeDiagnosticError
			if !errors.As(valeErr, &diag) {
				return valeErr
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: optional prose runner %q is unavailable; using embedded checker\n", "vale")
			useEmbedded = true
		} else {
			normalized := docprose.NormalizeValeAlerts(alerts)
			for _, finding := range normalized {
				if rel, ok := absToRel[finding.File]; ok {
					finding.File = rel
				}
				if settings.Severity != "" {
					finding.Severity = settings.Severity
				}
				findings = append(findings, finding)
			}
		}
	}

	if useEmbedded {
		checker, checkerErr := docprose.NewChecker(settings.Mode, settings.Vocabulary)
		if checkerErr != nil {
			return checkerErr
		}
		for _, entry := range entries {
			content, readErr := os.ReadFile(entry.abs)
			if readErr != nil {
				if os.IsNotExist(readErr) {
					continue
				}
				return fmt.Errorf("read %s: %w", entry.rel, readErr)
			}
			for _, finding := range checker.Findings(entry.rel, string(content)) {
				if settings.Severity != "" {
					finding.Severity = settings.Severity
				}
				findings = append(findings, finding)
			}
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].RuleID < findings[j].RuleID
	})

	out := cmd.OutOrStdout()
	if len(findings) == 0 {
		if changed {
			fmt.Fprintln(out, "No changed Markdown prose findings.")
		} else {
			fmt.Fprintln(out, "No prose findings.")
		}
		return nil
	}

	for _, finding := range findings {
		fmt.Fprintf(out, "%s:%d [%s] %s\n", finding.File, finding.Line, finding.Severity, finding.RuleID)
		fmt.Fprintf(out, "  rationale: %s\n", finding.Rationale)
		fmt.Fprintf(out, "  suggested edit: %s\n", finding.SuggestedEdit)
	}

	if isBlockingProsePolicy(settings.Policy, settings.Severity) {
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		return NewExitError(ExitCodeGeneralError, "")
	}
	return nil
}

func isBlockingProsePolicy(policy, severity string) bool {
	return strings.EqualFold(policy, "blocking") || strings.EqualFold(severity, "blocking")
}

func changedMarkdownPaths(rootDir string) ([]string, error) {
	out, err := internalgit.Command(context.Background(), rootDir, "status", "--porcelain=v1", "--untracked-files=all").Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}

	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" || len(line) < 3 {
			continue
		}
		status := line[:2]
		pathText := strings.TrimSpace(line[3:])
		if pathText == "" {
			continue
		}
		if strings.Contains(pathText, " -> ") {
			parts := strings.Split(pathText, " -> ")
			pathText = parts[len(parts)-1]
		}
		if strings.ContainsRune(status, 'D') {
			continue
		}
		if !strings.EqualFold(filepath.Ext(pathText), ".md") {
			continue
		}
		paths = append(paths, filepath.ToSlash(pathText))
	}
	sort.Strings(paths)
	return paths, nil
}

func normalizeDocPath(rootDir, input string) (string, string, bool) {
	if strings.TrimSpace(input) == "" {
		return "", "", false
	}
	absPath := input
	if !filepath.IsAbs(input) {
		absPath = filepath.Join(rootDir, input)
	}
	absPath = filepath.Clean(absPath)
	relPath, err := filepath.Rel(rootDir, absPath)
	if err != nil {
		relPath = filepath.ToSlash(filepath.Clean(input))
	} else {
		relPath = filepath.ToSlash(relPath)
	}
	return relPath, absPath, true
}

func pathAllowed(relPath string, includes, excludes []string) bool {
	relPath = filepath.ToSlash(relPath)
	if len(includes) > 0 && !anyGlobMatch(includes, relPath) {
		return false
	}
	if len(excludes) > 0 && anyGlobMatch(excludes, relPath) {
		return false
	}
	return true
}

func anyGlobMatch(patterns []string, relPath string) bool {
	for _, pattern := range patterns {
		if matchPathGlob(pattern, relPath) {
			return true
		}
	}
	return false
}

func matchPathGlob(pattern, name string) bool {
	pattern = filepath.ToSlash(pattern)
	name = filepath.ToSlash(name)
	if matched, _ := filepath.Match(pattern, name); matched {
		return true
	}
	if matched, _ := filepath.Match(pattern, filepath.Base(name)); matched {
		return true
	}
	if !strings.Contains(pattern, "**") {
		return false
	}
	return matchDoubleStar(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchDoubleStar(patternParts, nameParts []string) bool {
	if len(patternParts) == 0 {
		return len(nameParts) == 0
	}
	if patternParts[0] == "**" {
		for i := 0; i <= len(nameParts); i++ {
			if matchDoubleStar(patternParts[1:], nameParts[i:]) {
				return true
			}
		}
		return false
	}
	if len(nameParts) == 0 {
		return false
	}
	matched, err := filepath.Match(patternParts[0], nameParts[0])
	if err != nil || !matched {
		return false
	}
	return matchDoubleStar(patternParts[1:], nameParts[1:])
}
