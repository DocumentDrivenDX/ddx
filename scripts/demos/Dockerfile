# Dockerfile for running DDx demo recordings and website build
# Usage: docker run --rm -v $(pwd):/workspace ddx-demos:latest

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# Pinned tool versions
ARG HUGO_VERSION=0.159.2
ARG AGG_VERSION=1.4.4

RUN apt-get update && apt-get install -y \
    curl \
    git \
    python3 \
    python3-pip \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Install asciinema and agg
RUN pip3 install asciinema --break-system-packages
RUN curl -fsSL https://github.com/asciinema/agg/releases/download/v${AGG_VERSION}/agg-x86_64-unknown-linux-gnu \
    -o /usr/local/bin/agg && chmod +x /usr/local/bin/agg

# Install Hugo extended (pinned version)
RUN wget -O /tmp/hugo.deb https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_extended_${HUGO_VERSION}_linux-amd64.deb \
    && dpkg -i /tmp/hugo.deb \
    && rm /tmp/hugo.deb

WORKDIR /workspace
