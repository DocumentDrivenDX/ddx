# HELIX development tasks

# Run all tests
test: test-cli test-deploy-artifacts test-state-rules test-skills test-context-digests test-demo-fixtures test-install

# Serve the HELIX microsite at the canonical local review URL.
website-serve:
    bash website/scripts/serve-local.sh

# Run CLI wrapper tests
test-cli:
    bash tests/helix-cli.sh

# Validate deploy artifact graph consistency
test-deploy-artifacts:
    bash tests/validate-deploy-artifacts.sh

# Validate state detection rules
test-state-rules:
    bash tests/validate-state-rules.sh

# Run skill package validation
test-skills:
    bash tests/validate-skills.sh

# Validate live tracker context-digest coverage
test-context-digests:
    bash tests/validate-context-digests.sh

# Validate demo prompt fixtures
test-demo-fixtures:
    bash tests/validate-demo-fixtures.sh

# Run install integration test
test-install:
    bash tests/test-install.sh

# Run all tests and check for stale references
check: test lint

# Lint for common issues
lint:
    @echo "Checking for stale command references..."
    @! grep -rn 'NEXT_ACTION.*IMPLEMENT\b' workflows/ scripts/ tests/ --include='*.sh' --include='*.md' 2>/dev/null | grep -v 'BUILD|IMPLEMENT' || (echo "FAIL: stale IMPLEMENT references found" && exit 1)
    @! grep -rn 'NEXT_ACTION.*\bPLAN\b' workflows/actions/check.md scripts/helix tests/ 2>/dev/null | grep -v 'DESIGN|PLAN_STATUS\|PLAN_DOCUMENT\|PLAN_ROUNDS' || (echo "FAIL: stale PLAN references found" && exit 1)
    @echo "Checking git diff..."
    @git diff --check || true
    @echo "Lint OK"

# Install HELIX via DDx and verify
install:
    ddx install helix --force
    bash scripts/helix doctor --fix

# Show test count
count:
    @echo "CLI tests: $(grep -c '^run_test ' tests/helix-cli.sh)"
    @echo "Skills: $(ls .agents/skills/ | wc -l)"
