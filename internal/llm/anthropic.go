package llm

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// anthropicMaxTokens caps the reply. Reviewer replies are a few tokens of JSON,
// so a tight ceiling guards against runaway output. This is Anthropic-specific
// (the CLI has no equivalent), so it stays in this backend.
const anthropicMaxTokens = 128

// anthropicBackend calls the Anthropic API directly, authenticating with
// ANTHROPIC_API_KEY (or ANTHROPIC_AUTH_TOKEN). It caches the system prompt so
// repeated calls with the same instructions are cheaper.
type anthropicBackend struct {
	client anthropic.Client
	model  anthropic.Model
}

// NewAnthropicClient returns a Client backed by the Anthropic SDK. Model and
// timeout come from the shared options; the timeout is applied by the shared
// client layer, not by this backend.
func NewAnthropicClient(opts ...Option) Client {
	cfg := newConfig(opts...)
	b := &anthropicBackend{
		client: anthropic.NewClient(),
		model:  anthropic.Model(cfg.model),
	}
	return &client{generate: b.generate}
}

func (b *anthropicBackend) generate(ctx context.Context, systemPrompt, prompt string) (string, error) {
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
