#!/usr/bin/env bash
# AI Development Environment Setup for macOS
# This script installs and configures development tools for AI-assisted development

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if running on macOS
if [[ "$OSTYPE" != "darwin"* ]]; then
    log_error "This script is designed for macOS only"
    exit 1
fi

# Check if Homebrew is installed
if ! command -v brew &> /dev/null; then
    log_info "Homebrew not found. Installing Homebrew..."
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

    # Add Homebrew to PATH for Apple Silicon Macs
    if [[ -f "/opt/homebrew/bin/brew" ]]; then
        echo 'eval "$(/opt/homebrew/bin/brew shellenv)"' >> ~/.zprofile
        eval "$(/opt/homebrew/bin/brew shellenv)"
    fi
fi

# Update Homebrew
log_info "Updating Homebrew..."
brew update

# Install from Brewfile
log_info "Installing packages from Brewfile..."
brew bundle --file=Brewfile

# Configure Python environment
log_info "Configuring Python environment..."
if command -v pyenv &> /dev/null; then
    # Install Python 3.11 if not already installed
    if ! pyenv versions | grep -q "3.11"; then
        log_info "Installing Python 3.11 via pyenv..."
        pyenv install 3.11
    fi
    pyenv global 3.11

    # Add pyenv to shell configuration
    if ! grep -q "pyenv init" ~/.zshrc; then
        echo 'export PYENV_ROOT="$HOME/.pyenv"' >> ~/.zshrc
        echo 'command -v pyenv >/dev/null || export PATH="$PYENV_ROOT/bin:$PATH"' >> ~/.zshrc
        echo 'eval "$(pyenv init -)"' >> ~/.zshrc
    fi
fi

# Install Python AI packages
log_info "Installing Python AI packages..."
pip3 install --upgrade pip
pip3 install --user \
    openai \
    anthropic \
    langchain \
    chromadb \
    jupyter \
    notebook \
    ipython \
    numpy \
    pandas \
    scikit-learn \
    matplotlib \
    python-dotenv \
    black \
    pylint \
    pytest

# Configure Node.js environment
log_info "Configuring Node.js environment..."
if command -v nvm &> /dev/null; then
    # Add nvm to shell configuration
    if ! grep -q "NVM_DIR" ~/.zshrc; then
        echo 'export NVM_DIR="$HOME/.nvm"' >> ~/.zshrc
        echo '[ -s "/opt/homebrew/opt/nvm/nvm.sh" ] && \. "/opt/homebrew/opt/nvm/nvm.sh"' >> ~/.zshrc
    fi

    # Load nvm for current session
    export NVM_DIR="$HOME/.nvm"
    [ -s "/opt/homebrew/opt/nvm/nvm.sh" ] && \. "/opt/homebrew/opt/nvm/nvm.sh"

    # Install Node.js 20
    nvm install 20
    nvm use 20
    nvm alias default 20
fi

# Install global npm packages
log_info "Installing global npm packages..."
npm install -g \
    typescript \
    ts-node \
    nodemon \
    prettier \
    eslint \
    @anthropic-ai/sdk \
    openai \
    langchain

# Configure Go environment
log_info "Configuring Go environment..."
if command -v go &> /dev/null; then
    # Set up Go workspace
    mkdir -p ~/go/{bin,src,pkg}

    # Add Go to PATH if not already there
    if ! grep -q "GOPATH" ~/.zshrc; then
        echo 'export GOPATH=$HOME/go' >> ~/.zshrc
        echo 'export PATH=$PATH:$GOPATH/bin' >> ~/.zshrc
    fi

    # Install Go tools
    go install golang.org/x/tools/gopls@latest
    go install github.com/go-delve/delve/cmd/dlv@latest
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
fi

# Configure Docker/Colima
log_info "Configuring container runtime..."
if command -v colima &> /dev/null; then
    # Start Colima if not running
    if ! colima status &> /dev/null; then
        log_info "Starting Colima..."
        colima start --cpu 4 --memory 8 --disk 100
    fi
fi

# Set up Ollama for local LLMs
log_info "Setting up Ollama for local LLMs..."
if command -v ollama &> /dev/null; then
    # Start Ollama service
    brew services start ollama

    # Pull some popular models (optional - comment out if not needed)
    log_info "Pulling Ollama models (this may take a while)..."
    # ollama pull llama2
    # ollama pull codellama
    # ollama pull mistral
fi

# Create development directories
log_info "Creating development directories..."
mkdir -p ~/Development/{projects,scripts,configs}

# Configure Git (if not already configured)
if [ -z "$(git config --global user.email)" ]; then
    log_warn "Git user email not configured. Please run:"
    echo "  git config --global user.email 'your.email@example.com'"
fi

if [ -z "$(git config --global user.name)" ]; then
    log_warn "Git user name not configured. Please run:"
    echo "  git config --global user.name 'Your Name'"
fi

# Set up shell aliases
log_info "Setting up shell aliases..."
if ! grep -q "# AI Development Aliases" ~/.zshrc; then
    cat >> ~/.zshrc << 'EOL'

# AI Development Aliases
alias python=python3
alias pip=pip3
alias ll='ls -la'
alias gs='git status'
alias gd='git diff'
alias gc='git commit'
alias gp='git push'
alias docker-clean='docker system prune -a'
alias k=kubectl

# AI Development Functions
ai-env() {
    if [ -f .env ]; then
        export $(cat .env | grep -v '^#' | xargs)
        echo "Environment variables loaded from .env"
    else
        echo "No .env file found"
    fi
}
EOL
fi

# Create sample .env template
log_info "Creating sample .env template..."
cat > ~/.env.template << 'EOL'
# AI Service API Keys
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
LANGCHAIN_API_KEY=
HUGGINGFACE_API_TOKEN=

# GitHub Configuration
GITHUB_TOKEN=ghp_...
GITHUB_USER=
GITHUB_EMAIL=

# Database URLs
DATABASE_URL=postgresql://localhost/myapp
REDIS_URL=redis://localhost:6379

# Environment
NODE_ENV=development
ENVIRONMENT=development
EOL

# Final instructions
echo ""
log_info "=== Setup Complete ==="
echo ""
echo "Next steps:"
echo "1. Restart your terminal or run: source ~/.zshrc"
echo "2. Copy ~/.env.template to your project and configure API keys"
echo "3. Test the installation:"
echo "   - Python: python --version"
echo "   - Node.js: node --version"
echo "   - Go: go version"
echo "   - Docker: docker --version"
echo ""
echo "Optional:"
echo "- Start PostgreSQL: brew services start postgresql@15"
echo "- Start Redis: brew services start redis"
echo "- Pull Ollama models: ollama pull llama2"
echo ""
log_info "Happy coding!"