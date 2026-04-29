# Practices: demo-asciinema

## Requirements (Frame phase)
- Identify which workflows need demo reels — prioritize the "first 5 minutes" experience
- Define the narrative arc: what story should the viewer walk away understanding?
- Determine target audience: new users evaluating the tool, or existing users learning a feature?
- List prerequisites the demo viewer should already know

## Design
- One demo per major workflow — do not combine unrelated features into one recording
- Demo directory structure: `docs/demos/<name>/` with `demo.sh`, `Dockerfile`, `README.md`
- Script the entire demo in `demo.sh` — no manual recording
- Docker container provides reproducible environment with all dependencies pinned
- Terminal dimensions: 100x30 (fits most embeds without scrolling)
- Narrative structure: Setup → Core Workflow → Verification → Summary

## Implementation
- Write `demo.sh` first, test it locally, then containerize
- Use helper functions (`narrate`, `run`, `show_file`) for consistent pacing and visibility
- `narrate()` prints section headers with visual separators (e.g., `━━━` lines)
- `run()` echoes the command before executing it, with a short sleep after
- `show_file()` displays file contents with a header, truncated to readable length
- Add `sleep 2` between major sections for viewing comfort at 1.5x playback
- Support both Docker and local execution — auto-detect via mount points
- Include retry logic for API calls (`MAX_RETRIES=3` with backoff)
- Validate expected outputs with `require_file()` and `assert_output()` helpers
- Exit non-zero on any failure — a broken demo must not produce a recording

## Recording
- Build Docker image: `docker build -t <project>-demo docs/demos/<name>/`
- Run with credential mounts — the user's Claude CLI auth is mounted into
  the container so agent calls work without interactive login:
  ```bash
  docker run --rm \
    -v ~/.claude.json:/root/.claude.json:ro \
    -v ~/.claude:/root/.claude \
    -v $(pwd):/helix:ro \
    -v $(pwd)/../ddx/ddx:/usr/local/bin/ddx:ro \
    -v $(pwd)/docs/demos/<name>/recordings:/recordings \
    <project>-demo
  ```
- This is fully autonomous — an agent can build and run the container
  without human interaction as long as `~/.claude/` and `~/.claude.json` exist
- Review: `asciinema play recordings/<file>.cast` — watch at 1x to check pacing
- Export GIF: use `agg` with default settings for README/social embedding
- Copy `.cast` to `website/static/demos/` for microsite embedding
- Commit both the source recordings and the microsite copy

## Testing
- Run `demo.sh` in CI (without recording) to catch script breakage
- If the demo depends on external services, provide a mock or skip in CI
- After tool changes that affect the demo workflow, re-record and compare
- Playwright screenshot tests on the microsite demo page catch embed breakage

## Quality Gates
- `demo.sh` exits 0 when run in its Docker container
- `.cast` file exists and is valid JSON lines (asciinema v2 format)
- `.gif` file exists and is under 5MB (reasonable for README embedding)
- Demo page loads in microsite with working asciinema player

## Maintenance
- Re-record after major CLI or workflow changes
- Keep demo scripts pinned to specific tool versions in the Dockerfile
- When the demo drifts from the actual tool behavior, treat it as a bug
- Demo scripts are executable documentation — they must stay correct
