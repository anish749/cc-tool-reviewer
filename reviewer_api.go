package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// apiReviewer reviews tool calls via the Anthropic SDK (Haiku), authenticating
// with ANTHROPIC_API_KEY. It caches the system prompt to cut repeat cost.
type apiReviewer struct {
	client       *anthropic.Client
	systemPrompt string
}

func newAPIReviewer(systemPrompt string) *apiReviewer {
	client := anthropic.NewClient()
	return &apiReviewer{client: &client, systemPrompt: systemPrompt}
}

func (r *apiReviewer) Review(toolName string, toolInput json.RawMessage) (*ReviewDecision, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := r.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5,
		MaxTokens: 128,
		System: []anthropic.TextBlockParam{
			{
				Text: r.systemPrompt,
				CacheControl: anthropic.CacheControlEphemeralParam{
					Type: "ephemeral",
					TTL:  anthropic.CacheControlEphemeralTTLTTL1h,
				},
			},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(buildUserMessage(toolName, toolInput))),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("anthropic API error: %w", err)
	}
	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("empty response from API")
	}

	return parseReviewResponse(resp.Content[0].Text), nil
}
