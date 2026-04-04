---
title: Getting Started
weight: 1
prev: /docs
next: /docs/concepts
---

Get DDx installed and your first document library set up in under 5 minutes.

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

**Requirements:** Git 2.0+, git-subtree.

## Initialize Your Project

```bash
cd your-project
ddx init
```

This creates:
- `.ddx/config.yaml` — project configuration
- `.ddx/library/` — your document library with prompts, personas, patterns, templates

## Explore What's Available

```bash
ddx list              # See all document categories
ddx prompts list      # Browse AI prompts
ddx persona list      # Browse persona definitions
```

## Bind a Persona

Configure how agents behave in your project:

```bash
ddx persona bind code-reviewer strict-code-reviewer
```

This saves the binding to `.ddx.yml`. When agents work in your project, they pick up the persona and adjust their behavior accordingly.

## Sync with the Community

```bash
ddx update       # Pull latest improvements from upstream
ddx contribute   # Push your improvements back
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
