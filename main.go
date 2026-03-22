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
	"github.com/lmittmann/tint"
)

const DefaultSocketPath = "/tmp/cc-tool-reviewer.sock"

func main() {
	socketPath := flag.String("socket", DefaultSocketPath, "Unix socket path")
	flag.Parse()

	slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		TimeFormat: time.Kitchen,
	})))

	// Always remove stale socket before starting
	os.Remove(*socketPath)

	allow, deny, rawAllow := LoadRules()
	slog.Info("loaded rules", "allow", len(allow), "deny", len(deny))

	reviewer := NewReviewer(rawAllow)

	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		log.Fatalf("listen error: %v", err)
	}
	defer listener.Close()
	defer os.Remove(*socketPath)

	slog.Info("listening", "socket", *socketPath)

	server := NewServer(listener, allow, deny, reviewer)
	go server.Serve()

	// Watch config directory for settings changes
	configDir := claudeConfigDir()
	watcher, err := configwatcher.New([]string{configDir}, func() {
		newAllow, newDeny, newRawAllow := LoadRules()
		newReviewer := NewReviewer(newRawAllow)
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
