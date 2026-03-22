#!/usr/bin/env bash
set -euo pipefail

REPO="anish749/cc-tool-reviewer"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
BINARY="cc-tool-reviewer"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: ${ARCH}" >&2; exit 1 ;;
esac

VERSION="$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | head -1 | cut -d'"' -f4)"
if [ -z "${VERSION}" ]; then
  echo "Failed to fetch latest release version" >&2
  exit 1
fi
VERSION_NUM="${VERSION#v}"

TARBALL="${BINARY}_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "${TMPDIR}"' EXIT

echo "Downloading ${BINARY} ${VERSION} (${OS}/${ARCH})..."
curl -sL "${URL}" -o "${TMPDIR}/${TARBALL}"

echo "Installing to ${INSTALL_DIR}..."
tar -xzf "${TMPDIR}/${TARBALL}" -C "${TMPDIR}"
mkdir -p "${INSTALL_DIR}"
mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
chmod +x "${INSTALL_DIR}/${BINARY}"

# On macOS, compile the native approval dialog from source
if [ "${OS}" = "darwin" ]; then
  SWIFT_SRC_URL="https://raw.githubusercontent.com/anish749/cc-tool-reviewer/${VERSION}/promptui/swift/approval.swift"
  echo "Compiling native macOS dialog..."
  curl -sL "${SWIFT_SRC_URL}" -o "${TMPDIR}/approval.swift"
  swiftc "${TMPDIR}/approval.swift" -o "${INSTALL_DIR}/approval-dialog"
  chmod +x "${INSTALL_DIR}/approval-dialog"
fi

echo "Done. ${BINARY} ${VERSION} installed to ${INSTALL_DIR}"
