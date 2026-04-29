# Concerns Selection Prompt

Guide the user through selecting project concerns from the library and
declaring any project-specific overrides.

## Approach

1. List available concerns from `.ddx/plugins/helix/workflows/concerns/` grouped by category:
   - **Tech stack**: typescript-bun, python-uv, rust-cargo
   - **Accessibility**: a11y-wcag-aa
   - **Observability**: o11y-otel
   - **Internationalization**: i18n-icu

2. For each category, ask the user:
   - Tech stack: "What language, runtime, and package manager does this
     project use?"
   - Data: "What database or data layer?"
   - Infrastructure: "Where will this deploy?"
   - Quality: "Do you need accessibility (a11y), internationalization (i18n),
     or observability (o11y) support?"

3. For each selected concern:
   - Check if the user wants to override any library practices
   - If overriding, ask for the governing ADR or create one
   - Ask if an ADR exists that justified this concern selection

4. Declare the project's area labels — which `area:*` labels will beads use?
   The default set is: `ui`, `api`, `data`, `infra`, `cli`.

5. Check for practice conflicts between selected concerns and flag them.

6. Write `docs/helix/01-frame/concerns.md`.

## Key Rules

- Concerns are composable — selecting multiple is normal and expected.
- Project overrides take full precedence over library practices.
- Every override should reference a governing ADR when possible.
- The area taxonomy declared here controls which concerns are injected
  into which beads via `<context-digest>`.
