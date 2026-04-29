# Practices: e2e-playwright

## Requirements (Frame phase)
- Identify all user-facing pages and workflows that need testing
- Define what "test data" means for this project — what states must the UI show?
- Determine visual regression strategy: which pages get screenshot baselines?
- Decide on browser matrix: Chromium only, or cross-browser?

## Design
- Tests live in `<project-root>/e2e/` or `website/e2e/` for microsites
- One test file per feature area or page group
- Use `test.describe()` blocks to group related tests
- Use `test.step()` for every meaningful action — produces structured logs
- Test data is deterministic and committed (or seeded deterministically)
- Screenshot baselines committed under `e2e/*.spec.ts-snapshots/`
- Video recording enabled for all tests — `video: 'on'` in config

## Implementation
- Install: `npm install -D @playwright/test && npx playwright install`
- Configure `playwright.config.ts` with video, trace, and reporter settings
- Write tests that use real browser interactions (click, type, navigate)
- Use Playwright's locator API: `getByRole()`, `getByText()`, `getByLabel()`
- Prefer semantic selectors over CSS selectors or test-ids
- Every test should:
  1. Navigate to the page
  2. Verify key content is visible
  3. Capture a full-page screenshot for visual regression
- Workflow tests should:
  1. Start at the entry point
  2. Perform the user's action sequence
  3. Verify the end state
  4. Capture screenshots at key steps

## Test Data
- **Static sites**: ensure content files cover all page types, shortcodes,
  navigation levels, and edge cases. Add a "kitchen sink" test page if needed
  that exercises every component
- **Web apps**: create a seed script (e.g., `scripts/seed-test-data.sh`) that
  populates deterministic fake data. Run before tests. Include:
  - Empty states (zero items)
  - Populated states (10+ items)
  - Error states (invalid data, missing fields)
  - Edge cases (long text, special characters, large numbers)
- **Never test against production data** — always use controlled test data

## Testing
- Run locally: `npx playwright test` (starts dev server automatically)
- Update screenshots: `npx playwright test --update-snapshots`
- Review screenshots in PR diffs — screenshot changes must be intentional
- Run in CI: same command, upload `test-results/` and `playwright-report/`
  as artifacts
- After dependency updates (Playwright, browser, framework), re-baseline
  screenshots and review

## Video Review
- Videos are generated in `test-results/<test-name>/video.webm`
- Review videos when:
  - A test fails and the error message isn't clear
  - Updating screenshot baselines (watch the video to verify visual changes)
  - Debugging flaky tests (video shows timing/race conditions)
- In CI, upload `test-results/` as a build artifact for post-hoc review

## Demo Reel
- Create `e2e/demo.spec.ts` — a single test that walks through the entire app
- Seed with realistic data that makes the demo compelling (populated states,
  not empty states)
- Set viewport to 1280x720 for clean video output
- Pace with `page.waitForTimeout()`: 1-2s between clicks, 2-3s on key screens
- Use `test.step()` to narrate each section in the test output
- Structure: Dashboard overview -> key workflows -> detail pages -> settings
- Run separately: `npx playwright test e2e/demo.spec.ts`
- Output video lives in `test-results/` — convert to `.mp4` for embedding
- Re-record after significant UI changes
- The demo is documentation — keep it passing, keep it current

## Quality Gates
- `npx playwright test` passes with zero failures
- All screenshot baselines are committed and up-to-date
- Every navigable page has at least one test
- Every user-facing workflow has at least one end-to-end test
- Video recording is enabled (not disabled for speed)
- Demo reel script exists and produces a watchable video
- No tests skip or are marked `.only`

## CI Integration
- Run Playwright tests after the build step
- Upload `test-results/` and `playwright-report/` as artifacts
- Fail the build on any test failure
- Cache Playwright browsers to speed up CI
- For static sites: build first (`hugo --gc --minify`), then serve and test
