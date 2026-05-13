---
title: HELIX
layout: hextra-home
---

<div class="helix-hero-layout">
<div class="helix-hero-copy">

{{< hextra/hero-badge link="why/the-thesis" >}}
  <span>See the details</span>
  {{< icon name="arrow-circle-right" attributes="height=14" >}}
{{< /hextra/hero-badge >}}

{{< hextra/hero-headline >}}
Product lifecycle for AI.
{{< /hextra/hero-headline >}}

{{< hextra/hero-subtitle >}}
Agents do better work with context they can trust. HELIX is a document discipline for teams building software with agents. It turns project intent and evidence into shared memory, then keeps that memory current as the work changes.
{{< /hextra/hero-subtitle >}}

</div>
<figure class="helix-hero-image-panel" aria-label="Document spine double helix">
  <img class="helix-hero-image helix-hero-image-light" src="hero/concepts/document-spine-helix-light-2026-05-12.png" alt="A document spine double helix connecting human intent, AI execution, and governed artifacts." />
  <img class="helix-hero-image helix-hero-image-dark" src="hero/concepts/document-spine-helix-dark-2026-05-12.png" alt="" aria-hidden="true" />
</figure>
</div>

<div class="hx-mt-16"></div>

<section class="helix-home-section">

<div class="helix-section-kicker">How it works</div>

## How you use HELIX

Start with the documents you already need. HELIX gives you the order to write
them, the prompt to review them, and the handoff an agent can execute.

<div class="helix-home-grid helix-loop-steps">
  <a class="helix-home-card" href="artifact-types">
    <span class="helix-card-label">1 · Write the brief</span>
    <strong>Capture the goal, constraints, and decisions</strong>
    <span>Write the product vision, PRD, principles, and specs the agent should follow before it edits code.</span>
  </a>
  <a class="helix-home-card" href="artifacts">
    <span class="helix-card-label">2 · Update the chain</span>
    <strong>Push each change through the documents</strong>
    <span>When the goal changes, update the downstream specs, designs, tests, and plans instead of leaving stale instructions behind.</span>
  </a>
  <a class="helix-home-card" href="skills">
    <span class="helix-card-label">3 · Run alignment</span>
    <strong>Ask an agent what no longer matches</strong>
    <span>Use the HELIX skill to find missing context, contradictions, and the next implementation plan.</span>
  </a>
  <a class="helix-home-card" href="platforms">
    <span class="helix-card-label">4 · Execute the plan</span>
    <strong>Send bounded work to your runtime</strong>
    <span>Run the plan manually, in Claude or Codex, through DDx, or inside Databricks. Feed the result back into the documents.</span>
  </a>
</div>

</section>

<div class="hx-mt-16"></div>

<section class="helix-home-section">

<div class="helix-section-kicker">Live artifact spine</div>

## HELIX's own artifacts, wired together

<div class="helix-authority-graph helix-annotated-graph" aria-label="HELIX governing artifact relationship graph">
  <div class="helix-catalog-label">
    <span>Artifact catalog</span>
    <strong>Reusable document types in authority order</strong>
  </div>
  <div class="helix-strand-label helix-human-label">
    <strong>Human judgment</strong>
    <span>Manual edits + prompts</span>
  </div>
  <div class="helix-strand-label helix-ai-label">
    <strong>AI execution</strong>
    <span>Alignment reviews + updates</span>
  </div>
  <div class="helix-graph-visual" aria-hidden="true">
    {{< helix-hero >}}
  </div>
  <a class="helix-graph-node helix-node-left helix-node-vision" href="artifacts/product-vision">
    <b class="helix-step">01</b>
    <span>Intent</span>
    <strong>Product Vision</strong>
    <em>Why HELIX exists.</em>
  </a>
  <a class="helix-graph-node helix-node-right helix-node-prd" href="artifacts/prd">
    <b class="helix-step">02</b>
    <span>Requirements</span>
    <strong>PRD</strong>
    <em>What HELIX provides and refuses.</em>
  </a>
  <a class="helix-graph-node helix-node-left helix-node-features" href="artifact-types/feature-specification">
    <b class="helix-step">03</b>
    <span>Scope</span>
    <strong>Feature Specs</strong>
    <em>Capabilities derived from requirements.</em>
  </a>
  <a class="helix-graph-node helix-node-right helix-node-principles" href="artifacts/principles">
    <span>Constraint</span>
    <strong>Principles</strong>
    <em>Values for design and review.</em>
  </a>
  <a class="helix-graph-node helix-node-left helix-node-architecture" href="artifacts/architecture">
    <b class="helix-step">04</b>
    <span>Structure</span>
    <strong>Architecture</strong>
    <em>Boundaries and system shape.</em>
  </a>
  <a class="helix-graph-node helix-node-right helix-node-tests" href="artifact-types/test-plan">
    <b class="helix-step">05</b>
    <span>Proof</span>
    <strong>Test Plans</strong>
    <em>Executable expectations.</em>
  </a>
  <a class="helix-graph-node helix-node-left helix-node-implementation" href="artifacts/implementation-plan">
    <b class="helix-step">06</b>
    <span>Execution handoff</span>
    <strong>Implementation Plans</strong>
    <em>Scoped runtime work.</em>
  </a>
  <div class="helix-collaboration-note">
    Humans provide judgement and quality, steering with edits and prompts.
    Agents review, propose updates to keep the artifact stack coherent,
    create implementation plans and execute them.
  </div>
</div>

</section>

<div class="hx-mt-16"></div>

<section class="helix-home-section">

<div class="helix-section-kicker">Artifact spine</div>

## Start with the documents that matter first

HELIX has 43 artifact types, but they are not equally important. The homepage
path starts with the core spine: the documents an alignment review should read
before it reasons about optional supporting artifacts.

<div class="helix-spine-flow" aria-label="Core HELIX artifact spine">
  <a href="artifact-types/product-vision"><span>01</span><strong>Product Vision</strong><em>Intent</em></a>
  <a href="artifact-types/prd"><span>02</span><strong>PRD</strong><em>Requirements</em></a>
  <a href="artifact-types/principles"><span>03</span><strong>Principles</strong><em>Judgment</em></a>
  <a href="artifact-types/feature-specification"><span>04</span><strong>Feature Specs</strong><em>Scope</em></a>
  <a href="artifact-types/architecture"><span>05</span><strong>Architecture</strong><em>Structure</em></a>
  <a href="artifact-types/test-plan"><span>06</span><strong>Test Plans</strong><em>Proof</em></a>
  <a href="artifact-types/implementation-plan"><span>07</span><strong>Implementation Plans</strong><em>Execution</em></a>
</div>

</section>

<div class="hx-mt-16"></div>

<section class="helix-home-section">

<div class="helix-section-kicker">Runtime paths</div>

## Use HELIX where your team already works

HELIX is Markdown and methodology. The runtime supplies file editing, review,
execution, and evidence capture.

<div class="helix-home-grid helix-platform-grid">
  <a class="helix-home-card" href="use/manual-recipe">
    <span class="helix-card-label">Manual</span>
    <strong>Small teams adopting the method first</strong>
    <span>Use Markdown, reviews, and explicit prompts before adding queue automation.</span>
  </a>
  <a class="helix-home-card" href="use/claude-code-recipe">
    <span class="helix-card-label">Claude Code</span>
    <strong>Interactive artifact review and editing</strong>
    <span>Ask an agent to reconcile documents, propose patches, and explain open questions.</span>
  </a>
  <a class="helix-home-card" href="use/codex-recipe">
    <span class="helix-card-label">Codex</span>
    <strong>Codebase-aware planning and implementation</strong>
    <span>Use the artifact spine to guide bounded code changes and implementation plans.</span>
  </a>
  <a class="helix-home-card" href="use/ddx-runtime">
    <span class="helix-card-label">DDx</span>
    <strong>Reference runtime for queued execution</strong>
    <span>Map HELIX plans to beads when queue control and execution evidence matter.</span>
  </a>
  <a class="helix-home-card" href="use/databricks-recipe">
    <span class="helix-card-label">Databricks</span>
    <strong>Data and governance workflows</strong>
    <span>Apply HELIX artifacts to analytical systems, platform work, and managed evidence.</span>
  </a>
</div>

</section>

<div class="hx-mt-16"></div>

<section class="helix-home-section">

<div class="helix-section-kicker">Proof</div>

## Inspect the foundations

The method is public: the catalog, HELIX's own governing artifacts, and the
research foundation are all inspectable.

<div class="helix-home-grid helix-proof-grid">
  <a class="helix-home-card helix-card-human" href="artifacts">
    <span class="helix-card-label">Worked artifacts</span>
    <strong>HELIX governs itself in public</strong>
    <span>Read the actual vision, PRD, principles, contracts, designs, and alignment reviews behind the project.</span>
  </a>
  <a class="helix-home-card helix-card-ai" href="artifact-types">
    <span class="helix-card-label">Artifact catalog</span>
    <strong>Reusable prompts, templates, and quality guidance</strong>
    <span>Browse the document types and learn which ones are core versus supporting.</span>
  </a>
  <a class="helix-home-card helix-card-connect" href="research">
    <span class="helix-card-label">Research foundations</span>
    <strong>Why document-driven AI work needs governance</strong>
    <span>Trace the methodology back to the research and operating assumptions that shaped it.</span>
  </a>
</div>

</section>
