# HELIX development tasks

# Run all tests
test: test-cli test-skills

# Run CLI wrapper tests
test-cli:
    bash tests/helix-cli.sh

# Run skill package validation
test-skills:
    bash tests/validate-skills.sh

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
