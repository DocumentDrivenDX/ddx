package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ACClaim is one structural claim parsed from a bead's acceptance criteria.
// It names a concrete artifact (test function, file path, or struct field)
// whose state can be checked against the worktree without running tests.
//
// The already_satisfied gate uses these to refuse closure when the AC names
// a structural property the implementation has not actually exercised.
type ACClaim struct {
	Kind  string
	Value string
}

const (
	// ACClaimTestName names a Go test or benchmark function that the AC
	// expects to pass. The verifier checks whether the no_changes rationale
	// cites the test by name (the worker must claim to have exercised it).
	ACClaimTestName = "test_name"
	// ACClaimFileDeleted names a file path the AC asserts has been removed
	// or no longer exists. The verifier stats the path under projectRoot.
	ACClaimFileDeleted = "file_deleted"
	// ACClaimFieldRemoved names a struct field the AC asserts has been
	// removed. The verifier scans *.go files under projectRoot for a
	// declaration of that field.
	ACClaimFieldRemoved = "field_removed"
)

var (
	reACTestName = regexp.MustCompile(`\b(?:Test|Benchmark)[A-Z]\w*\b`)

	// "<path> deleted|removed|no longer exists"
	reACFileDeletedTrailing = regexp.MustCompile(`([\w./-]+\.\w+)\s+(?:is\s+)?(?:deleted|removed|no longer exists)`)
	// "deleted: <path>", "removed: <path>", "no longer contains <path>", "file <path> deleted"
	reACFileDeletedLeading = regexp.MustCompile(`(?:deleted:?\s+|removed:?\s+|no longer contains?\s+|file\s+)([\w./-]+\.\w+)`)

	// "field X removed", "field X is gone", "field X has been removed"
	reACFieldRemovedTrailing = regexp.MustCompile(`(?i)\bfield\s+(\w+)\s+(?:is\s+)?(?:removed|gone|has been removed|deleted)`)
	// "no field X", "does not have field X", "without field X",
	// "no longer has field X", "no longer contains field X"
	reACFieldRemovedLeading = regexp.MustCompile(`(?i)(?:no field|does not have field|without field|no longer (?:has|contains?) field)\s+(\w+)`)
)

// ParseACClaims extracts structural claims from the acceptance text. Empty
// or claim-free acceptance returns nil. Duplicate claims are collapsed.
func ParseACClaims(acceptance string) []ACClaim {
	if acceptance == "" {
		return nil
	}
	seen := map[string]bool{}
	var claims []ACClaim
	add := func(kind, value string) {
		if value == "" {
			return
		}
		key := kind + ":" + value
		if seen[key] {
			return
		}
		seen[key] = true
		claims = append(claims, ACClaim{Kind: kind, Value: value})
	}
	for _, m := range reACTestName.FindAllString(acceptance, -1) {
		add(ACClaimTestName, m)
	}
	for _, m := range reACFileDeletedTrailing.FindAllStringSubmatch(acceptance, -1) {
		add(ACClaimFileDeleted, m[1])
	}
	for _, m := range reACFileDeletedLeading.FindAllStringSubmatch(acceptance, -1) {
		add(ACClaimFileDeleted, m[1])
	}
	for _, m := range reACFieldRemovedTrailing.FindAllStringSubmatch(acceptance, -1) {
		add(ACClaimFieldRemoved, m[1])
	}
	for _, m := range reACFieldRemovedLeading.FindAllStringSubmatch(acceptance, -1) {
		add(ACClaimFieldRemoved, m[1])
	}
	return claims
}

// VerifyACClaims checks each claim against the project worktree and the
// worker's no_changes rationale. Returns (true, "") if every claim holds.
// On the first failure returns (false, reason) describing the unmet claim
// so the caller can record why already_satisfied was refused.
//
// projectRoot may be empty, in which case file/field claims are skipped
// (verifier degrades to a rationale-only check). This keeps the function
// usable in unit-test contexts where no worktree is available.
func VerifyACClaims(claims []ACClaim, projectRoot, rationale string) (bool, string) {
	for _, c := range claims {
		switch c.Kind {
		case ACClaimTestName:
			if !strings.Contains(rationale, c.Value) {
				return false, fmt.Sprintf("AC names test %q but no_changes rationale does not cite it", c.Value)
			}
		case ACClaimFileDeleted:
			if projectRoot == "" {
				continue
			}
			p := c.Value
			if !filepath.IsAbs(p) {
				p = filepath.Join(projectRoot, p)
			}
			if _, err := os.Stat(p); err == nil {
				return false, fmt.Sprintf("AC claims %q deleted but file still exists", c.Value)
			}
		case ACClaimFieldRemoved:
			if projectRoot == "" {
				continue
			}
			found, err := goFieldDeclared(projectRoot, c.Value)
			if err == nil && found {
				return false, fmt.Sprintf("AC claims field %q removed but field still declared", c.Value)
			}
		}
	}
	return true, ""
}

// goFieldDeclared reports whether any *.go file under root contains a field
// declaration whose name matches `name` (i.e. the identifier followed by a
// type token: "Name Type" or "Name *Type" or "Name []Type").
func goFieldDeclared(root, name string) (bool, error) {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\s+[\*\[\]\w.]+`)
	found := false
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || found {
			return walkErr
		}
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if re.Match(data) {
			found = true
		}
		return nil
	})
	return found, err
}
