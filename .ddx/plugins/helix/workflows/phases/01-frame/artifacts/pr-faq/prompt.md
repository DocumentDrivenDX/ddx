# PR/FAQ Prompt

## Purpose

Synthesize the source material into a launch-day press release and FAQ using
a working-backwards lens. The PR-FAQ is not marketing copy. It is the product
argument that proves the team understands the problem, the customer outcome,
the mechanism, the objections, and the adoption boundary before downstream
requirements are written.

## Research Basis

This artifact follows Amazon/AWS Working Backwards guidance: define the
intended customer experience first, write a concise future press release
before implementation commitment, then use customer and internal FAQs to
surface the most important questions before downstream requirements harden.
See:

- `docs/resources/amazon-working-backwards-prfaq.md`
- `docs/resources/working-backwards-prfaq-template.md`

## Key Principles

- Keep the press release customer-centric, future-facing, concise, and
  jargon-free.
- Start from the customer outcome, not from the team's existing capabilities.
- State the core thesis in one reusable sentence before elaborating in the
  internal sections.
- Name the mechanism that makes the outcome possible, not only the benefit.
- Define the quality bar for the context, data, behavior, or workflow the
  product depends on.
- Define the decision boundary. If the product automates, delegates, or
  recommends work, say what the system may decide, what assumptions it may
  record, and what must return to a human.
- Split the FAQ into customer-facing questions and internal decision
  questions.
- Use the FAQ to surface adoption blockers, feasibility concerns, business
  viability, validation needs, and credible reasons not to use the product.
- Call out assumptions and high-risk gaps instead of glossing over them.
- Name which downstream artifacts or public pages should derive from this
  PR-FAQ so the argument does not drift into parallel prose.

## Method

1. Read the governing Product Vision and any existing PRD, principles,
   concern, research, or website narrative relevant to the scope.
2. Identify the customer, their context, the problem or opportunity, the
   proposed solution, the most important benefit, and how success can be
   tested.
3. Extract the strongest version of the product thesis. Prefer a plain,
   falsifiable statement over a slogan.
4. Identify the mechanism behind the thesis. For example, "better context
   produces better agent work" is a mechanism claim; "teams ship faster" is
   only an outcome claim.
5. Identify the decision or autonomy model. Avoid both extremes: do not imply
   humans make every real decision, and do not imply the system can run past
   judgment boundaries without supervision.
6. Write the press release as if the product already shipped, then write the
   FAQ as if a skeptical product, engineering, finance, legal, or operations
   reviewer is trying to find the weak points.
7. End with the downstream projection: the docs, site pages, requirements, or
   principles that should inherit this exact argument.

## Quality Checklist

- The press release is readable on its own and fits on roughly one page.
- The press release uses customer language and names a concrete customer
  problem or opportunity.
- The core thesis is one sentence and is captured before detailed internal
  explanation.
- The mechanism is explicit enough to test in the PRD or research plan.
- The quality model names the attributes the product must preserve.
- Decision and autonomy boundaries are neither vague nor over-escalating.
- Customer FAQ answers likely buyer/user questions in plain language.
- Internal FAQ answers the hard commitment questions: feasibility, viability,
  resourcing, risks, scope, kill criteria, and validation.
- The FAQ names who should not adopt the product.
- Next steps, experiments, and downstream projection targets are explicit.
