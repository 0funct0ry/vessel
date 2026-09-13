#!/bin/sh
# Install the latest Vessel release binary.
#
#   curl -fsSL https://raw.githubusercontent.com/0funct0ry/vessel/main/install.sh | sh
#
# Override the install directory with INSTALL_DIR (default: /usr/local/bin).
set -eu

REPO="0funct0ry/vessel"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

log() { printf '%s\n' "$*" >&2; }
die() { log "error: $*"; exit 1; }

need() {
  command -v "$1" >/dev/null 2>&1 || die "'$1' is required but not found"
}

need curl
need tar

os="$(uname -s)"
case "$os" in
  Linux) goos=linux ;;
  Darwin) goos=darwin ;;
  *) die "unsupported OS: $os" ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) goarch=amd64 ;;
  arm64|aarch64) goarch=arm64 ;;
  *) die "unsupported architecture: $arch" ;;
esac

version="${VESSEL_VERSION:-latest}"
if [ "$version" = "latest" ]; then
  api_url="https://api.github.com/repos/${REPO}/releases/latest"
else
  api_url="https://api.github.com/repos/${REPO}/releases/tags/${version}"
fi

log "fetching release metadata ($version)..."
tag="$(curl -fsSL "$api_url" | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
[ -n "$tag" ] || die "could not determine release tag from GitHub API"

ver="${tag#v}"
asset="vessel_${ver}_${goos}_${goarch}.tar.gz"
base_url="https://github.com/${REPO}/releases/download/${tag}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

log "downloading ${asset} (${tag})..."
curl -fsSL -o "${tmp_dir}/${asset}" "${base_url}/${asset}"
curl -fsSL -o "${tmp_dir}/checksums.txt" "${base_url}/checksums.txt"

log "verifying checksum..."
(
  cd "$tmp_dir"
  expected="$(grep " ${asset}\$" checksums.txt | cut -d ' ' -f1)"
  [ -n "$expected" ] || die "no checksum entry for ${asset}"
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$asset" | cut -d ' ' -f1)"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$asset" | cut -d ' ' -f1)"
  else
    die "need sha256sum or shasum to verify the download"
  fi
  [ "$expected" = "$actual" ] || die "checksum mismatch for ${asset}"
)

log "extracting..."
tar -xzf "${tmp_dir}/${asset}" -C "$tmp_dir" vessel

if [ -w "$INSTALL_DIR" ]; then
  install -m 755 "${tmp_dir}/vessel" "${INSTALL_DIR}/vessel"
else
  log "sudo required to write to ${INSTALL_DIR}"
  sudo install -m 755 "${tmp_dir}/vessel" "${INSTALL_DIR}/vessel"
fi

log "installed vessel ${tag} to ${INSTALL_DIR}/vessel"
"${INSTALL_DIR}/vessel" version
