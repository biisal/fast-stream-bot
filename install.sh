#!/usr/bin/env bash
set -euo pipefail

APP=fast-stream-bot
REPO="biisal/fast-stream-bot"

# Colors
MUTED='\033[0;2m'
RED='\033[0;31m'
ORANGE='\033[38;5;214m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

# --- 1. Helper Functions ---

log() {
    echo -e "${MUTED}[$(date +'%H:%M:%S')]${NC} $1"
}

usage() {
    cat <<EOF
Fast Stream Bot Installer
Usage: install.sh [options]

Options:
    -h, --help                Display this help message
    -v, --version <version>   Install a specific version
    -b, --binary <path>       Install from a local binary
EOF
}

show_logo() {
    echo -e "${CYAN}"
    echo -e "  █▀▀ ▄▀█ █▀ ▀█▀ ▄▄ █▀ ▀█▀ █▀█ █▀▀ ▄▀█ █▀▄▀█ ▄▄ █▄▄ █▀█ ▀█▀"
    echo -e "  █▀░ █▀█ ▄█ ░█░ ░░ ▄█ ░█░ █▀▄ ██▄ █▀█ █░▀░█ ░░ █▄█ █▄█ ░█░"
    echo -e "${NC}"
}

# --- 2. Main Script Logic ---

requested_version=""
binary_path=""
no_modify_path=false

# Argument Parsing
while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help) usage; exit 0 ;;
        -v|--version) requested_version="${2:-}"; shift 2 ;;
        -b|--binary) binary_path="${2:-}"; shift 2 ;;
        --no-modify-path) no_modify_path=true; shift ;;
        *) shift ;;
    esac
done

INSTALL_DIR=$HOME/.fast-stream-bot/bin
mkdir -p "$INSTALL_DIR"

log "System: ${GREEN}$(uname -s)/$(uname -m)${NC}"

# Version and Asset Resolution
if [ -n "$binary_path" ]; then
    log "Mode: ${ORANGE}Local Binary Installation${NC}"
    specific_version="local-dev"
else
    log "Mode: ${GREEN}Remote Download${NC}"
    
    # OS/Arch Detection
    os=$(uname -s); arch=$(uname -m)
    [[ "$os" == "Darwin" ]] && os="Darwin" || os="Linux"
    [[ "$arch" == "arm64" || "$arch" == "aarch64" ]] && arch="arm64" || arch="x86_64"
    filename="${APP}_${os}_${arch}.tar.gz"

    log "Resolving latest version from GitHub..."
    # Fetches first release found (handles pre-releases/snapshots)
    release_data=$(curl -s "https://api.github.com/repos/$REPO/releases" | grep -m 1 '"tag_name":' || true)
    
    if [ -z "$release_data" ]; then
        echo -e "${RED}Error: No releases found at https://github.com/$REPO/releases${NC}"
        exit 1
    fi

    specific_version=$(echo "$release_data" | sed -E 's/.*"([^"]+)".*/\1/')
    url="https://github.com/$REPO/releases/download/$specific_version/$filename"
    log "Selected Version: ${CYAN}$specific_version${NC}"
fi

# Installation Execution
if [ -n "$binary_path" ]; then
    cp "$binary_path" "$INSTALL_DIR/$APP"
else
    log "Downloading: ${MUTED}$url${NC}"
    tmp_dir=$(mktemp -d)
    if ! curl -# -L -f -o "$tmp_dir/$filename" "$url"; then
        echo -e "${RED}Error: Download failed. The file may not exist for your architecture.${NC}"
        rm -rf "$tmp_dir"
        exit 1
    fi

    log "Extracting assets..."
    tar -xzf "$tmp_dir/$filename" -C "$tmp_dir"
    
    if [ -f "$tmp_dir/fsb" ]; then
        mv "$tmp_dir/fsb" "$INSTALL_DIR/$APP"
    elif [ -f "$tmp_dir/$APP" ]; then
        mv "$tmp_dir/$APP" "$INSTALL_DIR/$APP"
    else
        echo -e "${RED}Error: Binary not found in archive.${NC}"
        rm -rf "$tmp_dir"
        exit 1
    fi
    rm -rf "$tmp_dir"
fi

chmod +x "${INSTALL_DIR}/$APP"
log "Binary installed to: ${MUTED}$INSTALL_DIR/$APP${NC}"

# Shell Path Configuration
if [ "$no_modify_path" = "false" ]; then
    log "Configuring shell path..."
    
    shell_config=""
    case $(basename "$SHELL") in
        zsh)  shell_config="$HOME/.zshrc" ;;
        bash) shell_config="$HOME/.bashrc" ;;
        fish) shell_config="$HOME/.config/fish/config.fish" ;;
    esac

    if [ -n "$shell_config" ] && [ -f "$shell_config" ]; then
        if ! grep -q "$INSTALL_DIR" "$shell_config"; then
            echo -e "\n# Fast Stream Bot\nexport PATH=\"$INSTALL_DIR:\$PATH\"" >> "$shell_config"
            log "Permanent path added to $shell_config."
        fi
    fi

fi

# Final Success Output
echo -e "\n------------------------------------------------"
show_logo
echo -e "${GREEN}Fast Stream Bot $specific_version successfully installed!${NC}"
echo -e "Close the terminal and reopen it to run ${GREEN}fast-stream-bot${NC} command"
echo -e "Documentation: https://github.com/$REPO"
echo -e "------------------------------------------------\n"
