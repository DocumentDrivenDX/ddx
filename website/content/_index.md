---
title: DDx
---

{{< hextra/hero-badge link="https://github.com/DocumentDrivenDX/ddx" >}}
  <span>Open Source</span>
  {{< icon name="arrow-circle-right" attributes="height=14" >}}
{{< /hextra/hero-badge >}}

<!-- HERO_GRAPHIC_PLACEHOLDER
     V3 of bead ddx-5028c8e6 deferred the nano-banana hero graphic (no
     OPENROUTER_API_KEY at execution time). Drop the generated image at
     website/static/hero/landing.webp (or .png) and uncomment the block
     below. See website/static/hero/HERO_BLOCKER.md for the prompt and
     integration steps.

<div class="hx-mt-6 hx-flex hx-justify-center">
  <img src="hero/landing.webp" alt="DDx — documents drive the agents" class="hx-rounded-xl hx-shadow-xl" style="max-width: 960px; width: 100%; height: auto;" />
</div>

HERO_GRAPHIC_PLACEHOLDER -->

<div class="hx-mt-6 hx-mb-6">
{{< hextra/hero-headline >}}
  Documents drive the agents.&nbsp;<br class="sm:hx-block hx-hidden" />DDx drives the documents.
{{< /hextra/hero-headline >}}
</div>

<div class="hx-mt-2 hx-mb-6 hx-text-center">
  <a href="docs/concepts/software-factory/" class="hx-text-lg hx-text-gray-600 dark:hx-text-gray-400 hover:hx-text-primary-600">A document-driven software factory.</a>
</div>

<div class="hx-mb-12">
{{< hextra/hero-subtitle >}}
  The local-first platform for AI-assisted development.&nbsp;<br class="sm:hx-block hx-hidden" />Track work, dispatch agents, manage specs, and install workflow plugins — all from one CLI.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx-mb-12">
{{< hextra/hero-button text="Get Started" link="docs/getting-started" >}}
{{< hextra/hero-button text="Learn More" link="docs/concepts" style="alt" >}}
</div>

<div class="hx-mt-8"></div>

<!-- HERO_GRAPHIC_PLACEHOLDER
     Once website/static/hero/landing.webp (or .png) exists, uncomment the
     block below to render the hero image above the feature grid. The asset
     is generated via the nano-banana-pro-openrouter skill — see
     website/static/hero/HERO_BLOCKER.md for the prompt and selection
     criteria. Do not commit a generic-AI placeholder here.

<div class="hx-mb-12 hx-flex hx-justify-center">
  <img src="/hero/landing.webp" alt="Documents drive the agents — DDx drives the documents." class="hx-max-w-4xl hx-w-full hx-rounded-lg" />
</div>
-->

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Work Tracker"
    subtitle="Beads track every task with dependencies, claims, and status. Agents claim work, close beads, and the queue drives what happens next."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%,rgba(72,120,198,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Plugin Registry"
    subtitle="One command to install a workflow. ddx install helix gives you structured development with AI agents out of the box."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%,rgba(53,163,95,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Execution Engine"
    subtitle="Define, run, and record execution evidence. Every agent invocation, test run, and check is captured with structured results."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%,rgba(142,53,163,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Agent Dispatch"
    subtitle="Run AI agents through one interface. Track token usage and costs across Claude, Codex, and Gemini."
  >}}
  {{< hextra/feature-card
    title="MCP Server"
    subtitle="Serve beads, documents, and execution history over MCP and HTTP. Remote supervisors can observe and steer work."
  >}}
  {{< hextra/feature-card
    title="Workflow-Agnostic"
    subtitle="DDx provides primitives. HELIX, your methodology, or none at all — DDx works with any approach."
  >}}
{{< /hextra/feature-grid >}}

<div class="hx-mt-16"></div>

## See It In Action

{{< asciinema src="07-quickstart" cols="100" rows="30" >}}
