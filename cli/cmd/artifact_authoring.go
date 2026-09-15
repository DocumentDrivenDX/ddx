package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/docconvert"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
	"github.com/spf13/cobra"
)

// Check-in and check-out implement the external-tool authoring cycle the HELIX
// artifact schema defines but leaves to the executable layer:
//
//	checked-out  the document is being edited in the external tool; the body
//	             is the last known copy and is not dependable.
//	checked-in   the body was extracted from the committed export and matches
//	             the external document as of that extraction.
//
// Check-in is what makes `checked-in` a producible state rather than an
// aspiration, and the export_sha256 it records is what makes the claim
// checkable afterwards.

func (f *CommandFactory) newArtifactCheckInCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check-in <artifact-id>",
		Short: "Land an external-tool artifact's exported content into its Markdown body",
		Long: `Extract the committed export of an external-tool artifact into its Markdown body.

The artifact must declare ddx.authoring.home: external-tool and
ddx.authoring.export, the repository-relative path to the original file
exported from the authoring tool. That file is converted to Markdown, the
artifact's body is replaced with the result, and ddx.authoring.state is set to
checked-in alongside ddx.authoring.export_sha256 — the digest of the export the
body was extracted from, so the match is verifiable later.

The frontmatter is preserved; only the body and the authoring state change.
ddx.authoring.origin remains the write surface: edits go to the tool, not to
the Markdown.

Supported export formats: ` + strings.Join(docconvert.SupportedExtensions(), ", ") + `.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return f.runArtifactCheckIn(cmd, strings.TrimSpace(args[0]))
		},
	}
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) newArtifactCheckOutCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check-out <artifact-id>",
		Short: "Mark an external-tool artifact as being authored in its external tool",
		Long: `Set ddx.authoring.state to checked-out on an external-tool artifact.

Checking out marks the body as not dependable while the document is edited in
the tool at ddx.authoring.origin. It never deletes content the repository
already has: the body and ddx.authoring.export from the previous check-in stay
in place as the last known copy until the next check-in replaces them.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return f.runArtifactCheckOut(cmd, strings.TrimSpace(args[0]))
		},
	}
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) runArtifactCheckIn(cmd *cobra.Command, artifactID string) error {
	artifact, err := f.loadExternalToolArtifact(artifactID)
	if err != nil {
		return err
	}

	export := strings.TrimSpace(artifact.Authoring.Export)
	if export == "" {
		return fmt.Errorf("artifact %q declares no ddx.authoring.export; add the repository-relative path of the file exported from %s before checking in",
			artifactID, describeAuthoringTool(artifact.Authoring.Tool))
	}
	exportPath, err := artifact.resolve(export)
	if err != nil {
		return fmt.Errorf("artifact %q declares ddx.authoring.export %q: %w", artifactID, export, err)
	}
	info, err := os.Stat(exportPath)
	if err != nil {
		return fmt.Errorf("artifact %q declares ddx.authoring.export %q, but %s cannot be read: %w", artifactID, export, exportPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("artifact %q declares ddx.authoring.export %q, but %s is a directory", artifactID, export, exportPath)
	}

	body, err := docconvert.Convert(exportPath, export)
	if err != nil {
		return err
	}
	digest, err := fileSHA256(exportPath)
	if err != nil {
		return err
	}

	if err := docgraph.SetAuthoringFields(artifact.Frontmatter.Raw, map[string]string{
		"state":         docgraph.AuthoringStateCheckedIn,
		"export_sha256": digest,
	}); err != nil {
		return err
	}
	if err := artifact.write(body); err != nil {
		return err
	}

	result := artifactAuthoringResult{
		ID:           artifactID,
		Path:         artifact.RelPath,
		State:        docgraph.AuthoringStateCheckedIn,
		Tool:         artifact.Authoring.Tool,
		Origin:       artifact.Authoring.Origin,
		Export:       export,
		ExportSHA256: digest,
	}
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		return writeJSON(cmd.OutOrStdout(), result)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "checked in %s from %s (sha256:%s)\n", artifactID, export, digest[:12])
	return nil
}

func (f *CommandFactory) runArtifactCheckOut(cmd *cobra.Command, artifactID string) error {
	artifact, err := f.loadExternalToolArtifact(artifactID)
	if err != nil {
		return err
	}

	if err := docgraph.SetAuthoringFields(artifact.Frontmatter.Raw, map[string]string{
		"state": docgraph.AuthoringStateCheckedOut,
	}); err != nil {
		return err
	}
	// The body is written back unchanged: a checkout leaves the previous
	// check-in in place as the last known copy.
	if err := artifact.write(artifact.Body); err != nil {
		return err
	}

	result := artifactAuthoringResult{
		ID:           artifactID,
		Path:         artifact.RelPath,
		State:        docgraph.AuthoringStateCheckedOut,
		Tool:         artifact.Authoring.Tool,
		Origin:       artifact.Authoring.Origin,
		Export:       artifact.Authoring.Export,
		ExportSHA256: artifact.Authoring.ExportSHA256,
	}
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		return writeJSON(cmd.OutOrStdout(), result)
	}
	if artifact.Authoring.Origin != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "checked out %s; author at %s\n", artifactID, artifact.Authoring.Origin)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "checked out %s\n", artifactID)
	return nil
}

// artifactAuthoringResult is the --json shape shared by check-in and check-out.
type artifactAuthoringResult struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	State        string `json:"state"`
	Tool         string `json:"tool,omitempty"`
	Origin       string `json:"origin,omitempty"`
	Export       string `json:"export,omitempty"`
	ExportSHA256 string `json:"export_sha256,omitempty"`
}

// externalToolArtifact is one artifact document resolved on disk, with its
// frontmatter node kept live so callers can edit and re-encode it.
type externalToolArtifact struct {
	ID          string
	RootDir     string
	RelPath     string
	AbsPath     string
	Frontmatter docgraph.Frontmatter
	Body        string
	Authoring   docgraph.DocAuthoring
}

// resolve turns a repository-relative path from the frontmatter into an
// absolute one, against the document-graph root — the repository root under
// normal invocation.
//
// The schema calls export "a path, not a URL", and specifically a
// repository-relative one. A path that is absolute or climbs out of the root
// is rejected rather than followed: check-in copies whatever it is pointed at
// into a committed file, so it must not be steerable outside the repository.
func (a *externalToolArtifact) resolve(relative string) (string, error) {
	if filepath.IsAbs(relative) || filepath.IsAbs(filepath.FromSlash(relative)) {
		return "", fmt.Errorf("must be a repository-relative path, not an absolute one")
	}
	resolved := filepath.Join(a.RootDir, filepath.FromSlash(relative))
	within, err := filepath.Rel(a.RootDir, resolved)
	if err != nil {
		return "", err
	}
	if within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("resolves outside the repository root %s", a.RootDir)
	}
	return resolved, nil
}

// write re-encodes the frontmatter and writes it back with the given body.
func (a *externalToolArtifact) write(body string) error {
	frontmatterText, err := docgraph.EncodeFrontmatter(a.Frontmatter.Raw)
	if err != nil {
		return err
	}
	updated := "---\n" + frontmatterText + "\n---\n" + body
	return os.WriteFile(a.AbsPath, []byte(updated), 0644)
}

// loadExternalToolArtifact finds the artifact by its ddx.id and rejects
// anything that is not authored in an external tool. The document graph is the
// same lookup `artifact regenerate` uses, so an ID that resolves for one
// subcommand resolves for the others.
func (f *CommandFactory) loadExternalToolArtifact(artifactID string) (*externalToolArtifact, error) {
	if artifactID == "" {
		return nil, fmt.Errorf("artifact ID is required")
	}

	graph, err := f.buildDocGraph()
	if err != nil {
		return nil, err
	}
	doc, ok := graph.Show(artifactID)
	if !ok {
		return nil, fmt.Errorf("artifact %q not found in document graph", artifactID)
	}

	absPath := doc.Path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(graph.RootDir, doc.Path)
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read artifact %q: %w", artifactID, err)
	}
	frontmatter, body, err := docgraph.ParseFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("parse artifact %q: %w", artifactID, err)
	}
	if !frontmatter.HasFrontmatter || frontmatter.Raw == nil {
		return nil, fmt.Errorf("artifact %q at %s has no frontmatter", artifactID, doc.Path)
	}

	authoring := frontmatter.Doc.Authoring
	switch authoring.Home {
	case docgraph.AuthoringHomeExternalTool:
	case "":
		return nil, fmt.Errorf("artifact %q declares no ddx.authoring.home; check-in and check-out apply only to artifacts with home: %s",
			artifactID, docgraph.AuthoringHomeExternalTool)
	default:
		return nil, fmt.Errorf("artifact %q has ddx.authoring.home: %s; check-in and check-out apply only to artifacts with home: %s",
			artifactID, authoring.Home, docgraph.AuthoringHomeExternalTool)
	}

	return &externalToolArtifact{
		ID:          artifactID,
		RootDir:     graph.RootDir,
		RelPath:     doc.Path,
		AbsPath:     absPath,
		Frontmatter: frontmatter,
		Body:        body,
		Authoring:   authoring,
	}, nil
}

func describeAuthoringTool(tool string) string {
	if strings.TrimSpace(tool) == "" {
		return "the authoring tool"
	}
	return tool
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func writeJSON(out io.Writer, value any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
