# Concern: UX Interaction Patterns (Radix)

## Category
quality-attribute

## Areas
ui, frontend

## Components

- **Primitives**: Radix UI (headless, accessible by default)
- **Patterns**: WAI-ARIA design patterns for all interactive widgets
- **Scope**: Searching, editing, navigation, selection, and disclosure

## Constraints

### Searching

- Filterable lists use the combobox pattern (WAI-ARIA Combobox): text input
  with an associated listbox, arrow-key navigation, typeahead, and
  `aria-activedescendant` tracking
- Global search uses a command palette (Dialog + Combobox): `Cmd+K` /
  `Ctrl+K` opens a modal with instant filtering, grouped results, and
  keyboard-only completion
- Search inputs must have a visible label or `aria-label`; placeholder text
  alone is not a label
- Filtering must update results live without requiring a submit action;
  debounce network requests (200-300ms) but show local filtering instantly
- Empty states must be explicit: "No results for X" with a suggestion or
  action, not a blank container
- Clear/reset must be a single action (Escape in combobox, clear button in
  filter bars)

### Editing

- Inline editing uses the edit pattern: display value → click/Enter to
  activate → input with current value → Enter to confirm, Escape to cancel
- Form editing uses react-hook-form with Zod validation: errors appear on
  blur or submit, not on every keystroke
- Destructive actions require confirmation: Dialog with explicit
  confirm/cancel, destructive button visually distinct (red/danger variant)
- Optimistic updates for low-risk edits (toggling, reordering); pessimistic
  updates for high-risk edits (deletion, financial data)
- Undo support for reversible actions where feasible (toast with undo
  action, not just a confirmation dialog)
- Autosave for long-form content with visible save status indicator
  (saved / saving / unsaved changes)

### Navigation

- Primary navigation uses NavigationMenu (Radix): keyboard arrow-key
  traversal, `aria-current="page"` on active item, roving tabindex within
  menu groups
- Breadcrumbs for hierarchical navigation: `<nav aria-label="Breadcrumb">`
  with `aria-current="page"` on the current item, links for all ancestors
- Tab navigation uses Tabs (Radix): arrow keys switch tabs, `aria-selected`
  tracks active tab, tab panels are associated via `aria-labelledby`
- Page-level keyboard shortcuts documented and discoverable (help modal
  via `?` key); shortcuts must not conflict with browser or screen reader
  keys
- Focus must return to the trigger element when closing modals, popovers,
  and dropdown menus
- Navigation landmarks: `<main>`, `<nav>`, `<aside>`, `<header>`,
  `<footer>` — one `<main>` per page, labeled `<nav>` elements when
  multiple exist
- Skip-to-content link as the first focusable element on every page

### Selection

- Single selection: Select (Radix) or RadioGroup — arrow keys cycle options,
  typeahead jumps to matching item
- Multi-selection: Checkbox groups with `aria-describedby` for group context,
  select-all/none controls, count badge showing "N selected"
- Row selection in tables: checkbox column, Shift+click for range select,
  bulk action toolbar appears when selection is non-empty
- Selection state must be visually obvious (not just color — use checkmarks,
  borders, or background patterns for color-blind users)

### Disclosure and Overlays

- Tooltips: Tooltip (Radix) — hover and focus triggered, 200ms open delay,
  Escape to dismiss, never contain interactive content
- Popovers: Popover (Radix) — click triggered, focus trapped inside, Escape
  to close, returns focus to trigger
- Dialogs: Dialog (Radix) — focus trapped, Escape to close, scroll locked on
  body, returns focus to trigger on close
- Dropdown menus: DropdownMenu (Radix) — arrow-key navigation, typeahead,
  submenus, checkable items, Escape closes current level
- Accordions: Accordion (Radix) — arrow keys between headers, Enter/Space
  toggles, `aria-expanded` tracked
- Sheets/drawers: same focus trap and return rules as Dialog

## Drift Signals (anti-patterns to reject in review)

- Custom dropdown without keyboard navigation → use Radix Select or DropdownMenu
- Modal without focus trap → use Radix Dialog
- Search input without listbox association → use combobox pattern
- Tooltip with interactive content (links, buttons) → move to Popover
- Delete button without confirmation → add Dialog confirmation
- Filter that requires clicking "Apply" → filter live with debounce
- Navigation menu built from plain `<div>` + click handlers → use Radix NavigationMenu or semantic `<nav>` + `<a>`
- Tab implementation using divs with onClick → use Radix Tabs
- Focus lost after modal close → ensure focus returns to trigger
- Keyboard shortcut that overrides browser default (Ctrl+P, Ctrl+S) → choose non-conflicting binding

## When to use

Any project with interactive user interfaces that involve searching, editing,
navigating, or selecting data. Composes with `a11y-wcag-aa` for compliance
requirements and `react-nextjs` for React-specific implementation patterns.
Framework-agnostic in principle — the patterns are WAI-ARIA standards; Radix
is the reference implementation for React projects.

## ADR References
