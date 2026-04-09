---
name: ddx-review
description: Run a quorum or fresh-eyes code review using multiple agent harnesses via ddx agent run.
argument-hint: "[scope]"
---

# Review: Multi-Agent Code Review

Dispatch a structured code review across one or more agent harnesses using
`ddx agent run`. This skill prevents hallucinated reviews by using real agent
dispatch with explicit harness selection.

## When to Use

- You want a fresh-eyes review of recent changes
- You want a quorum review (multiple models agreeing on findings)
- You need to review a specific bead's implementation against its spec
- You want to avoid agents "claiming" to review code without actually running

## Steps

### 1. Determine scope

Ask the user or infer from context:
- **Bead review**: `ddx bead show <id>` to get the spec-id and acceptance
- **Diff review**: `git diff HEAD~N` or `git diff main..HEAD`
- **File review**: specific file paths

### 2. Check available harnesses

```bash
ddx agent list
ddx agent capabilities claude
ddx agent capabilities codex
```

### 3. Single harness review

```bash
ddx agent run --harness claude --effort high \
  --text "Review the following changes for correctness, security, and spec compliance: $(git diff HEAD~1)"
```

### 4. Quorum review (recommended for important changes)

```bash
ddx agent run --quorum majority --harnesses codex,claude \
  --text "Review these changes. Report: 1) bugs found, 2) security issues, 3) spec compliance. Be specific with file:line references."
```

The quorum result shows each harness's response and whether they reached
consensus.

### 5. Bead-scoped review

For reviewing a bead's implementation against its governing spec:

```bash
# Get the bead's context
ddx bead show <id>
# Read the governing artifact
ddx doc show <spec-id>

# Review with context
ddx agent run --harness claude --effort high \
  --text "Review bead <id> implementation against <spec-id>. Acceptance criteria: <paste from bead show>. Check: does the implementation meet all criteria?"
```

### 6. Report results

After the review completes:
- Summarize findings by severity (critical, warning, info)
- For quorum: note where harnesses agreed and disagreed
- Create follow-up beads for any critical findings

## References

- `ddx agent run --help`
- `ddx agent run --quorum --help`
- `ddx agent capabilities <harness>`
