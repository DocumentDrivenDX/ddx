package agent

import (
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

	original, err := store.Get(beadID)
	if err != nil {
		return fmt.Errorf("pre-claim intake rewrite: load bead %s: %w", beadID, err)
	}

	rewrite, before, after, preservation, err := validateAndApplyPreClaimIntakeRewrite(original, intake.Rewrite)
	if err != nil {
		return err
	}

	if err := store.Update(beadID, func(b *bead.Bead) {
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
		Source:    "ddx agent execute-loop",
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

func acceptancePreservesCriteria(before, after string) bool {
	beforeCriteria := parseAcceptanceCriteria(before)
	afterCriteria := parseAcceptanceCriteria(after)
	if len(beforeCriteria) == 0 {
		beforeCriteria = []string{normalizeWhitespace(strings.TrimSpace(before))}
	}
	if len(afterCriteria) == 0 {
		afterCriteria = []string{normalizeWhitespace(strings.TrimSpace(after))}
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

func parseAcceptanceCriterionBody(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", false
	}
	dot := strings.Index(trimmed, ".")
	if dot <= 0 {
		return "", false
	}
	if !allDigits(trimmed[:dot]) {
		return "", false
	}
	body := normalizeWhitespace(strings.TrimSpace(trimmed[dot+1:]))
	if body == "" {
		return "", false
	}
	return body, true
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
