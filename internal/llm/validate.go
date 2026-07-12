package llm

import (
	"errors"
	"fmt"
	"os"

	"github.com/anish/cc-tool-reviewer/internal/llm/claudecli"
)

// EnvUseCLIClient, when set to a non-empty value, routes review through the
// local `claude` CLI (using the machine's Claude login) instead of the
// Anthropic SDK (which authenticates with ANTHROPIC_API_KEY).
const EnvUseCLIClient = "USE_CLAUDE_CLI_CLIENT"

// anthropicEnvVars are the variables the Anthropic SDK reads at client
// construction. Any one of them makes the SDK backend usable —
// ANTHROPIC_BASE_URL alone covers gateways that need no token.
var anthropicEnvVars = []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL"}

// Validate reports whether the environment configures a usable reviewer
// backend. Without this check a misconfigured process starts fine and every
// review fails at call time — invisibly, once the process logs to a file.
func Validate() error {
	if os.Getenv(EnvUseCLIClient) != "" {
		if err := claudecli.Available(); err != nil {
			return fmt.Errorf("%s is set but %w", EnvUseCLIClient, err)
		}
		return nil
	}
	for _, name := range anthropicEnvVars {
		if os.Getenv(name) != "" {
			return nil
		}
	}
	return errors.New("no reviewer backend configured: set ANTHROPIC_API_KEY (or ANTHROPIC_AUTH_TOKEN / ANTHROPIC_BASE_URL for a gateway), or USE_CLAUDE_CLI_CLIENT=1 to use the local claude CLI")
}
