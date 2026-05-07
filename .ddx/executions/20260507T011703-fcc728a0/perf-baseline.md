# 2k Fixture Performance Baseline

Run: `20260507T011703-fcc728a0`
Bead: `ddx-5681cc57`
Page under test: fixture-backed artifacts list at `/artifacts?mediaType=text%2Fmarkdown`

Method:
- Playwright e2e against the fixture-backed DDx server.
- First-paint values were read from the browser `performance` paint entries on cold load and after reload.
- Scroll smoothness was measured by repeatedly scrolling the page for about 1.2s and averaging frame intervals.
- Search latency was measured from filling the search box to the filtered `1 total` state rendering.

Measurements:
- First-paint cold: `3108 ms`
- First-paint warm: `2900 ms`
- Scroll smoothness: `16.5 ms/frame` average, about `60.5 fps`, `73` samples
- Search latency: `372 ms`

Notes:
- The corpus contains 2,000 markdown artifacts under `cli/internal/server/frontend/e2e/fixtures/scale/docs/`.
- This is a baseline only; no CI gate is added by this bead.
