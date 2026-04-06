#!/bin/bash
# Build and run demo Docker container to regenerate casts and website
# Usage: ./run-docker-demos.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Build the demo image
echo "Building demo Docker image..."
docker build -t ddx-demos -f "$SCRIPT_DIR/Dockerfile" "$SCRIPT_DIR"

# Build ddx binary first (we need it in the container)
echo "Building ddx binary..."
(cd "$PROJECT_ROOT/cli" && go build -o "$PROJECT_ROOT/bin/ddx" .)

# Ensure output directory exists
mkdir -p "$PROJECT_ROOT/website/static/demos"

# Create a temp HOME inside project so volume permissions work
DEMO_HOME="$PROJECT_ROOT/.demo-home"
mkdir -p "$DEMO_HOME/.local/bin"
cp "$PROJECT_ROOT/bin/ddx" "$DEMO_HOME/.local/bin/ddx"

# Run the demos in Docker as the current user
echo "Running demo recordings in Docker..."
docker run --rm \
    --user "$(id -u):$(id -g)" \
    -v "$PROJECT_ROOT:/workspace" \
    -v "$DEMO_HOME:/home/demo" \
    -e PATH="/home/demo/.local/bin:/usr/local/bin:/usr/bin:/bin" \
    -e HOME="/home/demo" \
    -e GIT_TEMPLATE_DIR="" \
    -e DDX_INSTALL_SH="/workspace/install.sh" \
    -w /workspace \
    ddx-demos \
    bash -c '
        # Record each demo (01-05)
        for script in scripts/demos/0[1-5]*.sh; do
            name=$(basename "$script" .sh)
            echo "Recording $name..."
            asciinema rec --overwrite --cols 80 --rows 24 --command "bash $script" "website/static/demos/${name}.cast" || true
        done

        # Record quickstart (wider terminal)
        echo "Recording 07-quickstart..."
        asciinema rec --overwrite --cols 100 --rows 30 --command "bash scripts/demos/07-quickstart.sh" "website/static/demos/07-quickstart.cast" || true

        # Render GIFs
        for cast in website/static/demos/*.cast; do
            echo "Rendering $(basename "$cast" .cast).gif..."
            agg "$cast" "${cast%.cast}.gif" || true
        done

        echo "Done! Demo files updated in website/static/demos/"
    '

# Build the website in Docker (pinned Hugo version, reproducible)
echo "Building website in Docker..."
docker run --rm \
    --user "$(id -u):$(id -g)" \
    -v "$PROJECT_ROOT:/workspace" \
    -v "$DEMO_HOME:/home/demo" \
    -e HOME="/home/demo" \
    -w /workspace/website \
    ddx-demos \
    hugo --gc --minify

# Clean up temp home
rm -rf "$DEMO_HOME"

echo ""
echo "Demo recordings regenerated. Files in website/static/demos/"
ls -la "$PROJECT_ROOT/website/static/demos/"
echo ""
echo "Website built in website/public/"
