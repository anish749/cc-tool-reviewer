package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anish/cc-tool-reviewer/internal/claudecli"
)

// cliReviewer reviews tool calls by shelling out to the `claude` CLI, using the
// machine's existing Claude login instead of ANTHROPIC_API_KEY.
type cliReviewer struct {
	client       *claudecli.Client
	systemPrompt string
}

func newCLIReviewer(systemPrompt string) *cliReviewer {
	return &cliReviewer{
		client:       claudecli.New(),
		systemPrompt: systemPrompt,
	}
}

func (r *cliReviewer) Review(toolName string, toolInput json.RawMessage) (*ReviewDecision, error) {
	text, err := r.client.Text(context.Background(), r.systemPrompt, buildUserMessage(toolName, toolInput))
	if err != nil {
		return nil, fmt.Errorf("claude CLI review: %w", err)
	}

	return parseReviewResponse(text), nil
}
