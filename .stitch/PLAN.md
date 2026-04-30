# DDx Design System — Implementation Plan

**Source of truth:** `.stitch/DESIGN.md`  
**Stitch admin project:** `13321471308989423216`  
**Stitch microsite project:** `12971431365070529344`  
**Admin light screen (generated):** `7f95ab031fce4a8f9612fbe155e7d47c`

---

## Context

The current frontend uses generic Tailwind grays and a placeholder blue palette
(`#4878c6`). The design system is already well-factored — `design-tokens.json` drives
all Tailwind colors, and `layout.css` uses CSS custom properties for semantic status/
priority colors. The migration is a token swap + component polish, not a rewrite.

The website (Hugo) has no token system yet; it pulls generic styles from the theme.

---

## Phases

### Phase 1 — Token Foundation (admin frontend)
**Files:** `cli/internal/server/frontend/design-tokens.json`, `src/app.css`  
**Effort:** 1 session, no functional changes, safe to ship independently.

1. Replace `design-tokens.json` with the new palette:
   - `accent-lever` / `dark-accent-lever` replace `primary`
   - `accent-load` / `dark-accent-load` replace `secondary`
   - `accent-fulcrum` / `dark-accent-fulcrum` replace `tertiary`
   - Warm surface trio replaces generic white/gray `background`/`surface`
   - Typography scale updated to match DESIGN.md (Inter, correct sizes/weights)
   - `borderRadius` → all `0px` (admin is fully sharp)
   - Spacing updated to 4px grid

2. Update `layout.css` CSS custom properties:
   - Status colors mapped to OKLCH equivalents derived from lever/load/fulcrum hues
   - Doc colors updated to use warm surface tokens
   - Remove the hardcoded `dark:bg-gray-950/900/800/700` overrides at bottom of file

### Phase 2 — NavShell & Layout Chrome
**Files:** `src/lib/components/NavShell.svelte`  
**Effort:** 1 session. The structural change most visible to users.

Current NavShell uses: `bg-white dark:bg-gray-950`, `border-gray-200 dark:border-gray-800`,
`bg-gray-100 dark:bg-gray-800` for active states, `rounded` on nav items.

Target:
- Top bar: `bg-bg-surface dark:bg-dark-bg-surface` + `border-border-line dark:border-dark-border-line`
- Sidebar: same surface token, `w-56` → `w-64` (matching Stitch design)
- Active nav item: remove `rounded`, add `border-l-2 border-accent-lever dark:border-dark-accent-lever`
- Active background: `bg-bg-canvas dark:bg-dark-bg-canvas`
- Remove `rounded` from nav links — the Stitch design is sharp
- Brand logotype: `font-mono font-black tracking-tighter` (DDx monospaced logotype)
- Node name → style as system metadata, `font-mono text-xs text-fg-muted`

### Phase 3 — Component Library Sweep
**Files:** All `src/lib/components/*.svelte`  
**Effort:** 2–3 sessions. Can be beaded and done incrementally.

Priority order (most visible first):
1. **BeadForm + BeadDetail** — primary data entry surfaces; status badges here
2. **CommandPalette** — high-frequency interaction; modal overlay
3. **DrainIndicator** — status indicator; small but visible in top bar
4. **TypedConfirmDialog / ConfirmDialog** — destructive action surfaces
5. **IntegrityPanel, D3Graph, Tooltip** — lower priority, less user-facing

Per component:
- Replace `gray-*` border classes with `border-border-line dark:border-dark-border-line`
- Replace `gray-*` bg classes with appropriate surface token
- Replace `rounded-*` with `rounded-none` (admin is 0px)
- Replace `blue-*` / `primary` references with `accent-lever`
- Status badges: refactor to use CSS custom props from `layout.css` pattern

### Phase 4 — Bead List & Table View
**Files:** `src/routes/nodes/[nodeId]/beads/+page.svelte` and related  
**Effort:** 1–2 sessions. The highest-density view; benefits most from the design.

- Table styling per DESIGN.md: `border-collapse`, hairline row dividers, `mono-code` IDs
- Status badges: sharp rectangles, tint+border pattern
- Priority column: `label-caps` uppercase, `error` color for P0/CRITICAL
- Bead ID column: `font-mono-code text-accent-lever` (Steel Blue monospace)
- Progress indicators: 1px height bars, `accent-load` fill

### Phase 5 — Hugo Website
**Files:** `website/` (Hugo + Hextra theme)  
**Effort:** 2 sessions. Separate track from admin.

1. Add CSS custom properties for the token set to the Hugo site's base CSS
2. Add Google Fonts: Newsreader + Space Grotesk (Inter likely already there via Hextra)
3. Override Hextra theme variables to use warm palette
4. Landing page: implement the editorial layout (hero, prose column, principles grid,
   pull quote) matching Stitch microsite design
5. Navigation: match the fixed top bar with mono DDX logotype

### Phase 6 — Stitch Feedback Loop (ongoing)
**Admin project:** `13321471308989423216`  
**Microsite project:** `12971431365070529344`

As implementation progresses, use Stitch to:
- Generate additional screens for reference (Bead Detail, Agent Run Log, Executions view)
- Edit existing screens to refine UX details before implementing in code
- Validate dark mode parity

Stitch prompts should always reference `.stitch/DESIGN.md` as design context and use
the screen ID of an existing screen as the style reference for `edit_screens` calls.

---

## Implementation Notes

**Token naming in Tailwind:** The new `design-tokens.json` will add tokens under their
semantic names (`accent-lever`, `bg-canvas`, etc.) so they're usable as `bg-accent-lever`,
`text-fg-muted`, `border-border-line` etc. in Tailwind classes. Dark mode variants use the
`dark:` prefix with `dark-*` tokens.

**Status colors:** Keep the existing CSS custom property pattern in `layout.css` — it's
clean. Just re-derive the OKLCH values from the new palette hues (steel ≈ hue 220,
brass ≈ hue 55, error ≈ hue 25).

**No functional changes in any phase** — this is purely visual. Route logic, data
fetching, stores, and the Go backend are untouched.

**Test after Phase 1** before proceeding — a broken token file will break the entire UI.

---

## Bead Breakdown

Suggested bead structure (create with `ddx bead create`):

| Bead | Title | Phase |
|------|-------|-------|
| ddx-design-tokens | Update design-tokens.json + layout.css with new palette | 1 |
| ddx-navshell | Redesign NavShell to match lever/load/fulcrum system | 2 |
| ddx-component-sweep | Apply token classes across all lib/components | 3 |
| ddx-bead-views | Restyle bead list, detail, and form views | 4 |
| ddx-website-tokens | Add DDx token system to Hugo website | 5a |
| ddx-website-landing | Implement editorial landing page (microsite Stitch design) | 5b |
