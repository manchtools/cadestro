#!/bin/bash

set -e

GITHUB_REPO="manchtools/cadestro"

RELEASE_SIGNING_PUBLIC_KEY="__RELEASE_SIGNING_PUBLIC_KEY__"

INSTALLER_RELEASE_VERSION="__INSTALLER_RELEASE_VERSION__"

DATA_DIR="/var/lib/cadestro"
BINARY_PATH="/usr/local/bin/cadestrod"
SERVICE_NAME="cadestrod"
REGISTRATION_TOKEN=""
SERVER_URL=""
CA_FINGERPRINT_PIN=""
SKIP_DOWNLOAD=""
PRE_RELEASE=""
VERSION="latest"

ENABLE_URI_HANDLER="${CADESTRO_ENABLE_URI_HANDLER:-false}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1" >&2
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1" >&2
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

show_help() {
    cat << EOF
Cadestro Agent Installation Script

Usage:
  sudo ./install.sh [OPTIONS]

  One-liner:
  curl -fsSL https://github.com/${GITHUB_REPO}/releases/latest/download/cadestrod-install.sh | sudo bash -s -- -s URL -t TOKEN -p CA_SHA256

Options:
  -t, --token TOKEN       Registration token for initial setup
  -s, --server URL        Control server URL (e.g., https://control.example.com:8081)
  -p, --pin SHA256        Required control CA fingerprint supplied with the token
  -v, --version VERSION   Version to install (e.g., v2026.2.0; default: latest)
  --pre                   Install the latest prerelease (release candidate) version
  -d, --data-dir DIR      Data directory (default: /var/lib/cadestro)
  -b, --binary PATH       Path to the agent binary (default: /usr/local/bin/cadestrod)
  --skip-download         Skip downloading the binary (use existing binary at --binary path)
  --uninstall             Remove the agent and all configuration
  -h, --help              Show this help message

Examples:

  curl -fsSL https://github.com/${GITHUB_REPO}/releases/latest/download/cadestrod-install.sh | sudo bash -s -- -s https://cadestro.example.com -t abc123 -p CA_SHA256

  sudo ./install.sh --pre -s https://cadestro.example.com -t abc123 -p CA_SHA256

  sudo ./install.sh -v v2026.2.0 -s https://cadestro.example.com -t abc123 -p CA_SHA256

  sudo ./install.sh --skip-download -s https://cadestro.example.com -t abc123 -p CA_SHA256

  sudo ./install.sh --uninstall
EOF
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -t|--token)
                REGISTRATION_TOKEN="$2"
                shift 2
                ;;
            -s|--server)
                SERVER_URL="$2"
                shift 2
                ;;
            -p|--pin)
                CA_FINGERPRINT_PIN="$2"
                shift 2
                ;;
            -d|--data-dir)
                DATA_DIR="$2"
                shift 2
                ;;
            -b|--binary)
                BINARY_PATH="$2"
                shift 2
                ;;
            -v|--version)
                VERSION="$2"
                shift 2
                ;;
            --pre)
                PRE_RELEASE="true"
                shift
                ;;
            --skip-download)
                SKIP_DOWNLOAD="true"
                shift
                ;;
            --enable-uri-handler)
                ENABLE_URI_HANDLER="true"
                shift
                ;;
            --uninstall)
                uninstall
                exit 0
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root (use sudo)"
        exit 1
    fi
}

detect_arch() {
    local machine
    machine=$(uname -m)
    case "$machine" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)
            log_error "Unsupported architecture: $machine"
            exit 1
            ;;
    esac
}

resolve_latest_prerelease() {
    local api_url="https://api.github.com/repos/${GITHUB_REPO}/releases"
    log_info "Querying GitHub API for latest prerelease..."

    local response
    if command -v curl &>/dev/null; then
        response=$(curl -gfsSL "$api_url")
    elif command -v wget &>/dev/null; then
        response=$(wget -qO- "$api_url")
    else
        log_error "Neither curl nor wget found. Please install one and try again."
        exit 1
    fi

    local tag
    tag=$(echo "$response" | tr ',' '\n' | awk -F'"' '/"tag_name"/{tag=$4} /"prerelease": *true/{print tag; exit}' | tr -dc 'a-zA-Z0-9._-')

    if [[ -z "$tag" ]]; then
        log_error "No prerelease found on GitHub"
        exit 1
    fi

    echo "$tag"
}

verify_release_manifest() {
    local manifest_path="$1"
    local signature_path="$2"
    local public_key_path="$3"

    if ! openssl pkeyutl -verify -rawin -pubin -keyform DER \
        -inkey "$public_key_path" -sigfile "$signature_path" \
        -in "$manifest_path" >/dev/null 2>&1; then
        log_error "SHA256SUMS publisher signature is invalid. Refusing to install."
        return 1
    fi
}

download_binary() {
    if [[ -n "$SKIP_DOWNLOAD" ]]; then
        if [[ ! -f "$BINARY_PATH" ]]; then
            log_error "Agent binary not found at $BINARY_PATH (--skip-download was set)"
            exit 1
        fi
        log_info "Using existing binary at $BINARY_PATH"
        chmod +x "$BINARY_PATH"
        return
    fi

    local version_sentinel="__INSTALLER_RELEASE_VERSION"
    version_sentinel="${version_sentinel}__"
    if [[ "$VERSION" == "latest" ]] && [[ -z "$PRE_RELEASE" ]] \
        && [[ -n "$INSTALLER_RELEASE_VERSION" ]] \
        && [[ "$INSTALLER_RELEASE_VERSION" != "$version_sentinel" ]]; then
        VERSION="$INSTALLER_RELEASE_VERSION"
        log_info "No version named; installing this installer's release: ${VERSION}"
    fi

    if [[ -n "$PRE_RELEASE" ]] && [[ "$VERSION" == "latest" ]]; then
        VERSION=$(resolve_latest_prerelease)
        log_info "Latest prerelease: ${VERSION}"
    fi

    local arch
    arch=$(detect_arch)
    local binary_name="cadestrod-linux-${arch}"
    local download_url sums_url sums_signature_url release_base

    if [[ "$VERSION" == "latest" ]]; then
        release_base="https://github.com/${GITHUB_REPO}/releases/latest/download"
    else
        release_base="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}"
    fi
    download_url="${release_base}/${binary_name}"
    sums_url="${release_base}/SHA256SUMS"
    sums_signature_url="${release_base}/SHA256SUMS.sig"

    log_info "Detected architecture: ${arch}"
    log_info "Downloading agent from ${download_url}..."

    local dest_dir
    dest_dir=$(dirname "$BINARY_PATH")
    mkdir -p "$dest_dir"
    local tmp_binary tmp_sums tmp_signature tmp_public
    tmp_binary=$(mktemp "${dest_dir}/.cadestrod.XXXXXX")
    tmp_sums=$(mktemp "${dest_dir}/.SHA256SUMS.XXXXXX")
    tmp_signature=$(mktemp "${dest_dir}/.SHA256SUMS.sig.XXXXXX")
    tmp_public=$(mktemp "${dest_dir}/.release-signing-public.XXXXXX")

    cleanup_download_tmp() {
        rm -f "$tmp_binary" "$tmp_sums" "$tmp_signature" "$tmp_public"
    }
    trap cleanup_download_tmp EXIT INT TERM

    if command -v curl &>/dev/null; then
        if ! curl -gfSL --progress-bar -o "$tmp_binary" "$download_url"; then
            log_error "Download failed. Check the version and that the release exists."
            exit 1
        fi
        if ! curl -gfSL -o "$tmp_sums" "$sums_url"; then
            log_error "SHA256SUMS download failed. Refusing to install unverified binary."
            exit 1
        fi
        if ! curl -gfSL -o "$tmp_signature" "$sums_signature_url"; then
            log_error "SHA256SUMS signature download failed. Refusing to install unverified binary."
            exit 1
        fi
    elif command -v wget &>/dev/null; then
        if ! wget -q --show-progress -O "$tmp_binary" "$download_url"; then
            log_error "Download failed. Check the version and that the release exists."
            exit 1
        fi
        if ! wget -q -O "$tmp_sums" "$sums_url"; then
            log_error "SHA256SUMS download failed. Refusing to install unverified binary."
            exit 1
        fi
        if ! wget -q -O "$tmp_signature" "$sums_signature_url"; then
            log_error "SHA256SUMS signature download failed. Refusing to install unverified binary."
            exit 1
        fi
    else
        log_error "Neither curl nor wget found. Please install one and try again."
        exit 1
    fi

    local placeholder_sentinel="__RELEASE_SIGNING_PUBLIC_KEY"
    placeholder_sentinel="${placeholder_sentinel}__"
    if [[ "$RELEASE_SIGNING_PUBLIC_KEY" == "$placeholder_sentinel" ]] || \
        [[ -z "$RELEASE_SIGNING_PUBLIC_KEY" ]]; then
        log_error "Release signing public key is not configured. Refusing to install."
        exit 1
    fi
    if ! command -v openssl &>/dev/null || ! command -v base64 &>/dev/null; then
        log_error "openssl and base64 are required for release signature verification."
        exit 1
    fi
    if ! printf '%s' "$RELEASE_SIGNING_PUBLIC_KEY" | base64 --decode > "$tmp_public"; then
        log_error "Release signing public key is invalid. Refusing to install."
        exit 1
    fi
    if ! verify_release_manifest "$tmp_sums" "$tmp_signature" "$tmp_public"; then
        exit 1
    fi
    log_info "Publisher signature verified."

    local expected_sha actual_sha
    expected_sha=$(awk -v f="$binary_name" '$2 == f || $2 == "*" f { print $1; exit }' "$tmp_sums")
    if [[ -z "$expected_sha" ]]; then
        log_error "SHA256SUMS has no entry for ${binary_name}. Refusing to install."
        exit 1
    fi
    if ! command -v sha256sum &>/dev/null; then
        log_error "sha256sum not found. Cannot verify binary integrity; refusing to install."
        exit 1
    fi
    actual_sha=$(sha256sum "$tmp_binary" | awk '{print $1}')
    if [[ "$expected_sha" != "$actual_sha" ]]; then
        log_error "SHA256 mismatch for ${binary_name}."
        log_error "  expected: ${expected_sha}"
        log_error "  actual:   ${actual_sha}"
        log_error "Refusing to install tampered or corrupt binary."
        exit 1
    fi
    log_info "SHA256 verified."

    chmod 0755 "$tmp_binary"

    if ! mv -f "$tmp_binary" "$BINARY_PATH"; then
        log_error "Failed to install binary to ${BINARY_PATH}."
        exit 1
    fi
    rm -f "$tmp_sums" "$tmp_signature" "$tmp_public"
    trap - EXIT INT TERM

    log_info "Binary installed to $BINARY_PATH"
}

create_directories() {
    log_info "Creating data directory: $DATA_DIR"
    mkdir -p "$DATA_DIR"
    chown root:root "$DATA_DIR"
    chmod 700 "$DATA_DIR"
}

install_systemd_service() {
    log_info "Installing systemd service..."

    "$BINARY_PATH" install-unit --data-dir="$DATA_DIR"
}

enroll_agent() {
    if [[ -z "$REGISTRATION_TOKEN" ]] && [[ -z "$SERVER_URL" ]] && [[ -z "$CA_FINGERPRINT_PIN" ]]; then
        log_warn "No enrollment parameters provided; skipping enrollment"
        log_info "You can enroll later by running (no sudo required):"
        log_info "  $BINARY_PATH enroll -server=<URL> -token-file=<PATH> -pin=<CA_SHA256>"
        return
    fi
    if [[ -z "$REGISTRATION_TOKEN" ]] || [[ -z "$SERVER_URL" ]] || [[ -z "$CA_FINGERPRINT_PIN" ]]; then
        log_error "Registration token, server URL, and CA fingerprint pin must be provided together"
        return 1
    fi

    log_info "Enrolling agent with server via socket..."

    local token_file
    token_file="$(mktemp)"
    chmod 600 "$token_file"

    trap 'rm -f "$token_file"' RETURN
    printf '%s' "$REGISTRATION_TOKEN" > "$token_file"

    local -a enroll_cmd=(
        "$BINARY_PATH"
        "enroll"
        "-server=$SERVER_URL"
        "-token-file=$token_file"
        "-pin=$CA_FINGERPRINT_PIN"
    )

    local max_wait=10
    local waited=0
    while [[ ! -S "/run/cadestro/enroll.sock" ]] && [[ $waited -lt $max_wait ]]; do
        sleep 1
        waited=$((waited + 1))
    done

    if [[ ! -S "/run/cadestro/enroll.sock" ]]; then
        log_warn "Enrollment socket not available after ${max_wait}s, agent may already be enrolled"
        return
    fi

    if "${enroll_cmd[@]}"; then
        log_info "Agent enrolled successfully"
    else
        log_error "Agent enrollment failed"
        log_info "You can try again later by running:"
        log_info "  $BINARY_PATH enroll -server=$SERVER_URL -token-file=<PATH> -pin=$CA_FINGERPRINT_PIN"
        return 1
    fi
}

enable_and_start_service() {
    log_info "Enabling and starting service..."

    systemctl enable "$SERVICE_NAME"
    systemctl start "$SERVICE_NAME"
    log_info "Service started"

}

uninstall() {
    log_info "Uninstalling Cadestro Agent..."

    if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
        log_info "Stopping service..."
        systemctl stop "$SERVICE_NAME"
    fi

    if systemctl is-enabled --quiet "$SERVICE_NAME" 2>/dev/null; then
        log_info "Disabling service..."
        systemctl disable "$SERVICE_NAME"
    fi

    if [[ -f "/etc/systemd/system/${SERVICE_NAME}.service" ]]; then
        log_info "Removing service file..."
        rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
        systemctl daemon-reload
    fi

    if [[ -f "/usr/share/applications/cadestrod.desktop" ]]; then
        log_info "Removing desktop handler..."
        rm -f "/usr/share/applications/cadestrod.desktop"
    fi

    if [[ -d "$DATA_DIR" ]]; then
        read -p "Remove data directory $DATA_DIR? This will delete agent credentials! [y/N] " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            log_info "Removing data directory..."
            rm -rf "$DATA_DIR"
        else
            log_info "Data directory preserved"
        fi
    fi

    log_info "Uninstall complete"
}

install_desktop_handler() {
    local desktop_file="/usr/share/applications/cadestrod.desktop"

    log_info "Installing desktop URI handler (opt-in)..."

    cat > "$desktop_file" << EOF
[Desktop Entry]
Name=Cadestro Agent
Comment=Cadestro device agent
Exec=$BINARY_PATH %u
Terminal=false
Type=Application
MimeType=x-scheme-handler/cadestro;
NoDisplay=true
EOF

    chmod 644 "$desktop_file"

    if command -v xdg-mime &>/dev/null; then
        xdg-mime default cadestrod.desktop x-scheme-handler/cadestro 2>/dev/null || true
    fi

    log_info "Desktop URI handler installed"
}

show_status() {
    echo ""
    echo "=========================================="
    echo "  Cadestro Agent Installation Complete"
    echo "=========================================="
    echo ""
    echo "Runs As:       root"
    echo "Data Directory: $DATA_DIR"
    echo "Binary Path:   $BINARY_PATH"
    echo "Service Name:  $SERVICE_NAME"
    echo ""
    echo "Useful commands:"
    echo "  Check status:    sudo systemctl status $SERVICE_NAME"
    echo "  View logs:       sudo journalctl -u $SERVICE_NAME -f"
    echo "  Start service:   sudo systemctl start $SERVICE_NAME"
    echo "  Stop service:    sudo systemctl stop $SERVICE_NAME"
    echo "  Restart service: sudo systemctl restart $SERVICE_NAME"
    echo ""

    if [[ -f "$DATA_DIR/credentials.enc" ]]; then
        echo "Agent is enrolled and ready."
    else
        echo "Agent is NOT enrolled yet."
        echo "To enroll (no sudo required), run:"
        echo "  $BINARY_PATH enroll -server=<URL> -token-file=<PATH> -pin=<CA_SHA256>"
    fi
    echo ""
}

stop_service_if_running() {
    if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
        log_info "Stopping running agent service for update..."
        systemctl stop "$SERVICE_NAME"
    fi
}

main() {
    parse_args "$@"
    check_root

    download_binary

    stop_service_if_running

    log_info "Starting Cadestro Agent installation..."

    create_directories
    install_systemd_service

    if [[ "$ENABLE_URI_HANDLER" == "true" ]]; then
        install_desktop_handler
    else
        log_info "Skipping cadestro:// URI handler (enable with --enable-uri-handler)"
    fi

    enable_and_start_service

    if [[ -n "$REGISTRATION_TOKEN" ]] && [[ -n "$SERVER_URL" ]]; then
        enroll_agent
    fi

    show_status
}

if [[ "${1:-}" == "--internal-verify-release-manifest" ]]; then
    if [[ $# -ne 4 ]]; then
        exit 2
    fi
    if verify_release_manifest "$2" "$3" "$4"; then
        exit 0
    fi
    exit 1
fi

main "$@"
