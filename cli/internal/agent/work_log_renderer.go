package agent

import (
	"fmt"
	"strings"
	"time"

	agentlib "github.com/DocumentDrivenDX/fizeau"
)

type WorkLogRendererOptions struct {
	Now           func() time.Time
	CurrentBeadID string
}

type WorkLogRenderer struct {
	now           func() time.Time
	currentBeadID string
}

type WorkLogLifecycleLine struct {
	Phase       string
	Message     string
	BeadID      string
	Harness     string
	Provider    string
	Model       string
	RouteReason string
}

func NewWorkLogRenderer(opts WorkLogRendererOptions) WorkLogRenderer {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return WorkLogRenderer{
		now:           now,
		currentBeadID: strings.TrimSpace(opts.CurrentBeadID),
	}
}

func (r WorkLogRenderer) WithCurrentBeadID(beadID string) WorkLogRenderer {
	r.currentBeadID = strings.TrimSpace(beadID)
	return r
}

func (r WorkLogRenderer) at(ts time.Time) WorkLogRenderer {
	if !ts.IsZero() {
		r.now = func() time.Time { return ts }
	}
	return r
}

func (r WorkLogRenderer) FormatHeader(beadID, title string) string {
	beadID = strings.TrimSpace(beadID)
	if strings.TrimSpace(title) == "" {
		return fmt.Sprintf("\n▶ %s\n", beadID)
	}
	return fmt.Sprintf("\n▶ %s: %s\n", beadID, title)
}

func (r WorkLogRenderer) FormatLifecycleLine(line WorkLogLifecycleLine) string {
	parts := []string{r.timestamp()}
	if phase := strings.TrimSpace(line.Phase); phase != "" {
		parts = append(parts, phase)
	}
	if beadID := r.visibleBeadID(line.BeadID); beadID != "" {
		parts = append(parts, beadID)
	}
	if message := strings.TrimSpace(line.Message); message != "" {
		parts = append(parts, message)
	}
	if route := formatWorkLogRouteFields(line.Harness, line.Provider, line.Model, line.RouteReason); route != "" {
		parts = append(parts, route)
	}
	return strings.Join(parts, " ") + "\n"
}

func (r WorkLogRenderer) FormatRoutingDecision(routing *agentlib.ServiceRoutingDecisionData) string {
	line := strings.TrimSpace(FormatServiceRoutingDecision(routing))
	if line == "" {
		return ""
	}
	return r.timestamp() + " " + line + "\n"
}

func (r WorkLogRenderer) FormatServiceProgressEntries(entries []agentlib.ServiceProgressData) string {
	return r.prefixRenderedLines(FormatServiceProgressEntries(entries))
}

func (r WorkLogRenderer) prefixRenderedLines(rendered string) string {
	var sb strings.Builder
	for _, raw := range strings.Split(rendered, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		line = r.suppressCurrentBeadID(line)
		sb.WriteString(r.timestamp())
		sb.WriteString(" ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return sb.String()
}

func (r WorkLogRenderer) visibleBeadID(beadID string) string {
	beadID = strings.TrimSpace(beadID)
	if beadID == "" || beadID == r.currentBeadID {
		return ""
	}
	return beadID
}

func (r WorkLogRenderer) suppressCurrentBeadID(line string) string {
	if r.currentBeadID == "" {
		return line
	}
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[1] != r.currentBeadID {
		return line
	}
	fields = append(fields[:1], fields[2:]...)
	return strings.Join(fields, " ")
}

func (r WorkLogRenderer) timestamp() string {
	now := r.now
	if now == nil {
		now = time.Now
	}
	return now().Format("15:04:05")
}

func formatWorkLogRouteFields(harness, provider, model, reason string) string {
	parts := []string{}
	if harness = strings.TrimSpace(harness); harness != "" {
		parts = append(parts, "harness="+harness)
	}
	if provider = strings.TrimSpace(provider); provider != "" {
		parts = append(parts, "provider="+provider)
	}
	if model = strings.TrimSpace(model); model != "" {
		parts = append(parts, "model="+model)
	}
	if reason = strings.TrimSpace(reason); reason != "" {
		parts = append(parts, "reason="+reason)
	}
	if len(parts) == 0 {
		return ""
	}
	return "route: " + strings.Join(parts, " ")
}
