# Dockerfile for running DDx demo recordings
# Usage: docker run --rm -v $(pwd):/workspace ddx-demos:latest

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    curl \
    git \
    python3 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*

# Install asciinema and agg
RUN pip3 install asciinema --break-system-packages
RUN curl -fsSL https://github.com/asciinema/agg/releases/latest/download/agg-x86_64-unknown-linux-gnu \
    -o /usr/local/bin/agg && chmod +x /usr/local/bin/agg

WORKDIR /workspace
