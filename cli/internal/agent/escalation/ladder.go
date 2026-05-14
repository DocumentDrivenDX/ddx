// Package escalation implements a MinPower-only escalation ladder used by
// the execute-bead loop to pick the next power floor after a substantive
// failure. The ladder reasons about Power values only — it never inspects
// provider names, model IDs, harness names, or any other vendor-identifying
// attributes. This keeps escalation portable across catalogs that disagree
// on which vendor sits at which powerClass.
package escalation

import (
	"errors"
	"fmt"
	"sort"

	agentlib "github.com/easel/fizeau"
)

// ErrLadderExhausted is returned by Ladder.Next when no power powerClass above
// the supplied actualPower exists in the catalog. Power escalation cannot
// proceed any further at this catalog.
var ErrLadderExhausted = errors.New("escalation: ladder exhausted; no higher power powerClass in catalog")

// NoViableProviderError signals that the next catalog powerClass above the
// caller's actualPower has no viable provider — i.e., every model at that
// powerClass is either unavailable or not auto-routable. The loop bumps further
// by calling Next(Floor) to skip past this dead powerClass.
type NoViableProviderError struct {
	// Floor is the catalog power powerClass that was skipped because it had no
	// viable provider. Pass this value back into Next() to bump further.
	Floor int
}

func (e *NoViableProviderError) Error() string {
	return fmt.Sprintf("escalation: no viable provider at floor %d", e.Floor)
}

// Ladder is a MinPower-only escalation ladder. It enumerates the distinct
// power powerClasses present in a fizeau model listing, sorted ascending, and tracks
// which powerClasses have at least one viable model.
type Ladder struct {
	powerClasses []int
	viableSet    map[int]struct{}
}

// NewLadder constructs a Ladder from fizeau model metadata. Models with Power<=0
// are ignored as unrated. A model contributes to the viable set when it is
// both Available and AutoRoutable; non-viable powerClasses remain present in the
// ladder so Next can report a typed NoViableProviderError when one is hit.
func NewLadder(models []agentlib.ModelInfo) *Ladder {
	seen := map[int]struct{}{}
	viable := map[int]struct{}{}
	for _, m := range models {
		if m.Power <= 0 {
			continue
		}
		seen[m.Power] = struct{}{}
		if m.Available && m.AutoRoutable {
			viable[m.Power] = struct{}{}
		}
	}
	powerClasses := make([]int, 0, len(seen))
	for p := range seen {
		powerClasses = append(powerClasses, p)
	}
	sort.Ints(powerClasses)
	return &Ladder{powerClasses: powerClasses, viableSet: viable}
}

// PowerClasses returns the distinct catalog power powerClasses, ascending. The returned
// slice is a copy.
func (l *Ladder) PowerClasses() []int {
	if l == nil {
		return nil
	}
	out := make([]int, len(l.powerClasses))
	copy(out, l.powerClasses)
	return out
}

// Next returns the next power floor strictly greater than actualPower.
//
// actualPower is sourced from the previous attempt's routing-actual record
// (RoutingActual.Power). The escalation loop calls Next(actualPower) after
// a substantive failure to obtain the floor for the next attempt.
//
// Three outcomes:
//
//   - The next catalog powerClass above actualPower has at least one viable
//     model: returns (floor, nil).
//   - The next catalog powerClass above actualPower has no viable provider:
//     returns (0, *NoViableProviderError{Floor: powerClass}). The loop bumps
//     further by calling Next(powerClass).
//   - No catalog powerClass above actualPower exists: returns (0, ErrLadderExhausted).
func (l *Ladder) Next(actualPower int) (int, error) {
	if l == nil || len(l.powerClasses) == 0 {
		return 0, ErrLadderExhausted
	}
	for _, t := range l.powerClasses {
		if t <= actualPower {
			continue
		}
		if _, ok := l.viableSet[t]; !ok {
			return 0, &NoViableProviderError{Floor: t}
		}
		return t, nil
	}
	return 0, ErrLadderExhausted
}
