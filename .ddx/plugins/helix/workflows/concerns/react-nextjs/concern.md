# Concern: React + Next.js

## Category
tech-stack

## Areas
web, ui

## Components

- **UI Framework**: React 19 — functional components and hooks only
- **Meta-framework**: Next.js 15 — App Router (not Pages Router)
- **Component library**: shadcn/ui (copied components, not npm dependency) + Radix UI primitives
- **Styling**: Tailwind CSS 4 — utility-first, no CSS-in-JS
- **Forms**: react-hook-form + @hookform/resolvers with Zod schemas
- **Data tables**: TanStack React Table 8 — headless, sortable, filterable, virtualizable
- **Validation**: Zod — shared schemas between frontend and backend
- **E2E testing**: Playwright

## Constraints

- Use App Router (`app/` directory) — not Pages Router (`pages/`)
- Prefer React Server Components for data-heavy pages; use `"use client"` only where interactivity is required
- No class components — functional components with hooks only
- No `React.FC` type annotation — use plain function signatures with typed props
- Forms must use react-hook-form with uncontrolled components — no `useState` per field
- Validation schemas live in the shared package and are reused on frontend and backend
- shadcn/ui components are copied into the project — do not install as npm dependency
- Tailwind config extends the design system tokens (colors, spacing, typography)
- E2E tests use Playwright, not Cypress or Selenium

## Drift Signals (anti-patterns to reject in review)

- `pages/` directory for routing → must use `app/` (App Router)
- `class extends React.Component` → must use functional components
- `React.FC` or `React.FunctionComponent` → use plain typed functions
- `useState` for every form field → must use react-hook-form
- `styled-components`, `emotion`, or `css-modules` → use Tailwind utility classes
- `@shadcn/ui` as npm dependency → must be copied components
- `cypress` or `selenium` → use Playwright
- `getServerSideProps` or `getStaticProps` → use Server Components or route handlers
- Inline `fetch` in components without error/loading states → use data fetching pattern with Suspense boundaries

## When to use

React + Next.js frontend applications. Compose with `typescript-bun` for the
base TypeScript and Bun runtime concern. This concern adds React-specific UI
patterns, component library conventions, and E2E testing requirements.

## ADR References

- ADR-010: Frontend validation architecture (Zod shared schemas)
- ADR-011: ERP component library and navigation (shadcn/ui + Radix + Tailwind)
