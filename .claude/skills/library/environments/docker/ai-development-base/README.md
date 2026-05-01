# AI Development Base Environment

A comprehensive Docker-based development environment pre-configured for AI-assisted development with multiple language runtimes and AI SDKs.

## Features

- **Multiple Language Support**: Python 3, Node.js 20, Go 1.21, Rust
- **AI SDKs Pre-installed**: OpenAI, Anthropic, LangChain, ChromaDB
- **Development Tools**: Git, GitHub CLI, Docker CLI, vim, tmux
- **Optional Services**: PostgreSQL, Redis, ChromaDB (via profiles)
- **Volume Mounts**: Workspace, Git config, SSH keys
- **Environment Management**: Automatic .env file loading

## Quick Start

1. **Copy environment template**:
   ```bash
   cp .env.example .env
   # Edit .env with your API keys
   ```

2. **Build and start the environment**:
   ```bash
   # Basic environment only
   docker-compose up -d

   # With database
   docker-compose --profile with-db up -d

   # With all optional services
   docker-compose --profile with-db --profile with-cache --profile with-vectordb up -d
   ```

3. **Enter the development container**:
   ```bash
   docker-compose exec ai-dev bash
   ```

4. **Stop the environment**:
   ```bash
   docker-compose down
   ```

## Included Tools

### Languages & Runtimes
- Python 3.x with pip
- Node.js 20.x with npm
- Go 1.21
- Rust (latest stable)

### Python Packages
- AI/ML: numpy, pandas, scikit-learn, matplotlib
- AI APIs: openai, anthropic, langchain, chromadb
- Dev Tools: jupyter, ipython, black, pylint, pytest

### Node.js Packages
- TypeScript, ts-node
- AI SDKs: @anthropic-ai/sdk, openai, langchain
- Dev Tools: nodemon, prettier, eslint

### Go Tools
- gopls (Language Server)
- delve (Debugger)
- golint, golangci-lint

## Configuration

### Environment Variables
Set these in your `.env` file:
- `OPENAI_API_KEY`: OpenAI API key
- `ANTHROPIC_API_KEY`: Anthropic Claude API key
- `GITHUB_TOKEN`: GitHub personal access token
- `LANGCHAIN_API_KEY`: LangChain API key (optional)

### Volume Mounts
- `./`: Mounted to `/workspace` in container
- `~/.gitconfig`: Git configuration (read-only)
- `~/.ssh`: SSH keys (read-only)
- `/var/run/docker.sock`: Docker socket for Docker-in-Docker

### Resource Limits
Default limits (adjustable in docker-compose.yml):
- CPUs: 4 (reserved: 2)
- Memory: 8GB (reserved: 4GB)

## Optional Services

### PostgreSQL Database
Enable with `--profile with-db`:
- Version: PostgreSQL 15
- Database: ai_development
- User: developer
- Password: Set in .env

### Redis Cache
Enable with `--profile with-cache`:
- Version: Redis 7
- Persistence: Enabled (AOF)

### ChromaDB Vector Store
Enable with `--profile with-vectordb`:
- Port: 8000
- Persistence: Enabled

## Usage Examples

### Python AI Development
```bash
docker-compose exec ai-dev bash
cd /workspace
python3 -m venv venv
source venv/bin/activate
pip install -r requirements.txt
python your_ai_script.py
```

### Node.js AI Development
```bash
docker-compose exec ai-dev bash
cd /workspace
npm install
npm run dev
```

### Jupyter Notebook
```bash
docker-compose exec ai-dev bash
cd /workspace
jupyter notebook --ip=0.0.0.0 --allow-root
```

## Customization

### Adding More Tools
1. Create a `Dockerfile.custom` extending this base:
   ```dockerfile
   FROM ddx/ai-development-base:latest
   RUN apt-get update && apt-get install -y your-tools
   ```

2. Update docker-compose.yml to use custom Dockerfile

### Persistent Data
Add named volumes for persistent storage:
```yaml
volumes:
  - workspace_data:/workspace/data
```

## Troubleshooting

### Permission Issues
If you encounter permission issues with mounted volumes:
```bash
# Inside container
chmod -R 755 /workspace
```

### API Key Issues
Verify environment variables are loaded:
```bash
docker-compose exec ai-dev env | grep API_KEY
```

### Resource Constraints
Adjust limits in docker-compose.yml based on your system:
```yaml
deploy:
  resources:
    limits:
      cpus: '2'  # Reduce if needed
      memory: 4G  # Reduce if needed
```

## Contributing

To contribute improvements to this environment:
1. Fork and modify the configuration
2. Test thoroughly
3. Submit via `ddx environments contribute`

## License

This environment configuration is part of the DDx toolkit and follows the same license terms.