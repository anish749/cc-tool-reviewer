# Makefile for local development — builds from source and installs.
# End users should use install.sh instead, which downloads a pre-built release.
INSTALL_DIR ?= $(HOME)/.local/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build install clean

build:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o bin/cc-tool-reviewer .
ifeq ($(shell uname -s),Darwin)
	cd approval-dialog && swift build -c release
	cp approval-dialog/.build/release/approval-dialog bin/approval-dialog
endif

# install(1) writes a temp file and renames it into place, so a running daemon
# keeps its own inode. cp overwrites in place, which macOS SIGKILLs on exec.
install: build
	install -d $(INSTALL_DIR)
	install -m 755 bin/cc-tool-reviewer $(INSTALL_DIR)/cc-tool-reviewer
ifeq ($(shell uname -s),Darwin)
	install -m 755 bin/approval-dialog $(INSTALL_DIR)/approval-dialog
endif

clean:
	rm -rf bin/
	cd approval-dialog && swift package clean
