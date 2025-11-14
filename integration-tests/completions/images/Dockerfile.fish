FROM ubuntu:24.04

# Avoid interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install fish, git, fzf, and basic utilities
RUN apt-get update && apt-get install -y \
    fish \
    git \
    fzf \
    curl \
    bash \
    && rm -rf /var/lib/apt/lists/*

# Set up working directory
WORKDIR /workspace

# Default command
CMD ["/usr/bin/fish"]
