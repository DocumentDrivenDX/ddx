//go:build ignore

// harvest-corpus generates the held-out triage eval corpus from bead history.
//
// Usage:
//
//	go run ./scripts/triage/harvest-corpus.go \
//	  --output library/prompts/triage/eval-corpus.jsonl
//
// The script reads .ddx/beads.jsonl from this repository and, when present,
// ../agent/.ddx/beads.jsonl. It never invokes an agent and never performs
// model/provider/harness routing; it only prepares deterministic offline data.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type rawBead struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Status       string         `json:"status,omitempty"`
	IssueType    string         `json:"issue_type,omitempty"`
	Description  string         `json:"description,omitempty"`
	Acceptance   string         `json:"acceptance,omitempty"`
	Labels       []string       `json:"labels,omitempty"`
	Parent       string         `json:"parent,omitempty"`
	Notes        string         `json:"notes,omitempty"`
	Dependencies []rawDep       `json:"dependencies,omitempty"`
	Events       []rawEvent     `json:"events,omitempty"`
	Extra        map[string]any `json:"-"`
}

type rawDep struct {
	DependsOnID string `json:"depends_on_id"`
}

type rawEvent struct {
	Kind    string `json:"kind"`
	Summary string `json:"summary,omitempty"`
	Body    string `json:"body,omitempty"`
}

func (b *rawBead) UnmarshalJSON(data []byte) error {
	type alias rawBead
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*b = rawBead(decoded)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	known := map[string]bool{
		"acceptance": true, "created_at": true, "dependencies": true,
		"description": true, "events": true, "id": true, "issue_type": true,
		"labels": true, "notes": true, "owner": true, "parent": true,
		"priority": true, "status": true, "title": true, "updated_at": true,
	}
	b.Extra = make(map[string]any)
	for key, val := range raw {
		if known[key] {
			continue
		}
		var decoded any
		if err := json.Unmarshal(val, &decoded); err == nil {
			b.Extra[key] = decoded
		}
	}
	return nil
}

type sourcedBead struct {
	SourceRepo string
	Bead       rawBead
}

type corpusEntry struct {
	ID              string   `json:"id"`
	SourceRepo      string   `json:"source_repo"`
	ExpectedClass   string   `json:"expected_class"`
	Rationale       string   `json:"rationale"`
	Title           string   `json:"title"`
	Description     string   `json:"description,omitempty"`
	Acceptance      string   `json:"acceptance,omitempty"`
	Labels          []string `json:"labels,omitempty"`
	Bead            evalBead `json:"bead"`
	ChildTitles     []string `json:"child_titles,omitempty"`
	ChildAcceptance []string `json:"child_acceptance,omitempty"`
}

type evalBead struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Status      string   `json:"status,omitempty"`
	IssueType   string   `json:"issue_type,omitempty"`
	Description string   `json:"description,omitempty"`
	Acceptance  string   `json:"acceptance,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Parent      string   `json:"parent,omitempty"`
}

func main() {
	var output string
	var legacyOut string
	flag.StringVar(&output, "output", "library/prompts/triage/eval-corpus.jsonl", "output JSONL path")
	flag.StringVar(&legacyOut, "out", "", "deprecated alias for --output")
	flag.Parse()
	if legacyOut != "" {
		output = legacyOut
	}

	inputs := []string{".ddx/beads.jsonl"}
	if _, err := os.Stat("../agent/.ddx/beads.jsonl"); err == nil {
		inputs = append(inputs, "../agent/.ddx/beads.jsonl")
	} else {
		fmt.Fprintln(os.Stderr, "warning: ../agent/.ddx/beads.jsonl not found; harvesting local repo only")
	}

	var all []sourcedBead
	for _, input := range inputs {
		beads, err := readBeads(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading %s: %v\n", input, err)
			os.Exit(1)
		}
		source := filepath.Base(filepath.Dir(filepath.Dir(input)))
		if input == ".ddx/beads.jsonl" {
			source = "ddx"
		}
		for _, b := range beads {
			all = append(all, sourcedBead{SourceRepo: source, Bead: b})
		}
	}

	entries := harvest(all)
	eval := holdoutByClass(entries)
	if err := validateClasses(eval); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if err := writeJSONL(output, eval); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", output, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d eval entries to %s\n", len(eval), output)
}

func harvest(beads []sourcedBead) []corpusEntry {
	children := make(map[string][]rawBead)
	for _, item := range beads {
		if item.Bead.Parent != "" {
			key := item.SourceRepo + "\x00" + item.Bead.Parent
			children[key] = append(children[key], item.Bead)
		}
	}

	var out []corpusEntry
	for _, item := range beads {
		key := item.SourceRepo + "\x00" + item.Bead.ID
		class, rationale := classify(item.Bead, children[key])
		if class == "" {
			continue
		}
		entry := corpusEntry{
			SourceRepo:    item.SourceRepo,
			ExpectedClass: class,
			Rationale:     rationale,
			Title:         scrubRouteWords(item.Bead.Title),
			Description:   scrubRouteWords(item.Bead.Description),
			Acceptance:    scrubRouteWords(item.Bead.Acceptance),
			Labels:        scrubLabels(item.Bead.Labels),
			Bead:          toEvalBead(item.Bead),
		}
		for _, child := range children[key] {
			entry.ChildTitles = append(entry.ChildTitles, scrubRouteWords(child.Title))
			entry.ChildAcceptance = append(entry.ChildAcceptance, scrubRouteWords(child.Acceptance))
		}
		entry.ID = stableEntryID(entry)
		out = append(out, entry)
	}
	sortEntries(out)
	return out
}

func toEvalBead(b rawBead) evalBead {
	return evalBead{
		ID:          scrubRouteWords(b.ID),
		Title:       scrubRouteWords(b.Title),
		Status:      b.Status,
		IssueType:   b.IssueType,
		Description: scrubRouteWords(b.Description),
		Acceptance:  scrubRouteWords(b.Acceptance),
		Labels:      scrubLabels(b.Labels),
		Parent:      scrubRouteWords(b.Parent),
	}
}

func classify(b rawBead, children []rawBead) (string, string) {
	search := strings.ToLower(strings.Join([]string{
		b.Title,
		b.Description,
		b.Acceptance,
		b.Notes,
		eventsText(b.Events),
	}, "\n"))

	if len(children) >= 2 {
		return "decomposable", "parent bead has two or more child beads"
	}
	if containsAny(search, "no_changes", "no changes") &&
		containsAny(search, "epic", "split", "breakdown", "scope", "monolithic") {
		return "decomposable", "no_changes/review text says bead needs splitting"
	}
	if countReviewBlocks(b.Events, search) >= 2 &&
		containsAny(search, "scope", "split", "epic", "breakdown", "monolithic") {
		return "decomposable", "repeated reviewer blocks cite decomposition scope"
	}
	if attemptCount(b.Events) >= 3 && !hasClosingCommit(b) {
		return "decomposable", "three or more attempts without a closing commit"
	}

	if b.Status == "closed" && hasClosingCommit(b) && !containsAny(search, "epic", "split", "breakdown", "monolithic") {
		return "atomic", "closed with commit evidence and no decomposition signal"
	}
	if b.Status == "closed" && isSmallFocused(b) {
		return "atomic", "closed focused bead with small acceptance surface"
	}
	return "", ""
}

func holdoutByClass(entries []corpusEntry) []corpusEntry {
	byClass := make(map[string][]corpusEntry)
	for _, entry := range entries {
		byClass[entry.ExpectedClass] = append(byClass[entry.ExpectedClass], entry)
	}
	var out []corpusEntry
	for _, class := range []string{"atomic", "decomposable"} {
		classEntries := byClass[class]
		sortEntries(classEntries)
		if class == "decomposable" {
			classEntries = coverageFriendlySplits(classEntries)
		}
		if len(classEntries) == 0 {
			continue
		}
		target := len(classEntries) / 5
		if target == 0 {
			target = 1
		}
		for i := 0; i < len(classEntries) && len(selectedClass(out, class)) < target; i++ {
			if i%5 == 0 {
				out = append(out, classEntries[i])
			}
		}
	}
	sortEntries(out)
	return out
}

func coverageFriendlySplits(entries []corpusEntry) []corpusEntry {
	var preferred []corpusEntry
	for _, entry := range entries {
		if len(entry.ChildAcceptance) == 0 || acCoverage(entry.Acceptance, entry.ChildAcceptance) >= 0.90 {
			preferred = append(preferred, entry)
		}
	}
	if len(preferred) == 0 {
		return entries
	}
	sortEntries(preferred)
	return preferred
}

func selectedClass(entries []corpusEntry, class string) []corpusEntry {
	var out []corpusEntry
	for _, entry := range entries {
		if entry.ExpectedClass == class {
			out = append(out, entry)
		}
	}
	return out
}

func validateClasses(entries []corpusEntry) error {
	seen := map[string]bool{}
	for _, entry := range entries {
		seen[entry.ExpectedClass] = true
	}
	if !seen["atomic"] {
		return errors.New("eval corpus has no expected_class=atomic entries")
	}
	if !seen["decomposable"] {
		return errors.New("eval corpus has no expected_class=decomposable entries")
	}
	return nil
}

func readBeads(path string) ([]rawBead, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []rawBead
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var b rawBead
		if err := json.Unmarshal([]byte(line), &b); err != nil {
			return nil, fmt.Errorf("parse bead line %d: %w", len(out)+1, err)
		}
		out = append(out, b)
	}
	return out, sc.Err()
}

func writeJSONL(path string, entries []corpusEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			return err
		}
	}
	return nil
}

func sortEntries(entries []corpusEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ExpectedClass != entries[j].ExpectedClass {
			return entries[i].ExpectedClass < entries[j].ExpectedClass
		}
		return entries[i].ID < entries[j].ID
	})
}

func stableEntryID(entry corpusEntry) string {
	sum := sha256.Sum256([]byte(entry.SourceRepo + "\x00" + entry.ExpectedClass + "\x00" + entry.Bead.ID))
	return fmt.Sprintf("corpus-%x", sum[:6])
}

func eventsText(events []rawEvent) string {
	var parts []string
	for _, ev := range events {
		parts = append(parts, ev.Kind, ev.Summary, ev.Body)
	}
	return strings.Join(parts, "\n")
}

func countReviewBlocks(events []rawEvent, text string) int {
	count := strings.Count(text, "review:block") + strings.Count(text, "review_block")
	for _, ev := range events {
		if strings.EqualFold(ev.Summary, "BLOCK") || strings.EqualFold(ev.Summary, "review_block") {
			count++
		}
	}
	return count
}

func attemptCount(events []rawEvent) int {
	count := 0
	for _, ev := range events {
		if ev.Kind == "execute-bead" || ev.Kind == "cost" {
			count++
		}
	}
	return count
}

func hasClosingCommit(b rawBead) bool {
	v, ok := b.Extra["closing_commit_sha"]
	return ok && strings.TrimSpace(fmt.Sprint(v)) != ""
}

func isSmallFocused(b rawBead) bool {
	if b.IssueType == "epic" {
		return false
	}
	lines := 0
	for _, line := range strings.Split(b.Acceptance, "\n") {
		if strings.TrimSpace(line) != "" {
			lines++
		}
	}
	return lines > 0 && lines <= 3 && len(b.Labels) <= 5
}

func acCoverage(parent string, children []string) float64 {
	parentTokens := tokenSet(parent)
	if len(parentTokens) == 0 {
		return 1
	}
	var combined strings.Builder
	for _, child := range children {
		combined.WriteString(" ")
		combined.WriteString(child)
	}
	childTokens := tokenSet(combined.String())
	covered := 0
	for token := range parentTokens {
		if childTokens[token] {
			covered++
		}
	}
	return float64(covered) / float64(len(parentTokens))
}

func tokenSet(s string) map[string]bool {
	tokens := map[string]bool{}
	var cur strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
			continue
		}
		if cur.Len() >= 3 {
			tokens[cur.String()] = true
		}
		cur.Reset()
	}
	if cur.Len() >= 3 {
		tokens[cur.String()] = true
	}
	return tokens
}

func scrubLabels(labels []string) []string {
	out := make([]string, len(labels))
	for i, label := range labels {
		out[i] = scrubRouteWords(label)
	}
	return out
}

func scrubRouteWords(s string) string {
	replacements := []struct {
		old string
		new string
	}{
		{"provider", "upstream-service"},
		{"Provider", "Upstream-service"},
		{"PROVIDER", "UPSTREAM-SERVICE"},
		{"model", "route-target"},
		{"Model", "Route-target"},
		{"MODEL", "ROUTE-TARGET"},
		{"harness", "runner"},
		{"Harness", "Runner"},
		{"HARNESS", "RUNNER"},
	}
	for _, replacement := range replacements {
		s = strings.ReplaceAll(s, replacement.old, replacement.new)
	}
	return s
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
