#!/usr/bin/env bash
# Provisioning script for AI Development VM
# Sets up a complete development environment with AI tools and SDKs

set -euo pipefail

# Update system
echo "=== Updating system packages ==="
apt-get update
apt-get upgrade -y

# Install essential packages
echo "=== Installing essential packages ==="
apt-get install -y \
    build-essential \
    curl \
    wget \
    git \
    vim \
    nano \
    tmux \
    htop \
    tree \
    zip \
    unzip \
    tar \
    gzip \
    ca-certificates \
    gnupg \
    lsb-release \
    software-properties-common \
    apt-transport-https \
    net-tools \
    iputils-ping \
    dnsutils \
    jq \
    make \
    openssh-server \
    ufw

# Install Python 3.11
echo "=== Installing Python 3.11 ==="
add-apt-repository ppa:deadsnakes/ppa -y
apt-get update
apt-get install -y python3.11 python3.11-dev python3.11-venv python3-pip
update-alternatives --install /usr/bin/python3 python3 /usr/bin/python3.11 1
update-alternatives --set python3 /usr/bin/python3.11

# Install Python packages
echo "=== Installing Python packages ==="
pip3 install --upgrade pip setuptools wheel
pip3 install \
    openai \
    anthropic \
    langchain \
    chromadb \
    jupyter \
    notebook \
    jupyterlab \
    ipython \
    numpy \
    pandas \
    scikit-learn \
    matplotlib \
    seaborn \
    plotly \
    tensorflow \
    torch \
    transformers \
    datasets \
    python-dotenv \
    fastapi \
    uvicorn \
    flask \
    django \
    sqlalchemy \
    redis \
    celery \
    pytest \
    black \
    pylint \
    mypy \
    requests \
    httpx \
    pydantic

# Install Node.js 20
echo "=== Installing Node.js 20 ==="
curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
apt-get install -y nodejs
npm install -g npm@latest

# Install Node.js packages
echo "=== Installing Node.js packages ==="
npm install -g \
    typescript \
    ts-node \
    nodemon \
    pm2 \
    prettier \
    eslint \
    @anthropic-ai/sdk \
    openai \
    langchain \
    express \
    fastify \
    next \
    create-react-app \
    create-next-app \
    vercel \
    netlify-cli

# Install Go
echo "=== Installing Go ==="
GO_VERSION="1.21.6"
wget -q "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
rm "go${GO_VERSION}.linux-amd64.tar.gz"
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
echo 'export GOPATH=$HOME/go' >> /etc/profile
echo 'export PATH=$PATH:$GOPATH/bin' >> /etc/profile

# Install Go tools
export PATH=$PATH:/usr/local/go/bin
go install golang.org/x/tools/gopls@latest
go install github.com/go-delve/delve/cmd/dlv@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install Rust
echo "=== Installing Rust ==="
su - vagrant -c "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y"
echo 'source $HOME/.cargo/env' >> /home/vagrant/.bashrc

# Install Docker
echo "=== Installing Docker ==="
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
usermod -aG docker vagrant
systemctl enable docker
systemctl start docker

# Install PostgreSQL
echo "=== Installing PostgreSQL ==="
apt-get install -y postgresql postgresql-contrib postgresql-client
sudo -u postgres createuser -s vagrant
sudo -u postgres createdb vagrant
systemctl enable postgresql
systemctl start postgresql

# Install Redis
echo "=== Installing Redis ==="
apt-get install -y redis-server
sed -i 's/^# requirepass/requirepass vagrant/' /etc/redis/redis.conf
sed -i 's/^bind 127.0.0.1/bind 0.0.0.0/' /etc/redis/redis.conf
systemctl enable redis-server
systemctl restart redis-server

# Install MongoDB
echo "=== Installing MongoDB ==="
curl -fsSL https://pgp.mongodb.com/server-7.0.asc | gpg --dearmor -o /usr/share/keyrings/mongodb-server-7.0.gpg
echo "deb [ arch=amd64,arm64 signed-by=/usr/share/keyrings/mongodb-server-7.0.gpg ] https://repo.mongodb.org/apt/ubuntu jammy/mongodb-org/7.0 multiverse" | tee /etc/apt/sources.list.d/mongodb-org-7.0.list
apt-get update
apt-get install -y mongodb-org
systemctl enable mongod
systemctl start mongod

# Install GitHub CLI
echo "=== Installing GitHub CLI ==="
curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | gpg --dearmor -o /usr/share/keyrings/githubcli-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null
apt-get update
apt-get install -y gh

# Install kubectl
echo "=== Installing kubectl ==="
curl -fsSL https://packages.cloud.google.com/apt/doc/apt-key.gpg | gpg --dearmor -o /usr/share/keyrings/kubernetes-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/kubernetes-archive-keyring.gpg] https://apt.kubernetes.io/ kubernetes-xenial main" | tee /etc/apt/sources.list.d/kubernetes.list
apt-get update
apt-get install -y kubectl

# Install Terraform
echo "=== Installing Terraform ==="
wget -O- https://apt.releases.hashicorp.com/gpg | gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | tee /etc/apt/sources.list.d/hashicorp.list
apt-get update
apt-get install -y terraform

# Configure Jupyter
echo "=== Configuring Jupyter ==="
su - vagrant -c "jupyter notebook --generate-config"
cat >> /home/vagrant/.jupyter/jupyter_notebook_config.py <<EOF
c.NotebookApp.ip = '0.0.0.0'
c.NotebookApp.port = 8888
c.NotebookApp.open_browser = False
c.NotebookApp.token = 'vagrant'
c.NotebookApp.password = ''
c.NotebookApp.allow_origin = '*'
c.NotebookApp.allow_root = False
EOF

# Create systemd service for Jupyter
cat > /etc/systemd/system/jupyter.service <<EOF
[Unit]
Description=Jupyter Notebook
After=network.target

[Service]
Type=simple
User=vagrant
Group=vagrant
WorkingDirectory=/home/vagrant
ExecStart=/usr/local/bin/jupyter notebook --config=/home/vagrant/.jupyter/jupyter_notebook_config.py
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable jupyter

# Set up development directories
echo "=== Setting up development directories ==="
su - vagrant -c "mkdir -p ~/Development/{projects,notebooks,data,models}"
su - vagrant -c "mkdir -p ~/go/{bin,src,pkg}"

# Configure shell for vagrant user
echo "=== Configuring shell environment ==="
cat >> /home/vagrant/.bashrc <<'EOF'

# Development environment settings
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin:$HOME/.local/bin
export GOPATH=$HOME/go
export PYTHONPATH=/home/vagrant/Development:$PYTHONPATH

# Aliases
alias ll='ls -la'
alias gs='git status'
alias gd='git diff'
alias gc='git commit'
alias gp='git push'
alias docker-clean='docker system prune -a'
alias k='kubectl'
alias tf='terraform'

# Functions
ai-env() {
    if [ -f .env ]; then
        export $(cat .env | grep -v '^#' | xargs)
        echo "Environment variables loaded from .env"
    else
        echo "No .env file found"
    fi
}

# Load .cargo/env if it exists
[ -f "$HOME/.cargo/env" ] && source "$HOME/.cargo/env"

# Prompt customization
PS1='\[\033[1;34m\]ai-vm\[\033[0m\]:\[\033[1;36m\]\w\[\033[0m\]$ '

# Welcome message
echo "Welcome to AI Development VM!"
echo "Jupyter token: vagrant"
echo "Run 'ai-env' to load .env variables"
EOF

# Create sample .env file
cat > /home/vagrant/.env.example <<'EOF'
# AI Service API Keys
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
LANGCHAIN_API_KEY=
HUGGINGFACE_API_TOKEN=

# GitHub Configuration
GITHUB_TOKEN=ghp_...
GITHUB_USER=
GITHUB_EMAIL=

# Database Configuration
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=vagrant
POSTGRES_PASSWORD=vagrant
POSTGRES_DB=development

REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=vagrant

MONGODB_URI=mongodb://localhost:27017/development

# Environment
NODE_ENV=development
ENVIRONMENT=development
EOF

chown vagrant:vagrant /home/vagrant/.env.example

# Configure firewall
echo "=== Configuring firewall ==="
ufw --force enable
ufw allow 22/tcp    # SSH
ufw allow 3000/tcp  # Node.js
ufw allow 5000/tcp  # Flask/FastAPI
ufw allow 8000/tcp  # Django/ChromaDB
ufw allow 8888/tcp  # Jupyter
ufw allow 5432/tcp  # PostgreSQL
ufw allow 6379/tcp  # Redis
ufw reload

# Clean up
echo "=== Cleaning up ==="
apt-get autoremove -y
apt-get clean
rm -rf /var/lib/apt/lists/*

# Final message
echo "=== Provisioning complete! ==="
echo "AI Development VM is ready for use"
echo "Run 'vagrant ssh' to access the VM"