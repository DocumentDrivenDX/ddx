# DDx — Document-Driven Development eXperience

[![CI](https://github.com/easel/ddx/actions/workflows/ci.yml/badge.svg)](https://github.com/easel/ddx/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/easel/ddx?filename=cli/go.mod)](https://github.com/easel/ddx)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> Tools for maintaining the documents that drive AI agents to build software.

<p align="center">
  <img src="website/static/demos/02-init-explore.gif" alt="DDx init and explore demo" width="700">
</p>

**[Full Documentation →](https://easel.github.io/ddx/)**

## The Idea

**Documents are your new source code.** You maintain prompts, personas, patterns, and specs. Agents consume them and produce implementations. DDx is the toolkit that makes this workflow reliable.

## Quick Start

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/easel/ddx/main/install.sh | bash

# Initialize in your project
cd your-project
ddx init

# See what's available
ddx prompts list
ddx persona list
```

## Install a Workflow Plugin

```bash
# Search for available workflows
ddx search workflow

# Install HELIX structured development methodology
ddx install helix
```

## What DDx Does

- **Organizes** your agent-facing documents into a structured library
- **Syncs** document libraries across projects via git subtree
- **Shares** proven patterns and personas with the community
- **Composes** personas, patterns, and project context into agent briefs

## Key Commands

| Command | What it does |
|---------|-------------|
| `ddx init` | Initialize document library in your project |
| `ddx doctor` | Validate library health |
| `ddx update` | Pull latest documents from upstream |
| `ddx contribute` | Share improvements back |
| `ddx persona bind <role> <name>` | Assign persona to role |
| `ddx agent run` | Invoke an AI agent with a prompt |

## Persona System

Bind AI personalities to roles in your `.ddx.yml`:

```yaml
persona_bindings:
  code-reviewer: strict-code-reviewer
  architect: simplicity-architect
  test-engineer: test-engineer-tdd
```

Ships with personas like `strict-code-reviewer`, `pragmatic-implementer`, `test-engineer-tdd`, and `simplicity-architect`.

## Ecosystem

```
Workflow tools (HELIX, etc.)  →  opinionated practices
DDx (this project)            →  document infrastructure
AI agents (Claude, etc.)      →  consume docs, produce code
```

## Installation

```bash
# One-line install
curl -fsSL https://raw.githubusercontent.com/easel/ddx/main/install.sh | bash

# Or with Go
go install github.com/easel/ddx/cli@latest
```

**Requires:** Git 2.0+, git-subtree. Run `ddx doctor` to verify.

## License

MIT. See [LICENSE](LICENSE).
