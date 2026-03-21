package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

const DefaultSocketPath = "/tmp/cc-tool-reviewer.sock"

func main() {
	socketPath := flag.String("socket", DefaultSocketPath, "Unix socket path")
	flag.Parse()

	// Always remove stale socket before starting
	os.Remove(*socketPath)

	allow, deny, rawAllow := LoadRules()
	log.Printf("loaded %d allow rules, %d deny rules", len(allow), len(deny))

	reviewer := NewReviewer(rawAllow)

	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		log.Fatalf("listen error: %v", err)
	}
	defer listener.Close()
	defer os.Remove(*socketPath)

	log.Printf("listening on %s", *socketPath)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	server := NewServer(listener, allow, deny, reviewer)
	go server.Serve()

	<-sig
	fmt.Println()
	log.Println("shutting down")
}
