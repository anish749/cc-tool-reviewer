package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"sync"
)

type HookInput struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

type HookOutput struct {
	HookSpecificOutput *HookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

type HookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

type Server struct {
	listener net.Listener
	mu       sync.RWMutex
	allow    []Rule
	deny     []Rule
	reviewer *Reviewer
}

func NewServer(listener net.Listener, allow, deny []Rule, reviewer *Reviewer) *Server {
	return &Server{
		listener: listener,
		allow:    allow,
		deny:     deny,
		reviewer: reviewer,
	}
}

// Reload swaps the allow/deny rules and reviewer with new values.
// Safe to call while the server is handling requests.
func (s *Server) Reload(allow, deny []Rule, reviewer *Reviewer) {
	s.mu.Lock()
	s.allow = allow
	s.deny = deny
	s.reviewer = reviewer
	s.mu.Unlock()
}

func (s *Server) Serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// Listener closed
			return
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()

	data, err := io.ReadAll(conn)
	if err != nil {
		slog.Error("read error", "err", err)
		return
	}

	var input HookInput
	if err := json.Unmarshal(data, &input); err != nil {
		slog.Error("json parse error", "err", err)
		return
	}

	// Tools that are always allowed — no matching or AI needed
	if AutoAllow(input.ToolName, input.ToolInput) {
		s.writeAllow(conn, "auto-allowed tool type")
		return
	}

	s.mu.RLock()
	allow := s.allow
	deny := s.deny
	reviewer := s.reviewer
	s.mu.RUnlock()

	// Matched by allow or deny rules → empty response (let Claude Code handle it)
	if MatchesAny(input.ToolName, input.ToolInput, allow) {
		return
	}
	if MatchesAny(input.ToolName, input.ToolInput, deny) {
		return
	}

	// "Ask zone" — consult the reviewer
	decision, err := reviewer.Review(input.ToolName, input.ToolInput)
	if err != nil {
		slog.Error("reviewer error", "err", err)
		// Fall through to normal permission prompt
		return
	}

	slog.Info("reviewed", "tool", input.ToolName, "decision", decision.Decision, "reason", decision.Reason)

	output := HookOutput{
		HookSpecificOutput: &HookSpecificOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       decision.Decision,
			PermissionDecisionReason: "AI reviewer: " + decision.Reason,
		},
	}

	resp, err := json.Marshal(output)
	if err != nil {
		slog.Error("json marshal error", "err", err)
		return
	}

	conn.Write(resp)
}

func (s *Server) writeAllow(conn net.Conn, reason string) {
	output := HookOutput{
		HookSpecificOutput: &HookSpecificOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "allow",
			PermissionDecisionReason: reason,
		},
	}
	resp, _ := json.Marshal(output)
	conn.Write(resp)
}
