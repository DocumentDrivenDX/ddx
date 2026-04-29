# Claude Notes

See [AGENTS.md](AGENTS.md) for the main repository instructions.

Claude-specific notes:

- prefer `ddx agent run --harness claude` for non-interactive reproducible runs
- for interactive work, use Claude Code sessions with HELIX slash commands
  (`/helix-run`, `/helix-build`, etc.)
- the `helix` CLI dispatches agents via `ddx agent run` internally — avoid
  calling `claude -p` directly in examples or docs
- when updating the HELIX operator flow, also check:
  - `workflows/EXECUTION.md`
  - `ddx bead --help` (tracker conventions now in DDx FEAT-004)
  - `workflows/ratchets.md`
  - `tests/helix-cli.sh`
