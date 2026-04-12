# Dockerfile for running DDx demo recordings and website build
# Usage: docker run --rm -v $(pwd):/workspace ddx-demos:latest

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# Pinned tool versions
ARG HUGO_VERSION=0.159.2
ARG AGG_VERSION=1.7.0

RUN apt-get update && apt-get install -y \
    curl \
    git \
    golang-go \
    python3 \
    python3-pip \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Install asciinema and agg
RUN pip3 install asciinema --break-system-packages
RUN set -eux; \
    case "$(dpkg --print-architecture)" in \
        amd64) agg_asset="agg-x86_64-unknown-linux-gnu" ;; \
        arm64) agg_asset="agg-aarch64-unknown-linux-gnu" ;; \
        *) echo "unsupported architecture for agg: $(dpkg --print-architecture)" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://github.com/asciinema/agg/releases/download/v${AGG_VERSION}/${agg_asset}" \
      -o /usr/local/bin/agg && chmod +x /usr/local/bin/agg

# Install Hugo extended (pinned version)
RUN set -eux; \
    case "$(dpkg --print-architecture)" in \
        amd64) hugo_arch="amd64" ;; \
        arm64) hugo_arch="arm64" ;; \
        *) echo "unsupported architecture for Hugo: $(dpkg --print-architecture)" >&2; exit 1 ;; \
    esac; \
    wget -O /tmp/hugo.deb "https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_extended_${HUGO_VERSION}_linux-${hugo_arch}.deb" \
      && dpkg -i /tmp/hugo.deb \
      && rm /tmp/hugo.deb

WORKDIR /workspace
