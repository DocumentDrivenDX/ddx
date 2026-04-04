# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

DDx (Document-Driven Development eXperience) is a CLI toolkit for AI-assisted development that helps developers share templates, prompts, and patterns across projects. The project follows a medical differential diagnosis metaphor - using structured documentation to diagnose project issues, prescribe solutions, and share improvements.

## Architecture

The project has a dual structure:
- **CLI Application** (`/cli/`): Go-based command-line tool built with Cobra framework
- **Library Repository** (ddx-library): Templates, patterns, prompts, and configurations synced via git subtree to `.ddx/library/`

### Key Components

- `cli/` - Go CLI application source code
  - `cmd/` - Cobra command implementations (init, list, apply, doctor, update, contribute)
  - `internal/` - Internal packages (config, templates, git utilities)
  - `main.go` - Application entry point
- `.ddx/library/` - DDx library resources (synced from ddx-library repo)
  - `templates/` - Project templates (NextJS, Python, etc.)
  - `patterns/` - Reusable code patterns and examples
  - `prompts/` - AI prompts and instructions (Claude-specific and general)
  - `personas/` - AI persona definitions for consistent role-based interactions
  - `mcp-servers/` - MCP server registry and configurations
  - `configs/` - Tool configurations (ESLint, Prettier, TypeScript)
  - `workflows/` - Development workflow definitions
- `scripts/` - Build and automation scripts
- `docs/` - Project documentation

## Development Commands

### CLI Development (run from `/cli/` directory)

```bash
# Build and test
make build          # Build for current platform
make test           # Run Go tests
make lint           # Run golangci-lint (or go vet if not available)
make fmt            # Format Go code

# Development workflow
make all            # Clean, deps, test, build
make dev            # Development mode with file watching (requires air)
make run ARGS="..."  # Run CLI with arguments
make install        # Install locally to ~/.local/bin/ddx

# Dependencies
make deps           # Install and tidy Go modules
make update-deps    # Update all dependencies

# Multi-platform builds
make build-all      # Build for all platforms
make release        # Create release archives
```

### Project Structure Navigation

The CLI uses git subtree for managing the relationship between individual projects and the master DDx repository. The `.ddx.yml` configuration file defines:
- Repository URL and branch
- Included resources (prompts, scripts, templates, patterns)
- Template variables and overrides
- Git subtree settings

### Key Patterns

1. **Command Structure**: Each CLI command is implemented as a separate file in `cli/cmd/`
2. **Configuration Management**: Uses Viper for config file handling with YAML format
3. **Template Processing**: Variable substitution system for customizing templates
4. **Git Integration**: Built on git subtree for reliable version control and contribution workflows
5. **Cross-Platform Support**: Makefile supports building for multiple platforms (macOS, Linux, Windows)

## Architectural Principles

**CRITICAL**: The DDx CLI follows the principle of "Extensibility Through Composition" - keep the CLI core minimal and add features through library resources.

1. **CLI Core Minimalism**:
   - The CLI should only contain fundamental operations: init, update, apply, list, doctor, contribute
   - Tool-specific integrations (Obsidian, VSCode, etc.) belong in `.ddx/library/scripts/` or `.ddx/library/tools/`

2. **Feature Addition Pattern**:
   - New capabilities are added as templates, prompts, or scripts in the library
   - The CLI is a delivery mechanism, not a feature repository
   - Third-party tool integrations go through the library, not CLI actions

3. **Correct Implementation Pattern**:
   ```go
   // BAD: Hard-coding features in CLI
   var obsidianCmd = &cobra.Command{...}  // Don't do this

   // GOOD: Loading features from library
   ddx apply tools/obsidian/migrate.sh     // Use library scripts
   ```

## Testing and Quality

**CRITICAL**: All tests are release-critical and must pass before committing:

```bash
# Run full test suite - all tests must pass
cd cli && go test -v ./cmd
```

Every test validates critical functionality and must pass before any release.

- Go tests are in `*_test.go` files alongside source code
- Linting uses golangci-lint (fallback to go vet)
- Code formatting with `go fmt`
- Cross-platform compatibility is maintained

### Pre-commit Checks

The project uses Lefthook for git hooks. To run pre-commit checks manually:

```bash
# Run all pre-commit checks
lefthook run pre-commit

# Or stage files and run checks
git add <files>
lefthook run pre-commit
```

Pre-commit checks include:
- Secrets detection
- Binary file prevention
- Debug statement detection
- Merge conflict detection
- DDx configuration validation
- Go linting, formatting, building, and testing

## CLI Command Overview

The CLI follows a noun-verb command structure for clarity and consistency:

**Core Commands:**
- `ddx init` - Initialize DDx in a project (with optional template)
- `ddx doctor` - Check installation health and diagnose issues
- `ddx upgrade` - Upgrade DDx binary to latest release version
- `ddx update` - Update toolkit resources from master repository
- `ddx contribute` - Share improvements back to community

**Resource Commands (noun-verb structure):**
- `ddx prompts list` - List available AI prompts
- `ddx prompts show <name>` - Display a specific prompt
- `ddx templates list` - List available project templates
- `ddx templates apply <name>` - Apply a project template
- `ddx patterns list` - List available code patterns
- `ddx patterns apply <name>` - Apply a code pattern
- `ddx persona list` - List available AI personas
- `ddx persona show <name>` - Show persona details
- `ddx persona bind <role> <name>` - Bind persona to role
- `ddx mcp list` - List available MCP servers
The CLI follows the medical metaphor throughout, treating projects as patients that need diagnosis and treatment through appropriate templates and patterns.

## Persona System

DDX includes a persona system that provides consistent AI personalities for different roles:

- **Personas**: Reusable AI personality templates (e.g., `strict-code-reviewer`, `test-engineer-tdd`)
- **Roles**: Abstract functions that personas fulfill (e.g., `code-reviewer`, `test-engineer`)
- **Bindings**: Project-specific mappings between roles and personas in `.ddx.yml`

Personas enable consistent, high-quality AI interactions across team members and projects. Projects bind specific personas to roles. See `.ddx/library/personas/` for available personas and `.ddx/library/personas/README.md` for detailed documentation.

<!-- DDX-META-PROMPT:START -->
<!-- Source: claude/system-prompts/focused.md -->
# System Instructions

**Execute ONLY what is requested:**

- **YAGNI** (You Aren't Gonna Need It): Implement only specified features. No "useful additions" or "while we're here" features.
- **KISS** (Keep It Simple, Stupid): Choose the simplest solution that meets requirements. Avoid clever code or premature optimization.
- **DOWITYTD** (Do Only What I Told You To Do): Stop when the task is complete. No extra refactoring, documentation, or improvements unless explicitly requested.

**Response Style:**
- Be concise and direct
- Skip preamble and postamble
- Provide complete information without unnecessary elaboration
- Stop immediately when the task is done

**When coding:**
- Write only code needed to pass tests
- No gold-plating or speculative features
- Follow existing patterns and conventions
- Add only requested functionality
<!-- DDX-META-PROMPT:END -->
# important-instruction-reminders
Do what has been asked; nothing more, nothing less.
NEVER create files unless they're absolutely necessary for achieving your goal.
ALWAYS prefer editing an existing file to creating a new one.
NEVER proactively create documentation files (*.md) or README files. Only create documentation files if explicitly requested by the User.