#!/bin/sh
set -e

OWNER="${OWNER:-quonaro}"
REPO="${REPO:-locrest-client}"
VERSION="${VERSION:-latest}"
BIN_NAME="${BIN_NAME:-locrest-client}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

if [ "$(id -u)" -ne 0 ] && [ "$INSTALL_DIR" = "/usr/local/bin" ]; then
	INSTALL_DIR="${HOME}/.local/bin"
fi

OS=""
ARCH=""
BIN_TMP=""

if [ -t 1 ]; then
	RED='\033[0;31m'
	GREEN='\033[0;32m'
	YELLOW='\033[1;33m'
	BLUE='\033[0;34m'
	NC='\033[0m'
else
	RED='' GREEN='' YELLOW='' BLUE='' NC=''
fi

info() { printf "${GREEN}==>${NC} %s\n" "$*"; }
error() { printf "${RED}ERROR:${NC} %s\n" "$*" >&2; exit 1; }

detect_platform() {
	OS=$(uname -s | tr '[:upper:]' '[:lower:]')
	ARCH=$(uname -m)
	case "$ARCH" in
		x86_64|amd64) ARCH=amd64 ;;
		aarch64|arm64) ARCH=arm64 ;;
		armv7l|armv7) ARCH=arm ;;
		i386|i686) ARCH=386 ;;
		*) error "unsupported architecture: $ARCH" ;;
	esac
	case "$OS" in
		linux|darwin|freebsd) ;;
		*) error "unsupported OS: $OS" ;;
	esac
}

download() {
	local url="$1" file="$2"
	if command -v curl >/dev/null 2>&1; then curl -fsSL -o "$file" "$url"
	elif command -v wget >/dev/null 2>&1; then wget -q -O "$file" "$url"
	else error "curl or wget is required"
	fi
}

try_download() {
	local url="$1" file="$2"
	if download "$url" "$file" 2>/dev/null; then return 0; fi
	rm -f "$file"
	return 1
}

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
	else shasum -a 256 "$1" | awk '{print $1}'
	fi
}

download_binary() {
	local base_url="https://github.com/$OWNER/$REPO/releases/download/$VERSION"
	local tmp_dir asset candidate
	tmp_dir=$(mktemp -d)
	asset="$BIN_NAME-$OS-$ARCH"
	info "detected platform: $OS/$ARCH"
	info "downloading from $base_url"
	if try_download "$base_url/$asset" "$tmp_dir/$asset"; then BIN_TMP="$tmp_dir/$asset"
	elif try_download "$base_url/$asset.tar.gz" "$tmp_dir/$asset.tar.gz"; then
		tar -xzf "$tmp_dir/$asset.tar.gz" -C "$tmp_dir"
		candidate=$(find "$tmp_dir" -maxdepth 2 -type f -name "$BIN_NAME" | head -n 1)
		if [ -z "$candidate" ]; then
			error "archive did not contain $BIN_NAME"
		fi
		BIN_TMP="$candidate"
	else error "could not find release asset for $OS/$ARCH"
	fi
	if try_download "$base_url/$asset.sha256" "$tmp_dir/checksum"; then
		local expected actual
		expected=$(awk '{print $1}' "$tmp_dir/checksum")
		actual=$(sha256_file "$BIN_TMP")
		if [ "$actual" != "$expected" ]; then
			error "checksum mismatch"
		fi
		info "checksum verified"
	fi
}

install_binary() {
	info "installing binary to $INSTALL_DIR"
	mkdir -p "$INSTALL_DIR"
	cp "$BIN_TMP" "$INSTALL_DIR/$BIN_NAME"
	chmod +x "$INSTALL_DIR/$BIN_NAME"
	info "installed: $INSTALL_DIR/$BIN_NAME"
	if ! echo "$PATH" | grep -q "$INSTALL_DIR"; then
		echo "add $INSTALL_DIR to your PATH, e.g. export PATH=\"$INSTALL_DIR:\$PATH\""
	fi
}

banner() {
	printf "${BLUE}"
	printf '+-----------------------------------+\n'
	printf '|  Locrest Client Installer         |\n'
	printf '+-----------------------------------+\n'
	printf "${NC}\n"
}

main() {
	banner
	detect_platform
	download_binary
	install_binary
}

main "$@"
