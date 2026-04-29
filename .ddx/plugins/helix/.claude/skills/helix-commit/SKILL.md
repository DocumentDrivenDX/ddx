---
name: helix-commit
description: 'Commit staged changes with HELIX-compliant message format, pre-push build gate, and tracker update.'
argument-hint: '[issue-id]'
---

# Commit: HELIX-Compliant Git Commit

Create a well-formatted commit with issue traceability, run the project's
build gate, push, and optionally update the tracker.

## When to Use

- After implementing a tracked issue
- When you need a properly formatted commit message
- When codex or another agent needs commit support

## What It Does

1. **Inspect** staged changes (`git diff --cached --stat`)
2. **Format** commit message:
   - First line: `<issue-id> <concise summary>` (under 72 chars)
   - Body: what changed and why, governing artifact refs
   - Trailer: verification commands run
3. **Pre-push gate**: run the project's build check before pushing
   - `lefthook run pre-commit` if available
   - `cargo check --workspace` for Rust projects
   - `npm test` for Node.js projects
4. **Commit and push**: `git commit`, `git pull --rebase`, `git push`
5. **Tracker update** (if issue-id provided): close the issue

## Commit Message Format

```
hx-abc123 Short summary of the change

Longer description of what changed and why. Reference governing
artifacts when relevant (SD-001, TP-002, etc.).

Verification: cargo test -p changed-crate, cargo clippy
```

## Examples

```bash
helix commit hx-abc123      # commit, gate, push, close issue
helix commit                 # commit and gate without tracker update
```
