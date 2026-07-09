package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/anish/cc-tool-reviewer/configwatcher"
	"github.com/anish/cc-tool-reviewer/internal/reviewlog"
	"github.com/anish/cc-tool-reviewer/internal/selfupdate"
	"github.com/anish/cc-tool-reviewer/promptui"
	"github.com/lmittmann/tint"
)

// version and release are overridden at build time via ldflags.
// release is only set to "true" by goreleaser for official release builds.
var version = "dev"
var release = "false"

const DefaultSocketPath = "/tmp/cc-tool-reviewer.sock"

func main() {
	socketPath := flag.String("socket", DefaultSocketPath, "Unix socket path")
	legacyUI := flag.Bool("legacy-ui", false, "use the legacy AppKit dialog instead of SwiftUI")
	reviewLogPath := flag.String("llm-review-log", "", "path to a JSONL file for logging tool inputs sent to LLM review")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("cc-tool-reviewer", version)
		os.Exit(0)
	}

	// Handle subcommands
	if flag.NArg() > 0 && flag.Arg(0) == "update" {
		if err := selfupdate.Update(version, release == "true"); err != nil {
			fmt.Fprintf(os.Stderr, "update failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		TimeFormat: time.Kitchen,
	})))

	// Background update check (never blocks)
	selfupdate.AutoCheck(version, release == "true")

	// Always remove stale socket before starting
	os.Remove(*socketPath)

	promptui.UseLegacyUI = *legacyUI

	allow, deny, rawAllow := LoadRules()
	slog.Info("loaded rules", "allow", len(allow), "deny", len(deny))

	reviewer := NewReviewer(NewReviewLLM(), rawAllow)

	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		log.Fatalf("listen error: %v", err)
	}
	defer listener.Close()
	defer os.Remove(*socketPath)

	slog.Info("listening", "socket", *socketPath)

	projCache, err := NewProjectCache(5 * time.Hour)
	if err != nil {
		log.Fatalf("project cache: %v", err)
	}
	defer projCache.Stop()

	rl := reviewlog.New(*reviewLogPath)
	defer rl.Close()

	server := NewServer(listener, allow, deny, reviewer, projCache, rl)
	go server.Serve()

	// Watch config directory for settings changes
	configDir := claudeConfigDir()
	watcher, err := configwatcher.New([]string{configDir}, func() {
		newAllow, newDeny, newRawAllow := LoadRules()
		newReviewer := NewReviewer(NewReviewLLM(), newRawAllow)
		server.Reload(newAllow, newDeny, newReviewer)
		slog.Info("reloaded rules", "allow", len(newAllow), "deny", len(newDeny))
	})
	if err != nil {
		slog.Warn("config watcher failed to start", "err", err)
	} else {
		watcher.Start()
		defer watcher.Stop()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	<-sig
	fmt.Println()
	slog.Info("shutting down")
}
