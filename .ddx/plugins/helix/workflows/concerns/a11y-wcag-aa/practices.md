# Practices: Accessibility (WCAG 2.1 AA)

## Requirements (Frame phase)

- All user stories involving UI must include a11y acceptance criteria
- WCAG 2.1 AA is the minimum compliance target
- Accessibility is a requirement, not a nice-to-have

## Design

- All interactive components must be keyboard-navigable
- Color contrast ratios must meet AA (4.5:1 for text, 3:1 for large text)
- ARIA attributes required for custom components
- Semantic heading hierarchy required (h1 > h2 > h3, no skips)
- Focus order must follow visual reading order
- Touch targets minimum 44x44 CSS pixels

## Implementation

- Use semantic HTML elements over divs with roles
- All images require alt text (decorative images use `alt=""`)
- Form inputs require associated `<label>` elements
- Focus management for modals, drawers, and dynamic content
- Skip navigation link for keyboard users
- `aria-live` regions for dynamic status updates
- No `tabindex` values greater than 0

## Testing

- axe-core in CI for automated a11y checks
- Screen reader testing for critical user flows (VoiceOver, NVDA)
- Keyboard-only navigation testing for all interactive flows
- Color contrast validation in design review
- Test with browser zoom at 200%
