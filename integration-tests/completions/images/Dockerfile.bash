FROM ubuntu:24.04

# Avoid interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install bash (already present), git, fzf, and basic utilities
RUN apt-get update && apt-get install -y \
    bash \
    git \
    fzf \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Set up working directory
WORKDIR /workspace

# Default command
CMD ["/bin/bash"]
