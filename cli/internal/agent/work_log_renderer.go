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
	WorkPhase     string
}

type WorkLogRenderer struct {
	now           func() time.Time
	currentBeadID string
	workPhase     string
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
		workPhase:     strings.TrimSpace(opts.WorkPhase),
	}
}

func (r WorkLogRenderer) WithCurrentBeadID(beadID string) WorkLogRenderer {
	r.currentBeadID = strings.TrimSpace(beadID)
	return r
}

func (r WorkLogRenderer) WithWorkPhase(phase string) WorkLogRenderer {
	r.workPhase = strings.TrimSpace(phase)
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

func (r WorkLogRenderer) FormatNextReadyTransition(beadID, title string) string {
	beadID = strings.TrimSpace(beadID)
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Sprintf("\n%s taking next ready bead from queue: %s\n", r.timestamp(), beadID)
	}
	return fmt.Sprintf("\n%s taking next ready bead from queue: %s — %s\n", r.timestamp(), beadID, title)
}

func (r WorkLogRenderer) FormatWatchIdle(interval time.Duration, snapshot *QueueSnapshot, includeBlockers bool) string {
	message := "no execution-ready beads; sleeping " + interval.String()
	if counts := formatWorkLogQueueCounts(snapshot); counts != "" {
		message += "; " + counts
	}
	if summary := formatWorkLogHumanBlockerSummary(snapshot); summary != "" {
		message += "; " + summary
	}

	var sb strings.Builder
	sb.WriteString(r.FormatLifecycleLine(WorkLogLifecycleLine{
		Phase:   "idle:",
		Message: message,
	}))
	if includeBlockers && snapshot != nil {
		sb.WriteString(formatWorkLogHumanBlockerEntries(snapshot.HumanReviewBlockers, 10))
	}
	return sb.String()
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
	if r.workPhase != "" {
		return r.formatWorkPhaseRoutingDecision(routing)
	}
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
		if r.workPhase != "" {
			sb.WriteString(r.workPhase)
			sb.WriteString(" ")
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return sb.String()
}

func (r WorkLogRenderer) formatWorkPhaseRoutingDecision(routing *agentlib.ServiceRoutingDecisionData) string {
	if routing == nil || (routing.Model == "" && routing.Harness == "" && routing.Provider == "") {
		return ""
	}
	parts := []string{r.timestamp(), r.workPhase, "route"}
	if target := formatWorkLogRouteTarget(routing.Harness, routing.Model); target != "" {
		parts = append(parts, target)
	}
	if routing.Provider != "" {
		parts = append(parts, "provider="+routing.Provider)
	}
	if routing.Reason != "" {
		parts = append(parts, "reason="+routing.Reason)
	}
	return strings.Join(parts, " ") + "\n"
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

func formatWorkLogRouteTarget(harness, model string) string {
	harness = strings.TrimSpace(harness)
	model = strings.TrimSpace(model)
	switch {
	case harness != "" && model != "":
		return harness + "/" + model
	case model != "":
		return model
	case harness != "":
		return harness
	default:
		return ""
	}
}

func formatWorkLogQueueCounts(snapshot *QueueSnapshot) string {
	if snapshot == nil {
		return ""
	}
	parts := []string{
		fmt.Sprintf("execution-ready=%d", snapshot.ExecutionReadyCount),
		fmt.Sprintf("blocked=%d", snapshot.BlockedCount),
		fmt.Sprintf("operator-attention=%d", snapshot.ProposedOperatorAttentionCount),
		fmt.Sprintf("needs-human/investigation=%d", snapshot.HumanReviewBlockerCount),
		fmt.Sprintf("cooldown/deferred=%d", snapshot.RetryCooldownCount),
	}
	if snapshot.NextRetryAfter != "" {
		parts = append(parts, "next-retry="+snapshot.NextRetryAfter)
	}
	parts = append(parts,
		fmt.Sprintf("execution-ineligible=%d", snapshot.ExecutionIneligibleCount),
		fmt.Sprintf("superseded=%d", snapshot.SupersededCount),
		fmt.Sprintf("epics=%d", snapshot.SkippedEpicsCount),
		fmt.Sprintf("epic-closure-candidates=%d", snapshot.EpicClosureCandidatesCount),
	)
	return "queue: " + strings.Join(parts, " ")
}

func formatWorkLogHumanBlockerSummary(snapshot *QueueSnapshot) string {
	if snapshot == nil || snapshot.HumanReviewBlockedTotal <= 0 || snapshot.HumanReviewBlockerCount <= 0 {
		return ""
	}
	return fmt.Sprintf("%d beads blocked behind %d needs-human blockers", snapshot.HumanReviewBlockedTotal, snapshot.HumanReviewBlockerCount)
}

func formatWorkLogHumanBlockerEntries(blockers []HumanReviewBlockerSnapshot, limit int) string {
	if len(blockers) == 0 || limit <= 0 {
		return ""
	}
	if len(blockers) < limit {
		limit = len(blockers)
	}
	var sb strings.Builder
	for i := 0; i < limit; i++ {
		blocker := blockers[i]
		sb.WriteString(fmt.Sprintf("  %d. %s %s (%d downstream)\n", i+1, blocker.ID, blocker.Title, blocker.DownstreamBlockedCount))
	}
	if more := len(blockers) - limit; more > 0 {
		sb.WriteString(fmt.Sprintf("  and %d more\n", more))
	}
	return sb.String()
}

func workLogQueueSnapshotSignature(snapshot *QueueSnapshot) string {
	if snapshot == nil {
		return ""
	}
	parts := []string{
		fmt.Sprintf("ready=%d", snapshot.ExecutionReadyCount),
		fmt.Sprintf("blocked=%d", snapshot.BlockedCount),
		fmt.Sprintf("deps=%d", snapshot.DependencyWaitingCount),
		fmt.Sprintf("external=%d", snapshot.ExternalBlockedCount),
		fmt.Sprintf("proposed=%d", snapshot.ProposedOperatorAttentionCount),
		fmt.Sprintf("human-blockers=%d", snapshot.HumanReviewBlockerCount),
		fmt.Sprintf("human-blocked=%d", snapshot.HumanReviewBlockedTotal),
		fmt.Sprintf("cooldown=%d", snapshot.RetryCooldownCount),
		"next-retry=" + snapshot.NextRetryAfter,
		fmt.Sprintf("ineligible=%d", snapshot.ExecutionIneligibleCount),
		fmt.Sprintf("superseded=%d", snapshot.SupersededCount),
		fmt.Sprintf("epics=%d", snapshot.SkippedEpicsCount),
		fmt.Sprintf("epic-closures=%d", snapshot.EpicClosureCandidatesCount),
	}
	for _, blocker := range snapshot.HumanReviewBlockers {
		parts = append(parts, fmt.Sprintf("blocker=%s/%s/%d/%d", blocker.ID, blocker.Title, blocker.Priority, blocker.DownstreamBlockedCount))
	}
	return strings.Join(parts, "|")
}
