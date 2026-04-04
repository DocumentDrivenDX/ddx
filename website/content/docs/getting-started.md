---
title: Getting Started
weight: 1
prev: /docs
next: /docs/concepts
---

Get DDx installed, HELIX plugged in, and your first project built in under 10 minutes.

## Install

{{< tabs >}}

{{< tab name="curl" >}}
```bash
curl -fsSL https://raw.githubusercontent.com/easel/ddx/main/install.sh | bash
```
{{< /tab >}}

{{< tab name="Go" >}}
```bash
go install github.com/easel/ddx/cli@latest
```
{{< /tab >}}

{{< tab name="Source" >}}
```bash
git clone https://github.com/easel/ddx
cd ddx/cli
make install
```
{{< /tab >}}

{{< /tabs >}}

**Requirements:** Git 2.0+.

{{< asciinema src="01-install" >}}

## Initialize Your Project

```bash
cd your-project
ddx init
```

This creates:
- `.ddx/config.yaml` — project configuration
- `.ddx/library/` — your document library (prompts, personas, patterns, templates)

{{< asciinema src="02-init-explore" >}}

## Install a Workflow Plugin

```bash
ddx search workflow       # find available plugins
ddx install helix         # install HELIX methodology
ddx installed             # verify installation
```

{{< asciinema src="03-plugin-install" >}}

## Build Something

With DDx and HELIX installed, agents can frame, build, and evolve projects:

```bash
# Frame: agent creates specs and work items
ddx agent run --harness claude --prompt frame-prompt.md

# Build: agent implements per specs with TDD
ddx agent run --harness claude --prompt build-prompt.md

# Inspect: see what was built
ddx bead list             # work items tracked
ddx agent usage           # token consumption
```

{{< asciinema src="06-full-journey" cols="100" rows="30" >}}

## Explore What's Available

```bash
ddx list              # See all document categories
ddx persona list      # Browse persona definitions
ddx doctor            # Validate your setup
```

## Check Your Setup

```bash
ddx doctor
```

Doctor validates your library structure, git configuration, and dependencies.

## Next Steps

- Read about [document-driven development concepts](../concepts)
- See the full [CLI reference](../cli)
- Learn about the [DDx ecosystem](../ecosystem)
