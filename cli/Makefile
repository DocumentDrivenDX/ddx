# DDx CLI Makefile

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT ?= $(shell git rev-parse --short HEAD)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS = -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}"

# Build directory
BUILD_DIR = build

# Platforms to build for
PLATFORMS = \
	darwin/amd64 \
	darwin/arm64 \
	linux/amd64 \
	linux/arm64 \
	windows/amd64

.PHONY: all build clean test lint deps help obsidian-migrate obsidian-validate obsidian-test obsidian-dry-run obsidian-nav obsidian-revert bead-schema skill-schema gendocs

# Default target
all: clean deps test build

# Build for current platform
build:
	@echo "Building DDx CLI..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/ddx .

# Build for all platforms
build-all: clean deps
	@echo "Building for all platforms..."
	@mkdir -p $(BUILD_DIR)
	@for platform in $(PLATFORMS); do \
		OS=$$(echo $$platform | cut -d'/' -f1); \
		ARCH=$$(echo $$platform | cut -d'/' -f2); \
		echo "Building $$OS/$$ARCH..."; \
		GOOS=$$OS GOARCH=$$ARCH go build $(LDFLAGS) -o $(BUILD_DIR)/ddx-$$OS-$$ARCH .; \
		if [ $$OS = "windows" ]; then \
			mv $(BUILD_DIR)/ddx-$$OS-$$ARCH $(BUILD_DIR)/ddx-$$OS-$$ARCH.exe; \
		fi; \
	done

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Validate the shared bead record schema
bead-schema:
	@echo "Validating bead record schema..."
	go test ./internal/bead -run 'TestBeadRecordSchema' -count=1

# Validate bundled skill metadata
skill-schema:
	@echo "Validating bundled skill metadata..."
	go run . skills check ../skills internal/skills

# Run linter
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found, running go vet instead"; \
		go vet ./...; \
	fi

# Install locally for development
install: build
	@echo "Installing DDx locally..."
	cp $(BUILD_DIR)/ddx $(HOME)/.local/bin/ddx
	@echo "DDx installed to ~/.local/bin/ddx"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)

# Run the CLI (for development)
run:
	go run . $(ARGS)

# Format code
fmt:
	go fmt ./...

# Update dependencies
update-deps:
	go get -u all
	go mod tidy

# Create release archives
release: build-all
	@echo "Creating release archives..."
	@cd $(BUILD_DIR) && \
	for file in ddx-*; do \
		if [[ $$file == *.exe ]]; then \
			zip $${file%.*}.zip $$file; \
		else \
			tar -czf $$file.tar.gz $$file; \
		fi; \
	done

# Development server (watches for changes)
dev:
	@echo "Starting development mode..."
	@if command -v air >/dev/null 2>&1; then \
		air; \
	else \
		echo "Air not found. Install with: go install github.com/cosmtrek/air@latest"; \
		echo "Falling back to basic run..."; \
		go run . $(ARGS); \
	fi

# Obsidian integration targets
obsidian-migrate: build
	@echo "Migrating HELIX workflow to Obsidian format..."
	./$(BUILD_DIR)/ddx obsidian migrate workflows/helix

obsidian-validate: build
	@echo "Validating Obsidian format in HELIX workflow..."
	./$(BUILD_DIR)/ddx obsidian validate workflows/helix

obsidian-test:
	@echo "Running Obsidian integration tests..."
	go test ./internal/obsidian/... -v
	go test ./internal/obsidian/converter/... -v
	go test ./internal/obsidian/validator/... -v

obsidian-dry-run: build
	@echo "Running Obsidian migration dry-run..."
	./$(BUILD_DIR)/ddx obsidian migrate --dry-run workflows/helix

obsidian-nav: build
	@echo "Generating navigation hub..."
	./$(BUILD_DIR)/ddx obsidian nav workflows/helix

obsidian-revert: build
	@echo "Reverting Obsidian migration..."
	./$(BUILD_DIR)/ddx obsidian revert --strip-frontmatter workflows/helix

# Generate Hugo markdown docs for every CLI command
gendocs:
	@echo "Generating CLI reference docs..."
	go run ./tools/gendoc

# Show help
help:
	@echo "DDx CLI Build System"
	@echo ""
	@echo "Targets:"
	@echo "  all         - Clean, install deps, test, and build"
	@echo "  build       - Build for current platform"
	@echo "  build-all   - Build for all platforms"
	@echo "  deps        - Install dependencies"
	@echo "  test        - Run tests"
	@echo "  lint        - Run linter"
	@echo "  bead-schema - Validate the shared bead record schema"
	@echo "  skill-schema - Validate bundled SKILL.md metadata"
	@echo "  install     - Install locally for development"
	@echo "  clean       - Remove build artifacts"
	@echo "  run         - Run the CLI (set ARGS='...' for arguments)"
	@echo "  fmt         - Format code"
	@echo "  update-deps - Update dependencies"
	@echo "  release     - Create release archives"
	@echo "  dev         - Development mode with file watching"
	@echo "  gendocs     - Generate CLI reference docs for the website"
	@echo "  help        - Show this help"
	@echo ""
	@echo "Obsidian Integration:"
	@echo "  obsidian-migrate   - Migrate HELIX to Obsidian format"
	@echo "  obsidian-validate  - Validate Obsidian format"
	@echo "  obsidian-test      - Run Obsidian integration tests"
	@echo "  obsidian-dry-run   - Preview migration changes"
	@echo "  obsidian-nav       - Generate navigation hub"
	@echo "  obsidian-revert    - Revert Obsidian migration"
	@echo ""
	@echo "Variables:"
	@echo "  VERSION     - Version string (default: git describe)"
	@echo "  COMMIT      - Commit hash (default: git rev-parse)"
	@echo "  DATE        - Build date (default: current UTC time)"
	@echo "  ARGS        - Arguments to pass to 'run' target"
