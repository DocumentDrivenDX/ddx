---
title: DDx
layout: hextra-home
---

{{< hextra/hero-badge link="https://github.com/DocumentDrivenDX/ddx" >}}
  <span>Open Source</span>
  {{< icon name="arrow-circle-right" attributes="height=14" >}}
{{< /hextra/hero-badge >}}

<div class="hx-mt-6 hx-mb-6">
{{< hextra/hero-headline >}}
  Documents drive the agents.&nbsp;<br class="sm:hx-block hx-hidden" />DDx drives the documents.
{{< /hextra/hero-headline >}}
</div>

<div class="hx-mb-12">
{{< hextra/hero-subtitle >}}
  The shared infrastructure for document-driven development.&nbsp;<br class="sm:hx-block hx-hidden" />Manage the prompts, personas, patterns, and specs that AI agents consume to build software.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx-mb-6">
{{< hextra/hero-button text="Get Started" link="docs/getting-started" >}}
{{< hextra/hero-button text="Learn More" link="docs/concepts" style="alt" >}}
</div>

<div class="hx-mt-6"></div>

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Document Library Management"
    subtitle="Structured .ddx/library/ with prompts, personas, patterns, and templates. Organized, discoverable, version-controlled."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%,rgba(72,120,198,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Persona Composition"
    subtitle="Define how agents behave. Bind personas to roles — strict-code-reviewer, pragmatic-implementer, test-engineer-tdd — and get consistent behavior across projects."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%,rgba(142,53,163,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Plugin Registry"
    subtitle="Install workflow plugins with one command. ddx install helix gets you a complete development methodology. Build your own plugins and share them."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%,rgba(53,163,95,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="MCP Server"
    subtitle="Serve your document library over MCP endpoints. Agents browse, search, and read documents programmatically."
  >}}
  {{< hextra/feature-card
    title="Meta-Prompt Injection"
    subtitle="Automatically inject system-level instructions into CLAUDE.md. Right baseline context for every agent session."
  >}}
  {{< hextra/feature-card
    title="Workflow-Agnostic"
    subtitle="DDx provides primitives. HELIX, your team's methodology, or no methodology at all — DDx works with any approach."
  >}}
{{< /hextra/feature-grid >}}

## See It In Action

Watch DDx + HELIX build a working Go application from scratch — framing specs, implementing with TDD, and evolving with a new feature:

{{< asciinema src="06-full-journey" cols="100" rows="30" >}}

### Quick Setup

{{< asciinema src="02-init-explore" >}}
