INSTALL_DIR ?= $(HOME)/.local/bin

.PHONY: build install clean

build:
	go build -ldflags="-s -w" -o bin/cc-tool-reviewer .
ifeq ($(shell uname -s),Darwin)
	swiftc promptui/swift/approval.swift -o bin/approval-dialog
endif

install: build
	mkdir -p $(INSTALL_DIR)
	cp bin/cc-tool-reviewer $(INSTALL_DIR)/cc-tool-reviewer
ifeq ($(shell uname -s),Darwin)
	cp bin/approval-dialog $(INSTALL_DIR)/approval-dialog
endif

clean:
	rm -rf bin/
