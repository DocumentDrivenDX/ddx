# Contributing to DDx

## Prerequisites

| Software | Version | Install |
|----------|---------|---------|
| Go | 1.21+ | `brew install go` or [go.dev/dl](https://go.dev/dl/) |
| Git | 2.30+ | `brew install git` or system package manager |
| Make | 3.81+ | Included on macOS/Linux |
| Hugo | 0.159+ extended | `CGO_ENABLED=1 go install -tags extended github.com/gohugoio/hugo@latest` |

### Optional

| Software | Purpose | Install |
|----------|---------|---------|
| golangci-lint | Code linting | `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` |
| air | Hot reload | `go install github.com/cosmtrek/air@latest` |
| Lefthook | Git hooks | `go install github.com/evilmartians/lefthook@latest` |
| gh | GitHub CLI | `brew install gh` |

## Getting Started

```bash
# Clone
git clone https://github.com/easel/ddx.git
cd ddx

# Install dependencies and build CLI
cd cli
make deps
make build

# Run tests
make test

# Install locally
make install
```

### Set Up Git Hooks

```bash
# From repository root
lefthook install
```

Pre-commit hooks run: formatting, linting, tests, security checks.

## Repository Structure

```
ddx/
├── cli/                # Go CLI application (ddx binary)
│   ├── cmd/            # Cobra command implementations
│   ├── internal/       # Internal packages (config, persona, git, etc.)
│   ├── main.go         # Entry point
│   ├── Makefile        # Build automation
│   └── go.mod
├── website/            # Hugo site (ddx.github.io)
│   ├── content/        # Markdown content
│   └── hugo.yaml       # Hugo config
├── .ddx/library/       # DDx document library (synced from ddx-library)
├── library/            # Local library resources
├── docs/               # Project documentation
│   ├── helix/          # HELIX frame artifacts (vision, PRD, feature specs)
│   └── resources/      # Research references
└── scripts/            # Build and automation scripts
```

## Development Commands

All CLI commands run from the `cli/` directory:

```bash
make build          # Build for current platform
make test           # Run tests
make lint           # Run linter
make fmt            # Format code
make all            # Clean, deps, test, build
make dev            # Hot reload development (requires air)
make install        # Install to ~/.local/bin/ddx
make build-all      # Cross-platform builds
```

### Website

From the `website/` directory:

```bash
hugo server         # Local dev server with live reload
hugo                # Production build to public/
```

## Making Changes

### Adding a CLI Command

1. Create `cli/cmd/<command>.go`
2. Add factory method in `cli/cmd/command_factory_commands.go`
3. Register in `cli/cmd/command_factory.go` → `registerSubcommands()`
4. Add tests in `cli/cmd/<command>_test.go`

### Adding Website Content

1. Create or edit Markdown in `website/content/`
2. Preview with `hugo server`
3. Follows [Hextra](https://imfing.github.io/hextra/) shortcode conventions

### Running CI Locally

```bash
# Full pre-commit pipeline
lefthook run pre-commit

# Or manually
cd cli && make lint && make test && make build
```

## Testing

```bash
# All tests
make test

# Verbose
go test -v ./cmd/...

# Specific test
go test -v ./cmd/... -run TestInit

# With coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

All tests must pass before committing. Tests are release-critical.

## Code Style

- `go fmt` for formatting (enforced by hooks)
- `golangci-lint` for linting
- Follow existing patterns in the codebase
- Keep the CLI core minimal — new features go in the document library, not as CLI commands

## IDE Setup

### VS Code

Recommended extensions: `golang.go`, `ms-vscode.makefile-tools`, `eamodio.gitlens`

```json
{
  "go.lintTool": "golangci-lint",
  "go.formatTool": "goimports",
  "[go]": {
    "editor.formatOnSave": true,
    "editor.codeActionsOnSave": {
      "source.organizeImports": true
    }
  }
}
```

## Troubleshooting

**Go module errors:**
```bash
go clean -modcache && go mod download
```

**Build failures:**
```bash
cd cli && make clean && make all
```

**Hugo module errors:**
```bash
cd website && hugo mod get -u
```
