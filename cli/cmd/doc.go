package cmd

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/easel/ddx/internal/docgraph"
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
