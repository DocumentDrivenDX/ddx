# Land Safety Gates

DDx protects automatic lands with a small set of local safety gates. When one
of these gates fails, DDx does not discard the agent's work and does not leave
the target branch advanced. It preserves the attempted result under
`refs/ddx/iterations/<bead-id>/<attempt-id>-<target-tip>` and records the bead
outcome as `preserved_needs_review`.

Operator-visible bead notes and events include:

- `preserved-needs-review`
- `preserve_ref=<refs/ddx/iterations/...>`
- `gate_summary=<one-line reason>`

The preserve ref is local git evidence. Inspect it before clearing cooldowns or
rerunning the bead:

```bash
git show <preserve_ref>
git diff main...<preserve_ref>
```

## Gates

- Large deletion gate: preserves a result when one file deletes more than the
  configured threshold without an explicit acknowledgement in the commit
  message. The default threshold is 200 deleted lines.
- Syntax sanity gate: preserves obviously broken `.json`, `.go`, and truncated
  `.svelte` results before they touch the target branch.
- Post-land gate: if `git.post_land_command` is configured, DDx runs it after
  the local target ref advances and before evidence commits or push. Failure
  restores the target ref to its pre-land SHA and preserves the attempted
  result.

Configure a post-land command as argv, not a shell string:

```yaml
git:
  large_deletion_line_threshold: 200
  post_land_command:
    - sh
    - -c
    - go test ./...
```

## Intentional Large Deletions

To allow a large deletion through the gate, the worker commit message must
include one of these exact phrases:

- `intentional large deletion`
- `intentional file removal`
- `intentional file deletion`

Use the acknowledgement only when the deletion is deliberate and reviewed. A
good commit body names the removed file or directory and explains why it is
safe to remove.
