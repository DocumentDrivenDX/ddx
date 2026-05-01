# DDX Development Environments Library

A collection of reproducible development environment configurations for AI-assisted development, ensuring consistent tooling and setups across teams and projects.

## Overview

The environments library provides pre-configured development environments using various technologies:
- **Docker**: Containerized environments for maximum portability
- **Vagrant**: Full virtual machines for complete isolation
- **Brew**: macOS-specific package bundles
- **Dev Containers**: VS Code development containers

## Available Environments

### Docker Environments

#### ai-development-base
Complete Docker environment with multiple language runtimes and AI SDKs.
- Languages: Python 3, Node.js 20, Go 1.21, Rust
- AI Tools: OpenAI, Anthropic, LangChain SDKs
- Databases: PostgreSQL, Redis, ChromaDB (optional)
- Tools: Jupyter, Git, GitHub CLI, Docker CLI

### Vagrant Environments

#### ai-development-vm
Full Ubuntu 22.04 VM with comprehensive AI development stack.
- Complete isolation from host system
- Pre-configured services and databases
- Shared folders with host
- Port forwarding for services

### Homebrew Environments

#### ai-tools-macos
Comprehensive Homebrew bundle for macOS developers.
- Development languages and runtimes
- AI/ML tools including Ollama for local LLMs
- Container and cloud tools
- Modern CLI utilities

## Quick Start

### Using with DDX CLI

```bash
# List available environments
ddx environments list

# Show environment details
ddx environments show ai-development-base

# Apply environment to current project
ddx environments apply ai-development-base --type docker
```

### Manual Usage

#### Docker Environment
```bash
cd library/environments/docker/ai-development-base
docker-compose up -d
docker-compose exec ai-dev bash
```

#### Vagrant Environment
```bash
cd library/environments/vagrant/ai-development-vm
vagrant up
vagrant ssh
```

#### Homebrew Environment (macOS)
```bash
cd library/environments/brew/ai-tools-macos
./setup.sh  # Or: brew bundle --file=Brewfile
```

## Environment Types

### Docker
- **Pros**: Lightweight, fast startup, easy sharing, cloud-compatible
- **Cons**: Limited OS-level access, requires Docker
- **Best for**: Microservices, API development, cloud-native apps

### Vagrant
- **Pros**: Full OS control, complete isolation, multiple OS support
- **Cons**: Resource intensive, slower startup, larger disk usage
- **Best for**: Complex setups, OS-specific development, testing

### Homebrew
- **Pros**: Native performance, direct OS integration, no virtualization
- **Cons**: macOS only, can affect system, harder to isolate
- **Best for**: macOS developers, native app development

### Dev Containers
- **Pros**: IDE integration, consistent dev experience, portable
- **Cons**: Requires VS Code, container overhead
- **Best for**: Team standardization, remote development

## Creating Custom Environments

### Environment Structure
```
library/environments/
├── <type>/                    # docker, vagrant, brew, devcontainer
│   └── <environment-name>/
│       ├── README.md          # Documentation (required)
│       ├── .env.example       # Environment template (optional)
│       └── <config-files>     # Type-specific files
```

### Docker Environment Template
```
my-environment/
├── Dockerfile
├── docker-compose.yml
├── .env.example
└── README.md
```

### Vagrant Environment Template
```
my-environment/
├── Vagrantfile
├── provision.sh
├── .env.example
└── README.md
```

### Homebrew Environment Template
```
my-environment/
├── Brewfile
├── setup.sh
└── README.md
```

## Best Practices

### 1. Security
- Never commit API keys or secrets
- Use .env.example templates
- Scan environments for vulnerabilities
- Keep base images/packages updated

### 2. Resource Management
- Set appropriate resource limits
- Document minimum requirements
- Provide lite alternatives
- Clean up unused resources

### 3. Documentation
- Include comprehensive README
- Document all prerequisites
- Provide troubleshooting section
- Include usage examples

### 4. Portability
- Use standard configurations
- Avoid hard-coded paths
- Support multiple platforms
- Test on different systems

### 5. Versioning
- Pin package versions for reproducibility
- Document breaking changes
- Maintain backwards compatibility
- Use semantic versioning

## Contributing Environments

### Submission Process

1. **Create your environment**:
   ```bash
   ddx environments create my-ai-env --type docker
   ```

2. **Test thoroughly**:
   - Test on clean system
   - Verify all tools work
   - Check resource usage
   - Test with real projects

3. **Document completely**:
   - Fill out README template
   - Include troubleshooting
   - Add examples
   - List prerequisites

4. **Submit contribution**:
   ```bash
   ddx environments contribute ./my-ai-env
   ```

### Quality Guidelines

- **Minimal**: Include only necessary tools
- **Secure**: No hardcoded credentials
- **Documented**: Clear setup and usage instructions
- **Tested**: Works on target platforms
- **Maintained**: Plan for updates

## Environment Variables

Common environment variables across all environments:

```bash
# AI Service Keys
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
LANGCHAIN_API_KEY=...
HUGGINGFACE_API_TOKEN=...

# Development Settings
NODE_ENV=development
ENVIRONMENT=development
DEBUG=true

# Database Connections
DATABASE_URL=postgresql://...
REDIS_URL=redis://...
MONGODB_URI=mongodb://...
```

## Comparison Matrix

| Feature | Docker | Vagrant | Homebrew | Dev Container |
|---------|--------|---------|----------|---------------|
| Startup Speed | Fast | Slow | N/A | Fast |
| Resource Usage | Low | High | None | Low |
| Isolation | Good | Excellent | None | Good |
| OS Support | All | All | macOS | All |
| IDE Integration | Basic | Basic | Native | Excellent |
| Cloud Ready | Yes | Limited | No | Yes |
| Persistence | Volume | VM Disk | System | Volume |

## Troubleshooting

### Common Issues

#### Docker: "Cannot connect to Docker daemon"
```bash
# Start Docker service
sudo systemctl start docker  # Linux
open -a Docker              # macOS
```

#### Vagrant: "VT-x is not available"
- Enable virtualization in BIOS/UEFI
- Check Hyper-V conflicts on Windows

#### Homebrew: "Permission denied"
```bash
# Fix Homebrew permissions
sudo chown -R $(whoami) /opt/homebrew  # Apple Silicon
sudo chown -R $(whoami) /usr/local     # Intel
```

### Getting Help

1. Check environment-specific README
2. Review DDX documentation
3. Search existing issues
4. Ask in community forums
5. Submit bug report

## Maintenance

Environments are regularly updated for:
- Security patches
- New tool versions
- Bug fixes
- Performance improvements

Check for updates:
```bash
ddx environments check-updates
```

## License

All environments in this library are part of the DDX toolkit and follow the same license terms. Individual tools and software installed by these environments retain their original licenses.

## See Also

- [DDX Documentation](../../docs/README.md)
- [Feature Specification](../../docs/helix/01-frame/features/FEAT-013-environment-assets.md)
- [Contributing Guide](../../CONTRIBUTING.md)