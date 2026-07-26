package runrecord

import (
	"fmt"
	"strings"
)

// forbiddenInitialCorrelationKeys are DDx correlation map keys that encode
// concrete Fizeau routing, public results, cost, or provider-process data.
// The initial dispatching writer rejects them so a caller cannot smuggle
// pre-route knowledge through correlation before public Fizeau data exists.
var forbiddenInitialCorrelationKeys = map[string]struct{}{
	// Concrete routing decision.
	"harness":          {},
	"provider":         {},
	"model":            {},
	"route":            {},
	"route_reason":     {},
	"routing":          {},
	"routing_decision": {},
	"routing_policy":   {},
	// Immediate / final result claims.
	"result":             {},
	"immediate_result":   {},
	"final_result":       {},
	"immediate_error":    {},
	"final_status":       {},
	"final_exit_code":    {},
	"session_log_path":   {},
	"public_session_ref": {},
	"public_result_ref":  {},
	// Cost.
	"cost":     {},
	"cost_usd": {},
	// Provider PID / process metadata.
	"pid":                     {},
	"provider_pid":            {},
	"process_tree":            {},
	"provider_process_tree":   {},
	"children_pids":           {},
	"process_tree_metadata":   {},
	"session_canonical_state": {},
	"provider_session_state":  {},
	// Raw provider streams.
	"raw_output":          {},
	"provider_output":     {},
	"output_excerpt":      {},
	"stdout":              {},
	"stderr":              {},
	"raw_provider_output": {},
}

// PublishInitial is the initial run-record writer for the pre-Fizeau
// dispatching phase. It accepts only lifecycle phase "dispatching" and rejects
// any concrete harness, provider, model, route, immediate/final result, cost,
// provider PID/process metadata, or raw provider output claim — DDx must not
// persist knowledge that does not yet exist at this boundary.
//
// Valid records are written through the same crash-safe Publish path as later
// phase updates.
func PublishInitial(projectRoot string, rec Record) error {
	if err := validateInitialRecord(rec); err != nil {
		return err
	}
	return Publish(projectRoot, rec)
}

// validateInitialRecord enforces the pre-route field boundary for records in
// the dispatching phase (and for PublishInitial, which requires that phase).
func validateInitialRecord(rec Record) error {
	if rec.Phase != PhaseDispatching {
		return fmt.Errorf("runrecord: initial writer accepts only phase %q, got %q",
			PhaseDispatching, rec.Phase)
	}
	return validateNoPreRouteFields(rec)
}

// validateNoPreRouteFields rejects concrete routing, result, cost, and
// provider-process claims on a dispatching record.
func validateNoPreRouteFields(rec Record) error {
	if rec.Fizeau != nil && !rec.Fizeau.IsEmpty() {
		return fmt.Errorf("runrecord: initial dispatching record must not include Fizeau route/result fields before public route/result data exists")
	}
	// Non-nil empty Fizeau is normalized by callers; a non-nil pointer with
	// empty content is still a claim of a Fizeau block — clear is preferred,
	// but we only reject non-empty public data (IsEmpty already checked).
	if rec.Outcome != nil {
		return fmt.Errorf("runrecord: initial dispatching record must not include outcome/result fields before public route/result data exists")
	}
	if rec.FinishedAt != nil {
		return fmt.Errorf("runrecord: initial dispatching record must not set finished_at before public route/result data exists")
	}
	for k := range rec.Correlation {
		key := strings.ToLower(strings.TrimSpace(k))
		if _, bad := forbiddenInitialCorrelationKeys[key]; bad {
			return fmt.Errorf("runrecord: initial dispatching record must not include pre-route field %q in correlation", k)
		}
	}
	return nil
}
