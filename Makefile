# DDx Root Makefile
# Builds CLI and copies to root for easy access

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty)
CLI_DIR = cli
ROOT_BINARY = ddx

.PHONY: all build clean test lint install help cli-build cli-clean cli-test cli-lint cli-bead-schema cli-skill-schema bead-schema skill-schema docs-routing-lint

# Default target - build CLI and copy to root
all: build

# Build CLI and copy to root
build: cli-build
	@echo "Copying CLI binary to root..."
	cp $(CLI_DIR)/build/ddx $(ROOT_BINARY)
	@echo "✅ DDx CLI built and available as ./$(ROOT_BINARY)"

# Clean all build artifacts
clean: cli-clean
	@echo "Cleaning root binary..."
	rm -f $(ROOT_BINARY)

# Run tests
test: cli-test

# Validate the shared bead schema
bead-schema: cli-bead-schema

# Validate bundled skill metadata
skill-schema: cli-skill-schema

# Run linter
lint: cli-lint

# Check docs for unallowlisted legacy routing vocabulary
docs-routing-lint:
	@echo "Checking docs for legacy routing references..."
	bash scripts/check-legacy-routing-docs.sh

# Install locally
install: build
	@echo "Installing DDx locally..."
	cp $(ROOT_BINARY) $(HOME)/.local/bin/ddx
	@echo "✅ DDx installed to ~/.local/bin/ddx"

# Format code
fmt:
	@echo "Formatting Go code..."
	cd $(CLI_DIR) && go fmt ./...

# CLI-specific targets (delegate to cli/Makefile)
cli-build:
	@echo "Building DDx CLI..."
	cd $(CLI_DIR) && $(MAKE) build

cli-clean:
	@echo "Cleaning CLI build artifacts..."
	cd $(CLI_DIR) && $(MAKE) clean

cli-test:
	@echo "Running CLI tests..."
	cd $(CLI_DIR) && $(MAKE) test

cli-bead-schema:
	@echo "Validating bead schema..."
	cd $(CLI_DIR) && $(MAKE) bead-schema

cli-skill-schema:
	@echo "Validating skill metadata..."
	cd $(CLI_DIR) && $(MAKE) skill-schema

cli-lint:
	@echo "Running CLI linter..."
	cd $(CLI_DIR) && $(MAKE) lint

cli-deps:
	@echo "Installing CLI dependencies..."
	cd $(CLI_DIR) && $(MAKE) deps

cli-update-deps:
	@echo "Updating CLI dependencies..."
	cd $(CLI_DIR) && $(MAKE) update-deps

# Development targets
dev: build
	@echo "Running DDx in development mode..."
	./$(ROOT_BINARY) $(ARGS)

# Build for all platforms
build-all:
	@echo "Building for all platforms..."
	cd $(CLI_DIR) && $(MAKE) build-all

# Create release
release:
	@echo "Creating release..."
	cd $(CLI_DIR) && $(MAKE) release

# MCP server management (uses local .claude/settings.local.json by default)
mcp-list: build
	@echo "Listing available MCP servers..."
	./$(ROOT_BINARY) mcp list

mcp-install: build
	@echo "Installing MCP server to local project configuration..."
	@if [ -z "$(SERVER)" ]; then \
		echo "❌ Error: SERVER variable required. Usage: make mcp-install SERVER=server-name"; \
		exit 1; \
	fi
	./$(ROOT_BINARY) mcp install $(SERVER) --config-path .claude/settings.local.json --yes

mcp-install-global: build
	@echo "Installing MCP server to global configuration..."
	@if [ -z "$(SERVER)" ]; then \
		echo "❌ Error: SERVER variable required. Usage: make mcp-install-global SERVER=server-name"; \
		exit 1; \
	fi
	./$(ROOT_BINARY) mcp install $(SERVER) --yes

mcp-status: build
	@echo "Checking MCP server status..."
	./$(ROOT_BINARY) mcp status

# Diagnose project
doctor: build
	@echo "Running DDx diagnostics..."
	./$(ROOT_BINARY) doctor

# Update from master repository
update: build
	@echo "Updating DDx from master repository..."
	./$(ROOT_BINARY) update

# Show help
help:
	@echo "DDx Root Build System"
	@echo ""
	@echo "Main Targets:"
	@echo "  all          - Build CLI and copy to root (default)"
	@echo "  build        - Build CLI and copy to root"
	@echo "  clean        - Clean all build artifacts"
	@echo "  test         - Run all tests"
	@echo "  lint         - Run linter"
	@echo "  install      - Install DDx locally to ~/.local/bin"
	@echo "  fmt          - Format Go code"
	@echo ""
	@echo "Development:"
	@echo "  dev          - Run DDx with arguments (set ARGS='...')"
	@echo "  doctor       - Run DDx project diagnostics"
	@echo "  update       - Update from master repository"
	@echo ""
	@echo "CLI Targets:"
	@echo "  cli-build    - Build CLI only"
	@echo "  cli-clean    - Clean CLI build artifacts"
	@echo "  cli-test     - Run CLI tests"
	@echo "  cli-lint     - Run CLI linter"
	@echo "  cli-bead-schema - Validate the shared bead schema"
	@echo "  cli-skill-schema - Validate bundled skill metadata"
	@echo "  cli-deps     - Install CLI dependencies"
	@echo ""
	@echo "Release:"
	@echo "  build-all    - Build for all platforms"
	@echo "  release      - Create release archives"
	@echo ""
	@echo "MCP Server Management:"
	@echo "  mcp-list           - List available MCP servers"
	@echo "  mcp-install        - Install MCP server locally (requires SERVER=name)"
	@echo "  mcp-install-global - Install MCP server globally (requires SERVER=name)"
	@echo "  mcp-status         - Check MCP server status"
	@echo ""
	@echo "Variables:"
	@echo "  ARGS         - Arguments to pass to 'dev' target"
	@echo "  SERVER       - MCP server name for 'mcp-install' target"
	@echo ""
	@echo "Examples:"
	@echo "  make build                           # Build CLI"
	@echo "  make bead-schema                    # Validate bead schema"
	@echo "  make skill-schema                   # Validate SKILL.md metadata"
	@echo "  make dev ARGS='mcp list'             # Run with arguments"
	@echo "  make mcp-install SERVER=filesystem   # Install to local project"
	@echo "  make mcp-install-global SERVER=github # Install globally"
