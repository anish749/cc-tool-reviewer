package llm

import (
	"fmt"
	"os"

	"github.com/anish/cc-tool-reviewer/internal/llm/claudecli"
)

// anthropicEnvVars are the variables the Anthropic SDK reads at client
// construction. Any one of them makes the SDK backend usable —
// ANTHROPIC_BASE_URL alone covers gateways that need no token.
var anthropicEnvVars = []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL"}

// UseAnthropicSDK reports whether the environment selects the Anthropic SDK
// backend over the default local `claude` CLI backend.
func UseAnthropicSDK() bool {
	for _, name := range anthropicEnvVars {
		if os.Getenv(name) != "" {
			return true
		}
	}
	return false
}

// Validate reports whether the environment configures a usable reviewer
// backend. Without this check a misconfigured process starts fine and every
// review fails at call time — invisibly, once the process logs to a file.
func Validate() error {
	if UseAnthropicSDK() {
		return nil
	}
	if err := claudecli.Available(); err != nil {
		return fmt.Errorf("claude CLI is the default reviewer backend but %w (set ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN, or ANTHROPIC_BASE_URL to use the Anthropic API instead)", err)
	}
	return nil
}
