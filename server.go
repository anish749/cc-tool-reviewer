package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/anish/cc-tool-reviewer/promptui"
)

// readTimeout bounds how long the server waits for the client (nc) to
// send its payload and half-close the connection. If nc fails to
// close its write side, io.ReadAll blocks forever without this.
const readTimeout = 5 * time.Second

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
	AdditionalContext        string `json:"additionalContext,omitempty"`
}

// ReviewLogEntry is a single JSONL record written to the review log file.
type ReviewLogEntry struct {
	Timestamp string          `json:"ts"`
	ToolName  string          `json:"tool"`
	ToolInput json.RawMessage `json:"input"`
	Decision  string          `json:"decision"`
	Reason    string          `json:"reason"`
}

type Server struct {
	listener     net.Listener
	mu           sync.RWMutex
	allow        []Rule
	deny         []Rule
	reviewer     *Reviewer
	projRules    ProjectRulesProvider
	reviewLogMu  sync.Mutex
	reviewLogPath string
}

func NewServer(listener net.Listener, allow, deny []Rule, reviewer *Reviewer, projRules ProjectRulesProvider, reviewLogPath string) *Server {
	return &Server{
		listener:      listener,
		allow:         allow,
		deny:          deny,
		reviewer:      reviewer,
		projRules:     projRules,
		reviewLogPath: reviewLogPath,
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

	conn.SetReadDeadline(time.Now().Add(readTimeout))
	data, err := io.ReadAll(conn)
	if err != nil {
		slog.Error("read error", "err", err)
		return
	}
	// Clear deadline so AI review and dialog can take as long as needed.
	conn.SetReadDeadline(time.Time{})

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
	globalAllow := s.allow
	globalDeny := s.deny
	reviewer := s.reviewer
	s.mu.RUnlock()

	// Merge project-level rules from cache (loaded on miss, invalidated by fsnotify)
	proj := s.projRules.Get(input.CWD)
	allow := append(globalAllow, proj.Allow...)
	deny := append(globalDeny, proj.Deny...)

	// Deny checked first — specific deny rules must shadow broad allow rules.
	// e.g. deny [git reset *] must block even when allow has [git:*].
	if MatchesAny(input.ToolName, input.ToolInput, deny) {
		logLocalMatch(input, "deny")
		s.writeResponse(conn, "deny", "matched deny rule", "")
		return
	}
	if MatchesAll(input.ToolName, input.ToolInput, allow) {
		logLocalMatch(input, "allow")
		s.writeAllow(conn, "matched allow rule")
		return
	}

	// Read conversation context from transcript
	ctx := promptui.ReadContext(input.TranscriptPath, 6)

	// "Ask zone" — consult the AI reviewer
	decision, err := reviewer.Review(input.ToolName, input.ToolInput)
	if err != nil {
		slog.Error("reviewer failed, deferring to terminal",
			"tool", input.ToolName,
			"input", ToolInputString(input.ToolName, input.ToolInput),
			"err", err,
		)
		s.writeResponse(conn, "ask", "reviewer error: "+err.Error(), "")
		return
	}

	slog.Info("reviewed", "tool", input.ToolName, "decision", decision.Decision, "method", "llm-reviewed", "reason", decision.Reason, "input", string(input.ToolInput))
	s.writeReviewLog(input.ToolName, input.ToolInput, decision)

	// If AI says "allow", pass it through
	if decision.Decision == "allow" {
		s.writeAllow(conn, "AI reviewer: "+decision.Reason)
		return
	}

	// AI says "ask" — show the native dialog instead of falling back to terminal
	result, err := promptui.ShowApproval(input.ToolName, input.ToolInput, decision.Reason, input.CWD, ctx)
	if err != nil {
		slog.Error("dialog failed, deferring to terminal", "tool", input.ToolName, "err", err)
		s.writeResponse(conn, "ask", "dialog error: "+err.Error(), "")
		return
	}

	switch result.Decision {
	case promptui.DecisionApprove:
		slog.Info("user decided", "tool", input.ToolName, "decision", "allow", "method", "user-reviewed", "feedback", result.Feedback)
		s.writeResponse(conn, "allow", "user approved", result.Feedback)
	case promptui.DecisionDeny:
		slog.Info("user decided", "tool", input.ToolName, "decision", "deny", "method", "user-reviewed", "feedback", result.Feedback)
		s.writeResponse(conn, "deny", "user denied", result.Feedback)
	case promptui.DecisionLater:
		slog.Info("user decided", "tool", input.ToolName, "decision", "later", "method", "user-reviewed")
		s.writeResponse(conn, "ask", "deferred to terminal prompt", "")
	}
}

// logLocalMatch logs a decision resolved by the allow/deny rules, with no AI call.
func logLocalMatch(input HookInput, decision string) {
	slog.Info("rule matched",
		"tool", input.ToolName,
		"decision", decision,
		"method", "static-check",
		"input", truncate(ToolInputString(input.ToolName, input.ToolInput), 200),
	)
}

func (s *Server) writeReviewLog(toolName string, toolInput json.RawMessage, decision *ReviewDecision) {
	if s.reviewLogPath == "" {
		return
	}
	entry := ReviewLogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		ToolName:  toolName,
		ToolInput: toolInput,
		Decision:  decision.Decision,
		Reason:    decision.Reason,
	}
	line, err := json.Marshal(entry)
	if err != nil {
		slog.Error("failed to marshal review log entry", "err", err)
		return
	}
	line = append(line, '\n')

	s.reviewLogMu.Lock()
	defer s.reviewLogMu.Unlock()

	f, err := os.OpenFile(s.reviewLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		slog.Error("failed to open review log", "path", s.reviewLogPath, "err", err)
		return
	}
	defer f.Close()

	if _, err := f.Write(line); err != nil {
		slog.Error("failed to write review log entry", "err", err)
	}
}

func (s *Server) writeAllow(conn net.Conn, reason string) {
	s.writeResponse(conn, "allow", reason, "")
}

func (s *Server) writeResponse(conn net.Conn, decision, reason, additionalContext string) {
	output := HookOutput{
		HookSpecificOutput: &HookSpecificOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       decision,
			PermissionDecisionReason: reason,
			AdditionalContext:        additionalContext,
		},
	}
	resp, err := json.Marshal(output)
	if err != nil {
		slog.Error("marshal response failed", "err", err)
		return
	}
	if _, err := conn.Write(resp); err != nil {
		slog.Error("write response failed", "decision", decision, "err", err)
	}
}
