package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/anish/cc-tool-reviewer/promptui"
)

type HookInput struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	CWD            string          `json:"cwd"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolUseID      string          `json:"tool_use_id"`
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

	// Read conversation context from transcript
	ctx := promptui.ReadContext(input.TranscriptPath, 6)

	// "Ask zone" — consult the AI reviewer
	decision, err := reviewer.Review(input.ToolName, input.ToolInput)
	if err != nil {
		slog.Error("reviewer error", "err", err)
		return
	}

	slog.Info("reviewed", "tool", input.ToolName, "decision", decision.Decision, "reason", decision.Reason)

	// If AI says "allow", pass it through
	if decision.Decision == "allow" {
		s.writeAllow(conn, "AI reviewer: "+decision.Reason)
		return
	}

	// AI says "ask" — show the native dialog instead of falling back to terminal
	result, err := promptui.ShowApproval(input.ToolName, input.ToolInput, decision.Reason, input.CWD, ctx)
	if err != nil {
		slog.Error("dialog error", "err", err)
		return
	}

	switch result.Decision {
	case promptui.DecisionApprove:
		slog.Info("user decided", "tool", input.ToolName, "decision", "allow")
		s.writeAllow(conn, "user approved")
	case promptui.DecisionDeny:
		slog.Info("user decided", "tool", input.ToolName, "decision", "deny")
		s.writeDecision(conn, "deny", "user denied")
	case promptui.DecisionLater:
		slog.Info("user decided", "tool", input.ToolName, "decision", "later")
		s.writeDecision(conn, "ask", "deferred to terminal prompt")
	}
}

func (s *Server) writeAllow(conn net.Conn, reason string) {
	s.writeDecision(conn, "allow", reason)
}

func (s *Server) writeDecision(conn net.Conn, decision, reason string) {
	output := HookOutput{
		HookSpecificOutput: &HookSpecificOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       decision,
			PermissionDecisionReason: reason,
		},
	}
	resp, _ := json.Marshal(output)
	conn.Write(resp)
}
