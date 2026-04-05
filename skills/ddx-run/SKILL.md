---
skill:
  name: ddx-run
  description: Execute a bead with proper agent dispatch, verification, and lifecycle management.
  args: [bead-id]
---

# Run: Execute a Bead End-to-End

Pick a ready bead, dispatch an agent to implement it, verify the work, and
close the bead. This skill enforces the full lifecycle instead of letting
agents skip steps.

## When to Use

- You want to execute the next ready bead
- You want to implement a specific bead by ID
- You need the full claim → implement → verify → close lifecycle
- You want to prevent agents from "completing" work without running tests

## Steps

### 1. Select a bead

If no bead ID provided, pick the top ready bead:

```bash
ddx bead ready
```

Then inspect it:

```bash
ddx bead show <id>
```

Note the `spec-id`, `acceptance`, and `description` fields.

### 2. Read governing artifacts

```bash
ddx doc show <spec-id>
```

If the bead has a parent, read that too:

```bash
ddx bead show <parent-id>
```

### 3. Claim the bead

```bash
ddx bead update <id> --claim
```

### 4. Dispatch the agent

Assemble a prompt that includes:
- The bead's title and description
- The governing artifact content
- The acceptance criteria (verbatim)
- Instructions to commit with the bead ID in the message

```bash
ddx agent run --harness claude --effort high \
  --text "Implement bead <id>: <title>

Governing spec: <spec-id>
<paste spec content>

Acceptance criteria:
<paste from bead show>

Instructions:
1. Write tests first (TDD)
2. Implement to pass tests
3. Run: go test ./...
4. Commit with bead ID: git commit -m 'feat: <description> (<id>)'
"
```

### 5. Verify

After the agent completes, verify independently:

```bash
# Run tests
go test ./...

# Check the acceptance criteria manually
ddx bead show <id>  # re-read acceptance
```

### 6. Close or retry

If acceptance criteria are met:

```bash
ddx bead close <id>
```

If not met:

```bash
# Leave open for next attempt
ddx bead update <id> --unclaim
```

### 7. Check for unblocked work

```bash
ddx bead ready
```

Closing this bead may have unblocked downstream beads.

## Anti-Patterns This Skill Prevents

- **Claiming completion without tests**: Step 5 requires independent verification
- **Skipping the spec**: Step 2 forces reading the governing artifact
- **Hallucinated reviews**: Agent receives real spec content, not a vague prompt
- **Lost work**: Step 3 claims the bead so concurrent agents don't collide
- **Orphaned beads**: Step 6 ensures every bead ends in closed or unclaimed

## References

- `ddx bead ready`, `ddx bead show`, `ddx bead close`
- `ddx agent run --help`
- `ddx doc show <spec-id>`
