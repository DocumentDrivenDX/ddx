# 2k Fixture Performance Baseline

Run: `20260506T043749-7c9fea7a`
Bead: `ddx-5681cc57`
Page under test: fixture-backed artifacts list at `/artifacts?mediaType=text%2Fmarkdown`

Method:
- Playwright e2e against the fixture-backed DDx server.
- First-paint values were read from the browser `performance` paint entries on cold load and after reload.
- Scroll smoothness was measured by repeatedly scrolling the page for ~1.2s and averaging frame intervals.
- Search latency was measured from filling the search box to the filtered `1 total` state rendering.

Measurements:
- First-paint cold: `2612 ms`
- First-paint warm: `2516 ms`
- Scroll smoothness: `16.4 ms/frame` average, about `61 fps`, `74` samples
- Search latency: `878 ms`

Notes:
- The corpus contains 2,000 markdown artifacts under `cli/internal/server/frontend/e2e/fixtures/scale/docs/`.
- This is a baseline only; no CI gate is added by this bead.
