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

.PHONY: all build clean test lint deps help obsidian-migrate obsidian-validate obsidian-test obsidian-dry-run obsidian-nav obsidian-revert bead-schema skill-schema gendocs frontend-build frontend-rebuild frontend-verify copy-skills eval-skill bench-graphql

# Default target
all: clean deps test build

# Build SvelteKit frontend (required before Go build for //go:embed).
# Skip rebuild when no source file is newer than the last build output.
FRONTEND_SRC := $(shell find internal/server/frontend/src internal/server/frontend/static -type f 2>/dev/null)
FRONTEND_STAMP := internal/server/frontend/build/index.html

frontend-build: $(FRONTEND_STAMP)

$(FRONTEND_STAMP): $(FRONTEND_SRC) internal/server/frontend/package.json internal/server/frontend/svelte.config.js internal/server/frontend/vite.config.ts
	@echo "Building SvelteKit frontend..."
	cd internal/server/frontend && bun install --frozen-lockfile && bun run build
	@$(MAKE) --no-print-directory frontend-verify

# Force a frontend rebuild regardless of timestamps
frontend-rebuild:
	@echo "Rebuilding SvelteKit frontend (forced)..."
	cd internal/server/frontend && bun install --frozen-lockfile && bun run build
	@$(MAKE) --no-print-directory frontend-verify

# Verify the embedded frontend bundle is internally consistent: every
# /_app/* asset referenced by index.html must exist on disk. Catches the
# class of bug where index.html and the chunk dir get out of sync (partial
# build, file restore, manual edit) and go:embed silently bakes in a
# broken bundle that 404s every JS chunk to the SPA shell.
frontend-verify:
	@echo "Verifying embedded frontend bundle integrity..."
	@cd internal/server/frontend/build && \
	missing=0; \
	for ref in $$(grep -hoE '"/_app/[^"]+"' index.html | tr -d '"' | sort -u); do \
		path=".$$ref"; \
		if [ ! -f "$$path" ]; then \
			echo "  MISSING: $$ref"; \
			missing=$$((missing+1)); \
		fi; \
	done; \
	if [ $$missing -gt 0 ]; then \
		echo "frontend-verify: $$missing referenced asset(s) missing — run 'make frontend-rebuild'"; \
		exit 1; \
	fi; \
	echo "  ok"

# Copy the canonical top-level skills/ddx/ tree into cli/internal/skills/ddx/
# so //go:embed (which cannot traverse upward out of the package) picks it up.
# Runs as a prereq of build/test/dev so contributors never see stale embedded
# skills. rsync --delete keeps the destination in exact sync.
SKILLS_SRC := ../skills/ddx
SKILLS_DST := internal/skills/ddx
copy-skills:
	@if [ ! -d "$(SKILLS_SRC)" ]; then \
		echo "error: $(SKILLS_SRC) not found (run from cli/)"; exit 1; \
	fi
	@rsync -a --delete $(SKILLS_SRC)/ $(SKILLS_DST)/

# Build for current platform
build: copy-skills frontend-build
	@echo "Building DDx CLI..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/ddx .

# Build for all platforms
build-all: clean deps copy-skills
	@echo "Building for all platforms..."
	@mkdir -p $(BUILD_DIR)
	@echo "Building current platform..."
	go build $(LDFLAGS) -o $(BUILD_DIR)/ddx .
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
test: copy-skills
	@echo "Running tests..."
	go test -v ./...

# Run the GraphQL perf harness (ddx-9ce6842a). Seeds a 10 × 500 bead fixture
# and records baseline numbers under docs/helix/06-iterate/perf/. Override
# the fixture size or the report date via BENCH_REPORT_DATE.
BENCH_REPORT_DIR ?= $(abspath ../docs/helix/06-iterate/perf)
BENCH_REPORT_DATE ?= $(shell date -u +%Y-%m-%d)
bench-graphql:
	@echo "Running GraphQL perf harness (fixture: 10 × 500 beads = 5,000)..."
	@mkdir -p $(BENCH_REPORT_DIR)
	DDX_BENCH_REPORT_DIR=$(BENCH_REPORT_DIR) \
	DDX_BENCH_REPORT_DATE=$(BENCH_REPORT_DATE) \
	go test ./internal/server/perf/ -run TestGraphQLPerfMatrix_Baseline -v -timeout 5m
	@echo "Baseline report written to $(BENCH_REPORT_DIR)/$(BENCH_REPORT_DATE)-baseline.md"

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
dev: copy-skills
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

# Run the ddx skill eval suite (validation + routing checks)
# Use `make eval-skill` for the full suite, or invoke the script directly
# with `--validate` for structural conformance only (no harness calls).
eval-skill: copy-skills
	@cd .. && scripts/eval-skill.sh
