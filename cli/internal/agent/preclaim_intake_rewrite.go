package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

type preClaimIntakeRewriteSnapshot struct {
	DescriptionBytes int    `json:"description_bytes,omitempty"`
	AcceptanceBytes  int    `json:"acceptance_bytes,omitempty"`
	DescriptionSHA   string `json:"description_sha256,omitempty"`
	AcceptanceSHA    string `json:"acceptance_sha256,omitempty"`
}

type preClaimIntakeRewriteEventBody struct {
	Rationale            string                        `json:"rationale,omitempty"`
	ChangedFields        []string                      `json:"changed_fields,omitempty"`
	Before               preClaimIntakeRewriteSnapshot `json:"before"`
	After                preClaimIntakeRewriteSnapshot `json:"after"`
	PreservationEvidence []string                      `json:"preservation_evidence,omitempty"`
}

// Patterns for extracting explicit commitments from bead descriptions.
// A commitment is a durable anchor that must survive a replacement rewrite.
var (
	reGoverningRef = regexp.MustCompile(`\b[A-Z]{2,6}-\d{3,}\b`)
	reNamedTest    = regexp.MustCompile(`\bTest[A-Z]\w+\b`)
	reFileLine     = regexp.MustCompile(`\b[\w/.-]+\.go:\d+\b`)
	reDepID        = regexp.MustCompile(`\bddx-[0-9a-f]{8}\b`)
)

var descriptionSectionHeaders = []string{
	"PROBLEM", "ROOT CAUSE", "PROPOSED FIX", "NON-SCOPE", "NON SCOPE",
	"CONTEXT", "PARENT", "DEPS", "DEPENDENCIES", "BACKGROUND",
}

func isDescriptionSectionHeader(line string) bool {
	upper := strings.ToUpper(strings.TrimRight(normalizeWhitespace(line), ":"))
	for _, h := range descriptionSectionHeaders {
		if upper == h {
			return true
		}
	}
	return false
}

// extractNonScopeBullets returns normalized bullet text from the NON-SCOPE
// section of a bead description.
func extractNonScopeBullets(desc string) []string {
	lines := strings.Split(desc, "\n")
	var inNonScope bool
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(strings.TrimRight(normalizeWhitespace(trimmed), ":"))
		if upper == "NON-SCOPE" || upper == "NON SCOPE" {
			inNonScope = true
			continue
		}
		if inNonScope {
			if trimmed == "" {
				continue
			}
			if isDescriptionSectionHeader(trimmed) {
				inNonScope = false
				continue
			}
			bullet := strings.TrimLeft(trimmed, "-•*+# ")
			bullet = normalizeWhitespace(bullet)
			if bullet != "" {
				result = append(result, bullet)
			}
		}
	}
	return result
}

// extractDescriptionCommitments returns durable anchor strings that must be
// preserved across a description replacement rewrite:
//   - Governing artifact references (FEAT-010, ADR-023, etc.)
//   - Named test functions (TestFoo)
//   - File:line references (pkg/file.go:42)
//   - Dependency IDs (ddx-XXXXXXXX)
//   - NON-SCOPE section bullet points (prefixed with "non_scope:")
func extractDescriptionCommitments(desc string) []string {
	seen := make(map[string]struct{})
	var result []string
	add := func(s string) {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			result = append(result, s)
		}
	}
	for _, m := range reGoverningRef.FindAllString(desc, -1) {
		add(m)
	}
	for _, m := range reNamedTest.FindAllString(desc, -1) {
		add(m)
	}
	for _, m := range reFileLine.FindAllString(desc, -1) {
		add(m)
	}
	for _, m := range reDepID.FindAllString(desc, -1) {
		add(m)
	}
	for _, bullet := range extractNonScopeBullets(desc) {
		add("non_scope:" + bullet)
	}
	return result
}

// validateDescriptionReplacement checks that all explicit commitments in the
// old description are preserved in the new description. It returns the list of
// preservation evidence entries recorded. An error is returned if any
// commitment is missing from the replacement.
func validateDescriptionReplacement(oldDesc, newDesc string) ([]string, error) {
	commitments := extractDescriptionCommitments(oldDesc)
	normNew := normalizeWhitespace(newDesc)
	var evidence []string
	for _, c := range commitments {
		if strings.HasPrefix(c, "non_scope:") {
			checkIn := strings.TrimPrefix(c, "non_scope:")
			if !strings.Contains(normNew, checkIn) {
				return nil, fmt.Errorf("pre-claim intake rewrite: description drops commitment %q", c)
			}
		} else {
			if !strings.Contains(newDesc, c) {
				return nil, fmt.Errorf("pre-claim intake rewrite: description drops commitment %q", c)
			}
		}
		evidence = append(evidence, c)
	}
	return evidence, nil
}

func applyPreClaimIntakeRewrite(store ExecuteBeadLoopStore, beadID, actor string, intake PreClaimIntakeResult, createdAt time.Time) error {
	if store == nil {
		return fmt.Errorf("pre-claim intake rewrite: bead store required")
	}
	if strings.TrimSpace(beadID) == "" {
		return fmt.Errorf("pre-claim intake rewrite: bead id required")
	}
	if intake.normalizedOutcome() != PreClaimIntakeActionableButRewritten {
		return fmt.Errorf("pre-claim intake rewrite: unexpected outcome %q", intake.normalizedOutcome())
	}

	original, err := store.Get(context.Background(), beadID)
	if err != nil {
		return fmt.Errorf("pre-claim intake rewrite: load bead %s: %w", beadID, err)
	}

	rewrite, before, after, preservation, err := validateAndApplyPreClaimIntakeRewrite(original, intake.Rewrite)
	if err != nil {
		return err
	}

	if err := store.Update(context.Background(), beadID, func(b *bead.Bead) {
		if rewrite.Description != "" {
			b.Description = rewrite.Description
		}
		if rewrite.Acceptance != "" {
			b.Acceptance = rewrite.Acceptance
		}
	}); err != nil {
		return fmt.Errorf("pre-claim intake rewrite: update bead %s: %w", beadID, err)
	}

	body, err := json.Marshal(preClaimIntakeRewriteEventBody{
		Rationale:            strings.TrimSpace(intake.Detail),
		ChangedFields:        rewrite.ChangedFields,
		Before:               before,
		After:                after,
		PreservationEvidence: preservation,
	})
	if err != nil {
		return fmt.Errorf("pre-claim intake rewrite: encode event: %w", err)
	}

	summary := strings.Join(rewrite.ChangedFields, ",")
	if summary == "" {
		summary = "rewritten"
	}
	return store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "intake-rewritten",
		Summary:   summary,
		Body:      string(body),
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: createdAt.UTC(),
	})
}

func validateAndApplyPreClaimIntakeRewrite(original *bead.Bead, rewrite PreClaimIntakeRewrite) (PreClaimIntakeRewrite, preClaimIntakeRewriteSnapshot, preClaimIntakeRewriteSnapshot, []string, error) {
	errOut := func(msg string, args ...any) (PreClaimIntakeRewrite, preClaimIntakeRewriteSnapshot, preClaimIntakeRewriteSnapshot, []string, error) {
		return PreClaimIntakeRewrite{}, preClaimIntakeRewriteSnapshot{}, preClaimIntakeRewriteSnapshot{}, nil, fmt.Errorf(msg, args...)
	}

	if original == nil {
		return errOut("pre-claim intake rewrite: bead required")
	}

	normalized := PreClaimIntakeRewrite{
		Description:   strings.TrimSpace(rewrite.Description),
		Acceptance:    strings.TrimSpace(rewrite.Acceptance),
		ChangedFields: normalizePreClaimIntakeRewriteFields(rewrite.ChangedFields),
	}
	if len(normalized.ChangedFields) == 0 {
		return errOut("pre-claim intake rewrite: missing changed_fields")
	}

	allowed := map[string]struct{}{
		"description": {},
		"acceptance":  {},
	}
	for _, field := range normalized.ChangedFields {
		if _, ok := allowed[field]; !ok {
			return errOut("pre-claim intake rewrite: unsafe field %q", field)
		}
	}

	before := snapshotPreClaimIntakeRewrite(original.Description, original.Acceptance)

	var preservation []string
	oldDescription := strings.TrimSpace(original.Description)
	if hasString(normalized.ChangedFields, "description") {
		if normalized.Description == "" {
			return errOut("pre-claim intake rewrite: description rewrite missing")
		}
		if strings.EqualFold(strings.TrimSpace(normalized.Description), oldDescription) {
			return errOut("pre-claim intake rewrite: description unchanged")
		}
		ev, err := validateDescriptionReplacement(oldDescription, normalized.Description)
		if err != nil {
			return PreClaimIntakeRewrite{}, preClaimIntakeRewriteSnapshot{}, preClaimIntakeRewriteSnapshot{}, nil, err
		}
		preservation = ev
	} else if normalized.Description != "" {
		return errOut("pre-claim intake rewrite: description supplied without changed_fields")
	}

	oldAcceptance := strings.TrimSpace(original.Acceptance)
	if hasString(normalized.ChangedFields, "acceptance") {
		if normalized.Acceptance == "" {
			return errOut("pre-claim intake rewrite: acceptance rewrite missing")
		}
		if strings.EqualFold(strings.TrimSpace(normalized.Acceptance), oldAcceptance) {
			return errOut("pre-claim intake rewrite: acceptance unchanged")
		}
		if !acceptancePreservesCriteria(oldAcceptance, normalized.Acceptance) {
			return errOut("pre-claim intake rewrite: acceptance criteria dropped or altered")
		}
	} else if normalized.Acceptance != "" {
		return errOut("pre-claim intake rewrite: acceptance supplied without changed_fields")
	}

	afterDescription := oldDescription
	if containsString(normalized.ChangedFields, "description") {
		afterDescription = normalized.Description
	}
	afterAcceptance := oldAcceptance
	if containsString(normalized.ChangedFields, "acceptance") {
		afterAcceptance = normalized.Acceptance
	}
	after := snapshotPreClaimIntakeRewrite(afterDescription, afterAcceptance)
	return normalized, before, after, preservation, nil
}

func snapshotPreClaimIntakeRewrite(description, acceptance string) preClaimIntakeRewriteSnapshot {
	return preClaimIntakeRewriteSnapshot{
		DescriptionBytes: len(description),
		AcceptanceBytes:  len(acceptance),
		DescriptionSHA:   hashText(description),
		AcceptanceSHA:    hashText(acceptance),
	}
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// acceptancePreservesCriteria returns true when the rewritten acceptance field
// ("after") contains all verifiable assertions from the original ("before").
//
// Structural reformatting — renumbering ("AC1." → "1."), reordering,
// normalising whitespace, or splitting one unnumbered sentence into several
// numbered criteria — is treated as preserving the criteria, not dropping them.
//
// A drop is detected only when the original had parseable numbered criteria and
// one or more of those criteria bodies are absent from the rewrite.
//
// When the original has no parseable numbered criteria (e.g. a single vague
// sentence), any rewrite that adds structured numbered criteria is accepted as
// an expansion; a rewrite that only rephrases the same prose content is checked
// by full-text containment.
func acceptancePreservesCriteria(before, after string) bool {
	beforeCriteria := parseAcceptanceCriteria(before)
	afterCriteria := parseAcceptanceCriteria(after)

	// If the original had no parseable numbered criteria, use a lenient path:
	// any rewrite that introduces numbered criteria is an expansion (always
	// accepted). If the rewrite is also unstructured, fall back to full-text
	// containment on the normalised bodies.
	if len(beforeCriteria) == 0 {
		if len(afterCriteria) > 0 {
			// Unstructured original → structured rewrite: expansion always allowed.
			return true
		}
		normBefore := normalizeWhitespace(strings.TrimSpace(before))
		normAfter := normalizeWhitespace(strings.TrimSpace(after))
		if normBefore == "" {
			return true
		}
		return strings.Contains(normAfter, normBefore)
	}

	// Original had structured numbered criteria. Require each criterion body to
	// appear in at least one criterion body of the rewrite (either verbatim or
	// as a substring). The count check is a fast early-exit: if fewer criteria
	// are present in the rewrite there must be a drop.
	if len(afterCriteria) == 0 {
		// Rewrite lost all structure — fall back to full-text containment of
		// each criterion body inside the normalised rewrite text.
		normAfter := normalizeWhitespace(strings.TrimSpace(after))
		for _, criterion := range beforeCriteria {
			if !strings.Contains(normAfter, criterion) {
				return false
			}
		}
		return true
	}

	if len(afterCriteria) < len(beforeCriteria) {
		return false
	}

	for _, criterion := range beforeCriteria {
		found := false
		for _, candidate := range afterCriteria {
			if strings.Contains(candidate, criterion) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func parseAcceptanceCriteria(raw string) []string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if body, ok := parseAcceptanceCriterionBody(line); ok {
			out = append(out, body)
		}
	}
	return out
}

// parseAcceptanceCriterionBody extracts the verifiable assertion text from a
// single acceptance criterion line. It recognises common bullet-prefix
// conventions used in bead acceptance fields:
//
//   - Pure numeric:  "1. body"  "42. body"
//   - AC-prefix:     "AC1. body"  "AC2. body"  (letters then digits before ".")
//   - Dash/bullet:   "- body"  "• body"  "* body"
//
// All these forms are normalised so that a reformat (e.g. "AC1." → "1.") does
// not appear to have dropped a criterion when the assertion text is identical.
func parseAcceptanceCriterionBody(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", false
	}

	// Dash/star/bullet prefix: "- body", "* body", "+ body", "• body"
	// The bullet character (•, U+2022) is multi-byte in UTF-8, so we use
	// HasPrefix rather than a byte switch for that case.
	if len(trimmed) >= 2 {
		switch trimmed[0] {
		case '-', '*', '+':
			body := normalizeWhitespace(trimmed[1:])
			if body != "" {
				return body, true
			}
		}
	}
	if strings.HasPrefix(trimmed, "•") {
		body := normalizeWhitespace(strings.TrimPrefix(trimmed, "•"))
		if body != "" {
			return body, true
		}
	}

	dot := strings.Index(trimmed, ".")
	if dot <= 0 {
		return "", false
	}
	prefix := trimmed[:dot]

	// Pure numeric: "1.", "42."
	if allDigits(prefix) {
		body := normalizeWhitespace(strings.TrimSpace(trimmed[dot+1:]))
		if body != "" {
			return body, true
		}
		return "", false
	}

	// AC-prefix: letters immediately followed by digits before the dot.
	// Matches "AC1", "AC12", "A1", etc.
	if isAlphaNumericBullet(prefix) {
		body := normalizeWhitespace(strings.TrimSpace(trimmed[dot+1:]))
		if body != "" {
			return body, true
		}
		return "", false
	}

	return "", false
}

// isAlphaNumericBullet returns true when s looks like a labelled-list prefix
// of the form letters+digits (e.g. "AC1", "AC12", "A1"). At least one letter
// and one digit must be present; the digits must all trail the letters.
func isAlphaNumericBullet(s string) bool {
	if s == "" {
		return false
	}
	// Find the split point where letters end and digits begin.
	splitAt := -1
	for i, r := range s {
		isLetter := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
		isDigit := r >= '0' && r <= '9'
		if !isLetter && !isDigit {
			return false
		}
		if isDigit && splitAt == -1 {
			splitAt = i
		}
		if isLetter && splitAt != -1 {
			// letter after digit — not a clean ACN prefix
			return false
		}
	}
	// Must have at least one letter (splitAt > 0) and at least one digit.
	return splitAt > 0
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func hasString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
