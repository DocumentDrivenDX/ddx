FROM ubuntu:24.04

# System dependencies + Node.js
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    ca-certificates \
    jq \
    bc \
    python3 \
    pipx \
    nodejs \
    npm \
    && rm -rf /var/lib/apt/lists/*

# Install asciinema
RUN pipx install asciinema
ENV PATH="/root/.local/bin:$PATH"

# Install Claude Code CLI
RUN npm install -g @anthropic-ai/claude-code

# Git identity for commits inside the container
RUN git config --global user.name "HELIX Demo" \
    && git config --global user.email "demo@helix.dev" \
    && git config --global init.defaultBranch main

# Env for asciinema recording metadata
ENV SHELL=/bin/bash TERM=xterm-256color

# Working directory for the demo project
WORKDIR /workspace

# Copy the demo script
COPY demo.sh /usr/local/bin/demo.sh
RUN chmod +x /usr/local/bin/demo.sh

# Mount points:
#   /root/.claude.json — Claude auth token (bind-mount, read-only)
#   /root/.claude      — Claude session state (bind-mount, writable)
#   /helix             — helix repo (bind-mount, read-only)
#   /ddx               — ddx binary (bind-mount, optional)
#   /workspace         — demo project workspace (ephemeral)
#   /recordings        — asciinema output (bind-mount for extraction)

ENTRYPOINT ["/usr/local/bin/demo.sh"]
