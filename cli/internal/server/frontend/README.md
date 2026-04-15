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
