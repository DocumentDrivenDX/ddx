# Concern: E2E Visual Testing (Playwright)

## Category
testing

## Areas
ui, site

## Components

- **Test runner**: Playwright (`@playwright/test`)
- **Browser engines**: Chromium (default), optionally Firefox and WebKit
- **Screenshot snapshots**: Visual regression detection via `toHaveScreenshot()`
- **Video recording**: Full test execution videos for human review
- **Demo reel**: A dedicated slow-paced Playwright script that produces a
  polished walkthrough video of the application for stakeholder review,
  README embedding, or onboarding
- **Trace files**: Playwright traces for debugging failures
- **Test data**: Fake/seed data that exercises every meaningful UI state

## Constraints

- Tests run in real browsers, not jsdom or happy-dom — no simulated DOM
- Every navigable page must have at least one test that loads it and verifies
  key content
- Every user-facing workflow must have at least one test that exercises it
  end-to-end
- Test data must be realistic and comprehensive — seed the application with
  fake data that covers all UI states (empty, populated, error, edge cases)
- Tests must produce visible output:
  - Console logs showing what each test is doing (use `test.step()`)
  - Screenshots on failure (Playwright default)
  - Full-page screenshots for visual regression
  - Video recordings of every test for human review
- Screenshot baselines are committed to the repo and reviewed in PRs
- Tests must be runnable locally and in CI with identical results
- Playwright version pinned in `package.json`

## Test Data Requirements

The application under test must have a data seeding mechanism:

- **Static sites** (Hugo): content files ARE the test data — ensure content
  covers all page types, shortcodes, navigation patterns, and edge cases
  (empty sections, long titles, deep nesting)
- **Web apps**: seed script or fixture that populates the database/API with
  realistic fake data before tests run. The seed must be deterministic
  (same data every run)
- **Never test against an empty state** — an empty app tells you nothing
  about whether the UI works

## Configuration Pattern

```typescript
import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  timeout: 30000,

  // Video and trace for every test
  use: {
    baseURL: 'http://127.0.0.1:<port>',
    headless: true,
    video: 'on',
    trace: 'retain-on-failure',
    screenshot: 'on',
  },

  // Reporter with step-level detail
  reporter: [
    ['list'],
    ['html', { open: 'never' }],
  ],

  // Dev server
  webServer: {
    command: '<start command>',
    port: '<port>',
    reuseExistingServer: true,
    timeout: 15000,
  },
})
```

## Test Structure Pattern

```typescript
import { test, expect } from '@playwright/test'

test.describe('Feature Area', () => {
  test('page loads with expected content', async ({ page }) => {
    await test.step('navigate to page', async () => {
      await page.goto('/path')
    })

    await test.step('verify hero content', async () => {
      await expect(page.getByRole('heading', { name: 'Title' })).toBeVisible()
    })

    await test.step('capture screenshot', async () => {
      await expect(page).toHaveScreenshot('page-name.png', { fullPage: true })
    })
  })

  test('workflow: user completes action', async ({ page }) => {
    await test.step('start at entry point', async () => {
      await page.goto('/')
    })

    await test.step('click through workflow', async () => {
      await page.getByRole('link', { name: 'Get Started' }).click()
      await expect(page).toHaveURL(/getting-started/)
    })

    await test.step('verify end state', async () => {
      await expect(page.getByText('expected content')).toBeVisible()
    })
  })
})
```

Use `test.step()` for every meaningful action — this produces structured
logs that show exactly what each test is doing.

## Video and Artifact Output

Playwright generates:
- `test-results/` — videos, screenshots, traces per test
- `playwright-report/` — HTML report with embedded videos

Both directories should be gitignored. In CI, upload as artifacts.

## Demo Reel

Every project with a web UI should include a Playwright demo reel script that
produces a polished walkthrough video. This replaces manual screen recordings
with a reproducible, scriptable demo.

### Demo reel structure

The demo reel is a single Playwright test file (e.g., `e2e/demo.spec.ts`) that:

1. Seeds the application with comprehensive, realistic data
2. Navigates every major page and workflow at a human-readable pace
3. Uses `page.waitForTimeout()` between actions for viewing comfort
4. Produces a `.webm` video in `test-results/` that can be converted to
   `.mp4` or `.gif` for embedding

### Demo reel conventions

- File: `e2e/demo.spec.ts` (separate from test specs)
- Viewport: 1280x720 (HD, suitable for embedding)
- Pacing: 1-2 second pauses between navigations, 2-3 seconds on key screens
- Use `test.step()` for structured logging of what each section shows
- Narrative structure: Overview (dashboard) -> Primary workflows -> Detail views
- Run with: `npx playwright test e2e/demo.spec.ts`
- Output: `test-results/demo-*/video.webm`
- The demo must run against seeded data — never against an empty state
- Re-record the demo after major UI changes

### Converting output

```bash
# WebM to MP4 (for README/docs)
ffmpeg -i test-results/demo-*/video.webm -c:v libx264 -preset slow -crf 22 demo.mp4

# WebM to GIF (for README, keep under 10MB)
ffmpeg -i test-results/demo-*/video.webm -vf "fps=10,scale=960:-1" -loop 0 demo.gif
```

## When to use

Any project with a web UI that users interact with. This includes:
- Documentation sites (Hugo/Hextra microsites)
- Web applications
- Admin dashboards
- Any HTML output that needs visual consistency

## ADR References
