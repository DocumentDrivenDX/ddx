# Practices: react-nextjs

## Requirements (Frame phase)
- User stories involving UI must specify which pages or components are affected
- Acceptance criteria for UI features must include Playwright E2E coverage
- Data-heavy views (tables, dashboards) must specify expected row counts for virtualization decisions

## Design
- Use Next.js App Router: layouts in `app/layout.tsx`, pages in `app/page.tsx`, route groups with `(groupName)/`
- Default to React Server Components — add `"use client"` only for interactive components (forms, modals, dropdowns, client state)
- Component hierarchy: page (server) → layout (server) → interactive widget (client)
- shadcn/ui for UI primitives — copy components via `npx shadcn@latest add <component>`, customize in `components/ui/`
- Radix UI for accessible headless primitives underlying shadcn
- Design tokens (colors, spacing, typography) in `tailwind.config.ts` — components reference tokens, not raw values
- Forms: one Zod schema per entity in `@apogee/shared`, resolved via `@hookform/resolvers/zod`
- Data tables: TanStack React Table with column definitions typed against shared schemas
- State management: server state via TanStack Query (when added), client state via Zustand (when added), form state via react-hook-form — no Redux

## Implementation
- Component files: `ComponentName.tsx` in PascalCase
- Props: define inline or as `type Props = { ... }` — no `interface` for props, no `React.FC`
- Hooks: `use` prefix, one file per hook in `hooks/` directory
- Server Components: default export async functions, fetch data directly
- Client Components: `"use client"` directive at top of file, minimize scope
- Tailwind classes: use `cn()` utility (from shadcn) for conditional classes — no `classnames` or `clsx` separately
- Forms pattern:
  ```tsx
  const form = useForm<SchemaType>({ resolver: zodResolver(schema) });
  ```
- Error boundaries: wrap route segments with `error.tsx` files
- Loading states: use `loading.tsx` files or Suspense boundaries
- Images: use `next/image` with explicit width/height — no unoptimized `<img>` tags
- Links: use `next/link` — no `<a>` tags for internal navigation

## Testing
- Unit tests: `bun:test` for component logic, hooks, and utilities
- E2E tests: Playwright in `tests/e2e/` directory
- Playwright config: headless Chromium, 30s timeout, screenshots on failure
- Test naming: `feature-name.spec.ts`
- Use page object pattern for complex flows
- Run E2E: `bun run test:e2e` (headless) or `bun run test:e2e:headed` (visible browser)
- Do not mock API responses in E2E — test against real backend with seeded data

## Quality Gates (pre-commit / CI)
- `bun run typecheck` — tsc passes for web package (includes JSX type checking)
- `bun run lint` — Biome passes (includes JSX/TSX rules)
- `bun test` — unit tests pass
- `bun run test:e2e` — Playwright E2E tests pass (CI only, requires running backend)
- No `any` in component props or hook return types
- No inline styles — use Tailwind classes

## Accessibility
- All interactive elements must have accessible labels (aria-label, aria-labelledby, or visible text)
- shadcn/ui + Radix provide WCAG 2.1 AA keyboard and screen reader support by default — do not override with custom handlers that break accessibility
- Color contrast must meet WCAG AA (4.5:1 for normal text, 3:1 for large text)
- Forms must associate labels with inputs and display validation errors accessibly
