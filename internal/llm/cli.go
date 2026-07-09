package llm

import "github.com/anish/cc-tool-reviewer/internal/claudecli"

// NewCLIClient returns a Client backed by the local `claude` CLI, which uses
// the machine's existing Claude login instead of an API key. Its Complete
// method supplies the raw reply; Text/JSON handling lives in the shared client.
func NewCLIClient(opts ...claudecli.Option) Client {
	cli := claudecli.New(opts...)
	return &client{generate: cli.Complete}
}
