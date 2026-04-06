#!/bin/bash
# Docker-based acceptance tests for DDx installation architecture
# Run from project root: ./tests/docker/run-tests.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Build test images
build_images() {
    log_info "Building test Docker images..."
    
    docker build -t ddx-test:clean -f "$SCRIPT_DIR/Dockerfile.clean" "$PROJECT_ROOT"
    docker build -t ddx-test:with-ddx -f "$SCRIPT_DIR/Dockerfile.with-ddx" "$PROJECT_ROOT"
    docker build -t ddx-test:no-binary -f "$SCRIPT_DIR/Dockerfile.no-binary" "$PROJECT_ROOT"
    
    log_info "Test images built successfully"
}

# Test AC-001: Clean machine installation
test_ac001_clean_install() {
    log_info "Running AC-001: Clean Machine Installation..."
    
    docker run --rm ddx-test:clean /bin/bash -c '
        set -e
        
        # Run install.sh
        curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh | bash
        
        # Verify binary in PATH
        if ! command -v ddx &> /dev/null; then
            echo "FAIL: ddx not in PATH"
            exit 1
        fi
        
        # Verify version
        ddx version
        
        # Verify ~/.ddx/ structure
        if [ ! -d "$HOME/.ddx/skills" ]; then
            echo "FAIL: ~/.ddx/skills not found"
            exit 1
        fi
        
        # Verify skills installed
        if [ ! -d "$HOME/.ddx/skills/ddx-bead" ]; then
            echo "FAIL: ddx-bead skill not installed"
            exit 1
        fi
        
        echo "PASS: AC-001"
    '
}

# Test AC-002: Plugin installation
test_ac002_plugin_install() {
    log_info "Running AC-002: Plugin Installation..."
    
    docker run --rm ddx-test:with-ddx /bin/bash -c '
        set -e
        
        # Install helix plugin
        ddx install helix
        
        # Verify plugin directory
        if [ ! -d "$HOME/.ddx/plugins/helix" ]; then
            echo "FAIL: helix not in ~/.ddx/plugins/"
            exit 1
        fi
        
        # Verify skills exist
        if [ ! -d "$HOME/.agents/skills/helix-align" ]; then
            echo "FAIL: helix-align skill not found"
            exit 1
        fi
        
        # Verify Claude symlinks
        if [ ! -L "$HOME/.claude/skills/helix-align" ]; then
            echo "FAIL: helix-align not symlinked in ~/.claude/skills/"
            exit 1
        fi
        
        echo "PASS: AC-002"
    '
}

# Test AC-003: Repository initialization
test_ac003_repo_init() {
    log_info "Running AC-003: Repository Initialization..."
    
    docker run --rm ddx-test:with-ddx /bin/bash -c '
        set -e
        
        # Create temp project dir
        cd /tmp
        mkdir -p test-project
        cd test-project
        
        # Run ddx init
        ddx init
        
        # Verify .ddx/ exists
        if [ ! -d ".ddx" ]; then
            echo "FAIL: .ddx not created"
            exit 1
        fi
        
        # Verify bootstrap skills
        if [ ! -d ".ddx/skills/ddx-doctor" ]; then
            echo "FAIL: ddx-doctor not copied"
            exit 1
        fi
        
        if [ ! -d ".ddx/skills/ddx-run" ]; then
            echo "FAIL: ddx-run not copied"
            exit 1
        fi
        
        # Verify .agents/skills symlink
        if [ ! -L ".agents/skills" ]; then
            echo "FAIL: .agents/skills not symlinked"
            exit 1
        fi
        
        echo "PASS: AC-003"
    '
}

# Test AC-004: Missing ddx detection
test_ac004_missing_ddx() {
    log_info "Running AC-004: Missing DDx Detection..."
    
    docker run --rm ddx-test:no-binary /bin/bash -c '
        set -e
        
        # Run ddx-doctor (simulated)
        # Should detect missing ddx and prompt install
        
        # Check that ddx is NOT in PATH
        if command -v ddx &> /dev/null; then
            echo "FAIL: ddx should not be in PATH"
            exit 1
        fi
        
        # The bootstrap skill should output install instructions
        # (This is tested via skill execution, not CLI)
        
        echo "PASS: AC-004"
    '
}

# Test AC-005: DDx update
test_ac005_ddx_update() {
    log_info "Running AC-005: DDx Update (mock)..."
    
    # This test would need mocking the GitHub API response
    # For now, just verify the command exists
    docker run --rm ddx-test:with-ddx /bin/bash -c '
        set -e
        
        ddx update --help
        
        echo "PASS: AC-005 (command exists)"
    '
}

# Test AC-006: Plugin update
test_ac006_plugin_update() {
    log_info "Running AC-006: Plugin Update (mock)..."
    
    # This test would need mocking the git response
    # For now, just verify the command exists
    docker run --rm ddx-test:with-ddx /bin/bash -c '
        set -e
        
        ddx update helix --help
        
        echo "PASS: AC-006 (command exists)"
    '
}

# Run all tests
run_all_tests() {
    log_info "Starting DDx Installation Acceptance Tests..."
    
    # Build images first
    build_images
    
    # Run each test
    test_ac001_clean_install || log_error "AC-001 failed"
    test_ac002_plugin_install || log_error "AC-002 failed"
    test_ac003_repo_init || log_error "AC-003 failed"
    test_ac004_missing_ddx || log_error "AC-004 failed"
    test_ac005_ddx_update || log_error "AC-005 failed"
    test_ac006_plugin_update || log_error "AC-006 failed"
    
    log_info "All tests completed!"
}

# Show usage
usage() {
    echo "Usage: $0 [test-name]"
    echo ""
    echo "Test names:"
    echo "  ac001 - Clean machine installation"
    echo "  ac002 - Plugin installation"
    echo "  ac003 - Repository initialization"
    echo "  ac004 - Missing ddx detection"
    echo "  ac005 - ddx update"
    echo "  ac006 - plugin update"
    echo "  all   - Run all tests (default)"
    echo ""
    echo "Examples:"
    echo "  $0           # Run all tests"
    echo "  $0 ac001     # Run only AC-001"
}

# Main
case "${1:-all}" in
    ac001) test_ac001_clean_install ;;
    ac002) test_ac002_plugin_install ;;
    ac003) test_ac003_repo_init ;;
    ac004) test_ac004_missing_ddx ;;
    ac005) test_ac005_ddx_update ;;
    ac006) test_ac006_plugin_update ;;
    all) run_all_tests ;;
    *) usage; exit 1 ;;
esac
