# AI Development Base Environment
# Provides a comprehensive development environment for AI-assisted development
# with pre-installed tools, SDKs, and configurations

FROM ubuntu:22.04

# Prevent interactive prompts during installation
ENV DEBIAN_FRONTEND=noninteractive
ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8

# Install system dependencies
RUN apt-get update && apt-get install -y \
    # Development essentials
    build-essential \
    git \
    curl \
    wget \
    vim \
    nano \
    tmux \
    htop \
    # Python development
    python3 \
    python3-pip \
    python3-dev \
    python3-venv \
    # Node.js prerequisites
    ca-certificates \
    gnupg \
    lsb-release \
    # Network tools
    net-tools \
    iputils-ping \
    dnsutils \
    # Compression tools
    zip \
    unzip \
    tar \
    gzip \
    # Other utilities
    jq \
    tree \
    make \
    openssh-client \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Install Node.js 20.x
RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt-get install -y nodejs \
    && npm install -g npm@latest

# Install Go 1.21
RUN wget -q https://go.dev/dl/go1.21.6.linux-amd64.tar.gz \
    && tar -C /usr/local -xzf go1.21.6.linux-amd64.tar.gz \
    && rm go1.21.6.linux-amd64.tar.gz

ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/go"
ENV PATH="${GOPATH}/bin:${PATH}"

# Install Rust
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/root/.cargo/bin:${PATH}"

# Install Docker CLI (for Docker-in-Docker support)
RUN install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg \
    && chmod a+r /etc/apt/keyrings/docker.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null \
    && apt-get update \
    && apt-get install -y docker-ce-cli docker-compose-plugin \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Install Python AI/ML packages
RUN pip3 install --upgrade pip setuptools wheel \
    && pip3 install \
    # AI/ML essentials
    numpy \
    pandas \
    scikit-learn \
    matplotlib \
    jupyter \
    notebook \
    # API clients
    openai \
    anthropic \
    langchain \
    chromadb \
    # Development tools
    ipython \
    black \
    pylint \
    pytest \
    requests \
    python-dotenv \
    pydantic

# Install Node.js AI development packages globally
RUN npm install -g \
    typescript \
    ts-node \
    nodemon \
    prettier \
    eslint \
    @anthropic-ai/sdk \
    openai \
    langchain

# Install Go development tools
RUN go install golang.org/x/tools/gopls@latest \
    && go install github.com/go-delve/delve/cmd/dlv@latest \
    && go install golang.org/x/lint/golint@latest \
    && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install GitHub CLI
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | gpg --dearmor -o /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update \
    && apt-get install -y gh \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Install DDx CLI (placeholder - will be replaced with actual installation)
# RUN curl -fsSL https://raw.githubusercontent.com/yourusername/ddx/main/install.sh | sh

# Create workspace directory
WORKDIR /workspace

# Add helper scripts
COPY <<'EOF' /usr/local/bin/ai-env
#!/bin/bash
# Helper script to set up AI API keys from environment or .env file
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi
echo "AI Development Environment Ready"
echo "Available tools: python3, node, go, rust, docker"
echo "AI SDKs: OpenAI, Anthropic, LangChain"
exec "$@"
EOF

RUN chmod +x /usr/local/bin/ai-env

# Set up shell configuration
RUN echo 'export PS1="\[\033[1;34m\]ai-dev\[\033[0m\]:\[\033[1;36m\]\w\[\033[0m\]$ "' >> /root/.bashrc \
    && echo 'alias ll="ls -la"' >> /root/.bashrc \
    && echo 'alias gs="git status"' >> /root/.bashrc \
    && echo 'alias gd="git diff"' >> /root/.bashrc \
    && echo 'alias gc="git commit"' >> /root/.bashrc \
    && echo 'alias gp="git push"' >> /root/.bashrc

# Default command
ENTRYPOINT ["/usr/local/bin/ai-env"]
CMD ["/bin/bash"]