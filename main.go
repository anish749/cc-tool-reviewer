package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const DefaultSocketPath = "/tmp/cc-tool-reviewer.sock"

type kitchenTimeHandler struct {
	inner slog.Handler
}

func (h *kitchenTimeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *kitchenTimeHandler) Handle(ctx context.Context, r slog.Record) error {
	r.Time = r.Time.In(time.Local)
	return h.inner.Handle(ctx, r)
}

func (h *kitchenTimeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &kitchenTimeHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *kitchenTimeHandler) WithGroup(name string) slog.Handler {
	return &kitchenTimeHandler{inner: h.inner.WithGroup(name)}
}

func setupLogger() {
	replace := func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			a.Value = slog.StringValue(a.Value.Time().Format(time.Kitchen))
		}
		return a
	}
	inner := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		ReplaceAttr: replace,
	})
	slog.SetDefault(slog.New(&kitchenTimeHandler{inner: inner}))
}

func main() {
	socketPath := flag.String("socket", DefaultSocketPath, "Unix socket path")
	flag.Parse()

	setupLogger()

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

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	server := NewServer(listener, allow, deny, reviewer)
	go server.Serve()

	<-sig
	fmt.Println()
	slog.Info("shutting down")
}
