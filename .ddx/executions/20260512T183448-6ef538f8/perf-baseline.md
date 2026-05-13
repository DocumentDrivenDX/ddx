# 2k Artifact UI Baseline

Run ID: `20260512T183448-6ef538f8`

Measurement command:

```bash
XDG_DATA_HOME=$(mktemp -d) bun run test:e2e -- e2e/scale-fixture.spec.ts
```

Captured from `cli/internal/server/frontend/e2e/scale-fixture.spec.ts`.

| Metric | Value |
| --- | ---: |
| First paint, cold | `4164 ms` |
| First paint, warm | `3812 ms` |
| Scroll smoothness average frame time | `16.3 ms` |
| Scroll smoothness FPS | `61.2 fps` |
| Scroll smoothness samples | `74` |
| Search latency | `880 ms` |

Notes:

- Fixture size: `2000` artifacts under `cli/internal/server/frontend/e2e/fixtures/scale/`.
- The spec passed on this run.
- This is a baseline only. No CI gate is introduced in this bead.
