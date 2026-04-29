FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    ca-certificates \
    jq \
    unzip \
    python3 \
    pipx \
    nodejs \
    npm \
    && rm -rf /var/lib/apt/lists/*

# Install Bun
RUN curl -fsSL https://bun.sh/install | bash
ENV PATH="/root/.bun/bin:$PATH"

# Install asciinema
RUN pipx install asciinema
ENV PATH="/root/.local/bin:$PATH"

# Install Claude Code CLI (via npm — Claude CLI is a Node.js tool)
RUN npm install -g @anthropic-ai/claude-code

# Git identity
RUN git config --global user.name "HELIX Demo" \
    && git config --global user.email "demo@helix.dev" \
    && git config --global init.defaultBranch main

ENV SHELL=/bin/bash TERM=xterm-256color

WORKDIR /workspace

COPY demo.sh /usr/local/bin/demo.sh
RUN chmod +x /usr/local/bin/demo.sh

ENTRYPOINT ["/usr/local/bin/demo.sh"]
