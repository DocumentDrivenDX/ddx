package cmd

import (
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestCLIFlagDesign_DefaultOnCapabilitiesAreDisableable walks the entire ddx
// command tree and enforces the project's CLI flag-design rules (see
// docs/helix/02-design/cli-flag-design-principles.md) so we never again ship a
// default-on capability that can't be turned off discoverably. The seed case was
// `ddx work` self-refresh: a positive --self-refresh flag that was secretly
// on-by-default in watch mode, leaving --self-refresh=false as the only escape.
//
//	RULE 1 — a boolean flag may DEFAULT TO true only if a discoverable opt-out
//	         (--no-<name> / --disable-<name>) exists on the same command. A
//	         default-on capability must be disableable without the unintuitive
//	         `--flag=false` form.
//	RULE 2 — a positive boolean flag (not itself a --no-/--disable-/--skip- opt-
//	         out) whose help text advertises default-on behavior MUST have a
//	         paired --no-<name> opt-out. This catches capabilities whose default-on
//	         lives in runtime logic rather than the flag default (self-refresh).
//
// To add a justified exception, add "<command>:<flag>" to allowedExceptions with
// a comment explaining why — but the strong default is: just add the --no-X flag.
func TestCLIFlagDesign_DefaultOnCapabilitiesAreDisableable(t *testing.T) {
	root := NewCommandFactory(".").NewRootCommand()
	if violations := flagDesignViolations(root); len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("CLI flag-design violations (%d) — every default-on capability needs a discoverable --no-X opt-out:\n  - %s",
			len(violations), strings.Join(violations, "\n  - "))
	}
}

// TestCLIFlagDesign_CheckCatchesViolations proves the linter is not vacuous: a
// command with a default-true bool (no opt-out) and a positive bool whose help
// advertises default-on (no opt-out) must both be flagged.
func TestCLIFlagDesign_CheckCatchesViolations(t *testing.T) {
	bad := &cobra.Command{Use: "bad"}
	bad.Flags().Bool("tsnet", true, "Enable the listener")            // RULE 1
	bad.Flags().Bool("self-refresh", false, "Re-exec (defaults on)")  // RULE 2

	got := flagDesignViolations(bad)
	if len(got) != 2 {
		t.Fatalf("want 2 violations (RULE 1 + RULE 2), got %d: %v", len(got), got)
	}

	// Adding the opt-outs must clear both.
	ok := &cobra.Command{Use: "ok"}
	ok.Flags().Bool("tsnet", true, "Enable the listener (on by default; use --no-tsnet)")
	ok.Flags().Bool("no-tsnet", false, "Disable the listener")
	ok.Flags().Bool("self-refresh", false, "Re-exec (on by default)")
	ok.Flags().Bool("no-self-refresh", false, "Disable self-refresh")
	if got := flagDesignViolations(ok); len(got) != 0 {
		t.Fatalf("opt-outs present, want 0 violations, got: %v", got)
	}
}

// flagDesignViolations returns every flag-design rule violation in the command
// tree rooted at root. Empty slice == compliant.
func flagDesignViolations(root *cobra.Command) []string {
	// Justified, documented exceptions. Keep SHORT; each needs a reason.
	allowedExceptions := map[string]string{}

	defaultOnPhrases := []string{"on by default", "defaults on", "default on", "enabled by default", "default: on"}
	optOutPrefixes := []string{"no-", "disable-", "skip-"}

	var violations []string
	walkCommands(root, func(c *cobra.Command) {
		fs := visibleFlags(c)

		// opt-out flag names present/visible on this command, by capability name.
		optOut := map[string]bool{}
		fs.VisitAll(func(f *pflag.Flag) {
			for _, p := range optOutPrefixes {
				if strings.HasPrefix(f.Name, p) {
					optOut[strings.TrimPrefix(f.Name, p)] = true
				}
			}
		})

		fs.VisitAll(func(f *pflag.Flag) {
			if f.Value.Type() != "bool" {
				return
			}
			if _, ok := allowedExceptions[c.Name()+":"+f.Name]; ok {
				return
			}
			isOptOut := false
			for _, p := range optOutPrefixes {
				if strings.HasPrefix(f.Name, p) {
					isOptOut = true
				}
			}
			path := c.CommandPath() + " --" + f.Name

			// RULE 1: default-true bool requires a --no-<name> opt-out.
			if f.DefValue == "true" && !optOut[f.Name] {
				violations = append(violations, path+
					" defaults to true but has no --no-"+f.Name+
					" opt-out (RULE 1: a default-on capability must be disableable without --flag=false)")
			}

			// RULE 2: positive bool advertising default-on requires a --no-<name>.
			if !isOptOut && f.DefValue != "true" {
				usage := strings.ToLower(f.Usage)
				for _, phrase := range defaultOnPhrases {
					if strings.Contains(usage, phrase) {
						if !optOut[f.Name] {
							violations = append(violations, path+
								" help advertises default-on ("+phrase+") but has no --no-"+f.Name+
								" opt-out (RULE 2: a runtime-default-on capability must expose a discoverable --no-X)")
						}
						break
					}
				}
			}
		})
	})

	return violations
}

// walkCommands invokes fn on c and every descendant command.
func walkCommands(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, child := range c.Commands() {
		walkCommands(child, fn)
	}
}

// visibleFlags returns the flags reachable from c: its own flags plus inherited
// persistent flags. Calling InheritedFlags() forces Cobra to merge parents'
// persistent flags into c.Flags().
func visibleFlags(c *cobra.Command) *pflag.FlagSet {
	_ = c.InheritedFlags()
	return c.Flags()
}
