package cmd

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/serverreg"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newDocCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doc",
		Short: "Document dependency graph and staleness tracking",
		Long: `Manage the document dependency graph.

DDx tracks dependencies between documents using YAML frontmatter.
When an upstream document changes, DDx detects which downstream
documents are stale and need review.

Examples:
  ddx doc graph              # Show document dependency graph
  ddx doc stale              # List stale documents
  ddx doc stamp docs/prd.md  # Mark a document as reviewed
  ddx doc show helix.prd     # Show document metadata
  ddx doc deps helix.arch    # Show what a document depends on
  ddx doc dependents helix.prd  # Show what depends on a document`,
		Aliases: []string{"docs"},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			serverreg.TryRegisterAsync(f.WorkingDir)
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(f.newDocGraphCommand())
	cmd.AddCommand(f.newDocStaleCommand())
	cmd.AddCommand(f.newDocStampCommand())
	cmd.AddCommand(f.newDocShowCommand())
	cmd.AddCommand(f.newDocDepsCommand())
	cmd.AddCommand(f.newDocDependentsCommand())
	cmd.AddCommand(f.newDocValidateCommand())
	cmd.AddCommand(f.newDocMigrateCommand())
	cmd.AddCommand(f.newDocHistoryCommand())
	cmd.AddCommand(f.newDocDiffCommand())
	cmd.AddCommand(f.newDocChangedCommand())
	cmd.AddCommand(f.newDocAuditCommand())

	return cmd
}

// newDocAuditCommand surfaces structured integrity issues for CI and
// terminal use. Exits 0 when the graph is clean and 1 when any issue is
// detected, so CI can gate merges on a healthy graph.
func (f *CommandFactory) newDocAuditCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "audit",
		Short:   "Audit document graph integrity (duplicates, missing deps, broken id_to_path)",
		Aliases: []string{"integrity"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			graph, err := f.buildDocGraph()
			if err != nil {
				return err
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			exitZero, _ := cmd.Flags().GetBool("exit-zero")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(graph.Issues); err != nil {
					return err
				}
				if len(graph.Issues) == 0 || exitZero {
					return nil
				}
				cmd.SilenceUsage = true
				cmd.SilenceErrors = true
				return NewExitError(ExitCodeGeneralError, "")
			}
			out := cmd.OutOrStdout()
			if len(graph.Issues) == 0 {
				fmt.Fprintln(out, "Document graph is clean.")
				return nil
			}
			groups := map[docgraph.IssueKind][]docgraph.GraphIssue{}
			order := []docgraph.IssueKind{}
			for _, issue := range graph.Issues {
				if _, ok := groups[issue.Kind]; !ok {
					order = append(order, issue.Kind)
				}
				groups[issue.Kind] = append(groups[issue.Kind], issue)
			}
			sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
			for _, kind := range order {
				bucket := groups[kind]
				fmt.Fprintf(out, "%s (%d):\n", kind, len(bucket))
				for _, issue := range bucket {
					fmt.Fprintf(out, "  - %s\n", issue.Message)
				}
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "\n%d integrity issue(s) found\n", len(graph.Issues))
			// Return a typed ExitError so main() exits 1 without Cobra
			// reprinting the (empty) error message or usage summary; the
			// grouped issue list above is the user-facing report.
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			return NewExitError(ExitCodeGeneralError, "")
		},
	}
	cmd.Flags().Bool("json", false, "Output issues as JSON")
	cmd.Flags().Bool("exit-zero", false, "Always exit 0 after printing audit output")
	return cmd
}

func (f *CommandFactory) docRoot() string {
	root := os.Getenv("DDX_DOC_ROOT")
	if root != "" {
		return root
	}
	if f.WorkingDir != "" {
		return f.WorkingDir
	}
	wd, _ := os.Getwd()
	return wd
}

func (f *CommandFactory) buildDocGraph() (*docgraph.Graph, error) {
	return docgraph.BuildGraphWithConfig(f.docRoot())
}

func (f *CommandFactory) newDocGraphCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Show document dependency graph",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			graph, err := f.buildDocGraph()
			if err != nil {
				return err
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				return printDocGraphJSON(cmd, graph)
			}

			return printDocGraphText(cmd, graph)
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func (f *CommandFactory) newDocStaleCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stale",
		Short: "List stale documents",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			graph, err := f.buildDocGraph()
			if err != nil {
				return err
			}
			stale := graph.StaleDocs()

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(stale)
			}

			if len(stale) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "All documents are up to date.")
				return nil
			}

			for _, entry := range stale {
				reasons := strings.Join(entry.Reasons, "; ")
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  (%s)\n", entry.ID, entry.Path, reasons)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func (f *CommandFactory) newDocStampCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stamp [paths...]",
		Short: "Update review stamps on documents",
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")

			graph, err := f.buildDocGraph()
			if err != nil {
				return err
			}

			var targets []string
			if all {
				targets = graph.All()
			} else {
				if len(args) == 0 {
					return fmt.Errorf("provide document paths or use --all")
				}
				targets = args
			}

			stamped, warnings, err := graph.Stamp(targets, time.Now())
			if err != nil {
				return err
			}

			// Load config for auto-commit settings (best-effort; ignore errors).
			var acCfg internalgit.AutoCommitConfig
			if cfg, cfgErr := config.LoadWithWorkingDir(f.docRoot()); cfgErr == nil && cfg.Git != nil {
				acCfg.AutoCommit = cfg.Git.AutoCommit
				acCfg.CommitPrefix = cfg.Git.CommitPrefix
			}

			for _, w := range warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}
			for _, id := range stamped {
				doc, ok := graph.Documents[id]
				path := id
				if ok {
					path = doc.Path
				}
				fmt.Fprintf(cmd.OutOrStdout(), "stamped %s\n", path)
				autoCommitPath := path
				if ok && !filepath.IsAbs(autoCommitPath) {
					autoCommitPath = filepath.Join(graph.RootDir, autoCommitPath)
				}
				if _, acErr := internalgit.AutoCommit(autoCommitPath, id, "stamp reviewed", acCfg); acErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: auto-commit failed for %s: %v\n", path, acErr)
				}
			}
			return nil
		},
	}
	cmd.Flags().Bool("all", false, "Stamp all documents")
	return cmd
}

func (f *CommandFactory) newDocShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show document metadata and status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			graph, err := f.buildDocGraph()
			if err != nil {
				return err
			}
			doc, ok := graph.Show(args[0])
			if !ok {
				return fmt.Errorf("document not found: %s", args[0])
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				type showEntry struct {
					ID         string                   `json:"id"`
					Path       string                   `json:"path"`
					Title      string                   `json:"title,omitempty"`
					DependsOn  []string                 `json:"depends_on,omitempty"`
					Dependents []string                 `json:"dependents,omitempty"`
					Hash       string                   `json:"hash,omitempty"`
					Review     *docgraph.ReviewMetadata `json:"review,omitempty"`
				}
				var rev *docgraph.ReviewMetadata
				if doc.Review.SelfHash != "" {
					rev = &doc.Review
				}
				entry := showEntry{
					ID:         doc.ID,
					Path:       doc.Path,
					Title:      doc.Title,
					DependsOn:  doc.DependsOn,
					Dependents: doc.Dependents,
					Review:     rev,
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(entry)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "ID:         %s\n", doc.ID)
			fmt.Fprintf(out, "Path:       %s\n", doc.Path)
			if doc.Title != "" {
				fmt.Fprintf(out, "Title:      %s\n", doc.Title)
			}
			if len(doc.DependsOn) > 0 {
				fmt.Fprintf(out, "Deps:       %s\n", strings.Join(doc.DependsOn, ", "))
			}
			if len(doc.Dependents) > 0 {
				fmt.Fprintf(out, "Dependents: %s\n", strings.Join(doc.Dependents, ", "))
			}
			if doc.Review.SelfHash != "" {
				fmt.Fprintf(out, "Self Hash:  %s\n", doc.Review.SelfHash)
			}
			if doc.Review.ReviewedAt != "" {
				fmt.Fprintf(out, "Reviewed:   %s\n", doc.Review.ReviewedAt)
			}

			staleInfo, _ := graph.StaleReasonForID(doc.ID)
			if len(staleInfo.Reasons) > 0 {
				fmt.Fprintf(out, "Status:     STALE (%s)\n", strings.Join(staleInfo.Reasons, "; "))
			} else {
				fmt.Fprintf(out, "Status:     fresh\n")
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func (f *CommandFactory) newDocMigrateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate [path]",
		Short: "Convert legacy `dun:` frontmatter to `ddx:`",
		Long: `Migrate markdown documents by replacing legacy ` + "`" + `dun:` + "`" + ` frontmatter namespaces with ` + "`" + `ddx:` + "`" + `.

Examples:
  ddx doc migrate
  ddx doc migrate docs/`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := f.docRoot()
			if len(args) == 1 {
				target = args[0]
			}

			_, err := os.Stat(target)
			if err != nil {
				return fmt.Errorf("path not found: %s", target)
			}

			convertCount := 0
			skipCount := 0

			err = filepath.WalkDir(target, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					return nil
				}
				if !strings.HasSuffix(d.Name(), ".md") {
					return nil
				}

				content, readErr := os.ReadFile(path)
				if readErr != nil {
					return readErr
				}

				fm, body, parseErr := docgraph.ParseFrontmatter(content)
				if parseErr != nil || !fm.HasFrontmatter || fm.Namespace != "dun" {
					if parseErr != nil {
						return parseErr
					}
					skipCount++
					return nil
				}

				didMigrate := docgraph.MigrateLegacyDunFrontmatter(fm.Raw)
				if !didMigrate {
					skipCount++
					return nil
				}

				frontmatterText, encodeErr := docgraph.EncodeFrontmatter(fm.Raw)
				if encodeErr != nil {
					return encodeErr
				}

				updated := []byte(fmt.Sprintf("---\n%s\n---\n%s", frontmatterText, body))
				if err := os.WriteFile(path, updated, 0644); err != nil {
					return err
				}
				convertCount++
				return nil
			})
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "migrated %d files, skipped %d files\n", convertCount, skipCount)
			return nil
		},
	}
	return cmd
}

func (f *CommandFactory) newDocDepsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "deps <id>",
		Short: "Show what a document depends on",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			graph, err := f.buildDocGraph()
			if err != nil {
				return err
			}
			deps, err := graph.Dependencies(args[0])
			if err != nil {
				return err
			}
			if len(deps) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No dependencies.")
				return nil
			}
			for _, id := range deps {
				doc := graph.Documents[id]
				if doc != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "%s  %s\n", id, doc.Path)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s  (not found)\n", id)
				}
			}
			return nil
		},
	}
}

func (f *CommandFactory) newDocDependentsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "dependents <id>",
		Short: "Show what depends on a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			graph, err := f.buildDocGraph()
			if err != nil {
				return err
			}
			dependents, err := graph.DependentIDs(args[0])
			if err != nil {
				return err
			}
			if len(dependents) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No dependents.")
				return nil
			}
			for _, id := range dependents {
				doc := graph.Documents[id]
				if doc != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "%s  %s\n", id, doc.Path)
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), id)
				}
			}
			return nil
		},
	}
}

func (f *CommandFactory) newDocValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate document graph (check for cycles, missing deps)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			graph, err := f.buildDocGraph()
			if err != nil {
				return err
			}
			if len(graph.Warnings) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Document graph is valid.")
				return nil
			}
			for _, w := range graph.Warnings {
				fmt.Fprintf(cmd.OutOrStdout(), "warning: %s\n", w)
			}
			return nil
		},
	}
}

func printDocGraphJSON(cmd *cobra.Command, graph *docgraph.Graph) error {
	type graphNode struct {
		ID         string   `json:"id"`
		Path       string   `json:"path"`
		Title      string   `json:"title,omitempty"`
		DependsOn  []string `json:"depends_on,omitempty"`
		Dependents []string `json:"dependents,omitempty"`
	}
	nodes := make([]graphNode, 0, len(graph.Documents))
	for _, doc := range graph.Documents {
		nodes = append(nodes, graphNode{
			ID:         doc.ID,
			Path:       doc.Path,
			Title:      doc.Title,
			DependsOn:  doc.DependsOn,
			Dependents: doc.Dependents,
		})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(nodes)
}

func printDocGraphText(cmd *cobra.Command, graph *docgraph.Graph) error {
	if len(graph.Documents) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No documents with ddx: frontmatter found.")
		return nil
	}

	ids := graph.All()
	for _, id := range ids {
		doc := graph.Documents[id]
		deps := ""
		if len(doc.DependsOn) > 0 {
			deps = " -> " + strings.Join(doc.DependsOn, ", ")
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s  %s%s\n", id, doc.Path, deps)
	}
	return nil
}

func (f *CommandFactory) newDocHistoryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history <id>",
		Short: "Show commit log for an artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			graph, err := f.buildDocGraph()
			if err != nil {
				return err
			}
			doc, ok := graph.Documents[args[0]]
			if !ok {
				return fmt.Errorf("document not found: %s", args[0])
			}

			since, _ := cmd.Flags().GetString("since")
			asJSON, _ := cmd.Flags().GetBool("json")

			gitArgs := []string{"log", "--follow", "--format=%H\t%ai\t%an\t%s"}
			if since != "" {
				gitArgs = append(gitArgs, since+"..HEAD")
			}
			gitArgs = append(gitArgs, "--", doc.Path)

			gitCmd := internalgit.Command(cmd.Context(), f.docRoot(), gitArgs...)
			out, gitErr := gitCmd.Output()
			if gitErr != nil {
				if exitErr, ok2 := gitErr.(*exec.ExitError); ok2 {
					if strings.Contains(string(exitErr.Stderr), "not a git repository") {
						return fmt.Errorf("not a git repository")
					}
				}
				return fmt.Errorf("git log failed: %w", gitErr)
			}

			type commitEntry struct {
				Hash    string `json:"hash"`
				Date    string `json:"date"`
				Author  string `json:"author"`
				Message string `json:"message"`
			}

			lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
			entries := make([]commitEntry, 0, len(lines))
			for _, line := range lines {
				if line == "" {
					continue
				}
				parts := strings.SplitN(line, "\t", 4)
				if len(parts) < 4 {
					continue
				}
				hash := parts[0]
				if len(hash) > 7 {
					hash = hash[:7]
				}
				date := parts[1]
				if len(date) > 10 {
					date = date[:10]
				}
				entries = append(entries, commitEntry{
					Hash:    hash,
					Date:    date,
					Author:  parts[2],
					Message: parts[3],
				})
			}

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(entries)
			}

			if len(entries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No commits found.")
				return nil
			}
			for _, e := range entries {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s  %s\n", e.Hash, e.Date, e.Author, e.Message)
			}
			return nil
		},
	}
	cmd.Flags().String("since", "", "Show commits since this ref (e.g. HEAD~10)")
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func (f *CommandFactory) newDocDiffCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <id> [<ref1>] [<ref2>]",
		Short: "Show content diff for an artifact",
		Args:  cobra.RangeArgs(1, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			graph, err := f.buildDocGraph()
			if err != nil {
				return err
			}
			doc, ok := graph.Documents[args[0]]
			if !ok {
				return fmt.Errorf("document not found: %s", args[0])
			}

			var gitArgs []string
			switch len(args) {
			case 1:
				gitArgs = []string{"diff", "--", doc.Path}
			case 2:
				gitArgs = []string{"diff", args[1], "--", doc.Path}
			default:
				gitArgs = []string{"diff", args[1], args[2], "--", doc.Path}
			}

			gitCmd := internalgit.Command(cmd.Context(), f.docRoot(), gitArgs...)
			out, gitErr := gitCmd.Output()
			if gitErr != nil {
				if exitErr, ok2 := gitErr.(*exec.ExitError); ok2 {
					if strings.Contains(string(exitErr.Stderr), "not a git repository") {
						return fmt.Errorf("not a git repository")
					}
				}
				return fmt.Errorf("git diff failed: %w", gitErr)
			}

			if len(out) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No differences.")
				return nil
			}
			_, err = cmd.OutOrStdout().Write(out)
			return err
		},
	}
}

func (f *CommandFactory) newDocChangedCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "changed",
		Short: "List artifacts changed since a git ref",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			since, _ := cmd.Flags().GetString("since")
			if since == "" {
				since = "HEAD~5"
			}
			asJSON, _ := cmd.Flags().GetBool("json")

			out, gitErr := internalgit.Command(cmd.Context(), "", "diff", "--name-status", since, "HEAD").Output()
			if gitErr != nil {
				if exitErr, ok := gitErr.(*exec.ExitError); ok {
					if strings.Contains(string(exitErr.Stderr), "not a git repository") {
						return fmt.Errorf("not a git repository")
					}
				}
				return fmt.Errorf("git diff failed: %w", gitErr)
			}

			rootOut, gitErr := internalgit.Command(cmd.Context(), "", "rev-parse", "--show-toplevel").Output()
			if gitErr != nil {
				return fmt.Errorf("could not determine git root: %w", gitErr)
			}
			repoRoot := strings.TrimRight(string(rootOut), "\n")

			type changedEntry struct {
				ID         string `json:"id"`
				Path       string `json:"path"`
				ChangeType string `json:"change_type"`
			}

			graph, err := f.buildDocGraph()
			if err != nil {
				return err
			}

			var entries []changedEntry
			for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
				if line == "" {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) < 2 {
					continue
				}
				statusCode := fields[0]
				relPath := fields[len(fields)-1]

				if !strings.HasSuffix(relPath, ".md") {
					continue
				}

				absPath := filepath.Join(repoRoot, relPath)
				cleanPath := filepath.Clean(absPath)
				graphKey := cleanPath
				if rel, relErr := filepath.Rel(graph.RootDir, cleanPath); relErr == nil {
					graphKey = rel
				}

				var changeType string
				switch {
				case strings.HasPrefix(statusCode, "A"):
					changeType = "added"
				case strings.HasPrefix(statusCode, "D"):
					changeType = "deleted"
				default:
					changeType = "modified"
				}

				if changeType == "deleted" {
					if id, ok := graph.PathToID[graphKey]; ok {
						entries = append(entries, changedEntry{ID: id, Path: relPath, ChangeType: changeType})
					}
					continue
				}

				if id, ok := graph.PathToID[graphKey]; ok {
					entries = append(entries, changedEntry{ID: id, Path: relPath, ChangeType: changeType})
					continue
				}

				content, readErr := os.ReadFile(absPath)
				if readErr != nil {
					continue
				}
				fm, _, parseErr := docgraph.ParseFrontmatter(content)
				if parseErr != nil || !fm.HasFrontmatter || fm.Doc.ID == "" {
					continue
				}
				entries = append(entries, changedEntry{ID: fm.Doc.ID, Path: relPath, ChangeType: changeType})
			}

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(entries)
			}

			if len(entries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No artifact changes found.")
				return nil
			}
			for _, e := range entries {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s\n", e.ID, e.Path, e.ChangeType)
			}
			return nil
		},
	}
	cmd.Flags().String("since", "", "Compare from this ref (default: HEAD~5)")
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}
