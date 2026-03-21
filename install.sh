#!/bin/bash
set -e

cd "$(dirname "$0")"

echo "Building cc-tool-reviewer..."
go build -ldflags="-s -w" -o bin/cc-tool-reviewer .

echo "Installing to ~/.local/bin/cc-tool-reviewer..."
mkdir -p ~/.local/bin
cp bin/cc-tool-reviewer ~/.local/bin/cc-tool-reviewer

echo "Done. $(~/.local/bin/cc-tool-reviewer --help 2>&1 | head -1 || echo 'Installed.')"
