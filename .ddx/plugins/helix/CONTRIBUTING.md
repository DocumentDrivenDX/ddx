# Contributing to HELIX

## Development Setup

```bash
git clone https://github.com/DocumentDrivenDX/helix.git
cd helix
ddx install helix --local . --force
ln -s "$(pwd)/scripts/helix" ~/.local/bin/helix
helix doctor --fix
```

This symlinks the plugin and skills so edits to your checkout are
immediately reflected. `ddx install --local` will not overwrite the
`~/.local/bin/helix` symlink on subsequent runs.

Claude Code plugin mode (alternative — skills only, no CLI):

```bash
claude --plugin-dir /path/to/helix
```

## Testing

```bash
just test          # CLI tests + skill validation
just check         # tests + lint
just lint          # check for stale references
```

Individual test suites:

```bash
bash tests/helix-cli.sh       # CLI wrapper integration tests
bash tests/validate-skills.sh # Skill package structure validation
```

Website and Playwright tests (requires Hugo + Node):

```bash
cd website && hugo --gc --minify                    # build site
cd website && npx playwright test                   # e2e tests
cd website && npx playwright test --update-snapshots # update baselines
```

## Releasing

Use the `helix-release` skill (available in Claude Code when working in this
repo):

```
/helix-release 0.2.1
```

This orchestrates the full release lifecycle: pre-flight checks, site build,
optional demo recording, version bump, commit, tag, push, CI monitoring, and
GitHub Release creation.

The skill lives at `.claude/skills/helix-release/SKILL.md` — it is a
contributor tool, not part of the published HELIX skill surface.

### Manual Release

If not using the skill:

1. Run `just test` and `helix doctor`
2. Build the site: `cd website && hugo --gc --minify`
3. Re-record stale demos if needed (see `docs/demos/*/README.md`)
4. Bump version in `scripts/helix` and `.claude-plugin/plugin.json`
5. Commit: `git commit -m "Release vX.Y.Z"`
6. Tag: `git tag -a vX.Y.Z -m "vX.Y.Z: summary"`
7. Push: `git push origin main vX.Y.Z`
8. Monitor CI: `gh run watch ...`
9. Create release: `gh release create vX.Y.Z --title "vX.Y.Z" --notes "..."`

### Version Convention

- `v0.x.0` — minor: new features, actions, workflow model changes
- `v0.x.y` — patch: fixes, docs, config, demo re-recordings
- `v1.0.0` — reserved for stable workflow contract

## Skill Authoring

Published skills live in `skills/helix-<name>/SKILL.md` and are symlinked
from `.agents/skills/` and `.claude/skills/`. See `tests/validate-skills.sh`
for the required structure.

Repo-local development skills (like `helix-release`) go directly in
`.claude/skills/` without corresponding entries in `skills/` or
`.agents/skills/`.

## Demo Recording

Each demo lives in `docs/demos/<name>/` with a Dockerfile and `demo.sh`.
Recordings are `.cast` files (asciinema format). The latest recording for
each demo is copied to `website/static/demos/<name>.cast` for the microsite.

### Virtual agent replay (no API keys)

Demos use the DDX virtual agent for deterministic replay. Each demo stores
pre-recorded agent responses in `agent-dictionary/` and project fixtures in
`fixtures/`. To replay without API keys:

```bash
cd /tmp && mkdir demo && cd demo
DEMO_HARNESS=virtual HELIX_DEMO_RECORDING=1 \
  bash docs/demos/helix-quickstart/demo.sh
```

### Re-recording with real Claude

When demo scripts change, re-record golden responses:

```bash
cd /tmp && mkdir demo && cd demo
DEMO_RECORD=1 HELIX_DEMO_RECORDING=1 \
  bash docs/demos/helix-quickstart/demo.sh
```

This saves new `agent-dictionary/` and `fixtures/` back to the demo source
directory. Zero the `delay_ms` fields before committing (use python, not
`ddx jq`):

```bash
python3 -c "
import json, glob
for f in glob.glob('docs/demos/helix-*/agent-dictionary/*.json'):
    with open(f, 'r') as fh: data = json.load(fh)
    data['delay_ms'] = 0
    with open(f, 'w') as fh: json.dump(data, fh, indent=2, ensure_ascii=False)
"
```

### Docker recording

For reproducible full-container recordings, see
`docs/demos/helix-quickstart/README.md`.

### CI

The GitHub Actions pages workflow records all demos using the virtual agent
and builds the Hugo site automatically. No API keys or Docker required.
