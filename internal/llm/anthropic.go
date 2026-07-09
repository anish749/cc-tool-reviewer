package llm

import (
	"context"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

const (
	// anthropicModel is the model used for direct API calls (Haiku 4.5).
	anthropicModel = anthropic.ModelClaudeHaiku4_5
	// anthropicMaxTokens caps the reply. The reviewer's replies are a few
	// tokens of JSON, so a tight ceiling guards against runaway output.
	anthropicMaxTokens = 128
	// anthropicTimeout bounds a single API call.
	anthropicTimeout = 15 * time.Second
)

// anthropicBackend calls the Anthropic API directly, authenticating with
// ANTHROPIC_API_KEY (or ANTHROPIC_AUTH_TOKEN). It caches the system prompt so
// repeated calls with the same instructions are cheaper.
type anthropicBackend struct {
	client  anthropic.Client
	model   anthropic.Model
	timeout time.Duration
}

// NewAnthropicClient returns a Client backed by the Anthropic SDK.
func NewAnthropicClient() Client {
	b := &anthropicBackend{
		client:  anthropic.NewClient(),
		model:   anthropicModel,
		timeout: anthropicTimeout,
	}
	return &client{generate: b.generate}
}

func (b *anthropicBackend) generate(ctx context.Context, systemPrompt, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	resp, err := b.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     b.model,
		MaxTokens: anthropicMaxTokens,
		System: []anthropic.TextBlockParam{
			{
				Text: systemPrompt,
				CacheControl: anthropic.CacheControlEphemeralParam{
					Type: "ephemeral",
					TTL:  anthropic.CacheControlEphemeralTTLTTL1h,
				},
			},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("anthropic: %w", err)
	}
	if len(resp.Content) == 0 {
		return "", fmt.Errorf("anthropic: empty response")
	}
	return resp.Content[0].Text, nil
}
