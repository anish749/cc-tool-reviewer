package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net"
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

	// Matched by allow or deny rules → empty response (let Claude Code handle it)
	if MatchesAny(input.ToolName, input.ToolInput, s.allow) {
		return
	}
	if MatchesAny(input.ToolName, input.ToolInput, s.deny) {
		return
	}

	// "Ask zone" — consult the reviewer
	decision, err := s.reviewer.Review(input.ToolName, input.ToolInput)
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
