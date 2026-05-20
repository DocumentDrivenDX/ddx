ARG GO_IMAGE=golang:1.26.3-bookworm
FROM ${GO_IMAGE}

LABEL org.opencontainers.image.title="DDx attempt runner"
LABEL org.opencontainers.image.description="Baseline image for ddx try --attempt-backend docker-clone"

ARG BUN_VERSION=1.3.11
ARG GOLANGCI_LINT_VERSION=2.11.4
ARG LEFTHOOK_VERSION=v2.1.6
ENV DEBIAN_FRONTEND=noninteractive
ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8
ENV GOPATH=/opt/go
ENV GOMODCACHE=/opt/go/pkg/mod
ENV GOCACHE=/opt/go-build-cache
ENV BUN_INSTALL=/opt/bun
ENV PATH=/opt/bun/bin:/opt/go/bin:/usr/local/go/bin:${PATH}

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    build-essential \
    ca-certificates \
    curl \
    git \
    git-lfs \
    gnupg \
    jq \
    make \
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
  && curl -fsSL https://deb.nodesource.com/setup_lts.x | bash - \
  && apt-get install -y --no-install-recommends nodejs \
  && git lfs install --system \
  && rm -rf /var/lib/apt/lists/* \
  && node --version \
  && npm --version

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

RUN set -eux; \
  GOBIN=/usr/local/bin go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v${GOLANGCI_LINT_VERSION}"; \
  GOBIN=/usr/local/bin go install "github.com/evilmartians/lefthook/v2@${LEFTHOOK_VERSION}"; \
  golangci-lint --version; \
  lefthook version; \
  rm -rf /opt/go/pkg/mod /opt/go-build-cache

COPY runner-info /usr/local/bin/ddx-attempt-runner-info
RUN chmod 0755 /usr/local/bin/ddx-attempt-runner-info

WORKDIR /work
CMD ["/bin/bash"]
