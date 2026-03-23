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

# On macOS, build the SwiftUI approval dialog from source
if [ "${OS}" = "darwin" ]; then
  echo "Compiling native macOS dialog..."
  DIALOG_DIR="${TMPDIR}/dialog-src"
  mkdir -p "${DIALOG_DIR}"
  curl -sL "https://github.com/${REPO}/archive/refs/tags/${VERSION}.tar.gz" -o "${TMPDIR}/src.tar.gz"
  tar -xzf "${TMPDIR}/src.tar.gz" -C "${DIALOG_DIR}" --strip-components=1
  cd "${DIALOG_DIR}/approval-dialog" && swift build -c release --quiet
  cp "${DIALOG_DIR}/approval-dialog/.build/release/approval-dialog" "${INSTALL_DIR}/approval-dialog"
  chmod +x "${INSTALL_DIR}/approval-dialog"

  # Also compile legacy dialog
  swiftc "${DIALOG_DIR}/promptui/swift/approval.swift" -o "${INSTALL_DIR}/approval-dialog-legacy"
  chmod +x "${INSTALL_DIR}/approval-dialog-legacy"
fi

echo "Done. ${BINARY} ${VERSION} installed to ${INSTALL_DIR}"
