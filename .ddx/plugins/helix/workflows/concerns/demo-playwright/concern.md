# Concern: Demo Reels (Playwright)

## Category
demo

## Areas
ui, frontend

## Components

- **Recording tool**: Playwright — headless browser automation with video capture
- **Output**: WebM video files (Playwright native) and/or MP4 (via ffmpeg)
- **Playback**: `<video>` embed in microsite, or converted to GIF for README
- **Reproducible capture**: Scripted Playwright spec that drives the UI walkthrough
- **Demo scripts**: Playwright test files that exercise the app and narrate visually

## Constraints

- Demo reels are Playwright specs in `tests/e2e/` with a `reel` prefix or
  tag — they are both valid E2E tests and visual recordings
- Video recording is always-on for reel specs (not just on-failure)
- Reel specs run against a seeded database with realistic demo data — the
  same seed data used for E2E tests
- Recordings must be reproducible — re-running the reel spec should produce
  equivalent output
- Do not record manual browser sessions — always use a scripted reel spec
- Viewport: 1280x720 (720p) for consistent framing across machines
- No audio — the visual flow must be self-explanatory
- Pacing: include deliberate pauses (`page.waitForTimeout`) between actions
  so the viewer can follow at 1x speed

## Demo Reel Requirements

### Reel spec conventions

- File pattern: `tests/e2e/00-reel.spec.ts` (numbered to run first, before
  mutation-heavy tests that might dirty the seed data)
- Structured as `test.describe` blocks, one per section/act
- Each test within a describe block is one scene in the reel
- Scenes execute sequentially (`test.describe.serial`) — the reel tells a
  story with state building across scenes
- Include visual narration via injected overlay text or page titles between
  scenes — the viewer should understand what is being demonstrated

### Narrative structure

A demo reel tells a story, parallel to `demo-asciinema`:

1. **Setup** — Show the app's landing state, dashboard, or login
2. **Navigation** — Demonstrate moving between modules/sections
3. **Search** — Show finding data via search, filters, or command palette
4. **Editing** — Create, update, or delete entities through forms
5. **Verification** — Show the result: updated list, success state, changed data
6. **Summary** — Return to dashboard or overview showing the final state

### What to include in a reel

- The app's primary user journey from start to finish
- Real UI interactions that a user would perform (clicks, typing, navigation)
- Visible data that proves the app works (populated tables, completed forms)
- Transitions between major sections showing the app's breadth
- Loading states and confirmations — the viewer sees what the user sees

### What NOT to include

- Login flows (pre-authenticate or use seeded session)
- Long waits for API responses (seed data should be local/fast)
- Error states (unless error handling IS the demo topic)
- Browser chrome, DevTools, or console output
- Tooltips or hover states that flash too quickly to read at 1x

## Playwright Configuration for Reels

```typescript
// playwright.config.ts — reel-specific settings
const reelConfig = {
  use: {
    video: {
      mode: "on",           // always record for reel specs
      size: { width: 1280, height: 720 },
    },
    viewport: { width: 1280, height: 720 },
    launchOptions: {
      slowMo: 50,           // slight delay between actions for visual flow
    },
  },
};
```

Or per-spec override in the reel file:

```typescript
test.use({
  video: { mode: "on", size: { width: 1280, height: 720 } },
});
```

## Video Post-Processing

After the reel spec runs, Playwright writes `.webm` files to the test
output directory (`test-results/`).

Post-processing steps:

1. **Extract**: Copy the `.webm` from `test-results/` to
   `docs/demos/<name>/recordings/`
2. **Convert** (optional): `ffmpeg -i reel.webm -c:v libx264 reel.mp4` for
   broader browser support
3. **Trim** (optional): Cut dead time at start/end with ffmpeg
4. **GIF** (optional): `ffmpeg -i reel.webm -vf "fps=10,scale=640:-1" reel.gif`
   for README embedding (keep under 5MB)
5. **Copy to microsite**: Place in `website/static/demos/` for embedding

## Microsite Embedding

Use a standard HTML5 video element or a Hugo shortcode:

```html
<video autoplay loop muted playsinline width="100%">
  <source src="/demos/app-reel.mp4" type="video/mp4">
  <source src="/demos/app-reel.webm" type="video/webm">
</video>
```

Or a Hugo shortcode (create `layouts/shortcodes/demo-video.html`):

```markdown
{{</* demo-video src="app-reel" */>}}
```

## Drift Signals (anti-patterns to reject in review)

- Manual screen recording instead of scripted Playwright spec → automate it
- Reel spec with `video: "off"` or `video: "retain-on-failure"` → must be `"on"`
- Reel spec that depends on external services without seed data → seed locally
- Video with browser chrome visible → use headless or fullscreen viewport
- Reel with no pauses between actions → add `waitForTimeout` for pacing
- Screenshot-only demo → use video for multi-step workflows

## When to use

Any project with a web UI that needs to demonstrate functionality to users,
stakeholders, or evaluators. The web equivalent of `demo-asciinema` — scripted,
reproducible, and version-controlled. Composes with `e2e-playwright` for the
testing infrastructure and `ux-radix` for the interaction patterns being
demonstrated.

## ADR References
