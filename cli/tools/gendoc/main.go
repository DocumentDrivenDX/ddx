// gendoc generates Hugo-compatible markdown documentation for every ddx CLI
// command and writes it to website/content/docs/cli/commands/.
//
// Usage:
//
//	go run ./tools/gendoc [output-dir]
//
// If output-dir is omitted it defaults to ../website/content/docs/cli/commands
// relative to the cli/ root (i.e. the standard website location).
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func main() {
	// Determine output directory.
	outDir := defaultOutDir()
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Fatalf("creating output dir %s: %v", outDir, err)
	}

	// Build the root command (no working dir needed for doc generation).
	factory := cmd.NewCommandFactory("/tmp")
	root := factory.NewRootCommand()

	// Disable persistent hooks that would try to load config/network during
	// doc generation — we only need the metadata (Use, Short, Long, Flags).
	root.PersistentPreRunE = nil
	root.PersistentPreRun = nil
	root.PersistentPostRunE = nil
	root.PersistentPostRun = nil

	if err := doc.GenMarkdownTreeCustom(root, outDir, frontMatter, linkHandler); err != nil {
		log.Fatalf("generating docs: %v", err)
	}

	// Write a generated _index.md that lists every top-level command.
	if err := writeIndex(outDir, root); err != nil {
		log.Fatalf("writing index: %v", err)
	}

	fmt.Printf("docs written to %s\n", outDir)
}

// frontMatter returns Hugo front matter for the given filename.
// cobra/doc passes the full path; we derive the title from the basename.
func frontMatter(filename string) string {
	base := filepath.Base(filename)
	name := strings.TrimSuffix(base, ".md")

	// Convert "ddx_bead_create" → "bead create"
	title := strings.TrimPrefix(name, "ddx")
	title = strings.TrimPrefix(title, "_")
	title = strings.ReplaceAll(title, "_", " ")
	if title == "" {
		title = "ddx"
	}

	// Weight: root gets 1; subcommands sorted alphabetically get higher weights.
	// We use a simple fixed value — Hugo sorts within the section alphabetically.
	return fmt.Sprintf("---\ntitle: %q\ngenerated: true\n---\n\n", title)
}

// linkHandler rewrites cobra/doc cross-links so they point to the correct
// Hugo page path instead of neighbouring .md files.
func linkHandler(filename string) string {
	base := filepath.Base(filename)
	name := strings.TrimSuffix(base, ".md")
	return "/docs/cli/commands/" + name + "/"
}

// writeIndex writes an _index.md that lists each top-level command with its
// short description, linking to the generated command page.
func writeIndex(outDir string, root *cobra.Command) error {
	var sb strings.Builder
	sb.WriteString("---\ntitle: \"Command Reference\"\nweight: 10\n---\n\n")
	sb.WriteString("Auto-generated reference for every `ddx` command.\n\n")
	sb.WriteString("| Command | Description |\n")
	sb.WriteString("|---------|-------------|\n")

	for _, sub := range root.Commands() {
		if sub.Hidden {
			continue
		}
		name := sub.Name()
		short := sub.Short
		link := linkHandler("ddx_" + name + ".md")
		sb.WriteString(fmt.Sprintf("| [`ddx %s`](%s) | %s |\n", name, link, short))
	}

	return os.WriteFile(filepath.Join(outDir, "_index.md"), []byte(sb.String()), 0644)
}

// defaultOutDir returns the default output directory relative to the location
// of this source file (which lives at cli/tools/gendoc/).
func defaultOutDir() string {
	// When run as "go run ./tools/gendoc" from cli/, the working directory is cli/.
	// Walk up until we find the website/ sibling directory.
	cwd, err := os.Getwd()
	if err != nil {
		return "website/content/docs/cli/commands"
	}

	// If running from cli/ directory (normal case):
	candidate := filepath.Join(cwd, "..", "website", "content", "docs", "cli", "commands")
	if _, err := os.Stat(filepath.Join(cwd, "..", "website")); err == nil {
		return candidate
	}

	// Fallback: relative to cwd
	return filepath.Join(cwd, "website", "content", "docs", "cli", "commands")
}
