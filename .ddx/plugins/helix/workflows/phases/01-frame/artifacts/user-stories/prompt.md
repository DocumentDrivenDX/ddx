# User Story Generation Prompt

Create standalone user stories that serve as stable design artifacts — vertical
slices referenced throughout design, implementation, and testing.

## Storage Location

Store at: `docs/helix/01-frame/user-stories/US-NNN-<slug>.md` (one file per story)

## Purpose

User stories are **governing design artifacts**, not throwaway tickets. Each
story defines one persona's complete vertical journey through feature behavior
that is independently implementable and testable. Tracker issues reference
stories; stories don't reference tracker issues. Stories are more stable than
the implementation beads that fulfill them.

The feature spec owns behavior and boundaries. A user story owns a journey
through that behavior: who starts it, what they do, what the system shows, and
what outcome proves the slice works.

## Reference Anchors

Use these local resource summaries as grounding:

- `docs/resources/atlassian-user-stories.md` grounds persona-goal-value story
  framing and acceptance criteria.
- `docs/resources/cucumber-executable-specifications.md` grounds observable
  Given/When/Then acceptance criteria without requiring BDD tooling.

## Key Principles

- **One story, one vertical slice** — a story should trace a complete path
  from user action to outcome. If it can't be demonstrated end-to-end, it's
  not a story yet.
- **Stable reference** — stories will be referenced by multiple tracker issues
  across design, implementation, and testing. Write them to last.
- **Implementer-sufficient** — an implementer reading only this story and the
  parent feature spec should be able to build it without asking clarifying
  questions.
- **Test-first friendly** — acceptance criteria and test scenarios should be
  concrete enough to write tests before writing code.
- **Traceable to feature behavior** — each story should name the feature
  requirements it exercises. Do not invent behavior outside the parent feature
  spec.

## Boundary Test

| If you are writing... | Put it in... |
|---|---|
| Product-level scope, personas, priorities, or metrics | PRD |
| Complete feature behavior, functional areas, and edge cases | Feature Specification |
| One persona's journey through a feature slice | User Story |
| Component design, data model, API shape, or build approach | Solution/Technical Design |
| Detailed fixtures, test harnesses, or automation strategy | Story Test Plan |
| Work assignment, status, or execution notes | DDx bead or runtime issue |

## Section-by-Section Guidance

### Story (As a / I want / So that)
The "As a" must name a specific persona from the PRD, not a generic role.
The "I want" must describe what the user does, not what the system does
internally. The "So that" must name a measurable outcome or business value —
"so that I can use the feature" is circular.

### Context
This is the background an implementer needs to make judgment calls. Why does
this story exist? What's the user's situation? Which parent feature
requirements does it exercise? What pain are they hitting?
2-4 sentences, not a paragraph of filler. Test: would removing this section
force the implementer to ask a question? If not, it's too generic.

### Walkthrough
A step-by-step journey through the vertical slice. Present tense, concrete
actions. This is not a flowchart — it's one specific path (the happy path)
from trigger to outcome. Branching and error cases go in Edge Cases.

Test: could a QA engineer use this walkthrough as a manual test script?

### Acceptance Criteria
Given/When/Then format. Each criterion must be independently testable — one
clear precondition, one action, one observable outcome. Avoid compound
criteria ("Given A and B and C, when D, then E and F and G"). Split those
into separate criteria.

### Edge Cases
What happens when the user does something unexpected, inputs are invalid,
or the system is in an unusual state? Each edge case names the condition and
the expected behavior. Don't just list failure modes — specify what the system
should do.

### Test Scenarios
Concrete input/output pairs. An implementer should be able to copy these into
a test file with minimal modification. Include the happy path and at least one
edge case from the section above. Name specific values, not placeholders.

### Dependencies
Name other stories this one depends on (by ID), the parent feature spec,
and any external systems or APIs. If another story must be done first, say so.

### Traceability
Name the parent feature requirement IDs that the story exercises. If the story
needs behavior that is not in the feature spec, update the feature spec first.

### Out of Scope
What this story explicitly does not cover. Each item should exclude something
an implementer might reasonably try to include. This prevents scope creep
during implementation.

## Quality Checklist

After drafting, verify every item. If any blocking check fails, revise before
committing.

### Blocking

- [ ] Story names a specific persona from the PRD (not a generic role)
- [ ] "I want" describes a user action, not a system behavior
- [ ] "So that" names a measurable outcome, not a tautology
- [ ] Walkthrough traces a complete path from trigger to outcome
- [ ] Every acceptance criterion is independently testable (one Given/When/Then)
- [ ] Test scenarios include concrete values, not placeholders
- [ ] Story links to parent feature spec by ID
- [ ] Story names the parent feature requirement IDs it exercises

### Warning

- [ ] Context would be missed if removed (not generic filler)
- [ ] At least one edge case is documented
- [ ] Test scenarios cover both happy path and at least one edge case
- [ ] Out of scope excludes something plausible
- [ ] No compound acceptance criteria (split into separate items)
- [ ] Story does not invent behavior outside the parent feature spec
