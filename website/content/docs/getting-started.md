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
cd your-project
ddx init                    # create .ddx/ library structure
ddx install helix           # install HELIX workflow plugin
ddx doctor                  # verify everything works
```

## Create Work Items

```bash
ddx bead create "Design auth system" --type epic --labels "helix,phase:frame"
ddx bead create "Implement login" --type task --labels "helix,phase:build"
ddx bead dep add <task-id> <epic-id>
ddx bead list               # see all beads
ddx bead ready              # see what's unblocked
```

## Build with Agents

With DDx and HELIX installed, agents can frame, build, and evolve projects.
See the [HELIX quickstart](https://github.com/DocumentDrivenDX/helix#quickstart)
for a full walkthrough.

```bash
ddx agent run --harness claude --prompt frame-prompt.md   # create specs
ddx agent run --harness claude --prompt build-prompt.md   # implement
ddx agent usage             # check token consumption
```

## Next Steps

- Read about [document-driven development concepts](../concepts)
- See the full [CLI reference](../cli)
- Learn about the [DDx ecosystem](../ecosystem)
