#!/bin/sh
# Backlog CLI installer
#
# Usage:
#   curl -fsSL https://relay.example.com/install.sh | sh
#   curl -fsSL https://relay.example.com/install.sh | sh -s -- --name my-tenant --passphrase 'secret'
#
# Environment variables (alternative to flags):
#   BACKLOG_RELAY_URL    Relay server URL (auto-detected from download URL)
#   BACKLOG_NAME         Tenant name
#   BACKLOG_PASSPHRASE   Passphrase for portal authentication
#   BACKLOG_SPACE        Space host (e.g. example.backlog.jp)
#
# This script:
#   1. Installs backlog CLI (brew preferred, GitHub Releases fallback)
#   2. Optionally runs `backlog config setup` if credentials are provided

set -e

GITHUB_REPO="yacchi/backlog-cli"
BREW_TAP="yacchi/tap/backlog-cli"
INSTALL_DIR="${BACKLOG_INSTALL_DIR:-/usr/local/bin}"

# --- helpers ---

info() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mWarning:\033[0m %s\n' "$*" >&2; }
error() { printf '\033[1;31mError:\033[0m %s\n' "$*" >&2; exit 1; }

need_cmd() {
    if ! command -v "$1" > /dev/null 2>&1; then
        error "Required command not found: $1"
    fi
}

detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
        *) error "Unsupported OS: $(uname -s)" ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *) error "Unsupported architecture: $(uname -m)" ;;
    esac
}

# --- parse args ---

RELAY_URL="${BACKLOG_RELAY_URL:-}"
NAME="${BACKLOG_NAME:-}"
PASSPHRASE="${BACKLOG_PASSPHRASE:-}"
SPACE="${BACKLOG_SPACE:-}"
SKIP_SETUP=0

while [ $# -gt 0 ]; do
    case "$1" in
        --relay-url)    RELAY_URL="$2";    shift 2 ;;
        --name)         NAME="$2";         shift 2 ;;
        --passphrase)   PASSPHRASE="$2";   shift 2 ;;
        --space)        SPACE="$2";        shift 2 ;;
        --skip-setup)   SKIP_SETUP=1;      shift ;;
        --install-dir)  INSTALL_DIR="$2";  shift 2 ;;
        -h|--help)
            sed -n '2,/^$/s/^# //p' "$0" 2>/dev/null || true
            exit 0
            ;;
        *) error "Unknown option: $1" ;;
    esac
done

# --- install CLI ---

install_with_brew() {
    if ! command -v brew > /dev/null 2>&1; then
        return 1
    fi
    info "Installing via Homebrew..."
    brew install "$BREW_TAP" || brew upgrade "$BREW_TAP" 2>/dev/null || true
    return 0
}

install_from_releases() {
    need_cmd curl
    need_cmd tar

    local os arch archive_name url tmpdir checksum_url checksums

    os="$(detect_os)"
    arch="$(detect_arch)"

    info "Detecting latest release..."
    local latest
    latest="$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" \
        | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"

    if [ -z "$latest" ]; then
        error "Failed to detect latest release"
    fi

    local version="${latest#v}"
    archive_name="backlog-cli_${version}_${os}_${arch}.tar.gz"
    url="https://github.com/${GITHUB_REPO}/releases/download/${latest}/${archive_name}"
    checksum_url="https://github.com/${GITHUB_REPO}/releases/download/${latest}/checksums.txt"

    info "Downloading ${archive_name}..."
    tmpdir="$(mktemp -d)"
    trap 'rm -rf "$tmpdir"' EXIT

    curl -fsSL -o "${tmpdir}/${archive_name}" "$url"
    curl -fsSL -o "${tmpdir}/checksums.txt" "$checksum_url"

    info "Verifying checksum..."
    local expected actual
    expected="$(grep "${archive_name}" "${tmpdir}/checksums.txt" | awk '{print $1}')"
    if [ -z "$expected" ]; then
        error "Checksum not found for ${archive_name}"
    fi

    if command -v sha256sum > /dev/null 2>&1; then
        actual="$(sha256sum "${tmpdir}/${archive_name}" | awk '{print $1}')"
    elif command -v shasum > /dev/null 2>&1; then
        actual="$(shasum -a 256 "${tmpdir}/${archive_name}" | awk '{print $1}')"
    else
        warn "sha256sum/shasum not found, skipping checksum verification"
        actual="$expected"
    fi

    if [ "$expected" != "$actual" ]; then
        error "Checksum mismatch: expected ${expected}, got ${actual}"
    fi

    info "Extracting to ${INSTALL_DIR}..."
    tar -xzf "${tmpdir}/${archive_name}" -C "$tmpdir"

    if [ ! -w "$INSTALL_DIR" ]; then
        info "Requires sudo to install to ${INSTALL_DIR}"
        sudo mkdir -p "$INSTALL_DIR"
        sudo mv "${tmpdir}/backlog" "${INSTALL_DIR}/backlog"
        sudo chmod +x "${INSTALL_DIR}/backlog"
    else
        mkdir -p "$INSTALL_DIR"
        mv "${tmpdir}/backlog" "${INSTALL_DIR}/backlog"
        chmod +x "${INSTALL_DIR}/backlog"
    fi
}

if ! command -v backlog > /dev/null 2>&1; then
    if ! install_with_brew; then
        install_from_releases
    fi
    info "Installed: $(backlog version 2>/dev/null || echo 'backlog')"
else
    info "Backlog CLI already installed: $(backlog version 2>/dev/null || echo 'unknown version')"
    if command -v brew > /dev/null 2>&1 && brew list --formula 2>/dev/null | grep -q backlog-cli; then
        info "Upgrading via Homebrew..."
        brew upgrade "$BREW_TAP" 2>/dev/null || true
    fi
fi

# --- setup (optional) ---

if [ "$SKIP_SETUP" = "1" ]; then
    info "Skipping setup (--skip-setup)"
    exit 0
fi

if [ -z "$RELAY_URL" ] && [ -z "$NAME" ] && [ -z "$PASSPHRASE" ]; then
    info "Installation complete. To configure, run:"
    echo "  backlog config setup --relay-url <URL> --name <tenant> --passphrase <passphrase>"
    exit 0
fi

setup_args=""
if [ -n "$RELAY_URL" ]; then
    setup_args="$setup_args --relay-url $RELAY_URL"
fi
if [ -n "$NAME" ]; then
    setup_args="$setup_args --name $NAME"
fi
if [ -n "$PASSPHRASE" ]; then
    setup_args="$setup_args --passphrase $PASSPHRASE"
fi
if [ -n "$SPACE" ]; then
    setup_args="$setup_args --space $SPACE"
fi

info "Running setup..."
# shellcheck disable=SC2086
backlog config setup --yes $setup_args
