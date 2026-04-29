# DDx Frontend

SvelteKit frontend for the DDx server UI. Built with Bun, Tailwind CSS, Vitest, and Playwright.

## Setup

```sh
bun install
```

## Development

Start the dev server on http://localhost:5173:

```sh
bun run dev
```

## Building

Build for production (outputs to `build/`):

```sh
bun run build
```

Preview the production build:

```sh
bun run preview
```

## Testing

Run unit tests (Vitest):

```sh
bun run test:unit
```

Run end-to-end tests (Playwright):

```sh
bun run test:e2e
```

Run all tests:

```sh
bun run test
```

### Fixture-backed e2e harness

The default Playwright config (`playwright.config.ts`) boots a real `ddx server`
against a self-contained fixture workspace, so backend-dependent specs hit live
API endpoints without reading `$HOME`, `~/.config/ddx`, or the repository's live
`.ddx/` state.

What `bun run test:e2e` does on each run:

1. Runs `bun run build` to populate `frontend/build/` so the assets embedded
   into the Go binary by `cli/internal/server/embed.go` are fresh. You do
   **not** need to pre-build the frontend manually.
2. Copies `e2e/fixtures/` into a fresh `mktemp -d` workspace.
3. Runs `go build` from the `cli/` module to produce a `ddx` binary inside that
   temp dir. You do **not** need to pre-build or install `ddx` — Playwright's
   `webServer` builds it for you. (`go run` is not used; the harness execs the
   built binary so startup is fast and reproducible.) The Go toolchain must be
   on `PATH`.
4. Execs `./ddx server --tsnet=false --addr=127.0.0.1 --port=4174` from the
   temp workspace. Tailscale (tsnet) is disabled, and because the temp dir has
   no `.git`, `FindProjectRoot` falls back to it — the server sees only the
   fixture.

Override the port with `DDX_E2E_PORT` if 4174 is taken.

#### Fixture location and layout

The fixture lives at `cli/internal/server/frontend/e2e/fixtures/` and contains:

- `.ddx/config.yaml` — minimal DDx config pointing at the local plugin library.
- `.ddx/beads.jsonl` — open, closed, and blocked beads so bead endpoints return
  non-empty data.
- `.ddx/plugins/ddx/` — minimal personas, prompts, and templates so document
  and persona endpoints have something to list.
- `docs/` — small docs library for the document graph endpoint.

The fixture is treated as read-only: the harness copies it before boot, so test
runs never mutate the checked-in files.

#### Extending the fixture

When a new spec needs additional backend data, add it under
`e2e/fixtures/` as static files — never reach into `$HOME`, `~/.config/ddx`, or
the repository's own `.ddx/`. Typical extensions:

- More beads → append JSONL records to `.ddx/beads.jsonl`.
- New personas, prompts, or templates → add files under
  `.ddx/plugins/ddx/{personas,prompts,templates}/`.
- New docs for the document graph → add Markdown under `docs/`.

Keep additions minimal and self-contained; the fixture is the only supported
data source for the e2e suite. Developer-local DDx config is intentionally not
part of this harness.

> Note: `playwright.demo.config.ts` and `e2e/demo-recording.spec.ts` are a
> separate demo-recording configuration with its own setup — it is not the
> fixture-backed e2e harness described here.

## Type Checking

```sh
bun run check
```

## Linting & Formatting

```sh
bun run lint
bun run format
```

## Scaffold command

To recreate this project with the same configuration:

```sh
bun x sv@0.15.1 create --template minimal --types ts --add prettier eslint vitest="usages:unit" playwright tailwindcss="plugins:none" sveltekit-adapter="adapter:static" --no-download-check --install bun frontend
```
