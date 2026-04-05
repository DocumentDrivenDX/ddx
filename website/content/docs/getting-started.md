---
title: Getting Started
weight: 1
prev: /docs
next: /docs/concepts
---

Get DDx installed, HELIX plugged in, and your first beads created in under 5 minutes.

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

{{< tab name="Source" >}}
```bash
git clone https://github.com/DocumentDrivenDX/ddx
cd ddx/cli
make install
```
{{< /tab >}}

{{< /tabs >}}

**Requirements:** Git 2.0+.

## Initialize and Install HELIX

```bash
ddx init
ddx install helix
ddx doctor
```

## Create Work Items

```bash
ddx bead create "Design auth system" --type epic
ddx bead create "Implement login" --type task
ddx bead list
ddx bead ready
```

## Build with Agents

```bash
ddx agent run --harness claude --prompt specs.md
ddx agent usage
```

See the [HELIX quickstart](https://github.com/DocumentDrivenDX/helix#quickstart)
for a full walkthrough of the frame → design → test → build lifecycle.

## Next Steps

- [Document-driven development concepts](../concepts)
- [CLI reference](../cli)
- [DDx ecosystem](../ecosystem)
