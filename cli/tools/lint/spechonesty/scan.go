// Docs-directory scan for the spechonesty CLI (WB-1 validation path).
//
// Walks a docs tree for Markdown files, runs the status parser, and
// collects parse / missing-status diagnostics. Read-only: never writes.
package spechonesty

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Diagnostic is one CLI-reported failure for a docs-directory scan.
type Diagnostic struct {
	// Path is the document path relative to the scan root when possible,
	// otherwise the absolute path passed to the parser.
	Path string
	// Line is the 1-based diagnostic line (0 when not applicable).
	Line int
	// Kind classifies the failure (e.g. missing_status, parse_error).
	Kind string
	// Message is a human-readable description.
	Message string
}

// ScanDocsDirectory walks root recursively for `.md` files, parses each
// document's status stamp, and returns diagnostics for:
//   - I/O or parse errors
//   - missing status on SD/TD/ADR design documents
//   - duplicate document ids within the helix lint scope (HelixLintRelativeDirs)
//   - duplicate user-story ids across feature documents (01-frame/features/)
//
// The tree is never modified. Non-design documents without a status stamp
// do not produce a missing-status diagnostic. Duplicate-id and
// duplicate-US-id failures are non-waivable (WB-1 step 5).
func ScanDocsDirectory(root string) ([]Diagnostic, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		diag, err := scanOne(root)
		if err != nil {
			return nil, err
		}
		if diag == nil {
			return nil, nil
		}
		return []Diagnostic{*diag}, nil
	}

	var diags []Diagnostic
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		diag, scanErr := scanOne(path)
		if scanErr != nil {
			diags = append(diags, Diagnostic{
				Path:    path,
				Kind:    "parse_error",
				Message: scanErr.Error(),
			})
			return nil
		}
		if diag != nil {
			diags = append(diags, *diag)
		}
		return nil
	})
	if err != nil {
		return diags, err
	}

	// Duplicate document-id scan is confined to HelixLintRelativeDirs under root.
	dupDiags, dupErr := ScanDuplicateDocumentIDs(root)
	if dupErr != nil {
		return diags, dupErr
	}
	diags = append(diags, dupDiags...)

	// Duplicate US-id scan is confined to 01-frame/features/ under root.
	usDiags, usErr := ScanDuplicateUserStoryIDs(root)
	if usErr != nil {
		return diags, usErr
	}
	diags = append(diags, usDiags...)
	return diags, nil
}

// scanOne parses status for a single markdown file. Returns a diagnostic
// when the document fails the missing-status rule; nil when clean.
func scanOne(path string) (*Diagnostic, error) {
	res, err := ParseDocumentStatus(path)
	if err != nil {
		return nil, fmt.Errorf("parse status %s: %w", path, err)
	}
	if res.MissingDesignStatus {
		return &Diagnostic{
			Path:    path,
			Line:    1,
			Kind:    string(FindingMissingStatus),
			Message: "missing status stamp: SD/TD/ADR documents require a body **Status:** line or frontmatter status: key",
		}, nil
	}
	return nil, nil
}
