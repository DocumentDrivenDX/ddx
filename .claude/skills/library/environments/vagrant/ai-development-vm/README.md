# AI Development Virtual Machine

A complete, reproducible virtual machine environment for AI development using Vagrant, providing consistent tooling across different host operating systems.

## Features

- **Ubuntu 22.04 LTS** base system
- **Complete Language Stack**: Python 3.11, Node.js 20, Go 1.21, Rust
- **AI/ML Frameworks**: TensorFlow, PyTorch, Transformers, LangChain
- **Databases**: PostgreSQL, Redis, MongoDB
- **Container Support**: Docker and Docker Compose
- **Cloud Tools**: kubectl, Terraform, GitHub CLI
- **Pre-configured Jupyter**: Ready-to-use notebook server
- **Shared Folders**: Seamless file sharing with host
- **Port Forwarding**: Access services from host machine

## Prerequisites

- [Vagrant](https://www.vagrantup.com/downloads) (2.3.0 or later)
- Virtualization provider:
  - [VirtualBox](https://www.virtualbox.org/wiki/Downloads) (recommended)
  - VMware Fusion/Workstation
  - Parallels Desktop
- 20GB free disk space
- 8GB RAM available for VM

## Quick Start

1. **Clone or download this environment**:
   ```bash
   ddx environments apply ai-development-vm
   cd ai-development-vm
   ```

2. **Configure environment variables** (optional):
   ```bash
   cp .env.example .env
   # Edit .env with your API keys
   ```

3. **Start the VM**:
   ```bash
   vagrant up
   ```

4. **Access the VM**:
   ```bash
   vagrant ssh
   ```

5. **Access services from host**:
   - Jupyter: http://localhost:8888 (token: `vagrant`)
   - Node.js apps: http://localhost:3000
   - Python apps: http://localhost:5000
   - Django/FastAPI: http://localhost:8000

## VM Specifications

### Resources
- **Memory**: 8GB (configurable in Vagrantfile)
- **CPUs**: 4 cores (configurable in Vagrantfile)
- **Disk**: 40GB (auto-expanding)
- **Network**: Private network + NAT

### Port Mappings
| Service | Guest Port | Host Port | Description |
|---------|------------|-----------|-------------|
| Jupyter | 8888 | 8888 | Jupyter Notebook/Lab |
| Node.js | 3000 | 3000 | Node.js applications |
| Python | 5000 | 5000 | Flask/FastAPI apps |
| Django | 8000 | 8000 | Django/ChromaDB |
| PostgreSQL | 5432 | 5432 | Database |
| Redis | 6379 | 6379 | Cache/Queue |

### Shared Folders
- `/vagrant` - Current directory (where Vagrantfile is)
- `/home/vagrant/Development` - Maps to `~/Development` on host

## Installed Software

### Programming Languages
- Python 3.11 with pip
- Node.js 20 with npm
- Go 1.21
- Rust (latest stable)

### Python Packages
- **AI/ML**: openai, anthropic, langchain, transformers
- **ML Frameworks**: tensorflow, torch, scikit-learn
- **Data Science**: numpy, pandas, matplotlib, jupyter
- **Web**: fastapi, flask, django, uvicorn
- **Testing**: pytest, black, pylint, mypy

### Node.js Packages
- **AI SDKs**: @anthropic-ai/sdk, openai, langchain
- **Development**: typescript, ts-node, nodemon, pm2
- **Frameworks**: express, fastify, next
- **Tools**: prettier, eslint

### Databases
- PostgreSQL 15
- Redis 7
- MongoDB 7

### DevOps Tools
- Docker & Docker Compose
- Kubernetes CLI (kubectl)
- Terraform
- GitHub CLI

## Usage

### Basic Commands

```bash
# Start VM
vagrant up

# SSH into VM
vagrant ssh

# Stop VM (preserves state)
vagrant halt

# Suspend VM (saves exact state)
vagrant suspend

# Resume suspended VM
vagrant resume

# Restart VM
vagrant reload

# Update VM configuration
vagrant reload --provision

# Destroy VM (removes all data)
vagrant destroy
```

### Working with Jupyter

```bash
# Start Jupyter (auto-starts on boot)
sudo systemctl start jupyter

# Check Jupyter status
sudo systemctl status jupyter

# Access from host browser
# http://localhost:8888
# Token: vagrant

# Custom Jupyter start
vagrant ssh -c "cd /vagrant && jupyter lab --ip=0.0.0.0"
```

### Using Databases

```bash
# PostgreSQL
psql -U vagrant -d vagrant

# Redis
redis-cli -a vagrant

# MongoDB
mongosh
```

### Docker in VM

```bash
# Check Docker
docker --version
docker ps

# Run containers
docker run -p 8080:80 nginx

# Use Docker Compose
docker compose up
```

### Environment Variables

Load from .env file:
```bash
ai-env  # Custom function to load .env
```

Set permanently:
```bash
echo "export OPENAI_API_KEY=sk-..." >> ~/.bashrc
source ~/.bashrc
```

## Customization

### Adjusting Resources

Edit `Vagrantfile`:
```ruby
config.vm.provider "virtualbox" do |vb|
  vb.memory = "16384"  # 16GB RAM
  vb.cpus = 8          # 8 CPU cores
end
```

### Adding Software

1. Edit `provision.sh` to add packages
2. Reprovision the VM:
   ```bash
   vagrant reload --provision
   ```

### Changing Network

Private network IP:
```ruby
config.vm.network "private_network", ip: "192.168.56.20"
```

Additional port forwarding:
```ruby
config.vm.network "forwarded_port", guest: 9000, host: 9000
```

## Troubleshooting

### VM Won't Start
```bash
# Check virtualization is enabled in BIOS
# Check VirtualBox/VMware is installed
vagrant status
vagrant up --debug
```

### Slow Performance
```bash
# Increase resources in Vagrantfile
# Enable virtualization features
vb.customize ["modifyvm", :id, "--nested-hw-virt", "on"]
```

### Network Issues
```bash
# Restart network inside VM
vagrant ssh
sudo systemctl restart systemd-networkd
```

### Shared Folder Issues
```bash
# Install/update guest additions
vagrant plugin install vagrant-vbguest
vagrant reload
```

### Port Already in Use
```bash
# Find process using port
lsof -i :8888  # On macOS/Linux
netstat -ano | findstr :8888  # On Windows

# Change port in Vagrantfile
config.vm.network "forwarded_port", guest: 8888, host: 8889
```

## Best Practices

1. **Regular Snapshots**:
   ```bash
   vagrant snapshot save backup-$(date +%Y%m%d)
   vagrant snapshot list
   vagrant snapshot restore backup-20240118
   ```

2. **Keep Vagrantfile in Version Control**:
   ```bash
   git add Vagrantfile provision.sh
   git commit -m "Update VM configuration"
   ```

3. **Use .env for Secrets**:
   - Never commit .env files
   - Use .env.example as template

4. **Regular Updates**:
   ```bash
   vagrant ssh
   sudo apt update && sudo apt upgrade
   pip install --upgrade pip
   npm update -g
   ```

## Advanced Usage

### Multiple VMs
Create multiple environments:
```ruby
Vagrant.configure("2") do |config|
  config.vm.define "dev" do |dev|
    dev.vm.box = "ubuntu/jammy64"
    # Dev configuration
  end

  config.vm.define "test" do |test|
    test.vm.box = "ubuntu/jammy64"
    # Test configuration
  end
end
```

### Custom Provisioners
Add Ansible/Chef/Puppet:
```ruby
config.vm.provision "ansible" do |ansible|
  ansible.playbook = "playbook.yml"
end
```

### Package for Distribution
```bash
# Create a box file
vagrant package --output ai-dev.box

# Share with team
vagrant box add ai-dev ai-dev.box
```

## Cleanup

```bash
# Remove VM and all data
vagrant destroy -f

# Remove downloaded box
vagrant box remove ubuntu/jammy64

# Clean Vagrant cache
rm -rf ~/.vagrant.d/tmp/*
```

## Contributing

To improve this VM configuration:
1. Test changes thoroughly
2. Document new features
3. Submit via `ddx environments contribute`

## License

Part of the DDx toolkit - see main project license.