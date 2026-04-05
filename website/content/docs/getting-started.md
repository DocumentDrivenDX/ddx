---
title: Getting Started
weight: 1
prev: /docs
next: /docs/concepts
---

Get DDx installed and start tracking work in under 5 minutes.

{{< asciinema src="07-quickstart" cols="100" rows="30" >}}

## Install

{{< tabs >}}

{{< tab name="curl" >}}
```bash
curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh | bash
```
{{< /tab >}}

{{< tab name="Go" >}}
```bash
go install github.com/DocumentDrivenDX/ddx/cli@latest
```
{{< /tab >}}

{{< /tabs >}}

## Initialize and Install HELIX

```bash
ddx init
ddx install helix
ddx doctor
```

## Track Work

```bash
ddx bead create "Build login page" --type task
ddx bead create "Add auth middleware" --type task
ddx bead list
ddx bead ready
```

## Run Agents

```bash
ddx agent run --harness claude --prompt task.md
ddx agent usage
```

## Next Steps

- [CLI reference](../cli) — all commands
- [Ecosystem](../ecosystem) — how DDx fits with HELIX and other tools
- [Creating plugins](../plugins) — add your own workflow to the registry
