---
name: helix-build
description: 'Execute one bounded HELIX build pass. Public command surface: helix build.'
argument-hint: "[issue-id|scope]"
disable-model-invocation: true
---

# Build

Pick the next issue from the queue, build it fully, and close it.

If a specific issue or scope is given, use: $ARGUMENTS

## Steps

1. **Identify the next issue** — run `ddx bead ready`. Pick the
   highest-priority actionable issue with clear governing artifacts.

2. **Claim it** — `ddx bead update <id> --claim`.

3. **Load context** — read the governing artifacts referenced by the issue's
   spec-id and description. Understand what "done" means from the acceptance
   criteria.

4. **Build** — write the code, docs, or config changes needed to satisfy
   the issue. Stay within scope. Do not add unspecified functionality.

5. **Verify** — run the project's test suite and quality checks. Ensure:
   - All relevant tests pass
   - No previously passing checks now fail
   - Test coverage improves or stays stable
   - Lint, format, and static analysis gates pass

6. **Commit** — create a commit with:
   - Issue ID in the message
   - Concise summary of what was done
   - Reference to governing artifacts

7. **Close** — `ddx bead close <id>`.

8. **Capture follow-on work** — if build execution revealed additional work,
   create new issues for it. Do not silently absorb extra scope.
