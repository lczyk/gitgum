FROM ubuntu:24.04

# Avoid interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install bash (already present), git, fzf, bash-completion, and basic utilities
RUN apt-get update && apt-get install -y \
    bash \
    bash-completion \
    git \
    fzf \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Set up working directory
WORKDIR /work

# Default command
CMD ["/bin/bash"]
