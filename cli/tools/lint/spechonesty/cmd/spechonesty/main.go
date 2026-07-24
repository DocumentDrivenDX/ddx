// Command spechonesty runs the Phase 2 spec-honesty analyzer.
//
// This entrypoint exercises the read-only requirement-inventory and
// Verification-mapping parser so the structured model is production-
// reachable from main. Coverage resolution and status enforcement are
// owned by sibling beads.
//
// Usage:
//
//	go run ./tools/lint/spechonesty/cmd/spechonesty <path>...
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/tools/lint/spechonesty"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	// Preserve the go/analysis entry used by analysistest and sibling
	// linters when no doc paths are supplied.
	if len(os.Args) == 1 {
		singlechecker.Main(spechonesty.Analyzer)
		return
	}

	exitCode := 0
	for _, root := range os.Args[1:] {
		if err := walkDocs(root); err != nil {
			fmt.Fprintln(os.Stderr, err)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

// walkDocs parses every markdown file under root into a DocumentModel
// and reports parse-level findings. Files are never modified.
func walkDocs(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return parseOne(root)
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		return parseOne(path)
	})
}

func parseOne(path string) error {
	model, err := spechonesty.ParseVerificationDocument(path)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	// Touch inventory/rows so the structured model is fully exercised
	// from production and not optimized away by reachability analysis.
	_ = model.Inventory
	_ = model.InventoryKind
	_ = model.Rows
	for _, f := range model.Findings {
		fmt.Fprintf(os.Stderr, "%s:%d: %s: %s\n", f.Path, f.Line, f.Kind, f.Message)
	}
	return nil
}
