# DDx — Document-Driven Development eXperience

> Tools for maintaining the documents that drive AI agents to build software.

## The Idea

AI agents write code. But they only write *good* code when they have good documents to work from — clear specs, proven patterns, consistent personas, well-structured templates. DDx manages those documents.

Think of it this way: **documents are your new source code.** You maintain prompts, personas, patterns, and specs. Agents consume them and produce implementations. DDx is the toolkit that makes this workflow reliable.

## What DDx Does

- **Organizes** your agent-facing documents into a structured library
- **Syncs** document libraries across projects via git subtree
- **Shares** proven patterns and personas with the community
- **Composes** personas, patterns, and project context into agent briefs
- **Validates** that your document library is healthy and current

## Quick Start

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/easel/ddx/main/install.sh | bash

# Initialize in your project
cd your-project
ddx init

# See what's available
ddx list
ddx prompts list
ddx persona list
```

## Document Library Structure

DDx maintains a `.ddx/library/` directory in your project:

```
.ddx/library/
├── prompts/        # Instructions that direct agent behavior
├── personas/       # Behavioral definitions (strict reviewer, pragmatic builder, etc.)
├── patterns/       # Proven solutions agents can follow
├── templates/      # Project and file blueprints
├── configs/        # Tool configurations (linters, formatters)
├── mcp-servers/    # MCP server registry
└── environments/   # Environment-specific documents
```

These are plain Markdown and YAML files. Any agent can read them. Any developer can edit them.

## Commands

### Foundation
| Command | What it does |
|---------|-------------|
| `ddx init` | Initialize document library in your project |
| `ddx list [type]` | Browse available documents |
| `ddx doctor` | Validate library health |
| `ddx update` | Pull latest documents from upstream |
| `ddx contribute` | Share improvements back |
| `ddx upgrade` | Upgrade DDx binary |
| `ddx status` | Show sync state |

### Documents
| Command | What it does |
|---------|-------------|
| `ddx prompts list` | List available prompts |
| `ddx prompts show <name>` | Display a prompt |
| `ddx templates list` | List templates |
| `ddx templates apply <name>` | Apply a template |
| `ddx persona list` | List personas |
| `ddx persona show <name>` | View persona definition |
| `ddx persona bind <role> <name>` | Assign persona to role |
| `ddx mcp list` | List MCP servers |
| `ddx mcp install <name>` | Install MCP server |

## Persona System

Personas define how agents behave. DDx ships with personas like:

- **strict-code-reviewer** — Pedantic about quality, catches edge cases
- **pragmatic-implementer** — Ships working code fast, avoids over-engineering
- **test-engineer-tdd** — Writes tests first, validates thoroughly
- **simplicity-architect** — Chooses the simplest design that works

Bind personas to roles in your `.ddx.yml`:

```yaml
persona_bindings:
  code-reviewer: strict-code-reviewer
  architect: simplicity-architect
  test-engineer: test-engineer-tdd
```

Agents pick up these bindings and adjust their behavior accordingly.

## How Sync Works

DDx uses git subtree to sync your document library with a shared upstream repository:

```bash
ddx update       # Pull community improvements into your library
ddx contribute   # Push your improvements upstream
```

Every change is tracked in git. No magic, no service dependencies.

## Ecosystem

DDx is the shared infrastructure layer. Specific methodologies build on top:

```
Workflow tools (HELIX, etc.)  →  opinionated practices
DDx (this project)            →  document infrastructure
AI agents (Claude, etc.)      →  consume docs, produce code
```

What belongs in DDx: document library management, persona system, template engine, git sync, meta-prompt injection, MCP management.

What belongs elsewhere: development methodologies, phase enforcement, workflow-specific commands.

## Design Principles

1. **Documents are the product** — You maintain documents, agents produce code
2. **Git-native, file-first** — Plain files in git, no databases or services
3. **Infrastructure, not methodology** — DDx provides primitives, workflow tools provide opinions
4. **Composition over monoliths** — Combine small documents, don't maintain giant ones
5. **Agent-agnostic** — Documents work with any capable AI agent

## Installation

```bash
# One-line install
curl -fsSL https://raw.githubusercontent.com/easel/ddx/main/install.sh | bash

# Or with Go
go install github.com/easel/ddx/cli@latest

# Or from source
git clone https://github.com/easel/ddx
cd ddx/cli && make install
```

**Requires:** Git 2.0+, git-subtree. Run `ddx doctor` to verify.

## Configuration

```yaml
# .ddx.yml
version: 1
library_path: ./library
repository:
  url: https://github.com/easel/ddx
  branch: main
persona_bindings:
  code-reviewer: strict-code-reviewer
  architect: simplicity-architect
```

## License

MIT. See [LICENSE](LICENSE).
