---
name: helix-release
description: 'Cut a HELIX plugin release: verify, build site, record demos, tag, push, create GitHub release.'
argument-hint: "<version>"
---

# Release

Cut a versioned HELIX plugin release with full verification, site build,
optional demo recording, GitHub tag, and GitHub Release creation.

This is a **contributor tool** for releasing the HELIX plugin itself — not part
of the HELIX workflow that downstream users consume.

## When to Use

- Cutting a new version of the HELIX plugin
- The user says "release", "cut a release", "ship it", "tag a release"
- After a batch of work is done and needs to go out as a versioned milestone

## Version Convention

HELIX uses semver with a `v` prefix: `v0.2.0`, `v0.2.1`, `v0.3.0`.

- **Patch** (0.2.x): bug fixes, doc updates, demo re-recordings, config tweaks
- **Minor** (0.x.0): new features, new actions, workflow model changes, new skills
- **Major** (x.0.0): breaking changes to the workflow contract or CLI interface

Pre-1.0, all releases are minor or patch.

## Release Checklist

Execute these steps in order. Stop and report if any step fails.

### Step 1 — Pre-flight checks

```bash
just test                           # helix-cli.sh + validate-skills.sh
helix doctor                        # installation health
```

If tests fail, fix before proceeding.

### Step 2 — Build and verify the website

```bash
cd website && hugo --gc --minify && cd ..
```

If site content changed materially, run Playwright e2e tests:

```bash
cd website && npx playwright test && cd ..
```

If baselines are stale, update and commit:

```bash
cd website && npx playwright test --update-snapshots && cd ..
```

### Step 3 — Record demos (if needed)

Only re-record if demo scripts changed since last recording or the user
explicitly asks. Check staleness:

```bash
for demo in docs/demos/helix-*/demo.sh; do
  name=$(basename $(dirname "$demo"))
  cast="website/static/demos/${name}.cast"
  if [[ -f "$cast" ]]; then
    demo_mod=$(git log -1 --format=%ct -- "$demo")
    cast_mod=$(git log -1 --format=%ct -- "$cast")
    if (( demo_mod > cast_mod )); then
      echo "STALE: $name"
    fi
  else
    echo "MISSING: $name"
  fi
done
```

To record a demo:

```bash
docker build -t helix-demo-<name> docs/demos/<name>/
docker run --rm \
  -v ~/.claude.json:/root/.claude.json:ro \
  -v ~/.claude:/root/.claude \
  -v $(pwd):/helix:ro \
  -v $(pwd)/docs/demos/<name>/recordings:/recordings \
  -e DEMO_MODEL=claude-sonnet-4-6 \
  helix-demo-<name>

# Copy newest recording to site
cp docs/demos/<name>/recordings/$(ls -t docs/demos/<name>/recordings/*.cast | head -1) \
   website/static/demos/<name>.cast
```

### Step 4 — Version bump

```bash
current=$(grep 'HELIX_VERSION=' scripts/helix | cut -d'"' -f2)
new="$ARGUMENTS"  # or determine from change scope

sed -i "s/HELIX_VERSION=\"$current\"/HELIX_VERSION=\"$new\"/" scripts/helix
sed -i "s/\"version\": \"$current\"/\"version\": \"$new\"/" .claude-plugin/plugin.json

# Verify
grep HELIX_VERSION scripts/helix
grep '"version"' .claude-plugin/plugin.json
```

### Step 5 — Generate changelog

```bash
prev_tag=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
if [[ -n "$prev_tag" ]]; then
  git log --oneline "$prev_tag"..HEAD
fi
```

Categorize into: Features, Fixes, Documentation, Other.

### Step 6 — Commit and tag

```bash
git add scripts/helix .claude-plugin/plugin.json
git add website/static/demos/ website/e2e/*.png 2>/dev/null || true

git commit -m "Release v$new

<changelog summary>"

git tag -a "v$new" -m "v$new: <one-line summary>"
```

### Step 7 — Push and monitor CI

```bash
git push origin main "v$new"

# Monitor test workflow
gh run watch $(gh run list --workflow test.yml --limit 1 --json databaseId -q '.[0].databaseId')

# Monitor pages deploy (auto-triggers on tag)
gh run watch $(gh run list --workflow pages.yml --limit 1 --json databaseId -q '.[0].databaseId')
```

If CI fails, fix, re-tag, re-push. Do NOT create a release with broken CI.

### Step 8 — Create GitHub Release

```bash
gh release create "v$new" \
  --title "v$new" \
  --notes "$changelog" \
  --verify-tag
```

Add `--prerelease` for rc/beta tags.

### Step 9 — Post-release verification

```bash
gh release view "v$new"
curl -sI https://documentdrivendx.github.io/helix/ | head -5
bash scripts/helix version
```

## Output

```
RELEASE_STATUS: COMPLETE|FAILED|PARTIAL
RELEASE_VERSION: vX.Y.Z
RELEASE_URL: https://github.com/DocumentDrivenDX/helix/releases/tag/vX.Y.Z
SITE_DEPLOYED: YES|NO|PENDING
DEMOS_RECORDED: N of M
CI_STATUS: PASS|FAIL
```
