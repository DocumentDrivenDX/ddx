FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl ca-certificates jq \
    tmux \
    nodejs npm \
    python3 pipx \
    && rm -rf /var/lib/apt/lists/*

# Install asciinema
RUN pipx install asciinema
ENV PATH="/root/.local/bin:$PATH"

# Install Claude Code CLI
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
