# Claude Notes

See [AGENTS.md](AGENTS.md) for the main repository instructions.

Claude-specific notes:

- prefer `ddx agent run --harness claude` for non-interactive reproducible runs
- for interactive work, use Claude Code sessions with HELIX slash commands
  (currently `/helix-input`, `/helix-align`, `/helix-frame`, `/helix-design`,
  `/helix-evolve`, `/helix-review`; others are legacy — see AGENTS.md)
- `ddx agent run` is the dispatch surface — avoid calling `claude -p` directly
  in examples or docs
- when changing surfaces that touch the runtime boundary (queue, beads,
  agent dispatch), check:
  - `docs/helix/00-discover/product-vision.md` and `docs/helix/01-frame/prd.md`
    for the target shape HELIX is collapsing toward (content + one skill;
    DDx owns the runtime)
  - `workflows/principles.md` and `workflows/ratchets.md` for methodology
    invariants (in flux — verify before relying on)
  - `tests/helix-cli.sh` and `tests/validate-skills.sh` for behaviour the
    current wrappers preserve
