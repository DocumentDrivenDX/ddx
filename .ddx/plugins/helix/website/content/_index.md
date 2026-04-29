---
title: HELIX
layout: hextra-home
---

{{< hextra/hero-badge link="https://github.com/DocumentDrivenDX/helix" >}}
  <span>Open Source</span>
  {{< icon name="arrow-circle-right" attributes="height=14" >}}
{{< /hextra/hero-badge >}}

<div class="hx-mt-6 hx-mb-6">
{{< hextra/hero-headline >}}
  Express intent once.&nbsp;<br class="sm:hx-block hx-hidden" />Let HELIX carry the rest.
{{< /hextra/hero-headline >}}
</div>

<div class="hx-mb-12">
{{< hextra/hero-subtitle >}}
  Supervised autopilot for AI-assisted software delivery.&nbsp;<br class="sm:hx-block hx-hidden" />Plans, builds, reviews, and iterates — stopping only when human judgment is needed.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx-mb-12">
{{< hextra/hero-button text="Get Started" link="docs/getting-started" >}}
{{< hextra/hero-button text="Workflow Guide" link="docs/workflow" style="alt" >}}
</div>

<div class="hx-mt-8"></div>

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Supervisory Autopilot"
    subtitle="helix run continuously selects the highest-leverage next action — frame, design, build, review, align — until human input is actually needed."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%,rgba(72,120,198,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Tracker as Steering Wheel"
    subtitle="Users steer by creating issues, setting priorities, and approving gates. Agents read tracker state and execute. The tracker is the shared state between humans and agents."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%,rgba(142,53,163,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Authority-Ordered Reconciliation"
    subtitle="When artifacts disagree, HELIX resolves conflicts by escalating to the governing source — vision governs requirements, requirements govern design, design governs code."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%,rgba(53,163,95,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Cross-Model Verification"
    subtitle="Critical artifacts are reviewed by alternating AI models. Different models have different blind spots — adversarial rotation catches what self-review misses."
  >}}
  {{< hextra/feature-card
    title="Interactive at Any Layer"
    subtitle="Work directly on vision, specs, designs, tests, or code. HELIX picks up where you left off — direct commands are intervention points inside the same control system."
  >}}
  {{< hextra/feature-card
    title="Least-Power Execution"
    subtitle="Refine a spec before redesigning a system. Sharpen issues before implementing. Reconcile artifacts before inventing new ones. The smallest sufficient action wins."
  >}}
{{< /hextra/feature-grid >}}

<div class="hx-mt-16"></div>

## See It In Action

{{< asciinema src="helix-quickstart" >}}

See the [quickstart guide](docs/getting-started) to try it yourself.
