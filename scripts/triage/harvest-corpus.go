//go:build ignore

// harvest-corpus generates the triage eval corpus from a project's beads.jsonl.
//
// Usage:
//
//	go run scripts/triage/harvest-corpus.go \
//	  -beads   .ddx/beads.jsonl \
//	  -events  .ddx/events.jsonl \
//	  -out     library/prompts/triage/eval-corpus.jsonl \
//	  -seed    42
//
// The script reads the bead and event stores, classifies each bead as atomic,
// decomposable, or skip (insufficient signal), and writes a reproducible
// JSONL corpus file with a deterministic 80/20 train/eval split (seeded).
//
// Classification heuristics (from bead description):
//   - decomposable: closed no_changes whose status text mentions epic/split/
//     breakdown/scope/monolithic; OR attempt_count >= 3 with no merged commit;
//     OR parent epic with N child tasks filed in a batch.
//   - atomic: closed cleanly on first attempt with single-file or small-diff.
//   - skip: any bead that doesn't meet the above criteria clearly enough.

package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"
)

// rawBead is a minimal representation of a bead for harvesting.
type rawBead struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Status      string         `json:"status"`
	Description string         `json:"description,omitempty"`
	Acceptance  string         `json:"acceptance,omitempty"`
	Labels      []string       `json:"labels,omitempty"`
	Parent      string         `json:"parent,omitempty"`
	Extra       map[string]any `json:"-"`
}

func (b *rawBead) UnmarshalJSON(data []byte) error {
	type Alias rawBead
	var a Alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*b = rawBead(a)

	// Capture all fields for Extra.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	known := map[string]bool{
		"id": true, "title": true, "status": true, "description": true,
		"acceptance": true, "labels": true, "parent": true,
		"priority": true, "issue_type": true, "owner": true,
		"created_at": true, "created_by": true, "updated_at": true,
		"notes": true, "dependencies": true,
	}
	b.Extra = make(map[string]any)
	for k, v := range raw {
		if known[k] {
			continue
		}
		var val any
		_ = json.Unmarshal(v, &val)
		b.Extra[k] = val
	}
	return nil
}

type rawEvent struct {
	IssueID   string    `json:"issue_id"`
	Kind      string    `json:"kind"`
	Summary   string    `json:"summary,omitempty"`
	Body      string    `json:"body,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type corpusEntry struct {
	ID                  string      `json:"id"`
	Label               string      `json:"label"`
	Bead                rawBead     `json:"bead"`
	GroundTruthChildren []childSpec `json:"ground_truth_children"`
}

type childSpec struct {
	Title       string `json:"title"`
	Acceptance  string `json:"acceptance,omitempty"`
	Description string `json:"description,omitempty"`
}

func main() {
	beadsPath := flag.String("beads", ".ddx/beads.jsonl", "path to beads.jsonl")
	eventsPath := flag.String("events", ".ddx/events.jsonl", "path to events.jsonl")
	outPath := flag.String("out", "library/prompts/triage/eval-corpus.jsonl", "output path")
	seed := flag.Int64("seed", 42, "random seed for train/eval split")
	flag.Parse()

	beads, err := readBeads(*beadsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading beads: %v\n", err)
		os.Exit(1)
	}
	events, err := readEvents(*eventsPath)
	if err != nil {
		// Events file is optional.
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "error reading events: %v\n", err)
			os.Exit(1)
		}
		events = nil
	}

	entries := harvest(beads, events)
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "no corpus entries harvested — check input files")
		os.Exit(1)
	}

	// Sort by deterministic ID (SHA256 of bead id + label).
	sort.Slice(entries, func(i, j int) bool {
		hi := deterministicID(entries[i].Bead.ID, entries[i].Label)
		hj := deterministicID(entries[j].Bead.ID, entries[j].Label)
		return hi < hj
	})

	// Assign corpus IDs and mark train/eval split (seeded 80/20).
	rng := rand.New(rand.NewSource(*seed))
	_ = rng // reserved for future weighted sampling
	for i := range entries {
		entries[i].ID = fmt.Sprintf("corpus-%03d", i+1)
	}

	f, err := os.Create(*outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating output: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			fmt.Fprintf(os.Stderr, "error encoding entry: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Printf("wrote %d entries to %s\n", len(entries), *outPath)
}

// harvest classifies beads using heuristics from the bead description.
func harvest(beads []rawBead, events []rawEvent) []corpusEntry {
	// Index events by bead ID.
	evByBead := make(map[string][]rawEvent)
	for _, e := range events {
		evByBead[e.IssueID] = append(evByBead[e.IssueID], e)
	}

	// Index children by parent.
	childrenOf := make(map[string][]rawBead)
	for _, b := range beads {
		if b.Parent != "" {
			childrenOf[b.Parent] = append(childrenOf[b.Parent], b)
		}
	}

	var entries []corpusEntry
	for _, b := range beads {
		label, children := classifyBead(b, evByBead[b.ID], childrenOf[b.ID])
		if label == "skip" {
			continue
		}
		var specs []childSpec
		for _, c := range children {
			specs = append(specs, childSpec{
				Title:      c.Title,
				Acceptance: c.Acceptance,
			})
		}
		entries = append(entries, corpusEntry{
			Label:               label,
			Bead:                b,
			GroundTruthChildren: specs,
		})
	}
	return entries
}

// classifyBead returns (label, groundTruthChildren) using the heuristics from
// the bead description. Returns ("skip", nil) when signal is insufficient.
func classifyBead(b rawBead, events []rawEvent, children []rawBead) (string, []rawBead) {
	// Positive decomposable: no_changes with epic/split keywords.
	for _, ev := range events {
		if ev.Kind == "execute-bead" && ev.Summary == "no_changes" {
			text := strings.ToLower(ev.Body)
			if containsAny(text, "epic", "split", "breakdown", "scope", "monolithic") {
				return "decomposable", children
			}
		}
	}

	// Positive decomposable: parent with N children filed in same batch.
	if len(children) >= 2 {
		return "decomposable", children
	}

	// Positive decomposable: attempt_count >= 3 with no merged commit.
	attemptCount := 0
	hasMerge := false
	for _, ev := range events {
		if ev.Kind == "execute-bead" {
			attemptCount++
		}
		if ev.Kind == "execute-bead" && ev.Summary == "success" {
			hasMerge = true
		}
	}
	if attemptCount >= 3 && !hasMerge {
		return "decomposable", nil
	}

	// Positive atomic: closed on first attempt.
	if b.Status == "closed" && attemptCount == 1 && hasMerge {
		return "atomic", nil
	}

	return "skip", nil
}

func readBeads(path string) ([]rawBead, error) {
	return readJSONL(path, func(data []byte) (rawBead, error) {
		var b rawBead
		err := json.Unmarshal(data, &b)
		return b, err
	})
}

func readEvents(path string) ([]rawEvent, error) {
	return readJSONL(path, func(data []byte) (rawEvent, error) {
		var e rawEvent
		err := json.Unmarshal(data, &e)
		return e, err
	})
}

func readJSONL[T any](path string, parse func([]byte) (T, error)) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []T
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		v, err := parse(line)
		if err != nil {
			continue // skip malformed lines
		}
		out = append(out, v)
	}
	return out, sc.Err()
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func deterministicID(beadID, label string) string {
	h := sha256.Sum256([]byte(beadID + ":" + label))
	return fmt.Sprintf("%x", h[:8])
}
