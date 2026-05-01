# AI Tools for macOS - Homebrew Environment

A comprehensive Homebrew-based setup for AI development on macOS, including all essential tools, languages, and AI SDKs.

## Features

- **Complete Development Stack**: Python, Node.js, Go, Rust, Java
- **AI/ML Tools**: TensorFlow, PyTorch, Ollama for local LLMs
- **Container Support**: Docker, Colima, Kubernetes tools
- **Database Tools**: PostgreSQL, Redis, MongoDB, MySQL
- **Cloud CLIs**: AWS, Azure, Google Cloud
- **AI-Powered Tools**: Cursor editor, Warp terminal
- **Development Utilities**: Modern CLI tools and productivity enhancers

## Prerequisites

- macOS 11.0 or later
- Administrator access
- ~20GB free disk space
- Internet connection for downloads

## Installation

### Option 1: Automated Setup (Recommended)

Run the complete setup script:

```bash
./setup.sh
```

This will:
1. Install Homebrew (if needed)
2. Install all packages from Brewfile
3. Configure Python, Node.js, and Go environments
4. Install AI/ML packages
5. Set up shell aliases and functions
6. Create development directories

### Option 2: Manual Installation

Install only the packages you need:

```bash
# Install Homebrew (if not installed)
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Install from Brewfile
brew bundle --file=Brewfile

# Or install specific categories:
brew install python@3.11 node@20 go rust  # Languages
brew install docker colima kubectl        # Containers
brew install postgresql@15 redis          # Databases
```

### Option 3: Selective Installation

Edit the Brewfile to comment out unwanted packages, then:

```bash
brew bundle --file=Brewfile
```

## Package Categories

### Core Development
- Git, GitHub CLI, GitLab CLI
- Build tools (make, cmake, pkg-config)
- Package managers (pip, npm, cargo)

### Languages & Runtimes
- Python 3.11 with pyenv
- Node.js 20 with nvm
- Go (latest)
- Rust (latest)
- Java, Ruby

### AI/ML Specific
- Jupyter notebooks
- TensorFlow, PyTorch
- Ollama for running LLMs locally
- Python: openai, anthropic, langchain
- Node.js: AI SDK packages

### Databases
- PostgreSQL 15
- Redis
- MongoDB
- MySQL
- SQLite

### Container & Cloud
- Docker & Docker Compose
- Colima (Docker Desktop alternative)
- Kubernetes tools (kubectl, helm, k9s)
- AWS CLI, Azure CLI, Google Cloud SDK
- Terraform, Pulumi

### Developer Tools
- VS Code, Cursor (AI editor)
- iTerm2, Warp (AI terminal)
- Postman, Insomnia
- Database clients (TablePlus, DBeaver)
- JetBrains Toolbox

### Productivity Tools
- tmux, neovim
- fzf (fuzzy finder)
- ripgrep (fast grep)
- bat (better cat)
- jq/yq (JSON/YAML processing)

## Post-Installation

### 1. Configure Git
```bash
git config --global user.name "Your Name"
git config --global user.email "your.email@example.com"
```

### 2. Set Up API Keys
Copy the environment template and add your keys:
```bash
cp ~/.env.template .env
# Edit .env with your API keys
```

### 3. Start Services
```bash
# Start database services
brew services start postgresql@15
brew services start redis

# Start Colima for Docker
colima start --cpu 4 --memory 8

# Start Ollama for local LLMs
brew services start ollama
```

### 4. Pull AI Models (Optional)
```bash
# Download local LLM models
ollama pull llama2
ollama pull codellama
ollama pull mistral
```

## Shell Functions

The setup adds helpful functions to your shell:

### `ai-env`
Loads environment variables from .env file:
```bash
cd your-project
ai-env  # Loads .env variables
```

### Aliases
- `ll` - List files with details
- `gs` - Git status
- `gd` - Git diff
- `gc` - Git commit
- `gp` - Git push
- `k` - kubectl shortcut
- `docker-clean` - Clean Docker system

## Verification

Test your installation:

```bash
# Check language versions
python --version
node --version
go version
rustc --version

# Check tools
docker --version
kubectl version --client
gh --version

# Test Python packages
python -c "import openai; print('OpenAI SDK installed')"
python -c "import anthropic; print('Anthropic SDK installed')"

# Test Node packages
npm list -g @anthropic-ai/sdk
```

## Troubleshooting

### Homebrew Issues
```bash
# Update Homebrew
brew update
brew upgrade

# Fix permissions
sudo chown -R $(whoami) /opt/homebrew  # Apple Silicon
sudo chown -R $(whoami) /usr/local     # Intel
```

### Python Issues
```bash
# Reinstall Python
pyenv install 3.11
pyenv global 3.11

# Fix pip
python -m pip install --upgrade pip
```

### Docker/Colima Issues
```bash
# Reset Colima
colima delete
colima start --cpu 4 --memory 8

# Check status
colima status
docker ps
```

### Port Conflicts
```bash
# Find process using port
lsof -i :5432  # PostgreSQL
lsof -i :6379  # Redis

# Stop services
brew services stop postgresql@15
brew services stop redis
```

## Customization

### Adding Packages
Edit Brewfile and run:
```bash
brew bundle --file=Brewfile
```

### Removing Packages
```bash
brew uninstall package-name
brew autoremove  # Remove dependencies
```

### Alternative Versions
```bash
# Use different Python version
pyenv install 3.12
pyenv local 3.12

# Use different Node version
nvm install 18
nvm use 18
```

## Maintenance

### Regular Updates
```bash
# Update everything
brew update && brew upgrade
pip install --upgrade pip
npm update -g
```

### Cleanup
```bash
# Clean Homebrew cache
brew cleanup

# Clean Docker
docker system prune -a

# Clean npm cache
npm cache clean --force
```

## Security Notes

- Store API keys in .env files (never commit them)
- Use `direnv` for project-specific environments
- Keep tools updated for security patches
- Use `pass` or macOS Keychain for sensitive data

## Contributing

To improve this environment configuration:
1. Test changes thoroughly on macOS
2. Document any new dependencies
3. Submit via `ddx environments contribute`

## License

Part of the DDx toolkit - see main project license.