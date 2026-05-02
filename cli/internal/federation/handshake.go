package federation

import (
	"fmt"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/update"
)

// HandshakeResult is the outcome of validating a spoke's version handshake
// against the hub's own version + schema version.
type HandshakeResult struct {
	// Accept reports whether the registration should be accepted at all.
	Accept bool
	// Status is the SpokeStatus to record on accepted registrations:
	// StatusRegistered when fully compatible (heartbeat will move it to
	// StatusActive), or StatusDegraded when accepted-but-newer.
	Status SpokeStatus
	// Reason is a short, machine-readable code for the outcome — used as the
	// rejection reason on 4xx and as a degraded-status reason on accept.
	// Stable enum: "ok", "degraded_newer_minor",
	// "schema_mismatch", "ddx_major_mismatch", "missing_ddx_version",
	// "missing_schema_version", "invalid_ddx_version".
	Reason string
}

// Handshake validates a spoke's version handshake.
//
//   - schema_version must match the hub's schema version exactly. Anything
//     else is rejected ("schema_mismatch").
//   - ddx_version must parse as semver. Major version must match the hub.
//     A higher minor or patch on the spoke side accepts as StatusDegraded
//     ("degraded_newer_minor"). Equal-or-lower minors/patches accept as
//     StatusRegistered ("ok"). Mismatched majors are rejected
//     ("ddx_major_mismatch").
//
// Empty / unparseable inputs are rejected with descriptive reasons.
func Handshake(hubDDxVersion, hubSchemaVersion, spokeDDxVersion, spokeSchemaVersion string) HandshakeResult {
	if strings.TrimSpace(spokeSchemaVersion) == "" {
		return HandshakeResult{Reason: "missing_schema_version"}
	}
	if spokeSchemaVersion != hubSchemaVersion {
		return HandshakeResult{Reason: "schema_mismatch"}
	}
	if strings.TrimSpace(spokeDDxVersion) == "" {
		return HandshakeResult{Reason: "missing_ddx_version"}
	}
	hubParts, err := update.ParseVersion(hubDDxVersion)
	if err != nil {
		return HandshakeResult{Reason: fmt.Sprintf("invalid_hub_ddx_version:%v", err)}
	}
	spokeParts, err := update.ParseVersion(spokeDDxVersion)
	if err != nil {
		return HandshakeResult{Reason: "invalid_ddx_version"}
	}
	if spokeParts[0] != hubParts[0] {
		return HandshakeResult{Reason: "ddx_major_mismatch"}
	}
	// Same major, compare minor.patch. Any newer minor or patch on the spoke
	// side is "compatible-but-newer" → degraded.
	newer := spokeParts[1] > hubParts[1] ||
		(spokeParts[1] == hubParts[1] && spokeParts[2] > hubParts[2])
	if newer {
		return HandshakeResult{Accept: true, Status: StatusDegraded, Reason: "degraded_newer_minor"}
	}
	return HandshakeResult{Accept: true, Status: StatusRegistered, Reason: "ok"}
}
