package artifact

import (
	"fmt"
	"strings"
	"time"
)

func adrTemplate(id, title string, dependsOn []string) string {
	deps := formatDeps(dependsOn)
	date := time.Now().Format("2006-01-02")

	return fmt.Sprintf(`---
dun:
  id: %s
  depends_on: %s
---
# %s: %s

| Date | Status | Deciders | Confidence |
|------|--------|----------|------------|
| %s | Proposed | | |

## Context

<!-- What is the issue that we're seeing that motivates this decision? -->

## Decision

<!-- What is the change that we're proposing and/or doing? -->

## Alternatives

| Option | Pros | Cons | Evaluation |
|--------|------|------|------------|
| | | | |

## Consequences

| Type | Impact |
|------|--------|
| | |

## Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| | | | |
`, id, deps, id, title, date)
}

func sdTemplate(id, title string, dependsOn []string) string {
	deps := formatDeps(dependsOn)

	return fmt.Sprintf(`---
dun:
  id: %s
  depends_on: %s
---
# %s: %s

## Scope

**Feature:** <!-- Link to FEAT-NNN -->

## Acceptance Criteria

<!-- Use Given/When/Then format -->

1. Given ..., when ..., then ...

## Solution Approaches

**Selected Approach:**

<!-- Describe the chosen approach -->

**Key Decisions:**

-

## Component Changes

| Component | Current State | Changes | Trade-offs |
|-----------|--------------|---------|------------|
| | | | |
`, id, deps, id, title)
}

func formatDeps(deps []string) string {
	if len(deps) == 0 {
		return "[]"
	}
	quoted := make([]string, len(deps))
	for i, d := range deps {
		quoted[i] = fmt.Sprintf("%s", d)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
