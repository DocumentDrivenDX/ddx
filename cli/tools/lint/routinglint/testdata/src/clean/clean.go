// Package clean is the routinglint analyzer-test stub for the
// post-ddx-3bd7396a tree: no compensating-routing helpers and no
// retired flag/config-key strings. Running routinglint against this
// package must produce zero diagnostics.
package clean

// Test names are allowed to mention retired symbols as substrings —
// the analyzer matches identifier names exactly, not by substring.
func TestExecuteLoopLocalRejectsProfileLadders() {}

func TestLoadConfigHardErrorsOnProfileLadders() {}

// Strings that merely contain forbidden tokens as substrings are not
// flagged unless they equal the literal exactly.
const sentence = "the legacy profile_ladders_section concept was removed"

// Identifiers whose names happen to embed a retired symbol as a
// substring are not flagged.
type ProfileLaddersMigrationNote struct{ Reason string }

// Inline annotation exempts a deliberate rejection-path literal.
// routinglint:legacy-rejection reason="exempted rejection path in fixture"
const annotatedKey = "agent.routing.profile_ladders"

// Above-line annotation exempts the literal on the next line.
// routinglint:legacy-rejection reason="exempted rejection path in fixture"
const annotatedFlag = "--escalate"
