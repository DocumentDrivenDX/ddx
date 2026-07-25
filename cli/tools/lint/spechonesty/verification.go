// Package spechonesty hosts the Phase 2 spec-honesty analyzer.
// This file implements the read-only requirement-inventory and
// Verification-mapping parser (phase2-doc-truth-plan WB-1 step 3).
package spechonesty

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"
)

// InventoryKind identifies how DocumentModel.Inventory was produced.
type InventoryKind string

const (
	// InventoryRequirementIDs means Inventory holds declared REQ-* IDs.
	InventoryRequirementIDs InventoryKind = "requirement_id"
	// InventorySectionAnchors means Inventory holds stable section anchors
	// because the document declared no requirement IDs.
	InventorySectionAnchors InventoryKind = "section_anchor"
)

// ParseFindingKind classifies a parse-level Verification-row defect.
type ParseFindingKind string

const (
	// FindingMissingRequirementRef is emitted when a mapping row has no requirement/anchor.
	FindingMissingRequirementRef ParseFindingKind = "missing_requirement_ref"
	// FindingMissingEvidenceTarget is emitted when a mapping row has no evidence target.
	FindingMissingEvidenceTarget ParseFindingKind = "missing_evidence_target"
	// FindingMissingCommand is emitted when a mapping row has no verification command.
	FindingMissingCommand ParseFindingKind = "missing_command"
)

// ParseFinding is a parse-level issue (malformed mapping row).
// Coverage resolution is out of scope; these findings only describe
// structural defects so later stages can rely on well-formed rows.
type ParseFinding struct {
	Path    string
	Line    int
	Kind    ParseFindingKind
	Message string
}

// VerificationRow is one well-formed Verification mapping row.
type VerificationRow struct {
	// RequirementRef is the requirement ID or section anchor the row covers.
	RequirementRef string
	// EvidenceTarget is a Test* symbol, named static check, or runtime artifact path.
	EvidenceTarget string
	// Command is the executable verification command string.
	Command string
	// Line is the 1-based source line of the table row.
	Line int
}

// DocumentModel is the structured inventory + Verification mapping for one document.
type DocumentModel struct {
	Path          string
	Inventory     []string
	InventoryKind InventoryKind
	Rows          []VerificationRow
	Findings      []ParseFinding
}

var (
	// requirementIDRe matches normative requirement IDs such as REQ-001, REQ-12a.
	requirementIDRe = regexp.MustCompile(`\bREQ-[A-Za-z0-9][A-Za-z0-9._-]*\b`)
	// headingRe matches ATX headings (# through ######).
	headingRe = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)
	// tableRowRe matches a pipe-delimited markdown table row.
	tableRowRe = regexp.MustCompile(`^\s*\|(.+)\|\s*$`)
)

// ParseVerificationDocument reads path as UTF-8 markdown and returns the
// structured requirement inventory and Verification mapping model.
// It is read-only: the file is never modified.
func ParseVerificationDocument(path string) (*DocumentModel, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseVerificationMarkdown(path, string(data)), nil
}

// ParseVerificationMarkdown parses markdown content into a DocumentModel.
// path is recorded on the model and findings for diagnostics; content is not written back.
func ParseVerificationMarkdown(path, content string) *DocumentModel {
	model := &DocumentModel{
		Path:      path,
		Inventory: nil,
		Rows:      nil,
		Findings:  nil,
	}

	// Strip YAML frontmatter so status/id keys do not pollute inventory scans.
	body, lineOffset := stripFrontmatter(content)
	lines := splitLines(body)

	reqIDs := collectRequirementIDs(lines)
	if len(reqIDs) > 0 {
		model.Inventory = reqIDs
		model.InventoryKind = InventoryRequirementIDs
	} else {
		model.Inventory = collectSectionAnchors(lines)
		model.InventoryKind = InventorySectionAnchors
	}

	rows, findings := parseVerificationTable(path, lines, lineOffset)
	model.Rows = rows
	model.Findings = findings
	return model
}

// stripFrontmatter returns the body after a leading YAML frontmatter block
// and the number of lines consumed before the body (for 1-based line mapping).
func stripFrontmatter(content string) (body string, lineOffset int) {
	// Only treat leading --- ... --- as frontmatter.
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return content, 0
	}
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return content, 0
	}
	rest := normalized[len("---\n"):]
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		// Unterminated frontmatter: keep original content.
		return content, 0
	}
	fmBlock := rest[:idx]
	// Opening ---, frontmatter lines, closing ---.
	lineOffset = 1 + countNewlines(fmBlock) + 1 + 1
	return rest[idx+len("\n---\n"):], lineOffset
}

func countNewlines(s string) int {
	return strings.Count(s, "\n")
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// collectRequirementIDs returns unique REQ-* IDs in document order.
func collectRequirementIDs(lines []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, line := range lines {
		// Skip fenced code blocks' content? Keep simple: scan all lines.
		// REQ- IDs inside Verification table cells are inventory only if
		// they also appear as declarations; still collecting from whole
		// body is correct for fixtures that declare IDs in Requirements.
		for _, m := range requirementIDRe.FindAllString(line, -1) {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	return out
}

// collectSectionAnchors returns GitHub-style anchors for ATX headings.
// H1 document titles are omitted; the Verification section is omitted
// because it is the mapping container, not a normative requirement.
func collectSectionAnchors(lines []string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, line := range lines {
		m := headingRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		level := len(m[1])
		title := strings.TrimSpace(m[2])
		// Strip trailing hashes from closed ATX (## Title ##).
		title = strings.TrimRight(title, " #")
		title = strings.TrimSpace(title)
		if title == "" {
			continue
		}
		if level == 1 {
			// Document title is not a stable section inventory entry.
			continue
		}
		if strings.EqualFold(title, "Verification") ||
			strings.EqualFold(title, "Verification Mapping") {
			continue
		}
		anchor := slugifyAnchor(title)
		if anchor == "" || seen[anchor] {
			continue
		}
		seen[anchor] = true
		out = append(out, anchor)
	}
	return out
}

// slugifyAnchor produces a GitHub-like heading anchor.
func slugifyAnchor(title string) string {
	// Strip markdown emphasis/code markers for stable anchors.
	title = strings.ReplaceAll(title, "`", "")
	title = strings.ReplaceAll(title, "*", "")
	title = strings.ReplaceAll(title, "_", " ")
	var b strings.Builder
	prevHyphen := false
	for _, r := range strings.ToLower(title) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevHyphen = false
		case r == ' ' || r == '-' || r == '/':
			if b.Len() > 0 && !prevHyphen {
				b.WriteByte('-')
				prevHyphen = true
			}
		default:
			// Drop other punctuation.
		}
	}
	return strings.Trim(b.String(), "-")
}

type tableColumns struct {
	reqIdx      int
	evidenceIdx int
	commandIdx  int
}

func parseVerificationTable(path string, lines []string, lineOffset int) ([]VerificationRow, []ParseFinding) {
	start := findVerificationSection(lines)
	if start < 0 {
		return nil, nil
	}

	// Scan from the section body for the first markdown table.
	headerIdx := -1
	var cols tableColumns
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		// Stop at next same-or-higher-level heading? Any ## heading ends section.
		if m := headingRe.FindStringSubmatch(line); m != nil && len(m[1]) <= 2 {
			// Another ## section starts.
			if i > start+1 {
				break
			}
		}
		if !tableRowRe.MatchString(line) {
			continue
		}
		cells := splitTableCells(line)
		if isSeparatorRow(cells) {
			continue
		}
		// First non-separator table row is the header.
		headerIdx = i
		cols = mapTableColumns(cells)
		break
	}
	if headerIdx < 0 || cols.reqIdx < 0 || cols.evidenceIdx < 0 || cols.commandIdx < 0 {
		return nil, nil
	}

	var rows []VerificationRow
	var findings []ParseFinding

	for i := headerIdx + 1; i < len(lines); i++ {
		line := lines[i]
		if m := headingRe.FindStringSubmatch(line); m != nil {
			break
		}
		if !tableRowRe.MatchString(line) {
			// Blank or prose ends the table.
			if strings.TrimSpace(line) == "" {
				// Allow blank lines inside the table region.
				continue
			}
			// Non-table content after table start ends parsing.
			if len(rows) > 0 || len(findings) > 0 {
				break
			}
			continue
		}
		cells := splitTableCells(line)
		if isSeparatorRow(cells) {
			continue
		}

		// 1-based line in the original file (frontmatter offset + body index).
		lineNo := lineOffset + i + 1
		req := cellAt(cells, cols.reqIdx)
		evidence := cellAt(cells, cols.evidenceIdx)
		command := cellAt(cells, cols.commandIdx)

		missing := false
		if req == "" {
			missing = true
			findings = append(findings, ParseFinding{
				Path:    path,
				Line:    lineNo,
				Kind:    FindingMissingRequirementRef,
				Message: fmt.Sprintf("verification mapping row missing requirement ref at line %d", lineNo),
			})
		}
		if evidence == "" {
			missing = true
			findings = append(findings, ParseFinding{
				Path:    path,
				Line:    lineNo,
				Kind:    FindingMissingEvidenceTarget,
				Message: fmt.Sprintf("verification mapping row missing evidence target at line %d", lineNo),
			})
		}
		if command == "" {
			missing = true
			findings = append(findings, ParseFinding{
				Path:    path,
				Line:    lineNo,
				Kind:    FindingMissingCommand,
				Message: fmt.Sprintf("verification mapping row missing command at line %d", lineNo),
			})
		}
		if missing {
			continue
		}
		rows = append(rows, VerificationRow{
			RequirementRef: req,
			EvidenceTarget: evidence,
			Command:        command,
			Line:           lineNo,
		})
	}
	return rows, findings
}

func findVerificationSection(lines []string) int {
	for i, line := range lines {
		m := headingRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		title := strings.TrimSpace(m[2])
		title = strings.TrimRight(title, " #")
		title = strings.TrimSpace(title)
		if strings.EqualFold(title, "Verification") ||
			strings.EqualFold(title, "Verification Mapping") {
			return i
		}
	}
	return -1
}

func splitTableCells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(p)
	}
	return out
}

func isSeparatorRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		if c == "" {
			continue
		}
		for _, r := range c {
			if r != '-' && r != ':' && r != ' ' {
				return false
			}
		}
	}
	// At least one cell must look like a separator fragment.
	for _, c := range cells {
		if strings.Contains(c, "-") {
			return true
		}
	}
	return false
}

func mapTableColumns(header []string) tableColumns {
	cols := tableColumns{reqIdx: -1, evidenceIdx: -1, commandIdx: -1}
	for i, h := range header {
		key := normalizeHeader(h)
		switch {
		case cols.reqIdx < 0 && isRequirementHeader(key):
			cols.reqIdx = i
		case cols.evidenceIdx < 0 && isEvidenceHeader(key):
			cols.evidenceIdx = i
		case cols.commandIdx < 0 && isCommandHeader(key):
			cols.commandIdx = i
		}
	}
	return cols
}

func normalizeHeader(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	h = strings.ReplaceAll(h, "`", "")
	h = strings.ReplaceAll(h, "*", "")
	// Collapse whitespace and punctuation to single spaces for matching.
	var b strings.Builder
	prevSpace := false
	for _, r := range h {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
			continue
		}
		if !prevSpace {
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func isRequirementHeader(key string) bool {
	switch key {
	case "requirement", "requirements", "req", "anchor", "anchors",
		"requirement ref", "requirement anchor", "requirement id",
		"req anchor", "id":
		return true
	}
	if strings.Contains(key, "requirement") {
		return true
	}
	if strings.Contains(key, "anchor") && !strings.Contains(key, "evidence") {
		return true
	}
	return false
}

func isEvidenceHeader(key string) bool {
	switch key {
	case "evidence", "evidence target", "target", "test", "check",
		"evidence target string", "proof":
		return true
	}
	if strings.Contains(key, "evidence") {
		return true
	}
	if key == "target" || strings.HasSuffix(key, " target") {
		return true
	}
	return false
}

func isCommandHeader(key string) bool {
	switch key {
	case "command", "cmd", "verification command", "verify",
		"executable command", "verification":
		return true
	}
	return strings.Contains(key, "command")
}

func cellAt(cells []string, idx int) string {
	if idx < 0 || idx >= len(cells) {
		return ""
	}
	return strings.TrimSpace(cells[idx])
}
