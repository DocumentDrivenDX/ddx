# AGENTS.md

This project uses [DDx](https://github.com/DocumentDrivenDX/ddx) for
document-driven development.

## Files to commit

After modifying any of these paths, stage and commit them:

- `.ddx/beads.jsonl` — work item tracker
- `.ddx/config.yaml` — project configuration
- `.agents/skills/` — agent skill symlinks
- `.claude/skills/` — Claude skill symlinks
- `docs/` — project documentation and artifacts

## Conventions

- Use `ddx bead` for work tracking (not custom issue files)
- Documents with `ddx:` frontmatter are tracked in the document graph
- Run `ddx doctor` to check project health
- Run `ddx doc stale` to find documents needing review
