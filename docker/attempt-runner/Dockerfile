ARG GO_IMAGE=golang:1.26.3-bookworm
FROM ${GO_IMAGE}

LABEL org.opencontainers.image.title="DDx attempt runner"
LABEL org.opencontainers.image.description="Baseline image for ddx try --attempt-backend docker-clone"

ARG BUN_VERSION=1.3.11
ENV DEBIAN_FRONTEND=noninteractive
ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8
ENV GOPATH=/tmp/go
ENV GOMODCACHE=/tmp/go/pkg/mod
ENV GOCACHE=/tmp/go-build-cache
ENV BUN_INSTALL=/opt/bun
ENV PATH=/opt/bun/bin:/tmp/go/bin:/usr/local/go/bin:${PATH}

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    build-essential \
    ca-certificates \
    curl \
    git \
    git-lfs \
    jq \
    make \
    nodejs \
    npm \
    openssh-client \
    patch \
    procps \
    python3 \
    python3-pip \
    ripgrep \
    rsync \
    unzip \
    xz-utils \
    zip \
    zstd \
  && git lfs install --system \
  && rm -rf /var/lib/apt/lists/*

RUN set -eux; \
  case "$(dpkg --print-architecture)" in \
    amd64) bun_arch="x64" ;; \
    arm64) bun_arch="aarch64" ;; \
    *) echo "unsupported architecture for bun: $(dpkg --print-architecture)" >&2; exit 1 ;; \
  esac; \
  curl -fsSL "https://github.com/oven-sh/bun/releases/download/bun-v${BUN_VERSION}/bun-linux-${bun_arch}.zip" -o /tmp/bun.zip; \
  unzip -q /tmp/bun.zip -d /tmp; \
  mkdir -p /opt/bun/bin; \
  mv "/tmp/bun-linux-${bun_arch}/bun" /opt/bun/bin/bun; \
  chmod 0755 /opt/bun/bin/bun; \
  rm -rf /tmp/bun.zip "/tmp/bun-linux-${bun_arch}"

COPY runner-info /usr/local/bin/ddx-attempt-runner-info
RUN chmod 0755 /usr/local/bin/ddx-attempt-runner-info

WORKDIR /work
CMD ["/bin/bash"]
