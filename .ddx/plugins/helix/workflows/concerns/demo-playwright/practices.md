# Practices: demo-playwright

## Requirements (Frame phase)
- Identify which user journeys need demo reels — prioritize the "first
  impression" experience (dashboard → search → create → verify)
- Define the narrative arc: what should the viewer understand after watching?
- Determine target audience: evaluators seeing the product for the first
  time, or users learning a specific workflow?
- Ensure seed data exists that makes the demo look realistic (real names,
  realistic numbers, populated lists)

## Design
- One reel per major workflow or product area — do not combine unrelated
  features into one recording
- Reel spec lives alongside E2E tests: `tests/e2e/00-reel.spec.ts`
- Numbered `00-` prefix so it runs first, before mutation-heavy tests
- Sequential scenes within `test.describe.serial` blocks — each scene
  builds on the previous state
- Narrative structure: Landing → Navigate → Search → Edit → Verify → Summary
- Viewport: 1280x720 (720p) — consistent across all recordings
- Pacing: 500-1000ms pause between major actions via `page.waitForTimeout`

## Implementation

### Reel spec structure

```typescript
import { test } from "@playwright/test";

test.use({
  video: { mode: "on", size: { width: 1280, height: 720 } },
  viewport: { width: 1280, height: 720 },
});

test.describe.serial("Product Demo Reel", () => {
  test("Act 1: Dashboard overview", async ({ page }) => {
    await page.goto("/");
    await page.waitForTimeout(1000);  // let viewer absorb the layout
    // assert key elements visible
  });

  test("Act 2: Navigate to module", async ({ page }) => {
    await page.goto("/");
    await page.click('[data-testid="nav-finance"]');
    await page.waitForTimeout(500);
  });

  test("Act 3: Search for entity", async ({ page }) => {
    await page.goto("/finance");
    await page.keyboard.press("Meta+k");  // command palette
    await page.waitForTimeout(300);
    await page.fill('[role="combobox"]', "Orbital");
    await page.waitForTimeout(500);
    // select result, navigate
  });

  test("Act 4: Edit entity", async ({ page }) => {
    // create or update via form
    await page.waitForTimeout(500);
  });

  test("Act 5: Verify result", async ({ page }) => {
    // show updated list, success toast, changed data
    await page.waitForTimeout(1000);
  });
});
```

### Seed data requirements

- Reel specs run against the same seeded database as E2E tests
- Seed data must look realistic: real company names, plausible addresses,
  varied statuses, enough rows to fill tables without looking empty
- Deterministic seeds (fixed UUIDs, `ON CONFLICT DO NOTHING`) so reels
  are reproducible
- Include multiple entity states (draft, pending, approved, completed) to
  show workflow progression

### Pacing guidelines

| Action | Pause after |
|--------|------------|
| Page navigation | 1000ms |
| Modal/dialog open | 500ms |
| Form field fill | 300ms per field |
| Button click (submit) | 500ms |
| Search result appear | 500ms |
| Success confirmation | 1000ms |
| Section transition | 1500ms |

### Visual narration

Since there is no audio, the UI itself must tell the story:

- Use page titles and breadcrumbs to show location
- Ensure visible feedback for every action (toasts, status changes, count
  updates)
- Fill forms with readable demo data (not "test123" — use "Orbital Dynamics
  Corp", "Q4 2026 Revenue Accrual")
- If the app has a command palette, show it — it demonstrates power-user
  capability

## Recording

1. **Ensure seed data**: `bun run seed` or `bun run demo` to populate the database
2. **Run the reel spec with video**:
   ```bash
   bun run test:e2e -- tests/e2e/00-reel.spec.ts
   ```
3. **Extract video**: Playwright writes `.webm` to `test-results/`
4. **Convert** (optional):
   ```bash
   ffmpeg -i test-results/.../video.webm -c:v libx264 -crf 23 reel.mp4
   ```
5. **Copy to microsite**: `cp reel.mp4 website/static/demos/`
6. **Embed**: Use `demo-video` shortcode or `<video>` tag
7. **Commit**: Both the source recordings and microsite copy

Steps 1-3 are fully autonomous — an agent or CI job can run them without
human interaction as long as the app and database are running.

## Testing

- Reel specs are valid E2E tests — they assert correctness, not just
  visual appearance
- Run reel specs in CI without video (`video: "off"`) as a smoke test
- Run with video locally or in a scheduled CI job to regenerate recordings
- After UI changes that affect the demo flow, re-record and review

## Quality Gates

- Reel spec passes (exit 0) — broken demos must not produce recordings
- Video file exists and is playable
- Video is under 30MB for web embedding (compress further with ffmpeg if
  needed)
- All scenes in the reel exercise real functionality against seeded data
- Demo page in microsite loads and plays the video

## Maintenance

- Re-record after major UI changes, navigation restructuring, or data
  model changes
- Keep seed data aligned with the reel script — if the reel searches for
  "Orbital Dynamics", the seed must contain that entity
- When the reel drifts from the actual app behavior, treat it as a bug —
  either the reel or the app needs fixing
- Reel specs are executable documentation — they must stay correct
